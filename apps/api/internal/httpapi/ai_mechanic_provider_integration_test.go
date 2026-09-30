//go:build integration

package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"mindstrata-stage1/api/internal/testsupport"
)

// TestAdminAISettings_MechanicProviders_RoundTrip (Claims: GET/POST declare
// support for orchestrationProvider/summaryProvider/attachmentAnnotationProvider;
// we check that what was written really reads back).
func TestAdminAISettings_MechanicProviders_RoundTrip(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)
	ts.LoginAs(f.CreateSession(f.CreateUser(TestUserOpts{Role: "admin"}).ID))

	code, body := httpJSON(t, ts, "POST", "/api/admin/ai-settings", map[string]any{
		"orchestrationProvider":        "gemini",
		"summaryProvider":              "anthropic",
		"attachmentAnnotationProvider": "gemini",
	})
	if code != http.StatusOK {
		t.Fatalf("post: %d body=%v", code, body)
	}

	code, getBody := httpJSON(t, ts, "GET", "/api/admin/ai-settings", nil)
	if code != http.StatusOK {
		t.Fatalf("get: %d body=%v", code, getBody)
	}
	if getBody["orchestrationProvider"] != "gemini" {
		t.Fatalf("orchestrationProvider = %v, want gemini", getBody["orchestrationProvider"])
	}
	if getBody["summaryProvider"] != "anthropic" {
		t.Fatalf("summaryProvider = %v, want anthropic", getBody["summaryProvider"])
	}
	if getBody["attachmentAnnotationProvider"] != "gemini" {
		t.Fatalf("attachmentAnnotationProvider = %v, want gemini", getBody["attachmentAnnotationProvider"])
	}
}

// TestAdminAISettings_MechanicProviders_DefaultVsegpt: without configuration it is
// vsegpt (backward compatibility with already configured installations).
func TestAdminAISettings_MechanicProviders_DefaultVsegpt(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)
	ts.LoginAs(f.CreateSession(f.CreateUser(TestUserOpts{Role: "admin"}).ID))

	code, body := httpJSON(t, ts, "GET", "/api/admin/ai-settings", nil)
	if code != http.StatusOK {
		t.Fatalf("get: %d body=%v", code, body)
	}
	for _, key := range []string{"orchestrationProvider", "summaryProvider", "attachmentAnnotationProvider"} {
		if body[key] != "vsegpt" {
			t.Fatalf("%s = %v, want vsegpt (default)", key, body[key])
		}
	}
}

// TestChatOrchestration_UsesConfiguredGeminiProvider (World/Purpose: changing the
// orchestration provider really changes where the request goes, not just a value
// in settings. The Handler has NO vsegpt key at all: if the provider were not
// picked up and the code still went down the vsegpt path, the test would fail
// with a configuration error rather than pass silently.)
func TestChatOrchestration_UsesConfiguredGeminiProvider(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)

	user := f.CreateUser(TestUserOpts{})
	mode1 := f.CreateMode(TestModeOpts{Name: "Уточнить фразу"})
	mode2 := f.CreateMode(TestModeOpts{Name: "Переговоры"})
	if _, err := env.Pool.Exec(context.Background(), `update modes set orchestrator_check_interval = 1 where id = $1`, mode1.ID); err != nil {
		t.Fatalf("seed interval: %v", err)
	}
	f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: mode1.ID, DailyMessageLimit: 50})
	f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: mode2.ID, DailyMessageLimit: 50})
	dialog := f.CreateDialog(user.ID, mode1.ID)
	if _, err := env.Pool.Exec(context.Background(),
		`update users set current_mode=$2, current_dialog=$3, accepted_tos=true where id=$1`,
		user.ID, mode1.ID, dialog.ID); err != nil {
		t.Fatalf("seed current dialog: %v", err)
	}

	var geminiHit bool
	geminiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		geminiHit = true
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"modeId\":` + itoa(mode2.ID) + `,\"reason\":\"switch\"}"}}],"usage":{"prompt_tokens":10,"completion_tokens":5}}`))
	}))
	defer geminiSrv.Close()

	// Only the Gemini key is configured: vsegpt/anthropic are unavailable entirely.
	ts := NewTestServerWithHandler(t, env.Pool, Handler{GeminiAPIKey: "gk-test", GeminiAPIBaseURL: geminiSrv.URL, HTTPClient: &http.Client{Timeout: 5 * time.Second}})
	if _, err := env.Pool.Exec(context.Background(), `
		insert into system_settings (key, value, description, created_at, updated_at)
		values ('ai_orchestration_provider', 'gemini', 'test', now(), now())
		on conflict (key) do update set value=excluded.value`); err != nil {
		t.Fatalf("seed provider setting: %v", err)
	}
	ts.LoginAs(f.CreateSession(user.ID))

	status, body := httpJSON(t, ts, "POST", "/api/chat/send", map[string]any{
		"dialogId":         dialog.ID,
		"text":             "первое сообщение",
		"responseMode":     "live",
		"knowledgeModeIds": []int64{mode1.ID, mode2.ID},
	})
	if status != http.StatusOK {
		t.Fatalf("send: %d body=%v", status, body)
	}
	if !geminiHit {
		t.Fatalf("orchestration did not call the fake Gemini server at all")
	}
	if switched, _ := body["modeSwitched"].(bool); !switched {
		t.Fatalf("modeSwitched=false body=%v (orchestration via gemini should still switch modes)", body)
	}
}

// TestAnnotateChatAttachment_UsesConfiguredGeminiProvider: a large file
// (> DirectMaxBytes) must be annotated through the configured gemini provider,
// not through vsegpt (which is absent here: no key set).
func TestAnnotateChatAttachment_UsesConfiguredGeminiProvider(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)

	var geminiHit bool
	geminiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		geminiHit = true
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"сжатая аннотация от gemini"}}],"usage":{"prompt_tokens":50,"completion_tokens":5}}`))
	}))
	defer geminiSrv.Close()

	ts := NewTestServerWithHandler(t, env.Pool, Handler{GeminiAPIKey: "gk-test", GeminiAPIBaseURL: geminiSrv.URL, HTTPClient: &http.Client{Timeout: 5 * time.Second}})
	if _, err := env.Pool.Exec(context.Background(), `
		insert into system_settings (key, value, description, created_at, updated_at) values
		('chat_attachment_annotation_provider', 'gemini', 'test', now(), now()),
		('chat_attachment_annotation_model', 'gemini-3.1-flash-lite', 'test', now(), now()),
		('chat_attachment_direct_max_bytes', '10', 'test', now(), now())
		on conflict (key) do update set value=excluded.value`); err != nil {
		t.Fatalf("seed settings: %v", err)
	}

	user := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{})
	f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: mode.ID})
	ts.LoginAs(f.CreateSession(user.ID))

	att := uploadTestAttachment(t, ts, "big.txt", "text/plain", []byte("Это достаточно длинный текст файла, чтобы превысить лимит прямой отправки и запустить аннотацию через AI."))
	if att.AnnotationStatus != "annotated" {
		t.Fatalf("annotationStatus = %q, want annotated", att.AnnotationStatus)
	}
	if !geminiHit {
		t.Fatalf("annotation did not call the fake Gemini server at all")
	}
}
