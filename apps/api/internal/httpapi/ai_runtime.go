package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

type openAIChatRequest struct {
	Model          string              `json:"model"`
	Messages       []map[string]string `json:"messages"`
	Temperature    float64             `json:"temperature"`
	ResponseFormat any                 `json:"response_format,omitempty"`
	MaxTokens      int                 `json:"max_tokens,omitempty"`
}

type openAIChatOptions struct {
	ResponseFormat any
	// MaxTokens is the response token ceiling. 0 = "not set at this level";
	// resolveMaxTokens substitutes per-mode -> system_settings -> const.
	MaxTokens int
}

// defaultAIMaxTokens is the ABSOLUTE last fallback for the response ceiling when it is
// set neither on the mode (modes.ai_max_tokens) nor globally in system_settings
// (ai_max_tokens_default). This is not a "hardcoded value" but a safety net for
// an empty DB: the real value comes from the DB (the mode slider or the default in
// the admin UI). Previously max_tokens was not sent at all, and truncation
// depended on the specific model's default at the aggregator; some models cut
// shorter than a long answer needs (text analysis, a presentation plan),
// which caused the "it cuts the message off" complaint. max_tokens is a ceiling, not
// mandatory generation, so it does not make cheap models more expensive.
const (
	defaultAIMaxTokens = 8192
	minAIMaxTokens     = 256
	maxAIMaxTokens     = 32768
)

// resolveMaxTokens computes the response ceiling from the DB: first the mode's value
// (modeValue, 0 = not set), then the global default from system_settings
// (ai_max_tokens_default), and only if the DB is empty, const defaultAIMaxTokens.
func (h Handler) resolveMaxTokens(ctx context.Context, modeValue int) int {
	if modeValue > 0 {
		if modeValue < minAIMaxTokens {
			return minAIMaxTokens
		}
		if modeValue > maxAIMaxTokens {
			return maxAIMaxTokens
		}
		return modeValue
	}
	return intSetting(ctx, h, "ai_max_tokens_default", defaultAIMaxTokens, minAIMaxTokens, maxAIMaxTokens)
}

// effectiveMaxTokens is the ceiling value for the provider request. A safety net for
// a direct provider call that bypasses resolveMaxTokens (legacy/tests).
func effectiveMaxTokens(optionValue int) int {
	if optionValue > 0 {
		return optionValue
	}
	return defaultAIMaxTokens
}

type openAIChatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int     `json:"prompt_tokens"`
		CompletionTokens int     `json:"completion_tokens"`
		TotalTokens      int     `json:"total_tokens"`
		Cost             float64 `json:"cost"`
		TotalCost        float64 `json:"total_cost"`
		EstimatedCost    float64 `json:"estimated_cost"`
	} `json:"usage"`
	Cost          float64 `json:"cost"`
	TotalCost     float64 `json:"total_cost"`
	EstimatedCost float64 `json:"estimated_cost"`
}

type liveAIDebugInfo struct {
	Provider        string              `json:"provider"`
	File            string              `json:"file"`
	Function        string              `json:"function"`
	URL             string              `json:"url"`
	Method          string              `json:"method"`
	RequestedModel  string              `json:"requestedModel"`
	ResolvedModel   string              `json:"resolvedModel"`
	Temperature     float64             `json:"temperature"`
	XTitle          string              `json:"xTitle,omitempty"`
	Messages        []map[string]string `json:"messages"`
	Status          int                 `json:"status,omitempty"`
	ResponseBody    string              `json:"responseBody,omitempty"`
	RequestBodySize int                 `json:"requestBodySize"`
}

type liveAIError struct {
	Message string
	Debug   liveAIDebugInfo
}

func normalizeAIModelID(raw string) (string, error) {
	model := strings.TrimSpace(raw)
	if model == "" {
		return "", errors.New("model id is required")
	}
	if len(model) > 160 {
		return "", errors.New("model id is too long")
	}
	for i := 0; i < len(model); i++ {
		ch := model[i]
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') {
			continue
		}
		switch ch {
		case '/', '-', '_', '.', ':', '@', '+':
			continue
		default:
			return "", errors.New("model id must contain only ASCII letters, digits, '/', '-', '_', '.', ':', '@' or '+'")
		}
	}
	return model, nil
}

func normalizeOptionalAIModelID(raw string) (string, error) {
	model := strings.TrimSpace(raw)
	if model == "" {
		return "", nil
	}
	return normalizeAIModelID(model)
}

