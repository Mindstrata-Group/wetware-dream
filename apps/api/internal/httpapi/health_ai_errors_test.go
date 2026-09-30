//go:build !integration

// These tests run in the unit build (the api-unit job `go test ./...` and api-race
// `go test -race ./...`) and are deliberately excluded from the integration
// build: they swap the global AI error window, while integration tests run in
// parallel and really call doAIChatResilientSpec → recordAIError. In the unit
// build no other test touches that path, so the swap is safe.

package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// withIsolatedAIErrorWindow swaps the global AI error window for a fresh one with
// a controlled clock and returns a pointer to "now", which the test moves itself.
// Restored via t.Cleanup.
func withIsolatedAIErrorWindow(t *testing.T, window time.Duration) *time.Time {
	t.Helper()

	prevEvents := aiErrorAlertEvents
	prevWindow := aiErrorAlertWindow

	fresh := newAlertWindow(alertWindowBucket, alertWindowMaxLookback)
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	clock := &now
	fresh.now = func() time.Time { return *clock }

	aiErrorAlertEvents = fresh
	aiErrorAlertWindow = window
	t.Cleanup(func() {
		aiErrorAlertEvents = prevEvents
		aiErrorAlertWindow = prevWindow
	})
	return clock
}

// callAIErrorsCheck calls the endpoint directly (no DB needed) and returns code + body.
func callAIErrorsCheck(t *testing.T, query string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/health/ai-errors"+query, nil)
	rec := httptest.NewRecorder()
	Handler{}.AIErrorsAlertCheck(rec, req)

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("ответ не JSON: %v (%s)", err, rec.Body.String())
	}
	return rec.Code, body
}

func mustInt64(t *testing.T, body map[string]any, key string) int64 {
	t.Helper()
	v, ok := body[key].(float64)
	if !ok {
		t.Fatalf("в ответе нет числового поля %q: %v", key, body)
	}
	return int64(v)
}

// The essence of the fix: two independent observers in a row see THE SAME
// events, not half each. On the old code (aiErrorsSinceLastCheck.Swap(0)) the
// second request got 200 and new_errors=0, and the test failed.
func TestAIErrorsAlertCheck_TwoObserversSeeSameErrors(t *testing.T) {
	withIsolatedAIErrorWindow(t, time.Minute)

	recordAIError("anthropic/claude-opus-5", 429)

	// Observer #1: Uptime Kuma.
	status1, body1 := callAIErrorsCheck(t, "")
	if status1 != http.StatusServiceUnavailable {
		t.Fatalf("наблюдатель №1: хотели 503, получили %d (%v)", status1, body1)
	}

	// Observer #2: a person with curl (or a retry of the same Kuma).
	status2, body2 := callAIErrorsCheck(t, "")
	if status2 != http.StatusServiceUnavailable {
		t.Fatalf("наблюдатель №2 не увидел ошибку, съеденную первым чтением: %d (%v)", status2, body2)
	}

	n1 := mustInt64(t, body1, "new_errors")
	n2 := mustInt64(t, body2, "new_errors")
	if n1 != 1 || n2 != 1 {
		t.Fatalf("оба должны видеть ровно 1 ошибку, а видят %d и %d", n1, n2)
	}
	if got := mustInt64(t, body2, "total_errors"); got != 1 {
		t.Fatalf("монотонный total_errors = %d, хотели 1", got)
	}
	if ok, _ := body2["ok"].(bool); ok {
		t.Fatalf("ok должен быть false при ошибке в окне: %v", body2)
	}
}

// A third observer arriving later also sees the same error while the window is alive.
func TestAIErrorsAlertCheck_WindowExpiresByTimeNotByReads(t *testing.T) {
	clock := withIsolatedAIErrorWindow(t, 5*time.Minute)

	recordAIError("openai/gpt-5", 500)

	for i := 0; i < 5; i++ {
		if status, body := callAIErrorsCheck(t, ""); status != http.StatusServiceUnavailable {
			t.Fatalf("чтение №%d: хотели 503, получили %d (%v)", i+1, status, body)
		}
		*clock = clock.Add(30 * time.Second) // 2.5 minutes in total: the window is still open
	}

	// The window expired: the endpoint turned green by itself, nobody had to "reset" anything.
	*clock = clock.Add(6 * time.Minute)
	status, body := callAIErrorsCheck(t, "")
	if status != http.StatusOK {
		t.Fatalf("после истечения окна хотели 200, получили %d (%v)", status, body)
	}
	if got := mustInt64(t, body, "new_errors"); got != 0 {
		t.Fatalf("new_errors после истечения окна = %d, хотели 0", got)
	}
	// The monotonic counter is never reset: deltas are computed from it.
	if got := mustInt64(t, body, "total_errors"); got != 1 {
		t.Fatalf("total_errors = %d, хотели 1 (монотонный счётчик не сбрасывается)", got)
	}
}

