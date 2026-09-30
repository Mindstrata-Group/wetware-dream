package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestProviderAPIKey_EnvFallbackWithoutDB (History heuristic: the old env-only
// behaviour must not break when the admin override is added; h.DB=nil here
// imitates a deploy with no DB at hand at call time, which systemSetting already
// treats as "no override").
func TestProviderAPIKey_EnvFallbackWithoutDB(t *testing.T) {
	h := Handler{OpenAIAPIKey: "vsegpt-env", GeminiAPIKey: "gemini-env", AnthropicAPIKey: "anthropic-env"}
	ctx := context.Background()
	cases := []struct {
		provider string
		want     string
	}{
		{aiProviderVsegpt, "vsegpt-env"},
		{aiProviderGemini, "gemini-env"},
		{aiProviderAnthropic, "anthropic-env"},
	}
	for _, c := range cases {
		if got := h.providerAPIKey(ctx, c.provider); got != c.want {
			t.Errorf("providerAPIKey(%q) = %q, want %q (env fallback must work without DB override)", c.provider, got, c.want)
		}
		if !h.providerConfigured(ctx, c.provider) {
			t.Errorf("providerConfigured(%q) = false, want true (env key present)", c.provider)
		}
	}
}

// TestProviderAPIKey_EmptyWhenNothingConfigured: with no key anywhere it is empty,
// not a panic and not a default stub (User expectations: a predictable fallback).
func TestProviderAPIKey_EmptyWhenNothingConfigured(t *testing.T) {
	h := Handler{}
	ctx := context.Background()
	for _, p := range []string{aiProviderVsegpt, aiProviderGemini, aiProviderAnthropic} {
		if got := h.providerAPIKey(ctx, p); got != "" {
			t.Errorf("providerAPIKey(%q) = %q, want empty", p, got)
		}
		if h.providerConfigured(ctx, p) {
			t.Errorf("providerConfigured(%q) = true, want false", p)
		}
	}
}

func TestNormalizeAIProvider(t *testing.T) {
	cases := map[string]string{
		"gemini":     "gemini",
		"Anthropic":  "anthropic",
		"VSEGPT":     "vsegpt",
		"unknown":    "vsegpt",
		"":           "vsegpt",
		"  gemini  ": "gemini",
	}
	for in, want := range cases {
		if got := normalizeAIProvider(in); got != want {
			t.Errorf("normalizeAIProvider(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNormalizeThinkingMode(t *testing.T) {
	cases := map[string]string{
		"off":     "off",
		"LOW":     "low",
		"high":    "high",
		"weird":   "default",
		"":        "default",
		"default": "default",
	}
	for in, want := range cases {
		if got := normalizeThinkingMode(in); got != want {
			t.Errorf("normalizeThinkingMode(%q) = %q, want %q", in, got, want)
		}
	}
}

// testGateways: the three built-in gateways in priority order, with fallback
// models set. A separate helper so chain tests talk about the order of links,
// not about filling structs.
func testGateways(defaults map[string]string) []aiGateway {
	out := []aiGateway{}
	for _, g := range builtinGateways() {
		g.DefaultModel = defaults[g.ID]
		out = append(out, g)
	}
	return out
}

// testGatewaysWithout: the same, minus the listed gateways: that is how a
// gateway switched off by its toggle or left without a key looks when the chain
// is built.
func testGatewaysWithout(defaults map[string]string, drop ...string) []aiGateway {
	dropped := map[string]bool{}
	for _, id := range drop {
		dropped[id] = true
	}
	out := []aiGateway{}
	for _, g := range testGateways(defaults) {
		if dropped[g.ID] {
			continue
		}
		out = append(out, g)
	}
	return out
}

var testChainDefaults = map[string]string{
	aiProviderAnthropic: "claude-haiku-4-5",
	aiProviderGemini:    "gemini-3.1-flash-lite",
	aiProviderVsegpt:    "google/gemini-3.1-flash-lite-thinking",
}

func TestBuildProviderChainFrom_SelectedFirst(t *testing.T) {
	spec := aiCallSpec{Provider: "gemini", Model: "gemini-3.5-flash-minimal", ThinkingMode: "high"}

	chain := buildProviderChainFrom(spec, testGateways(testChainDefaults))
	if len(chain) != 3 {
		t.Fatalf("chain len = %d, want 3: %+v", len(chain), chain)
	}
	if chain[0].Provider != aiProviderGemini || chain[0].Model != "gemini-3.5-flash-minimal" || chain[0].ThinkingMode != "high" {
		t.Errorf("first link = %+v, want selected gemini with own model/thinking", chain[0])
	}
	// Then the fixed order anthropic → gemini → vsegpt, without repeating the selected gemini.
	if chain[1].Provider != aiProviderAnthropic || chain[1].Model != "claude-haiku-4-5" {
		t.Errorf("second link = %+v, want anthropic fallback", chain[1])
	}
	if chain[2].Provider != aiProviderVsegpt {
		t.Errorf("third link = %+v, want vsegpt fallback", chain[2])
	}
	for _, l := range chain {
		if l.Provider == aiProviderGemini && l != chain[0] {
			t.Errorf("gemini duplicated in chain: %+v", chain)
		}
	}
}

func TestBuildProviderChainFrom_AnthropicSelected(t *testing.T) {
	spec := aiCallSpec{Provider: "anthropic", Model: "claude-sonnet-5", ThinkingMode: "default"}

	chain := buildProviderChainFrom(spec, testGateways(testChainDefaults))
	want := []string{aiProviderAnthropic, aiProviderGemini, aiProviderVsegpt}
	if len(chain) != 3 {
		t.Fatalf("chain len = %d, want 3: %+v", len(chain), chain)
	}
	for i, w := range want {
		if chain[i].Provider != w {
			t.Errorf("chain[%d].Provider = %q, want %q", i, chain[i].Provider, w)
		}
	}
	if chain[0].Model != "claude-sonnet-5" {
		t.Errorf("selected anthropic should keep its own model, got %q", chain[0].Model)
	}
}

func TestBuildProviderChainFrom_DisabledSkipped(t *testing.T) {
	spec := aiCallSpec{Provider: "gemini", Model: "gemini-3.5-flash-minimal"}

	chain := buildProviderChainFrom(spec, testGatewaysWithout(testChainDefaults, aiProviderAnthropic))
	if len(chain) != 2 {
		t.Fatalf("chain len = %d, want 2 (anthropic disabled/no key): %+v", len(chain), chain)
	}
	for _, l := range chain {
		if l.Provider == aiProviderAnthropic {
			t.Errorf("disabled anthropic must not appear in chain: %+v", chain)
		}
	}
}

func TestBuildProviderChainFrom_NoneAvailable(t *testing.T) {
	spec := aiCallSpec{Provider: "gemini", Model: "gemini-3.5-flash-minimal"}
	chain := buildProviderChainFrom(spec, nil)
	if len(chain) != 0 {
		t.Fatalf("chain should be empty when nothing configured, got %+v", chain)
	}
}

func TestEstimateAICostRUB_AnthropicWithCache(t *testing.T) {
	// haiku 4.5: in=90₽/M, out=450₽/M (see ai_pricing.go).
	// 1000 regular input, 500 out, 2000 cache_read, 300 cache_write.
	got := estimateAICostRUB(aiProviderAnthropic, "claude-haiku-4-5", 1000, 500, 2000, 300)
	// in: 1000*90/1e6 = 0.09
	// out: 500*450/1e6 = 0.225
	// cache_read: 2000*90/1e6*0.1 = 0.018
	// cache_write: 300*90/1e6*1.25 = 0.03375
	want := 0.09 + 0.225 + 0.018 + 0.03375
	if diff := got - want; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("estimateAICostRUB = %v, want %v", got, want)
	}
}

func TestEstimateAICostRUB_GeminiCached(t *testing.T) {
	got := estimateAICostRUB(aiProviderGemini, "gemini-3.1-flash-lite", 1000, 500, 500, 0)
	// flash-lite: in=22.5₽/M, out=135₽/M
	want := 1000*22.5/1e6 + 500*135/1e6 + 500*22.5/1e6*0.1
	if diff := got - want; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("estimateAICostRUB(gemini) = %v, want %v", got, want)
	}
}

func TestEstimateAICostRUB_UnknownModelZero(t *testing.T) {
	if got := estimateAICostRUB(aiProviderGemini, "some-unlisted-model", 1000, 500, 0, 0); got != 0 {
		t.Errorf("unknown model should cost 0, got %v", got)
	}
}

func TestGeminiReasoningEffort(t *testing.T) {
	cases := map[string]string{"off": "none", "low": "low", "high": "high", "default": "", "": ""}
	for in, want := range cases {
		if got := geminiReasoningEffort(in); got != want {
			t.Errorf("geminiReasoningEffort(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAnthropicThinkingConfig(t *testing.T) {
	if th, max := anthropicThinkingConfig("off"); th != nil || max != anthropicDefaultMaxTokens {
		t.Errorf("off: thinking=%+v max=%d, want nil/%d", th, max, anthropicDefaultMaxTokens)
	}
	if th, max := anthropicThinkingConfig("low"); th == nil || th.BudgetTokens != 2048 || max != 10240 {
		t.Errorf("low: thinking=%+v max=%d, want budget=2048 max=10240", th, max)
	}
	if th, max := anthropicThinkingConfig("high"); th == nil || th.BudgetTokens != 8192 || max != 16384 {
		t.Errorf("high: thinking=%+v max=%d, want budget=8192 max=16384", th, max)
	}
}

func TestAnthropicMessagesFromOpenAI(t *testing.T) {
	messages := []map[string]string{
		{"role": "system", "content": "Ты психолог."},
		{"role": "assistant", "content": "Здравствуйте"},
		{"role": "user", "content": "Привет"},
		{"role": "user", "content": "Ещё вопрос"},
	}
	system, converted := anthropicMessagesFromOpenAI(messages)
	if system != "Ты психолог." {
		t.Errorf("system = %q, want %q", system, "Ты психолог.")
	}
	// The dialog starts with an assistant welcome message → a service user message is
	// inserted before it, then assistant, then two user messages merged into one.
	if len(converted) != 3 {
		t.Fatalf("converted len = %d, want 3: %+v", len(converted), converted)
	}
	if converted[0].Role != "user" {
		t.Errorf("first message must be role=user (Anthropic requirement), got %q", converted[0].Role)
	}
	if converted[1].Role != "assistant" || converted[1].Content != "Здравствуйте" {
		t.Errorf("second message = %+v, want assistant welcome", converted[1])
	}
	if converted[2].Role != "user" || converted[2].Content != "Привет\n\nЕщё вопрос" {
		t.Errorf("consecutive user messages should merge, got %+v", converted[2])
	}
}

// TestResolveMaxTokens (the "cuts the message off" bug): the token ceiling comes
// from the DB: first the mode's value, then the global default, and only with
// an empty DB the const safety net. Here h.DB=nil imitates "setting not set" →
// defaultAIMaxTokens must apply, and the mode value must be clamped to range.
func TestResolveMaxTokens(t *testing.T) {
	h := Handler{} // DB=nil → system_settings empty → const fallback
	ctx := context.Background()
	cases := []struct {
		modeValue int
		want      int
	}{
		{5000, 5000},             // valid mode value: as is
		{100, minAIMaxTokens},    // below the minimum: clamp
		{999999, maxAIMaxTokens}, // above the maximum: clamp
		{0, defaultAIMaxTokens},  // not set on the mode, DB empty: const
		{-1, defaultAIMaxTokens}, // negative = not set
	}
	for _, c := range cases {
		if got := h.resolveMaxTokens(ctx, c.modeValue); got != c.want {
			t.Errorf("resolveMaxTokens(%d) = %d, want %d", c.modeValue, got, c.want)
		}
	}
	if got := effectiveMaxTokens(0); got != defaultAIMaxTokens {
		t.Errorf("effectiveMaxTokens(0) = %d, want %d (safety fallback)", got, defaultAIMaxTokens)
	}
	if got := effectiveMaxTokens(1234); got != 1234 {
		t.Errorf("effectiveMaxTokens(1234) = %d, want 1234", got)
	}
}

// captureLogOutput redirects the standard logger to a buffer for the test and
// restores it afterwards (log.Printf is used for the warning about a response
// cut off by max_tokens: the only way to check it).
func captureLogOutput(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(prev) })
	return &buf
}

// TestDoOpenAIChat_SendsMaxTokensAndLogsTruncation (the "cuts the message off,
// does not finish the answer" bug): the vsegpt/OpenAI-compatible path serves
// 100% of live chat on prod (all active modes use vsegpt), but max_tokens used
// not to be sent at all: truncation depended on the aggregator's default for the
// model. We check that max_tokens now goes into the request and that
// finish_reason="length" (response really truncated) is logged, not silently
// swallowed.
func TestDoOpenAIChat_SendsMaxTokensAndLogsTruncation(t *testing.T) {
	var gotMaxTokens int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req openAIChatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		gotMaxTokens = req.MaxTokens
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"обрезанный ответ"},"finish_reason":"length"}],"usage":{"prompt_tokens":10,"completion_tokens":8192}}`))
	}))
	defer srv.Close()

	buf := captureLogOutput(t)
	h := Handler{OpenAIAPIKey: "test-key", OpenAIBaseURL: srv.URL, HTTPClient: http.DefaultClient}
	content, _, _, _, err := h.doOpenAIChat(context.Background(), "test-model", 0.5, []map[string]string{{"role": "user", "content": "привет"}}, "")
	if err != nil {
		t.Fatalf("doOpenAIChat: %v", err)
	}
	if content != "обрезанный ответ" {
		t.Fatalf("content = %q", content)
	}
	if gotMaxTokens != defaultAIMaxTokens {
		t.Errorf("request max_tokens = %d, want %d (explicit ceiling — раньше поле не отправлялось вовсе)", gotMaxTokens, defaultAIMaxTokens)
	}
	if !strings.Contains(buf.String(), "truncated by max_tokens") {
		t.Errorf("expected truncation warning in log output, got: %q", buf.String())
	}
}

// TestDoGeminiChat_SendsMaxTokensAndLogsTruncation: the same protection for the
// direct Gemini provider (multi-provider routing is not yet on for any mode on
// prod, but the code must behave the same for all three providers; otherwise the
// bug simply moves there once it is switched on).
func TestDoGeminiChat_SendsMaxTokensAndLogsTruncation(t *testing.T) {
	var gotMaxTokens int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req geminiChatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		gotMaxTokens = req.MaxTokens
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"обрезанный ответ"},"finish_reason":"length"}],"usage":{"prompt_tokens":10,"completion_tokens":8192}}`))
	}))
	defer srv.Close()

	buf := captureLogOutput(t)
	h := Handler{GeminiAPIKey: "test-key", GeminiAPIBaseURL: srv.URL, HTTPClient: http.DefaultClient}
	content, _, _, _, err := h.doGeminiChat(context.Background(), "test-model", 0.5, "default", []map[string]string{{"role": "user", "content": "привет"}}, openAIChatOptions{})
	if err != nil {
		t.Fatalf("doGeminiChat: %v", err)
	}
	if content != "обрезанный ответ" {
		t.Fatalf("content = %q", content)
	}
	if gotMaxTokens != defaultAIMaxTokens {
		t.Errorf("request max_tokens = %d, want %d", gotMaxTokens, defaultAIMaxTokens)
	}
	if !strings.Contains(buf.String(), "truncated by max_tokens") {
		t.Errorf("expected truncation warning in log output, got: %q", buf.String())
	}
}

// TestDoAnthropicChat_LogsTruncationOnMaxTokensStopReason: likewise for the
// direct Anthropic provider: stop_reason="max_tokens" in the response must reach
// the log, not be silently ignored.
func TestDoAnthropicChat_LogsTruncationOnMaxTokensStopReason(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"обрезанный ответ"}],"stop_reason":"max_tokens","usage":{"input_tokens":10,"output_tokens":8192}}`))
	}))
	defer srv.Close()

	buf := captureLogOutput(t)
	h := Handler{AnthropicAPIKey: "test-key", AnthropicAPIBaseURL: srv.URL, HTTPClient: http.DefaultClient}
	content, _, _, _, err := h.doAnthropicChat(context.Background(), "test-model", 0.5, "default", []map[string]string{{"role": "user", "content": "привет"}}, openAIChatOptions{})
	if err != nil {
		t.Fatalf("doAnthropicChat: %v", err)
	}
	if content != "обрезанный ответ" {
		t.Fatalf("content = %q", content)
	}
	if !strings.Contains(buf.String(), "truncated by max_tokens") {
		t.Errorf("expected truncation warning in log output, got: %q", buf.String())
	}
}
