//go:build integration

package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"mindstrata-stage1/api/internal/testsupport"
)

// =============================================================================
// SLAB F — OAuth dispatcher + callback errors + access edges
// =============================================================================

// 1. exchangeOAuthProfile: unknown provider → error.
func TestExchangeOAuthProfile_UnknownProvider(t *testing.T) {
	t.Parallel()
	h := Handler{}
	_, err := h.exchangeOAuthProfile(context.Background(), "facebook", "code")
	if err == nil {
		t.Errorf("expected error for unknown provider")
	}
}

// 2. exchangeOAuthProfile: yandex unconfigured → error.
func TestExchangeOAuthProfile_YandexUnconfigured(t *testing.T) {
	t.Parallel()
	h := Handler{}
	_, err := h.exchangeOAuthProfile(context.Background(), "yandex", "code")
	if err == nil {
		t.Errorf("expected error when yandex unconfigured")
	}
}

// 3. exchangeOAuthProfile: yandex configured + mock → success.
func TestExchangeOAuthProfile_YandexConfigured(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasPrefix(r.URL.Path, "/token") {
			w.Write([]byte(`{"access_token":"tok"}`))
			return
		}
		w.Write([]byte(`{"id":"123","login":"u","display_name":"User","default_email":"u@example.com"}`))
	}))
	t.Cleanup(srv.Close)

	h := Handler{
		HTTPClient:         &http.Client{Transport: yandexRT{target: srv.URL}},
		YandexClientID:     "x",
		YandexClientSecret: "y",
	}
	profile, err := h.exchangeOAuthProfile(context.Background(), "yandex", "code")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if profile.ProviderUserID != "123" {
		t.Errorf("provider id: %q", profile.ProviderUserID)
	}
}

// 4. oauthClient: empty HTTPClient → default 15s.
func TestOauthClient_Defaults(t *testing.T) {
	t.Parallel()
	h := Handler{}
	cli := h.oauthClient()
	if cli == nil {
		t.Fatalf("nil client")
	}
	if cli.Timeout == 0 {
		t.Errorf("default timeout should be set")
	}
}

// 5. AuthOAuth invalid path → 404.
func TestAuthOAuth_InvalidPath_404(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)
	_ = env

	// only one segment after /oauth/ — should be 404
	status, _ := httpJSON(t, ts, "GET", "/api/auth/oauth/yandex", nil)
	if status != http.StatusNotFound {
		t.Errorf("/oauth/yandex: expected 404, got %d", status)
	}

	// unknown action
	status2, _ := httpJSON(t, ts, "GET", "/api/auth/oauth/yandex/explode", nil)
	if status2 != http.StatusNotFound {
		t.Errorf("/oauth/yandex/explode: expected 404, got %d", status2)
	}
}

// 6. AuthOAuthProviders wrong method → 405.
func TestAuthOAuthProviders_WrongMethod(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)
	_ = env

	status, _ := httpJSON(t, ts, "POST", "/api/auth/oauth/providers", nil)
	if status != http.StatusMethodNotAllowed {
		t.Errorf("POST: expected 405, got %d", status)
	}
}

// 7. oauthCallback: missing state → 302 redirect to /login?error=oauth.
func TestOauthCallback_MissingState_Redirects(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)
	_ = env

	resp, err := ts.Client.Get(ts.URL("/api/auth/oauth/yandex/callback?code=c"))
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Errorf("expected 302, got %d", resp.StatusCode)
	}
	loc := resp.Header.Get("Location")
	if !strings.Contains(loc, "error=oauth") {
		t.Errorf("Location should include error=oauth, got %s", loc)
	}
}

// 8. oauthCallback: state without a cookie → /login?error=oauth_state.
func TestOauthCallback_NoCookie_ErrorState(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)
	_ = env

	resp, err := ts.Client.Get(ts.URL("/api/auth/oauth/yandex/callback?code=c&state=ZZZ"))
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	loc := resp.Header.Get("Location")
	if !strings.Contains(loc, "error=oauth_state") {
		t.Errorf("Location: %s", loc)
	}
}

