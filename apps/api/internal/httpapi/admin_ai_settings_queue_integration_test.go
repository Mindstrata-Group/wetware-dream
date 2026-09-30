//go:build integration

package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"mindstrata-stage1/api/internal/testsupport"
)

func loginAdminForAISettings(t *testing.T, env *testsupport.Env) (*TestServer, *Factory) {
	t.Helper()
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)
	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))
	return ts, f
}

func aiSettingsPayload(overrides map[string]any) map[string]any {
	payload := map[string]any{
		"summaryModel":              "openai/gpt-4o-mini",
		"summaryTemperature":        "0.3",
		"orchestrationModel":        "openai/gpt-4o-mini",
		"orchestrationTemperature":  "0.1",
		"orchestrationHistoryLimit": "12",
		"chatHistoryLimit":          "20",
		"chatMessageMaxChars":       "30720",
		"retryAttempts":             "2",
		"retryInitialDelayMs":       "250",
		"queueEnabled":              "1",
		"queueIntervalMs":           "1100",
		"queueTimeoutMs":            "60000",
		"queueConcurrency":          "1",
	}
	for key, value := range overrides {
		payload[key] = value
	}
	return payload
}

func TestAdminAISettings_GET_ReturnsQueueDefaults(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts, _ := loginAdminForAISettings(t, env)

	status, body := httpJSON(t, ts, "GET", "/api/admin/ai-settings", nil)
	if status != http.StatusOK {
		t.Fatalf("GET ai-settings status=%d body=%v", status, body)
	}
	for key, want := range map[string]string{
		"summaryModel":              defaultAIFallbackModel,
		"orchestrationModel":        defaultAIOrchestrationModel,
		"orchestrationHistoryLimit": "12",
		"queueEnabled":              "0",
		"queueIntervalMs":           "1100",
		"queueTimeoutMs":            "60000",
		"queueConcurrency":          "1",
		"chatMessageMaxChars":       "30720",
	} {
		if got, _ := body[key].(string); got != want {
			t.Fatalf("%s=%q want %q body=%v", key, got, want, body)
		}
	}
}

func TestAdminAISettings_MessageMaxChars_ClampsBelowMinimumOnChatStart(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	adminServer, _ := loginAdminForAISettings(t, env)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	_ = admin
	adminServer.LoginAs(f.CreateSession(admin.ID))

	status, _ := httpJSON(t, adminServer, "POST", "/api/admin/ai-settings", aiSettingsPayload(map[string]any{
		"chatMessageMaxChars": "0",
	}))
	if status != http.StatusOK {
		t.Fatalf("POST status=%d", status)
	}

	user := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{})
	f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: mode.ID, DailyMessageLimit: 50})
	userServer := NewTestServer(t, env.Pool)
	userServer.LoginAs(f.CreateSession(user.ID))

	status, body := httpJSON(t, userServer, "POST", "/api/chat/start", map[string]any{})
	if status != http.StatusOK {
		t.Fatalf("start status=%d body=%v", status, body)
	}
	got, ok := body["chatMessageMaxChars"].(float64)
	if !ok {
		t.Fatalf("chatMessageMaxChars type=%T value=%v", body["chatMessageMaxChars"], body["chatMessageMaxChars"])
	}
	if int(got) != minChatMessageMaxChars {
		t.Fatalf("chatMessageMaxChars=%d want min=%d", int(got), minChatMessageMaxChars)
	}
}

func TestAdminAISettings_MessageMaxChars_ClampsAboveMaximumOnChatStart(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	adminServer, _ := loginAdminForAISettings(t, env)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	_ = admin
	adminServer.LoginAs(f.CreateSession(admin.ID))

	status, _ := httpJSON(t, adminServer, "POST", "/api/admin/ai-settings", aiSettingsPayload(map[string]any{
		"chatMessageMaxChars": "99999999",
	}))
	if status != http.StatusOK {
		t.Fatalf("POST status=%d", status)
	}

	user := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{})
	f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: mode.ID, DailyMessageLimit: 50})
	userServer := NewTestServer(t, env.Pool)
	userServer.LoginAs(f.CreateSession(user.ID))

	status, body := httpJSON(t, userServer, "POST", "/api/chat/start", map[string]any{})
	if status != http.StatusOK {
		t.Fatalf("start status=%d body=%v", status, body)
	}
	got, ok := body["chatMessageMaxChars"].(float64)
	if !ok {
		t.Fatalf("chatMessageMaxChars type=%T value=%v", body["chatMessageMaxChars"], body["chatMessageMaxChars"])
	}
	if int(got) != maxChatMessageMaxChars {
		t.Fatalf("chatMessageMaxChars=%d want max=%d", int(got), maxChatMessageMaxChars)
	}
}

