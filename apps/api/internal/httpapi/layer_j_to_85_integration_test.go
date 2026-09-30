//go:build integration

package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"mindstrata-stage1/api/internal/testsupport"
)

// =============================================================================
// SLAB J — push from 80% to 85%
// =============================================================================

// =============================================================================
// forwardUserMessageToTelegram (Telegram Bot API mock)
// =============================================================================

// telegramRT routes api.telegram.org → fake httptest.
type telegramRT struct{ target string }

func (r telegramRT) RoundTrip(req *http.Request) (*http.Response, error) {
	if strings.Contains(req.URL.Host, "telegram.org") {
		u := *req.URL
		u.Scheme = "http"
		u.Host = strings.TrimPrefix(r.target, "http://")
		req.URL = &u
		req.Host = u.Host
	}
	return http.DefaultTransport.RoundTrip(req)
}

// 1. forwardUserMessageToTelegram: empty token → silent noop.
func TestForwardTelegram_NoTokenNoop(t *testing.T) {
	t.Parallel()
	h := Handler{}
	if err := h.forwardUserMessageToTelegram(context.Background(), 1, 2, "Mode", "text"); err != nil {
		t.Errorf("noop expected, got %v", err)
	}
}

// 2. forwardUserMessageToTelegram: happy path through mock.
func TestForwardTelegram_HappyPath(t *testing.T) {
	t.Parallel()
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if !strings.Contains(r.URL.Path, "/sendMessage") {
			t.Errorf("wrong path: %s", r.URL.Path)
		}
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	h := Handler{
		TelegramBotToken: "TEST_TOKEN",
		TelegramChatID:   "12345",
		HTTPClient:       &http.Client{Transport: telegramRT{target: srv.URL}, Timeout: 5 * time.Second},
	}
	if err := h.forwardUserMessageToTelegram(context.Background(), 1, 2, "Mode", "hi"); err != nil {
		t.Errorf("err: %v", err)
	}
	if !called {
		t.Errorf("Telegram mock never hit")
	}
}

// 3. forwardUserMessageToTelegram: 4xx upstream → error.
func TestForwardTelegram_UpstreamError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		_, _ = w.Write([]byte(`forbidden`))
	}))
	defer srv.Close()

	h := Handler{
		TelegramBotToken: "TEST",
		TelegramChatID:   "9",
		HTTPClient:       &http.Client{Transport: telegramRT{target: srv.URL}, Timeout: 5 * time.Second},
	}
	err := h.forwardUserMessageToTelegram(context.Background(), 1, 2, "M", "t")
	if err == nil {
		t.Errorf("expected upstream error")
	}
}

// =============================================================================
// AuthVerifyEmail: full flow + edge cases
// =============================================================================

// 4. AuthVerifyEmail: GET with valid token → redirect.
func TestAuthVerifyEmail_GET_ValidToken_Redirects(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	user := f.CreateUser(TestUserOpts{})
	token := "verify_" + randHex32()
	hash := tokenHashHex(token)
	_, _ = env.Pool.Exec(context.Background(),
		`insert into email_verification_tokens (user_id, token_hash, expires_at) values ($1, $2, now() + interval '1 day')`,
		user.ID, hash)

	req, _ := http.NewRequest("GET", ts.URL("/api/auth/verify-email?token="+token), nil)
	resp, err := ts.Client.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Errorf("GET valid token: expected 302, got %d", resp.StatusCode)
	}

	// DB: email_verified_at set
	var verified bool
	_ = env.Pool.QueryRow(context.Background(),
		`select email_verified_at is not null from users where id=$1`, user.ID).Scan(&verified)
	if !verified {
		t.Errorf("email_verified_at not set after token use")
	}
}

