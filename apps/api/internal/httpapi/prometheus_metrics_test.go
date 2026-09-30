package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMetricsRequiresBasicAuth(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()

	Handler{MetricsBasicAuth: "user:pass"}.Metrics(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
	if got := w.Header().Get("WWW-Authenticate"); !strings.Contains(got, "Basic") {
		t.Fatalf("WWW-Authenticate = %q, want Basic challenge", got)
	}
}

func TestMetricsReturnsPrometheusFormatWithBasicAuth(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.SetBasicAuth("user", "pass")
	w := httptest.NewRecorder()

	Handler{MetricsBasicAuth: "user:pass"}.Metrics(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", w.Code, http.StatusOK, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "cache_hit_ratio") {
		t.Fatalf("metrics body does not contain cache_hit_ratio: %.200q", w.Body.String())
	}
}

func TestNormalizeMetricsPathBoundsDynamicLabels(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"/api/auth/oauth/yandex/callback":    "/api/auth/oauth/",
		"/api/admin/users/123":               "/api/admin/users/",
		"/api/admin/dialogs/456/messages":    "/api/admin/dialogs/",
		"/api/tester/users/789/chat-preview": "/api/tester/users/",
		"/webhooks/yookassa/secret-path":     "/webhooks/*",
		"/health":                            "/health",
	}

	for path, want := range cases {
		if got := normalizeMetricsPath(path); got != want {
			t.Fatalf("normalizeMetricsPath(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestRouterAddsRequestIDAndRecordsNormalizedMetricsPath(t *testing.T) {
	t.Parallel()

	router := NewRouter(Handler{MetricsBasicAuth: "user:pass"}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/users/123", nil)
	req.Header.Set(requestIDHeader, "ci-trace-1")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if got := w.Header().Get(requestIDHeader); got != "ci-trace-1" {
		t.Fatalf("request id header = %q, want ci-trace-1", got)
	}

	metricsReq := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	metricsReq.SetBasicAuth("user", "pass")
	metricsW := httptest.NewRecorder()
	router.ServeHTTP(metricsW, metricsReq)

	body := metricsW.Body.String()
	if !strings.Contains(body, `path="/api/admin/users/",status="401"`) {
		t.Fatalf("metrics body does not contain normalized admin users label: %.500q", body)
	}
	if strings.Contains(body, `path="/api/admin/users/123"`) {
		t.Fatalf("metrics body contains high-cardinality user id label: %.500q", body)
	}
}
