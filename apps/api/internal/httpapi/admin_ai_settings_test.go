package httpapi

import (
	"context"
	"testing"
)

func TestSystemSetting_NilDBReturnsEmptyWithoutPanic(t *testing.T) {
	t.Parallel()

	got, err := Handler{}.systemSetting(context.Background(), "ai_queue_enabled")
	if err != nil {
		t.Fatalf("systemSetting nil DB err=%v", err)
	}
	if got != "" {
		t.Fatalf("systemSetting nil DB=%q want empty", got)
	}
}
