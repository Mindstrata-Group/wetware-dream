package httpapi

import (
	"context"
	"strings"
	"sync"
	"time"
)

// AI gateway registry: rows in ai_gateways instead of constants in code.
//
// Why. Before the registry the provider list was hardcoded in aiProviders, and the
// fallback order was the string anthropic -> gemini -> vsegpt. Adding another gateway
// took a Go change and a release. Exactly when a gateway is needed (the balance hit
// zero, the key got geo-banned, the relay went down), a release is too slow.
//
// What the registry does NOT give. A protocol is code. There are three, one per
// written adapter, and an arbitrary provider with its own response format cannot
// be added in the admin UI. What can be added is a new INSTANCE of a known protocol:
// another account, another relay, another base URL. In practice this covers almost
// everything, since most gateways speak an OpenAI-compatible protocol.
const (
	gatewayProtocolOpenAI    = "openai"
	gatewayProtocolAnthropic = "anthropic"
	gatewayProtocolGemini    = "gemini"
)

// gatewayProtocols lists the allowed values of ai_gateways.protocol. Keep it
// in one place with the migration's check constraint: if they diverge, the admin
// UI will offer a protocol the DB rejects.
var gatewayProtocols = map[string]bool{
	gatewayProtocolOpenAI:    true,
	gatewayProtocolAnthropic: true,
	gatewayProtocolGemini:    true,
}

// aiGateway is a registry row.
type aiGateway struct {
	ID           string
	Title        string
	Protocol     string
	BaseURL      string
	RelayURL     string
	UseRelay     bool
	APIKey       string
	DefaultModel string
	Priority     int
	Enabled      bool
}

// effectiveBaseURL is the address the request will actually go to.
//
// Both addresses are kept at once on purpose: geo-bans come and go, and
// switching between the direct path and the relay should cost one checkbox, not
// an env change with a container rebuild. The checkbox does nothing when
// relay_url is empty; otherwise turning on "via VPS" without an address would
// silently send the channel nowhere.
func (g aiGateway) effectiveBaseURL() string {
	if g.UseRelay {
		if relay := strings.TrimSpace(g.RelayURL); relay != "" {
			return relay
		}
	}
	return strings.TrimSpace(g.BaseURL)
}

// aiGatewaysCacheTTL: the registry is read on every chat message but changes
// once in weeks. Half a minute is the compromise: an admin edit shows up almost
// immediately (and the cache is cleared explicitly on save anyway), while the
// message flow does not hit an extra SELECT.
const aiGatewaysCacheTTL = 30 * time.Second

type aiGatewaysCache struct {
	sync.RWMutex
	expiresAt time.Time
	items     []aiGateway
}

func (c *aiGatewaysCache) get() ([]aiGateway, bool) {
	c.RLock()
	defer c.RUnlock()
	if c.items == nil || time.Now().After(c.expiresAt) {
		return nil, false
	}
	return c.items, true
}

func (c *aiGatewaysCache) set(items []aiGateway) {
	c.Lock()
	c.items = items
	c.expiresAt = time.Now().Add(aiGatewaysCacheTTL)
	c.Unlock()
}

func (c *aiGatewaysCache) clear() {
	c.Lock()
	c.items = nil
	c.expiresAt = time.Time{}
	c.Unlock()
}

// builtinGateways is the registry as it was before the table: three gateways in the order
// anthropic -> gemini -> vsegpt.
//
// It is not decorative but a working path for two real cases: unit tests
// with a nil DB, and a database where the migration has not been applied yet (migrations
// in this project are applied by hand, see AGENTS.md). In both cases the old behaviour
// is better than "no gateways" and a silent chat.
func builtinGateways() []aiGateway {
	return []aiGateway{
		{ID: aiProviderAnthropic, Title: "Claude (Anthropic)", Protocol: gatewayProtocolAnthropic, Priority: 10, Enabled: true},
		{ID: aiProviderGemini, Title: "Gemini (Google)", Protocol: gatewayProtocolGemini, Priority: 20, Enabled: true},
		{ID: aiProviderVsegpt, Title: "VseGPT (OpenAI-совместимый)", Protocol: gatewayProtocolOpenAI, Priority: 30, Enabled: true},
	}
}

// activeGateways returns non-archived gateways in priority order.
func (h Handler) activeGateways(ctx context.Context) []aiGateway {
	if h.c != nil {
		if cached, ok := h.c.aiGateways.get(); ok {
			return cached
		}
	}
	items := h.queryAIGateways(ctx)
	if h.c != nil {
		h.c.aiGateways.set(items)
	}
	return items
}

