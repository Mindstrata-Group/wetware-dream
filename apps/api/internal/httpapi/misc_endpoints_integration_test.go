//go:build integration

package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mindstrata-stage1/api/internal/testsupport"
)

// =============================================================================
// ExpertStatus
// =============================================================================

func TestExpertStatus_OK_AsExpert(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	expert := f.CreateUser(TestUserOpts{Role: "expert"})
	ts.LoginAs(f.CreateSession(expert.ID))

	status, body := httpJSON(t, ts, "GET", "/api/expert/status", nil)
	if status != http.StatusOK {
		t.Fatalf("expert GET: %d body=%v", status, body)
	}
	if body["ok"] != true {
		t.Errorf("ok!=true: %v", body)
	}
	if _, ok := body["user"].(map[string]any); !ok {
		t.Errorf("user field missing: %v", body)
	}
}

func TestExpertStatus_OK_AsAdmin(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	status, _ := httpJSON(t, ts, "GET", "/api/expert/status", nil)
	if status != http.StatusOK {
		t.Fatalf("admin GET expert: %d", status)
	}
}

func TestExpertStatus_RegularUser_403(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	user := f.CreateUser(TestUserOpts{Role: "user"})
	ts.LoginAs(f.CreateSession(user.ID))

	status, _ := httpJSON(t, ts, "GET", "/api/expert/status", nil)
	if status != http.StatusForbidden && status != http.StatusUnauthorized {
		t.Fatalf("user GET expert: want 403/401, got %d", status)
	}
}

func TestExpertStatus_POST_405(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	expert := f.CreateUser(TestUserOpts{Role: "expert"})
	ts.LoginAs(f.CreateSession(expert.ID))

	status, _ := httpJSON(t, ts, "POST", "/api/expert/status", nil)
	if status != http.StatusMethodNotAllowed {
		t.Fatalf("POST expert: want 405, got %d", status)
	}
}

// =============================================================================
// CookieConsent
// =============================================================================

func TestCookieConsent_OK_GuestRecord(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)

	consentID := "test-consent-" + uniqueEmail("ck") // unique ID

	status, body := httpJSON(t, ts, "POST", "/api/cookie-consent", map[string]any{
		"consentId":  consentID,
		"sourcePath": "/",
	})
	if status != http.StatusOK {
		t.Fatalf("cookie consent: %d body=%v", status, body)
	}

	// The record is saved in the DB
	var cnt int64
	_ = env.Pool.QueryRow(t.Context(),
		`select count(*) from cookie_consents where consent_id = $1`, consentID).Scan(&cnt)
	if cnt != 1 {
		t.Errorf("cookie_consents row missing, count=%d", cnt)
	}
}

func TestCookieConsent_EmptyConsentID_400(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)
	_ = env

	status, _ := httpJSON(t, ts, "POST", "/api/cookie-consent", map[string]any{
		"consentId": "",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("empty consentId: want 400, got %d", status)
	}
}

func TestCookieConsent_GET_405(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)
	_ = env

	status, _ := httpJSON(t, ts, "GET", "/api/cookie-consent", nil)
	if status != http.StatusMethodNotAllowed {
		t.Fatalf("GET cookie-consent: want 405, got %d", status)
	}
}

// A repeated POST with the same consentID updates the record (upsert), not duplicates it.
func TestCookieConsent_Upsert_NoDuplicate(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)

	consentID := "upsert-consent-" + uniqueEmail("up")

	for i := 0; i < 2; i++ {
		status, _ := httpJSON(t, ts, "POST", "/api/cookie-consent", map[string]any{
			"consentId": consentID,
		})
		if status != http.StatusOK {
			t.Fatalf("iter %d: %d", i, status)
		}
	}

	var cnt int64
	_ = env.Pool.QueryRow(t.Context(),
		`select count(*) from cookie_consents where consent_id = $1`, consentID).Scan(&cnt)
	if cnt != 1 {
		t.Errorf("upsert should yield exactly 1 row, got %d", cnt)
	}
}

// =============================================================================
// FrontendErrorReport
// =============================================================================

func TestFrontendErrorReport_OK(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)
	_ = env

	status, body := httpJSON(t, ts, "POST", "/api/_error", map[string]any{
		"type":    "error",
		"message": "Cannot read property 'x' of undefined",
		"page":    "/chat",
		"stack":   "Error at ChatPage.tsx:42",
	})
	if status != http.StatusOK {
		t.Fatalf("frontend error report: %d body=%v", status, body)
	}
	if body["ok"] != true {
		t.Errorf("ok!=true: %v", body)
	}
}

func TestFrontendErrorReport_RejectsOversizedPayload(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)
	_ = env

	status, _ := httpJSON(t, ts, "POST", "/api/_error", strings.Repeat("x", 17<<10))
	if status != http.StatusBadRequest {
		t.Fatalf("oversized frontend error report: want 400, got %d", status)
	}
}

