//go:build integration

package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"mindstrata-stage1/api/internal/testsupport"
)

// TestAdminMode_ProviderThinking_CreateGetPatch: provider/thinking_mode are
// available via POST/GET/PATCH /api/admin/modes, default vsegpt/default,
// invalid values are normalised (not a 400: silent normalisation like the
// mode's other enum-like fields).
func TestAdminMode_ProviderThinking_CreateGetPatch(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	// Creation without an explicit provider → default vsegpt/default.
	code, body := httpJSON(t, ts, "POST", "/api/admin/modes", map[string]any{
		"name":             "ProviderModeDefault",
		"prompt":           "Test prompt",
		"aiModel":          "openai/gpt-4o-mini",
		"modelTemperature": 0.5,
	})
	if code != http.StatusCreated {
		t.Fatalf("create mode: %d body=%v", code, body)
	}
	mid := int64(body["modeId"].(float64))

	var provider, thinking string
	if err := env.Pool.QueryRow(context.Background(),
		`select ai_provider, thinking_mode from modes where id=$1`, mid).Scan(&provider, &thinking); err != nil {
		t.Fatalf("scan defaults: %v", err)
	}
	if provider != "vsegpt" || thinking != "default" {
		t.Fatalf("defaults: provider=%q thinking=%q, want vsegpt/default", provider, thinking)
	}

	// GET returns the fields.
	code, getBody := httpJSON(t, ts, "GET", fmt.Sprintf("/api/admin/modes/%d", mid), nil)
	if code != http.StatusOK {
		t.Fatalf("get mode: %d body=%v", code, getBody)
	}
	mode, _ := getBody["mode"].(map[string]any)
	if mode["aiProvider"] != "vsegpt" || mode["thinkingMode"] != "default" {
		t.Fatalf("GET mode provider fields: %+v", mode)
	}

	// PATCH changes provider/model/thinking.
	code, patchBody := httpJSON(t, ts, "PATCH", fmt.Sprintf("/api/admin/modes/%d", mid), map[string]any{
		"aiProvider":   "anthropic",
		"aiModel":      "claude-sonnet-5",
		"thinkingMode": "high",
	})
	if code != http.StatusOK {
		t.Fatalf("patch mode: %d body=%v", code, patchBody)
	}
	var aiModel string
	if err := env.Pool.QueryRow(context.Background(),
		`select ai_provider, thinking_mode, ai_model from modes where id=$1`, mid).Scan(&provider, &thinking, &aiModel); err != nil {
		t.Fatalf("scan after patch: %v", err)
	}
	if provider != "anthropic" || thinking != "high" || aiModel != "claude-sonnet-5" {
		t.Fatalf("after patch: provider=%q thinking=%q model=%q", provider, thinking, aiModel)
	}

	// An invalid provider value is silently normalised to vsegpt (see
	// normalizeAIProvider: the same pattern as the mode's other enum fields,
	// not strict 400 validation).
	code, patchBody = httpJSON(t, ts, "PATCH", fmt.Sprintf("/api/admin/modes/%d", mid), map[string]any{
		"aiProvider":   "not-a-real-provider",
		"thinkingMode": "not-a-real-mode",
	})
	if code != http.StatusOK {
		t.Fatalf("patch invalid provider: %d body=%v", code, patchBody)
	}
	if err := env.Pool.QueryRow(context.Background(),
		`select ai_provider, thinking_mode from modes where id=$1`, mid).Scan(&provider, &thinking); err != nil {
		t.Fatalf("scan after invalid patch: %v", err)
	}
	if provider != "vsegpt" || thinking != "default" {
		t.Fatalf("invalid values should normalize: provider=%q thinking=%q", provider, thinking)
	}
}

