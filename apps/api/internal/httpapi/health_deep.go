package httpapi

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// frontendErrorAlertEvents is the window of frontend JS errors for Kuma alerts.
// Incremented on POST /api/_error, read on GET /health/frontend-errors.
//
// Logic: if users generated >=1 JS error during the last frontendErrorAlertWindow,
// the endpoint returns 503 -> Kuma pushes an ntfy notification to the phone. A burst of errors
// inside the window = one alert (no spam).
//
// Error details (type, page, message, stack) are in Dozzle:
// api container logs -> filter "frontend error".
//
// This used to be an `atomic.Int64` reset on read (`Swap(0)`), the same
// "read with a side effect" trap that was fixed for the AI counter: two
// observers eat each other's events, and Kuma's own retry (maxretries=1)
// kills its own alert: the first failed request reset the counter, the retry
// saw 200 and the monitor silently went back to UP. The detailed analysis is in
// alert_window.go. Now reads are non-destructive, and all observers see the same
// fact.
var frontendErrorAlertEvents = newAlertWindow(alertWindowBucket, alertWindowMaxLookback)

// frontendErrorAlertWindow is how long the endpoint keeps returning 503
// after the last JS error.
//
// As with the AI window, it MUST be noticeably longer than Kuma's polling interval
// plus its retry_interval (monitor 20: 120s + 1x60s), otherwise the alert
// "dissolves" between the check and its retry.
//
// Overridden by the FRONTEND_ERRORS_ALERT_WINDOW_SECONDS env variable.
var frontendErrorAlertWindow = envAlertWindowSeconds("FRONTEND_ERRORS_ALERT_WINDOW_SECONDS", 5*time.Minute)

// logFrontendError sends a JS error to stdout (Dozzle picks it up), truncated
// to protect against spam. Including breadcrumbs (the user's last actions before the crash):
// one line in Dozzle, and the "frontend error" filter shows the whole chain.
func logFrontendError(r *http.Request, errType, message, page, stack string, breadcrumbs []map[string]interface{}) {
	if len(message) > 500 {
		message = message[:500]
	}
	if len(stack) > 400 {
		stack = stack[:400]
	}
	bcStr := formatBreadcrumbs(breadcrumbs)
	log.Printf("frontend error: type=%q page=%q msg=%q stack=%q remote=%s breadcrumbs=%s",
		errType, page, message, stack, r.RemoteAddr, bcStr)
}

// formatBreadcrumbs builds a compact log string from the breadcrumbs array.
// Example: "[click button.tab-modes 'Modes'; route /admin?tab=modes; fetch GET /api/admin/modes 200 35ms; click button.edit 'Edit']"
func formatBreadcrumbs(breadcrumbs []map[string]interface{}) string {
	if len(breadcrumbs) == 0 {
		return "[]"
	}
	parts := make([]string, 0, len(breadcrumbs))
	for _, bc := range breadcrumbs {
		t, _ := bc["type"].(string)
		data, _ := bc["data"].(map[string]interface{})
		switch t {
		case "click":
			sel, _ := data["selector"].(string)
			text, _ := data["text"].(string)
			if text != "" {
				parts = append(parts, fmt.Sprintf("click %s %q", sel, text))
			} else {
				parts = append(parts, fmt.Sprintf("click %s", sel))
			}
		case "route":
			to, _ := data["to"].(string)
			parts = append(parts, fmt.Sprintf("route %s", to))
		case "fetch":
			method, _ := data["method"].(string)
			url, _ := data["url"].(string)
			var status, ms float64
			if v, ok := data["status"].(float64); ok {
				status = v
			}
			if v, ok := data["ms"].(float64); ok {
				ms = v
			}
			parts = append(parts, fmt.Sprintf("fetch %s %s %.0f %dms", method, url, status, int(ms)))
		case "console":
			msg, _ := data["msg"].(string)
			parts = append(parts, fmt.Sprintf("console.error %q", msg))
		}
	}
	out := "[" + strings.Join(parts, "; ") + "]"
	if len(out) > 2000 {
		out = out[:2000] + "…]"
	}
	return out
}

// frontendErrorsTotal counts client errors reported by the frontend
// via POST /api/_error. Uptime-Kuma polls /metrics with a keyword monitor on
// "frontend_errors_total" and sends an alert when the value grows.
var frontendErrorsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "frontend_errors_total",
	Help: "Client-side JS errors reported by browser via window.onerror.",
}, []string{"type", "page"})

