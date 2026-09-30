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
	"strings"
	"time"
)

func (h Handler) doOpenAIChat(ctx context.Context, model string, temperature float64, messages []map[string]string, xTitle string) (string, int, int, float64, error) {
	return h.doOpenAIChatWithOptions(ctx, model, temperature, messages, xTitle, openAIChatOptions{})
}

func (h Handler) doOpenAIChatWithOptions(ctx context.Context, model string, temperature float64, messages []map[string]string, xTitle string, options openAIChatOptions) (string, int, int, float64, error) {
	requestModel := h.openAICompatibleModel(model)
	body, _ := json.Marshal(openAIChatRequest{Model: requestModel, Messages: messages, Temperature: temperature, ResponseFormat: options.ResponseFormat, MaxTokens: effectiveMaxTokens(options.MaxTokens)})
	endpoint := strings.TrimRight(h.OpenAIBaseURL, "/") + "/chat/completions"
	debug := liveAIDebugInfo{
		Provider:        "openai-compatible",
		File:            "apps/api/internal/httpapi/handlers.go",
		Function:        "doOpenAIChat",
		URL:             redactGatewayURL(endpoint),
		Method:          http.MethodPost,
		RequestedModel:  h.effectiveLiveAIModel(model),
		ResolvedModel:   requestModel,
		Temperature:     temperature,
		XTitle:          strings.TrimSpace(xTitle),
		Messages:        messages,
		RequestBodySize: len(body),
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", 0, 0, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+h.providerAPIKey(ctx, aiProviderVsegpt))
	req.Header.Set("Content-Type", "application/json")

	if strings.TrimSpace(xTitle) != "" {
		req.Header.Set("X-Title", xTitle)
	}

	client := h.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		debug.ResponseBody = redactURLInError(err).Error()
		return "", 0, 0, 0, &liveAIError{Message: fmt.Sprintf("live AI transport error (%s): %s", requestModel, redactURLInError(err).Error()), Debug: debug}
	}
	defer resp.Body.Close()
	// W-NEW-1: limit on the AI response (8 MB) protects against OOM from a malicious/buggy upstream.
	payload, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	debug.Status = resp.StatusCode
	debug.ResponseBody = strings.TrimSpace(string(payload))
	if resp.StatusCode >= 300 {
		message := debug.ResponseBody
		if message == "" {
			message = resp.Status
		}
		return "", 0, 0, 0, &liveAIError{Message: fmt.Sprintf("live AI error (%s): %s", requestModel, message), Debug: debug}
	}
	var parsed openAIChatResponse
	if err := json.Unmarshal(payload, &parsed); err != nil {
		return "", 0, 0, 0, err
	}
	if len(parsed.Choices) == 0 {
		return "", 0, 0, 0, errors.New("live AI returned no choices")
	}
	totalTokens := parsed.Usage.TotalTokens
	if totalTokens == 0 {
		totalTokens = parsed.Usage.PromptTokens + parsed.Usage.CompletionTokens
	}
	cost := firstPositiveFloat(parsed.Usage.Cost, parsed.Usage.TotalCost, parsed.Usage.EstimatedCost, parsed.Cost, parsed.TotalCost, parsed.EstimatedCost)
	content := strings.TrimSpace(parsed.Choices[0].Message.Content)
	if parsed.Choices[0].FinishReason == "length" {
		// The answer was really truncated by the token limit: not an API error, but the client
		// gets unfinished text. Log it to diagnose "it cuts the message off"
		// complaints instead of silently returning partial text.
		log.Printf("live AI response truncated by max_tokens model=%s completion_tokens=%d", requestModel, parsed.Usage.CompletionTokens)
	}
	if parsed.Usage.PromptTokens == 0 && content != "" {
		var raw map[string]json.RawMessage
		if json.Unmarshal(payload, &raw) == nil {
			log.Printf("AI zero-token warning model=%s usage=%s", requestModel, string(raw["usage"]))
		}
	}
	return content, parsed.Usage.PromptTokens, parsed.Usage.CompletionTokens, cost, nil
}