func TestAdminAISettings_MessageMaxChars_DefaultUsedInChatStart(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	user := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{})
	f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: mode.ID, DailyMessageLimit: 50})
	server := NewTestServer(t, env.Pool)
	server.LoginAs(f.CreateSession(user.ID))

	_, _ = env.Pool.Exec(context.Background(), `delete from system_settings where key = $1`, "chat_message_max_chars")
	status, body := httpJSON(t, server, "POST", "/api/chat/start", map[string]any{})
	if status != http.StatusOK {
		t.Fatalf("start status=%d body=%v", status, body)
	}
	got, ok := body["chatMessageMaxChars"].(float64)
	if !ok {
		t.Fatalf("chatMessageMaxChars type=%T value=%v", body["chatMessageMaxChars"], body["chatMessageMaxChars"])
	}
	if int(got) != defaultChatMessageMaxChars {
		t.Fatalf("chatMessageMaxChars=%d want default=%d", int(got), defaultChatMessageMaxChars)
	}
}

func TestAdminAISettings_POST_PersistsQueueSettings(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts, _ := loginAdminForAISettings(t, env)

	status, body := httpJSON(t, ts, "POST", "/api/admin/ai-settings", map[string]any{
		"summaryModel":             "openai/gpt-4o-mini",
		"summaryTemperature":       "0.3",
		"orchestrationModel":       "openai/gpt-4o-mini",
		"orchestrationTemperature": "0.1",
		"chatHistoryLimit":         "20",
		"retryAttempts":            "2",
		"retryInitialDelayMs":      "250",
		"queueEnabled":             "0",
		"queueIntervalMs":          "1200",
		"queueTimeoutMs":           "65000",
		"queueConcurrency":         "3",
	})
	if status != http.StatusOK {
		t.Fatalf("POST ai-settings status=%d body=%v", status, body)
	}
	for key, want := range map[string]string{
		"ai_queue_enabled":     "0",
		"ai_queue_interval_ms": "1200",
		"ai_queue_timeout_ms":  "65000",
		"ai_queue_concurrency": "3",
	} {
		var got string
		if err := env.Pool.QueryRow(context.Background(), `select value from system_settings where key=$1`, key).Scan(&got); err != nil {
			t.Fatalf("query %s: %v", key, err)
		}
		if got != want {
			t.Fatalf("%s=%q want %q", key, got, want)
		}
	}
}

func TestAdminAISettings_POST_PersistsMessageLimit(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts, _ := loginAdminForAISettings(t, env)

	status, _ := httpJSON(t, ts, "POST", "/api/admin/ai-settings", aiSettingsPayload(map[string]any{
		"chatMessageMaxChars": "12345",
	}))
	if status != http.StatusOK {
		t.Fatalf("POST status=%d", status)
	}
	var got string
	if err := env.Pool.QueryRow(context.Background(), `select value from system_settings where key=$1`, "chat_message_max_chars").Scan(&got); err != nil {
		t.Fatalf("query chat_message_max_chars: %v", err)
	}
	if got != "12345" {
		t.Fatalf("chat_message_max_chars=%q want 12345", got)
	}
}