// TestAdminAISettings_ProviderToggles: GET/POST /api/admin/ai-settings
// contains a providers block with enabled/configured/defaultModel for each
// provider; the toggle really is saved in system_settings.
func TestAdminAISettings_ProviderToggles(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	code, body := httpJSON(t, ts, "GET", "/api/admin/ai-settings", nil)
	if code != http.StatusOK {
		t.Fatalf("get ai-settings: %d body=%v", code, body)
	}
	providers, _ := body["providers"].(map[string]any)
	if providers == nil {
		t.Fatalf("providers block missing: %v", body)
	}
	for _, p := range []string{"vsegpt", "gemini", "anthropic"} {
		entry, ok := providers[p].(map[string]any)
		if !ok {
			t.Fatalf("providers[%q] missing or wrong type: %v", p, providers)
		}
		if _, ok := entry["enabled"]; !ok {
			t.Fatalf("providers[%q].enabled missing", p)
		}
		if _, ok := entry["configured"]; !ok {
			t.Fatalf("providers[%q].configured missing", p)
		}
		if _, ok := entry["defaultModel"]; !ok {
			t.Fatalf("providers[%q].defaultModel missing", p)
		}
	}

	// Turn gemini off via POST, check what was saved in system_settings.
	code, postBody := httpJSON(t, ts, "POST", "/api/admin/ai-settings", map[string]any{
		"providerGeminiEnabled": "0",
	})
	if code != http.StatusOK {
		t.Fatalf("post ai-settings: %d body=%v", code, postBody)
	}
	var value string
	if err := env.Pool.QueryRow(context.Background(),
		`select value from system_settings where key='ai_provider_gemini_enabled'`).Scan(&value); err != nil {
		t.Fatalf("scan setting: %v", err)
	}
	if value != "0" {
		t.Fatalf("ai_provider_gemini_enabled = %q, want 0", value)
	}
}

// TestAdminAISettings_APIKeyNeverLeaksInResponse (Image heuristic: the key is a
// product secret; not leaking it in an HTTP response is an invariant, not a side
// effect; we check it on the raw response body, not only on the structure).
func TestAdminAISettings_APIKeyNeverLeaksInResponse(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServerWithHandler(t, env.Pool, Handler{GeminiAPIKey: "super-secret-gemini-key-xyz"})

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	req, err := http.NewRequest("GET", ts.URL("/api/admin/ai-settings"), nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := ts.Client.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if strings.Contains(string(raw), "super-secret-gemini-key-xyz") {
		t.Fatalf("raw API key leaked in response body: %s", raw)
	}
	if !strings.Contains(string(raw), "****") {
		t.Fatalf("expected masked key marker in response, got: %s", raw)
	}
}

// TestAdminAISettings_SetAPIKey_DBOverridesEnvAndStaysMasked: POST with a new
// key stores it in system_settings (never in clear in the response); a
// following GET shows apiKeySetFromAdmin=true and a different mask.
func TestAdminAISettings_SetAPIKey_DBOverridesEnvAndStaysMasked(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServerWithHandler(t, env.Pool, Handler{AnthropicAPIKey: "env-anthropic-key"})

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	code, postBody := httpJSON(t, ts, "POST", "/api/admin/ai-settings", map[string]any{
		"providerAnthropicApiKey": "brand-new-admin-key-123",
	})
	if code != http.StatusOK {
		t.Fatalf("post ai-settings: %d body=%v", code, postBody)
	}

	var stored string
	if err := env.Pool.QueryRow(context.Background(),
		`select value from system_settings where key=$1`, providerAPIKeySetting(aiProviderAnthropic)).Scan(&stored); err != nil {
		t.Fatalf("scan stored key: %v", err)
	}
	if stored != "brand-new-admin-key-123" {
		t.Fatalf("stored key = %q, want brand-new-admin-key-123", stored)
	}

	code, getBody := httpJSON(t, ts, "GET", "/api/admin/ai-settings", nil)
	if code != http.StatusOK {
		t.Fatalf("get ai-settings: %d body=%v", code, getBody)
	}
	providers, _ := getBody["providers"].(map[string]any)
	anthropic, _ := providers["anthropic"].(map[string]any)
	if anthropic["apiKeySetFromAdmin"] != true {
		t.Fatalf("apiKeySetFromAdmin = %v, want true after admin override", anthropic["apiKeySetFromAdmin"])
	}
	masked, _ := anthropic["apiKeyMasked"].(string)
	if !strings.HasSuffix(masked, "-123") {
		t.Fatalf("masked key should end with real last 4 chars, got %q", masked)
	}
	if strings.Contains(masked, "brand-new-admin-key") {
		t.Fatalf("masked key must not contain the full secret: %q", masked)
	}

	// An empty value in PATCH must not overwrite an already saved key
	// (otherwise one open, unchanged form draft would erase the secret).
	code, postBody = httpJSON(t, ts, "POST", "/api/admin/ai-settings", map[string]any{
		"providerAnthropicApiKey": "",
	})
	if code != http.StatusOK {
		t.Fatalf("post empty key: %d body=%v", code, postBody)
	}
	if err := env.Pool.QueryRow(context.Background(),
		`select value from system_settings where key=$1`, providerAPIKeySetting(aiProviderAnthropic)).Scan(&stored); err != nil {
		t.Fatalf("scan stored key after empty post: %v", err)
	}
	if stored != "brand-new-admin-key-123" {
		t.Fatalf("empty POST must not erase existing key, got %q", stored)
	}
}

// TestDoGeminiChat_EndToEnd: a real HTTP call of doGeminiChat through an
// httptest server (GeminiAPIBaseURL points straight at it: the base URL is
// configurable precisely so tests can do this and so prod can later point it
// at the Deno proxy domain). Checks reasoning_effort in the request body with
// thinking_mode=high, and the cost estimate.
func TestDoGeminiChat_EndToEnd(t *testing.T) {
	t.Parallel()
	var captured map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer gk-test" {
			t.Errorf("Authorization = %q", got)
		}
		_ = json.NewDecoder(r.Body).Decode(&captured)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"content": "привет от gemini"}}},
			"usage": map[string]any{
				"prompt_tokens":     1000,
				"completion_tokens": 200,
				"prompt_tokens_details": map[string]any{
					"cached_tokens": 300,
				},
			},
		})
	}))
	defer srv.Close()

	h := Handler{GeminiAPIKey: "gk-test", GeminiAPIBaseURL: srv.URL, HTTPClient: srv.Client()}
	content, in, out, cost, err := h.doGeminiChat(context.Background(), "gemini-3.1-flash-lite", 0.5, "high",
		[]map[string]string{{"role": "user", "content": "привет"}}, openAIChatOptions{})
	if err != nil {
		t.Fatalf("doGeminiChat: %v", err)
	}
	if content != "привет от gemini" {
		t.Fatalf("content = %q", content)
	}
	if in != 1000 || out != 200 {
		t.Fatalf("in=%d out=%d, want 1000/200", in, out)
	}
	if cost <= 0 {
		t.Fatalf("cost = %v, want > 0", cost)
	}
	if captured["reasoning_effort"] != "high" {
		t.Fatalf("captured reasoning_effort = %v, want high", captured["reasoning_effort"])
	}
}