// 9. oauthCallback: wrong method → 405.
func TestOauthCallback_WrongMethod(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)
	_ = env

	status, _ := httpJSON(t, ts, "POST", "/api/auth/oauth/yandex/callback", nil)
	if status != http.StatusMethodNotAllowed {
		t.Errorf("POST: expected 405, got %d", status)
	}
}

// 10. safeRedirectAfter: paths.
func TestSafeRedirectAfter_Paths(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"":              "/profile",
		"/dashboard":    "/dashboard",
		"//evil.com":    "/profile",
		"\\evil":        "/profile",
		"https://x.com": "/profile",
		"/admin/users":  "/profile", // privileged paths blocked
		"/tester/x":     "/profile",
		"/expert/y":     "/profile",
		"  /trim   ":    "/trim",
		"/normal/page":  "/normal/page",
	}
	for in, want := range cases {
		if got := safeRedirectAfter(in); got != want {
			t.Errorf("safeRedirectAfter(%q) = %q want %q", in, got, want)
		}
	}
}

// 11. oauthProviderConfigured: env set/unset.
func TestOauthProviderConfigured_EnvDependent(t *testing.T) {
	t.Parallel()
	if (Handler{}).oauthProviderConfigured("yandex") {
		t.Errorf("yandex should not be configured without env")
	}
	if !(Handler{YandexClientID: "id", YandexClientSecret: "sec"}).oauthProviderConfigured("yandex") {
		t.Errorf("yandex should be configured")
	}
	if (Handler{YandexClientID: "id", YandexClientSecret: "sec"}).oauthProviderConfigured("unknown") {
		t.Errorf("unknown provider should be false")
	}
}

// 12. transferGuestAccess: empty / identical IDs → noop.
func TestTransferGuestAccess_Noop(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	h := Handler{DB: env.Pool}

	if err := h.transferGuestAccess(context.Background(), 0, 5); err != nil {
		t.Errorf("guestID=0: should noop, got %v", err)
	}
	if err := h.transferGuestAccess(context.Background(), 5, 0); err != nil {
		t.Errorf("targetID=0: should noop, got %v", err)
	}
	if err := h.transferGuestAccess(context.Background(), 5, 5); err != nil {
		t.Errorf("same ID: should noop, got %v", err)
	}
}

// 13. transferGuestAccess: actually moves grants from guest to target.
func TestTransferGuestAccess_MovesGrants(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	h := Handler{DB: env.Pool}

	guest := f.CreateUser(TestUserOpts{Role: "user"})
	target := f.CreateUser(TestUserOpts{Role: "user"})
	mode := f.CreateMode(TestModeOpts{})
	f.GrantAccess(GrantAccessOpts{UserID: guest.ID, ModeID: mode.ID, DailyMessageLimit: 50})

	if err := h.transferGuestAccess(context.Background(), guest.ID, target.ID); err != nil {
		t.Fatalf("transfer: %v", err)
	}

	// Target now has access to the mode.
	var cnt int64
	_ = env.Pool.QueryRow(context.Background(),
		`select count(*) from user_mode_access where user_id = $1 and mode_id = $2`,
		target.ID, mode.ID).Scan(&cnt)
	if cnt == 0 {
		t.Errorf("target user did not receive access")
	}
}