// queryAIGateways reads the registry from the DB.
//
// Any error (no DB, no table: a database before the migration) means falling back
// to the built-in list, not an empty answer. This is a deliberate silent fallback: it
// keeps the chat working instead of turning a missing migration into a denial of
// service. The cost is that the query error is not visible here; it is visible where
// the registry is managed, in the admin endpoint, which returns the error honestly.
func (h Handler) queryAIGateways(ctx context.Context) []aiGateway {
	if h.DB == nil {
		return builtinGateways()
	}
	rows, err := h.DB.Query(ctx, `
		select id, title, protocol, coalesce(base_url, ''), coalesce(relay_url, ''),
		       coalesce(use_relay, false), coalesce(api_key, ''),
		       coalesce(default_model, ''), priority, enabled
		from ai_gateways
		where archived_at is null
		order by priority, id`)
	if err != nil {
		return builtinGateways()
	}
	defer rows.Close()
	items := []aiGateway{}
	for rows.Next() {
		var g aiGateway
		if err := rows.Scan(&g.ID, &g.Title, &g.Protocol, &g.BaseURL, &g.RelayURL, &g.UseRelay, &g.APIKey, &g.DefaultModel, &g.Priority, &g.Enabled); err != nil {
			return builtinGateways()
		}
		items = append(items, g)
	}
	if rows.Err() != nil || len(items) == 0 {
		return builtinGateways()
	}
	return items
}

// gatewayByID looks up a gateway among the non-archived ones.
func (h Handler) gatewayByID(ctx context.Context, id string) (aiGateway, bool) {
	id = strings.ToLower(strings.TrimSpace(id))
	for _, g := range h.activeGateways(ctx) {
		if g.ID == id {
			return g, true
		}
	}
	return aiGateway{}, false
}

// gatewayAPIKey: the key from the registry row, and if it is empty, the legacy sources
// (system_settings, then env; vsegpt has no env any more: its key lives only in the DB).
// This way the migration does not copy secrets into a second table, and installations
// that are already configured keep working without changes.
func (h Handler) gatewayAPIKey(ctx context.Context, g aiGateway) string {
	if key := strings.TrimSpace(g.APIKey); key != "" {
		return key
	}
	return h.providerAPIKey(ctx, g.ID)
}

// gatewayConfigured reports whether a registry gateway has a working key. Unlike
// providerConfigured it also sees a key set in the ai_gateways row, and for
// vsegpt that is now the main place where the key can be at all.
func (h Handler) gatewayConfigured(ctx context.Context, id string) bool {
	g, ok := h.gatewayByID(ctx, id)
	return ok && h.gatewayAPIKey(ctx, g) != ""
}

// gatewayFallbackModel is the link's model when the gateway was chosen not by the
// mode but by the fallback. An empty value in the registry means "the protocol
// default", not "no model".
func gatewayFallbackModel(g aiGateway) string {
	if m := strings.TrimSpace(g.DefaultModel); m != "" {
		return m
	}
	switch g.Protocol {
	case gatewayProtocolAnthropic:
		return defaultAnthropicFallbackModel
	case gatewayProtocolGemini:
		return defaultGeminiFallbackModel
	default:
		return defaultAIFallbackModel
	}
}

// withGateway returns a copy of Handler whose provider address and key
// come from the registry row.
//
// Handler is a value, so the copy is cheap and does not touch the original
// handler (the caches in h.c are a pointer that intentionally stays shared). This
// approach was chosen so the provider adapters (doAnthropicChat, doGeminiChat, the
// openai path) keep their signatures: they already read the address and key from
// Handler fields, and swapping the fields is more honest than threading the
// gateway as a seventh argument through five layers.
func (h Handler) withGateway(ctx context.Context, g aiGateway) Handler {
	h.gatewayKeyOverride = h.gatewayAPIKey(ctx, g)
	base := g.effectiveBaseURL()
	switch g.Protocol {
	case gatewayProtocolAnthropic:
		if base != "" {
			h.AnthropicAPIBaseURL = base
		}
	case gatewayProtocolGemini:
		if base != "" {
			// An explicit gateway address wins over Vertex: the row with an address was
			// created precisely to go there. If the service account config is not cleared,
			// doGeminiChat goes to vertexBaseURL and the address from the admin UI is
			// silently ignored, the worst possible outcome.
			h.GeminiAPIBaseURL = base
			h.GeminiVertexSAJSON = ""
		}
	default:
		if base != "" {
			h.OpenAIBaseURL = base
		}
		// The openai path reads the key straight from the field, not via providerAPIKey.
		if h.gatewayKeyOverride != "" {
			h.OpenAIAPIKey = h.gatewayKeyOverride
		}
	}
	return h
}
