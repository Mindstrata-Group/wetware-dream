package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

func (h Handler) AdminModeDetail(w http.ResponseWriter, r *http.Request) {
	mutation := r.Method != http.MethodGet
	if r.Method == http.MethodDelete {
		mutation = true
	}
	actor, ok := h.requireAdminSection(w, r, "modes", mutation)
	if !ok {
		return
	}
	modeID, suffix, err := parseIDFromPath(r.URL.Path, "/api/admin/modes/")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid mode id"})
		return
	}
	if suffix == "test-model" {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
			return
		}
		var req struct {
			Model        string   `json:"model"`
			Provider     string   `json:"provider"`
			ThinkingMode string   `json:"thinkingMode"`
			Text         string   `json:"text"`
			Temperature  *float64 `json:"temperature"`
		}
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&req)
		mode, err := getModeByID(r.Context(), h.DB, modeID)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]any{"ok": false, "error": "mode not found"})
			return
		}
		provider := strings.TrimSpace(req.Provider)
		if provider == "" {
			provider = mode.AIProvider
		}
		provider = normalizeAIProvider(provider)
		model := strings.TrimSpace(req.Model)
		if model == "" {
			model = strings.TrimSpace(mode.AIModel)
		}
		// normalizeAIModelID handles the legacy vsegpt format (provider/model); for direct
		// providers the model name is passed as is, without this normalisation.
		if provider == aiProviderVsegpt {
			normalizedModel, err := normalizeAIModelID(model)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid model: " + err.Error(), "model": model})
				return
			}
			model = normalizedModel
		}
		thinkingMode := strings.TrimSpace(req.ThinkingMode)
		if thinkingMode == "" {
			thinkingMode = mode.ThinkingMode
		}
		thinkingMode = normalizeThinkingMode(thinkingMode)
		temperature := mode.Temperature
		if req.Temperature != nil {
			temperature = *req.Temperature
		}
		text := strings.TrimSpace(req.Text)
		if text == "" {
			text = "Привет"
		}
		guardrail, err := h.defaultModeGuardrail(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		messages := []map[string]string{
			{"role": "system", "content": buildRuntimeModePrompt(mode.Prompt, guardrail)},
			{"role": "user", "content": text},
		}
		spec := aiCallSpec{Provider: provider, Model: model, Temperature: temperature, ThinkingMode: thinkingMode}
		answer, inTok, outTok, cost, err := h.doAIChatResilientSpec(r.Context(), spec, messages, buildXTitle(mode.ID, actor.ID, "ADMIN_MODEL_TEST"), openAIChatOptions{})
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error(), "model": model, "provider": provider})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "ok", "model": model, "provider": provider, "requestedModel": model, "response": answer, "inputTokens": inTok, "outputTokens": outTok, "cost": cost})
		return
	}
	if suffix == "detach-paid-tariffs" {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
			return
		}
		cmd, err := h.DB.Exec(r.Context(), `
			delete from tariff_mode tm
			using tariffs t
			where t.id = tm.tariff_id
			  and tm.mode_id = $1
			  and coalesce(t.monthly_price, 0) > 0
			  and t.available_for_subscription = true
			  and t.archived_at is null`, modeID)
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
		h.writeAdminAudit(r.Context(), r, actor.ID, "admin.mode.detach_paid_tariffs", "mode", &modeID, map[string]any{"removed": cmd.RowsAffected()})
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "removed": cmd.RowsAffected(), "cacheCleared": true})
		return
	}
	if suffix == "copy" {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
			return
		}
		var newID int64
		err := h.DB.QueryRow(r.Context(), `
			INSERT INTO modes (name, prompt, welcome_message, ai_model, model_temperature, demo_chat, hidden_at, audio_enabled, criteria, orchestrator_check_interval, reminder_count, created_at, updated_at)
			SELECT 'Копия ' || name, prompt, welcome_message, ai_model, model_temperature, demo_chat, now(), audio_enabled, criteria, orchestrator_check_interval, reminder_count, now(), now()
			FROM modes WHERE id = $1
			RETURNING id`, modeID).Scan(&newID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				writeJSON(w, http.StatusNotFound, map[string]any{"ok": false, "error": "mode not found"})
			} else {
				writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			}
			return
		}
		_, _ = h.DB.Exec(r.Context(), `delete from public.public_demo_modes_cache where cache_key = 'home_demo_modes'`)
		h.clearPublicDemoModesCache()
		h.clearModesCache()
		clearAdminExportOptionsCache()
		h.c.adminModes.clear()
		h.c.adminModeModelStats.clear()
		h.writeAdminAudit(r.Context(), r, actor.ID, "admin.mode.copy", "mode", &newID, map[string]any{"sourceId": modeID})
		writeJSON(w, http.StatusCreated, map[string]any{"ok": true, "modeId": newID})
		return
	}
	// Prompt version history (git mirror): GET .../history, GET .../history/{sha}.
	// The UI does not use the words git/commit/SHA, only "versions" (see frontend EditPanel).
	if suffix == "history" || strings.HasPrefix(suffix, "history/") {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
			return
		}
		if suffix == "history" {
			entries, err := h.modeHistoryOrNoop().history(r.Context(), modeID, 50)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
				return
			}
			if entries == nil {
				entries = []modeHistoryEntry{}
			}
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "versions": entries})
			return
		}
		sha := strings.TrimPrefix(suffix, "history/")
		snap, err := h.modeHistoryOrNoop().snapshotAt(r.Context(), modeID, sha)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]any{"ok": false, "error": "версия не найдена"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "version": snap})
		return
	}
	// Restoring a version works like git revert, not git reset: the snapshot
	// fields are applied through the regular UPDATE path, creating a NEW version on
	// top instead of forcibly rewriting history.
	if strings.HasPrefix(suffix, "restore/") {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
			return
		}
		sha := strings.TrimPrefix(suffix, "restore/")
		snap, err := h.modeHistoryOrNoop().snapshotAt(r.Context(), modeID, sha)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]any{"ok": false, "error": "версия не найдена"})
			return
		}
		var name, prompt, welcome, criteria string
		err = h.DB.QueryRow(r.Context(), `
			update modes
			set prompt = $2, welcome_message = $3, criteria = $4, updated_at = now()
			where id = $1
			returning name, prompt, coalesce(welcome_message, ''), coalesce(criteria, '')`,
			modeID, snap.Prompt, snap.WelcomeMessage, snap.Criteria).Scan(&name, &prompt, &welcome, &criteria)
		if err != nil {
			status := http.StatusBadRequest
			if errors.Is(err, pgx.ErrNoRows) {
				status = http.StatusNotFound
			}
			writeJSON(w, status, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		_, _ = h.DB.Exec(r.Context(), `delete from public.public_demo_modes_cache where cache_key = 'home_demo_modes'`)
		h.clearPublicDemoModesCache()
		h.clearModesCache()
		clearAdminExportOptionsCache()
		h.c.adminModes.clear()
		h.c.adminModeModelStats.clear()
		restoredSnap := modeSnapshot{Name: name, Prompt: prompt, WelcomeMessage: welcome, Criteria: criteria}
		shortSHA := sha
		if len(shortSHA) > 8 {
			shortSHA = shortSHA[:8]
		}
		if _, commitErr := h.modeHistoryOrNoop().commitSnapshot(r.Context(), modeID, restoredSnap, actor.Email, actor.Email, fmt.Sprintf("режим %d: восстановление версии %s — %s", modeID, shortSHA, actor.Email)); commitErr != nil {
			h.writeAdminAudit(r.Context(), r, actor.ID, "admin.mode.history_write_failed", "mode", &modeID, map[string]any{"error": commitErr.Error()})
		}
		h.writeAdminAudit(r.Context(), r, actor.ID, "admin.mode.restore", "mode", &modeID, map[string]any{"sha": sha})
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}
	if suffix == "" && r.Method == http.MethodDelete {
		tx, err := beginTxTimeout(r.Context(), h.DB, dbAcquireTimeout)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		defer tx.Rollback(r.Context())

		_, _ = tx.Exec(r.Context(), `delete from tariff_mode where mode_id = $1`, modeID)
		var name string
		if err := tx.QueryRow(r.Context(), `delete from modes where id = $1 returning name`, modeID).Scan(&name); err != nil {
			status := http.StatusBadRequest
			if errors.Is(err, pgx.ErrNoRows) {
				status = http.StatusNotFound
			}
			writeJSON(w, status, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		if err := tx.Commit(r.Context()); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		_, _ = h.DB.Exec(r.Context(), `delete from public.public_demo_modes_cache where cache_key = 'home_demo_modes'`)
		h.clearPublicDemoModesCache()
		h.clearModesCache()
		clearAdminExportOptionsCache()
		h.c.adminModes.clear()
		h.c.adminModeModelStats.clear()
		h.writeAdminAudit(r.Context(), r, actor.ID, "admin.mode.delete", "mode", &modeID, map[string]any{"name": name})
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "deleted": true, "name": name})
		return
	}
	switch r.Method {
	case http.MethodGet:
		var id int64
		var name, prompt, welcome, aiModel, aiProvider, thinkingMode, demoChat, criteria string
		var temperature float64
		var hidden, audioEnabled bool
		var orchestratorInterval int
		var reminderCount *int
		var aiMaxTokens *int64
		var leadNotifyEnabled bool
		var leadNotifyChatIDs, leadNotifyTelegramIDs string
		var leadNotifyThreshold int
		err := h.DB.QueryRow(r.Context(), `
			select id, name, prompt, coalesce(welcome_message, ''), ai_model, coalesce(ai_provider, 'vsegpt'), coalesce(thinking_mode, 'default'), model_temperature, coalesce(demo_chat::text, ''), hidden_at is not null, audio_enabled, coalesce(criteria, ''), orchestrator_check_interval, reminder_count, ai_max_tokens, coalesce(lead_notify_enabled, false), coalesce(lead_notify_chat_ids, ''), coalesce(lead_notify_telegram_ids, ''), coalesce(lead_notify_threshold, 3)
			from modes
			where id = $1`, modeID).Scan(&id, &name, &prompt, &welcome, &aiModel, &aiProvider, &thinkingMode, &temperature, &demoChat, &hidden, &audioEnabled, &criteria, &orchestratorInterval, &reminderCount, &aiMaxTokens, &leadNotifyEnabled, &leadNotifyChatIDs, &leadNotifyTelegramIDs, &leadNotifyThreshold)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]any{"ok": false, "error": "mode not found"})
			return
		}
		if !adminMutationAllowed(actor.Role) {
			demoChat = "[]"
		}
		// aiMaxTokens is returned as a number (0 = "global default") so the UI shows
		// 0/empty on the slider and does not confuse the admin with NULL.
		aiMaxTokensOut := int64(0)
		if aiMaxTokens != nil {
			aiMaxTokensOut = *aiMaxTokens
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "mode": map[string]any{"id": id, "name": name, "prompt": prompt, "welcomeMessage": welcome, "aiModel": aiModel, "aiProvider": aiProvider, "thinkingMode": thinkingMode, "modelTemperature": temperature, "demoChat": demoChat, "hidden": hidden, "audioEnabled": audioEnabled, "criteria": criteria, "orchestratorCheckInterval": orchestratorInterval, "reminderCount": reminderCount, "aiMaxTokens": aiMaxTokensOut, "leadNotifyEnabled": leadNotifyEnabled, "leadNotifyChatIds": leadNotifyChatIDs, "leadNotifyTelegramIds": leadNotifyTelegramIDs, "leadNotifyThreshold": leadNotifyThreshold}})
	case http.MethodPatch:
		var req adminPatchModeRequest
		if err := decodeJSONStrict(w, r, 256<<10, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid payload"})
			return
		}
		if !adminMutationAllowed(actor.Role) {
			req.DemoChat = nil
		}
		if req.AIModel != nil {
			normalizedModel, err := normalizeAIModelID(*req.AIModel)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid aiModel: " + err.Error()})
				return
			}
			req.AIModel = &normalizedModel
		}
		if req.AIProvider != nil {
			normalized := normalizeAIProvider(*req.AIProvider)
			req.AIProvider = &normalized
		}
		if req.ThinkingMode != nil {
			normalized := normalizeThinkingMode(*req.ThinkingMode)
			req.ThinkingMode = &normalized
		}
		// AIMaxTokens: nil = do not change; 0 (or <0) = reset to the global
		// default (NULL in the DB); >0 = clamp to the allowed range [256,32768]
		// (matches the migration's CHECK constraint, otherwise the UPDATE fails).
		if req.AIMaxTokens != nil {
			v := *req.AIMaxTokens
			if v <= 0 {
				v = 0
			} else if v < minAIMaxTokens {
				v = minAIMaxTokens
			} else if v > maxAIMaxTokens {
				v = maxAIMaxTokens
			}
			req.AIMaxTokens = &v
		}
		// The lead notification recipient list is stored in canonical form
		// "id1,id2": junk and non-numeric items are silently dropped.
		if req.LeadNotifyChatIDs != nil {
			ids := parseLeadChatIDs(*req.LeadNotifyChatIDs)
			parts := make([]string, 0, len(ids))
			for _, id := range ids {
				parts = append(parts, strconv.FormatInt(id, 10))
			}
			normalized := strings.Join(parts, ",")
			req.LeadNotifyChatIDs = &normalized
		}
		if req.LeadNotifyTelegramIDs != nil {
			ids := parseLeadTelegramIDs(*req.LeadNotifyTelegramIDs)
			normalized := strings.Join(ids, ",")
			req.LeadNotifyTelegramIDs = &normalized
		}
		if req.LeadNotifyThreshold != nil {
			v := *req.LeadNotifyThreshold
			if v < 1 {
				v = 1
			} else if v > 100 {
				v = 100
			}
			req.LeadNotifyThreshold = &v
		}
		var name, prompt, welcome, criteria string
		err := h.DB.QueryRow(r.Context(), `
			update modes
			set name = coalesce($2, name),
			    prompt = coalesce($3, prompt),
			    welcome_message = coalesce($4, welcome_message),
			    ai_model = coalesce($5, ai_model),
			    ai_provider = coalesce($6, ai_provider),
			    thinking_mode = coalesce($7, thinking_mode),
			    model_temperature = coalesce($8, model_temperature),
			    demo_chat = case when $9::text is null then demo_chat else $9::jsonb end,
			    hidden_at = case when $10::bool is null then hidden_at when $10::bool then coalesce(hidden_at, now()) else null end,
			    audio_enabled = coalesce($11, audio_enabled),
			    criteria = coalesce($12, criteria),
			    orchestrator_check_interval = coalesce($13, orchestrator_check_interval),
			    reminder_count = coalesce($14, reminder_count),
			    ai_max_tokens = case when $15::int is null then ai_max_tokens when $15::int <= 0 then null else $15::int end,
			    lead_notify_enabled = coalesce($16, lead_notify_enabled),
			    lead_notify_chat_ids = coalesce($17, lead_notify_chat_ids),
			    lead_notify_threshold = coalesce($18, lead_notify_threshold),
			    lead_notify_telegram_ids = coalesce($19, lead_notify_telegram_ids),
			    updated_at = now()
			where id = $1
			returning name, prompt, coalesce(welcome_message, ''), coalesce(criteria, '')`,
			modeID, req.Name, req.Prompt, req.WelcomeMessage, req.AIModel, req.AIProvider, req.ThinkingMode, req.ModelTemperature, req.DemoChat, req.Hidden, req.AudioEnabled, req.Criteria, req.OrchestratorCheckInterval, req.ReminderCount, req.AIMaxTokens, req.LeadNotifyEnabled, req.LeadNotifyChatIDs, req.LeadNotifyThreshold, req.LeadNotifyTelegramIDs).
			Scan(&name, &prompt, &welcome, &criteria)
		if err != nil {
			status := http.StatusBadRequest
			if errors.Is(err, pgx.ErrNoRows) {
				status = http.StatusNotFound
			}
			writeJSON(w, status, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		_, _ = h.DB.Exec(r.Context(), `delete from public.public_demo_modes_cache where cache_key = 'home_demo_modes'`)
		h.clearPublicDemoModesCache()
		h.clearModesCache()
		clearAdminExportOptionsCache()
		h.c.adminModes.clear()
		h.c.adminModeModelStats.clear()
		// Git snapshot of the prompt history happens after a successful DB write; its
		// error must not break a save that already succeeded (see the pattern in notifications.go).
		snap := modeSnapshot{Name: name, Prompt: prompt, WelcomeMessage: welcome, Criteria: criteria}
		if _, commitErr := h.modeHistoryOrNoop().commitSnapshot(r.Context(), modeID, snap, actor.Email, actor.Email, fmt.Sprintf("режим %d: правка — %s", modeID, actor.Email)); commitErr != nil {
			h.writeAdminAudit(r.Context(), r, actor.ID, "admin.mode.history_write_failed", "mode", &modeID, map[string]any{"error": commitErr.Error()})
		}
		h.writeAdminAudit(r.Context(), r, actor.ID, "admin.mode.patch", "mode", &modeID, nil)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
	}
}
