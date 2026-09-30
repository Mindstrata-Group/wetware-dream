package httpapi

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"
)

// An AI provider returns 404 if the model was removed (like gemini-2.0-flash-lite-001
// in incident 2026-06-01), and 429 on rate limit (vsegpt.ru allows 1 req/sec).
// This wrapper does:
// 1. Retry with exponential backoff on 429
// 2. Fallback to a backup model on 404
// 3. Fail loud if both fail
//
// Config in system_settings (the real value is set in the admin UI, NOT in
// code; the constant below is only a last-resort safety net for an empty DB):
//   ai_provider_vsegpt_default_model - vsegpt backup model (admin ->
//     Orchestration -> AI providers -> vsegpt; the only place for this
//     setting; there used to be a separate ai_fallback_model field
//     duplicating it, removed on 2026-07-15)
//   ai_retry_attempts            - max attempts (default 3)
//   ai_retry_initial_delay_ms    - initial delay between attempts (default 1500ms)

const (
	// defaultAIFallbackModel is the last-resort model of the vsegpt link, used only if
	// ai_provider_vsegpt_default_model is not set in system_settings (see
	// providerDefaultModel in ai_providers.go). Keep a LIVE model (not a
	// deprecated one): incident 2026-07-10 showed that a dead model in the
	// fallback turns any transient failure of the primary into a hard 404 for the
	// user. Also used as a general safe default elsewhere
	// (admin_export_collect.go, chat_attachments.go), not only for vsegpt.
	defaultAIFallbackModel            = "google/gemini-3.5-flash"
	defaultAIOrchestrationModel       = "google/gemini-2.5-flash-lite"
	defaultAIOrchestrationTemperature = 0.0
	defaultAIRetryAttempts            = 3
	defaultAIRetryInitialMS           = 1500
	defaultAIRetryMaxInitMS           = 10000 // guard against a human typo
)

func (h Handler) aiRetryAttempts(ctx context.Context) int {
	return intSetting(ctx, h, "ai_retry_attempts", defaultAIRetryAttempts, 1, 10)
}

func (h Handler) aiRetryInitialDelay(ctx context.Context) time.Duration {
	ms := intSetting(ctx, h, "ai_retry_initial_delay_ms", defaultAIRetryInitialMS, 100, defaultAIRetryMaxInitMS)
	return time.Duration(ms) * time.Millisecond
}

func intSetting(ctx context.Context, h Handler, key string, def, min, max int) int {
	v, err := h.systemSetting(ctx, key)
	if err != nil || strings.TrimSpace(v) == "" {
		return def
	}
	var parsed int
	if _, err := fmt.Sscanf(v, "%d", &parsed); err != nil {
		return def
	}
	if parsed < min {
		return min
	}
	if parsed > max {
		return max
	}
	return parsed
}

// liveAIErrorStatus returns the HTTP status code from liveAIError (or 0).
func liveAIErrorStatus(err error) int {
	if err == nil {
		return 0
	}
	var lae *liveAIError
	if !errors.As(err, &lae) {
		return 0
	}
	return lae.Debug.Status
}

// doAIChatResilient wraps doAIChat: retry + fallback. The legacy entry point
// (tests, old code) always uses vsegpt.
// Returns the same as doAIChat, plus an error only if ALL attempts are exhausted.
func (h Handler) doAIChatResilient(ctx context.Context, model string, temperature float64, messages []map[string]string, xTitle string) (string, int, int, float64, error) {
	return h.doAIChatResilientWithOptions(ctx, model, temperature, messages, xTitle, openAIChatOptions{})
}

func (h Handler) doAIChatResilientWithOptions(ctx context.Context, model string, temperature float64, messages []map[string]string, xTitle string, options openAIChatOptions) (string, int, int, float64, error) {
	spec := aiCallSpec{Provider: aiProviderVsegpt, Model: model, Temperature: temperature, ThinkingMode: "default"}
	return h.doAIChatResilientSpec(ctx, spec, messages, xTitle, options)
}

// doMechanicAIChat serves service AI mechanics (orchestration, summarisation,
// attachment annotation): the provider comes from a separate system_settings entry
// (providerSettingKey), not from the per-mode ai_provider, because these calls are not
// tied to a specific mode. The model is interpreted as for modes: for vsegpt
// it is the legacy "provider/model", for gemini/anthropic the name is used as is.
func (h Handler) doMechanicAIChat(ctx context.Context, providerSettingKey, model string, temperature float64, messages []map[string]string, xTitle string, options openAIChatOptions) (string, int, int, float64, error) {
	provider := h.mechanicProvider(ctx, providerSettingKey)
	spec := aiCallSpec{Provider: provider, Model: model, Temperature: temperature, ThinkingMode: "default"}
	return h.doAIChatResilientSpec(ctx, spec, messages, xTitle, options)
}

