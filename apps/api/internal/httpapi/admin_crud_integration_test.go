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

// adminPatch: helper for HTTP PATCH (httpJSON only supports POST/GET/DELETE).
func adminPatch(t *testing.T, ts *TestServer, path string, body any) (int, map[string]any) {
	t.Helper()
	buf, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req, err := http.NewRequest("PATCH", ts.URL(path), bytes.NewReader(buf))
	if err != nil {
		t.Fatalf("new req: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := ts.Client.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close()
	var parsed map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&parsed)
	return resp.StatusCode, parsed
}

// TestAdminCRUD_CreateUser_HappyPath: POST /api/admin/users → 201, user in DB, audit logged.
func TestAdminCRUD_CreateUser_HappyPath(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	newEmail := uniqueEmail("created")
	code, body := httpJSON(t, ts, "POST", "/api/admin/users", map[string]any{
		"email":    newEmail,
		"password": "createdpass123",
		"role":     "tester",
		"status":   "active",
	})
	if code != http.StatusCreated {
		t.Fatalf("create: %d body=%v", code, body)
	}
	uid, _ := body["userId"].(float64)
	if uid == 0 {
		t.Fatalf("no userId in response: %v", body)
	}

	// DB: user created with the correct role
	var role string
	_ = env.Pool.QueryRow(context.Background(),
		`select role from users where id = $1`, int64(uid)).Scan(&role)
	if role != "tester" {
		t.Fatalf("role: got %q want tester", role)
	}

	// Audit log
	var auditCount int64
	_ = env.Pool.QueryRow(context.Background(),
		`select count(*) from admin_audit_log
		 where actor_user_id = $1 and action = 'admin.user.create' and target_id = $2`,
		admin.ID, int64(uid)).Scan(&auditCount)
	if auditCount == 0 {
		t.Fatalf("admin.user.create audit log missing")
	}
}

// TestAdminCRUD_CreateUser_YandexOnly_NoPassword: an empty password does not generate a local password.
func TestAdminCRUD_CreateUser_YandexOnly_NoPassword(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	newEmail := uniqueEmail("created_yandex_only")
	code, body := httpJSON(t, ts, "POST", "/api/admin/users", map[string]any{
		"email":  newEmail,
		"role":   "user",
		"status": "active",
	})
	if code != http.StatusCreated {
		t.Fatalf("create without password: %d body=%v", code, body)
	}
	uid, _ := body["userId"].(float64)
	if uid == 0 {
		t.Fatalf("no userId in response: %v", body)
	}

	var hasPassword bool
	if err := env.Pool.QueryRow(context.Background(),
		`select password_hash is not null from users where id = $1`, int64(uid)).Scan(&hasPassword); err != nil {
		t.Fatalf("query password_hash: %v", err)
	}
	if hasPassword {
		t.Fatalf("password_hash must stay null for Yandex-only user created without password")
	}
}

func TestAdminCRUD_CreateUser_WhitespacePasswordStaysNull(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	code, body := httpJSON(t, ts, "POST", "/api/admin/users", map[string]any{
		"email":    uniqueEmail("blank_password"),
		"password": "   \t  ",
		"role":     "user",
		"status":   "active",
	})
	if code != http.StatusCreated {
		t.Fatalf("create with whitespace password: %d body=%v", code, body)
	}
	uid, _ := body["userId"].(float64)
	var hasPassword bool
	if err := env.Pool.QueryRow(context.Background(), `select password_hash is not null from users where id=$1`, int64(uid)).Scan(&hasPassword); err != nil {
		t.Fatalf("query password_hash: %v", err)
	}
	if hasPassword {
		t.Fatalf("whitespace password must be treated as omitted")
	}
}

func TestAdminCRUD_CreateUser_ShortExplicitPasswordRejected(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	email := uniqueEmail("short_password")
	code, _ := httpJSON(t, ts, "POST", "/api/admin/users", map[string]any{
		"email":    email,
		"password": "short",
		"role":     "user",
		"status":   "active",
	})
	if code != http.StatusBadRequest {
		t.Fatalf("short explicit password status=%d want 400", code)
	}
	var count int64
	_ = env.Pool.QueryRow(context.Background(), `select count(*) from users where email=$1`, email).Scan(&count)
	if count != 0 {
		t.Fatalf("short-password request created %d users", count)
	}
}

func TestAdminCRUD_CreateUser_ExplicitPasswordIsHashed(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	password := "createdpass123"
	code, body := httpJSON(t, ts, "POST", "/api/admin/users", map[string]any{
		"email":    uniqueEmail("explicit_password"),
		"password": password,
		"role":     "user",
		"status":   "active",
	})
	if code != http.StatusCreated {
		t.Fatalf("create with explicit password: %d body=%v", code, body)
	}
	uid, _ := body["userId"].(float64)
	var hash string
	if err := env.Pool.QueryRow(context.Background(), `select coalesce(password_hash, '') from users where id=$1`, int64(uid)).Scan(&hash); err != nil {
		t.Fatalf("query password_hash: %v", err)
	}
	if hash == "" || hash == password {
		t.Fatalf("password_hash not stored securely: %q", hash)
	}
	if !verifyPassword(password, hash) {
		t.Fatalf("stored hash does not verify")
	}
}

func TestAdminCRUD_CreateUser_ResponseDoesNotExposePassword(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	code, body := httpJSON(t, ts, "POST", "/api/admin/users", map[string]any{
		"email":    uniqueEmail("response_password"),
		"password": "createdpass123",
		"role":     "user",
		"status":   "active",
	})
	if code != http.StatusCreated {
		t.Fatalf("create: %d body=%v", code, body)
	}
	for _, key := range []string{"password", "passwordHash", "password_hash"} {
		if _, ok := body[key]; ok {
			t.Fatalf("response leaks %s: %v", key, body)
		}
	}
}

// TestAdminCRUD_CreateUser_InvalidRole: role='superuser' → 400.
func TestAdminCRUD_CreateUser_InvalidRole(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	code, _ := httpJSON(t, ts, "POST", "/api/admin/users", map[string]any{
		"email": uniqueEmail("bad"), "password": "createdpass123",
		"role": "superuser", "status": "active",
	})
	if code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid role, got %d", code)
	}
}

