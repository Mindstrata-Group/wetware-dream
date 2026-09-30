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

func TestAIResilience_QueueTimeoutRollsBackQuota(t *testing.T) {
	// not t.Parallel(): the test exclusively owns the global aiProviderQueue
	env := testsupport.NewEnv(t)
	_, _ = env.Pool.Exec(context.Background(), `
		insert into system_settings (key, value) values
		('ai_queue_enabled', '1'),
		('ai_queue_interval_ms', '100'),
		('ai_queue_timeout_ms', '1000'),
		('ai_queue_concurrency', '1'),
		('ai_retry_attempts', '1')
		on conflict (key) do update set value=excluded.value`)

	resetAIProviderQueueForTest(t)
	t.Cleanup(func() { resetAIProviderQueueForTest(t) })
	occupiedSettings := aiQueueSettings{Enabled: true, Interval: 0, Timeout: time.Second, Concurrency: 1}
	if err := acquireAIProviderSlot(context.Background(), occupiedSettings); err != nil {
		t.Fatalf("occupy queue: %v", err)
	}
	defer releaseAIProviderSlot()

	var calls atomic.Int64
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"content": "should-not-be-called"}}},
			"usage":   map[string]any{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
		})
	}))
	t.Cleanup(fake.Close)

	ts := aiTestServer(t, env, fake.URL, "test-key-for-live-ai")
	user, _, dialog := setupUserForLiveAI(t, env, ts)

	status, body := httpJSON(t, ts, "POST", "/api/chat/send", map[string]any{
		"dialogId":     dialog.ID,
		"text":         "queue timeout must not spend quota",
		"responseMode": "live",
	})
	if status != http.StatusBadGateway {
		t.Fatalf("status=%d want 502 body=%v", status, body)
	}
	if code, _ := body["code"].(string); code != "live_ai_error" {
		t.Fatalf("code=%q want live_ai_error body=%v", code, body)
	}
	if got := calls.Load(); got != 0 {
		t.Fatalf("OpenAI calls=%d want 0 while request is blocked in queue", got)
	}

	var quotaCount int64
	_ = env.Pool.QueryRow(context.Background(),
		`select coalesce(count, 0) from daily_message_counts where user_id=$1 and date=current_date`,
		user.ID).Scan(&quotaCount)
	if quotaCount != 0 {
		t.Fatalf("quota count after queue timeout=%d want 0", quotaCount)
	}

	var messageCount int64
	if err := env.Pool.QueryRow(context.Background(),
		`select count(*) from dialogs_messages where dialog_id=$1`, dialog.ID).Scan(&messageCount); err != nil {
		t.Fatalf("count dialog messages: %v", err)
	}
	if messageCount != 1 {
		t.Fatalf("dialog messages after queue timeout=%d want only user message", messageCount)
	}
}
