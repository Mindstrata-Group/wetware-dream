package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const adminStatsCacheTTL = 60 * time.Second

func (h Handler) AdminStats(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.requireAdminSection(w, r, "stats", false)
	if !ok {
		return
	}
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
		return
	}

	// Cache fast-path
	h.c.adminStats.Lock()
	if h.c.adminStats.payload != nil && time.Now().Before(h.c.adminStats.expiresAt) {
		cached := h.c.adminStats.payload
		h.c.adminStats.Unlock()
		h.writeAdminAudit(r.Context(), r, actor.ID, "admin.stats.read", "system", nil, nil)
		writeJSON(w, http.StatusOK, cached)
		return
	}
	h.c.adminStats.Unlock()

	ctx := r.Context()
	var totalRequests, totalUsers, totalInputTokens, totalOutputTokens, totalTokens int64
	var totalCost, avgTokens, avgModesPerUser float64
	_ = h.DB.QueryRow(ctx, `
		select count(*), count(distinct user_id), coalesce(sum(input_tokens),0), coalesce(sum(output_tokens),0), coalesce(sum(total_tokens),0), coalesce(sum(estimated_cost),0), coalesce(avg(nullif(total_tokens,0)),0)
		from message_usage`).Scan(&totalRequests, &totalUsers, &totalInputTokens, &totalOutputTokens, &totalTokens, &totalCost, &avgTokens)
	_ = h.DB.QueryRow(ctx, `
		select coalesce(avg(mode_count), 0)
		from (
			select count(distinct mode_id)::float8 as mode_count
			from message_usage
			group by user_id
		) s`).Scan(&avgModesPerUser)
	costPerToken := 0.0
	if totalTokens > 0 {
		costPerToken = totalCost / float64(totalTokens)
	}
	totals := map[string]any{"requests": totalRequests, "users": totalUsers, "inputTokens": totalInputTokens, "outputTokens": totalOutputTokens, "tokens": totalTokens, "cost": totalCost, "costPerToken": costPerToken, "avgTokens": avgTokens, "avgModesPerUser": avgModesPerUser}

	daily, err := h.queryAdminStatsRows(ctx, `
		select to_char(date_trunc('day', created_at), 'YYYY-MM-DD') as label,
		       count(*) as requests,
		       count(distinct user_id) as users,
		       coalesce(sum(total_tokens),0) as tokens,
		       coalesce(sum(estimated_cost),0) as cost,
		       coalesce(avg(nullif(total_tokens,0)),0) as avg_tokens
		from message_usage
		where created_at >= now() - interval '30 days'
		group by 1
		order by 1 desc`)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	byUser, err := h.queryAdminStatsRows(ctx, `
		select coalesce(nullif(u.email,''), nullif(u.telegram_username,''), 'user#' || mu.user_id::text) as label,
		       count(*) as requests,
		       1 as users,
		       coalesce(sum(mu.total_tokens),0) as tokens,
		       coalesce(sum(mu.estimated_cost),0) as cost,
		       coalesce(avg(nullif(mu.total_tokens,0)),0) as avg_tokens
		from message_usage mu
		left join users u on u.id = mu.user_id
		group by mu.user_id, label
		order by tokens desc
		limit 50`)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	byMode, err := h.queryAdminStatsRows(ctx, `
		select coalesce(m.name, 'mode#' || mu.mode_id::text) as label,
		       count(*) as requests,
		       count(distinct mu.user_id) as users,
		       coalesce(sum(mu.total_tokens),0) as tokens,
		       coalesce(sum(mu.estimated_cost),0) as cost,
		       coalesce(avg(nullif(mu.total_tokens,0)),0) as avg_tokens
		from message_usage mu
		left join modes m on m.id = mu.mode_id
		group by mu.mode_id, label
		order by users desc, tokens desc
		limit 50`)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	byTariff, err := h.queryAdminStatsRows(ctx, `
		select coalesce(t.name, case when access.access_type = 'promocode' then 'Промокод' when access.access_type = 'manual' then 'Ручной доступ' else 'Без тарифа' end) as label,
		       count(*) as requests,
		       count(distinct mu.user_id) as users,
		       coalesce(sum(mu.total_tokens),0) as tokens,
		       coalesce(sum(mu.estimated_cost),0) as cost,
		       coalesce(avg(nullif(mu.total_tokens,0)),0) as avg_tokens
		from message_usage mu
		left join user_mode_access access on access.id = mu.access_id
		left join subscriptions s on access.access_type = 'subscription' and s.id = access.source_id
		left join tariffs t on t.id = s.tariff_id
		group by label
		order by tokens desc
		limit 50`)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	payload := map[string]any{"ok": true, "totals": totals, "daily": daily, "byUser": byUser, "byMode": byMode, "byTariff": byTariff}

	// Store in the cache
	h.c.adminStats.Lock()
	h.c.adminStats.payload = payload
	h.c.adminStats.expiresAt = time.Now().Add(adminStatsCacheTTL)
	h.c.adminStats.Unlock()

	h.writeAdminAudit(ctx, r, actor.ID, "admin.stats.read", "system", nil, nil)
	writeJSON(w, http.StatusOK, payload)
}

func (h Handler) queryAdminStatsRows(ctx context.Context, sql string, args ...any) ([]map[string]any, error) {
	rows, err := h.DB.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var label string
		var requests, users, tokens int64
		var cost, avgTokens float64
		if err := rows.Scan(&label, &requests, &users, &tokens, &cost, &avgTokens); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{"label": label, "requests": requests, "users": users, "tokens": tokens, "cost": cost, "avgTokens": avgTokens})
	}
	return out, rows.Err()
}