func TestAIErrorsAlertCheck_NoErrors200(t *testing.T) {
	withIsolatedAIErrorWindow(t, time.Minute)

	status, body := callAIErrorsCheck(t, "")
	if status != http.StatusOK {
		t.Fatalf("хотели 200, получили %d (%v)", status, body)
	}
	if got := mustInt64(t, body, "new_errors"); got != 0 {
		t.Fatalf("new_errors = %d, хотели 0", got)
	}
	if got := mustInt64(t, body, "window_seconds"); got != 60 {
		t.Fatalf("window_seconds = %d, хотели 60", got)
	}
}

// ?window_seconds= widens the inspection depth and is clamped to the ring maximum.
func TestAIErrorsAlertCheck_WindowSecondsQueryParam(t *testing.T) {
	clock := withIsolatedAIErrorWindow(t, 30*time.Second)

	recordAIError("gemini/gemini-3-pro", 503)
	*clock = clock.Add(2 * time.Minute)

	// The default window is 30s: the error is already outside it.
	if status, body := callAIErrorsCheck(t, ""); status != http.StatusOK {
		t.Fatalf("дефолтное окно: хотели 200, получили %d (%v)", status, body)
	}
	// Manual inspection with a wider window: the error is visible.
	status, body := callAIErrorsCheck(t, "?window_seconds=600")
	if status != http.StatusServiceUnavailable {
		t.Fatalf("?window_seconds=600: хотели 503, получили %d (%v)", status, body)
	}
	// Clamped to the ring maximum (15 minutes).
	_, body = callAIErrorsCheck(t, "?window_seconds=99999")
	if got := mustInt64(t, body, "window_seconds"); got != int64(alertWindowMaxLookback.Seconds()) {
		t.Fatalf("window_seconds = %d, хотели кламп до %.0f", got, alertWindowMaxLookback.Seconds())
	}
}

func TestAlertWindow_CountsOnlyEventsInsideWindow(t *testing.T) {
	w := newAlertWindow(alertWindowBucket, alertWindowMaxLookback)
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	w.now = func() time.Time { return now }

	w.record()
	now = now.Add(time.Minute)
	w.record()
	w.record()

	// The first event was 60s ago: it no longer falls into the 30-second window.
	if got := w.snapshot(30 * time.Second).Count; got != 2 {
		t.Fatalf("в окне 30с должно быть 2 события, а не %d", got)
	}
	if got := w.snapshot(5 * time.Minute).Count; got != 3 {
		t.Fatalf("в окне 5м должно быть 3 события, а не %d", got)
	}
	if got := w.snapshot(time.Minute).Total; got != 3 {
		t.Fatalf("total = %d, хотели 3", got)
	}

	// The ring is reused: old buckets must not "come back to life".
	now = now.Add(alertWindowMaxLookback + time.Minute)
	if got := w.snapshot(alertWindowMaxLookback).Count; got != 0 {
		t.Fatalf("после полного оборота кольца в окне должно быть 0, а не %d", got)
	}
}

// No races: this matters for the api-race job (go test -race ./...).
func TestAlertWindow_ConcurrentRecord(t *testing.T) {
	w := newAlertWindow(alertWindowBucket, alertWindowMaxLookback)

	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				w.record()
				_ = w.snapshot(time.Minute)
			}
		}()
	}
	wg.Wait()

	if got := w.snapshot(time.Minute).Total; got != 800 {
		t.Fatalf("total = %d, хотели 800", got)
	}
	if got := w.snapshot(time.Minute).Count; got != 800 {
		t.Fatalf("count в окне = %d, хотели 800", got)
	}
}
