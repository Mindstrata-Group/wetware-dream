package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const adminPromoListCacheTTL = 30 * time.Second

func (h Handler) AdminPromocodes(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.requireOwnerAdmin(w, r)
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		q := strings.TrimSpace(r.URL.Query().Get("q"))
		dateFrom := strings.TrimSpace(r.URL.Query().Get("dateFrom"))
		dateTo := strings.TrimSpace(r.URL.Query().Get("dateTo"))
		offset, _ := strconv.ParseInt(r.URL.Query().Get("offset"), 10, 64)
		limit, _ := strconv.ParseInt(r.URL.Query().Get("limit"), 10, 64)
		if limit < 1 || limit > 500 {
			limit = 100
		}
		if offset < 0 {
			offset = 0
		}

		cacheKey := fmt.Sprintf("%s|%s|%s|%d|%d", q, dateFrom, dateTo, offset, limit)
		if body, ok := h.c.adminPromoList.get(cacheKey); ok {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(body)
			return
		}

		// Build the WHERE clause with accumulated $ parameters
		args := []any{q}
		where := []string{
			`($1 = '' or upper(p.code) like upper('%' || $1 || '%') or coalesce(p.comment, '') ilike '%' || $1 || '%' or coalesce(p.purpose, '') ilike '%' || $1 || '%')`,
		}
		if dateFrom != "" {
			args = append(args, dateFrom)
			where = append(where, fmt.Sprintf(`p.active_from >= $%d::date`, len(args)))
		}
		if dateTo != "" {
			args = append(args, dateTo)
			where = append(where, fmt.Sprintf(`p.active_from <= $%d::date`, len(args)))
		}
		whereStr := strings.Join(where, " and ")

		var total int64
		_ = h.DB.QueryRow(r.Context(), `select count(*) from promocodes p where `+whereStr, args...).Scan(&total)

		args = append(args, limit, offset)
		limitPos := len(args) - 1
		offsetPos := len(args)
		rows, err := h.DB.Query(r.Context(), `
			select p.id, p.code, p.grants_type, p.target_id, p.max_uses, p.used_count, p.active_from, p.active_to, p.duration::text, p.daily_message_limit, p.access_priority, p.limit_type, p.price,
			       coalesce(p.comment, ''), coalesce(p.purpose, ''), p.summary_limit, coalesce(p.temporary_admin_enabled, false),
			       (select max(used_at) from promocode_usages u where u.promocode_id = p.id), p.first_mode_id
			from promocodes p
			where `+whereStr+`
			order by p.id desc
			limit $`+strconv.Itoa(limitPos)+` offset $`+strconv.Itoa(offsetPos), args...)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		defer rows.Close()
		out := []map[string]any{}
		for rows.Next() {
			var id, targetID int64
			var code, grantsType, duration, limitType string
			var maxUses, usedCount, accessPriority int
			var activeFrom, activeTo, lastUsedAt *time.Time
			var dailyLimit, summaryLimit *int64
			var firstModeID *int64
			var price *float64
			var comment, purpose string
			var temporaryAdmin bool
			if err := rows.Scan(&id, &code, &grantsType, &targetID, &maxUses, &usedCount, &activeFrom, &activeTo, &duration, &dailyLimit, &accessPriority, &limitType, &price, &comment, &purpose, &summaryLimit, &temporaryAdmin, &lastUsedAt, &firstModeID); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
				return
			}
			targetIDs, err := h.promocodeTargetIDs(r.Context(), id, targetID)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
				return
			}
			modeIDs, err := h.promocodeModeIDsForTargets(r.Context(), grantsType, targetIDs)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
				return
			}
			out = append(out, map[string]any{"id": id, "code": code, "grantsType": grantsType, "targetId": targetID, "targetIds": targetIDs, "modeIds": modeIDs, "firstModeId": firstModeID, "maxUses": maxUses, "usedCount": usedCount, "activeFrom": activeFrom, "activeTo": activeTo, "duration": duration, "dailyMessageLimit": dailyLimit, "accessPriority": accessPriority, "limitType": limitType, "price": price, "comment": comment, "purpose": purpose, "summaryLimit": summaryLimit, "temporaryAdmin": temporaryAdmin, "lastUsedAt": lastUsedAt, "active": promocodeIsActive(activeFrom, activeTo, maxUses, usedCount), "applyUrl": promocodeApplyURL(r, code), "temporaryAdminUrl": h.temporaryAdminURL(r, code)})
		}
		body, _ := json.Marshal(map[string]any{"ok": true, "promocodes": out, "total": total, "offset": offset, "limit": limit})
		h.c.adminPromoList.set(cacheKey, body, adminPromoListCacheTTL)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	case http.MethodPost:
		var req adminPromocodeRequest
		if err := decodeJSONStrict(w, r, 128<<10, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid payload"})
			return
		}
		created, err := h.createAdminPromocodes(r, actor, req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		h.c.adminPromoList.clear()
		writeJSON(w, http.StatusCreated, map[string]any{"ok": true, "promocodes": created})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
	}
}