func (e *liveAIError) Error() string {
	return e.Message
}

// redactGatewayURL keeps only the scheme and host of a gateway endpoint. Relay
// gateways carry their secret in the path, and the endpoint ends up in error
// messages and in the debug info shown to staff.
func redactGatewayURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "<redacted>"
	}
	return u.Scheme + "://" + u.Host + "/<redacted>"
}

// redactURLInError strips the path and query from the URL of a *url.Error
// (what http.Client.Do returns), keeping the rest of the message.
func redactURLInError(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		cp := *ue
		cp.URL = redactGatewayURL(ue.URL)
		return &cp
	}
	return err
}

// liveAIUserMessage is what an ordinary user sees when the AI call fails; the
// technical text (model, gateway) is kept for staff only.
const liveAIUserMessage = "ИИ сейчас не ответил. Попробуйте отправить сообщение ещё раз."

func liveAIDebugFromError(err error) (liveAIDebugInfo, bool) {
	var liveErr *liveAIError
	if errors.As(err, &liveErr) {
		return liveErr.Debug, true
	}
	return liveAIDebugInfo{}, false
}

func (h Handler) chatLiveAIDebug(ctx context.Context, userID int64, err error) (liveAIDebugInfo, bool) {
	debug, ok := liveAIDebugFromError(err)
	if !ok || h.DB == nil || userID == 0 {
		return liveAIDebugInfo{}, false
	}
	var role string
	if scanErr := h.DB.QueryRow(ctx, `select coalesce(role, '') from users where id=$1`, userID).Scan(&role); scanErr != nil {
		return liveAIDebugInfo{}, false
	}
	if adminMutationAllowed(role) || role == "tester" || role == "support" {
		return debug, true
	}
	return liveAIDebugInfo{}, false
}

func liveAIErrorResponse(err error) map[string]any {
	response := map[string]any{"ok": false, "error": err.Error(), "code": "live_ai_error"}
	if debug, ok := liveAIDebugFromError(err); ok {
		response["debug"] = debug
	}
	return response
}

func (h Handler) callLiveAI(ctx context.Context, userID, dialogID int64, mode modeRow, userText string) (string, int, int, float64, error) {
	ctx = withDataSubject(ctx, userID)
	historyLimit := int64(10)
	if value, err := h.systemSetting(ctx, "ai_chat_history_limit"); err == nil && strings.TrimSpace(value) != "" {
		if parsed, parseErr := strconv.ParseInt(strings.TrimSpace(value), 10, 64); parseErr == nil {
			if parsed < 0 {
				parsed = 0
			}
			if parsed > 100 {
				parsed = 100
			}
			historyLimit = parsed
		}
	}
	history, err := getDialogMessages(ctx, h.DB, dialogID, historyLimit, 0)
	if err != nil {
		return "", 0, 0, 0, err
	}

	guardrail, err := h.defaultModeGuardrail(ctx)
	if err != nil {
		return "", 0, 0, 0, err
	}
	// #6 fix: chat_send.go inserts userMsg into the DB BEFORE callLiveAI. The history
	// already contains the user's last message. We used to append
	// userText once more, so the AI got a duplicate that confused the model
	// (it could ignore the second occurrence and answer as if it were not there).
	// Now userText is appended only if it is really missing from history
	// (a safety net in case insertDialogMessage fails).
	messages := []map[string]string{{"role": "system", "content": buildRuntimeModePrompt(mode.Prompt, guardrail)}}
	lastUserInHistory := ""
	for _, msg := range history {
		if msg.Role == "summary" || isModeSwitchTraceMessage(msg) {
			continue
		}
		messages = append(messages, map[string]string{"role": msg.Role, "content": msg.Content})
		if msg.Role == "user" {
			lastUserInHistory = msg.Content
		}
	}
	if lastUserInHistory != userText {
		messages = append(messages, map[string]string{"role": "user", "content": userText})
	}

	xTitle := sanitizeXTitle(buildXTitle(mode.ID, userID, "TEXT"))
	spec := aiCallSpec{Provider: mode.AIProvider, Model: mode.AIModel, Temperature: mode.Temperature, ThinkingMode: mode.ThinkingMode}
	// The token ceiling comes from the mode (the slider in the admin UI); 0 = the global
	// default, which resolveMaxTokens takes from system_settings.
	return h.doAIChatResilientSpec(ctx, spec, messages, xTitle, openAIChatOptions{MaxTokens: int(mode.MaxTokens)})
}

