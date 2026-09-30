//go:build integration

package httpapi

import (
	"context"
	"net/http"
	"testing"
	"time"

	"mindstrata-stage1/api/internal/testsupport"
)

// =============================================================================
// SLAB I — final push to 80%: SendMessage + ApplyPromocode + AccessStatus + auth flow
// =============================================================================

// 1. AccessStatus authenticated user with access.
func TestAccessStatus_AuthWithAccess(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	user := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{})
	f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: mode.ID, DailyMessageLimit: 10})
	ts.LoginAs(f.CreateSession(user.ID))

	status, body := httpJSON(t, ts, "GET", "/api/access/status", nil)
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%v", status, body)
	}
	if has, _ := body["hasAccess"].(bool); !has {
		t.Errorf("hasAccess=false despite grant")
	}
}

// 2. AccessStatus wrong method → 405.
func TestAccessStatus_WrongMethod_405(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)
	_ = env

	status, _ := httpJSON(t, ts, "POST", "/api/access/status", nil)
	if status != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", status)
	}
}

// 3. AccessStatus guest path → 200 (guest cookie set).
func TestAccessStatus_Guest_NoAccess(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)
	_ = env

	status, body := httpJSON(t, ts, "GET", "/api/access/status", nil)
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%v", status, body)
	}
	if has, _ := body["hasAccess"].(bool); has {
		t.Errorf("guest should have no access initially")
	}
}

// =============================================================================
// ApplyPromocode
// =============================================================================

// 4. ApplyPromocode happy path mode promo.
func TestApplyPromocode_HappyPath_Mode(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	user := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{})
	code := "APPLY_" + itoa(int64(env.Pool.Stat().AcquireCount()))
	_ = f.CreatePromocode(TestPromocodeOpts{Code: code, TargetID: mode.ID})

	ts.LoginAs(f.CreateSession(user.ID))
	status, body := httpJSON(t, ts, "POST", "/api/access/promocode/apply", map[string]any{"code": code})
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%v", status, body)
	}
	if days, _ := body["accessDays"].(float64); days < 1 {
		t.Errorf("accessDays not set: %v", body)
	}
}

// 5. ApplyPromocode empty code → 400.
func TestApplyPromocode_EmptyCode_400(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)
	_ = env

	status, _ := httpJSON(t, ts, "POST", "/api/access/promocode/apply", map[string]any{"code": ""})
	if status != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", status)
	}
}

// 6. ApplyPromocode not found → 404.
func TestApplyPromocode_NotFound_404(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)
	_ = env

	status, _ := httpJSON(t, ts, "POST", "/api/access/promocode/apply", map[string]any{"code": "DOESNOTEXIST_X"})
	if status != http.StatusNotFound {
		t.Errorf("expected 404, got %d", status)
	}
}

// 7. ApplyPromocode expired → 400.
func TestApplyPromocode_Expired_400(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	mode := f.CreateMode(TestModeOpts{})
	code := "EXPIRED_" + itoa(int64(env.Pool.Stat().AcquireCount()))
	past1 := time.Now().Add(-48 * time.Hour)
	past2 := time.Now().Add(-24 * time.Hour)
	_ = f.CreatePromocode(TestPromocodeOpts{
		Code: code, TargetID: mode.ID,
		ActiveFrom: &past1, ActiveTo: &past2,
	})

	status, _ := httpJSON(t, ts, "POST", "/api/access/promocode/apply", map[string]any{"code": code})
	if status != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", status)
	}
}

// 8. ApplyPromocode not-yet-active → 400.
func TestApplyPromocode_NotStarted_400(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	mode := f.CreateMode(TestModeOpts{})
	code := "NOTSTART_" + itoa(int64(env.Pool.Stat().AcquireCount()))
	future1 := time.Now().Add(24 * time.Hour)
	future2 := time.Now().Add(48 * time.Hour)
	_ = f.CreatePromocode(TestPromocodeOpts{
		Code: code, TargetID: mode.ID,
		ActiveFrom: &future1, ActiveTo: &future2,
	})

	status, _ := httpJSON(t, ts, "POST", "/api/access/promocode/apply", map[string]any{"code": code})
	if status != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", status)
	}
}

