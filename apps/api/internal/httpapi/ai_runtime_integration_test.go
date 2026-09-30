//go:build integration

package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"mindstrata-stage1/api/internal/testsupport"
)

// fakeOpenAI: an HTTP server posing as an OpenAI-compatible API.
// Returns the given response + checks that the Authorization header is present.
func fakeOpenAI(t *testing.T, status int, choiceContent string, usage map[string]any) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Authorization") == "" {
			t.Errorf("missing Authorization header")
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("missing Content-Type: %s", r.Header.Get("Content-Type"))
		}
		// X-Title is optional, but if present we check it is not empty.
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		resp := map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"role": "assistant", "content": choiceContent}},
			},
			"usage": usage,
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// aiTestHandler builds a handler with the mock OpenAI URL.
func aiTestHandler(t *testing.T, pool any, fakeURL string) Handler {
	t.Helper()
	return Handler{
		OpenAIAPIKey:  "test-api-key",
		OpenAIBaseURL: fakeURL,
		HTTPClient:    &http.Client{Timeout: 5 * time.Second},
	}
}

// TestAIRuntime_DoOpenAIChat_HappyPath:
// fake returns 200 + content + usage → the callee gets clean text + tokens + cost.
func TestAIRuntime_DoOpenAIChat_HappyPath(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	srv := fakeOpenAI(t, 200, "Hello from AI!", map[string]any{
		"prompt_tokens":     12,
		"completion_tokens": 8,
		"total_tokens":      20,
		"cost":              0.0001,
	})
	h := aiTestHandler(t, env.Pool, srv.URL)

	text, inTok, outTok, cost, err := h.doOpenAIChat(context.Background(),
		"openai/gpt-4o-mini", 0.7,
		[]map[string]string{{"role": "user", "content": "hi"}},
		"Mindstrata_test")
	if err != nil {
		t.Fatalf("doOpenAIChat: %v", err)
	}
	if text != "Hello from AI!" {
		t.Errorf("text: got %q want \"Hello from AI!\"", text)
	}
	if inTok != 12 || outTok != 8 {
		t.Errorf("tokens: got in=%d out=%d, want 12/8", inTok, outTok)
	}
	if cost != 0.0001 {
		t.Errorf("cost: got %v want 0.0001", cost)
	}
}

// TestAIRuntime_DoOpenAIChat_4xx_WrappedAsLiveAIError.
func TestAIRuntime_DoOpenAIChat_4xx_WrappedAsLiveAIError(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	srv := fakeOpenAI(t, 401, "", nil)
	h := aiTestHandler(t, env.Pool, srv.URL)

	_, _, _, _, err := h.doOpenAIChat(context.Background(),
		"openai/gpt-4o-mini", 0.7,
		[]map[string]string{{"role": "user", "content": "hi"}},
		"")
	if err == nil {
		t.Fatalf("expected error on 401, got nil")
	}
	if _, ok := liveAIDebugFromError(err); !ok {
		t.Errorf("error should be liveAIError, got %T: %v", err, err)
	}
}

// TestAIRuntime_DoAIChat_MissingAPIKey: empty OPENAI_API_KEY → error.
// (The EnableLiveAI flag was removed on 2026-05-29: the key is now the only gate.)
func TestAIRuntime_DoAIChat_NoAPIKey(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	_ = env
	h := Handler{
		HTTPClient: &http.Client{Timeout: 5 * time.Second},
		DB:         env.Pool,
		// OpenAIAPIKey is empty: must fail.
	}
	_, _, _, _, err := h.doAIChat(context.Background(),
		"openai/gpt-4o-mini", 0.7,
		[]map[string]string{{"role": "user", "content": "hi"}}, "")
	if err == nil {
		t.Fatalf("expected error when OPENAI_API_KEY empty, got nil")
	}
	if !strings.Contains(err.Error(), "vsegpt API key missing") {
		t.Errorf("error should mention missing vsegpt key: %v", err)
	}
}