func TestFrontendErrorReport_GET_405(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)
	_ = env

	status, _ := httpJSON(t, ts, "GET", "/api/_error", nil)
	if status != http.StatusMethodNotAllowed {
		t.Fatalf("GET _error: want 405, got %d", status)
	}
}

// =============================================================================
// HealthDeep
// =============================================================================

func TestHealthDeep_DBConnected_200(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServerWithHandler(t, env.Pool, Handler{
		PromoAdminSecret:  "test-secret",
		GuestCookieSecret: "test-guest-secret",
	})

	status, body := httpJSON(t, ts, "GET", "/health/deep", nil)
	if status != http.StatusOK {
		t.Fatalf("health/deep: %d body=%v", status, body)
	}
	if body["ok"] != true {
		t.Errorf("ok!=true: %v", body)
	}
}

func TestHealthDeep_NoStoreHeader(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	router := NewRouter(Handler{
		DB:                env.Pool,
		PromoAdminSecret:  "test-secret",
		GuestCookieSecret: "test-guest-secret",
	}, nil)
	req := httptest.NewRequest(http.MethodGet, "/health/deep", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("health/deep: %d", w.Code)
	}
	if got := w.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}
}

func TestHealthDeep_MissingSecrets_503(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	// Handler without secrets: HealthDeep must return 503.
	ts := NewTestServerWithHandler(t, env.Pool, Handler{})

	status, body := httpJSON(t, ts, "GET", "/health/deep", nil)
	if status != http.StatusServiceUnavailable {
		t.Fatalf("health/deep no secrets: want 503, got %d body=%v", status, body)
	}
}

// =============================================================================
// AIErrorsAlertCheck + FrontendErrorsAlertCheck
// =============================================================================

func TestAIErrorsAlertCheck_NoErrors_200(t *testing.T) {
	// The global atomic counter of the alert window is not isolated between parallel tests.
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)
	_ = env

	// Reading the endpoint NO LONGER resets the window (that is the point of the fix),
	// so we clear what other tests accumulated directly.
	aiErrorAlertEvents.reset()

	status, body := httpJSON(t, ts, "GET", "/health/ai-errors", nil)
	if status != http.StatusOK {
		t.Fatalf("ai-errors no errors: %d body=%v", status, body)
	}
	// A repeated read must give the same result: it is idempotent.
	status, body = httpJSON(t, ts, "GET", "/health/ai-errors", nil)
	if status != http.StatusOK {
		t.Fatalf("ai-errors повторное чтение: %d body=%v", status, body)
	}
}

func TestFrontendErrorsAlertCheck_NoErrors_200(t *testing.T) {
	// WITHOUT t.Parallel() on purpose: the frontend error window is global, and other
	// tests in the package send POST /api/_error. Go runs sequential tests while the
	// parallel ones are paused: only this way does reset() below mean anything.
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)
	_ = env

	// Reading the endpoint NO LONGER resets the window (that is the point of the fix),
	// so we clear what other tests accumulated directly.
	frontendErrorAlertEvents.reset()

	status, body := httpJSON(t, ts, "GET", "/health/frontend-errors", nil)
	if status != http.StatusOK {
		t.Fatalf("frontend-errors no errors: %d body=%v", status, body)
	}
	// A repeated read must give the same result: it is idempotent.
	status, body = httpJSON(t, ts, "GET", "/health/frontend-errors", nil)
	if status != http.StatusOK {
		t.Fatalf("frontend-errors повторное чтение: %d body=%v", status, body)
	}
}

// After POST /_error the frontend error window turns red → /health/frontend-errors → 503,
// and stays red for the second observer (the Kuma retry).
func TestFrontendErrorsAlertCheck_AfterReport_503(t *testing.T) {
	// WITHOUT t.Parallel(): see the explanation in the test above.
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)
	_ = env

	frontendErrorAlertEvents.reset()

	// Report an error.
	httpJSON(t, ts, "POST", "/api/_error", map[string]any{
		"type":    "error",
		"message": "test error for alert check",
		"page":    "/test",
	})

	status, body := httpJSON(t, ts, "GET", "/health/frontend-errors", nil)
	if status != http.StatusServiceUnavailable {
		t.Fatalf("frontend-errors after report: want 503, got %d body=%v", status, body)
	}
	if errCount, _ := body["new_errors"].(float64); errCount < 1 {
		t.Errorf("new_errors should be ≥1, got %v", body["new_errors"])
	}

	// The Kuma retry sees the same error, not an emptiness eaten by the first read.
	status, body = httpJSON(t, ts, "GET", "/health/frontend-errors", nil)
	if status != http.StatusServiceUnavailable {
		t.Fatalf("frontend-errors второе чтение должно остаться 503: %d body=%v", status, body)
	}
	if errCount, _ := body["new_errors"].(float64); errCount < 1 {
		t.Errorf("new_errors на втором чтении should be ≥1, got %v", body["new_errors"])
	}
}