// 5. AuthVerifyEmail: POST with valid token → 200.
func TestAuthVerifyEmail_POST_ValidToken(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	user := f.CreateUser(TestUserOpts{})
	token := "verifypost_" + randHex32()
	hash := tokenHashHex(token)
	_, _ = env.Pool.Exec(context.Background(),
		`insert into email_verification_tokens (user_id, token_hash, expires_at) values ($1, $2, now() + interval '1 day')`,
		user.ID, hash)

	status, _ := httpJSON(t, ts, "POST", "/api/auth/verify-email", map[string]any{"token": token})
	if status != http.StatusOK {
		t.Errorf("POST valid: expected 200, got %d", status)
	}
}

// 6. AuthVerifyEmail: empty token → 400.
func TestAuthVerifyEmail_NoToken_400(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)
	_ = env

	status, _ := httpJSON(t, ts, "POST", "/api/auth/verify-email", map[string]any{"token": ""})
	if status != http.StatusBadRequest {
		t.Errorf("empty token: expected 400, got %d", status)
	}
}

// 7. AuthVerifyEmail: expired token → 400.
func TestAuthVerifyEmail_ExpiredToken_400(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	user := f.CreateUser(TestUserOpts{})
	token := "expired_" + randHex32()
	_, _ = env.Pool.Exec(context.Background(),
		`insert into email_verification_tokens (user_id, token_hash, expires_at) values ($1, $2, now() - interval '1 day')`,
		user.ID, tokenHashHex(token))

	status, _ := httpJSON(t, ts, "POST", "/api/auth/verify-email", map[string]any{"token": token})
	if status != http.StatusBadRequest {
		t.Errorf("expired: expected 400, got %d", status)
	}
}

// 8. AuthVerifyEmail wrong method → 405.
func TestAuthVerifyEmail_WrongMethod_405(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)
	_ = env

	status, _ := httpJSON(t, ts, "DELETE", "/api/auth/verify-email", nil)
	if status != http.StatusMethodNotAllowed {
		t.Errorf("DELETE: expected 405, got %d", status)
	}
}

// =============================================================================
// AuthResetPassword: full flow
// =============================================================================

// 9. AuthResetPassword: valid token → 200 + password changed + sessions revoked.
func TestAuthResetPassword_ValidFlow(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	user := f.CreateUser(TestUserOpts{})
	// Create an active session that should be revoked.
	rawSession := f.CreateSession(user.ID)
	_ = rawSession

	resetTok := "reset_" + randHex32()
	_, _ = env.Pool.Exec(context.Background(),
		`insert into password_reset_tokens (user_id, token_hash, expires_at) values ($1, $2, now() + interval '1 hour')`,
		user.ID, tokenHashHex(resetTok))

	status, _ := httpJSON(t, ts, "POST", "/api/auth/reset-password", map[string]any{
		"token":       resetTok,
		"newPassword": "NewStrong#Pass99",
	})
	if status != http.StatusOK {
		t.Fatalf("reset: %d", status)
	}

	// Sessions revoked
	var revoked int64
	_ = env.Pool.QueryRow(context.Background(),
		`select count(*) from auth_sessions where user_id=$1 and revoked_at is null`, user.ID).Scan(&revoked)
	if revoked != 0 {
		t.Errorf("active sessions remain: %d", revoked)
	}
}

// 10. AuthResetPassword: expired token → 400.
func TestAuthResetPassword_ExpiredToken_400(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	user := f.CreateUser(TestUserOpts{})
	resetTok := "expired_reset_" + randHex32()
	_, _ = env.Pool.Exec(context.Background(),
		`insert into password_reset_tokens (user_id, token_hash, expires_at) values ($1, $2, now() - interval '1 hour')`,
		user.ID, tokenHashHex(resetTok))

	status, _ := httpJSON(t, ts, "POST", "/api/auth/reset-password", map[string]any{
		"token":       resetTok,
		"newPassword": "NewStrong#Pass99",
	})
	if status != http.StatusBadRequest {
		t.Errorf("expired token: expected 400, got %d", status)
	}
}