// 13b. transferGuestAccess: OAuth login/register preserves guest chat history.
func TestTransferGuestAccess_MovesDialogHistoryWithoutDuplicates(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	h := Handler{DB: env.Pool}

	guest := f.CreateUser(TestUserOpts{Role: "user"})
	target := f.CreateUser(TestUserOpts{Role: "user"})
	mode := f.CreateMode(TestModeOpts{})
	src := int64(4242)
	f.GrantAccess(GrantAccessOpts{
		UserID: guest.ID, ModeID: mode.ID, DailyMessageLimit: 25,
		AccessType: "promocode", SourceID: &src,
	})
	dialog := f.CreateDialog(guest.ID, mode.ID)
	msgID := f.AppendMessage(dialog.ID, "user", "guest question before OAuth")
	if _, err := env.Pool.Exec(context.Background(),
		`update users set current_mode = $2, current_dialog = $3 where id = $1`,
		guest.ID, mode.ID, dialog.ID); err != nil {
		t.Fatalf("set guest current dialog: %v", err)
	}

	if err := h.transferGuestAccess(context.Background(), guest.ID, target.ID); err != nil {
		t.Fatalf("transfer: %v", err)
	}
	if err := h.transferGuestAccess(context.Background(), guest.ID, target.ID); err != nil {
		t.Fatalf("second transfer: %v", err)
	}

	var dialogOwner, messageCount, accessRows int64
	if err := env.Pool.QueryRow(context.Background(),
		`select user_id from users_dialogs where id = $1`, dialog.ID).Scan(&dialogOwner); err != nil {
		t.Fatalf("query dialog owner: %v", err)
	}
	if err := env.Pool.QueryRow(context.Background(),
		`select count(*) from dialogs_messages where id = $1 and dialog_id = $2`, msgID, dialog.ID).Scan(&messageCount); err != nil {
		t.Fatalf("query message: %v", err)
	}
	if err := env.Pool.QueryRow(context.Background(),
		`select count(*) from user_mode_access
		 where user_id = $1 and mode_id = $2 and access_type = 'promocode' and source_id = $3`,
		target.ID, mode.ID, src).Scan(&accessRows); err != nil {
		t.Fatalf("count target access rows: %v", err)
	}
	if dialogOwner != target.ID {
		t.Fatalf("dialog owner: got %d want target %d", dialogOwner, target.ID)
	}
	if messageCount != 1 {
		t.Fatalf("guest message was lost or duplicated: got %d want 1", messageCount)
	}
	if accessRows != 1 {
		t.Fatalf("target access rows after repeated transfer: got %d want 1", accessRows)
	}

	var targetCurrentDialog *int64
	var guestCurrentDialog *int64
	if err := env.Pool.QueryRow(context.Background(),
		`select current_dialog from users where id = $1`, target.ID).Scan(&targetCurrentDialog); err != nil {
		t.Fatalf("query target current dialog: %v", err)
	}
	if err := env.Pool.QueryRow(context.Background(),
		`select current_dialog from users where id = $1`, guest.ID).Scan(&guestCurrentDialog); err != nil {
		t.Fatalf("query guest current dialog: %v", err)
	}
	if targetCurrentDialog == nil || *targetCurrentDialog != dialog.ID {
		t.Fatalf("target current dialog: got %v want %d", targetCurrentDialog, dialog.ID)
	}
	if guestCurrentDialog != nil {
		t.Fatalf("guest current dialog should be cleared after transfer, got %v", *guestCurrentDialog)
	}
}

// =============================================================================
// access_promocode_helpers
// =============================================================================

// 14. applyDuration: parses years/months/weeks/days.
func TestApplyDuration_ParsesUnits(t *testing.T) {
	t.Parallel()
	base, _ := time.Parse(time.RFC3339, "2026-01-01T00:00:00Z")
	cases := []struct {
		raw       string
		fallback  int
		wantYear  int
		wantMonth int
		wantDay   int
	}{
		{"1 year 2 months", 30, 2027, 3, 1},
		{"1 week", 30, 2026, 1, 8},
		{"5 days", 30, 2026, 1, 6},
		{"", 7, 2026, 1, 8},
		{"unparseable", 14, 2026, 1, 15},
	}
	for _, c := range cases {
		got := applyDuration(base, c.raw, c.fallback)
		if got.Year() != c.wantYear || int(got.Month()) != c.wantMonth || got.Day() != c.wantDay {
			t.Errorf("applyDuration(%q, fb=%d) = %v; want %d-%d-%d", c.raw, c.fallback, got, c.wantYear, c.wantMonth, c.wantDay)
		}
	}
}