// aiErrorsTotal counts server-side AI errors (404 fallback, 429 retry exhausted, etc.)
// Incremented in ai_resilience via recordAIError().
var aiErrorsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "ai_errors_total",
	Help: "Server-side AI provider errors by HTTP status (post-retry).",
}, []string{"status", "model"})

// aiErrorAlertEvents is the window of AI errors for Kuma alerts.
//
// This used to be an `atomic.Int64` reset on read (`Swap(0)`), which
// broke as soon as more than one observer polled the endpoint (see the detailed
// analysis in alert_window.go). Now reads are non-destructive: an error keeps the
// endpoint "red" for aiErrorAlertWindow, and all observers see
// the same fact.
var aiErrorAlertEvents = newAlertWindow(alertWindowBucket, alertWindowMaxLookback)

// aiErrorAlertWindow is how long the endpoint keeps returning 503 after
// the last AI error.
//
// The window MUST be noticeably longer than Kuma's polling interval plus its
// retry_interval: a monitor with maxretries>=1 confirms a failure with a repeat
// request, and if the window has closed by then, Kuma decides everything
// is fixed and no alert goes out. With a 60s interval and a 60s retry, five minutes
// is plenty.
//
// Overridden by the AI_ERRORS_ALERT_WINDOW_SECONDS env variable.
var aiErrorAlertWindow = envAlertWindowSeconds("AI_ERRORS_ALERT_WINDOW_SECONDS", 5*time.Minute)

// envAlertWindowSeconds reads the window length from env (in seconds).
// Garbage and non-positive values are ignored; the default is used.
func envAlertWindowSeconds(name string, def time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return def
	}
	secs, err := strconv.Atoi(raw)
	if err != nil || secs <= 0 {
		log.Printf("health: %s=%q — не число секунд, беру значение по умолчанию %s", name, raw, def)
		return def
	}
	return time.Duration(secs) * time.Second
}

func recordAIError(model string, status int) {
	if model == "" {
		model = "unknown"
	}
	if len(model) > 64 {
		model = model[:64]
	}
	statusLabel := "0"
	if status > 0 {
		statusLabel = fmt.Sprintf("%d", status)
	}
	aiErrorsTotal.WithLabelValues(statusLabel, model).Inc()
	aiErrorAlertEvents.record()
}

