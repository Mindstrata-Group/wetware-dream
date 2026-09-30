package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

func (h Handler) AdminAISettings(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.requireAdminSection(w, r, "orchestration", r.Method != http.MethodGet)
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		if writeNamedCached(w, "admin_ai_settings", &h.c.adminAISettings) {
			return
		}
		// Ollama fields removed on 2026-05-28; the response only returns the OpenAI pipeline.
		summaryModel, summaryTemperature := h.adminSummaryModel(r.Context())
		orchestrationModel, orchestrationTemperature := h.configuredAIModel(r.Context(), "ai_orchestration_model", "ai_orchestration_temperature", defaultAIOrchestrationModel, defaultAIOrchestrationTemperature)
		leadSummaryModel, leadSummaryTemperature := h.configuredAIModel(r.Context(), "ai_lead_summary_model", "ai_lead_summary_temperature", defaultAIOrchestrationModel, defaultAIOrchestrationTemperature)
		orchestrationHistoryLimit := strconv.Itoa(h.orchestrationHistoryLimit(r.Context()))
		chatHistoryLimit, _ := h.systemSetting(r.Context(), "ai_chat_history_limit")
		if chatHistoryLimit == "" {
			chatHistoryLimit = "10"
		}
		chatMessageMaxChars := h.chatMessageMaxChars(r.Context())
		retryAttempts, _ := h.systemSetting(r.Context(), "ai_retry_attempts")
		if retryAttempts == "" {
			retryAttempts = "3"
		}
		retryDelayMs, _ := h.systemSetting(r.Context(), "ai_retry_initial_delay_ms")
		if retryDelayMs == "" {
			retryDelayMs = "1500"
		}
		queueEnabled, _ := h.systemSetting(r.Context(), "ai_queue_enabled")
		if queueEnabled == "" {
			queueEnabled = "0"
		}
		queueIntervalMs, _ := h.systemSetting(r.Context(), "ai_queue_interval_ms")
		if queueIntervalMs == "" {
			queueIntervalMs = "1100"
		}
		queueTimeoutMs, _ := h.systemSetting(r.Context(), "ai_queue_timeout_ms")
		if queueTimeoutMs == "" {
			queueTimeoutMs = "60000"
		}
		queueConcurrency, _ := h.systemSetting(r.Context(), "ai_queue_concurrency")
		if queueConcurrency == "" {
			queueConcurrency = "1"
		}
		attachmentSettings := h.chatAttachmentSettings(r.Context())
		providers := map[string]any{}
		for _, p := range []string{aiProviderVsegpt, aiProviderGemini, aiProviderAnthropic} {
			apiKeySetting, _ := h.systemSetting(r.Context(), providerAPIKeySetting(p))
			providers[p] = map[string]any{
				"enabled":      h.providerEnabled(r.Context(), p),
				"configured":   h.providerConfigured(r.Context(), p),
				"defaultModel": h.providerDefaultModel(r.Context(), p),
				"apiKeyMasked": maskSecret(h.providerAPIKey(r.Context(), p)),
				// apiKeySetFromAdmin: true if the key is overridden from the admin UI
				// (not just inherited from env), so the UI can show the source.
				"apiKeySetFromAdmin": strings.TrimSpace(apiKeySetting) != "",
			}
		}
		body, _ := json.Marshal(map[string]any{
			"providers":                  providers,
			"ok":                         true,
			"summaryModel":               summaryModel,
			"summaryTemperature":         summaryTemperature,
			"orchestrationModel":         orchestrationModel,
			"orchestrationTemperature":   orchestrationTemperature,
			"leadSummaryModel":           leadSummaryModel,
			"leadSummaryTemperature":     leadSummaryTemperature,
			"orchestrationHistoryLimit":  orchestrationHistoryLimit,
			"chatHistoryLimit":           chatHistoryLimit,
			"chatMessageMaxChars":        strconv.Itoa(chatMessageMaxChars),
			"retryAttempts":              retryAttempts,
			"retryInitialDelayMs":        retryDelayMs,
			"queueEnabled":               queueEnabled,
			"queueIntervalMs":            queueIntervalMs,
			"queueTimeoutMs":             queueTimeoutMs,
			"queueConcurrency":           queueConcurrency,
			"attachmentDirectMaxBytes":   strconv.FormatInt(attachmentSettings.DirectMaxBytes, 10),
			"attachmentMaxUploadBytes":   strconv.FormatInt(attachmentSettings.MaxUploadBytes, 10),
			"attachmentContextMaxChars":  strconv.Itoa(attachmentSettings.ContextMaxChars),
			"attachmentAnnotationModel":  attachmentSettings.Model,
			"attachmentAnnotationPrompt": attachmentSettings.Prompt,
			// Provider for service AI mechanics, separate from the per-mode ai_provider
			// (these calls are not tied to a specific mode).
			"orchestrationProvider":        h.mechanicProvider(r.Context(), "ai_orchestration_provider"),
			"leadSummaryProvider":          h.mechanicProvider(r.Context(), "ai_lead_summary_provider"),
			"summaryProvider":              h.mechanicProvider(r.Context(), "ai_summary_provider"),
			"attachmentAnnotationProvider": h.mechanicProvider(r.Context(), "chat_attachment_annotation_provider"),
		})
		h.c.adminAISettings.set(body, adminConfigCacheTTL)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	case http.MethodPost:
		var req struct {
			SummaryModel                  string  `json:"summaryModel"`
			SummaryTemperature            string  `json:"summaryTemperature"`
			OrchestrationModel            string  `json:"orchestrationModel"`
			OrchestrationTemperature      string  `json:"orchestrationTemperature"`
			LeadSummaryModel              string  `json:"leadSummaryModel"`
			LeadSummaryTemperature        string  `json:"leadSummaryTemperature"`
			OrchestrationHistoryLimit     *string `json:"orchestrationHistoryLimit"`
			ChatHistoryLimit              string  `json:"chatHistoryLimit"`
			ChatMessageMaxChars           *string `json:"chatMessageMaxChars"`
			RetryAttempts                 string  `json:"retryAttempts"`
			RetryInitialDelayMs           string  `json:"retryInitialDelayMs"`
			QueueEnabled                  string  `json:"queueEnabled"`
			QueueIntervalMs               string  `json:"queueIntervalMs"`
			QueueTimeoutMs                string  `json:"queueTimeoutMs"`
			QueueConcurrency              string  `json:"queueConcurrency"`
			AttachmentDirectMaxBytes      *string `json:"attachmentDirectMaxBytes"`
			AttachmentMaxUploadBytes      *string `json:"attachmentMaxUploadBytes"`
			AttachmentContextMaxChars     *string `json:"attachmentContextMaxChars"`
			AttachmentAnnotationModel     *string `json:"attachmentAnnotationModel"`
			AttachmentAnnotationPrompt    *string `json:"attachmentAnnotationPrompt"`
			ProviderVsegptEnabled         *string `json:"providerVsegptEnabled"`
			ProviderGeminiEnabled         *string `json:"providerGeminiEnabled"`
			ProviderAnthropicEnabled      *string `json:"providerAnthropicEnabled"`
			ProviderGeminiDefaultModel    *string `json:"providerGeminiDefaultModel"`
			ProviderAnthropicDefaultModel *string `json:"providerAnthropicDefaultModel"`
			ProviderVsegptDefaultModel    *string `json:"providerVsegptDefaultModel"`
			ProviderVsegptAPIKey          *string `json:"providerVsegptApiKey"`
			ProviderGeminiAPIKey          *string `json:"providerGeminiApiKey"`
			ProviderAnthropicAPIKey       *string `json:"providerAnthropicApiKey"`
			OrchestrationProvider         *string `json:"orchestrationProvider"`
			LeadSummaryProvider           *string `json:"leadSummaryProvider"`
			SummaryProvider               *string `json:"summaryProvider"`
			AttachmentAnnotationProvider  *string `json:"attachmentAnnotationProvider"`
		}
		if err := decodeJSONStrict(w, r, 64<<10, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid payload"})
			return
		}
		settings := map[string]string{
			"ai_summary_model":             strings.TrimSpace(req.SummaryModel),
			"ai_summary_temperature":       strings.TrimSpace(req.SummaryTemperature),
			"ai_orchestration_model":       strings.TrimSpace(req.OrchestrationModel),
			"ai_orchestration_temperature": strings.TrimSpace(req.OrchestrationTemperature),
			"ai_lead_summary_model":        strings.TrimSpace(req.LeadSummaryModel),
			"ai_lead_summary_temperature":  strings.TrimSpace(req.LeadSummaryTemperature),
			"ai_chat_history_limit":        strings.TrimSpace(req.ChatHistoryLimit),
			"ai_retry_attempts":            strings.TrimSpace(req.RetryAttempts),
			"ai_retry_initial_delay_ms":    strings.TrimSpace(req.RetryInitialDelayMs),
			"ai_queue_enabled":             strings.TrimSpace(req.QueueEnabled),
			"ai_queue_interval_ms":         strings.TrimSpace(req.QueueIntervalMs),
			"ai_queue_timeout_ms":          strings.TrimSpace(req.QueueTimeoutMs),
			"ai_queue_concurrency":         strings.TrimSpace(req.QueueConcurrency),
		}
		if req.ChatMessageMaxChars != nil {
			settings["chat_message_max_chars"] = strings.TrimSpace(*req.ChatMessageMaxChars)
		}
		if req.OrchestrationHistoryLimit != nil {
			settings["ai_orchestration_history_limit"] = strings.TrimSpace(*req.OrchestrationHistoryLimit)
		}
		if req.AttachmentDirectMaxBytes != nil {
			settings["chat_attachment_direct_max_bytes"] = strings.TrimSpace(*req.AttachmentDirectMaxBytes)
		}
		if req.AttachmentMaxUploadBytes != nil {
			settings["chat_attachment_max_upload_bytes"] = strings.TrimSpace(*req.AttachmentMaxUploadBytes)
		}
		if req.AttachmentContextMaxChars != nil {
			settings["chat_attachment_context_max_chars"] = strings.TrimSpace(*req.AttachmentContextMaxChars)
		}
		if req.AttachmentAnnotationModel != nil {
			settings["chat_attachment_annotation_model"] = strings.TrimSpace(*req.AttachmentAnnotationModel)
		}
		if req.AttachmentAnnotationPrompt != nil {
			settings["chat_attachment_annotation_prompt"] = strings.TrimSpace(*req.AttachmentAnnotationPrompt)
		}
		if req.ProviderVsegptEnabled != nil {
			settings["ai_provider_vsegpt_enabled"] = strings.TrimSpace(*req.ProviderVsegptEnabled)
		}
		if req.ProviderGeminiEnabled != nil {
			settings["ai_provider_gemini_enabled"] = strings.TrimSpace(*req.ProviderGeminiEnabled)
		}
		if req.ProviderAnthropicEnabled != nil {
			settings["ai_provider_anthropic_enabled"] = strings.TrimSpace(*req.ProviderAnthropicEnabled)
		}
		if req.ProviderGeminiDefaultModel != nil {
			settings["ai_provider_gemini_default_model"] = strings.TrimSpace(*req.ProviderGeminiDefaultModel)
		}
		if req.ProviderAnthropicDefaultModel != nil {
			settings["ai_provider_anthropic_default_model"] = strings.TrimSpace(*req.ProviderAnthropicDefaultModel)
		}
		if req.ProviderVsegptDefaultModel != nil {
			settings["ai_provider_vsegpt_default_model"] = strings.TrimSpace(*req.ProviderVsegptDefaultModel)
		}
		// Keys are written ONLY when non-empty: an empty string from the frontend
		// (the field was left untouched and shows a mask) must not overwrite
		// an already saved key. An explicit reset goes through system_settings directly.
		if req.ProviderVsegptAPIKey != nil && strings.TrimSpace(*req.ProviderVsegptAPIKey) != "" {
			settings[providerAPIKeySetting(aiProviderVsegpt)] = strings.TrimSpace(*req.ProviderVsegptAPIKey)
		}
		if req.ProviderGeminiAPIKey != nil && strings.TrimSpace(*req.ProviderGeminiAPIKey) != "" {
			settings[providerAPIKeySetting(aiProviderGemini)] = strings.TrimSpace(*req.ProviderGeminiAPIKey)
		}
		if req.ProviderAnthropicAPIKey != nil && strings.TrimSpace(*req.ProviderAnthropicAPIKey) != "" {
			settings[providerAPIKeySetting(aiProviderAnthropic)] = strings.TrimSpace(*req.ProviderAnthropicAPIKey)
		}
		if req.OrchestrationProvider != nil {
			settings["ai_orchestration_provider"] = normalizeAIProvider(*req.OrchestrationProvider)
		}
		if req.LeadSummaryProvider != nil {
			settings["ai_lead_summary_provider"] = normalizeAIProvider(*req.LeadSummaryProvider)
		}
		if req.SummaryProvider != nil {
			settings["ai_summary_provider"] = normalizeAIProvider(*req.SummaryProvider)
		}
		if req.AttachmentAnnotationProvider != nil {
			settings["chat_attachment_annotation_provider"] = normalizeAIProvider(*req.AttachmentAnnotationProvider)
		}
		for _, key := range []string{"ai_summary_model", "ai_orchestration_model", "ai_lead_summary_model", "chat_attachment_annotation_model"} {
			value, ok := settings[key]
			if !ok {
				continue
			}
			normalizedModel, err := normalizeOptionalAIModelID(value)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": key + ": " + err.Error()})
				return
			}
			settings[key] = normalizedModel
		}
		for key, value := range settings {
			_, err := h.DB.Exec(r.Context(), `
				insert into system_settings (key, value, description, created_at, updated_at)
				values ($1, $2, 'AI provider runtime setting', now(), now())
				on conflict (key) do update set value=excluded.value, updated_at=now()`, key, value)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
				return
			}
		}
		h.c.adminAISettings.clear()
		attachmentModel := ""
		if req.AttachmentAnnotationModel != nil {
			attachmentModel = strings.TrimSpace(*req.AttachmentAnnotationModel)
		}
		h.writeAdminAudit(r.Context(), r, actor.ID, "admin.ai_settings", "system_setting", nil, map[string]any{
			"summaryModel":              strings.TrimSpace(req.SummaryModel),
			"orchestrationModel":        strings.TrimSpace(req.OrchestrationModel),
			"attachmentAnnotationModel": attachmentModel,
			// The keys themselves are NOT written to the audit log, only the fact of the change, for traceability.
			"vsegptApiKeyChanged":    req.ProviderVsegptAPIKey != nil && strings.TrimSpace(*req.ProviderVsegptAPIKey) != "",
			"geminiApiKeyChanged":    req.ProviderGeminiAPIKey != nil && strings.TrimSpace(*req.ProviderGeminiAPIKey) != "",
			"anthropicApiKeyChanged": req.ProviderAnthropicAPIKey != nil && strings.TrimSpace(*req.ProviderAnthropicAPIKey) != "",
		})
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
	}
}

func (h Handler) systemSetting(ctx context.Context, key string) (string, error) {
	if h.DB == nil {
		return "", nil
	}
	var value string
	err := h.DB.QueryRow(ctx, `select coalesce(value, '') from system_settings where key=$1`, key).Scan(&value)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return strings.TrimSpace(value), err
}