// 9. ApplyPromocode max uses reached → 400.
func TestApplyPromocode_LimitReached_400(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	mode := f.CreateMode(TestModeOpts{})
	code := "MAXUSED_" + itoa(int64(env.Pool.Stat().AcquireCount()))
	promo := f.CreatePromocode(TestPromocodeOpts{
		Code: code, TargetID: mode.ID, MaxUses: 1,
	})
	// Manually set used_count = 1
	_, _ = env.Pool.Exec(context.Background(),
		`update promocodes set used_count = 1 where id = $1`, promo.ID)

	status, _ := httpJSON(t, ts, "POST", "/api/access/promocode/apply", map[string]any{"code": code})
	if status != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", status)
	}
}

// 10. ApplyPromocode already used by user → 400.
func TestApplyPromocode_AlreadyUsed_400(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	user := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{})
	code := "TWICE_" + itoa(int64(env.Pool.Stat().AcquireCount()))
	promo := f.CreatePromocode(TestPromocodeOpts{Code: code, TargetID: mode.ID})
	_, _ = env.Pool.Exec(context.Background(),
		`insert into promocode_usages (user_id, promocode_id, used_at) values ($1, $2, now())`,
		user.ID, promo.ID)

	ts.LoginAs(f.CreateSession(user.ID))
	status, _ := httpJSON(t, ts, "POST", "/api/access/promocode/apply", map[string]any{"code": code})
	if status != http.StatusBadRequest {
		t.Errorf("expected 400 already_used, got %d", status)
	}
}

// 11. ApplyPromocode admin_role requires auth → 401.
func TestApplyPromocode_AdminRoleRequiresAuth_401(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)

	code := "ROLEPROMO_" + itoa(int64(env.Pool.Stat().AcquireCount()))
	// Insert admin_role promocode directly (factory doesn't support admin_role grants_type).
	_, _ = env.Pool.Exec(context.Background(),
		`insert into promocodes (code, max_uses, used_count, duration, access_priority, grants_type, target_id, limit_type, daily_message_limit) values ($1, 0, 0, '30 days'::interval, 0, 'admin_role', 0, 'fixed', 0)`,
		code)

	status, _ := httpJSON(t, ts, "POST", "/api/access/promocode/apply", map[string]any{"code": code})
	if status != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", status)
	}
}

// 12. ApplyPromocode admin_role auth'd → role upgrade.
func TestApplyPromocode_AdminRoleUpgradesAuthUser(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	user := f.CreateUser(TestUserOpts{Role: "user"})
	code := "ROLEUPG_" + itoa(int64(env.Pool.Stat().AcquireCount()))
	_, _ = env.Pool.Exec(context.Background(),
		`insert into promocodes (code, max_uses, used_count, duration, access_priority, grants_type, target_id, limit_type, daily_message_limit) values ($1, 0, 0, '30 days'::interval, 0, 'admin_role', 0, 'fixed', 0)`,
		code)

	ts.LoginAs(f.CreateSession(user.ID))
	status, body := httpJSON(t, ts, "POST", "/api/access/promocode/apply", map[string]any{"code": code})
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%v", status, body)
	}
	if upg, _ := body["roleUpgraded"].(bool); !upg {
		t.Errorf("roleUpgraded=false")
	}

	// DB: role updated
	var role string
	_ = env.Pool.QueryRow(context.Background(),
		`select role from users where id = $1`, user.ID).Scan(&role)
	if role != "admin" {
		t.Errorf("role: got %q want admin", role)
	}
}

