package httpapi

import (
	"fmt"
	"net/http"
	"strings"
)

func (h Handler) AdminPromocodeDetail(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.requireOwnerAdmin(w, r)
	if !ok {
		return
	}
	id, _, err := parseIDFromPath(r.URL.Path, "/api/admin/promocodes/")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid promocode id"})
		return
	}
	switch r.Method {
	case http.MethodDelete:
		_, err = h.DB.Exec(r.Context(), `update promocodes set active_to = now(), updated_at = now() where id = $1`, id)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		clearAdminExportOptionsCache()
		h.c.adminPromoList.clear()
		h.writeAdminAudit(r.Context(), r, actor.ID, "admin.promocode.deactivate", "promocode", &id, nil)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	case http.MethodPatch:
		var req adminPromocodePatchRequest
		if err := decodeJSONStrict(w, r, 16<<10, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid payload"})
			return
		}
		result, err := h.updateAdminPromocode(r, id, req)
		if err != nil {
			writeJSON(w, adminPromocodeUpdateStatus(err), map[string]any{"ok": false, "error": err.Error()})
			return
		}
		clearAdminExportOptionsCache()
		h.c.adminPromoList.clear()
		action := "admin.promocode.update"
		if result.OnlyActivation {
			action = "admin.promocode.activate"
		}
		h.writeAdminAudit(r.Context(), r, actor.ID, action, "promocode", &id, map[string]any{
			"activeTo":            result.ActiveTo,
			"maxUses":             result.MaxUses,
			"updateScope":         result.UpdateScope,
			"targetsUpdated":      result.TargetsUpdated,
			"upsertedAccesses":    result.UpsertedAccesses,
			"deactivatedAccesses": result.DeactivatedAccesses,
		})
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":                  true,
			"updatedActivations":  result.UpsertedAccesses,
			"deactivatedAccesses": result.DeactivatedAccesses,
			"roleUpgrades":        result.RoleUpgrades,
			"updateScope":         result.UpdateScope,
		})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
	}
}

func (h Handler) createAdminPromocodes(r *http.Request, actor AuthenticatedUser, req adminPromocodeRequest) ([]map[string]any, error) {
	bulk := req.BulkCount
	if bulk <= 0 {
		bulk = 1
	}
	if bulk > 200 {
		return nil, fmt.Errorf("bulkCount must be <= 200")
	}
	grantType := strings.TrimSpace(req.GrantsType)
	if grantType == "" {
		grantType = "tariff"
	}
	targetIDs := uniquePositiveIDs(req.TargetIDs)
	if len(targetIDs) == 0 && grantType != "admin_role" && grantType != "all" && grantType != "all_modes" && grantType != "all_modes_access" {
		return nil, fmt.Errorf("select at least one promocode target")
	}
	if req.DurationDays <= 0 {
		req.DurationDays = 30
	}
	if req.MaxUses <= 0 {
		req.MaxUses = 1
	}
	if req.AccessPriority == 0 {
		req.AccessPriority = 100
	}
	dailyMessageLimit := normalizedDailyMessageLimit(req.DailyMessageLimit)
	limitType := strings.TrimSpace(req.LimitType)
	if limitType == "" {
		limitType = "shared"
	}
	activeFrom, err := parseOptionalAdminTime(req.ActiveFrom)
	if err != nil {
		return nil, err
	}
	activeTo, err := parseOptionalAdminEndTime(req.ActiveTo)
	if err != nil {
		return nil, err
	}
	if grantType == "tariff" {
		for _, targetID := range targetIDs {
			var ok bool
			if err := h.DB.QueryRow(r.Context(), `select exists(select 1 from tariffs where id=$1 and archived_at is null)`, targetID).Scan(&ok); err != nil || !ok {
				return nil, fmt.Errorf("tariff %d is archived or not found", targetID)
			}
		}
	}
	created := []map[string]any{}
	legacyTargetID := int64(0)
	if len(targetIDs) > 0 {
		legacyTargetID = targetIDs[0]
	}
	for i := 0; i < bulk; i++ {
		code := strings.ToUpper(strings.TrimSpace(req.Code))
		if req.AutoGenerate || code == "" || bulk > 1 {
			code = randomPromoCode()
		}
		tx, err := beginTxTimeout(r.Context(), h.DB, dbAcquireTimeout)
		if err != nil {
			return nil, err
		}
		var id int64
		var firstModeID *int64
		if req.FirstModeID != nil && *req.FirstModeID > 0 {
			firstModeID = req.FirstModeID
		}
		err = tx.QueryRow(r.Context(), `
			insert into promocodes (code, max_uses, used_count, active_from, active_to, duration, daily_message_limit, access_priority, grants_type, target_id, limit_type, price, comment, purpose, summary_limit, temporary_admin_enabled, temporary_admin_note, first_mode_id, created_at, updated_at)
			values ($1, $2, 0, $3, $4, ($5::int || ' days')::interval, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, now(), now())
			returning id`, code, req.MaxUses, activeFrom, activeTo, req.DurationDays, dailyMessageLimit, req.AccessPriority, grantType, legacyTargetID, limitType, req.Price, strings.TrimSpace(req.Comment), strings.TrimSpace(req.Purpose), req.SummaryLimit, req.TemporaryAdmin, temporaryAdminDefaultNote(req.Purpose), firstModeID).Scan(&id)
		if err == nil && len(targetIDs) > 0 {
			_, err = tx.Exec(r.Context(), `
				insert into promocode_targets (promocode_id, target_id, created_at)
				select $1, unnest($2::bigint[]), now()
				on conflict do nothing`, id, targetIDs)
		}
		if err != nil {
			_ = tx.Rollback(r.Context())
			return nil, err
		}
		if err := tx.Commit(r.Context()); err != nil {
			return nil, err
		}
		h.writeAdminAudit(r.Context(), r, actor.ID, "admin.promocode.create", "promocode", &id, map[string]any{"code": code, "targetIds": targetIDs})
		created = append(created, map[string]any{"id": id, "code": code, "targetId": legacyTargetID, "targetIds": targetIDs, "applyUrl": promocodeApplyURL(r, code), "temporaryAdminUrl": h.temporaryAdminURL(r, code)})
	}
	clearAdminExportOptionsCache()
	return created, nil
}

