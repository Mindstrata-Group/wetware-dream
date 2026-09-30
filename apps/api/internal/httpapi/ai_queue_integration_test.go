//go:build integration

package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"mindstrata-stage1/api/internal/testsupport"
)

func TestAIQueue_DisabledBypassesOccupiedQueue(t *testing.T) {
	// not t.Parallel(): the test exclusively owns the global aiProviderQueue
	env := testsupport.NewEnv(t)
	_, _ = env.Pool.Exec(context.Background(), `
		insert into system_settings (key, value) values
		('ai_queue_enabled', '0'),
		('ai_queue_timeout_ms', '1000')
		on conflict (key) do update set value=excluded.value`)

	resetAIProviderQueueForTest(t)
	occupiedSettings := aiQueueSettings{Enabled: true, Interval: 0, Timeout: time.Second, Concurrency: 1}
	if err := acquireAIProviderSlot(context.Background(), occupiedSettings); err != nil {
		t.Fatalf("occupy queue: %v", err)
	}
	defer releaseAIProviderSlot()

	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"content": "queue-disabled-ok"}}},
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
	ctx, cancel := context.WithTimeout(context.Background(), 2000*time.Millisecond)
	defer cancel()
	content, _, _, _, err := h.queuedOpenAIChat(ctx, "openai/gpt-4o-mini", 0.2, []map[string]string{{"role": "user", "content": "hi"}}, "queue-disabled-test")
	if err != nil {
		t.Fatalf("queuedOpenAIChat with disabled queue err=%v", err)
	}
	if content != "queue-disabled-ok" {
		t.Fatalf("content=%q want queue-disabled-ok", content)
	}
}