// TestDoGeminiChat_MissingKey: without a key it is a configuration error, no HTTP call.
func TestDoGeminiChat_MissingKey(t *testing.T) {
	t.Parallel()
	h := Handler{}
	_, _, _, _, err := h.doGeminiChat(context.Background(), "gemini-3.1-flash-lite", 0.5, "default", nil, openAIChatOptions{})
	if err == nil {
		t.Fatalf("expected error when GeminiAPIKey is empty")
	}
}

// TestDoAnthropicChat_EndToEnd: a real HTTP call of doAnthropicChat through an
// httptest server. Checks the native format (system block with cache_control,
// thinking block at high, forced temperature=1) and cost accounting with cache.
func TestDoAnthropicChat_EndToEnd(t *testing.T) {
	t.Parallel()
	var captured map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("x-api-key"); got != "ak-test" {
			t.Errorf("x-api-key = %q", got)
		}
		if got := r.Header.Get("anthropic-version"); got != "2023-06-01" {
			t.Errorf("anthropic-version = %q", got)
		}
		_ = json.NewDecoder(r.Body).Decode(&captured)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"content": []map[string]any{{"type": "text", "text": "привет от claude"}},
			"usage": map[string]any{
				"input_tokens":                1000,
				"output_tokens":               200,
				"cache_read_input_tokens":     500,
				"cache_creation_input_tokens": 50,
			},
		})
	}))
	defer srv.Close()

	h := Handler{AnthropicAPIKey: "ak-test", AnthropicAPIBaseURL: srv.URL, HTTPClient: srv.Client()}
	messages := []map[string]string{
		{"role": "system", "content": "Ты психолог."},
		{"role": "user", "content": "Привет"},
	}
	content, in, out, cost, err := h.doAnthropicChat(context.Background(), "claude-sonnet-5", 0.5, "high", messages, openAIChatOptions{})
	if err != nil {
		t.Fatalf("doAnthropicChat: %v", err)
	}
	if content != "привет от claude" {
		t.Fatalf("content = %q", content)
	}
	if in != 1000+50+500 || out != 200 {
		t.Fatalf("in=%d out=%d", in, out)
	}
	if cost <= 0 {
		t.Fatalf("cost = %v, want > 0", cost)
	}
	if temp, _ := captured["temperature"].(float64); temp != 1 {
		t.Fatalf("temperature = %v, want 1 (forced by extended thinking)", captured["temperature"])
	}
	thinking, _ := captured["thinking"].(map[string]any)
	if thinking == nil || thinking["budget_tokens"].(float64) != 8192 {
		t.Fatalf("thinking = %v, want budget_tokens=8192", captured["thinking"])
	}
	system, _ := captured["system"].([]any)
	if len(system) != 1 {
		t.Fatalf("system blocks = %v, want 1", captured["system"])
	}
	sysBlock := system[0].(map[string]any)
	if sysBlock["cache_control"] == nil {
		t.Fatalf("system block missing cache_control: %v", sysBlock)
	}
}