// TestAIRuntime_DoAIChat_MissingAPIKey.
func TestAIRuntime_DoAIChat_MissingAPIKey(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	h := Handler{
		OpenAIAPIKey: "", // no key
		HTTPClient:   &http.Client{Timeout: 5 * time.Second},
		DB:           env.Pool,
	}
	_, _, _, _, err := h.doAIChat(context.Background(),
		"openai/gpt-4o-mini", 0.7,
		[]map[string]string{{"role": "user", "content": "hi"}}, "")
	if err == nil {
		t.Fatalf("expected error without API key")
	}
}

// TestCallLiveAI_BuildsHistoryAndCallsOpenAI: end-to-end through the mock.
func TestCallLiveAI_BuildsHistoryAndCallsOpenAI(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	srv := fakeOpenAI(t, 200, "Generated answer", map[string]any{
		"prompt_tokens": 50, "completion_tokens": 30, "total_tokens": 80,
	})
	h := Handler{
		OpenAIAPIKey:  "test-key",
		OpenAIBaseURL: srv.URL,
		HTTPClient:    &http.Client{Timeout: 5 * time.Second},
		DB:            env.Pool,
	}

	user := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{Prompt: "You are a test mode"})
	dialog := f.CreateDialog(user.ID, mode.ID)
	// Up front: a few messages in the history (callLiveAI will pull them in).
	_ = f.AppendMessage(dialog.ID, "user", "previous user msg")
	_ = f.AppendMessage(dialog.ID, "assistant", "previous assistant msg")
	_ = f.AppendMessage(dialog.ID, "summary", "skip-this-summary")

	modeRow := modeRow{ID: mode.ID, Name: mode.Name, Prompt: mode.Prompt,
		AIModel: mode.AIModel, Temperature: mode.Temperature}
	text, inTok, outTok, _, err := h.callLiveAI(context.Background(),
		user.ID, dialog.ID, modeRow, "current user message")
	if err != nil {
		t.Fatalf("callLiveAI: %v", err)
	}
	if text != "Generated answer" {
		t.Errorf("text: got %q", text)
	}
	if inTok+outTok == 0 {
		t.Errorf("tokens not accumulated")
	}
}

// TestCallLiveAISummary_SkipsSummaryMessagesInTranscript.
func TestCallLiveAISummary_SkipsSummaryMessagesInTranscript(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	requests := make(chan openAIChatRequest, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload openAIChatRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode request: %v", err)
		}
		requests <- payload
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"role": "assistant", "content": "Summary text"}},
			},
			"usage": map[string]any{"prompt_tokens": 100, "completion_tokens": 20},
		})
	}))
	t.Cleanup(srv.Close)
	h := Handler{
		OpenAIAPIKey:  "test-key",
		OpenAIBaseURL: srv.URL,
		HTTPClient:    &http.Client{Timeout: 5 * time.Second},
		DB:            env.Pool,
	}

	mode := modeRow{ID: 1, Name: "Coach", Prompt: "You are coach", AIModel: "openai/gpt-4o-mini", Temperature: 0.7}
	dialog := []ChatMessage{
		{Role: "user", Content: "msg-1"},
		{Role: "summary", Content: "OLD-SUMMARY-MUST-NOT-LEAK"},
		{Role: "system", Content: modeSwitchTraceContent("Mode A", "Mode B", true)},
		{Role: "assistant", Content: "msg-2"},
	}

	text, _, _, _, err := h.callLiveAISummary(context.Background(), 42, mode, dialog)
	if err != nil {
		t.Fatalf("callLiveAISummary: %v", err)
	}
	if text != "Summary text" {
		t.Errorf("got %q", text)
	}
	payload := <-requests
	if len(payload.Messages) < 2 {
		t.Fatalf("payload messages=%v", payload.Messages)
	}
	transcript := payload.Messages[len(payload.Messages)-1]["content"]
	if !strings.Contains(transcript, "msg-1") || !strings.Contains(transcript, "msg-2") {
		t.Fatalf("transcript missing dialog messages: %q", transcript)
	}
	if strings.Contains(transcript, "OLD-SUMMARY-MUST-NOT-LEAK") || strings.Contains(transcript, modeSwitchTracePrefix) {
		t.Fatalf("transcript leaked summary or switch trace: %q", transcript)
	}
}
