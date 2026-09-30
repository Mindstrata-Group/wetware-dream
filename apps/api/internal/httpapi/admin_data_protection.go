package httpapi

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Admin side of the data-protection policy. Spec: docs/specs/data-protection.md.
//
// Section "privacy" is not in the read-only role table on purpose, so only
// owner/admin get in: the access log itself reveals whose data was looked at.

// AdminDataProtection — GET /api/admin/data-protection: the installation
// profile and where each gateway processes data.
func (h Handler) AdminDataProtection(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireAdminSection(w, r, "privacy", false); !ok {
		return
	}
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
		return
	}
	countries := h.gatewayCountries(r.Context())
	gateways := []map[string]any{}
	for _, g := range h.activeGateways(r.Context()) {
		gateways = append(gateways, map[string]any{
			"id":                g.ID,
			"title":             g.Title,
			"enabled":           g.Enabled,
			"processingCountry": countries[g.ID],
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":             true,
		"profile":        h.dataProtectionProfile(),
		"storageCountry": strings.ToUpper(strings.TrimSpace(h.DataStorageCountry)),
		"warnings":       dataProtectionWarnings(h.DataProtectionProfile, h.DataStorageCountry),
		"gateways":       gateways,
	})
}

// AdminDataProtectionGatewayCountry — POST {gatewayId, country}.
func (h Handler) AdminDataProtectionGatewayCountry(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.requireAdminSection(w, r, "privacy", true)
	if !ok {
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
		return
	}
	var req struct {
		GatewayID string `json:"gatewayId"`
		Country   string `json:"country"`
	}
	if err := decodeJSONStrict(w, r, 1<<10, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid payload"})
		return
	}
	id := strings.ToLower(strings.TrimSpace(req.GatewayID))
	if !gatewayIDPattern.MatchString(id) {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "некорректный id шлюза"})
		return
	}
	country, err := normalizeProcessingCountry(req.Country)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "страна — двухбуквенный код, например RU или DE"})
		return
	}
	tag, err := h.DB.Exec(r.Context(), `
		update ai_gateways set processing_country = $2, updated_at = now() where id = $1`, id, country)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "нужна миграция 20260930_120000_data_protection.sql"})
		return
	}
	if tag.RowsAffected() == 0 {
		writeJSON(w, http.StatusNotFound, map[string]any{"ok": false, "error": "шлюз не найден"})
		return
	}
	h.c.aiGateways.clear()
	h.writeAdminAudit(r.Context(), r, actor.ID, "admin.data_protection.gateway_country", "ai_gateway", nil, map[string]any{
		"id":      id,
		"country": country,
	})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "gatewayId": id, "country": country})
}

// AdminDataProtectionAccessLog — GET: who opened personal data and when.
// Optional filters: subjectUserId, actorUserId, limit (1..500, default 100).
func (h Handler) AdminDataProtectionAccessLog(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireAdminSection(w, r, "privacy", false); !ok {
		return
	}
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
		return
	}
	q := r.URL.Query()
	var subject, actorFilter any
	if v, err := strconv.ParseInt(strings.TrimSpace(q.Get("subjectUserId")), 10, 64); err == nil && v > 0 {
		subject = strconv.FormatInt(v, 10)
	}
	if v, err := strconv.ParseInt(strings.TrimSpace(q.Get("actorUserId")), 10, 64); err == nil && v > 0 {
		actorFilter = v
	}
	limit := 100
	if v, err := strconv.Atoi(strings.TrimSpace(q.Get("limit"))); err == nil && v > 0 && v <= 500 {
		limit = v
	}
	rows, err := h.DB.Query(r.Context(), `
		select id, actor_user_id, action, target_type, target_id,
		       coalesce(meta->>'subjectUserId', case when target_type = 'user' then target_id::text end),
		       created_at
		from admin_audit_log
		where action in ('admin.dialog.read', 'admin.user.read', 'tester.user.read', 'tester.user_chat_preview',
		                 'admin.export.messages.read', 'admin.export.messages.summary')
		  and ($1::text is null or coalesce(meta->>'subjectUserId', case when target_type = 'user' then target_id::text end) = $1::text)
		  and ($2::bigint is null or actor_user_id = $2::bigint)
		order by created_at desc, id desc
		limit $3`, subject, actorFilter, limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer rows.Close()
	entries := []map[string]any{}
	for rows.Next() {
		var (
			id         int64
			actorID    *int64
			action     string
			targetType string
			targetID   *int64
			subjectID  *string
			createdAt  time.Time
		)
		if err := rows.Scan(&id, &actorID, &action, &targetType, &targetID, &subjectID, &createdAt); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		entries = append(entries, map[string]any{
			"id":            id,
			"actorUserId":   actorID,
			"action":        action,
			"targetType":    targetType,
			"targetId":      targetID,
			"subjectUserId": subjectID,
			"createdAt":     createdAt,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "entries": entries})
}
