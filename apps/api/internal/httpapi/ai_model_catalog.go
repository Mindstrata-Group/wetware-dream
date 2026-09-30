package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Automatic retrieval of the provider's real model list, instead of the
// admin remembering exact ids by heart. The list is stable (new models come out
// once in weeks), so it is cached for an hour rather than for seconds like other
// admin endpoints.
const aiModelCatalogCacheTTL = time.Hour

// aiCatalogModel is what the dropdown actually needs: id + label +
// thinking support (so the UI can hide the toggle if the model cannot do it).
type aiCatalogModel struct {
	ID               string `json:"id"`
	DisplayName      string `json:"displayName"`
	SupportsThinking bool   `json:"supportsThinking"`
	ContextWindow    int64  `json:"contextWindow,omitempty"`
}

// AdminAIModels handles GET /api/admin/ai-models?provider=gemini|anthropic
// We do not request the model list from vsegpt: it is an open router over dozens of
// providers without a single settled format, and model names there are already
// stabilised by the project convention (normalizeAIModelID). Free input
// remains the only way for gemini/anthropic too (the datalist on the
// frontend does not replace it, it only suggests).
func (h Handler) AdminAIModels(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireAdminSection(w, r, "modes", false)
	if !ok {
		return
	}
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
		return
	}
	provider := normalizeAIProvider(r.URL.Query().Get("provider"))
	if provider == aiProviderVsegpt {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "provider": provider, "models": []aiCatalogModel{}, "note": "vsegpt: свободный ввод, список не поддерживается"})
		return
	}
	cacheKey := provider
	if body, hit := h.c.adminAIModels.get(cacheKey); hit {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
		return
	}
	apiKey := h.providerAPIKey(r.Context(), provider)
	if apiKey == "" {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "provider": provider, "models": []aiCatalogModel{}, "note": "провайдер не настроен (нет ключа)"})
		return
	}
	var models []aiCatalogModel
	var err error
	switch provider {
	case aiProviderGemini:
		models, err = h.fetchGeminiModelCatalog(r.Context(), apiKey)
	case aiProviderAnthropic:
		models, err = h.fetchAnthropicModelCatalog(r.Context(), apiKey)
	}
	if err != nil {
		// A retrieval failure must not break the mode editor: return an empty
		// list with the reason, and the frontend silently stays on free input.
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "provider": provider, "models": []aiCatalogModel{}, "note": "не удалось получить список: " + err.Error()})
		return
	}
	body, _ := json.Marshal(map[string]any{"ok": true, "provider": provider, "models": models})
	h.c.adminAIModels.set(cacheKey, body, aiModelCatalogCacheTTL)
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body)
}

type geminiModelsResponse struct {
	Models []struct {
		Name                       string   `json:"name"`
		BaseModelID                string   `json:"baseModelId"`
		DisplayName                string   `json:"displayName"`
		InputTokenLimit            int64    `json:"inputTokenLimit"`
		SupportedGenerationMethods []string `json:"supportedGenerationMethods"`
		Thinking                   bool     `json:"thinking"`
	} `json:"models"`
}

// fetchGeminiModelCatalog: GET {geminiProxyRoot}/v1beta/models?key=...
// GeminiAPIBaseURL is configured for chat completions (it ends with
// /v1beta/openai, either directly at Google or through our Deno proxy,
// which forwards the path as is). The model list lives NOT under /openai/
// but directly under /v1beta/models, so we cut the known suffix to get the
// root (the Google domain or the proxy root with the secret) and rebuild the path.
func (h Handler) fetchGeminiModelCatalog(ctx context.Context, apiKey string) ([]aiCatalogModel, error) {
	root := strings.TrimSuffix(strings.TrimRight(h.GeminiAPIBaseURL, "/"), "/v1beta/openai")
	endpoint := root + "/v1beta/models?pageSize=200&key=" + apiKey
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	client := h.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("gemini models: статус %d", resp.StatusCode)
	}
	var parsed geminiModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, err
	}
	out := make([]aiCatalogModel, 0, len(parsed.Models))
	for _, m := range parsed.Models {
		supportsChat := false
		for _, method := range m.SupportedGenerationMethods {
			if method == "generateContent" {
				supportsChat = true
				break
			}
		}
		if !supportsChat {
			continue
		}
		id := strings.TrimPrefix(m.Name, "models/")
		out = append(out, aiCatalogModel{
			ID:               id,
			DisplayName:      m.DisplayName,
			SupportsThinking: m.Thinking,
			ContextWindow:    m.InputTokenLimit,
		})
	}
	return out, nil
}

type anthropicModelsResponse struct {
	Data []struct {
		ID           string `json:"id"`
		DisplayName  string `json:"display_name"`
		MaxInput     int64  `json:"max_input_tokens"`
		Capabilities struct {
			Thinking struct {
				Supported bool `json:"supported"`
			} `json:"thinking"`
		} `json:"capabilities"`
	} `json:"data"`
}

// fetchAnthropicModelCatalog: GET {anthropicBaseURL}/v1/models.
// AnthropicAPIBaseURL is the root (the Anthropic domain or the root of our
// Deno proxy with the secret); "/v1/models" is appended here, just as
// doAnthropicChat appends "/v1/messages".
func (h Handler) fetchAnthropicModelCatalog(ctx context.Context, apiKey string) ([]aiCatalogModel, error) {
	endpoint := strings.TrimRight(h.AnthropicAPIBaseURL, "/") + "/v1/models?limit=100"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", anthropicVersion)
	client := h.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("anthropic models: статус %d", resp.StatusCode)
	}
	var parsed anthropicModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, err
	}
	out := make([]aiCatalogModel, 0, len(parsed.Data))
	for _, m := range parsed.Data {
		out = append(out, aiCatalogModel{
			ID:               m.ID,
			DisplayName:      m.DisplayName,
			SupportsThinking: m.Capabilities.Thinking.Supported,
			ContextWindow:    m.MaxInput,
		})
	}
	return out, nil
}
