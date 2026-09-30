//go:build integration

package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"mindstrata-stage1/api/internal/testsupport"
)

func newAIResilienceTestHandler(t *testing.T, baseURL string, timeout time.Duration) Handler {
	t.Helper()
	env := testsupport.NewEnv(t)
	h := Handler{
		DB:            env.Pool,
		OpenAIAPIKey:  "test-key",
		OpenAIBaseURL: baseURL,
		HTTPClient:    &http.Client{Timeout: timeout},
	}
	upsertSetting(t, h, "ai_queue_enabled", "0")
	upsertSetting(t, h, "ai_retry_attempts", "2")
	upsertSetting(t, h, "ai_retry_initial_delay_ms", "100")
	upsertSetting(t, h, "ai_provider_vsegpt_default_model", "fallback-model")
	return h
}

func decodeOpenAIModelForTest(t *testing.T, r *http.Request) string {
	t.Helper()
	var req openAIChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		t.Fatalf("decode OpenAI-compatible request: %v", err)
	}
	return req.Model
}

func writeOpenAISuccessForTest(t *testing.T, w http.ResponseWriter, content string) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{
		"choices": []map[string]any{
			{"message": map[string]any{"content": content}},
		},
		"usage": map[string]any{
			"prompt_tokens":     2,
			"completion_tokens": 3,
			"total_tokens":      5,
			"cost":              0.001,
		},
	}); err != nil {
		t.Fatalf("write OpenAI success: %v", err)
	}
}

func TestAIResilient_TimeoutIsRetriedAndCanRecover(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_ = decodeOpenAIModelForTest(t, r)
		if calls.Load() == 1 {
			time.Sleep(80 * time.Millisecond)
			return
		}
		writeOpenAISuccessForTest(t, w, "recovered-after-timeout")
	}))
	t.Cleanup(fake.Close)

	h := newAIResilienceTestHandler(t, fake.URL, 25*time.Millisecond)
	content, _, _, _, err := h.doAIChatResilient(context.Background(), "primary-model", 0.2, []map[string]string{{"role": "user", "content": "hi"}}, "TIMEOUT_RETRY")
	if err != nil {
		t.Fatalf("doAIChatResilient timeout retry: %v", err)
	}
	if content != "recovered-after-timeout" {
		t.Fatalf("content=%q want recovered-after-timeout", content)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("calls=%d want 2", got)
	}
}

func TestAIResilient_PrimaryTimeoutExhaustedFallsBack(t *testing.T) {
	t.Parallel()

	var primaryCalls atomic.Int32
	var fallbackCalls atomic.Int32
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		model := decodeOpenAIModelForTest(t, r)
		if model == "primary-model" {
			primaryCalls.Add(1)
			time.Sleep(70 * time.Millisecond)
			return
		}
		if model == "fallback-model" {
			fallbackCalls.Add(1)
			writeOpenAISuccessForTest(t, w, "fallback-after-timeouts")
			return
		}
		t.Fatalf("unexpected model %q", model)
	}))
	t.Cleanup(fake.Close)

	h := newAIResilienceTestHandler(t, fake.URL, 20*time.Millisecond)
	content, _, _, _, err := h.doAIChatResilient(context.Background(), "primary-model", 0.2, []map[string]string{{"role": "user", "content": "hi"}}, "TIMEOUT_FALLBACK")
	if err != nil {
		t.Fatalf("doAIChatResilient timeout fallback: %v", err)
	}
	if content != "fallback-after-timeouts" {
		t.Fatalf("content=%q want fallback-after-timeouts", content)
	}
	if got := primaryCalls.Load(); got != 2 {
		t.Fatalf("primary calls=%d want 2", got)
	}
	if got := fallbackCalls.Load(); got != 1 {
		t.Fatalf("fallback calls=%d want 1", got)
	}
}

func TestAIResilient_Retry429RecoversWithoutFallback(t *testing.T) {
	t.Parallel()

	var primaryCalls atomic.Int32
	var fallbackCalls atomic.Int32
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		model := decodeOpenAIModelForTest(t, r)
		switch model {
		case "primary-model":
			if primaryCalls.Add(1) == 1 {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write([]byte(`{"error":"rate_limit_exceeded"}`))
				return
			}
			writeOpenAISuccessForTest(t, w, "primary-recovered")
		case "fallback-model":
			fallbackCalls.Add(1)
			writeOpenAISuccessForTest(t, w, "unexpected-fallback")
		default:
			t.Fatalf("unexpected model %q", model)
		}
	}))
	t.Cleanup(fake.Close)

	h := newAIResilienceTestHandler(t, fake.URL, 2*time.Second)
	content, _, _, _, err := h.doAIChatResilient(context.Background(), "primary-model", 0.2, []map[string]string{{"role": "user", "content": "hi"}}, "RETRY_429")
	if err != nil {
		t.Fatalf("doAIChatResilient 429 retry: %v", err)
	}
	if content != "primary-recovered" {
		t.Fatalf("content=%q want primary-recovered", content)
	}
	if got := primaryCalls.Load(); got != 2 {
		t.Fatalf("primary calls=%d want 2", got)
	}
	if got := fallbackCalls.Load(); got != 0 {
		t.Fatalf("fallback calls=%d want 0", got)
	}
}

func TestAIResilient_404FallsBackWithoutRetryingPrimary(t *testing.T) {
	t.Parallel()

	var primaryCalls atomic.Int32
	var fallbackCalls atomic.Int32
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		model := decodeOpenAIModelForTest(t, r)
		switch model {
		case "primary-model":
			primaryCalls.Add(1)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"model_not_found"}`))
		case "fallback-model":
			fallbackCalls.Add(1)
			writeOpenAISuccessForTest(t, w, "fallback-after-404")
		default:
			t.Fatalf("unexpected model %q", model)
		}
	}))
	t.Cleanup(fake.Close)

	h := newAIResilienceTestHandler(t, fake.URL, 2*time.Second)
	content, _, _, _, err := h.doAIChatResilient(context.Background(), "primary-model", 0.2, []map[string]string{{"role": "user", "content": "hi"}}, "FALLBACK_404")
	if err != nil {
		t.Fatalf("doAIChatResilient 404 fallback: %v", err)
	}
	if content != "fallback-after-404" {
		t.Fatalf("content=%q want fallback-after-404", content)
	}
	if got := primaryCalls.Load(); got != 1 {
		t.Fatalf("primary calls=%d want 1", got)
	}
	if got := fallbackCalls.Load(); got != 1 {
		t.Fatalf("fallback calls=%d want 1", got)
	}
}
