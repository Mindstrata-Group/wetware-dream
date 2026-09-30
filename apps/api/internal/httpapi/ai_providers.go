package httpapi

import (
	"context"
	"strings"
)

// Multi-provider AI routing.
//
// Each mode has ai_provider (vsegpt | gemini | anthropic) and
// thinking_mode (default | off | low | high). The chosen provider goes
// first, then the fallback chain in a fixed order
// anthropic -> gemini -> vsegpt (without duplicates): if the chosen one fails with
// a transient error (429/5xx/network) or is unavailable, the request goes to the next
// CONFIGURED and ENABLED provider with its default model.
//
// Enabling/disabling and default models are in system_settings:
//   ai_provider_<id>_enabled        - "1"/"0" (default "1")
//   ai_provider_<id>_default_model  - the link's model on fallback
// "Configured" = there is an API key in env (an empty key -> the provider silently drops out
// of the chain; this lets us merge code before the keys exist).

const (
	aiProviderVsegpt    = "vsegpt"
	aiProviderGemini    = "gemini"
	aiProviderAnthropic = "anthropic"

	defaultAnthropicFallbackModel = "claude-haiku-4-5"
	defaultGeminiFallbackModel    = "gemini-3.1-flash-lite"
)

// aiThinkingModes lists the allowed values of modes.thinking_mode.
var aiThinkingModes = map[string]bool{"default": true, "off": true, "low": true, "high": true}

// aiProviders lists the allowed values of modes.ai_provider.
var aiProviders = map[string]bool{aiProviderVsegpt: true, aiProviderGemini: true, aiProviderAnthropic: true}

func normalizeAIProvider(raw string) string {
	p := strings.ToLower(strings.TrimSpace(raw))
	if aiProviders[p] {
		return p
	}
	return aiProviderVsegpt
}

func normalizeThinkingMode(raw string) string {
	t := strings.ToLower(strings.TrimSpace(raw))
	if aiThinkingModes[t] {
		return t
	}
	return "default"
}

// aiCallSpec fully describes a mode-level AI call.
type aiCallSpec struct {
	Provider     string
	Model        string
	Temperature  float64
	ThinkingMode string
}

// aiChainLink is one link of the fallback chain.
//
// Provider is the gateway id (also the value of modes.ai_provider), Protocol is the
// conversation format. Before the registry these were the same thing, so dispatch switched
// on the name; now there can be any number of gateways with the openai protocol, and
// Protocol is what decides.
type aiChainLink struct {
	Provider     string
	Protocol     string
	Model        string
	ThinkingMode string
}

// providerAPIKeySetting returns the system_settings key name for the provider's API key.
func providerAPIKeySetting(provider string) string {
	return "ai_provider_" + provider + "_api_key"
}

// providerAPIKey: the key from the admin UI (system_settings) takes priority over env, so an edit
// in the UI takes effect at once, without recreating the container. An empty DB setting
// means env is used (compatibility with the current .env-based deploy).
func (h Handler) providerAPIKey(ctx context.Context, provider string) string {
	// The registry gateway's key beats everything: it is set exactly for this call
	// (see withGateway in ai_gateways.go), and any other source here would mean
	// going to a gateway other than the one the chain picked.
	if key := strings.TrimSpace(h.gatewayKeyOverride); key != "" {
		return key
	}
	if v, err := h.systemSetting(ctx, providerAPIKeySetting(provider)); err == nil {
		if v = strings.TrimSpace(v); v != "" {
			return v
		}
	}
	switch provider {
	case aiProviderVsegpt:
		return strings.TrimSpace(h.OpenAIAPIKey)
	case aiProviderGemini:
		return strings.TrimSpace(h.GeminiAPIKey)
	case aiProviderAnthropic:
		return strings.TrimSpace(h.AnthropicAPIKey)
	}
	return ""
}

// providerConfigured reports whether the provider has a key (DB or env).
func (h Handler) providerConfigured(ctx context.Context, provider string) bool {
	return h.providerAPIKey(ctx, provider) != ""
}

// mechanicProvider is the provider for service AI mechanics (orchestration,
// summarisation, attachment annotation), a setting separate from the per-mode
// ai_provider because these calls are not tied to a specific mode.
// Default vsegpt keeps already configured installations compatible.
func (h Handler) mechanicProvider(ctx context.Context, settingKey string) string {
	v, _ := h.systemSetting(ctx, settingKey)
	return normalizeAIProvider(v)
}

// providerEnabled: the toggle in system_settings (default: enabled).
func (h Handler) providerEnabled(ctx context.Context, provider string) bool {
	v, err := h.systemSetting(ctx, "ai_provider_"+provider+"_enabled")
	if err != nil || strings.TrimSpace(v) == "" {
		return true
	}
	return strings.TrimSpace(v) != "0"
}