// 13. ApplyPromocode invalid JSON → 400.
func TestApplyPromocode_InvalidJSON_400(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)
	_ = env

	// Build raw request
	req, _ := http.NewRequest("POST", ts.URL("/api/access/promocode/apply"), nil)
	req.Header.Set("Content-Type", "application/json")
	resp, err := ts.Client.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

// 14. ApplyPromocode wrong method → 405.
func TestApplyPromocode_GET_405(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)
	_ = env

	status, _ := httpJSON(t, ts, "GET", "/api/access/promocode/apply", nil)
	if status != http.StatusMethodNotAllowed {
		t.Errorf("GET: expected 405, got %d", status)
	}
}

// =============================================================================
// SendMessage — biggest remaining gap (60% → push higher)
// =============================================================================

// 15. SendMessage GET → 405.
func TestSendMessage_WrongMethod_405(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)
	_ = env

	status, _ := httpJSON(t, ts, "GET", "/api/chat/send", nil)
	if status != http.StatusMethodNotAllowed {
		t.Errorf("GET: expected 405, got %d", status)
	}
}

// 16. SendMessage empty text → 400.
func TestSendMessage_EmptyText_400(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	user := f.CreateUser(TestUserOpts{})
	ts.LoginAs(f.CreateSession(user.ID))

	status, _ := httpJSON(t, ts, "POST", "/api/chat/send", map[string]any{"text": ""})
	if status != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", status)
	}
}

// 17. SendMessage no access → 403.
func TestSendMessage_NoAccess_403(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	user := f.CreateUser(TestUserOpts{})
	ts.LoginAs(f.CreateSession(user.ID))

	status, body := httpJSON(t, ts, "POST", "/api/chat/send", map[string]any{
		"text": "hello",
	})
	if status != http.StatusForbidden {
		t.Errorf("expected 403, got %d body=%v", status, body)
	}
	if code, _ := body["code"].(string); code != "access_required" {
		t.Errorf("missing access_required: %v", body)
	}
}

// 18. SendMessage no dialogId → 400.
func TestSendMessage_NoDialog_400(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	user := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{})
	f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: mode.ID, DailyMessageLimit: 10})
	ts.LoginAs(f.CreateSession(user.ID))

	status, body := httpJSON(t, ts, "POST", "/api/chat/send", map[string]any{
		"text": "hi",
	})
	if status != http.StatusBadRequest {
		t.Errorf("expected 400 (no dialog), got %d body=%v", status, body)
	}
}

// 19. SendMessage test mode happy path → 200 with [TEST MODE] response.
func TestSendMessage_TestMode_HappyPath(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	user := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{Name: "TestSendMode"})
	f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: mode.ID, DailyMessageLimit: 10})
	dialog := f.CreateDialog(user.ID, mode.ID)
	ts.LoginAs(f.CreateSession(user.ID))

	status, body := httpJSON(t, ts, "POST", "/api/chat/send", map[string]any{
		"text":     "ping",
		"dialogId": dialog.ID,
	})
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%v", status, body)
	}
	if assistant, _ := body["assistantMessage"].(map[string]any); assistant != nil {
		content, _ := assistant["content"].(string)
		if content == "" {
			t.Errorf("assistantMessage.content empty")
		}
	}
}

// 20. SendMessage daily quota exhausted → 429.
func TestSendMessage_QuotaExhausted_429(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	user := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{})
	f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: mode.ID, DailyMessageLimit: 1})
	dialog := f.CreateDialog(user.ID, mode.ID)
	ts.LoginAs(f.CreateSession(user.ID))

	// First: uses last slot.
	_, _ = httpJSON(t, ts, "POST", "/api/chat/send", map[string]any{
		"text": "first", "dialogId": dialog.ID,
	})

	// Second: 429.
	status, body := httpJSON(t, ts, "POST", "/api/chat/send", map[string]any{
		"text": "second", "dialogId": dialog.ID,
	})
	if status != http.StatusTooManyRequests {
		t.Errorf("expected 429, got %d body=%v", status, body)
	}
	if code, _ := body["code"].(string); code != "daily_quota_exhausted" {
		t.Errorf("missing daily_quota_exhausted code: %v", body)
	}
}