// TestAdminCRUD_CreateUser_BySupport_Forbidden: support cannot create (read only).
func TestAdminCRUD_CreateUser_BySupport_Forbidden(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	support := f.CreateUser(TestUserOpts{Role: "support"})
	ts.LoginAs(f.CreateSession(support.ID))

	code, _ := httpJSON(t, ts, "POST", "/api/admin/users", map[string]any{
		"email": uniqueEmail("ssbad"), "password": "createdpass123",
		"role": "user", "status": "active",
	})
	if code != http.StatusForbidden {
		t.Fatalf("expected 403 for support mutation, got %d", code)
	}
}

// TestAdminCRUD_PatchUser_ChangeRoleAndStatus: PATCH /api/admin/users/{id}.
func TestAdminCRUD_PatchUser_ChangeRoleAndStatus(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	target := f.CreateUser(TestUserOpts{Role: "user", Status: "active"})
	ts.LoginAs(f.CreateSession(admin.ID))

	code, body := adminPatch(t, ts, "/api/admin/users/"+itoa(target.ID), map[string]any{
		"email":  target.Email,
		"role":   "tester",
		"status": "blocked",
	})
	if code != http.StatusOK {
		t.Fatalf("patch: %d body=%v", code, body)
	}

	// DB: role + status updated
	var role, status string
	_ = env.Pool.QueryRow(context.Background(),
		`select role, status from users where id = $1`, target.ID).Scan(&role, &status)
	if role != "tester" || status != "blocked" {
		t.Fatalf("after patch: role=%q status=%q, want tester+blocked", role, status)
	}

	// Audit log
	var auditCount int64
	_ = env.Pool.QueryRow(context.Background(),
		`select count(*) from admin_audit_log
		 where actor_user_id = $1 and action = 'admin.user.patch' and target_id = $2`,
		admin.ID, target.ID).Scan(&auditCount)
	if auditCount == 0 {
		t.Fatalf("admin.user.patch audit not written")
	}
}

// TestAdminCRUD_PatchUser_PasswordChange_RevokesSessions: password change → all target sessions revoked.
func TestAdminCRUD_PatchUser_PasswordChange_RevokesSessions(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	target := f.CreateUser(TestUserOpts{})
	// 2 sessions for target
	_ = f.CreateSession(target.ID)
	_ = f.CreateSession(target.ID)

	ts.LoginAs(f.CreateSession(admin.ID))
	code, _ := adminPatch(t, ts, "/api/admin/users/"+itoa(target.ID), map[string]any{
		"email":    target.Email,
		"role":     target.Role,
		"status":   target.Status,
		"password": "newpassword123",
	})
	if code != http.StatusOK {
		t.Fatalf("patch w/ password: %d", code)
	}

	// DB: all target sessions revoked
	var active int64
	_ = env.Pool.QueryRow(context.Background(),
		`select count(*) from auth_sessions where user_id = $1 and revoked_at is null`,
		target.ID).Scan(&active)
	if active != 0 {
		t.Fatalf("sessions not revoked after password change: %d still active", active)
	}
}