func (h Handler) AdminPromocodesBulkDeactivate(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.requireOwnerAdmin(w, r)
	if !ok {
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
		return
	}
	var req struct {
		DateFrom string `json:"dateFrom"`
		DateTo   string `json:"dateTo"`
	}
	if err := decodeJSONStrict(w, r, 16<<10, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid payload"})
		return
	}
	where := []string{"(active_to is null or active_to > now())"}
	args := []any{}
	if from, ok := parseAdminExportTime(req.DateFrom, false); ok {
		args = append(args, from)
		where = append(where, fmt.Sprintf("created_at >= $%d", len(args)))
	}
	if to, ok := parseAdminExportTime(req.DateTo, true); ok {
		args = append(args, to)
		where = append(where, fmt.Sprintf("created_at <= $%d", len(args)))
	}
	whereSQL := strings.Join(where, " and ")
	var deactivated int64
	if err := h.DB.QueryRow(r.Context(), fmt.Sprintf(`select count(*) from promocodes where %s`, whereSQL), args...).Scan(&deactivated); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if deactivated > 0 {
		if _, err := h.DB.Exec(r.Context(), fmt.Sprintf(`update promocodes set active_to = now(), updated_at = now() where %s`, whereSQL), args...); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
	}
	clearAdminExportOptionsCache()
	h.c.adminPromoList.clear()
	h.writeAdminAudit(r.Context(), r, actor.ID, "admin.promocode.bulk_deactivate", "promocode", nil, map[string]any{"dateFrom": req.DateFrom, "dateTo": req.DateTo, "updated": deactivated})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "updated": deactivated})
}

func (h Handler) AdminOrchestrationPrompt(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.requireAdminSection(w, r, "orchestration", r.Method != http.MethodGet)
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		if writeNamedCached(w, "admin_orchestration", &h.c.adminOrchestration) {
			return
		}
		prompt, err := h.systemSetting(r.Context(), "orchestration_prompt_default")
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		body, _ := json.Marshal(map[string]any{"ok": true, "prompt": prompt})
		h.c.adminOrchestration.set(body, adminConfigCacheTTL)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	case http.MethodPost:
		var req struct {
			Prompt string `json:"prompt"`
		}
		if err := decodeJSONStrict(w, r, 128<<10, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid payload"})
			return
		}
		prompt := strings.TrimSpace(req.Prompt)
		if prompt == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "prompt is required"})
			return
		}
		_, err := h.DB.Exec(r.Context(), `insert into system_settings (key, value, description, created_at, updated_at) values ('orchestration_prompt_default', $1, 'Default prompt for mode orchestration', now(), now()) on conflict (key) do update set value=excluded.value, updated_at=now()`, prompt)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		h.c.adminOrchestration.clear()
		h.writeAdminAudit(r.Context(), r, actor.ID, "admin.orchestration_prompt", "system_setting", nil, nil)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
	}
}

func (h Handler) AdminDialogSummaryPrompt(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.requireAdminSection(w, r, "orchestration", r.Method != http.MethodGet)
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		if writeNamedCached(w, "admin_dialog_summary", &h.c.adminDialogSummary) {
			return
		}
		prompt, err := h.dialogSummaryPrompt(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		body, _ := json.Marshal(map[string]any{"ok": true, "prompt": prompt})
		h.c.adminDialogSummary.set(body, adminConfigCacheTTL)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	case http.MethodPost:
		var req struct {
			Prompt string `json:"prompt"`
		}
		if err := decodeJSONStrict(w, r, 128<<10, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid payload"})
			return
		}
		prompt := strings.TrimSpace(req.Prompt)
		if prompt == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "prompt is required"})
			return
		}
		_, err := h.DB.Exec(r.Context(), `insert into system_settings (key, value, description, created_at, updated_at) values ('dialog_summary_prompt_default', $1, 'Default prompt for user-facing dialog completion summaries', now(), now()) on conflict (key) do update set value=excluded.value, updated_at=now()`, prompt)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		h.c.adminDialogSummary.clear()
		h.writeAdminAudit(r.Context(), r, actor.ID, "admin.dialog_summary_prompt", "system_setting", nil, nil)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
	}
}

// AdminLeadSummaryPrompt handles GET/POST of the AI summary prompt in lead notifications
// (see lead_notifications.go), configurable the same way as the orchestration
// and dialog summary prompts instead of being hardcoded.
func (h Handler) AdminLeadSummaryPrompt(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.requireAdminSection(w, r, "orchestration", r.Method != http.MethodGet)
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		if writeNamedCached(w, "admin_lead_summary", &h.c.adminLeadSummaryPrompt) {
			return
		}
		prompt := h.leadSummaryPrompt(r.Context())
		body, _ := json.Marshal(map[string]any{"ok": true, "prompt": prompt})
		h.c.adminLeadSummaryPrompt.set(body, adminConfigCacheTTL)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	case http.MethodPost:
		var req struct {
			Prompt string `json:"prompt"`
		}
		if err := decodeJSONStrict(w, r, 128<<10, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid payload"})
			return
		}
		prompt := strings.TrimSpace(req.Prompt)
		if prompt == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "prompt is required"})
			return
		}
		_, err := h.DB.Exec(r.Context(), `insert into system_settings (key, value, description, created_at, updated_at) values ('lead_summary_prompt_default', $1, 'Prompt for AI summary in lead notifications', now(), now()) on conflict (key) do update set value=excluded.value, updated_at=now()`, prompt)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		h.c.adminLeadSummaryPrompt.clear()
		h.writeAdminAudit(r.Context(), r, actor.ID, "admin.lead_summary_prompt", "system_setting", nil, nil)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
	}
}
