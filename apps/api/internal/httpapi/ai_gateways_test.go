package httpapi

import (
	"context"
	"testing"
)

// TestAIGatewayAuditActions_AllClassified: the audit classification guard looks
// for a string literal right in the writeAdminAudit call, while gateway actions
// come from a map, so it cannot see them. This check closes exactly that gap:
// add an operation, forget to classify it, and it fails here.
func TestAIGatewayAuditActions_AllClassified(t *testing.T) {
	if len(aiGatewayAuditActions) == 0 {
		t.Fatal("карта действий пуста — сторож ничего не проверяет")
	}
	for request, action := range aiGatewayAuditActions {
		class, ok := adminAuditActionClassifications[action]
		if !ok {
			t.Errorf("действие %q (запрос %q) не классифицировано в admin_audit_policy.go", action, request)
			continue
		}
		if class.Section == "" {
			t.Errorf("действие %q классифицировано с пустой секцией", action)
		}
	}
}

// TestQueryAIGateways_FallsBackToBuiltinWithoutDB: without a DB (unit tests, and
// on prod a database before the migration is applied) the registry must return
// the three built-in gateways, not nothing. Nothing would mean "the chain is
// empty" and a silent chat.
func TestQueryAIGateways_FallsBackToBuiltinWithoutDB(t *testing.T) {
	h := Handler{}
	got := h.queryAIGateways(context.Background())
	if len(got) != 3 {
		t.Fatalf("queryAIGateways без DB вернул %d шлюзов, want 3: %+v", len(got), got)
	}
	wantOrder := []string{aiProviderAnthropic, aiProviderGemini, aiProviderVsegpt}
	for i, want := range wantOrder {
		if got[i].ID != want {
			t.Errorf("шлюз[%d] = %q, want %q (порядок фолбека до реестра)", i, got[i].ID, want)
		}
		if !got[i].Enabled {
			t.Errorf("встроенный шлюз %q пришёл выключенным", got[i].ID)
		}
	}
}

// TestBuiltinGateways_ProtocolsAreKnown: a built-in gateway's protocol must be
// in the same list as the migration's check constraint. If they diverge,
// dispatch goes down the openai branch for Claude, and users notice it, not
// tests.
func TestBuiltinGateways_ProtocolsAreKnown(t *testing.T) {
	for _, g := range builtinGateways() {
		if !gatewayProtocols[g.Protocol] {
			t.Errorf("шлюз %q имеет неизвестный протокол %q", g.ID, g.Protocol)
		}
	}
}

// TestBuildProviderChainFrom_CustomGatewayTakesPriorityPlace: a gateway added
// by hand takes its place in the chain by its priority and carries its own
// protocol: that is the whole point.
func TestBuildProviderChainFrom_CustomGatewayTakesPriorityPlace(t *testing.T) {
	relay := aiGateway{
		ID:           "relay-fi",
		Title:        "Зарубежный релей",
		Protocol:     gatewayProtocolOpenAI,
		BaseURL:      "https://relay.example/v1",
		DefaultModel: "openai/gpt-5",
		Priority:     5,
		Enabled:      true,
	}
	available := append([]aiGateway{relay}, testGateways(testChainDefaults)...)

	chain := buildProviderChainFrom(aiCallSpec{Provider: aiProviderAnthropic, Model: "claude-sonnet-5"}, available)
	if len(chain) != 4 {
		t.Fatalf("длина цепочки = %d, want 4: %+v", len(chain), chain)
	}
	if chain[0].Provider != aiProviderAnthropic {
		t.Errorf("первым должен идти выбранный режимом шлюз, got %q", chain[0].Provider)
	}
	// relay with priority 5 goes before all built-ins, but after the selected one.
	if chain[1].Provider != "relay-fi" {
		t.Errorf("chain[1] = %q, want relay-fi (priority 5 — раньше встроенных)", chain[1].Provider)
	}
	if chain[1].Protocol != gatewayProtocolOpenAI {
		t.Errorf("протокол relay-fi = %q, want %q", chain[1].Protocol, gatewayProtocolOpenAI)
	}
	if chain[1].Model != "openai/gpt-5" {
		t.Errorf("фолбек-звено должно идти с дефолтной моделью шлюза, got %q", chain[1].Model)
	}
}

