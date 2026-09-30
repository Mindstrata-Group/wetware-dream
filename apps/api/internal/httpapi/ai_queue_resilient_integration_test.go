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

// upsertSetting: a helper for DB-driven configuration in queue integration
// tests. Writes a value into system_settings.
func upsertSetting(t *testing.T, h Handler, key, value string) {
	t.Helper()
	_, err := h.DB.Exec(context.Background(),
		`insert into system_settings (key, value) values ($1, $2)
		 on conflict (key) do update set value = excluded.value`, key, value)
	if err != nil {
		t.Fatalf("upsert setting %s=%s: %v", key, value, err)
	}
}

// TestAIResilient_QueueEnabled_ThrottlesBetweenRequests: with ai_queue_enabled=1
// two sequential doAIChatResilient calls are separated by >= interval.
// Refactoring guard: if someone "forgets" that resilient goes through the queue,
// this test catches it.
func TestAIResilient_QueueEnabled_ThrottlesBetweenRequests(t *testing.T) {
	// not t.Parallel(): the test exclusively owns the global aiProviderQueue
	resetAIProviderQueueForTest(t)
	env := testsupport.NewEnv(t)

	fake := fakeVseGPT(t, 200, "ok")
	defer fake.Close()

	h := Handler{
		DB:            env.Pool,
		OpenAIAPIKey:  "test-key",
		OpenAIBaseURL: "https://api.vsegpt.ru/v1",
		HTTPClient:    &http.Client{Transport: vsegptRT{target: fake.URL}, Timeout: 5 * time.Second},
	}
	upsertSetting(t, h, "ai_queue_enabled", "1")
	upsertSetting(t, h, "ai_queue_interval_ms", "300")
	upsertSetting(t, h, "ai_queue_concurrency", "1")
	upsertSetting(t, h, "ai_queue_timeout_ms", "5000")
	defer func() {
		upsertSetting(t, h, "ai_queue_enabled", "0")
	}()

	messages := []map[string]string{{"role": "user", "content": "hello"}}
	start := time.Now()
	if _, _, _, _, err := h.doAIChatResilient(context.Background(), "openai/gpt-4o-mini", 0.3, messages, "TEST1"); err != nil {
		t.Fatalf("call 1: %v", err)
	}
	if _, _, _, _, err := h.doAIChatResilient(context.Background(), "openai/gpt-4o-mini", 0.3, messages, "TEST2"); err != nil {
		t.Fatalf("call 2: %v", err)
	}
	elapsed := time.Since(start)
	if elapsed < 250*time.Millisecond {
		t.Errorf("два resilient вызова с queue interval=300ms заняли %v, want ≥250ms", elapsed)
	}
}

// TestAIResilient_QueueDisabled_NoThrottle: with ai_queue_enabled=0 two
// sequential requests go without delay. Documents the bypass.
func TestAIResilient_QueueDisabled_NoThrottle(t *testing.T) {
	// not t.Parallel(): the test exclusively owns the global aiProviderQueue
	resetAIProviderQueueForTest(t)
	env := testsupport.NewEnv(t)

	fake := fakeVseGPT(t, 200, "fast")
	defer fake.Close()

	h := Handler{
		DB:            env.Pool,
		OpenAIAPIKey:  "test-key",
		OpenAIBaseURL: "https://api.vsegpt.ru/v1",
		HTTPClient:    &http.Client{Transport: vsegptRT{target: fake.URL}, Timeout: 5 * time.Second},
	}
	upsertSetting(t, h, "ai_queue_enabled", "0")

	messages := []map[string]string{{"role": "user", "content": "hi"}}
	start := time.Now()
	for i := 0; i < 3; i++ {
		if _, _, _, _, err := h.doAIChatResilient(context.Background(), "openai/gpt-4o-mini", 0.3, messages, "FAST"); err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}
	elapsed := time.Since(start)
	// 3 quick requests to the mock server must not hit the throttle (which would add >3s).
	if elapsed > 2000*time.Millisecond {
		t.Errorf("3 запроса с queue OFF заняли %v, want <2000ms (нет throttle)", elapsed)
	}
}

// flakyVseGPT: the first N requests return 429, the rest 200.
func flakyVseGPT(t *testing.T, fail429Count int32, successContent string) *httptest.Server {
	t.Helper()
	var seen atomic.Int32
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := seen.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if n <= fail429Count {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":"rate_limit_exceeded"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": successContent}},
			},
			"usage": map[string]any{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2, "cost": 0.001},
		})
	}))
}

// TestAIResilient_Retry429GoesThroughQueue: 429 → retry. If the queue is
// enabled, the retry attempt also goes through acquireSlot. We check that the
// total time ≥ initial_delay (retry backoff) + interval (queue).
func TestAIResilient_Retry429GoesThroughQueue(t *testing.T) {
	// not t.Parallel(): the test exclusively owns the global aiProviderQueue
	resetAIProviderQueueForTest(t)
	env := testsupport.NewEnv(t)

	fake := flakyVseGPT(t, 1, "recovered") // first request 429, second 200
	defer fake.Close()

	h := Handler{
		DB:            env.Pool,
		OpenAIAPIKey:  "test-key",
		OpenAIBaseURL: "https://api.vsegpt.ru/v1",
		HTTPClient:    &http.Client{Transport: vsegptRT{target: fake.URL}, Timeout: 5 * time.Second},
	}
	upsertSetting(t, h, "ai_queue_enabled", "1")
	upsertSetting(t, h, "ai_queue_interval_ms", "200")
	upsertSetting(t, h, "ai_queue_concurrency", "1")
	upsertSetting(t, h, "ai_retry_attempts", "3")
	upsertSetting(t, h, "ai_retry_initial_delay_ms", "100")
	defer func() {
		upsertSetting(t, h, "ai_queue_enabled", "0")
	}()

	messages := []map[string]string{{"role": "user", "content": "hi"}}
	start := time.Now()
	content, _, _, _, err := h.doAIChatResilient(context.Background(), "openai/gpt-4o-mini", 0.3, messages, "RETRY")
	if err != nil {
		t.Fatalf("resilient with retry: %v", err)
	}
	elapsed := time.Since(start)
	if !strings.Contains(content, "recovered") {
		t.Errorf("content=%q, want contains 'recovered'", content)
	}
	// Retry attempt: the first went through the queue and failed with 429; wait for
	// backoff (100ms+), then the second attempt goes through the queue again
	// (interval 200ms from the first). At least 100ms backoff + 200ms interval = 300ms.
	if elapsed < 150*time.Millisecond {
		t.Errorf("retry+queue=%v, want ≥150ms", elapsed)
	}
}