// providerDefaultModel: the link's model when falling back to this provider.
func (h Handler) providerDefaultModel(ctx context.Context, provider string) string {
	if v, err := h.systemSetting(ctx, "ai_provider_"+provider+"_default_model"); err == nil && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	switch provider {
	case aiProviderAnthropic:
		return defaultAnthropicFallbackModel
	case aiProviderGemini:
		return defaultGeminiFallbackModel
	default:
		return defaultAIFallbackModel
	}
}

// buildProviderChain builds the chain of links for spec.
//
// The fallback order is no longer hardcoded: it comes from the registry priority
// (ai_gateways.go). A gateway without a key or disabled by its toggle drops out
// of the chain, exactly as a provider used to.
//
// The pure logic lives in buildProviderChainFrom for unit tests.
func (h Handler) buildProviderChain(ctx context.Context, spec aiCallSpec) []aiChainLink {
	chain, _ := h.buildProviderChainWithPolicy(ctx, spec)
	return chain
}

// buildProviderChainWithPolicy also applies the data-protection policy
// (data_protection.go) and reports whether it removed any gateway, so the
// caller can tell "blocked by law" from "nothing configured".
func (h Handler) buildProviderChainWithPolicy(ctx context.Context, spec aiCallSpec) ([]aiChainLink, bool) {
	available := []aiGateway{}
	for _, g := range h.activeGateways(ctx) {
		if !g.Enabled || h.gatewayAPIKey(ctx, g) == "" {
			continue
		}
		available = append(available, g)
	}
	available, blockedByPolicy := h.filterGatewaysByJurisdiction(ctx, available)
	return buildProviderChainFrom(spec, available), blockedByPolicy
}

// buildProviderChainFrom: the gateway chosen by the mode comes first (with its model and
// thinking mode), then the other available ones in priority order without
// duplicates. Fallback links use the gateway's default model and thinking "default".
//
// available must arrive already filtered and sorted: this
// function is about the order of links, not availability.
//
// If the mode points to a gateway that is not in the registry (archived, removed
// from the configuration), there is simply no first link and the request goes by
// priority. Previously an unknown name was silently turned into vsegpt, which lied to
// the admin: statistics showed vsegpt although the mode was configured otherwise.
func buildProviderChainFrom(spec aiCallSpec, available []aiGateway) []aiChainLink {
	primary := strings.ToLower(strings.TrimSpace(spec.Provider))
	chain := []aiChainLink{}
	for _, g := range available {
		if g.ID != primary {
			continue
		}
		chain = append(chain, aiChainLink{
			Provider:     g.ID,
			Protocol:     g.Protocol,
			Model:        spec.Model,
			ThinkingMode: normalizeThinkingMode(spec.ThinkingMode),
		})
		break
	}
	for _, g := range available {
		if g.ID == primary {
			continue
		}
		chain = append(chain, aiChainLink{
			Provider:     g.ID,
			Protocol:     g.Protocol,
			Model:        gatewayFallbackModel(g),
			ThinkingMode: "default",
		})
	}
	return chain
}

// dispatchAIChat routes the request to a specific provider. gemini/anthropic
// go through queuedProviderDispatch (ai_queue_provider.go), a per-provider queue
// enabled via system_settings ai_queue_<provider>_*; vsegpt
// uses a separate legacy queue (see its comment in ai_queue.go).
func (h Handler) dispatchAIChat(ctx context.Context, link aiChainLink, temperature float64, messages []map[string]string, xTitle string, options openAIChatOptions) (string, int, int, float64, error) {
	// Address and key come from the registry row if the gateway is there. If not (the link
	// was built from the built-in list), the old sources apply: env and
	// system_settings.
	if g, ok := h.gatewayByID(ctx, link.Provider); ok {
		h = h.withGateway(ctx, g)
	}
	// The queue is named by gateway id, not by protocol: limits are issued per account,
	// and two different accounts of the same protocol must not share one slot.
	switch link.Protocol {
	case gatewayProtocolGemini:
		return h.queuedProviderDispatch(ctx, link.Provider, func(ctx context.Context) (string, int, int, float64, error) {
			return h.doGeminiChat(ctx, link.Model, temperature, link.ThinkingMode, messages, options)
		})
	case gatewayProtocolAnthropic:
		return h.queuedProviderDispatch(ctx, link.Provider, func(ctx context.Context) (string, int, int, float64, error) {
			return h.doAnthropicChat(ctx, link.Model, temperature, link.ThinkingMode, messages, options)
		})
	default:
		return h.queuedOpenAIChatWithOptions(ctx, link.Model, temperature, messages, xTitle, options)
	}
}
