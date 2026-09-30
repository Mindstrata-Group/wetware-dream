//go:build integration

package httpapi

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"mindstrata-stage1/api/internal/testsupport"
)

// TestAdminAIModels_Vsegpt_ReturnsEmptyWithNote: vsegpt is free input,
// listing models is not supported (dozens of providers, no single API).
func TestAdminAIModels_Vsegpt_ReturnsEmptyWithNote(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)
	ts.LoginAs(f.CreateSession(f.CreateUser(TestUserOpts{Role: "admin"}).ID))

	code, body := httpJSON(t, ts, "GET", "/api/admin/ai-models?provider=vsegpt", nil)
	if code != http.StatusOK {
		t.Fatalf("vsegpt: %d body=%v", code, body)
	}
	models, _ := body["models"].([]any)
	if len(models) != 0 {
		t.Fatalf("vsegpt models should be empty: %v", models)
	}
}

// TestAdminAIModels_UnconfiguredProvider_ReturnsEmptyWithNote: without a key
// (neither in env nor in system_settings) an empty list with a reason, not an error.
func TestAdminAIModels_UnconfiguredProvider_ReturnsEmptyWithNote(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)
	ts.LoginAs(f.CreateSession(f.CreateUser(TestUserOpts{Role: "admin"}).ID))

	code, body := httpJSON(t, ts, "GET", "/api/admin/ai-models?provider=gemini", nil)
	if code != http.StatusOK {
		t.Fatalf("gemini unconfigured: %d body=%v", code, body)
	}
	models, _ := body["models"].([]any)
	if len(models) != 0 {
		t.Fatalf("unconfigured gemini models should be empty: %v", models)
	}
	if body["note"] == nil {
		t.Fatalf("expected a note explaining why the list is empty")
	}
}

// TestAdminAIModels_Gemini_FiltersAndCaches: the real Gemini /v1beta/models
// response format: filter by generateContent in supportedGenerationMethods;
// the second request is served from cache (does not hit the fake server again).
func TestAdminAIModels_Gemini_FiltersAndCaches(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)

	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.URL.Path != "/v1beta/models" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"models":[
			{"name":"models/gemini-3.1-flash-lite","displayName":"Gemini 3.1 Flash Lite","inputTokenLimit":1000000,"supportedGenerationMethods":["generateContent"],"thinking":true},
			{"name":"models/embedding-001","displayName":"Embedding","inputTokenLimit":2048,"supportedGenerationMethods":["embedContent"],"thinking":false}
		]}`))
	}))
	defer srv.Close()

	// GeminiAPIBaseURL is configured as for chat completions
	// (it ends with /v1beta/openai): fetchGeminiModelCatalog strips that suffix
	// and rebuilds the path to /v1beta/models, hence the same shape here.
	ts := NewTestServerWithHandler(t, env.Pool, Handler{GeminiAPIKey: "gk-test", GeminiAPIBaseURL: srv.URL + "/v1beta/openai"})
	ts.LoginAs(f.CreateSession(f.CreateUser(TestUserOpts{Role: "admin"}).ID))

	code, body := httpJSON(t, ts, "GET", "/api/admin/ai-models?provider=gemini", nil)
	if code != http.StatusOK {
		t.Fatalf("gemini: %d body=%v", code, body)
	}
	models, _ := body["models"].([]any)
	if len(models) != 1 {
		t.Fatalf("expected 1 chat-capable model (embedContent filtered out), got %v", models)
	}
	m0, _ := models[0].(map[string]any)
	if m0["id"] != "gemini-3.1-flash-lite" || m0["displayName"] != "Gemini 3.1 Flash Lite" || m0["supportsThinking"] != true {
		t.Fatalf("unexpected model shape: %+v", m0)
	}

	// The second request must come from cache, with no new hit on the fake server.
	code, body = httpJSON(t, ts, "GET", "/api/admin/ai-models?provider=gemini", nil)
	if code != http.StatusOK {
		t.Fatalf("gemini cached: %d body=%v", code, body)
	}
	if hits.Load() != 1 {
		t.Fatalf("expected exactly 1 upstream hit (second call from cache), got %d", hits.Load())
	}
}

// TestAdminAIModels_Anthropic_ParsesRealShape: the Anthropic GET /v1/models
// response format (data[] with id/display_name/capabilities.thinking.supported).
func TestAdminAIModels_Anthropic_ParsesRealShape(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("x-api-key") != "ak-test" {
			t.Fatalf("missing x-api-key header")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[
			{"id":"claude-sonnet-5","display_name":"Claude Sonnet 5","max_input_tokens":200000,"capabilities":{"thinking":{"supported":true}}}
		],"has_more":false}`))
	}))
	defer srv.Close()

	ts := NewTestServerWithHandler(t, env.Pool, Handler{AnthropicAPIKey: "ak-test", AnthropicAPIBaseURL: srv.URL})
	ts.LoginAs(f.CreateSession(f.CreateUser(TestUserOpts{Role: "admin"}).ID))

	code, body := httpJSON(t, ts, "GET", "/api/admin/ai-models?provider=anthropic", nil)
	if code != http.StatusOK {
		t.Fatalf("anthropic: %d body=%v", code, body)
	}
	models, _ := body["models"].([]any)
	if len(models) != 1 {
		t.Fatalf("expected 1 model, got %v", models)
	}
	m0, _ := models[0].(map[string]any)
	if m0["id"] != "claude-sonnet-5" || m0["displayName"] != "Claude Sonnet 5" || m0["supportsThinking"] != true {
		t.Fatalf("unexpected model shape: %+v", m0)
	}
}

// TestAdminAIModels_AdminKeyOverridesEnv: the key from system_settings takes
// priority over env: an admin edit applies without recreating the container.
func TestAdminAIModels_AdminKeyOverridesEnv(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)

	var gotKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.URL.Query().Get("key")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"models":[]}`))
	}))
	defer srv.Close()

	// An env key exists ("env-key"), but system_settings holds another one: that one must win.
	ts := NewTestServerWithHandler(t, env.Pool, Handler{GeminiAPIKey: "env-key", GeminiAPIBaseURL: srv.URL + "/v1beta/openai"})
	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	if _, err := env.Pool.Exec(t.Context(),
		`insert into system_settings (key, value, description, created_at, updated_at) values ($1, $2, 'test', now(), now())`,
		providerAPIKeySetting(aiProviderGemini), "admin-key"); err != nil {
		t.Fatalf("seed system_settings: %v", err)
	}

	code, body := httpJSON(t, ts, "GET", "/api/admin/ai-models?provider=gemini", nil)
	if code != http.StatusOK {
		t.Fatalf("gemini: %d body=%v", code, body)
	}
	if gotKey != "admin-key" {
		t.Fatalf("expected admin-overridden key to win, got %q", gotKey)
	}
}