// TestDoAnthropicChat_MissingKey: without a key it is a configuration error.
func TestDoAnthropicChat_MissingKey(t *testing.T) {
	t.Parallel()
	h := Handler{}
	_, _, _, _, err := h.doAnthropicChat(context.Background(), "claude-sonnet-5", 0.5, "default", nil, openAIChatOptions{})
	if err == nil {
		t.Fatalf("expected error when AnthropicAPIKey is empty")
	}
}

// TestDoAIChatResilientSpec_FallsBackAcrossProviders: the selected gemini fails
// with 429 on every attempt → the chain moves to anthropic (next in the fixed
// order) with ITS default model, not with the gemini model.
func TestDoAIChatResilientSpec_FallsBackAcrossProviders(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)

	_, _ = env.Pool.Exec(context.Background(),
		`insert into system_settings (key, value) values ('ai_retry_attempts', '1'), ('ai_provider_anthropic_default_model', 'claude-haiku-4-5')
		 on conflict (key) do update set value = excluded.value`)

	geminiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":"rate limited"}`))
	}))
	defer geminiSrv.Close()

	var anthropicModelUsed string
	anthropicSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		_ = json.NewDecoder(r.Body).Decode(&payload)
		anthropicModelUsed, _ = payload["model"].(string)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"content": []map[string]any{{"type": "text", "text": "спасён anthropic"}},
			"usage":   map[string]any{"input_tokens": 10, "output_tokens": 5},
		})
	}))
	defer anthropicSrv.Close()

	h := Handler{
		DB:                  env.Pool,
		GeminiAPIKey:        "gk-test",
		GeminiAPIBaseURL:    geminiSrv.URL,
		AnthropicAPIKey:     "ak-test",
		AnthropicAPIBaseURL: anthropicSrv.URL,
		HTTPClient:          &http.Client{Timeout: 5 * time.Second},
	}

	spec := aiCallSpec{Provider: "gemini", Model: "gemini-3.1-flash-lite", Temperature: 0.5, ThinkingMode: "default"}
	content, _, _, _, err := h.doAIChatResilientSpec(context.Background(), spec,
		[]map[string]string{{"role": "user", "content": "привет"}}, "test", openAIChatOptions{})
	if err != nil {
		t.Fatalf("doAIChatResilientSpec: %v", err)
	}
	if content != "спасён anthropic" {
		t.Fatalf("content = %q, want fallback to anthropic", content)
	}
	if anthropicModelUsed != "claude-haiku-4-5" {
		t.Fatalf("anthropic model used = %q, want its own default model claude-haiku-4-5", anthropicModelUsed)
	}
}

// TestDoAIChatResilientSpec_SkipsDisabledProvider: gemini is disabled via
// system_settings → the chain goes straight to anthropic, even though gemini
// has a key (configured, but not enabled).
func TestDoAIChatResilientSpec_SkipsDisabledProvider(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	_, _ = env.Pool.Exec(context.Background(),
		`insert into system_settings (key, value) values ('ai_provider_gemini_enabled', '0')
		 on conflict (key) do update set value = excluded.value`)

	var anthropicCalled bool
	anthropicSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		anthropicCalled = true
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"content": []map[string]any{{"type": "text", "text": "ok"}},
			"usage":   map[string]any{"input_tokens": 1, "output_tokens": 1},
		})
	}))
	defer anthropicSrv.Close()

	h := Handler{
		DB:                  env.Pool,
		GeminiAPIKey:        "gk-test", // has a key, but disabled by setting
		GeminiAPIBaseURL:    "http://127.0.0.1:0",
		AnthropicAPIKey:     "ak-test",
		AnthropicAPIBaseURL: anthropicSrv.URL,
		HTTPClient:          &http.Client{Timeout: 5 * time.Second},
	}
	spec := aiCallSpec{Provider: "gemini", Model: "gemini-3.1-flash-lite", Temperature: 0.5, ThinkingMode: "default"}
	_, _, _, _, err := h.doAIChatResilientSpec(context.Background(), spec,
		[]map[string]string{{"role": "user", "content": "привет"}}, "test", openAIChatOptions{})
	if err != nil {
		t.Fatalf("doAIChatResilientSpec: %v", err)
	}
	if !anthropicCalled {
		t.Fatalf("expected fallback straight to anthropic since gemini disabled")
	}
}