// TestAdminCRUD_PatchUser_SelfDemoteBlocked: an admin cannot demote or block themselves.
func TestAdminCRUD_PatchUser_SelfDemoteBlocked(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	// Attempt to lower the role
	code, _ := adminPatch(t, ts, "/api/admin/users/"+itoa(admin.ID), map[string]any{
		"email":  admin.Email,
		"role":   "user",
		"status": "active",
	})
	if code != http.StatusBadRequest {
		t.Fatalf("self-demote: expected 400, got %d", code)
	}

	// Attempt to block
	code, _ = adminPatch(t, ts, "/api/admin/users/"+itoa(admin.ID), map[string]any{
		"email":  admin.Email,
		"role":   admin.Role,
		"status": "blocked",
	})
	if code != http.StatusBadRequest {
		t.Fatalf("self-block: expected 400, got %d", code)
	}
}

// TestAdminCRUD_CreateMode_HappyPath: POST /api/admin/modes → 201, mode in DB, audit logged.
func TestAdminCRUD_CreateMode_HappyPath(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	code, body := httpJSON(t, ts, "POST", "/api/admin/modes", map[string]any{
		"name":             "TestMode_via_API",
		"prompt":           "Test system prompt",
		"welcomeMessage":   "Hello test",
		"aiModel":          "openai/gpt-4o-mini",
		"modelTemperature": 0.5,
	})
	if code != http.StatusCreated {
		t.Fatalf("create mode: %d body=%v", code, body)
	}
	mid, _ := body["modeId"].(float64)
	if mid == 0 {
		t.Fatalf("no modeId returned: %v", body)
	}

	// DB: mode created
	var name, prompt string
	_ = env.Pool.QueryRow(context.Background(),
		`select name, prompt from modes where id = $1`, int64(mid)).Scan(&name, &prompt)
	if name != "TestMode_via_API" || prompt != "Test system prompt" {
		t.Fatalf("mode fields: name=%q prompt=%q", name, prompt)
	}

	// Audit log
	var auditCount int64
	_ = env.Pool.QueryRow(context.Background(),
		`select count(*) from admin_audit_log
		 where actor_user_id = $1 and action = 'admin.mode.create' and target_id = $2`,
		admin.ID, int64(mid)).Scan(&auditCount)
	if auditCount == 0 {
		t.Fatalf("admin.mode.create audit missing")
	}
}

// TestAdminCRUD_CreateMode_MissingFields: name="" → 400.
func TestAdminCRUD_CreateMode_MissingFields(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	code, _ := httpJSON(t, ts, "POST", "/api/admin/modes", map[string]any{
		"prompt": "no name provided",
	})
	if code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing name, got %d", code)
	}
}

// TestAdminCRUD_Modes_ContentAdmin_ReadOnly:
// content_admin can only GET modes, not POST.
// Consistent with adminSectionAllowed: mutation requires owner/admin.
func TestAdminCRUD_Modes_ContentAdmin_ReadOnly(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	ca := f.CreateUser(TestUserOpts{Role: "content_admin"})
	ts.LoginAs(f.CreateSession(ca.ID))

	// GET — ok
	code, _ := httpJSON(t, ts, "GET", "/api/admin/modes", nil)
	if code != http.StatusOK {
		t.Fatalf("content_admin GET /api/admin/modes: expected 200, got %d", code)
	}

	// POST: 403 (mutation requires admin)
	code, _ = httpJSON(t, ts, "POST", "/api/admin/modes", map[string]any{
		"name": "ca_should_fail", "prompt": "x",
	})
	if code != http.StatusForbidden {
		t.Fatalf("content_admin POST /api/admin/modes: expected 403, got %d", code)
	}
}

// TestAdminCRUD_CreateMode_BillingAdmin_Forbidden: billing_admin must not create modes.
func TestAdminCRUD_CreateMode_BillingAdmin_Forbidden(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	ba := f.CreateUser(TestUserOpts{Role: "billing_admin"})
	ts.LoginAs(f.CreateSession(ba.ID))

	code, _ := httpJSON(t, ts, "POST", "/api/admin/modes", map[string]any{
		"name":   "BA_should_fail",
		"prompt": "fail",
	})
	if code != http.StatusForbidden {
		t.Fatalf("billing_admin should not create modes, got %d", code)
	}
}
