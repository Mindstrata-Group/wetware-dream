package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// Direct Anthropic through the NATIVE Messages API (not OpenAI compatibility):
// only the native format allows prompt caching via cache_control.
// The system prompt (the static prefix of every request) is marked with an
// ephemeral cache: a cache read costs 0.1x the input price, the cache is shared
// across the organisation and extended on every hit (TTL 5 minutes), so all
// dialogs of one mode share one cache.
//
// thinking_mode -> API:
//   off/default -> no thinking block, max_tokens 8192;
//   low  -> {"type":"enabled","budget_tokens":2048},  max_tokens 10240;
//   high -> {"type":"enabled","budget_tokens":8192},  max_tokens 16384.
// With thinking enabled Anthropic requires temperature=1, so we force it.
//
// Limits raised on 2026-07-10 after the complaint "it cuts the message off,
// does not finish the answer": the old 4096-token ceiling for off/default cut
// long answers (text analysis, a presentation plan, etc.) midway with no sign in
// the response that the text was truncated. See also stop_reason below: we
// now log a warning when output is truncated by max_tokens, for diagnostics.

const (
	anthropicVersion          = "2023-06-01"
	anthropicDefaultMaxTokens = 8192
)

type anthropicSystemBlock struct {
	Type         string          `json:"type"`
	Text         string          `json:"text"`
	CacheControl json.RawMessage `json:"cache_control,omitempty"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicThinking struct {
	Type         string `json:"type"`
	BudgetTokens int    `json:"budget_tokens"`
}

type anthropicRequest struct {
	Model       string                 `json:"model"`
	MaxTokens   int                    `json:"max_tokens"`
	System      []anthropicSystemBlock `json:"system,omitempty"`
	Messages    []anthropicMessage     `json:"messages"`
	Temperature *float64               `json:"temperature,omitempty"`
	Thinking    *anthropicThinking     `json:"thinking,omitempty"`
}

type anthropicResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	StopReason string `json:"stop_reason"`
	Usage      struct {
		InputTokens              int `json:"input_tokens"`
		OutputTokens             int `json:"output_tokens"`
		CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
		CacheReadInputTokens     int `json:"cache_read_input_tokens"`
	} `json:"usage"`
}

// anthropicTemperatureDeprecated: for generation-5 models (sonnet-5, opus-5,
// fable-5, haiku-5...) the temperature parameter is deprecated: any value
// other than the default of one is rejected with
// `invalid_request_error: temperature is deprecated for this model`.
// Prod failed on this since 2026-08-06: all anthropic modes were set to 0.55-0.7,
// every request was rejected with 400 and fell down the chain to the backup provider.
// For these models temperature is not sent at all; the API uses its default.
func anthropicTemperatureDeprecated(model string) bool {
	m := strings.ToLower(strings.TrimSpace(model))
	// Exactly claude-<family>-5[-suffix]: claude-sonnet-5, claude-opus-5-20260101.
	// The old claude-3-5-sonnet and claude-haiku-4-5 must not match: their
	// generation number comes BEFORE the family name, and temperature works there.
	return anthropicGen5Re.MatchString(m)
}

// claude-<family>-5 with an optional version/date suffix.
var anthropicGen5Re = regexp.MustCompile(`^claude-[a-z]+-5(-.+)?$`)

// anthropicThinkingConfig maps thinking_mode -> (thinking, max_tokens).
func anthropicThinkingConfig(thinkingMode string) (*anthropicThinking, int) {
	switch normalizeThinkingMode(thinkingMode) {
	case "low":
		return &anthropicThinking{Type: "enabled", BudgetTokens: 2048}, 10240
	case "high":
		return &anthropicThinking{Type: "enabled", BudgetTokens: 8192}, 16384
	}
	return nil, anthropicDefaultMaxTokens
}

// anthropicMessagesFromOpenAI converts our OpenAI-format history into the
// native one: the first system message goes into the system block (with
// cache_control), other system inserts are appended to it; adjacent identical
// roles are merged (the Messages API requires user/assistant alternation starting with user).
func anthropicMessagesFromOpenAI(messages []map[string]string) (string, []anthropicMessage) {
	system := ""
	converted := []anthropicMessage{}
	for _, msg := range messages {
		role := msg["role"]
		content := msg["content"]
		if role == "system" {
			if system != "" {
				system += "\n\n"
			}
			system += content
			continue
		}
		if role != "assistant" {
			role = "user"
		}
		if len(converted) > 0 && converted[len(converted)-1].Role == role {
			converted[len(converted)-1].Content += "\n\n" + content
			continue
		}
		converted = append(converted, anthropicMessage{Role: role, Content: content})
	}
	// It must start with user: our dialog may begin with the assistant's
	// welcome message.
	if len(converted) > 0 && converted[0].Role == "assistant" {
		converted = append([]anthropicMessage{{Role: "user", Content: "(начало диалога)"}}, converted...)
	}
	if len(converted) == 0 {
		converted = append(converted, anthropicMessage{Role: "user", Content: ""})
	}
	return system, converted
}

func (h Handler) doAnthropicChat(ctx context.Context, model string, temperature float64, thinkingMode string, messages []map[string]string, options openAIChatOptions) (string, int, int, float64, error) {
	apiKey := h.providerAPIKey(ctx, aiProviderAnthropic)
	if apiKey == "" {
		return "", 0, 0, 0, errors.New("anthropic provider is not configured: ANTHROPIC_API_KEY missing")
	}
	system, converted := anthropicMessagesFromOpenAI(messages)
	thinking, maxTokens := anthropicThinkingConfig(thinkingMode)
	// The ceiling from the mode/settings (options.MaxTokens) overrides the thinking
	// config default, but cannot be below the thinking budget: Anthropic
	// requires max_tokens > budget_tokens, otherwise nothing is left for the answer.
	if want := effectiveMaxTokens(options.MaxTokens); want > 0 {
		floor := maxTokens
		if thinking != nil && thinking.BudgetTokens+512 > floor {
			floor = thinking.BudgetTokens + 512
		}
		if want < floor {
			want = floor
		}
		maxTokens = want
	}
	temp := temperature
	if thinking != nil {
		// API requirement: extended thinking only works with temperature=1.
		temp = 1
	}
	reqPayload := anthropicRequest{
		Model:     strings.TrimSpace(model),
		MaxTokens: maxTokens,
		Messages:  converted,
		Thinking:  thinking,
	}
	// For generation-5 models temperature is deprecated (see anthropicTemperatureDeprecated):
	// sending ANY value other than 1 returns 400 and knocks the provider out of the chain,
	// so outside extended thinking the parameter is simply not sent.
	// With extended thinking the API requires an explicit temperature=1 in the request body (verified
	// live: gen-5 accepts temp=1 and rejects only non-one values), so this exception
	// is always kept, regardless of the model's deprecated status.
	if thinking != nil || !anthropicTemperatureDeprecated(reqPayload.Model) {
		reqPayload.Temperature = &temp
	}
	if system != "" {
		reqPayload.System = []anthropicSystemBlock{{
			Type:         "text",
			Text:         system,
			CacheControl: json.RawMessage(`{"type":"ephemeral"}`),
		}}
	}
	body, _ := json.Marshal(reqPayload)
	endpoint := strings.TrimRight(h.AnthropicAPIBaseURL, "/") + "/v1/messages"
	debug := liveAIDebugInfo{
		Provider:        "anthropic-direct",
		File:            "apps/api/internal/httpapi/ai_anthropic.go",
		Function:        "doAnthropicChat",
		URL:             redactGatewayURL(endpoint),
		Method:          http.MethodPost,
		RequestedModel:  model,
		ResolvedModel:   reqPayload.Model,
		Temperature:     temp,
		Messages:        messages,
		RequestBodySize: len(body),
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", 0, 0, 0, err
	}
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", anthropicVersion)
	req.Header.Set("Content-Type", "application/json")

	client := h.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 120 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		debug.ResponseBody = redactURLInError(err).Error()
		return "", 0, 0, 0, &liveAIError{Message: fmt.Sprintf("anthropic transport error (%s): %s", model, redactURLInError(err).Error()), Debug: debug}
	}
	defer resp.Body.Close()
	payload, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	debug.Status = resp.StatusCode
	debug.ResponseBody = strings.TrimSpace(string(payload))
	if resp.StatusCode >= 300 {
		message := debug.ResponseBody
		if message == "" {
			message = resp.Status
		}
		return "", 0, 0, 0, &liveAIError{Message: fmt.Sprintf("anthropic error (%s): %s", model, message), Debug: debug}
	}
	var parsed anthropicResponse
	if err := json.Unmarshal(payload, &parsed); err != nil {
		return "", 0, 0, 0, err
	}
	var text strings.Builder
	for _, block := range parsed.Content {
		if block.Type == "text" {
			text.WriteString(block.Text)
		}
	}
	content := strings.TrimSpace(text.String())
	if content == "" {
		return "", 0, 0, 0, errors.New("anthropic returned no text content")
	}
	if parsed.StopReason == "max_tokens" {
		// The answer was really truncated by the token limit: not an API error, but the client
		// gets unfinished text. Log it to diagnose "it cuts the message off"
		// complaints instead of silently returning partial text.
		log.Printf("anthropic response truncated by max_tokens model=%s max_tokens=%d output_tokens=%d", model, maxTokens, parsed.Usage.OutputTokens)
	}
	u := parsed.Usage
	// In the native usage, input_tokens does NOT include the cache counters; for the familiar
	// prompt_tokens semantics (as in OpenAI-compatible APIs) we sum everything.
	totalIn := u.InputTokens + u.CacheCreationInputTokens + u.CacheReadInputTokens
	cost := estimateAICostRUB(aiProviderAnthropic, model, u.InputTokens, u.OutputTokens, u.CacheReadInputTokens, u.CacheCreationInputTokens)
	return content, totalIn, u.OutputTokens, cost, nil
}