func TestAdminAISettings_POST_PersistsOrchestrationHistoryLimit(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts, _ := loginAdminForAISettings(t, env)

	status, _ := httpJSON(t, ts, "POST", "/api/admin/ai-settings", aiSettingsPayload(map[string]any{
		"orchestrationHistoryLimit": "8",
	}))
	if status != http.StatusOK {
		t.Fatalf("POST status=%d", status)
	}
	var got string
	if err := env.Pool.QueryRow(context.Background(), `select value from system_settings where key=$1`, "ai_orchestration_history_limit").Scan(&got); err != nil {
		t.Fatalf("query ai_orchestration_history_limit: %v", err)
	}
	if got != "8" {
		t.Fatalf("ai_orchestration_history_limit=%q want 8", got)
	}

	status, body := httpJSON(t, ts, "GET", "/api/admin/ai-settings", nil)
	if status != http.StatusOK {
		t.Fatalf("GET status=%d body=%v", status, body)
	}
	if got, _ := body["orchestrationHistoryLimit"].(string); got != "8" {
		t.Fatalf("orchestrationHistoryLimit=%q want 8 body=%v", got, body)
	}
}

func TestAdminAISettings_POST_ClearsGETCache(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts, _ := loginAdminForAISettings(t, env)

	status, body := httpJSON(t, ts, "GET", "/api/admin/ai-settings", nil)
	if status != http.StatusOK {
		t.Fatalf("initial GET status=%d body=%v", status, body)
	}
	if got, _ := body["queueIntervalMs"].(string); got != "1100" {
		t.Fatalf("initial queueIntervalMs=%q", got)
	}

	status, body = httpJSON(t, ts, "POST", "/api/admin/ai-settings", map[string]any{
		"summaryModel":             "openai/gpt-4o-mini",
		"summaryTemperature":       "0.3",
		"orchestrationModel":       "openai/gpt-4o-mini",
		"orchestrationTemperature": "0.1",
		"chatHistoryLimit":         "20",
		"retryAttempts":            "2",
		"retryInitialDelayMs":      "250",
		"queueEnabled":             "1",
		"queueIntervalMs":          "1500",
		"queueTimeoutMs":           "70000",
		"queueConcurrency":         "2",
	})
	if status != http.StatusOK {
		t.Fatalf("POST status=%d body=%v", status, body)
	}

	status, body = httpJSON(t, ts, "GET", "/api/admin/ai-settings", nil)
	if status != http.StatusOK {
		t.Fatalf("second GET status=%d body=%v", status, body)
	}
	for key, want := range map[string]string{
		"queueEnabled":     "1",
		"queueIntervalMs":  "1500",
		"queueTimeoutMs":   "70000",
		"queueConcurrency": "2",
	} {
		if got, _ := body[key].(string); got != want {
			t.Fatalf("cached %s=%q want %q body=%v", key, got, want, body)
		}
	}
}

func TestAdminAISettings_POST_QueueRuntimeClampsExtremeValues(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts, _ := loginAdminForAISettings(t, env)

	status, body := httpJSON(t, ts, "POST", "/api/admin/ai-settings", aiSettingsPayload(map[string]any{
		"queueEnabled":     "7",
		"queueIntervalMs":  "1",
		"queueTimeoutMs":   "9999999",
		"queueConcurrency": "99",
	}))
	if status != http.StatusOK {
		t.Fatalf("POST status=%d body=%v", status, body)
	}

	settings := Handler{DB: env.Pool}.aiQueueSettings(context.Background())
	if !settings.Enabled {
		t.Fatalf("queue enabled should clamp non-zero value to enabled")
	}
	if settings.Interval != 100*time.Millisecond {
		t.Fatalf("interval=%s want 100ms", settings.Interval)
	}
	if settings.Timeout != 600000*time.Millisecond {
		t.Fatalf("timeout=%s want 600000ms", settings.Timeout)
	}
	if settings.Concurrency != 10 {
		t.Fatalf("concurrency=%d want 10", settings.Concurrency)
	}
}

func TestAdminAISettings_InvalidQueueNumbersUseRuntimeDefaults(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	_, _ = env.Pool.Exec(context.Background(), `
		insert into system_settings (key, value) values
		('ai_queue_enabled', 'abc'),
		('ai_queue_interval_ms', 'abc'),
		('ai_queue_timeout_ms', 'abc'),
		('ai_queue_concurrency', 'abc')
		on conflict (key) do update set value=excluded.value`)

	settings := Handler{DB: env.Pool}.aiQueueSettings(context.Background())
	if settings.Enabled {
		t.Fatalf("invalid queueEnabled should fall back to default disabled")
	}
	if settings.Interval != time.Duration(defaultAIQueueIntervalMS)*time.Millisecond {
		t.Fatalf("interval=%s want default %dms", settings.Interval, defaultAIQueueIntervalMS)
	}
	if settings.Timeout != time.Duration(defaultAIQueueTimeoutMS)*time.Millisecond {
		t.Fatalf("timeout=%s want default %dms", settings.Timeout, defaultAIQueueTimeoutMS)
	}
	if settings.Concurrency != defaultAIQueueConcurrency {
		t.Fatalf("concurrency=%d want default %d", settings.Concurrency, defaultAIQueueConcurrency)
	}
}

