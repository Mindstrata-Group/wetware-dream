package httpapi

import (
	"errors"
	"strings"
	"testing"
)

// Unit tests for the AI runtime: no DB/HTTP, pure functions.

func TestBuildXTitle_Format(t *testing.T) {
	t.Parallel()

	got := buildXTitle(42, 7, "TEXT")
	want := "Mindstrata_USER_7_MODE_42_TEXT"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestSanitizeXTitle_StripsBadChars(t *testing.T) {
	t.Parallel()

	cases := []struct{ in, want string }{
		{"hello world", "helloworld"},
		{"abc123_-.", "abc123_-."},
		{"раздел!@#$%^&*()", "Mindstrata"},                 // only non-ASCII non-allowed → fallback
		{"", "Mindstrata"},                                 // empty → fallback
		{strings.Repeat("a", 80), strings.Repeat("a", 50)}, // truncated to 50
	}
	for _, c := range cases {
		got := sanitizeXTitle(c.in)
		if got != c.want {
			t.Errorf("sanitizeXTitle(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func FuzzSanitizeXTitle(f *testing.F) {
	for _, s := range []string{"normal", "spaces and stuff", "русский текст",
		strings.Repeat("x", 200), "abc!@#", "_._.-.-_"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, in string) {
		out := sanitizeXTitle(in)
		if out == "" {
			t.Errorf("empty output for input %q", in)
		}
		if len(out) > 50 {
			t.Errorf("too long: %d for input %q → %q", len(out), in, out)
		}
		for _, r := range out {
			ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
				(r >= '0' && r <= '9') || r == '_' || r == '-' || r == '.'
			if !ok {
				t.Errorf("bad char %q in output for input %q → %q", r, in, out)
			}
		}
	})
}

func TestMessagesText_EmptyAndJoin(t *testing.T) {
	t.Parallel()

	if got := messagesText(nil); got != "" {
		t.Errorf("empty input: got %q want \"\"", got)
	}
	got := messagesText([]map[string]string{
		{"role": "system", "content": "you are a helper"},
		{"role": "user", "content": "hi"},
	})
	if !strings.Contains(got, "you are a helper") || !strings.Contains(got, "hi") {
		t.Errorf("missing content: %q", got)
	}
}

func TestNullableCost(t *testing.T) {
	t.Parallel()

	if nullableCost(0) != nil {
		t.Errorf("0 must be nil (not stored)")
	}
	v := 0.05
	got := nullableCost(v)
	if got == nil || *got != v {
		t.Errorf("0.05: got %v want pointer to 0.05", got)
	}
}

func TestFirstPositiveFloat(t *testing.T) {
	t.Parallel()

	if got := firstPositiveFloat(0, 0, 0); got != 0 {
		t.Errorf("all zero: got %v want 0", got)
	}
	if got := firstPositiveFloat(0, 0.5, 1.0); got != 0.5 {
		t.Errorf("first positive: got %v want 0.5", got)
	}
	if got := firstPositiveFloat(-1, 0, 2.5); got != 2.5 {
		t.Errorf("skip negatives and zeros: got %v want 2.5", got)
	}
}

func TestEffectiveLiveAIModel_Empty(t *testing.T) {
	t.Parallel()

	h := Handler{}
	if got := h.effectiveLiveAIModel(""); got == "" {
		t.Errorf("empty input should fall back to default model, got empty")
	}
}

func TestOpenAICompatibleModel_VseGPTPrefix(t *testing.T) {
	t.Parallel()

	h := Handler{OpenAIBaseURL: "https://api.vsegpt.ru/v1"}
	got := h.openAICompatibleModel("gpt-4o-mini")
	// VseGPT requires provider prefix for plain model names.
	if !strings.Contains(got, "/") {
		t.Errorf("VseGPT model should have provider prefix: got %q", got)
	}
}

func TestOpenAICompatibleModel_NoPrefixForNonVseGPT(t *testing.T) {
	t.Parallel()

	h := Handler{OpenAIBaseURL: "https://api.openai.com/v1"}
	got := h.openAICompatibleModel("gpt-4o-mini")
	if strings.Contains(got, "/") {
		t.Errorf("non-VseGPT model should not get prefix: got %q", got)
	}
}

func TestLiveAIError_ImplementsError(t *testing.T) {
	t.Parallel()

	err := &liveAIError{Message: "test error"}
	if err.Error() != "test error" {
		t.Errorf("Error() = %q want \"test error\"", err.Error())
	}
}

func TestLiveAIDebugFromError_WrappedError(t *testing.T) {
	t.Parallel()

	debug := liveAIDebugInfo{Provider: "openai-compatible", URL: "test"}
	err := &liveAIError{Message: "wrapped", Debug: debug}
	got, ok := liveAIDebugFromError(err)
	if !ok {
		t.Fatalf("expected ok=true for liveAIError")
	}
	if got.Provider != "openai-compatible" {
		t.Errorf("Provider: got %q", got.Provider)
	}
}

func TestLiveAIDebugFromError_PlainError(t *testing.T) {
	t.Parallel()

	_, ok := liveAIDebugFromError(errors.New("plain error"))
	if ok {
		t.Errorf("plain error should return false")
	}
}