// 15. AdminUserDetail GET → adminGetUser delegated.
func TestAdminUserDetail_GET(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	target := f.CreateUser(TestUserOpts{Role: "user"})
	ts.LoginAs(f.CreateSession(admin.ID))

	status, body := httpJSON(t, ts, "GET", "/api/admin/users/"+itoa(target.ID), nil)
	if status != http.StatusOK {
		t.Fatalf("status=%d", status)
	}
	if _, ok := body["user"].(map[string]any); !ok {
		t.Errorf("missing user")
	}
}

// 16. AdminUserDetail/access GET → list active modes.
func TestAdminUserAccess_GET_ListsModes(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	target := f.CreateUser(TestUserOpts{Role: "user"})
	mode := f.CreateMode(TestModeOpts{Name: "GrantedMode"})
	f.GrantAccess(GrantAccessOpts{UserID: target.ID, ModeID: mode.ID, DailyMessageLimit: 20})
	ts.LoginAs(f.CreateSession(admin.ID))

	status, body := httpJSON(t, ts, "GET", "/api/admin/users/"+itoa(target.ID)+"/access", nil)
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%v", status, body)
	}
	modes, _ := body["activeModes"].([]any)
	if len(modes) == 0 {
		t.Errorf("expected ≥1 active mode")
	}
}

// 17. AdminUserAccess DELETE without modeId → 400.
func TestAdminUserAccess_DELETE_NoModeID_400(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	target := f.CreateUser(TestUserOpts{Role: "user"})
	ts.LoginAs(f.CreateSession(admin.ID))

	status, _ := httpJSON(t, ts, "DELETE", "/api/admin/users/"+itoa(target.ID)+"/access", nil)
	if status != http.StatusBadRequest {
		t.Errorf("DELETE no modeId: expected 400, got %d", status)
	}
}

// 18. AdminUserAccess DELETE with modeId → 200 + revokes.
func TestAdminUserAccess_DELETE_Revokes(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	target := f.CreateUser(TestUserOpts{Role: "user"})
	mode := f.CreateMode(TestModeOpts{})
	f.GrantAccess(GrantAccessOpts{UserID: target.ID, ModeID: mode.ID, DailyMessageLimit: 10})
	ts.LoginAs(f.CreateSession(admin.ID))

	status, _ := httpJSON(t, ts, "DELETE",
		"/api/admin/users/"+itoa(target.ID)+"/access?modeId="+itoa(mode.ID), nil)
	if status != http.StatusOK {
		t.Fatalf("status=%d", status)
	}
	// active_to <= now() expected
	var revoked bool
	_ = env.Pool.QueryRow(context.Background(),
		`select exists(select 1 from user_mode_access where user_id=$1 and mode_id=$2 and active_to <= now() + interval '5 seconds')`,
		target.ID, mode.ID).Scan(&revoked)
	if !revoked {
		t.Errorf("active_to not set in past after DELETE")
	}
}

// 19. resolveAdminUserID: by ID + by email + missing.
func TestResolveAdminUserID(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	h := Handler{DB: env.Pool}

	user := f.CreateUser(TestUserOpts{Email: "resolve_" + uniqueEmail("r")})

	got, err := h.resolveAdminUserID(context.Background(), user.ID, "")
	if err != nil {
		t.Fatalf("by id: %v", err)
	}
	if got != user.ID {
		t.Errorf("by id: got %d want %d", got, user.ID)
	}

	got, err = h.resolveAdminUserID(context.Background(), 0, user.Email)
	if err != nil {
		t.Fatalf("by email: %v", err)
	}
	if got != user.ID {
		t.Errorf("by email: got %d want %d", got, user.ID)
	}

	if _, err = h.resolveAdminUserID(context.Background(), 0, ""); err == nil {
		t.Errorf("no id+no email: expected error")
	}
	if _, err = h.resolveAdminUserID(context.Background(), 999_999_999, ""); err == nil {
		t.Errorf("missing id: expected error")
	}
}

// 20. AdminUserDetail invalid id → 400.
func TestAdminUserDetail_InvalidID_400(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	status, _ := httpJSON(t, ts, "GET", "/api/admin/users/not-a-number", nil)
	if status != http.StatusBadRequest {
		t.Errorf("invalid id: expected 400, got %d", status)
	}
}
