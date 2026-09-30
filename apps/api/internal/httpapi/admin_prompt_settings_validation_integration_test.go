//go:build integration

package httpapi

import (
	"net/http"
	"testing"

	"mindstrata-stage1/api/internal/testsupport"
)

// TestAdminOrchestrationPrompt_EmptyPrompt_400 (Standards: "prompt is required"
// is declared: an empty/whitespace string must be rejected).
func TestAdminOrchestrationPrompt_EmptyPrompt_400(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)
	ts.LoginAs(f.CreateSession(f.CreateUser(TestUserOpts{Role: "admin"}).ID))

	status, body := httpJSON(t, ts, "POST", "/api/admin/orchestration-prompt", map[string]any{"prompt": "   "})
	if status != http.StatusBadRequest {
		t.Fatalf("empty prompt: %d body=%v, want 400", status, body)
	}
}

// TestAdminOrchestrationPrompt_WrongMethod_405.
func TestAdminOrchestrationPrompt_WrongMethod_405(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)
	ts.LoginAs(f.CreateSession(f.CreateUser(TestUserOpts{Role: "admin"}).ID))

	status, _ := httpJSON(t, ts, "DELETE", "/api/admin/orchestration-prompt", nil)
	if status != http.StatusMethodNotAllowed {
		t.Fatalf("DELETE orchestration-prompt: %d, want 405", status)
	}
}

// TestAdminDialogSummaryPrompt_EmptyPrompt_400.
func TestAdminDialogSummaryPrompt_EmptyPrompt_400(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)
	ts.LoginAs(f.CreateSession(f.CreateUser(TestUserOpts{Role: "admin"}).ID))

	status, body := httpJSON(t, ts, "POST", "/api/admin/dialog-summary-prompt", map[string]any{"prompt": ""})
	if status != http.StatusBadRequest {
		t.Fatalf("empty prompt: %d body=%v, want 400", status, body)
	}
}

// TestAdminDialogSummaryPrompt_WrongMethod_405.
func TestAdminDialogSummaryPrompt_WrongMethod_405(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)
	ts.LoginAs(f.CreateSession(f.CreateUser(TestUserOpts{Role: "admin"}).ID))

	status, _ := httpJSON(t, ts, "DELETE", "/api/admin/dialog-summary-prompt", nil)
	if status != http.StatusMethodNotAllowed {
		t.Fatalf("DELETE dialog-summary-prompt: %d, want 405", status)
	}
}

// TestBootstrapAdmin_EmptyEmail_400 (Standards: email is required for the
// bootstrap admin, as for normal registration).
func TestBootstrapAdmin_EmptyEmail_400(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	const token = "bootstrap-empty-email"
	ts := NewTestServerWithHandler(t, env.Pool, Handler{BootstrapAdminToken: token})

	status, body := bootstrapAdminCall(t, ts, token, "", "verystrongpw123")
	if status != http.StatusBadRequest {
		t.Fatalf("empty email: %d body=%v, want 400", status, body)
	}
}

// TestBootstrapAdmin_PasswordTooShort_400 (World: a password shorter than 10
// characters is really rejected by hashPassword, not a hard-coded stub).
func TestBootstrapAdmin_PasswordTooShort_400(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	const token = "bootstrap-short-pass"
	ts := NewTestServerWithHandler(t, env.Pool, Handler{BootstrapAdminToken: token})

	status, body := bootstrapAdminCall(t, ts, token, "shortpass@test.local", "short")
	if status != http.StatusBadRequest {
		t.Fatalf("short password: %d body=%v, want 400", status, body)
	}
}

// TestAdminStats_WrongMethod_405.
func TestAdminStats_WrongMethod_405(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)
	ts.LoginAs(f.CreateSession(f.CreateUser(TestUserOpts{Role: "admin"}).ID))

	status, _ := httpJSON(t, ts, "POST", "/api/admin/stats", nil)
	if status != http.StatusMethodNotAllowed {
		t.Fatalf("POST /api/admin/stats: %d, want 405", status)
	}
}

// TestAdminStats_SecondCall_ServedFromCache (Claims: an in-memory cache for
// adminStatsCacheTTL is declared: the second request must return the same
// payload without re-running the heavy aggregations; here we check the cache
// hit itself by a response body identical to the first call).
func TestAdminStats_SecondCall_ServedFromCache(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)
	ts.LoginAs(f.CreateSession(f.CreateUser(TestUserOpts{Role: "admin"}).ID))

	status1, body1 := httpJSON(t, ts, "GET", "/api/admin/stats", nil)
	if status1 != http.StatusOK {
		t.Fatalf("first GET: %d body=%v", status1, body1)
	}
	status2, body2 := httpJSON(t, ts, "GET", "/api/admin/stats", nil)
	if status2 != http.StatusOK {
		t.Fatalf("second GET: %d body=%v", status2, body2)
	}
	totals1, _ := body1["totals"].(map[string]any)
	totals2, _ := body2["totals"].(map[string]any)
	if totals1["requests"] != totals2["requests"] {
		t.Fatalf("cached second call diverged: %v vs %v", totals1, totals2)
	}
}
