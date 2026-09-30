//go:build integration

package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"mindstrata-stage1/api/internal/testsupport"
)

// TestBootstrapAdmin_RequiresToken: no X-Bootstrap-Token → 403.
func TestBootstrapAdmin_RequiresToken(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServerWithHandler(t, env.Pool, Handler{BootstrapAdminToken: "bootstrap-secret-xyz"})

	status, body := httpJSON(t, ts, "POST", "/api/bootstrap/admin", map[string]any{
		"email": "admin@test.local", "password": "verystrongpw123",
	})
	if status != http.StatusForbidden {
		t.Fatalf("expected 403 without token, got %d body=%v", status, body)
	}
}

// TestBootstrapAdmin_WrongToken: wrong token → 403.
func TestBootstrapAdmin_WrongToken(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServerWithHandler(t, env.Pool, Handler{BootstrapAdminToken: "bootstrap-secret-xyz"})

	req, _ := http.NewRequest("POST", ts.URL("/api/bootstrap/admin"),
		jsonBody(map[string]any{"email": "admin@test.local", "password": "verystrongpw123"}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Bootstrap-Token", "wrong")
	resp, err := ts.Client.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 with wrong token, got %d", resp.StatusCode)
	}
}

// TestBootstrapAdmin_HappyPath: correct token, no admin yet → 201, admin created.
func TestBootstrapAdmin_HappyPath(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	const token = "bootstrap-secret-xyz"
	ts := NewTestServerWithHandler(t, env.Pool, Handler{BootstrapAdminToken: token})

	status, body := bootstrapAdminCall(t, ts, token, "first@test.local", "verystrongpw123")
	if status != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%v", status, body)
	}
	if userIDF, ok := body["userId"].(float64); !ok || userIDF == 0 {
		t.Fatalf("missing userId in response: %v", body)
	}

	// DB: admin created
	var role string
	_ = env.Pool.QueryRow(context.Background(),
		`select role from users where lower(email) = $1`, "first@test.local").Scan(&role)
	if role != "admin" {
		t.Fatalf("role: got %q want admin", role)
	}
}

// TestBootstrapAdmin_SecondCall_Conflict: admin already exists → 409.
func TestBootstrapAdmin_SecondCall_Conflict(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	const token = "bootstrap-secret-xyz"
	ts := NewTestServerWithHandler(t, env.Pool, Handler{BootstrapAdminToken: token})

	// Create the admin through the factory (bypassing BootstrapAdmin)
	f := NewFactory(t, env.Pool)
	f.CreateUser(TestUserOpts{Role: "admin"})

	// Bootstrap call → must return 409
	status, body := bootstrapAdminCall(t, ts, token, "second@test.local", "verystrongpw123")
	if status != http.StatusConflict {
		t.Fatalf("expected 409 when admin exists, got %d body=%v", status, body)
	}
}

// TestAdminAccess_NonAdmin_Forbidden: a regular user hits /api/admin/access → 403.
func TestAdminAccess_NonAdmin_Forbidden(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	user := f.CreateUser(TestUserOpts{Role: "user"})
	token := f.CreateSession(user.ID)
	ts.LoginAs(token)

	status, _ := httpJSON(t, ts, "POST", "/api/admin/access", map[string]any{
		"userId": user.ID, "modeId": 1, "days": 7,
	})
	if status != http.StatusForbidden && status != http.StatusUnauthorized {
		t.Fatalf("expected 403/401, got %d", status)
	}
}

