package httpapi

import (
	"context"
	"net/http"
	"strings"
	"time"
)

func (h Handler) AdminTariffDetail(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.requireAdminSection(w, r, "tariffs", r.Method != http.MethodGet)
	if !ok {
		return
	}
	id, suffix, err := parseIDFromPath(r.URL.Path, "/api/admin/tariffs/")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid tariff id"})
		return
	}
	// Bulk AI change for all modes of a tariff: one UPDATE over the
	// tariff_mode membership, so nobody has to click provider/model in every mode by hand.
	if suffix == "set-modes-ai" {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
			return
		}
		var req struct {
			Provider string `json:"provider"`
			Model    string `json:"model"`
		}
		if err := decodeJSONStrict(w, r, 16<<10, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid payload"})
			return
		}
		// Not normalizeAIProvider: a silent vsegpt default on a typo is dangerous for
		// a bulk operation, so an explicitly valid provider is required.
		provider := strings.ToLower(strings.TrimSpace(req.Provider))
		if !aiProviders[provider] {
			writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "unknown provider"})
			return
		}
		model, err := normalizeAIModelID(req.Model)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid model: " + err.Error()})
			return
		}
		var tariffName string
		if err := h.DB.QueryRow(r.Context(), `select name from tariffs where id=$1`, id).Scan(&tariffName); err != nil {
			writeJSON(w, http.StatusNotFound, map[string]any{"ok": false, "error": "tariff not found"})
			return
		}
		cmd, err := h.DB.Exec(r.Context(), `
			update modes
			set ai_provider = $2, ai_model = $3, updated_at = now()
			where id in (select mode_id from tariff_mode where tariff_id = $1)`, id, provider, model)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		_, _ = h.DB.Exec(r.Context(), `delete from public.public_demo_modes_cache where cache_key = 'home_demo_modes'`)
		h.clearPublicDemoModesCache()
		h.clearModesCache()
		clearAdminExportOptionsCache()
		h.c.adminModes.clear()
		h.c.adminModeModelStats.clear()
		h.writeAdminAudit(r.Context(), r, actor.ID, "admin.tariff.set_modes_ai", "tariff", &id, map[string]any{"provider": provider, "model": model, "updated": cmd.RowsAffected()})
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "updated": cmd.RowsAffected()})
		return
	}
	switch r.Method {
	case http.MethodGet:
		var item map[string]any
		var name, description, tariffType, limitType, groupName string
		var monthlyPrice float64
		var dailyLimit *int64
		var groupID *int64
		var firstModeID *int64
		var available bool
		var createdAt time.Time
		var archivedAt *time.Time
		err := h.DB.QueryRow(r.Context(), `
			select t.name, coalesce(t.description, ''), coalesce(t.tariff_type, ''), coalesce(t.monthly_price, 0), t.daily_message_limit, coalesce(t.limit_type, ''), t.group_id, t.available_for_subscription, t.created_at, t.archived_at, coalesce(tg.name, ''), t.first_mode_id
			from tariffs t left join tariff_groups tg on tg.id = t.group_id where t.id = $1`, id).Scan(&name, &description, &tariffType, &monthlyPrice, &dailyLimit, &limitType, &groupID, &available, &createdAt, &archivedAt, &groupName, &firstModeID)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]any{"ok": false, "error": "tariff not found"})
			return
		}
		modeIDs, _ := h.queryTariffModeIDs(r.Context(), id)
		item = map[string]any{"id": id, "name": name, "description": description, "tariffType": tariffType, "monthlyPrice": monthlyPrice, "dailyMessageLimit": dailyLimit, "limitType": limitType, "groupId": groupID, "groupName": groupName, "availableForSubscription": available, "createdAt": createdAt, "archivedAt": archivedAt, "modeIds": modeIDs, "firstModeId": firstModeID}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "tariff": item})
	case http.MethodPatch:
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
		dailyMessageLimit := normalizedDailyMessageLimit(req.DailyMessageLimit)
		modeIDs := uniquePositiveIDs(req.ModeIDs)
		firstModeIDPatch, ok := resolveFirstModeID(req.FirstModeID, modeIDs)
		if !ok {
			writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "firstModeId must be one of modeIds"})
			return
		}
		tx, err := beginTxTimeout(r.Context(), h.DB, dbAcquireTimeout)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		defer tx.Rollback(r.Context())
		_, err = tx.Exec(r.Context(), `update tariffs set name=$2, description=$3, tariff_type=$4, monthly_price=$5, daily_message_limit=$6, limit_type=$7, group_id=$8, available_for_subscription=$9, archived_at=case when $9 then null else coalesce(archived_at, now()) end, first_mode_id=$10, updated_at=now() where id=$1`, id, name, strings.TrimSpace(req.Description), tariffType, req.MonthlyPrice, dailyMessageLimit, limitType, req.GroupID, req.AvailableForSubscription, firstModeIDPatch)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		_, _ = tx.Exec(r.Context(), `delete from tariff_mode where tariff_id=$1`, id)
		for _, modeID := range modeIDs {
			if _, err := tx.Exec(r.Context(), `insert into tariff_mode (tariff_id, mode_id, daily_message_limit, created_at) values ($1,$2,$3,now())`, id, modeID, dailyMessageLimit); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
				return
			}
		}
		if err := tx.Commit(r.Context()); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		h.writeAdminAudit(r.Context(), r, actor.ID, "admin.tariff.patch", "tariff", &id, nil)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	case http.MethodDelete:
		var archivedAt *time.Time
		var available bool
		if err := h.DB.QueryRow(r.Context(), `select archived_at, available_for_subscription from tariffs where id=$1`, id).Scan(&archivedAt, &available); err != nil {
			writeJSON(w, http.StatusNotFound, map[string]any{"ok": false, "error": "tariff not found"})
			return
		}
		if archivedAt != nil || !available {
			tx, err := beginTxTimeout(r.Context(), h.DB, dbAcquireTimeout)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
				return
			}
			defer tx.Rollback(r.Context())
			_, _ = tx.Exec(r.Context(), `delete from tariff_mode where tariff_id=$1`, id)
			cmd, err := tx.Exec(r.Context(), `delete from tariffs where id=$1`, id)
			if err != nil {
				writeJSON(w, http.StatusConflict, map[string]any{"ok": false, "error": "tariff has related subscriptions or invoices", "details": err.Error()})
				return
			}
			if cmd.RowsAffected() == 0 {
				writeJSON(w, http.StatusNotFound, map[string]any{"ok": false, "error": "tariff not found"})
				return
			}
			if err := tx.Commit(r.Context()); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
				return
			}
			h.writeAdminAudit(r.Context(), r, actor.ID, "admin.tariff.destroy", "tariff", &id, nil)
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "deleted": true})
			return
		}
		_, err = h.DB.Exec(r.Context(), `update tariffs set available_for_subscription=false, archived_at=coalesce(archived_at, now()), updated_at=now() where id=$1`, id)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		h.writeAdminAudit(r.Context(), r, actor.ID, "admin.tariff.archive", "tariff", &id, nil)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "archived": true})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
	}
}

func (h Handler) queryTariffModeIDs(ctx context.Context, tariffID int64) ([]int64, error) {
	rows, err := h.DB.Query(ctx, `select mode_id from tariff_mode where tariff_id=$1 order by mode_id`, tariffID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}
