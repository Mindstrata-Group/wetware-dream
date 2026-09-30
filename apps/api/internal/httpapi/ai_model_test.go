package httpapi

import "testing"

func TestEffectiveLiveAIModel(t *testing.T) {
	t.Parallel()

	h := Handler{}
	cases := []struct {
		model string
		want  string
	}{
		{"", "gpt-4o-mini"},
		{"   ", "gpt-4o-mini"},
		{"gpt-4o", "gpt-4o"},
		{"  gpt-4o  ", "gpt-4o"},
		{"anthropic/claude-3-haiku", "anthropic/claude-3-haiku"},
	}
	for _, tc := range cases {
		if got := h.effectiveLiveAIModel(tc.model); got != tc.want {
			t.Errorf("effectiveLiveAIModel(%q) = %q, want %q", tc.model, got, tc.want)
		}
	}
}

func TestOpenAICompatibleModelAddsProviderPrefixForVseGPT(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		baseURL string
		model   string
		want    string
	}{
		{name: "vsegpt bare gpt", baseURL: "https://api.vsegpt.ru/v1", model: "gpt-4o-mini", want: "openai/gpt-4o-mini"},
		{name: "vsegpt prefixed stays", baseURL: "https://api.vsegpt.ru/v1", model: "anthropic/claude-3-haiku", want: "anthropic/claude-3-haiku"},
		{name: "direct openai stays", baseURL: "https://api.openai.com/v1", model: "gpt-4o-mini", want: "gpt-4o-mini"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := Handler{OpenAIBaseURL: tc.baseURL}
			if got := h.openAICompatibleModel(tc.model); got != tc.want {
				t.Fatalf("openAICompatibleModel(%q) = %q, want %q", tc.model, got, tc.want)
			}
		})
	}
}

func TestNormalizeAIModelID(t *testing.T) {
	t.Parallel()

	valid := []string{
		"google/gemini-2.5-flash-lite-pre-0925",
		"openai/gpt-4o-mini",
		"anthropic/claude-sonnet-4.5",
		"provider/model:v1@beta+tools",
	}
	for _, model := range valid {
		model := model
		t.Run("valid "+model, func(t *testing.T) {
			t.Parallel()
			got, err := normalizeAIModelID("  " + model + "  ")
			if err != nil {
				t.Fatalf("normalizeAIModelID(%q) err=%v", model, err)
			}
			if got != model {
				t.Fatalf("normalizeAIModelID(%q)=%q", model, got)
			}
		})
	}

	invalid := []string{
		"",
		"google gemini",
		"Работу я оцениваю на семь. Это не id модели.",
		"google/gemini\nopenai/gpt",
	}
	for _, model := range invalid {
		model := model
		t.Run("invalid "+model, func(t *testing.T) {
			t.Parallel()
			if got, err := normalizeAIModelID(model); err == nil {
				t.Fatalf("normalizeAIModelID(%q)=%q, want error", model, got)
			}
		})
	}
}