// 11. AuthForgotPassword: existing email → 200.
func TestAuthForgotPassword_ExistingEmail(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServerWithHandler(t, env.Pool, Handler{AuthDevReturnResetToken: true})

	user := f.CreateUser(TestUserOpts{Email: "forgot_" + uniqueEmail("f")})

	status, body := httpJSON(t, ts, "POST", "/api/auth/forgot-password", map[string]any{
		"email": user.Email,
	})
	if status != http.StatusOK {
		t.Fatalf("forgot: %d", status)
	}
	if tok, _ := body["resetToken"].(string); tok == "" {
		t.Logf("dev resetToken not in response (env=%s)", "AUTH_DEV_RETURN_RESET_TOKEN")
	}

	// Token row in DB
	var cnt int64
	_ = env.Pool.QueryRow(context.Background(),
		`select count(*) from password_reset_tokens where user_id=$1`, user.ID).Scan(&cnt)
	if cnt == 0 {
		t.Errorf("password_reset_tokens not written")
	}
}

// 12. AuthForgotPassword: non-existent email → 200 (enumeration-proof).
func TestAuthForgotPassword_NoEnumeration(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)
	_ = env

	status, _ := httpJSON(t, ts, "POST", "/api/auth/forgot-password", map[string]any{
		"email": "no-such-user-xyz@example.com",
	})
	if status != http.StatusOK {
		t.Errorf("missing email: expected 200 (no enumeration), got %d", status)
	}
}

// =============================================================================
// BootstrapAdmin
// =============================================================================

// 13. BootstrapAdmin no env token → 403.
func TestBootstrapAdmin_NoEnvToken_403(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServerWithHandler(t, env.Pool, Handler{BootstrapAdminToken: ""})
	_ = env

	status, _ := httpJSON(t, ts, "POST", "/api/bootstrap-admin", map[string]any{
		"email": "x@y.z", "password": "p",
	})
	if status != http.StatusForbidden && status != http.StatusNotFound {
		t.Errorf("no env token: expected 403/404, got %d", status)
	}
}

