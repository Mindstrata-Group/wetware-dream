//go:build !integration

// These tests run in the unit build (the api-unit job `go test ./...` and api-race
// `go test -race ./...`) and are deliberately excluded from the integration
// build: they swap the global frontend error window, while integration tests
// run in parallel and really call POST /api/_error, which would race on a
// package-level variable. In the unit build no other test touches that path,
// so the swap is safe.

package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// withIsolatedFrontendErrorWindow swaps the global frontend error window for a
// fresh one with a controlled clock and returns a pointer to "now", which the
// test moves itself. Restored via t.Cleanup.
func withIsolatedFrontendErrorWindow(t *testing.T, window time.Duration) *time.Time {
	t.Helper()

	prevEvents := frontendErrorAlertEvents
	prevWindow := frontendErrorAlertWindow

	fresh := newAlertWindow(alertWindowBucket, alertWindowMaxLookback)
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	clock := &now
	fresh.now = func() time.Time { return *clock }

	frontendErrorAlertEvents = fresh
	frontendErrorAlertWindow = window
	t.Cleanup(func() {
		frontendErrorAlertEvents = prevEvents
		frontendErrorAlertWindow = prevWindow
	})
	return clock
}

// reportFrontendError calls POST /api/_error directly (no DB needed): the same
// path the browser reporter uses.
func reportFrontendError(t *testing.T, message string) {
	t.Helper()
	body := `{"type":"error","message":"` + message + `","page":"/chat","stack":"at foo()"}`
	req := httptest.NewRequest(http.MethodPost, "/api/_error", strings.NewReader(body))
	rec := httptest.NewRecorder()
	Handler{}.FrontendErrorReport(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/_error: хотели 200, получили %d (%s)", rec.Code, rec.Body.String())
	}
}

// callFrontendErrorsCheck calls the alert endpoint and returns code + body.
func callFrontendErrorsCheck(t *testing.T, query string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/health/frontend-errors"+query, nil)
	rec := httptest.NewRecorder()
	Handler{}.FrontendErrorsAlertCheck(rec, req)

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("ответ не JSON: %v (%s)", err, rec.Body.String())
	}
	return rec.Code, body
}

// The essence of the fix: two consecutive observers see THE SAME error, not half
// each. This is exactly the Kuma scenario with maxretries=1: the first request
// fails, the retry confirms the failure, and only then ntfy fires. On the old
// code (frontendErrorsSinceLastCheck.Swap(0)) the second request got 200 and
// new_errors=0, the monitor silently went back to UP, and no alert fired at all.
func TestFrontendErrorsAlertCheck_TwoObserversSeeSameErrors(t *testing.T) {
	withIsolatedFrontendErrorWindow(t, time.Minute)

	reportFrontendError(t, "TypeError: undefined is not a function")

	// Observer #1: Uptime Kuma.
	status1, body1 := callFrontendErrorsCheck(t, "")
	if status1 != http.StatusServiceUnavailable {
		t.Fatalf("наблюдатель №1: хотели 503, получили %d (%v)", status1, body1)
	}

	// Observer #2: a retry of the same Kuma (or a person with curl).
	status2, body2 := callFrontendErrorsCheck(t, "")
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
	// The old-format field remains, but now honestly says "not resetting".
	if resets, _ := body2["resets_on_call"].(bool); resets {
		t.Fatalf("resets_on_call должен быть false: %v", body2)
	}
}

// The window expires by time, not by number of reads: an observer arriving later
// sees the same error while the window is alive.
func TestFrontendErrorsAlertCheck_WindowExpiresByTimeNotByReads(t *testing.T) {
	clock := withIsolatedFrontendErrorWindow(t, 5*time.Minute)

	reportFrontendError(t, "unhandledrejection: 500 from /api/chat")

	for i := 0; i < 5; i++ {
		if status, body := callFrontendErrorsCheck(t, ""); status != http.StatusServiceUnavailable {
			t.Fatalf("чтение №%d: хотели 503, получили %d (%v)", i+1, status, body)
		}
		*clock = clock.Add(30 * time.Second) // 2.5 minutes in total: the window is still open
	}

	// The window expired: the endpoint turned green by itself, nobody had to "reset" anything.
	*clock = clock.Add(6 * time.Minute)
	status, body := callFrontendErrorsCheck(t, "")
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

func TestFrontendErrorsAlertCheck_NoErrors200(t *testing.T) {
	withIsolatedFrontendErrorWindow(t, time.Minute)

	status, body := callFrontendErrorsCheck(t, "")
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
func TestFrontendErrorsAlertCheck_WindowSecondsQueryParam(t *testing.T) {
	clock := withIsolatedFrontendErrorWindow(t, 30*time.Second)

	reportFrontendError(t, "ReferenceError: x is not defined")
	*clock = clock.Add(2 * time.Minute)

	// The default window is 30s: the error is already outside it.
	if status, body := callFrontendErrorsCheck(t, ""); status != http.StatusOK {
		t.Fatalf("дефолтное окно: хотели 200, получили %d (%v)", status, body)
	}
	// Manual inspection with a wider window: the error is visible.
	status, body := callFrontendErrorsCheck(t, "?window_seconds=600")
	if status != http.StatusServiceUnavailable {
		t.Fatalf("?window_seconds=600: хотели 503, получили %d (%v)", status, body)
	}
	// Clamped to the ring maximum (15 minutes).
	_, body = callFrontendErrorsCheck(t, "?window_seconds=99999")
	if got := mustInt64(t, body, "window_seconds"); got != int64(alertWindowMaxLookback.Seconds()) {
		t.Fatalf("window_seconds = %d, хотели кламп до %.0f", got, alertWindowMaxLookback.Seconds())
	}
}

// Several errors in a row within the window = one alert, but the counter does not lose them.
func TestFrontendErrorsAlertCheck_CountsAllErrorsInWindow(t *testing.T) {
	withIsolatedFrontendErrorWindow(t, 5*time.Minute)

	for i := 0; i < 3; i++ {
		reportFrontendError(t, "boom")
	}
	status, body := callFrontendErrorsCheck(t, "")
	if status != http.StatusServiceUnavailable {
		t.Fatalf("хотели 503, получили %d (%v)", status, body)
	}
	if got := mustInt64(t, body, "new_errors"); got != 3 {
		t.Fatalf("new_errors = %d, хотели 3", got)
	}
	if _, ok := body["last_error_ago_seconds"]; !ok {
		t.Fatalf("в ответе нет last_error_ago_seconds: %v", body)
	}
}
