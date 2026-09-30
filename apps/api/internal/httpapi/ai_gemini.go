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

// Direct Google Gemini through its OpenAI-compatible endpoint
// (.../v1beta/openai/chat/completions). Advantages over vsegpt: no subscription,
// Google list prices, and implicit prompt caching (the cached prefix, our
// system prompt, is billed at 0.1x the input price automatically,
// without cache_control and without a storage fee).
//
// thinking_mode -> API:
//   off  -> reasoning_effort:"none"; Gemini 3.x in OpenAI compat accepts
//          none/low/medium/high (for 2.5, "none" is unavailable on pro; our
//          modes use 3.x, so this is fine);
//   low  -> reasoning_effort:"low";
//   high -> reasoning_effort:"high";
//   default -> the field is not sent, the model decides.

type geminiChatRequest struct {
	Model           string              `json:"model"`
	Messages        []map[string]string `json:"messages"`
	Temperature     float64             `json:"temperature"`
	ResponseFormat  any                 `json:"response_format,omitempty"`
	ReasoningEffort string              `json:"reasoning_effort,omitempty"`
	MaxTokens       int                 `json:"max_tokens,omitempty"`
}

type geminiChatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens        int `json:"prompt_tokens"`
		CompletionTokens    int `json:"completion_tokens"`
		PromptTokensDetails struct {
			CachedTokens int `json:"cached_tokens"`
		} `json:"prompt_tokens_details"`
	} `json:"usage"`
}

func geminiReasoningEffort(thinkingMode string) string {
	switch normalizeThinkingMode(thinkingMode) {
	case "off":
		return "none"
	case "low":
		return "low"
	case "high":
		return "high"
	}
	return ""
}

func (h Handler) doGeminiChat(ctx context.Context, model string, temperature float64, thinkingMode string, messages []map[string]string, options openAIChatOptions) (string, int, int, float64, error) {
	// Two ways to reach Gemini. Vertex is enabled by a service account key and
	// differs in exactly two things: the base URL and authorisation; its request
	// body is the same OpenAI-compatible one. Why it was needed at all
	// (a geo-ban on the datacenter IP that routing cannot fix): see
	// ai_gemini_vertex.go.
	vertex := h.vertexConfigured()
	apiKey := ""
	modelName := strings.TrimSpace(model)
	baseURL := h.GeminiAPIBaseURL
	if vertex {
		token, err := h.vertexAccessToken(ctx)
		if err != nil {
			return "", 0, 0, 0, err
		}
		apiKey = token
		baseURL = h.vertexBaseURL()
		modelName = vertexModelName(modelName)
	} else {
		apiKey = h.providerAPIKey(ctx, aiProviderGemini)
		if apiKey == "" {
			return "", 0, 0, 0, errors.New("gemini provider is not configured: GEMINI_API_KEY missing")
		}
	}
	reqPayload := geminiChatRequest{
		Model:           modelName,
		Messages:        messages,
		Temperature:     temperature,
		ResponseFormat:  options.ResponseFormat,
		ReasoningEffort: geminiReasoningEffort(thinkingMode),
		MaxTokens:       effectiveMaxTokens(options.MaxTokens),
	}
	body, _ := json.Marshal(reqPayload)
	endpoint := strings.TrimRight(baseURL, "/") + "/chat/completions"
	providerLabel := "gemini-direct"
	if vertex {
		providerLabel = "gemini-vertex"
	}
	debug := liveAIDebugInfo{
		Provider:        providerLabel,
		File:            "apps/api/internal/httpapi/ai_gemini.go",
		Function:        "doGeminiChat",
		URL:             redactGatewayURL(endpoint),
		Method:          http.MethodPost,
		RequestedModel:  model,
		ResolvedModel:   reqPayload.Model,
		Temperature:     temperature,
		Messages:        messages,
		RequestBodySize: len(body),
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", 0, 0, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	client := h.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		debug.ResponseBody = redactURLInError(err).Error()
		return "", 0, 0, 0, &liveAIError{Message: fmt.Sprintf("gemini transport error (%s): %s", model, redactURLInError(err).Error()), Debug: debug}
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
		return "", 0, 0, 0, &liveAIError{Message: fmt.Sprintf("gemini error (%s): %s", model, message), Debug: debug}
	}
	var parsed geminiChatResponse
	if err := json.Unmarshal(payload, &parsed); err != nil {
		return "", 0, 0, 0, err
	}
	if len(parsed.Choices) == 0 {
		return "", 0, 0, 0, errors.New("gemini returned no choices")
	}
	content := strings.TrimSpace(parsed.Choices[0].Message.Content)
	if parsed.Choices[0].FinishReason == "length" {
		// The answer was really truncated by the token limit: not an API error, but the client
		// gets unfinished text. Log it to diagnose "it cuts the message off"
		// complaints instead of silently returning partial text.
		log.Printf("gemini response truncated by max_tokens model=%s completion_tokens=%d", model, parsed.Usage.CompletionTokens)
	}
	in := parsed.Usage.PromptTokens
	out := parsed.Usage.CompletionTokens
	cached := parsed.Usage.PromptTokensDetails.CachedTokens
	if cached > in {
		cached = in
	}
	// prompt_tokens includes cached tokens; for pricing we split them into fresh and cached.
	cost := estimateAICostRUB(aiProviderGemini, model, in-cached, out, cached, 0)
	return content, in, out, cost, nil
}