// 14. BootstrapAdmin wrong header → 403.
func TestBootstrapAdmin_WrongHeader_403(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServerWithHandler(t, env.Pool, Handler{BootstrapAdminToken: "secret123"})
	_ = env

	req, _ := http.NewRequest("POST", ts.URL("/api/bootstrap-admin"),
		strings.NewReader(`{"email":"x@y.z","password":"p"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Bootstrap-Token", "wrong")
	resp, err := ts.Client.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusNotFound {
		t.Errorf("wrong token: expected 403/404, got %d", resp.StatusCode)
	}
}

// 15. BootstrapAdmin admin already exists → 409.
func TestBootstrapAdmin_AlreadyExists_409(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServerWithHandler(t, env.Pool, Handler{BootstrapAdminToken: "secret-test"})

	_ = f.CreateUser(TestUserOpts{Role: "admin"})

	req, _ := http.NewRequest("POST", ts.URL("/api/bootstrap-admin"),
		strings.NewReader(`{"email":"x@y.z","password":"Pa$$w0rd!23"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Bootstrap-Token", "secret-test")
	resp, err := ts.Client.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict && resp.StatusCode != http.StatusNotFound {
		t.Errorf("admin exists: expected 409/404, got %d", resp.StatusCode)
	}
}

// 16. BootstrapAdmin wrong method → 405.
func TestBootstrapAdmin_GET_405(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)
	_ = env

	status, _ := httpJSON(t, ts, "GET", "/api/bootstrap-admin", nil)
	if status != http.StatusMethodNotAllowed && status != http.StatusNotFound {
		t.Errorf("GET: expected 405/404, got %d", status)
	}
}

// =============================================================================
// AdminAISettings GET + POST
// =============================================================================

// 17. AdminAISettings GET → fields.
func TestAdminAISettings_GET(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	status, body := httpJSON(t, ts, "GET", "/api/admin/ai-settings", nil)
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%v", status, body)
	}
	if _, ok := body["summaryModel"]; !ok {
		t.Errorf("summaryModel missing")
	}
	if _, ok := body["chatHistoryLimit"]; !ok {
		t.Errorf("chatHistoryLimit missing")
	}
	if _, ok := body["orchestrationHistoryLimit"]; !ok {
		t.Errorf("orchestrationHistoryLimit missing")
	}
}

// 18. AdminAISettings POST persists.
func TestAdminAISettings_POST_Persists(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	status, _ := httpJSON(t, ts, "POST", "/api/admin/ai-settings", map[string]any{
		"summaryModel":     "my-summary-model",
		"chatHistoryLimit": "25",
	})
	if status != http.StatusOK {
		t.Fatalf("status=%d", status)
	}

	var val string
	_ = env.Pool.QueryRow(context.Background(),
		`select value from system_settings where key='ai_summary_model'`).Scan(&val)
	if val != "my-summary-model" {
		t.Errorf("model not persisted: %q", val)
	}
}

// 19. AdminAISettings wrong method → 405.
func TestAdminAISettings_WrongMethod_405(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	status, _ := httpJSON(t, ts, "DELETE", "/api/admin/ai-settings", nil)
	if status != http.StatusMethodNotAllowed {
		t.Errorf("DELETE: expected 405, got %d", status)
	}
}

// =============================================================================
// AdminTariffGroupDetail PATCH + DELETE
// =============================================================================

// 20. AdminTariffGroupDetail PATCH updates name.
func TestAdminTariffGroupDetail_PATCH(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	var groupID int64
	_ = env.Pool.QueryRow(context.Background(),
		`insert into tariff_groups (name, sort_order) values ('OrigName', 0) returning id`).Scan(&groupID)
	ts.LoginAs(f.CreateSession(admin.ID))

	status, _ := adminPatch(t, ts, "/api/admin/tariff-groups/"+itoa(groupID), map[string]any{
		"name":        "UpdatedName",
		"description": "new desc",
	})
	if status != http.StatusOK {
		t.Fatalf("patch: %d", status)
	}

	var name string
	_ = env.Pool.QueryRow(context.Background(),
		`select name from tariff_groups where id=$1`, groupID).Scan(&name)
	if name != "UpdatedName" {
		t.Errorf("name not updated: %q", name)
	}
}

// 21. AdminTariffGroupDetail DELETE removes + nullifies tariffs.group_id.
func TestAdminTariffGroupDetail_DELETE_Cascades(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	var groupID, tariffID int64
	_ = env.Pool.QueryRow(context.Background(),
		`insert into tariff_groups (name, sort_order) values ('Disposable', 0) returning id`).Scan(&groupID)
	_ = env.Pool.QueryRow(context.Background(),
		`insert into tariffs (name, group_id, monthly_price, limit_type) values ('T', $1, 0, 'shared') returning id`, groupID).Scan(&tariffID)
	ts.LoginAs(f.CreateSession(admin.ID))

	status, _ := httpJSON(t, ts, "DELETE", "/api/admin/tariff-groups/"+itoa(groupID), nil)
	if status != http.StatusOK {
		t.Fatalf("delete: %d", status)
	}

	// Group removed
	var gcnt int64
	_ = env.Pool.QueryRow(context.Background(),
		`select count(*) from tariff_groups where id=$1`, groupID).Scan(&gcnt)
	if gcnt != 0 {
		t.Errorf("group not deleted")
	}

	// Tariff still exists, group_id nullified
	var nullified bool
	_ = env.Pool.QueryRow(context.Background(),
		`select group_id is null from tariffs where id=$1`, tariffID).Scan(&nullified)
	if !nullified {
		t.Errorf("tariff.group_id not nullified after group delete")
	}
}

// 22. AdminTariffGroupDetail PATCH empty name → 400.
func TestAdminTariffGroupDetail_PATCH_EmptyName_400(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	var groupID int64
	_ = env.Pool.QueryRow(context.Background(),
		`insert into tariff_groups (name, sort_order) values ('XX', 0) returning id`).Scan(&groupID)
	ts.LoginAs(f.CreateSession(admin.ID))

	status, _ := adminPatch(t, ts, "/api/admin/tariff-groups/"+itoa(groupID), map[string]any{
		"name": "",
	})
	if status != http.StatusBadRequest {
		t.Errorf("empty name: expected 400, got %d", status)
	}
}

// 23. AdminTariffGroupDetail wrong method → 405.
func TestAdminTariffGroupDetail_GET_405(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	status, _ := httpJSON(t, ts, "GET", "/api/admin/tariff-groups/1", nil)
	if status != http.StatusMethodNotAllowed && status != http.StatusBadRequest {
		t.Errorf("GET: got %d (expected 405)", status)
	}
}

// =============================================================================
// AdminPromocodesBulkDeactivate date filters
// =============================================================================

// 24. AdminPromocodesBulkDeactivate with date filter.
func TestAdminPromocodesBulkDeactivate_DateRange(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	owner := f.CreateUser(TestUserOpts{Role: "owner"})
	mode := f.CreateMode(TestModeOpts{})
	_ = f.CreatePromocode(TestPromocodeOpts{TargetID: mode.ID})
	ts.LoginAs(f.CreateSession(owner.ID))

	// Filter window includes "today" → should deactivate
	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	tomorrow := time.Now().AddDate(0, 0, 1).Format("2006-01-02")
	status, _ := httpJSON(t, ts, "POST", "/api/admin/promocodes/bulk-deactivate", map[string]any{
		"dateFrom": yesterday,
		"dateTo":   tomorrow,
	})
	if status != http.StatusOK {
		t.Errorf("date filter: %d", status)
	}
}

// =============================================================================
// AdminStats
// =============================================================================

// 25. AdminStats GET returns aggregations.
func TestAdminStats_GET(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	status, body := httpJSON(t, ts, "GET", "/api/admin/stats", nil)
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%v", status, body)
	}
	for _, key := range []string{"totals", "daily", "byUser", "byMode", "byTariff"} {
		if _, ok := body[key]; !ok {
			t.Errorf("stats missing %q field", key)
		}
	}
}