func (h Handler) AdminModeGuardrail(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.requireAdminSection(w, r, "orchestration", r.Method != http.MethodGet)
	if !ok {
		return
	}
	if r.Method == http.MethodGet {
		text, err := h.defaultModeGuardrail(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "text": text})
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
		return
	}
	var req adminGuardrailRequest
	if err := decodeJSONStrict(w, r, 128<<10, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid payload"})
		return
	}
	text := strings.TrimSpace(req.Text)
	if text == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "guardrail text is required"})
		return
	}
	_, err := h.DB.Exec(r.Context(), `insert into system_settings (key, value, description, created_at, updated_at) values ('mode_guardrail_default', $1, 'Default guardrail appended to live prompts at request time', now(), now()) on conflict (key) do update set value = excluded.value, updated_at = now()`, text)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	updatedPrompts := int64(0)
	if req.ReplaceAll {
		find := strings.TrimSpace(req.Find)
		replacement, modelErr := normalizeAIModelID(req.Replacement)
		if find == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "find text is required"})
			return
		}
		if modelErr != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid replacement model: " + modelErr.Error()})
			return
		}
		cmd, err := h.DB.Exec(r.Context(), `update modes set ai_model = $2, updated_at = now() where ai_model = $1`, find, replacement)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		updatedPrompts = cmd.RowsAffected()
	}
	h.writeAdminAudit(r.Context(), r, actor.ID, "admin.mode.guardrail", "system_setting", nil, map[string]any{"runtimeAppend": true, "updatedPrompts": updatedPrompts})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "updated": updatedPrompts, "runtimeAppend": true})
}

func (h Handler) AdminModeModelStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
		return
	}
	if writeNamedCached(w, "admin_mode_model_stats", &h.c.adminModeModelStats) {
		return
	}
	rows, err := h.DB.Query(r.Context(), `
		select coalesce(nullif(trim(ai_model), ''), '—') as model,
		       count(*) as mode_count,
		       count(*) filter (where hidden_at is null) as visible_count,
		       count(*) filter (where hidden_at is not null) as hidden_count
		from modes
		group by 1
		order by mode_count desc, model asc`)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var model string
		var total, visible, hidden int64
		if err := rows.Scan(&model, &total, &visible, &hidden); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		out = append(out, map[string]any{"model": model, "total": total, "visible": visible, "hidden": hidden})
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "models": out})
}