// HealthDeep checks the real state of dependencies.
// Returns 200 if everything is OK, 503 + JSON with details otherwise.
// Uptime-Kuma monitors this endpoint and pushes to the phone on 503.
func (h Handler) HealthDeep(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")

	status := map[string]any{
		"ok":   true,
		"time": time.Now().UTC().Format(time.RFC3339),
	}
	failures := []string{}

	// Is the DB reachable?
	if h.DB == nil {
		failures = append(failures, "database not configured")
	} else {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := h.DB.Ping(ctx); err != nil {
			failures = append(failures, "database ping failed: "+err.Error())
		}
	}

	// Are the required secrets set?
	if strings.TrimSpace(h.PromoAdminSecret) == "" {
		failures = append(failures, "PROMO_ADMIN_SECRET not set")
	}
	if strings.TrimSpace(h.GuestCookieSecret) == "" {
		failures = append(failures, "GUEST_COOKIE_SECRET not set")
	}

	// Is the pool not exhausted?
	if h.DB != nil {
		stat := h.DB.Stat()
		status["pool"] = map[string]any{
			"acquired": stat.AcquiredConns(),
			"total":    stat.TotalConns(),
			"max":      stat.MaxConns(),
		}
		if stat.MaxConns() > 0 && float64(stat.AcquiredConns())/float64(stat.MaxConns()) > 0.9 {
			failures = append(failures, "pgxpool >90% utilized")
		}
	}

	if len(failures) > 0 {
		status["ok"] = false
		status["failures"] = failures
		writeJSON(w, http.StatusServiceUnavailable, status)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

// FrontendErrorReport accepts a JS error from the browser and increments the counter.
// The frontend sends it via apps/web/src/lib/errorReporter.ts from window.onerror.
// Body <=4KB, fields: type, message, page, stack (first 200 characters).
func (h Handler) FrontendErrorReport(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false})
		return
	}
	var req struct {
		Type        string                   `json:"type"`        // "error" | "unhandledrejection"
		Message     string                   `json:"message"`     // first 500 characters
		Page        string                   `json:"page"`        // pathname
		Stack       string                   `json:"stack"`       // first 200 characters
		Breadcrumbs []map[string]interface{} `json:"breadcrumbs"` // the user's last 30 actions
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10) // 16KB: breadcrumbs add extra weight
	if err := decodeJSONStrictBody(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false})
		return
	}
	errType := strings.TrimSpace(req.Type)
	if errType == "" {
		errType = "unknown"
	}
	page := strings.TrimSpace(req.Page)
	if page == "" {
		page = "unknown"
	}
	// Labels with bounded cardinality: page is normalised.
	if len(page) > 64 {
		page = page[:64]
	}
	frontendErrorsTotal.WithLabelValues(errType, page).Inc()
	frontendErrorAlertEvents.record()

	// Log to stdout, it ends up in Dozzle.
	logFrontendError(r, req.Type, req.Message, req.Page, req.Stack, req.Breadcrumbs)

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// AIErrorsAlertCheck is the endpoint for the Kuma uptime monitor "Prod / AI errors".
//
// 503 if there was at least one AI error during the last aiErrorAlertWindow
// (after retry+fallback), otherwise 200.
//
// Reads are NON-destructive and idempotent: however many observers hit the
// endpoint (Kuma, its own retry, a human with curl), they all see the same thing and do not
// eat each other's events. This used to be Swap(0), which left the second
// reader with nothing.
//
// Compatibility: the URL, response codes and the new_errors field are kept. Only
// the meaning of new_errors changed: "errors in the window" instead of "errors since the last
// read". For observers that want to compute deltas themselves, a
// monotonic total_errors field was added.
//
// The depth can be widened once with ?window_seconds=N (for manual
// investigation; clamped by alertWindowMaxLookback).
func (h Handler) AIErrorsAlertCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")

	window := aiErrorAlertWindow
	if raw := strings.TrimSpace(r.URL.Query().Get("window_seconds")); raw != "" {
		if secs, err := strconv.Atoi(raw); err == nil && secs > 0 {
			window = time.Duration(secs) * time.Second
		}
	}
	if window > alertWindowMaxLookback {
		window = alertWindowMaxLookback
	}

	snap := aiErrorAlertEvents.snapshot(window)
	body := map[string]any{
		"ok":             snap.Count == 0,
		"new_errors":     snap.Count,
		"window_seconds": int(window.Seconds()),
		"total_errors":   snap.Total,
	}
	if !snap.Last.IsZero() {
		body["last_error_ago_seconds"] = int(time.Since(snap.Last).Seconds())
	}
	if snap.Count > 0 {
		body["hint"] = "детали в логах контейнера api (Dozzle): фильтр 'live AI error'"
		writeJSON(w, http.StatusServiceUnavailable, body)
		return
	}
	writeJSON(w, http.StatusOK, body)
}

// FrontendErrorsAlertCheck is the endpoint for the Kuma uptime monitor
// "Staging / Frontend JS errors" (id 20).
//
// 503 if users generated at least one JS error during the last frontendErrorAlertWindow,
// otherwise 200.
//
// Reads are NON-destructive and idempotent: however many observers hit the
// endpoint (Kuma, its own retry, a human with curl), they all see the same thing and do not
// eat each other's events. This used to be Swap(0), which left the second
// reader with nothing, and the alert never arrived.
//
// Compatibility: the URL, response codes and the new_errors field are kept, the monitor
// does not need reconfiguring. Only the meaning of new_errors changed: "errors in the window" instead of
// "errors since the last read". For observers that compute deltas themselves,
// a monotonic total_errors field was added.
//
// The depth can be widened once with ?window_seconds=N (for manual
// investigation; clamped by alertWindowMaxLookback).
func (h Handler) FrontendErrorsAlertCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")

	window := frontendErrorAlertWindow
	if raw := strings.TrimSpace(r.URL.Query().Get("window_seconds")); raw != "" {
		if secs, err := strconv.Atoi(raw); err == nil && secs > 0 {
			window = time.Duration(secs) * time.Second
		}
	}
	if window > alertWindowMaxLookback {
		window = alertWindowMaxLookback
	}

	snap := frontendErrorAlertEvents.snapshot(window)
	body := map[string]any{
		"ok":             snap.Count == 0,
		"new_errors":     snap.Count,
		"window_seconds": int(window.Seconds()),
		"total_errors":   snap.Total,
		// The field remains for those who read the old response: it is now always
		// false, since reading no longer resets anything.
		"resets_on_call": false,
	}
	if !snap.Last.IsZero() {
		body["last_error_ago_seconds"] = int(time.Since(snap.Last).Seconds())
	}
	if snap.Count > 0 {
		body["hint"] = "детали в логах контейнера api (Dozzle): фильтр 'frontend error'"
		writeJSON(w, http.StatusServiceUnavailable, body)
		return
	}
	writeJSON(w, http.StatusOK, body)
}