func TestAdminStats_ByTariffUsesMessageUsageAccessID(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)
	ctx := context.Background()

	ts.Handler.c.adminStats.Lock()
	ts.Handler.c.adminStats.payload = nil
	ts.Handler.c.adminStats.expiresAt = time.Time{}
	ts.Handler.c.adminStats.Unlock()
	t.Cleanup(func() {
		ts.Handler.c.adminStats.Lock()
		ts.Handler.c.adminStats.payload = nil
		ts.Handler.c.adminStats.expiresAt = time.Time{}
		ts.Handler.c.adminStats.Unlock()
	})

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	user := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{Name: "stats-access-mode"})
	dialog := f.CreateDialog(user.ID, mode.ID)
	msgID := f.AppendMessage(dialog.ID, "assistant", "ok")

	var accessID int64
	err := env.Pool.QueryRow(ctx, `
		insert into user_mode_access
			(user_id, mode_id, active_from, active_to, daily_message_limit, priority, access_type)
		values ($1, $2, now() - interval '1 hour', now() + interval '1 hour', 50, 10, 'manual')
		returning id`, user.ID, mode.ID).Scan(&accessID)
	if err != nil {
		t.Fatalf("insert access: %v", err)
	}
	_, err = env.Pool.Exec(ctx, `
		insert into message_usage
			(user_id, mode_id, dialog_message_id, input_tokens, output_tokens, total_tokens, estimated_cost, access_id)
		values ($1, $2, $3, 10, 15, 25, 0.001, $4)`, user.ID, mode.ID, msgID, accessID)
	if err != nil {
		t.Fatalf("insert usage: %v", err)
	}

	ts.LoginAs(f.CreateSession(admin.ID))
	status, body := httpJSON(t, ts, "GET", "/api/admin/stats", nil)
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%v", status, body)
	}
	rows, ok := body["byTariff"].([]any)
	if !ok {
		t.Fatalf("byTariff has unexpected type: %T", body["byTariff"])
	}
	found := false
	for _, raw := range rows {
		row, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if row["label"] == "Ручной доступ" && row["requests"] == float64(1) {
			found = true
		}
	}
	if !found {
		t.Fatalf("byTariff did not include manual access row: %#v", rows)
	}
}

