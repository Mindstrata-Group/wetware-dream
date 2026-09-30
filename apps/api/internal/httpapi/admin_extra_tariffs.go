package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
)

func (h Handler) AdminTariffGroups(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.requireAdminSection(w, r, "tariffs", r.Method != http.MethodGet)
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		if writeNamedCached(w, "admin_tariff_groups", &h.c.adminTariffGroups) {
			return
		}
		rows, err := h.DB.Query(r.Context(), `select id, name, coalesce(description, ''), available_for_subscription, sort_order from tariff_groups order by sort_order asc, id asc`)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		defer rows.Close()
		out := []map[string]any{}
		for rows.Next() {
			var id int64
			var name, description string
			var available bool
			var sortOrder int
			if err := rows.Scan(&id, &name, &description, &available, &sortOrder); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
				return
			}
			out = append(out, map[string]any{"id": id, "name": name, "description": description, "availableForSubscription": available, "sortOrder": sortOrder})
		}
		body, _ := json.Marshal(map[string]any{"ok": true, "groups": out})
		h.c.adminTariffGroups.set(body, adminConfigCacheTTL)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	case http.MethodPost:
		var req adminTariffGroupRequest
		if err := decodeJSONStrict(w, r, 64<<10, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid payload"})
			return
		}
		name := strings.TrimSpace(req.Name)
		if name == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "name is required"})
			return
		}
		var id int64
		err := h.DB.QueryRow(r.Context(), `
			insert into tariff_groups (name, description, sort_order, available_for_subscription, created_at, updated_at)
			values ($1, $2, coalesce((select max(sort_order) + 10 from tariff_groups), 10), $3, now(), now())
			returning id`, name, strings.TrimSpace(req.Description), req.AvailableForSubscription).Scan(&id)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		h.c.adminTariffGroups.clear()
		h.writeAdminAudit(r.Context(), r, actor.ID, "admin.tariff_group.create", "tariff_group", &id, map[string]any{"name": name})
		writeJSON(w, http.StatusCreated, map[string]any{"ok": true, "groupId": id})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
	}
}

func (h Handler) AdminTariffGroupDetail(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.requireAdminSection(w, r, "tariffs", r.Method != http.MethodGet)
	if !ok {
		return
	}
	id, _, err := parseIDFromPath(r.URL.Path, "/api/admin/tariff-groups/")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid group id"})
		return
	}
	switch r.Method {
	case http.MethodPatch:
		var req adminTariffGroupRequest
		if err := decodeJSONStrict(w, r, 64<<10, &req); err != nil || strings.TrimSpace(req.Name) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "name is required"})
			return
		}
		_, err = h.DB.Exec(r.Context(), `update tariff_groups set name=$2, description=$3, available_for_subscription=$4, updated_at=now() where id=$1`, id, strings.TrimSpace(req.Name), strings.TrimSpace(req.Description), req.AvailableForSubscription)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		h.c.adminTariffGroups.clear()
		h.writeAdminAudit(r.Context(), r, actor.ID, "admin.tariff_group.patch", "tariff_group", &id, nil)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	case http.MethodDelete:
		var name string
		if err := h.DB.QueryRow(r.Context(), `select name from tariff_groups where id=$1`, id).Scan(&name); err != nil {
			writeJSON(w, http.StatusNotFound, map[string]any{"ok": false, "error": "group not found"})
			return
		}
		if protectedTariffGroupName(name) {
			writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "base participants group is protected"})
			return
		}
		tx, err := beginTxTimeout(r.Context(), h.DB, dbAcquireTimeout)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		defer tx.Rollback(r.Context())
		_, _ = tx.Exec(r.Context(), `update tariffs set group_id = null, updated_at = now() where group_id = $1`, id)
		if _, err := tx.Exec(r.Context(), `delete from tariff_groups where id=$1`, id); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		if err := tx.Commit(r.Context()); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		h.c.adminTariffGroups.clear()
		h.writeAdminAudit(r.Context(), r, actor.ID, "admin.tariff_group.delete", "tariff_group", &id, map[string]any{"name": name})
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
	}
}

func (h Handler) adminCreateTariff(w http.ResponseWriter, r *http.Request, actor AuthenticatedUser) {
	var req adminTariffRequest
	if err := decodeJSONStrict(w, r, 128<<10, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid payload"})
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "name is required"})
		return
	}
	tariffType := strings.TrimSpace(req.TariffType)
	if tariffType == "" {
		tariffType = "regular"
	}
	limitType := strings.TrimSpace(req.LimitType)
	if limitType == "" {
		limitType = "shared"
	}
	if tariffType != "regular" && tariffType != "promo" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "tariffType must be regular or promo"})
		return
	}
	if limitType != "shared" && limitType != "split" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "limitType must be shared or split"})
		return
	}
	modeIDs := uniquePositiveIDs(req.ModeIDs)
	firstModeID, ok := resolveFirstModeID(req.FirstModeID, modeIDs)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "firstModeId must be one of modeIds"})
		return
	}
	dailyMessageLimit := normalizedDailyMessageLimit(req.DailyMessageLimit)
	tx, err := beginTxTimeout(r.Context(), h.DB, dbAcquireTimeout)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer tx.Rollback(r.Context())
	var id int64
	err = tx.QueryRow(r.Context(), `
		insert into tariffs (name, description, monthly_price, daily_message_limit, limit_type, group_id, available_for_subscription, tariff_type, first_mode_id, created_at, updated_at)
		values ($1, $2, $3, $4, $5, $6, $7, $8, $9, now(), now())
		returning id`, name, strings.TrimSpace(req.Description), req.MonthlyPrice, dailyMessageLimit, limitType, req.GroupID, req.AvailableForSubscription, tariffType, firstModeID).Scan(&id)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	for _, modeID := range modeIDs {
		_, err = tx.Exec(r.Context(), `insert into tariff_mode (tariff_id, mode_id, daily_message_limit, created_at) values ($1, $2, $3, now())`, id, modeID, dailyMessageLimit)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
			return
		}
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	h.writeAdminAudit(r.Context(), r, actor.ID, "admin.tariff.create", "tariff", &id, map[string]any{"name": name})
	writeJSON(w, http.StatusCreated, map[string]any{"ok": true, "tariffId": id})
}
