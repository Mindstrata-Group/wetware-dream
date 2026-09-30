//go:build integration

package httpapi

import (
	"bytes"
	"net/http"
	"strings"
	"testing"

	"mindstrata-stage1/api/internal/testsupport"
)

// adminEndpoint describes one admin route + which roles may use it.
type adminEndpoint struct {
	method      string
	path        string
	allowed     []string // roles that should get 2xx or 400 (NOT 401/403)
	denied      []string // roles that MUST get 401/403
	bodyAllowed []byte   // body sent for allowed-role checks (can be nil)
}

// allRoles is the canonical set of validRole() outputs.
var allRoles = []string{"owner", "admin", "billing_admin", "content_admin", "support", "tester", "user"}

// rolesExcept returns allRoles minus the given subset.
func rolesExcept(exclude ...string) []string {
	set := map[string]bool{}
	for _, e := range exclude {
		set[e] = true
	}
	out := []string{}
	for _, r := range allRoles {
		if !set[r] {
			out = append(out, r)
		}
	}
	return out
}

// adminEndpoints: table of every admin/system endpoint and its RBAC matrix.
// Updated whenever a new admin route is added.
var adminEndpoints = []adminEndpoint{
	// Read endpoints
	{method: "GET", path: "/api/admin/users", allowed: []string{"owner", "admin", "support"}, denied: rolesExcept("owner", "admin", "support")},
	{method: "GET", path: "/api/admin/modes", allowed: []string{"owner", "admin", "content_admin"}, denied: rolesExcept("owner", "admin", "content_admin")},
	{method: "GET", path: "/api/admin/status", allowed: []string{"owner", "admin", "billing_admin", "support"}, denied: rolesExcept("owner", "admin", "billing_admin", "support")},
	{method: "GET", path: "/api/admin/tariffs", allowed: []string{"owner", "admin", "billing_admin"}, denied: rolesExcept("owner", "admin", "billing_admin")},
	{method: "GET", path: "/api/admin/payments", allowed: []string{"owner", "admin", "billing_admin"}, denied: rolesExcept("owner", "admin", "billing_admin")},
	{method: "GET", path: "/api/admin/notifications/templates", allowed: []string{"owner", "admin"}, denied: rolesExcept("owner", "admin")},
	{method: "GET", path: "/api/admin/promocodes", allowed: []string{"owner", "admin"}, denied: rolesExcept("owner", "admin")},
	{method: "GET", path: "/api/admin/dialogs", allowed: []string{"owner", "admin", "support"}, denied: rolesExcept("owner", "admin", "support")},

	// Tester routes — note: support has read-only access to tester views
	// (it's used for customer-support workflows).
	{method: "GET", path: "/api/tester/status", allowed: []string{"owner", "admin", "tester"}, denied: rolesExcept("owner", "admin", "tester")},
	{method: "GET", path: "/api/tester/users", allowed: []string{"owner", "admin", "tester", "support"}, denied: rolesExcept("owner", "admin", "tester", "support")},

	// Write endpoints (mutations) — only owner/admin
	{method: "POST", path: "/api/admin/users", bodyAllowed: []byte(`{"email":"contract@test.local","password":"contractpw123","role":"user","status":"active"}`),
		allowed: []string{"owner", "admin"}, denied: rolesExcept("owner", "admin")},
	{method: "POST", path: "/api/admin/modes", bodyAllowed: []byte(`{"name":"contract_mode","prompt":"x"}`),
		allowed: []string{"owner", "admin"}, denied: rolesExcept("owner", "admin")},
	{method: "POST", path: "/api/admin/notifications/templates", bodyAllowed: []byte(`{"key":"service.contract","name":"Contract","consentType":"service","defaultChannel":"in_site","title":"Contract","body":"Body","active":true}`),
		allowed: []string{"owner", "admin"}, denied: rolesExcept("owner", "admin")},
	{method: "POST", path: "/api/admin/notifications/test", bodyAllowed: []byte(`{"userId":1,"templateKey":"service.reminder_30m","channel":"in_site"}`),
		allowed: []string{"owner", "admin"}, denied: rolesExcept("owner", "admin")},
}

