package httpapi

import (
	"context"
	"sync"
	"time"
)

// A generic (per-provider) queue for gemini/anthropic, the same algorithm
// as the legacy aiProviderQueue (ai_queue.go, which serves only vsegpt and is
// deliberately left alone: many unit/integration tests read its internal
// fields directly).
//
// Why it exists: incident 2026-07-14, a free-tier Gemini key returns
// RESOURCE_EXHAUSTED on the 16th request per minute (the limit is 15
// GenerateRequestsPerMinutePerProjectPerModel-FreeTier), and all 84 modes
// share one key. Without a queue, a burst from several concurrent
// users spills into the paid fallback (Claude/vsegpt) instead of
// just waiting a little.
//
// Settings live in system_settings, keyed with the provider prefix:
//
//	ai_queue_<provider>_enabled       - "1"/"0"
//	ai_queue_<provider>_interval_ms   - pause between requests
//	ai_queue_<provider>_timeout_ms    - how long to wait for a turn
//	ai_queue_<provider>_concurrency   - parallel requests
type namedQueueState struct {
	mu        chan struct{}
	active    int
	nextStart time.Time
}

var namedProviderQueues sync.Map // provider string -> *namedQueueState

func namedQueue(provider string) *namedQueueState {
	if v, ok := namedProviderQueues.Load(provider); ok {
		return v.(*namedQueueState)
	}
	q := &namedQueueState{mu: make(chan struct{}, 1)}
	q.mu <- struct{}{}
	actual, _ := namedProviderQueues.LoadOrStore(provider, q)
	return actual.(*namedQueueState)
}

// defaultProviderQueueSettings holds defaults for providers without a legacy queue
// (vsegpt uses its own in ai_queue.go). Off by default,
// the same reactive principle as vsegpt: enable it in /admin ->
// Orchestration when the provider really starts returning 429/RESOURCE_EXHAUSTED
// (visible in the daily aireport). gemini is not enabled by
// default even though the 15 req/min limit was confirmed by a load
// test on 2026-07-14: the shared package-level queue state would otherwise hit
// parallel integration tests that do not know about this queue
// (unlike the vsegpt queue, for which tests explicitly call
// resetAIProviderQueueForTest). The 4500ms interval is a ready safe
// preset (~13.3 req/min, headroom for jitter) if the admin enables the queue.
func defaultProviderQueueSettings(provider string) aiQueueSettings {
	switch provider {
	case aiProviderGemini:
		return aiQueueSettings{Enabled: false, Interval: 4500 * time.Millisecond, Timeout: 60 * time.Second, Concurrency: 1}
	default:
		return aiQueueSettings{Enabled: false, Interval: 1100 * time.Millisecond, Timeout: 60 * time.Second, Concurrency: 1}
	}
}

func (h Handler) namedQueueSettings(ctx context.Context, provider string) aiQueueSettings {
	def := defaultProviderQueueSettings(provider)
	defEnabled := 0
	if def.Enabled {
		defEnabled = 1
	}
	enabled := intSetting(ctx, h, "ai_queue_"+provider+"_enabled", defEnabled, 0, 1) == 1
	intervalMS := intSetting(ctx, h, "ai_queue_"+provider+"_interval_ms", int(def.Interval/time.Millisecond), 100, 60000)
	timeoutMS := intSetting(ctx, h, "ai_queue_"+provider+"_timeout_ms", int(def.Timeout/time.Millisecond), 1000, 600000)
	concurrency := intSetting(ctx, h, "ai_queue_"+provider+"_concurrency", def.Concurrency, 1, 10)
	return aiQueueSettings{
		Enabled:     enabled,
		Interval:    time.Duration(intervalMS) * time.Millisecond,
		Timeout:     time.Duration(timeoutMS) * time.Millisecond,
		Concurrency: concurrency,
	}
}

// queuedProviderDispatch wraps a provider call in the shared rate-limit
// queue (see the package comment). vsegpt keeps going through its own
// legacy queue inside queuedOpenAIChatWithOptions and does not take part here.
func (h Handler) queuedProviderDispatch(ctx context.Context, provider string, call func(context.Context) (string, int, int, float64, error)) (string, int, int, float64, error) {
	settings := h.namedQueueSettings(ctx, provider)
	if !settings.Enabled {
		return call(ctx)
	}
	queueCtx, cancel := context.WithTimeout(ctx, settings.Timeout)
	defer cancel()
	q := namedQueue(provider)
	if err := acquireNamedQueueSlot(queueCtx, q, settings); err != nil {
		return "", 0, 0, 0, err
	}
	defer releaseNamedQueueSlot(q)
	return call(queueCtx)
}

func acquireNamedQueueSlot(ctx context.Context, q *namedQueueState, settings aiQueueSettings) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-q.mu:
		}
		now := time.Now()
		if q.active < settings.Concurrency && !now.Before(q.nextStart) {
			q.active++
			q.nextStart = now.Add(settings.Interval)
			q.mu <- struct{}{}
			return nil
		}
		wait := 50 * time.Millisecond
		if now.Before(q.nextStart) {
			if untilNext := time.Until(q.nextStart); untilNext > 0 && untilNext < wait {
				wait = untilNext
			}
		}
		q.mu <- struct{}{}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
		}
	}
}

func releaseNamedQueueSlot(q *namedQueueState) {
	<-q.mu
	if q.active > 0 {
		q.active--
	}
	q.mu <- struct{}{}
}