func TestAdminAISettings_POST_WritesAuditLog(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)
	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	status, body := httpJSON(t, ts, "POST", "/api/admin/ai-settings", aiSettingsPayload(map[string]any{
		"summaryModel":       "openai/summary-audit",
		"orchestrationModel": "openai/orchestration-audit",
	}))
	if status != http.StatusOK {
		t.Fatalf("POST status=%d body=%v", status, body)
	}

	var auditCount int64
	if err := env.Pool.QueryRow(context.Background(), `
		select count(*)
		from admin_audit_log
		where actor_user_id=$1 and action='admin.ai_settings' and target_type='system_setting'`, admin.ID).Scan(&auditCount); err != nil {
		t.Fatalf("query audit log: %v", err)
	}
	if auditCount != 1 {
		t.Fatalf("auditCount=%d want 1", auditCount)
	}
}

func TestAdminAISettings_NonAdminForbidden(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)
	user := f.CreateUser(TestUserOpts{Role: "user"})
	ts.LoginAs(f.CreateSession(user.ID))

	for _, method := range []string{"GET", "POST"} {
		status, _ := httpJSON(t, ts, method, "/api/admin/ai-settings", map[string]any{})
		if status != http.StatusForbidden && status != http.StatusUnauthorized {
			t.Fatalf("%s non-admin status=%d want 401/403", method, status)
		}
	}
}

func TestAdminAISettings_WrongMethod405(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts, _ := loginAdminForAISettings(t, env)

	for _, method := range []string{http.MethodPut, http.MethodPatch, http.MethodDelete} {
		req, err := http.NewRequest(method, ts.URL("/api/admin/ai-settings"), strings.NewReader(`{}`))
		if err != nil {
			t.Fatalf("request %s: %v", method, err)
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := ts.Client.Do(req)
		if err != nil {
			t.Fatalf("do %s: %v", method, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusMethodNotAllowed {
			t.Fatalf("%s status=%d want 405", method, resp.StatusCode)
		}
	}
}

func TestAIResilience_Fallback404ThroughQueue(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	_, _ = env.Pool.Exec(context.Background(), `
		insert into system_settings (key, value) values
		('ai_provider_vsegpt_default_model', 'openai/fallback-model'),
		('ai_retry_attempts', '1'),
		('ai_queue_enabled', '1'),
		('ai_queue_interval_ms', '100'),
		('ai_queue_timeout_ms', '2000'),
		('ai_queue_concurrency', '1')
		on conflict (key) do update set value=excluded.value`)

	var calls atomic.Int64
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var payload struct {
			Model string `json:"model"`
		}
		_ = json.NewDecoder(r.Body).Decode(&payload)
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(payload.Model, "primary-missing") {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"model not found"}`))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"content": "fallback-ok"}}},
			"usage":   map[string]any{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
		})
	}))
	t.Cleanup(fake.Close)

	h := Handler{
		DB:            env.Pool,
		OpenAIAPIKey:  "test-key",
		OpenAIBaseURL: "https://api.vsegpt.ru/v1",
		HTTPClient:    &http.Client{Transport: vsegptRT{target: fake.URL}, Timeout: 5 * time.Second},
	}
	content, _, _, _, err := h.doAIChatResilient(context.Background(), "openai/primary-missing", 0.2, []map[string]string{{"role": "user", "content": "hi"}}, "queue-fallback-test")
	if err != nil {
		t.Fatalf("doAIChatResilient err=%v", err)
	}
	if content != "fallback-ok" {
		t.Fatalf("content=%q want fallback-ok", content)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("calls=%d want primary 404 + fallback success", got)
	}
}