// TestBuildProviderChainFrom_UnknownPrimaryIsNotCoercedToVsegpt: if a mode
// points at a gateway that is not in the registry (archived), the chain simply
// follows priority. The old behaviour coerced an unknown name to vsegpt, and the
// statistics showed vsegpt where the mode was configured otherwise.
func TestBuildProviderChainFrom_UnknownPrimaryIsNotCoercedToVsegpt(t *testing.T) {
	chain := buildProviderChainFrom(aiCallSpec{Provider: "archived-gw", Model: "whatever"}, testGateways(testChainDefaults))
	if len(chain) != 3 {
		t.Fatalf("длина цепочки = %d, want 3: %+v", len(chain), chain)
	}
	if chain[0].Provider != aiProviderAnthropic {
		t.Errorf("chain[0] = %q, want anthropic (первый по приоритету)", chain[0].Provider)
	}
	for _, l := range chain {
		if l.Model == "whatever" {
			t.Errorf("модель несуществующего шлюза не должна попадать в цепочку: %+v", chain)
		}
	}
}

// TestGatewayEffectiveBaseURL_RelayToggle: the "via VPS" checkbox switches the
// address but does NOT erase either of the two. The point is rollback: geo-bans
// come and go, and switching back must not cost re-entering the secret relay
// path.
func TestGatewayEffectiveBaseURL_RelayToggle(t *testing.T) {
	direct := "https://generativelanguage.googleapis.com/v1beta/openai"
	relay := "https://vps.example/секрет/v1beta/openai"

	cases := []struct {
		name string
		in   aiGateway
		want string
	}{
		{"галочка выключена — прямой адрес", aiGateway{BaseURL: direct, RelayURL: relay, UseRelay: false}, direct},
		{"галочка включена — через релей", aiGateway{BaseURL: direct, RelayURL: relay, UseRelay: true}, relay},
		{
			// A checked box with an empty relay must not send the channel
			// nowhere: that is a silent failure the user would notice, not the
			// admin.
			"галочка включена, релей не задан — остаётся прямой",
			aiGateway{BaseURL: direct, RelayURL: "   ", UseRelay: true},
			direct,
		},
		{"оба пусты — пусто, дальше решает протокол", aiGateway{UseRelay: true}, ""},
	}
	for _, c := range cases {
		if got := c.in.effectiveBaseURL(); got != c.want {
			t.Errorf("%s: effectiveBaseURL = %q, want %q", c.name, got, c.want)
		}
	}
}

// TestWithGateway_RelayToggleReachesProvider: the checkbox must reach the
// actual call, not just the admin screen.
func TestWithGateway_RelayToggleReachesProvider(t *testing.T) {
	ctx := context.Background()
	base := Handler{GeminiAPIBaseURL: "https://generativelanguage.googleapis.com/v1beta/openai"}
	g := aiGateway{
		ID: aiProviderGemini, Protocol: gatewayProtocolGemini,
		BaseURL:  "https://generativelanguage.googleapis.com/v1beta/openai",
		RelayURL: "https://vps.example/secret/v1beta/openai",
		UseRelay: true,
	}
	if got := base.withGateway(ctx, g).GeminiAPIBaseURL; got != g.RelayURL {
		t.Errorf("с включённой галочкой запрос должен уходить на релей, а уходит на %q", got)
	}
	g.UseRelay = false
	if got := base.withGateway(ctx, g).GeminiAPIBaseURL; got != g.BaseURL {
		t.Errorf("с выключенной галочкой запрос должен идти напрямую, а идёт на %q", got)
	}
}

// TestGatewayFallbackModel: empty in the registry means "protocol default", not
// "no model": the provider rejects a link without a model.
func TestGatewayFallbackModel(t *testing.T) {
	cases := []struct {
		name string
		in   aiGateway
		want string
	}{
		{"своя модель сильнее дефолта", aiGateway{Protocol: gatewayProtocolAnthropic, DefaultModel: "claude-opus-5"}, "claude-opus-5"},
		{"пусто у anthropic", aiGateway{Protocol: gatewayProtocolAnthropic}, defaultAnthropicFallbackModel},
		{"пусто у gemini", aiGateway{Protocol: gatewayProtocolGemini}, defaultGeminiFallbackModel},
		{"пусто у openai", aiGateway{Protocol: gatewayProtocolOpenAI}, defaultAIFallbackModel},
		{"пробелы считаются пустотой", aiGateway{Protocol: gatewayProtocolGemini, DefaultModel: "   "}, defaultGeminiFallbackModel},
	}
	for _, c := range cases {
		if got := gatewayFallbackModel(c.in); got != c.want {
			t.Errorf("%s: gatewayFallbackModel = %q, want %q", c.name, got, c.want)
		}
	}
}