// doAIChatResilientSpec is the multi-provider version: retry inside a link +
// fallback between providers (see ai_providers.go). Inside the vsegpt link,
// providerDefaultModel(vsegpt) is also tried as the link's second model
// (if it differs from the primary) before moving on to the next
// provider.
func (h Handler) doAIChatResilientSpec(ctx context.Context, spec aiCallSpec, messages []map[string]string, xTitle string, options openAIChatOptions) (string, int, int, float64, error) {
	if spec.Provider == aiProviderVsegpt || strings.TrimSpace(spec.Provider) == "" {
		spec.Model = h.effectiveLiveAIModel(spec.Model)
	}
	chain, blockedByPolicy := h.buildProviderChainWithPolicy(ctx, spec)
	if len(chain) == 0 {
		if blockedByPolicy {
			return "", 0, 0, 0, errCrossBorderConsentRequired
		}
		return "", 0, 0, 0, errors.New("live AI is not configured: no provider has an API key")
	}
	// Resolve the token ceiling once (per-mode -> system_settings -> const) and
	// put it in options; dispatchAIChat hands it to every provider in the chain.
	options.MaxTokens = h.resolveMaxTokens(ctx, options.MaxTokens)

	maxAttempts := h.aiRetryAttempts(ctx)
	initialDelay := h.aiRetryInitialDelay(ctx)

	var lastErr error
	var lastStatus int
	var lastModel string
linkLoop:
	for _, link := range chain {
		models := []string{link.Model}
		if link.Provider == aiProviderVsegpt {
			if fallback := h.providerDefaultModel(ctx, aiProviderVsegpt); fallback != "" && !strings.EqualFold(fallback, link.Model) {
				models = append(models, fallback)
			}
		}
		for _, m := range models {
			delay := initialDelay
			for attempt := 1; attempt <= maxAttempts; attempt++ {
				// Protocol must move into the attempt: dispatchAIChat picks the adapter
				// by it, and a link without it would silently go down the
				// openai branch instead of Claude/Gemini.
				attemptLink := aiChainLink{Provider: link.Provider, Protocol: link.Protocol, Model: m, ThinkingMode: link.ThinkingMode}
				content, in, out, cost, err := h.dispatchAIChat(ctx, attemptLink, spec.Temperature, messages, xTitle, options)
				if err == nil {
					h.recordAIProviderEvent(link.Provider, m, "success", 0)
					return content, in, out, cost, nil
				}
				lastErr = err
				lastModel = link.Provider + "/" + m
				status := liveAIErrorStatus(err)
				lastStatus = status
				h.recordAIProviderEvent(link.Provider, m, "attempt_failed", status)

				// 404 (model not found) -> no retry, move to the next model/link.
				if status == 404 {
					break
				}
				// 0 means transport/timeout before an HTTP response. Transient like 429/5xx:
				// retry with backoff, then the next model/provider.
				if status == 0 || status == 429 || (status >= 500 && status < 600) {
					if attempt < maxAttempts {
						select {
						case <-ctx.Done():
							return "", 0, 0, 0, ctx.Err()
						case <-time.After(delay):
						}
						delay *= 2
						continue
					}
					break
				}
				// Other 4xx (auth/validation, e.g. an expired key or a request format
				// that is wrong for THIS particular provider): no retry,
				// and no second model of THE SAME link (this keeps the exact
				// historical behaviour: chat_orchestration.go catches
				// such an error and retries the request itself without response_format).
				// But the next PROVIDER in the chain is a different service and
				// may succeed (e.g. the vsegpt key expired while Gemini is alive).
				continue linkLoop
			}
		}
	}
	// Prometheus metric for the Kuma alert via /health/ai-errors
	recordAIError(lastModel, lastStatus)
	h.recordAIProviderEvent(exhaustedEventProvider(lastModel), exhaustedEventModel(lastModel), "exhausted", lastStatus)
	return "", 0, 0, 0, lastErr
}

// exhaustedEventProvider/exhaustedEventModel parse lastModel of the form
// "provider/model" (see lastModel = link.Provider + "/" + m above).
func exhaustedEventProvider(lastModel string) string {
	if provider, _, ok := strings.Cut(lastModel, "/"); ok {
		return provider
	}
	return "unknown"
}

func exhaustedEventModel(lastModel string) string {
	if _, model, ok := strings.Cut(lastModel, "/"); ok {
		return model
	}
	return lastModel
}

// recordAIProviderEvent is telemetry for the daily report (aireport):
// how many messages each provider handled, how many attempts failed,
// how many times the whole fallback chain was exhausted. Best effort: a write
// error must not break the user's response, we only log it.
func (h Handler) recordAIProviderEvent(provider, model, outcome string, httpStatus int) {
	if h.DB == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var statusArg any
	if httpStatus > 0 {
		statusArg = httpStatus
	}
	if _, err := h.DB.Exec(ctx, `
		insert into ai_provider_events (provider, model, outcome, http_status)
		values ($1, $2, $3, $4)
	`, provider, model, outcome, statusArg); err != nil {
		log.Printf("recordAIProviderEvent: insert failed provider=%s model=%s outcome=%s: %v", provider, model, outcome, err)
	}
}
