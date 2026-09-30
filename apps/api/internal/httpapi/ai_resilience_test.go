package httpapi

import (
	"errors"
	"testing"
)

// Unit tests for helpers, no DB.

func TestLiveAIErrorStatus_NilErrorReturnsZero(t *testing.T) {
	t.Parallel()

	if got := liveAIErrorStatus(nil); got != 0 {
		t.Errorf("nil error: got %d, want 0", got)
	}
}

func TestLiveAIErrorStatus_PlainErrorReturnsZero(t *testing.T) {
	t.Parallel()

	if got := liveAIErrorStatus(errors.New("oops")); got != 0 {
		t.Errorf("plain error: got %d, want 0", got)
	}
}

func TestLiveAIErrorStatus_WrappedLiveAIError(t *testing.T) {
	t.Parallel()

	src := &liveAIError{
		Message: "test",
		Debug:   liveAIDebugInfo{Status: 429},
	}
	if got := liveAIErrorStatus(src); got != 429 {
		t.Errorf("liveAIError(429): got %d, want 429", got)
	}
}

func TestLiveAIErrorStatus_404(t *testing.T) {
	t.Parallel()

	src := &liveAIError{Debug: liveAIDebugInfo{Status: 404}}
	if got := liveAIErrorStatus(src); got != 404 {
		t.Errorf("404: got %d", got)
	}
}