// TestGatewayAPIKey_RowWinsThenLegacy: the registry row key beats env, and an
// empty key in the row means "take it as before"; otherwise a migration that
// did not copy secrets would cut off every already configured installation.
func TestGatewayAPIKey_RowWinsThenLegacy(t *testing.T) {
	h := Handler{AnthropicAPIKey: "env-key"}
	ctx := context.Background()

	withRow := aiGateway{ID: aiProviderAnthropic, Protocol: gatewayProtocolAnthropic, APIKey: "row-key"}
	if got := h.gatewayAPIKey(ctx, withRow); got != "row-key" {
		t.Errorf("ключ из строки реестра = %q, want row-key", got)
	}

	emptyRow := aiGateway{ID: aiProviderAnthropic, Protocol: gatewayProtocolAnthropic}
	if got := h.gatewayAPIKey(ctx, emptyRow); got != "env-key" {
		t.Errorf("пустой ключ в строке должен падать в env, got %q", got)
	}
}

// TestWithGateway_OverridesAddressAndKey: the Handler copy goes to the
// gateway's address with its key, while the original handler stays untouched.
func TestWithGateway_OverridesAddressAndKey(t *testing.T) {
	ctx := context.Background()
	base := Handler{
		AnthropicAPIBaseURL: "https://api.anthropic.com",
		AnthropicAPIKey:     "env-anthropic",
		OpenAIBaseURL:       "https://api.vsegpt.ru/v1",
		OpenAIAPIKey:        "env-vsegpt",
	}

	claude := base.withGateway(ctx, aiGateway{
		ID: "claude-2", Protocol: gatewayProtocolAnthropic,
		BaseURL: "https://relay.example/anthropic", APIKey: "second-account",
	})
	if claude.AnthropicAPIBaseURL != "https://relay.example/anthropic" {
		t.Errorf("адрес шлюза не применился: %q", claude.AnthropicAPIBaseURL)
	}
	if got := claude.providerAPIKey(ctx, aiProviderAnthropic); got != "second-account" {
		t.Errorf("ключ шлюза не перебил env: %q", got)
	}
	if base.AnthropicAPIBaseURL != "https://api.anthropic.com" || base.gatewayKeyOverride != "" {
		t.Errorf("исходный Handler изменился: %+v", base)
	}

	openai := base.withGateway(ctx, aiGateway{
		ID: "relay-fi", Protocol: gatewayProtocolOpenAI,
		BaseURL: "https://relay.example/v1", APIKey: "relay-key",
	})
	// the openai path reads the key straight from the field, so we check the field.
	if openai.OpenAIAPIKey != "relay-key" || openai.OpenAIBaseURL != "https://relay.example/v1" {
		t.Errorf("openai-шлюз не применился: key=%q url=%q", openai.OpenAIAPIKey, openai.OpenAIBaseURL)
	}
}

// TestWithGateway_ExplicitGeminiAddressBeatsVertex: if a gemini gateway has an
// address, go there. Without switching off the Vertex config, doGeminiChat would
// go to vertexBaseURL and the admin's address would be silently ignored.
func TestWithGateway_ExplicitGeminiAddressBeatsVertex(t *testing.T) {
	base := Handler{
		GeminiAPIBaseURL:   "https://generativelanguage.googleapis.com/v1beta/openai",
		GeminiVertexSAJSON: `{"client_email":"x","private_key":"y"}`,
	}
	g := base.withGateway(context.Background(), aiGateway{
		ID: "gemini-direct", Protocol: gatewayProtocolGemini, BaseURL: "https://relay.example/gemini",
	})
	if g.GeminiAPIBaseURL != "https://relay.example/gemini" {
		t.Errorf("адрес gemini-шлюза не применился: %q", g.GeminiAPIBaseURL)
	}
	if g.GeminiVertexSAJSON != "" {
		t.Error("конфиг Vertex должен гаситься явным адресом шлюза, иначе адрес молча игнорируется")
	}
	if base.GeminiVertexSAJSON == "" {
		t.Error("исходный Handler потерял конфиг Vertex — копия обязана быть изолированной")
	}
}

// TestWithGateway_EmptyRowKeepsCurrentConfiguration: a gateway without address
// and key (migration seed) must not overwrite anything; otherwise applying the
// migration would wipe the working configuration from env.
func TestWithGateway_EmptyRowKeepsCurrentConfiguration(t *testing.T) {
	base := Handler{
		AnthropicAPIBaseURL: "https://api.anthropic.com",
		AnthropicAPIKey:     "env-anthropic",
	}
	g := base.withGateway(context.Background(), aiGateway{ID: aiProviderAnthropic, Protocol: gatewayProtocolAnthropic})
	if g.AnthropicAPIBaseURL != "https://api.anthropic.com" {
		t.Errorf("пустой base_url затёр адрес: %q", g.AnthropicAPIBaseURL)
	}
	if got := g.providerAPIKey(context.Background(), aiProviderAnthropic); got != "env-anthropic" {
		t.Errorf("пустой ключ в строке затёр env: %q", got)
	}
}