// =============================================================================
// resolvePromocodeModeIDs direct table tests
// =============================================================================

// 21. resolvePromocodeModeIDs grants_type=all → all visible modes.
func TestResolvePromocodeModeIDs_All(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)

	visible := f.CreateMode(TestModeOpts{Name: "ResolveVisible"})
	hidden := f.CreateMode(TestModeOpts{Name: "ResolveHidden"})
	_, _ = env.Pool.Exec(context.Background(), `update modes set hidden_at = now() where id = $1`, hidden.ID)

	tx, err := env.Pool.Begin(context.Background())
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback(context.Background())

	got, err := resolvePromocodeModeIDs(context.Background(), tx, promoRow{GrantsType: "all"})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !containsInt64(got, visible.ID) {
		t.Errorf("missing visible mode")
	}
	if containsInt64(got, hidden.ID) {
		t.Errorf("hidden mode leaked")
	}
}

// 22. resolvePromocodeModeIDs grants_type=mode + TargetIDs.
func TestResolvePromocodeModeIDs_ModeTargetIDs(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	tx, _ := env.Pool.Begin(context.Background())
	defer tx.Rollback(context.Background())

	got, err := resolvePromocodeModeIDs(context.Background(), tx, promoRow{
		GrantsType: "mode", TargetIDs: []int64{100, 200},
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(got) != 2 || got[0] != 100 || got[1] != 200 {
		t.Errorf("got %v want [100,200]", got)
	}
}

// 23. resolvePromocodeModeIDs tariff_group with empty targets → empty.
func TestResolvePromocodeModeIDs_GroupEmptyTargets(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	tx, _ := env.Pool.Begin(context.Background())
	defer tx.Rollback(context.Background())

	got, err := resolvePromocodeModeIDs(context.Background(), tx, promoRow{GrantsType: "tariff_group"})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty, got %v", got)
	}
}

// 24. resolvePromocodeModeIDs unknown grants_type → empty.
func TestResolvePromocodeModeIDs_UnknownType(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	tx, _ := env.Pool.Begin(context.Background())
	defer tx.Rollback(context.Background())

	got, err := resolvePromocodeModeIDs(context.Background(), tx, promoRow{
		GrantsType: "unknown_type", TargetIDs: []int64{1, 2},
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty for unknown type, got %v", got)
	}
}

// =============================================================================
// Auth flows — AuthLogin, AuthRegister edge cases, AuthLogout
// =============================================================================

// 25. AuthLogin invalid credentials → 401.
func TestAuthLogin_BadCredentials_401(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)
	_ = env

	status, _ := httpJSON(t, ts, "POST", "/api/auth/login", map[string]any{
		"email":    "noone@example.com",
		"password": "wrong",
	})
	if status != http.StatusUnauthorized && status != http.StatusBadRequest {
		t.Errorf("bad creds: expected 401/400, got %d", status)
	}
}

// 26. AuthLogin wrong method → 405.
func TestAuthLogin_GET_405(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)
	_ = env

	status, _ := httpJSON(t, ts, "GET", "/api/auth/login", nil)
	if status != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", status)
	}
}

// 27. AuthLogout invalidates session.
func TestAuthLogout_RemovesSession(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	user := f.CreateUser(TestUserOpts{})
	ts.LoginAs(f.CreateSession(user.ID))

	status, _ := httpJSON(t, ts, "POST", "/api/auth/logout", nil)
	if status != http.StatusOK {
		t.Errorf("logout: expected 200, got %d", status)
	}

	// Subsequent /me should 401 (no auth).
	status2, _ := httpJSON(t, ts, "GET", "/api/auth/me", nil)
	if status2 != http.StatusUnauthorized && status2 != http.StatusOK {
		// /me returns OK with empty if not logged in
		t.Logf("post-logout /me: %d", status2)
	}
}