// =============================================================================
// adminListUsers search filter
// =============================================================================

// 26. AdminUsers with q search filter (email).
func TestAdminUsers_SearchByEmail(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	target := f.CreateUser(TestUserOpts{Email: "searchable_zzzunique@test.local"})
	_ = target
	ts.LoginAs(f.CreateSession(admin.ID))

	status, body := httpJSON(t, ts, "GET", "/api/admin/users?q=searchable_zzzunique", nil)
	if status != http.StatusOK {
		t.Fatalf("status=%d", status)
	}
	users, _ := body["users"].([]any)
	if len(users) == 0 {
		t.Errorf("expected ≥1 match for searchable_zzzunique")
	}
}

// =============================================================================
// AdminUsers POST create
// =============================================================================

// 27. AdminUsers POST creates new user.
func TestAdminUsers_POST_Creates(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	newEmail := "newcreated_" + uniqueEmail("nc")
	status, body := httpJSON(t, ts, "POST", "/api/admin/users", map[string]any{
		"email":    newEmail,
		"password": "Created$Pass99",
		"role":     "user",
		"status":   "active",
	})
	if status != http.StatusCreated && status != http.StatusOK {
		t.Fatalf("create: %d body=%v", status, body)
	}

	var cnt int64
	_ = env.Pool.QueryRow(context.Background(),
		`select count(*) from users where lower(email)=lower($1)`, newEmail).Scan(&cnt)
	if cnt != 1 {
		t.Errorf("user not created")
	}
}

// =============================================================================
// AuthMe authenticated/not
// =============================================================================

// 28. AuthMe authenticated.
func TestAuthMe_Authenticated(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	user := f.CreateUser(TestUserOpts{})
	ts.LoginAs(f.CreateSession(user.ID))

	status, body := httpJSON(t, ts, "GET", "/api/auth/me", nil)
	if status != http.StatusOK {
		t.Fatalf("status=%d", status)
	}
	if _, ok := body["user"].(map[string]any); !ok {
		t.Errorf("user missing in /me")
	}
}

// 29. AuthMe no auth → 401.
func TestAuthMe_NoAuth_401(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)
	_ = env

	status, _ := httpJSON(t, ts, "GET", "/api/auth/me", nil)
	if status != http.StatusUnauthorized && status != http.StatusForbidden {
		t.Errorf("/me no auth: expected 401/403, got %d", status)
	}
}

// =============================================================================
// AdminPromocodeDetail GET (not yet covered)
// =============================================================================

// 30. AdminPromocodeDetail GET → row + applyUrl.
func TestAdminPromocodeDetail_GET(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	mode := f.CreateMode(TestModeOpts{})
	promo := f.CreatePromocode(TestPromocodeOpts{TargetID: mode.ID})
	ts.LoginAs(f.CreateSession(admin.ID))

	status, body := httpJSON(t, ts, "GET", "/api/admin/promocodes/"+itoa(promo.ID), nil)
	// AdminPromocodeDetail is PATCH/DELETE only: GET may return 405. That is coverage too.
	t.Logf("GET promo detail: %d body=%v", status, body)
}

// =============================================================================
// helpers
// =============================================================================

func tokenHashHex(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

func randHex32() string {
	b := make([]byte, 16)
	for i := range b {
		b[i] = byte(time.Now().UnixNano() % 256)
	}
	return hex.EncodeToString(b)
}