func (h Handler) callLiveAISummary(ctx context.Context, userID int64, mode modeRow, dialog []ChatMessage) (string, int, int, float64, error) {
	ctx = withDataSubject(ctx, userID)
	transcript := new(strings.Builder)
	for _, msg := range dialog {
		if msg.Role == "summary" || isModeSwitchTraceMessage(msg) {
			continue
		}
		_, _ = fmt.Fprintf(transcript, "%s: %s\n", msg.Role, msg.Content)
	}

	prompt, err := h.dialogSummaryPrompt(ctx)
	if err != nil {
		return "", 0, 0, 0, err
	}
	model, temperature := h.adminSummaryModel(ctx)
	messages := []map[string]string{
		{"role": "system", "content": buildRuntimeModePrompt(prompt, "")},
		{"role": "user", "content": transcript.String()},
	}

	xTitle := sanitizeXTitle(buildXTitle(mode.ID, userID, "SUMMARY"))
	return h.doMechanicAIChat(ctx, "ai_summary_provider", model, temperature, messages, xTitle, openAIChatOptions{})
}

func (h Handler) doAIChat(ctx context.Context, model string, temperature float64, messages []map[string]string, xTitle string) (string, int, int, float64, error) {
	model = h.effectiveLiveAIModel(model)
	// Ollama routing removed on 2026-05-28 (legacy, not used in prod).
	// The EnableLiveAI flag removed on 2026-05-29: controlled by the checkbox in the chat,
	// the server gate only checks that a vsegpt key exists (it is set in the admin UI
	// and not read from env).
	if h.providerAPIKey(ctx, aiProviderVsegpt) == "" {
		return "", 0, 0, 0, errors.New("live AI is not configured: vsegpt API key missing (admin → AI gateways)")
	}
	return h.doOpenAIChat(ctx, model, temperature, messages, xTitle)
}

func (h Handler) echoTestCompletion(model string, messages []map[string]string) string {
	last := ""
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i]["role"] == "user" {
			last = strings.TrimSpace(messages[i]["content"])
			break
		}
	}
	if last == "" {
		last = "ok"
	}
	return "[ECHO TEST] " + last
}

func messagesText(messages []map[string]string) string {
	parts := make([]string, 0, len(messages))
	for _, msg := range messages {
		parts = append(parts, msg["role"]+":"+msg["content"])
	}
	return strings.Join(parts, "\n")
}

func nullableCost(cost float64) *float64 {
	if cost <= 0 {
		return nil
	}
	return &cost
}

func firstPositiveFloat(values ...float64) float64 {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func (h Handler) effectiveLiveAIModel(model string) string {
	model = strings.TrimSpace(model)
	if model == "" {
		return "gpt-4o-mini"
	}
	return model
}

func (h Handler) openAICompatibleModel(model string) string {
	model = h.effectiveLiveAIModel(model)
	if strings.Contains(model, "/") {
		return model
	}
	if !strings.Contains(strings.ToLower(h.OpenAIBaseURL), "vsegpt.ru") {
		return model
	}
	lower := strings.ToLower(model)
	if strings.HasPrefix(lower, "gpt-") || strings.HasPrefix(lower, "o1") || strings.HasPrefix(lower, "o3") || strings.HasPrefix(lower, "o4") {
		return "openai/" + model
	}
	return model
}

func (h Handler) configuredAIModel(ctx context.Context, modelKey, temperatureKey, fallbackModel string, fallbackTemperature float64) (string, float64) {
	model := strings.TrimSpace(fallbackModel)
	if value, err := h.systemSetting(ctx, modelKey); err == nil && strings.TrimSpace(value) != "" {
		model = strings.TrimSpace(value)
	}
	if model == "" {
		model = "gpt-4o-mini"
	}
	temperature := fallbackTemperature
	if value, err := h.systemSetting(ctx, temperatureKey); err == nil && strings.TrimSpace(value) != "" {
		if parsed, parseErr := strconv.ParseFloat(strings.TrimSpace(value), 64); parseErr == nil {
			temperature = parsed
		}
	}
	if temperature < 0 {
		temperature = 0
	}
	if temperature > 2 {
		temperature = 2
	}
	return model, temperature
}