// TestAdminAccess_GrantsAccess_AndWritesAudit: admin creates a grant + an audit record.
func TestAdminAccess_GrantsAccess_AndWritesAudit(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	target := f.CreateUser(TestUserOpts{Role: "user"})
	mode := f.CreateMode(TestModeOpts{})

	token := f.CreateSession(admin.ID)
	ts.LoginAs(token)

	status, body := httpJSON(t, ts, "POST", "/api/admin/access", map[string]any{
		"userId":            target.ID,
		"modeIds":           []int64{mode.ID},
		"days":              30,
		"dailyMessageLimit": 100,
	})
	if status != http.StatusOK {
		t.Fatalf("admin/access: %d body=%v", status, body)
	}

	// DB: grant created with the correct limit
	var grantCount int64
	var limit int64
	err := env.Pool.QueryRow(context.Background(),
		`select count(*), coalesce(max(daily_message_limit), 0) from user_mode_access
		 where user_id = $1 and mode_id = $2 and access_type = 'manual'`,
		target.ID, mode.ID).Scan(&grantCount, &limit)
	if err != nil {
		t.Fatalf("query grant: %v", err)
	}
	if grantCount == 0 {
		t.Fatalf("admin grant not created")
	}
	if limit != 100 {
		t.Fatalf("daily_message_limit: got %d want 100", limit)
	}

	// DB: audit record created (admin_audit_log, action=admin.access.grant)
	var auditCount int64
	_ = env.Pool.QueryRow(context.Background(),
		`select count(*) from admin_audit_log
		 where actor_user_id = $1 and target_type = 'user' and target_id = $2
		   and action = 'admin.access.grant'`,
		admin.ID, target.ID).Scan(&auditCount)
	if auditCount == 0 {
		t.Fatalf("admin_audit_log entry not created for admin.access.grant")
	}
}

// TestAdminAccess_ResetLimits_CreatesResetRow: resetLimits=true → admin_mode_usage_resets row.
func TestAdminAccess_ResetLimits_CreatesResetRow(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	target := f.CreateUser(TestUserOpts{Role: "user"})
	mode := f.CreateMode(TestModeOpts{})

	token := f.CreateSession(admin.ID)
	ts.LoginAs(token)

	status, body := httpJSON(t, ts, "POST", "/api/admin/access", map[string]any{
		"userId":      target.ID,
		"modeIds":     []int64{mode.ID},
		"days":        30,
		"resetLimits": true,
	})
	if status != http.StatusOK {
		t.Fatalf("admin/access with reset: %d body=%v", status, body)
	}

	var resetCount int64
	_ = env.Pool.QueryRow(context.Background(),
		`select count(*) from admin_mode_usage_resets
		 where user_id = $1 and mode_id = $2 and actor_user_id = $3`,
		target.ID, mode.ID, admin.ID).Scan(&resetCount)
	if resetCount == 0 {
		t.Fatalf("admin_mode_usage_resets row not created for resetLimits=true")
	}
}

// TestAdminUsers_RequiresAdmin: a regular user must not see the list.
func TestAdminUsers_RequiresAdmin(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	user := f.CreateUser(TestUserOpts{Role: "user"})
	token := f.CreateSession(user.ID)
	ts.LoginAs(token)

	status, _ := httpJSON(t, ts, "GET", "/api/admin/users", nil)
	if status != http.StatusForbidden && status != http.StatusUnauthorized {
		t.Fatalf("expected 403/401 for regular user, got %d", status)
	}
}

// TestAdminUsers_AllowsSupport_ReadOnly: support can read /api/admin/users (read).
func TestAdminUsers_AllowsSupport_ReadOnly(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	support := f.CreateUser(TestUserOpts{Role: "support"})
	token := f.CreateSession(support.ID)
	ts.LoginAs(token)

	status, _ := httpJSON(t, ts, "GET", "/api/admin/users", nil)
	if status != http.StatusOK {
		t.Fatalf("support read /api/admin/users: expected 200, got %d", status)
	}
}

// jsonBody marshals body to bytes.Reader; nil → nil.
func jsonBody(v any) *bytes.Reader {
	if v == nil {
		return nil
	}
	buf, _ := json.Marshal(v)
	return bytes.NewReader(buf)
}

// bootstrapAdminCall makes a POST /api/bootstrap/admin with the given token header.
func bootstrapAdminCall(t *testing.T, ts *TestServer, token, email, password string) (int, map[string]any) {
	t.Helper()
	req, err := http.NewRequest("POST", ts.URL("/api/bootstrap/admin"),
		jsonBody(map[string]any{"email": email, "password": password}))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Bootstrap-Token", token)
	resp, err := ts.Client.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close()
	var body map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&body)
	return resp.StatusCode, body
}
