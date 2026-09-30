package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type testerRunCheckRequest struct {
	Check string `json:"check"`
}

func testerSafeChecks() []string {
	return []string{"db", "auth_core", "users", "modes", "public_demo_modes", "access_quota", "chat_tables", "tariffs", "promocodes", "promo_apply_infra", "exports", "ai_settings", "live_config", "ollama_config"}
}

func (h Handler) TesterUsers(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.requireRole(w, r, "tester", "owner", "admin", "support")
	if !ok {
		return
	}
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	where := "where deleted_at is null"
	args := []any{}
	if q != "" {
		where += " and (lower(coalesce(email, '')) like lower($1) or lower(coalesce(telegram_username, '')) like lower($1) or id::text = $2)"
		args = append(args, "%"+q+"%", q)
	}
	rows, err := h.DB.Query(r.Context(), fmt.Sprintf(`
		select id, coalesce(email,''), coalesce(telegram_username,''), role, status
		from users
		%s
		order by id desc
		limit 50`, where), args...)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id int64
		var email, username, role, status string
		if err := rows.Scan(&id, &email, &username, &role, &status); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		out = append(out, map[string]any{"id": id, "email": email, "telegramUsername": username, "role": role, "status": status})
	}
	h.writeAdminAudit(r.Context(), r, actor.ID, "tester.users.read", "user", nil, map[string]any{"q": q})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "users": out})
}

func (h Handler) TesterUserDetail(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.requireRole(w, r, "tester", "owner", "admin", "support")
	if !ok {
		return
	}
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
		return
	}
	userID, _, err := parseIDFromPath(r.URL.Path, "/api/tester/users/")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid user id"})
		return
	}
	h.writeAdminAudit(r.Context(), r, actor.ID, "tester.user.read", "user", &userID, nil)
	h.adminGetUser(w, r, userID)
}

func (h Handler) TesterStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
		return
	}
	user, ok := h.requireRole(w, r, "tester", "admin")
	if !ok {
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	dbOK := false
	dbError := ""
	if h.DB != nil {
		if err := h.DB.Ping(ctx); err != nil {
			dbError = err.Error()
		} else {
			dbOK = true
		}
	}

	var modeCount, publicDemoCount int64
	_ = h.DB.QueryRow(r.Context(), `select count(*) from modes where hidden_at is null`).Scan(&modeCount)
	_ = h.DB.QueryRow(r.Context(), `select count(*) from public.get_public_demo_modes_cached()`).Scan(&publicDemoCount)

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":     true,
		"tester": user,
		"db":     map[string]any{"ok": dbOK, "error": dbError},
		"liveAI": map[string]any{"enabled": h.gatewayConfigured(r.Context(), aiProviderVsegpt), "baseURLConfigured": strings.TrimSpace(h.OpenAIBaseURL) != "", "apiKeyConfigured": h.gatewayConfigured(r.Context(), aiProviderVsegpt)},
		"counts": map[string]any{"modes": modeCount, "publicDemoModes": publicDemoCount},
		"time":   time.Now(),
	})
}

func (h Handler) runGuardrailPromptAppendCheck(r *http.Request) map[string]any {
	result := map[string]any{"check": "guardrail_prompt_append", "ok": true}
	var mode modeRow
	if err := h.DB.QueryRow(r.Context(), `select id, name, prompt, coalesce(welcome_message,''), ai_model, model_temperature from modes where hidden_at is null order by random() limit 1`).Scan(&mode.ID, &mode.Name, &mode.Prompt, &mode.WelcomeMessage, &mode.AIModel, &mode.Temperature); err != nil {
		result["ok"] = false
		result["error"] = err.Error()
		return result
	}
	guardrail, err := h.defaultModeGuardrail(r.Context())
	if err != nil {
		result["ok"] = false
		result["error"] = err.Error()
		return result
	}
	finalPrompt := buildRuntimeModePrompt(mode.Prompt, guardrail)
	result["modeId"] = mode.ID
	result["modeName"] = mode.Name
	result["basePromptTail"] = tailString(mode.Prompt, 500)
	result["attachedGuardrail"] = guardrail
	result["finalPromptTail"] = tailString(finalPrompt, 1200)
	if h.gatewayConfigured(r.Context(), aiProviderVsegpt) {
		messages := []map[string]string{
			{"role": "system", "content": finalPrompt},
			{"role": "user", "content": "Тест: ответь одной короткой фразой, что защитный блок приложен в конце системного промпта."},
		}
		answer, _, _, _, err := h.doAIChatResilient(r.Context(), mode.AIModel, mode.Temperature, messages, buildXTitle(mode.ID, 0, "GUARDRAIL_TEST"))
		if err != nil {
			result["liveResponseError"] = err.Error()
		} else {
			result["liveResponse"] = answer
		}
	} else {
		result["liveResponse"] = "Live AI выключен: тест показал сформированный finalPromptTail без отправки в модель."
	}
	return result
}

func (h Handler) TesterUserChatPreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
		return
	}
	actor, ok := h.requireRole(w, r, "tester", "admin")
	if !ok {
		return
	}
	userID, _ := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("userId")), 10, 64)
	if userID <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "userId is required"})
		return
	}
	var email, role, status, username string
	if err := h.DB.QueryRow(r.Context(), `select coalesce(email,''), role, status, coalesce(telegram_username,'') from users where id=$1 and deleted_at is null`, userID).Scan(&email, &role, &status, &username); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"ok": false, "error": "user not found"})
		return
	}
	modes, err := h.listModes(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	modes = h.addQuotasToModes(r.Context(), userID, modes)
	visibleModes := []ModeOption{}
	for _, mode := range modes {
		if mode.Quota == nil {
			continue
		}
		visibleModes = append(visibleModes, mode)
	}
	h.writeAdminAudit(r.Context(), r, actor.ID, "tester.user_chat_preview", "user", &userID, nil)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":              true,
		"user":            map[string]any{"id": userID, "email": email, "telegramUsername": username, "role": role, "status": status},
		"modes":           visibleModes,
		"historyScrubbed": true,
		"readOnly":        true,
	})
}

func tailString(value string, limit int) string {
	value = strings.TrimSpace(value)
	if len(value) <= limit {
		return value
	}
	return value[len(value)-limit:]
}

func (h Handler) TesterRunCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
		return
	}
	user, ok := h.requireRole(w, r, "tester", "admin")
	if !ok {
		return
	}
	var req testerRunCheckRequest
	_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&req)
	check := strings.TrimSpace(strings.ToLower(req.Check))
	if check == "" {
		check = "db"
	}

	started := time.Now()
	result := map[string]any{"check": check, "ok": true}
	switch check {
	case "all", "all_safe":
		items := testerSafeChecks()
		if check == "all" {
			items = append(items, "guardrail_prompt_append")
		}
		results := []map[string]any{}
		allOK := true
		for _, item := range items {
			res := h.runTesterCheck(r, item)
			if ok, _ := res["ok"].(bool); !ok {
				allOK = false
			}
			results = append(results, res)
		}
		result["ok"] = allOK
		result["results"] = results
	default:
		result = h.runTesterCheck(r, check)
		if ok, _ := result["ok"].(bool); !ok && result["error"] == "unknown check" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "unknown check"})
			return
		}
	}
	result["durationMs"] = time.Since(started).Milliseconds()
	h.writeAdminAudit(r.Context(), r, user.ID, "tester.check", "system", nil, map[string]any{"check": check, "result": result})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "result": result})
}
