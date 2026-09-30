package httpapi

import (
	"context"
	"time"
)

const (
	// defaultAIQueueEnabled = 0: the queue is OFF by default.
	// It is turned on in /admin -> Orchestration -> AI when VSEGPT starts
	// returning 429 (rate limit). With concurrency=1 + interval=1100ms
	// the queue serialises all AI requests and can slow the UX,
	// so we only enable it when things break without it.
	defaultAIQueueEnabled     = 0
	defaultAIQueueIntervalMS  = 1100
	defaultAIQueueTimeoutMS   = 60000
	defaultAIQueueConcurrency = 1
)

type aiQueueSettings struct {
	Enabled     bool
	Interval    time.Duration
	Timeout     time.Duration
	Concurrency int
}

var aiProviderQueue = struct {
	mu        chan struct{}
	active    int
	nextStart time.Time
}{mu: make(chan struct{}, 1)}

func init() {
	aiProviderQueue.mu <- struct{}{}
}

func (h Handler) aiQueueSettings(ctx context.Context) aiQueueSettings {
	enabled := intSetting(ctx, h, "ai_queue_enabled", defaultAIQueueEnabled, 0, 1) == 1
	intervalMS := intSetting(ctx, h, "ai_queue_interval_ms", defaultAIQueueIntervalMS, 100, 60000)
	timeoutMS := intSetting(ctx, h, "ai_queue_timeout_ms", defaultAIQueueTimeoutMS, 1000, 600000)
	concurrency := intSetting(ctx, h, "ai_queue_concurrency", defaultAIQueueConcurrency, 1, 10)
	return aiQueueSettings{
		Enabled:     enabled,
		Interval:    time.Duration(intervalMS) * time.Millisecond,
		Timeout:     time.Duration(timeoutMS) * time.Millisecond,
		Concurrency: concurrency,
	}
}

func (h Handler) queuedOpenAIChat(ctx context.Context, model string, temperature float64, messages []map[string]string, xTitle string) (string, int, int, float64, error) {
	return h.queuedOpenAIChatWithOptions(ctx, model, temperature, messages, xTitle, openAIChatOptions{})
}

func (h Handler) queuedOpenAIChatWithOptions(ctx context.Context, model string, temperature float64, messages []map[string]string, xTitle string, options openAIChatOptions) (string, int, int, float64, error) {
	settings := h.aiQueueSettings(ctx)
	if !settings.Enabled {
		return h.doOpenAIChatWithOptions(ctx, model, temperature, messages, xTitle, options)
	}
	queueCtx, cancel := context.WithTimeout(ctx, settings.Timeout)
	defer cancel()
	if err := acquireAIProviderSlot(queueCtx, settings); err != nil {
		return "", 0, 0, 0, err
	}
	defer releaseAIProviderSlot()
	return h.doOpenAIChatWithOptions(queueCtx, model, temperature, messages, xTitle, options)
}

func acquireAIProviderSlot(ctx context.Context, settings aiQueueSettings) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-aiProviderQueue.mu:
		}
		now := time.Now()
		if aiProviderQueue.active < settings.Concurrency && !now.Before(aiProviderQueue.nextStart) {
			aiProviderQueue.active++
			aiProviderQueue.nextStart = now.Add(settings.Interval)
			aiProviderQueue.mu <- struct{}{}
			return nil
		}
		wait := 50 * time.Millisecond
		if now.Before(aiProviderQueue.nextStart) {
			untilNext := time.Until(aiProviderQueue.nextStart)
			if untilNext > 0 && untilNext < wait {
				wait = untilNext
			}
		}
		aiProviderQueue.mu <- struct{}{}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
		}
	}
}

func releaseAIProviderSlot() {
	<-aiProviderQueue.mu
	if aiProviderQueue.active > 0 {
		aiProviderQueue.active--
	}
	aiProviderQueue.mu <- struct{}{}
}
