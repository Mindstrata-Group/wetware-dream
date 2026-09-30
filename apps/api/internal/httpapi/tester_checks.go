package httpapi

import (
	"context"
	"net/http"
	"strings"
	"time"
)

func (h Handler) runTesterCheck(r *http.Request, check string) map[string]any {
	started := time.Now()
	result := map[string]any{"check": check, "ok": true}
	switch check {
	case "db":
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		if err := h.DB.Ping(ctx); err != nil {
			result["ok"] = false
			result["error"] = err.Error()
		}
	case "public_demo_modes":
		var count int64
		if err := h.DB.QueryRow(r.Context(), `select count(*) from public.get_public_demo_modes_cached()`).Scan(&count); err != nil {
			result["ok"] = false
			result["error"] = err.Error()
		} else {
			result["count"] = count
		}
	case "modes":
		modes, err := h.listModes(r.Context())
		if err != nil {
			result["ok"] = false
			result["error"] = err.Error()
		} else {
			result["count"] = len(modes)
		}
	case "users":
		var count int64
		if err := h.DB.QueryRow(r.Context(), `select count(*) from users where deleted_at is null`).Scan(&count); err != nil {
			result["ok"] = false
			result["error"] = err.Error()
		} else {
			result["count"] = count
		}
	case "tariffs":
		var count int64
		if err := h.DB.QueryRow(r.Context(), `select count(*) from tariffs`).Scan(&count); err != nil {
			result["ok"] = false
			result["error"] = err.Error()
		} else {
			result["count"] = count
		}
	case "promocodes":
		var count int64
		if err := h.DB.QueryRow(r.Context(), `select count(*) from promocodes`).Scan(&count); err != nil {
			result["ok"] = false
			result["error"] = err.Error()
		} else {
			result["count"] = count
		}
	case "chat_tables":
		var dialogs, messages int64
		err1 := h.DB.QueryRow(r.Context(), `select count(*) from users_dialogs where deleted_at is null`).Scan(&dialogs)
		err2 := h.DB.QueryRow(r.Context(), `select count(*) from dialogs_messages`).Scan(&messages)
		if err1 != nil {
			result["ok"] = false
			result["error"] = err1.Error()
		} else if err2 != nil {
			result["ok"] = false
			result["error"] = err2.Error()
		} else {
			result["dialogs"] = dialogs
			result["messages"] = messages
		}
	case "auth_core":
		var users, sessions int64
		err1 := h.DB.QueryRow(r.Context(), `select count(*) from users where deleted_at is null`).Scan(&users)
		err2 := h.DB.QueryRow(r.Context(), `select count(*) from auth_sessions where revoked_at is null and expires_at > now()`).Scan(&sessions)
		if err1 != nil {
			result["ok"] = false
			result["error"] = err1.Error()
		} else if err2 != nil {
			result["ok"] = false
			result["error"] = err2.Error()
		} else {
			result["users"] = users
			result["activeSessions"] = sessions
		}
	case "access_quota":
		var activeAccess, usageToday int64
		err1 := h.DB.QueryRow(r.Context(), `select count(*) from user_mode_access where active_from <= now() and (active_to is null or active_to >= now())`).Scan(&activeAccess)
		err2 := h.DB.QueryRow(r.Context(), `select count(*) from message_usage where created_at >= date_trunc('day', now())`).Scan(&usageToday)
		if err1 != nil {
			result["ok"] = false
			result["error"] = err1.Error()
		} else if err2 != nil {
			result["ok"] = false
			result["error"] = err2.Error()
		} else {
			result["activeAccessRows"] = activeAccess
			result["usageRowsToday"] = usageToday
		}
	case "promo_apply_infra":
		var promos, usages int64
		err1 := h.DB.QueryRow(r.Context(), `select count(*) from promocodes`).Scan(&promos)
		err2 := h.DB.QueryRow(r.Context(), `select count(*) from promocode_usages`).Scan(&usages)
		if err1 != nil {
			result["ok"] = false
			result["error"] = err1.Error()
		} else if err2 != nil {
			result["ok"] = false
			result["error"] = err2.Error()
		} else {
			result["promocodes"] = promos
			result["usages"] = usages
		}
	case "exports":
		items, exportText, err := h.collectAdminExport(r.Context(), nil, nil, nil, "all", 1, "", "")
		if err != nil {
			result["ok"] = false
			result["error"] = err.Error()
		} else {
			result["sampleMessages"] = len(items)
			result["sampleBytes"] = len(exportText)
		}
	case "ai_settings":
		model, temperature := h.adminSummaryModel(r.Context())
		result["summaryModel"] = model
		result["summaryTemperature"] = temperature
		result["openAIBaseURLConfigured"] = strings.TrimSpace(h.OpenAIBaseURL) != ""
	case "ollama_config":
		// Ollama removed on 2026-05-28; a stub so old UI checks do not fail.
		result["enabled"] = false
		result["baseURL"] = ""
		result["model"] = ""
		result["removed"] = true
	case "live_config":
		// 2026-05-29: the EnableLiveAI flag was removed. Live AI is available if the key is set.
		result["enabled"] = h.gatewayConfigured(r.Context(), aiProviderVsegpt)
		result["baseURLConfigured"] = strings.TrimSpace(h.OpenAIBaseURL) != ""
		result["apiKeyConfigured"] = h.gatewayConfigured(r.Context(), aiProviderVsegpt)
	case "guardrail_prompt_append":
		result = h.runGuardrailPromptAppendCheck(r)
	default:
		result["ok"] = false
		result["error"] = "unknown check"
	}
	result["durationMs"] = time.Since(started).Milliseconds()
	return result
}