// TestContract_RBAC_Matrix runs every endpoint × role combination.
// Catches: forgotten requireAdminSection() calls, role-set drift, accidentally
// opened mutation endpoints.
func TestContract_RBAC_Matrix(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)

	// One user per role, one session per user — reused across all endpoints.
	tokensByRole := map[string]string{}
	for _, role := range allRoles {
		user := f.CreateUser(TestUserOpts{Role: role})
		tokensByRole[role] = f.CreateSession(user.ID)
	}

	for _, ep := range adminEndpoints {
		ep := ep // capture loop var
		// For allowed roles: expect NOT 401/403. (2xx, 400, 404, 409 all fine.)
		for _, role := range ep.allowed {
			role := role
			t.Run("ALLOW_"+role+"_"+ep.method+"_"+sanitizePathForTestName(ep.path), func(t *testing.T) {
				ts := NewTestServer(t, env.Pool)
				ts.LoginAs(tokensByRole[role])
				status := doRawRequest(t, ts, ep.method, ep.path, ep.bodyAllowed)
				if status == http.StatusUnauthorized || status == http.StatusForbidden {
					t.Errorf("role %q DENIED %s %s (got %d), should be allowed",
						role, ep.method, ep.path, status)
				}
			})
		}
		// For denied roles: MUST get 401 or 403.
		for _, role := range ep.denied {
			role := role
			t.Run("DENY_"+role+"_"+ep.method+"_"+sanitizePathForTestName(ep.path), func(t *testing.T) {
				ts := NewTestServer(t, env.Pool)
				ts.LoginAs(tokensByRole[role])
				status := doRawRequest(t, ts, ep.method, ep.path, ep.bodyAllowed)
				if status != http.StatusUnauthorized && status != http.StatusForbidden {
					t.Errorf("role %q ALLOWED %s %s (got %d), should be denied",
						role, ep.method, ep.path, status)
				}
			})
		}
	}
}

// TestContract_MalformedJSON_Returns400: every mutating endpoint must reject
// non-JSON bodies with 400 (not 500 or panic).
func TestContract_MalformedJSON_Returns400(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	adminToken := f.CreateSession(admin.ID)

	mutationPaths := []struct{ method, path string }{
		{"POST", "/api/admin/users"},
		{"POST", "/api/admin/modes"},
		{"POST", "/api/admin/access"},
		{"POST", "/api/chat/send"},
		{"POST", "/api/chat/select-mode"},
		{"POST", "/api/chat/complete"},
		{"POST", "/api/auth/login"},
		{"POST", "/api/auth/register"},
		{"POST", "/api/access/promocode/apply"},
	}

	for _, mp := range mutationPaths {
		mp := mp
		t.Run(mp.method+"_"+sanitizePathForTestName(mp.path), func(t *testing.T) {
			ts := NewTestServer(t, env.Pool)
			ts.LoginAs(adminToken)
			status := doRawRequest(t, ts, mp.method, mp.path, []byte("not a json at all }{"))
			if status == http.StatusInternalServerError {
				t.Errorf("%s %s with malformed JSON returned 500 (should be 400)",
					mp.method, mp.path)
			}
			// 400 or other client-side errors are fine. The point: NOT 5xx.
			if status >= 500 {
				t.Errorf("%s %s with malformed JSON returned %d (5xx is server bug)",
					mp.method, mp.path, status)
			}
		})
	}
}

// doRawRequest sends a request with an optional raw body and returns status.
func doRawRequest(t *testing.T, ts *TestServer, method, path string, body []byte) int {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	var req *http.Request
	var err error
	if reader == nil {
		req, err = http.NewRequest(method, ts.URL(path), nil)
	} else {
		req, err = http.NewRequest(method, ts.URL(path), reader)
	}
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := ts.Client.Do(req)
	if err != nil {
		t.Fatalf("do %s %s: %v", method, path, err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

// sanitizePathForTestName: subtests can't contain "/" in their names cleanly.
func sanitizePathForTestName(path string) string {
	return strings.ReplaceAll(strings.Trim(path, "/"), "/", "_")
}
