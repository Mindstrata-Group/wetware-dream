package httpapi

import (
	"context"
	"testing"
	"time"
)

func TestDefaultProviderQueueSettings_GeminiDisabledByDefaultButSafePreset(t *testing.T) {
	got := defaultProviderQueueSettings(aiProviderGemini)
	if got.Enabled {
		t.Fatalf("expected gemini queue disabled by default (reactive enablement, like vsegpt)")
	}
	if got.Concurrency != 1 {
		t.Fatalf("Concurrency = %d, want 1", got.Concurrency)
	}
	// The interval preset must be ready to turn on below the confirmed limit of 15/min.
	perMinute := float64(time.Minute) / float64(got.Interval)
	if perMinute >= 15 {
		t.Fatalf("interval %v allows %.1f req/min, want < 15 (confirmed free-tier limit)", got.Interval, perMinute)
	}
}

func TestDefaultProviderQueueSettings_AnthropicDisabledByDefault(t *testing.T) {
	got := defaultProviderQueueSettings(aiProviderAnthropic)
	if got.Enabled {
		t.Fatalf("expected anthropic queue disabled by default (no known tight limit yet)")
	}
}

func TestQueuedProviderDispatch_DisabledCallsThrough(t *testing.T) {
	h := Handler{}
	called := false
	content, in, out, cost, err := h.queuedProviderDispatch(context.Background(), aiProviderAnthropic, func(ctx context.Context) (string, int, int, float64, error) {
		called = true
		return "ok", 1, 2, 0.5, nil
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !called {
		t.Fatalf("expected wrapped call to run when queue disabled")
	}
	if content != "ok" || in != 1 || out != 2 || cost != 0.5 {
		t.Fatalf("unexpected result: %q %d %d %f", content, in, out, cost)
	}
}

func TestAcquireNamedQueueSlot_SerializesConcurrentCallers(t *testing.T) {
	q := &namedQueueState{mu: make(chan struct{}, 1)}
	q.mu <- struct{}{}
	settings := aiQueueSettings{Enabled: true, Interval: 0, Timeout: 2 * time.Second, Concurrency: 1}

	if err := acquireNamedQueueSlot(context.Background(), q, settings); err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	if q.active != 1 {
		t.Fatalf("active = %d, want 1", q.active)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if err := acquireNamedQueueSlot(ctx, q, settings); err == nil {
		t.Fatalf("expected second acquire to block/time out while slot is held")
	}

	releaseNamedQueueSlot(q)
	if q.active != 0 {
		t.Fatalf("active after release = %d, want 0", q.active)
	}

	if err := acquireNamedQueueSlot(context.Background(), q, settings); err != nil {
		t.Fatalf("acquire after release: %v", err)
	}
	releaseNamedQueueSlot(q)
}

func TestNamedQueue_PerProviderIsolation(t *testing.T) {
	a := namedQueue("test-provider-a")
	b := namedQueue("test-provider-b")
	if a == b {
		t.Fatalf("expected distinct queue instances per provider key")
	}
	again := namedQueue("test-provider-a")
	if a != again {
		t.Fatalf("expected same queue instance on repeated lookup for same provider")
	}
}
