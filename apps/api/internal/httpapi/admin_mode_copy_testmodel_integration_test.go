//go:build integration

package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mindstrata-stage1/api/internal/testsupport"
)

// TestAdminModeDetail_Copy_DuplicatesContentAndClearsCaches (Claims: the copy's
// name is declared as "Copy of {name}" (in Russian), and the mode itself must really appear
// in the DB with the same prompt, not just return 201 with an arbitrary id).
func TestAdminModeDetail_Copy_DuplicatesContentAndClearsCaches(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	mode := f.CreateMode(TestModeOpts{Name: "Оригинал", Prompt: "Исходный промпт режима"})
	ts.LoginAs(f.CreateSession(admin.ID))

	status, body := httpJSON(t, ts, "POST", "/api/admin/modes/"+itoa(mode.ID)+"/copy", nil)
	if status != http.StatusCreated {
		t.Fatalf("copy: %d body=%v", status, body)
	}
	newID := int64(body["modeId"].(float64))
	if newID == mode.ID {
		t.Fatalf("copy returned the same id as the source")
	}

	var name, prompt string
	if err := env.Pool.QueryRow(context.Background(),
		`select name, prompt from modes where id=$1`, newID).Scan(&name, &prompt); err != nil {
		t.Fatalf("scan copy: %v", err)
	}
	if name != "Копия Оригинал" {
		t.Fatalf("copy name = %q, want %q", name, "Копия Оригинал")
	}
	if prompt != "Исходный промпт режима" {
		t.Fatalf("copy prompt = %q, want the source prompt unchanged", prompt)
	}
}

// TestAdminModeDetail_Copy_NotFound_404: copying a mode that does not exist.
func TestAdminModeDetail_Copy_NotFound_404(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)
	ts.LoginAs(f.CreateSession(f.CreateUser(TestUserOpts{Role: "admin"}).ID))

	status, body := httpJSON(t, ts, "POST", "/api/admin/modes/999999999/copy", nil)
	if status != http.StatusNotFound {
		t.Fatalf("copy missing mode: %d body=%v", status, body)
	}
}

// TestAdminModeDetail_Copy_WrongMethod_405.
func TestAdminModeDetail_Copy_WrongMethod_405(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)
	mode := f.CreateMode(TestModeOpts{})
	ts.LoginAs(f.CreateSession(f.CreateUser(TestUserOpts{Role: "admin"}).ID))

	status, _ := httpJSON(t, ts, "GET", "/api/admin/modes/"+itoa(mode.ID)+"/copy", nil)
	if status != http.StatusMethodNotAllowed {
		t.Fatalf("GET on /copy: %d, want 405", status)
	}
}

// TestAdminModeDetail_TestModel_RoutesThroughModeProviderByDefault (History:
// this is exactly the branch that used to fail silently: test-model ignored
// the provider from the request body and always went down the vsegpt path; the
// fix was checked by hand in the same session, this is the permanent HTTP-level
// regression test with a real fake Gemini server via httptest).
func TestAdminModeDetail_TestModel_RoutesThroughModeProviderByDefault(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)

	var gotAuth string
	geminiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"тестовый ответ gemini"}}],"usage":{"prompt_tokens":10,"completion_tokens":5}}`))
	}))
	defer geminiSrv.Close()

	ts := NewTestServerWithHandler(t, env.Pool, Handler{GeminiAPIKey: "gk-test", GeminiAPIBaseURL: geminiSrv.URL})
	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	mode := f.CreateMode(TestModeOpts{})
	if _, err := env.Pool.Exec(context.Background(),
		`update modes set ai_provider='gemini', ai_model='gemini-3.1-flash-lite' where id=$1`, mode.ID); err != nil {
		t.Fatalf("seed provider: %v", err)
	}
	ts.LoginAs(f.CreateSession(admin.ID))

	status, body := httpJSON(t, ts, "POST", "/api/admin/modes/"+itoa(mode.ID)+"/test-model", map[string]any{
		"text": "Привет",
	})
	if status != http.StatusOK {
		t.Fatalf("test-model: %d body=%v", status, body)
	}
	if body["provider"] != "gemini" {
		t.Fatalf("provider = %v, want gemini (должен взяться из режима по умолчанию)", body["provider"])
	}
	if body["response"] != "тестовый ответ gemini" {
		t.Fatalf("response = %v, want the fake gemini answer (proves request actually went to gemini, not vsegpt)", body["response"])
	}
	if !strings.HasPrefix(gotAuth, "Bearer gk-test") {
		t.Fatalf("gemini fake server did not receive the gemini API key, got Authorization=%q", gotAuth)
	}
}

// TestAdminModeDetail_TestModel_ExplicitProviderOverridesMode: the request may
// explicitly name a provider other than the mode's default.
func TestAdminModeDetail_TestModel_ExplicitProviderOverridesMode(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)

	anthropicSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"тестовый ответ claude"}],"usage":{"input_tokens":10,"output_tokens":5}}`))
	}))
	defer anthropicSrv.Close()

	ts := NewTestServerWithHandler(t, env.Pool, Handler{AnthropicAPIKey: "ak-test", AnthropicAPIBaseURL: anthropicSrv.URL})
	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	// The mode defaults to vsegpt, but the request explicitly asks for anthropic.
	mode := f.CreateMode(TestModeOpts{})
	ts.LoginAs(f.CreateSession(admin.ID))

	status, body := httpJSON(t, ts, "POST", "/api/admin/modes/"+itoa(mode.ID)+"/test-model", map[string]any{
		"text":     "Привет",
		"provider": "anthropic",
		"model":    "claude-haiku-4-5",
	})
	if status != http.StatusOK {
		t.Fatalf("test-model: %d body=%v", status, body)
	}
	if body["provider"] != "anthropic" || body["response"] != "тестовый ответ claude" {
		t.Fatalf("explicit provider override did not take effect: %+v", body)
	}
}

// TestAdminModeDetail_TestModel_NotFound_404.
func TestAdminModeDetail_TestModel_NotFound_404(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)
	ts.LoginAs(f.CreateSession(f.CreateUser(TestUserOpts{Role: "admin"}).ID))

	status, body := httpJSON(t, ts, "POST", "/api/admin/modes/999999999/test-model", map[string]any{"text": "hi"})
	if status != http.StatusNotFound {
		t.Fatalf("test-model missing mode: %d body=%v", status, body)
	}
}

// TestAdminModeDetail_TestModel_WrongMethod_405.
func TestAdminModeDetail_TestModel_WrongMethod_405(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)
	mode := f.CreateMode(TestModeOpts{})
	ts.LoginAs(f.CreateSession(f.CreateUser(TestUserOpts{Role: "admin"}).ID))

	status, _ := httpJSON(t, ts, "GET", "/api/admin/modes/"+itoa(mode.ID)+"/test-model", nil)
	if status != http.StatusMethodNotAllowed {
		t.Fatalf("GET on /test-model: %d, want 405", status)
	}
}