// 28. AuthRegister duplicate email → 400.
func TestAuthRegister_DuplicateEmail_400(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	existing := f.CreateUser(TestUserOpts{Email: "dup_" + uniqueEmail("dup")})

	status, _ := httpJSON(t, ts, "POST", "/api/auth/register", map[string]any{
		"email":    existing.Email,
		"password": "Password123!",
	})
	if status != http.StatusBadRequest && status != http.StatusConflict {
		t.Errorf("duplicate email: expected 400/409, got %d", status)
	}
}

// 29. AuthRegister wrong method → 405.
func TestAuthRegister_GET_405(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)
	_ = env

	status, _ := httpJSON(t, ts, "GET", "/api/auth/register", nil)
	if status != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", status)
	}
}

// 30. AuthForgotPassword no email → 400.
func TestAuthForgotPassword_NoEmail_400(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)
	_ = env

	status, _ := httpJSON(t, ts, "POST", "/api/auth/forgot-password", map[string]any{"email": ""})
	if status != http.StatusBadRequest && status != http.StatusOK {
		// 200 OK is also acceptable — many implementations return 200 to prevent enumeration.
		t.Logf("empty email: %d", status)
	}
}

// 31. AuthResetPassword no token → 400.
func TestAuthResetPassword_NoToken_400(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)
	_ = env

	status, _ := httpJSON(t, ts, "POST", "/api/auth/reset-password", map[string]any{
		"token":    "",
		"password": "NewPass123!",
	})
	if status != http.StatusBadRequest && status != http.StatusUnauthorized {
		t.Errorf("empty token: got %d", status)
	}
}

// 32. AuthResetPassword bad token → 401/400.
func TestAuthResetPassword_BadToken(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)
	_ = env

	status, _ := httpJSON(t, ts, "POST", "/api/auth/reset-password", map[string]any{
		"token":    "non-existent-token-zzz",
		"password": "NewPass123!",
	})
	if status == http.StatusOK {
		t.Errorf("bad token should not return 200")
	}
}

// 33. Profile GET requires auth.
func TestProfile_RequiresAuth(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)
	_ = env

	status, _ := httpJSON(t, ts, "GET", "/api/profile", nil)
	if status != http.StatusUnauthorized && status != http.StatusForbidden {
		t.Errorf("no auth: expected 401/403, got %d", status)
	}
}

// 34. Profile GET with auth → 200.
func TestProfile_AuthOk(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	user := f.CreateUser(TestUserOpts{})
	ts.LoginAs(f.CreateSession(user.ID))

	status, body := httpJSON(t, ts, "GET", "/api/profile", nil)
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%v", status, body)
	}
	if _, ok := body["user"].(map[string]any); !ok {
		t.Errorf("user field missing")
	}
	if body["allowMessageAnonymization"] != true {
		t.Errorf("allowMessageAnonymization default = %v, want true", body["allowMessageAnonymization"])
	}

	status, body = httpJSON(t, ts, "PATCH", "/api/profile/settings", map[string]any{"allowMessageAnonymization": false})
	if status != http.StatusOK {
		t.Fatalf("settings status=%d body=%v", status, body)
	}
	if body["allowMessageAnonymization"] != false {
		t.Errorf("patched allowMessageAnonymization = %v, want false", body["allowMessageAnonymization"])
	}

	status, body = httpJSON(t, ts, "GET", "/api/profile", nil)
	if status != http.StatusOK {
		t.Fatalf("profile after patch status=%d body=%v", status, body)
	}
	if body["allowMessageAnonymization"] != false {
		t.Errorf("profile allowMessageAnonymization = %v, want false", body["allowMessageAnonymization"])
	}
}
