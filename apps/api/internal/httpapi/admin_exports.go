package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

func (h Handler) AdminExports(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.requireAdminSection(w, r, "exports", r.Method != http.MethodGet)
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		if r.URL.Query().Get("options") == "true" {
			h.adminExportOptions(w, r)
			return
		}
		modeIDs := parseAdminIDList(r.URL.Query()["modeIds"], r.URL.Query().Get("modeId"))
		userIDs := parseAdminIDList(r.URL.Query()["userIds"], r.URL.Query().Get("userId"))
		promoIDs := parseAdminIDList(r.URL.Query()["promocodeIds"], r.URL.Query().Get("promocodeId"))
		withSummary := r.URL.Query().Get("withSummary") == "true"
		summaryPrompt := strings.TrimSpace(r.URL.Query().Get("summaryPrompt"))
		items, exportText, err := h.collectAdminExport(r.Context(), modeIDs, userIDs, promoIDs, adminExportRoleFilter(r.URL.Query().Get("roleFilter")), adminExportLimit(r.URL.Query().Get("limit")), r.URL.Query().Get("dateFrom"), r.URL.Query().Get("dateTo"))
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		response := map[string]any{"ok": true, "messages": items, "messageCount": len(items), "sourceBytes": len(exportText), "approxTokens": approxTokenCount(exportText)}
		if withSummary {
			if summaryPrompt == "" {
				summaryPrompt = "В начале ответа дословно процитируй задачу пользователя в формате: «Задача: ...». Затем дай саммари строго нумерованным списком: 1) кто пишет; 2) боли; 3) повторяющиеся вопросы; 4) буквальные формулировки спроса; 5) что предложить; 6) темы для конверсии; 7) сообщения для оффера."
			}
			response["summaryPayload"] = summaryPrompt + "\n\nЗадача: проанализировать файл выгрузки ниже как текст и выдать нумерованное саммари.\n\n" + exportText
		}
		h.writeAdminAudit(r.Context(), r, actor.ID, "admin.export.messages.read", "export", nil, map[string]any{
			"modeIds":               modeIDs,
			"userIds":               userIDs,
			"promocodeIds":          promoIDs,
			"roleFilter":            r.URL.Query().Get("roleFilter"),
			"limit":                 adminExportLimit(r.URL.Query().Get("limit")),
			"withSummary":           withSummary,
			"messageCount":          len(items),
			"sourceBytes":           len(exportText),
			"approxTokens":          approxTokenCount(exportText),
			"summaryPromptProvided": summaryPrompt != "",
		})
		writeJSON(w, http.StatusOK, response)
	case http.MethodPost:
		var req adminExportSummaryRequest
		if err := decodeJSONStrict(w, r, 256<<10, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid payload"})
			return
		}
		items, exportText, err := h.collectAdminExport(r.Context(), uniquePositiveIDs(req.ModeIDs), uniquePositiveIDs(req.UserIDs), uniquePositiveIDs(req.PromocodeIDs), adminExportRoleFilter(req.RoleFilter), adminExportLimitInt(req.Limit), req.DateFrom, req.DateTo)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		prompt := strings.TrimSpace(req.Prompt)
		if prompt == "" && req.PromptID > 0 {
			_ = h.DB.QueryRow(r.Context(), `select prompt from admin_summary_prompts where id=$1`, req.PromptID).Scan(&prompt)
		}
		if prompt == "" {
			prompt = "В начале ответа дословно процитируй задачу пользователя в формате: «Задача: ...». Затем дай саммари строго нумерованным списком по аудитории, болям, вопросам, формулировкам спроса и офферам."
		}
		model, temperature := h.adminSummaryModel(r.Context())
		messages := []map[string]string{{"role": "system", "content": prompt}, {"role": "user", "content": "Задача: проанализировать сырые сообщения ниже и выдать нумерованное саммари.\n\n" + exportText}}
		result, _, _, _, err := h.doMechanicAIChat(r.Context(), "ai_summary_provider", model, temperature, messages, buildXTitle(0, actor.ID, "ADMIN_EXPORT_SUMMARY"), openAIChatOptions{})
		if err != nil {
			response := liveAIErrorResponse(err)
			response["messageCount"] = len(items)
			response["sourceBytes"] = len(exportText)
			response["approxTokens"] = approxTokenCount(exportText)
			writeJSON(w, http.StatusBadRequest, response)
			return
		}
		filters := map[string]any{"modeIds": req.ModeIDs, "userIds": req.UserIDs, "promocodeIds": req.PromocodeIDs}
		filterJSON, _ := json.Marshal(filters)
		var id int64
		_ = h.DB.QueryRow(r.Context(), `insert into admin_export_summaries (actor_user_id, prompt_id, prompt, filters, source_message_count, source_bytes, approx_tokens, result, created_at) values ($1,$2,$3,$4::jsonb,$5,$6,$7,$8,now()) returning id`, actor.ID, nullablePositive(req.PromptID), prompt, string(filterJSON), len(items), len(exportText), approxTokenCount(exportText), result).Scan(&id)
		h.writeAdminAudit(r.Context(), r, actor.ID, "admin.export.messages.summary", "export_summary", &id, map[string]any{
			"modeIds":      req.ModeIDs,
			"userIds":      req.UserIDs,
			"promocodeIds": req.PromocodeIDs,
			"roleFilter":   req.RoleFilter,
			"limit":        req.Limit,
			"promptId":     req.PromptID,
			"messageCount": len(items),
			"sourceBytes":  len(exportText),
			"approxTokens": approxTokenCount(exportText),
		})
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "summaryId": id, "result": result, "messageCount": len(items), "sourceBytes": len(exportText), "approxTokens": approxTokenCount(exportText), "createdAt": time.Now()})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
	}
}
