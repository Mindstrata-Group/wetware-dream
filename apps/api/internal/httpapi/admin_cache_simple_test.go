package httpapi

import (
	"net/http/httptest"
	"testing"
	"time"
)

// Unit tests for simpleGlobalCache. This helper backs 6 configuration
// admin endpoints (ai-settings, tariff-groups, summary-prompts,
// orchestration-prompt, dialog-summary-prompt, modes/model-stats).
// Regression tests: without them a broken clear() or a set/get race is
// easy to miss.

func TestSimpleGlobalCache_GetBeforeSetReturnsNil(t *testing.T) {
	t.Parallel()

	var c simpleGlobalCache
	if c.get() != nil {
		t.Errorf("fresh cache returned non-nil body")
	}
}

func TestSimpleGlobalCache_SetThenGet(t *testing.T) {
	t.Parallel()

	var c simpleGlobalCache
	body := []byte(`{"ok":true}`)
	c.set(body, time.Hour)
	got := c.get()
	if string(got) != string(body) {
		t.Errorf("get returned %q, want %q", got, body)
	}
}

func TestSimpleGlobalCache_ExpiredReturnsNil(t *testing.T) {
	t.Parallel()

	var c simpleGlobalCache
	c.set([]byte("data"), 0)
	time.Sleep(time.Millisecond)
	if c.get() != nil {
		t.Errorf("expired cache still returned body")
	}
}

func TestSimpleGlobalCache_ClearWipes(t *testing.T) {
	t.Parallel()

	var c simpleGlobalCache
	c.set([]byte("data"), time.Hour)
	c.clear()
	if c.get() != nil {
		t.Errorf("clear did not wipe body")
	}
}

func TestSimpleGlobalCache_WriteCachedFastPath(t *testing.T) {
	t.Parallel()

	var c simpleGlobalCache
	c.set([]byte(`{"cached":true}`), time.Hour)
	w := httptest.NewRecorder()
	if !writeCached(w, &c) {
		t.Errorf("writeCached returned false on populated cache")
	}
	if w.Code != 200 {
		t.Errorf("status: %d", w.Code)
	}
	if got := w.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type: %q", got)
	}
	if w.Body.String() != `{"cached":true}` {
		t.Errorf("body: %q", w.Body.String())
	}
}

func TestSimpleGlobalCache_WriteCachedMissReturnsFalse(t *testing.T) {
	t.Parallel()

	var c simpleGlobalCache
	w := httptest.NewRecorder()
	if writeCached(w, &c) {
		t.Errorf("writeCached returned true on empty cache")
	}
}
