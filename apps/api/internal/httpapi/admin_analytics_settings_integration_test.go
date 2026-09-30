//go:build integration

package httpapi

import (
	"net/http"
	"testing"

	"mindstrata-stage1/api/internal/testsupport"
)

func TestAdminAnalyticsSettings_SaveAndPublicRead(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	// Before configuration: the public endpoint returns an empty counter and default parameters.
	status, body := httpJSON(t, ts, http.MethodGet, "/api/public/analytics", nil)
	if status != http.StatusOK || body["ok"] != true {
		t.Fatalf("public before setup: status=%d body=%v", status, body)
	}
	if body["metrikaCounterId"] != "" {
		t.Fatalf("expected empty counter id before setup, got %v", body["metrikaCounterId"])
	}
	if params, ok := body["metrikaParams"].(map[string]any); !ok || params["clickmap"] != true {
		t.Fatalf("expected default params object, got %v", body["metrikaParams"])
	}

	// Admin saves the counter.
	status, body = httpJSON(t, ts, http.MethodPost, "/api/admin/analytics-settings", map[string]any{
		"metrikaCounterId": "12345678",
		"metrikaParams":    `{"clickmap":true,"webvisor":true}`,
	})
	if status != http.StatusOK || body["ok"] != true {
		t.Fatalf("save: status=%d body=%v", status, body)
	}

	// Admin GET returns what was saved.
	status, body = httpJSON(t, ts, http.MethodGet, "/api/admin/analytics-settings", nil)
	if status != http.StatusOK || body["metrikaCounterId"] != "12345678" {
		t.Fatalf("admin get: status=%d body=%v", status, body)
	}

	// The public endpoint sees the new value (cache cleared on POST).
	status, body = httpJSON(t, ts, http.MethodGet, "/api/public/analytics", nil)
	if status != http.StatusOK || body["metrikaCounterId"] != "12345678" {
		t.Fatalf("public after save: status=%d body=%v", status, body)
	}
	params, ok := body["metrikaParams"].(map[string]any)
	if !ok || params["webvisor"] != true {
		t.Fatalf("public params: %v", body["metrikaParams"])
	}

	// Value change: the public endpoint cache must be cleared again.
	status, _ = httpJSON(t, ts, http.MethodPost, "/api/admin/analytics-settings", map[string]any{
		"metrikaCounterId": "87654321",
		"metrikaParams":    "",
	})
	if status != http.StatusOK {
		t.Fatalf("re-save status=%d", status)
	}
	status, body = httpJSON(t, ts, http.MethodGet, "/api/public/analytics", nil)
	if status != http.StatusOK || body["metrikaCounterId"] != "87654321" {
		t.Fatalf("public after re-save: status=%d body=%v", status, body)
	}

	// Empty counter = analytics off.
	status, _ = httpJSON(t, ts, http.MethodPost, "/api/admin/analytics-settings", map[string]any{
		"metrikaCounterId": "",
		"metrikaParams":    "",
	})
	if status != http.StatusOK {
		t.Fatalf("disable status=%d", status)
	}
	status, body = httpJSON(t, ts, http.MethodGet, "/api/public/analytics", nil)
	if status != http.StatusOK || body["metrikaCounterId"] != "" {
		t.Fatalf("public after disable: status=%d body=%v", status, body)
	}
}

func TestAdminAnalyticsSettings_RejectsInvalidInput(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	cases := []map[string]any{
		{"metrikaCounterId": "abc123", "metrikaParams": ""},
		{"metrikaCounterId": "<script>alert(1)</script>", "metrikaParams": ""},
		{"metrikaCounterId": "123", "metrikaParams": "не json"},
		{"metrikaCounterId": "123", "metrikaParams": "[1,2,3]"},
	}
	for _, payload := range cases {
		status, body := httpJSON(t, ts, http.MethodPost, "/api/admin/analytics-settings", payload)
		if status != http.StatusBadRequest {
			t.Fatalf("payload %v: status=%d body=%v, want 400", payload, status, body)
		}
	}
}

func TestAdminAnalyticsSettings_RoleAccess(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)

	// A regular user can neither read nor write.
	tsUser := NewTestServer(t, env.Pool)
	user := f.CreateUser(TestUserOpts{Role: "user"})
	tsUser.LoginAs(f.CreateSession(user.ID))
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		status, _ := httpJSON(t, tsUser, method, "/api/admin/analytics-settings", map[string]any{})
		if status != http.StatusForbidden && status != http.StatusUnauthorized {
			t.Fatalf("user %s status=%d want 401/403", method, status)
		}
	}

	// support: section "system" is readable, but mutation is forbidden.
	tsSupport := NewTestServer(t, env.Pool)
	support := f.CreateUser(TestUserOpts{Role: "support"})
	tsSupport.LoginAs(f.CreateSession(support.ID))
	status, _ := httpJSON(t, tsSupport, http.MethodGet, "/api/admin/analytics-settings", nil)
	if status != http.StatusOK {
		t.Fatalf("support GET status=%d want 200", status)
	}
	status, _ = httpJSON(t, tsSupport, http.MethodPost, "/api/admin/analytics-settings", map[string]any{
		"metrikaCounterId": "1", "metrikaParams": "",
	})
	if status != http.StatusForbidden {
		t.Fatalf("support POST status=%d want 403", status)
	}
}
