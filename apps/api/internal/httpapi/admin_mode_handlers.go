package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const adminModesCacheTTL = 60 * time.Second

func (h Handler) AdminModes(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.requireAdminSection(w, r, "modes", r.Method != http.MethodGet)
	if !ok {
		return
	}
	if r.Method == http.MethodPost {
		var req adminPatchModeRequest
		if err := decodeJSONStrict(w, r, 256<<10, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid payload"})
			return
		}
		name := stringValue(req.Name)
		prompt := stringValue(req.Prompt)
		aiModel := strings.TrimSpace(stringValue(req.AIModel))
		if name == "" || prompt == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "name and prompt are required"})
			return
		}
		aiProvider := normalizeAIProvider(stringValue(req.AIProvider))
		thinkingMode := normalizeThinkingMode(stringValue(req.ThinkingMode))
		if aiModel == "" {
			aiModel = "gpt-4o-mini"
		} else {
			normalizedModel, err := normalizeAIModelID(aiModel)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid aiModel: " + err.Error()})
				return
			}
			aiModel = normalizedModel
		}
		temperature := 0.7
		if req.ModelTemperature != nil {
			temperature = *req.ModelTemperature
		}
		demoChat := "[]"
		if adminMutationAllowed(actor.Role) {
			demoChat = stringValue(req.DemoChat)
			if strings.TrimSpace(demoChat) == "" {
				demoChat = "[]"
			}
		}
		welcomeMessage := stringValue(req.WelcomeMessage)
		criteria := stringValue(req.Criteria)
		var id int64
		err := h.DB.QueryRow(r.Context(), `
			insert into modes (name, prompt, welcome_message, ai_model, ai_provider, thinking_mode, model_temperature, demo_chat, hidden_at, audio_enabled, criteria, orchestrator_check_interval, reminder_count, created_at, updated_at)
			values ($1, $2, $3, $4, $5, $6, $7, $8::jsonb, case when $9 then now() else null end, $10, $11, $12, $13, now(), now())
			returning id`, name, prompt, welcomeMessage, aiModel, aiProvider, thinkingMode, temperature, demoChat, boolValue(req.Hidden), boolValue(req.AudioEnabled), criteria, intValue(req.OrchestratorCheckInterval, 5), req.ReminderCount).Scan(&id)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		_, _ = h.DB.Exec(r.Context(), `delete from public.public_demo_modes_cache where cache_key = 'home_demo_modes'`)
		h.clearPublicDemoModesCache()
		h.clearModesCache()
		clearAdminExportOptionsCache()
		h.c.adminModes.clear()
		h.c.adminModeModelStats.clear()
		// Git snapshot of the prompt history happens after a successful DB write; its
		// error must not break a mode creation that already succeeded (see the pattern in notifications.go).
		snap := modeSnapshot{Name: name, Prompt: prompt, WelcomeMessage: welcomeMessage, Criteria: criteria}
		if _, commitErr := h.modeHistoryOrNoop().commitSnapshot(r.Context(), id, snap, actor.Email, actor.Email, fmt.Sprintf("режим %d: создание — %s", id, actor.Email)); commitErr != nil {
			h.writeAdminAudit(r.Context(), r, actor.ID, "admin.mode.history_write_failed", "mode", &id, map[string]any{"error": commitErr.Error()})
		}
		h.writeAdminAudit(r.Context(), r, actor.ID, "admin.mode.create", "mode", &id, map[string]any{"name": name})
		writeJSON(w, http.StatusCreated, map[string]any{"ok": true, "modeId": id})
		return
	}
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
		return
	}
	limit := int64(50)
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if parsed, err := strconv.ParseInt(raw, 10, 64); err == nil && (parsed == 10 || parsed == 50 || parsed == 100) {
			limit = parsed
		}
	}
	offset := int64(0)
	if raw := r.URL.Query().Get("offset"); raw != "" {
		if parsed, err := strconv.ParseInt(raw, 10, 64); err == nil && parsed >= 0 {
			offset = parsed
		}
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	cacheKey := fmt.Sprintf("%s|%d|%d", q, limit, offset)
	if body, ok := h.c.adminModes.get(cacheKey); ok {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
		return
	}
	where := ""
	filterArgs := []any{}
	if q != "" {
		where = "where lower(name) like lower($1) or id::text = $2"
		filterArgs = append(filterArgs, "%"+q+"%", q)
	}
	var total int64
	if err := h.DB.QueryRow(r.Context(), fmt.Sprintf(`select count(*) from modes %s`, where), filterArgs...).Scan(&total); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	args := append([]any{}, filterArgs...)
	limitPlaceholder := len(args) + 1
	offsetPlaceholder := len(args) + 2
	args = append(args, limit, offset)
	rows, err := h.DB.Query(r.Context(), fmt.Sprintf(`
		select id, name, coalesce(welcome_message, ''), ai_model, model_temperature, hidden_at is not null, exists(select 1 from tariff_mode tm join tariffs t on t.id = tm.tariff_id where tm.mode_id = modes.id and coalesce(t.monthly_price, 0) > 0 and t.available_for_subscription = true and t.archived_at is null), coalesce((select string_agg(modes.name || ' → ' || t.name || coalesce(' → ' || tg.name, ''), '; ' order by t.name) from tariff_mode tm join tariffs t on t.id = tm.tariff_id left join tariff_groups tg on tg.id = t.group_id where tm.mode_id = modes.id and coalesce(t.monthly_price,0) > 0 and t.available_for_subscription = true and t.archived_at is null), '')
		from modes
		%s
		order by id asc
		limit $%d offset $%d`, where, limitPlaceholder, offsetPlaceholder), args...)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id int64
		var name, welcome, aiModel string
		var temperature float64
		var hidden bool
		var paid bool
		var paidChains string
		if err := rows.Scan(&id, &name, &welcome, &aiModel, &temperature, &hidden, &paid, &paidChains); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		out = append(out, map[string]any{"id": id, "name": name, "welcomeMessage": welcome, "aiModel": aiModel, "modelTemperature": temperature, "hidden": hidden, "paid": paid, "paidChains": paidChains})
	}
	body, _ := json.Marshal(map[string]any{"ok": true, "modes": out, "total": total, "limit": limit, "offset": offset})
	h.c.adminModes.set(cacheKey, body, adminModesCacheTTL)
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body)
}
