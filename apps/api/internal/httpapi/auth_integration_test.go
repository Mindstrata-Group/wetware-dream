//go:build integration

package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"mindstrata-stage1/api/internal/testsupport"
)

// httpJSON sends JSON to path and returns status code + parsed body.
func httpJSON(t testing.TB, ts *TestServer, method, path string, body any) (int, map[string]any) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		reader = bytes.NewReader(buf)
	}
	req, err := http.NewRequest(method, ts.URL(path), reader)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := ts.Client.Do(req)
	if err != nil {
		t.Fatalf("do request %s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	var parsed map[string]any
	raw, _ := io.ReadAll(resp.Body)
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &parsed)
	}
	return resp.StatusCode, parsed
}

// uniqueEmail returns an email that doesn't collide with other tests in the same session.
func uniqueEmail(prefix string) string {
	return fmt.Sprintf("%s_%d@test.local", prefix, idCounter.Add(1))
}

var idCounter = func() *uniqueCounter { return &uniqueCounter{} }()

type uniqueCounter struct {
	n atomic.Int64
}

func (c *uniqueCounter) Add(delta int64) int64 {
	return c.n.Add(delta)
}

// TestAuth_Register_HappyPath: new email + 10+ char password → 200, user in the DB.
func TestAuth_Register_HappyPath(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)

	email := uniqueEmail("happy")
	code, body := httpJSON(t, ts, "POST", "/api/auth/register", map[string]any{
		"email":    email,
		"password": "testpass123",
	})
	if code != http.StatusOK {
		t.Fatalf("register: got %d body=%v", code, body)
	}
	if ok, _ := body["ok"].(bool); !ok {
		t.Fatalf("register: ok=false: %v", body)
	}

	// Session set → /api/auth/me returns the user
	code2, me := httpJSON(t, ts, "GET", "/api/auth/me", nil)
	if code2 != http.StatusOK {
		t.Fatalf("/me after register: %d %v", code2, me)
	}
	user, _ := me["user"].(map[string]any)
	if user == nil {
		t.Fatalf("/me missing user: %v", me)
	}
	if gotEmail, _ := user["email"].(string); !strings.EqualFold(gotEmail, email) {
		t.Fatalf("user.email: got %q want %q", gotEmail, email)
	}
}

// TestAuth_Register_DuplicateEmail: registering again → 409.
func TestAuth_Register_DuplicateEmail(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)

	email := uniqueEmail("dup")
	code, _ := httpJSON(t, ts, "POST", "/api/auth/register", map[string]any{
		"email": email, "password": "testpass123",
	})
	if code != http.StatusOK {
		t.Fatalf("first register: %d", code)
	}

	// Use another client (without cookies from the first registration).
	ts2 := NewTestServer(t, env.Pool)
	code2, body := httpJSON(t, ts2, "POST", "/api/auth/register", map[string]any{
		"email": email, "password": "anotherpass99",
	})
	if code2 != http.StatusConflict {
		t.Fatalf("expected 409 conflict on duplicate, got %d body=%v", code2, body)
	}
}

// TestAuth_Register_AfterDelete_AllowsSameEmail: deleting the account nulls PII
// and soft-deletes the user; the same email must be able to register again.
func TestAuth_Register_AfterDelete_AllowsSameEmail(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)

	email := uniqueEmail("reregister")
	code, body := httpJSON(t, ts, "POST", "/api/auth/register", map[string]any{
		"email": email, "password": "testpass123",
	})
	if code != http.StatusOK {
		t.Fatalf("first register: got %d body=%v", code, body)
	}

	deleteCode, deleteBody := httpJSON(t, ts, "DELETE", "/api/profile", nil)
	if deleteCode != http.StatusOK {
		t.Fatalf("delete account: got %d body=%v", deleteCode, deleteBody)
	}

	ts2 := NewTestServer(t, env.Pool)
	code2, body2 := httpJSON(t, ts2, "POST", "/api/auth/register", map[string]any{
		"email": email, "password": "testpass456",
	})
	if code2 != http.StatusOK {
		t.Fatalf("second register after delete: got %d body=%v", code2, body2)
	}

	var activeCount, deletedCount int64
	if err := env.Pool.QueryRow(t.Context(),
		`select count(*) from users where lower(email)=lower($1) and deleted_at is null`, email).Scan(&activeCount); err != nil {
		t.Fatalf("count active users: %v", err)
	}
	if err := env.Pool.QueryRow(t.Context(),
		`select count(*) from users where deleted_at is not null`).Scan(&deletedCount); err != nil {
		t.Fatalf("count deleted users: %v", err)
	}
	if activeCount != 1 {
		t.Fatalf("active users with email=%d, want 1", activeCount)
	}
	if deletedCount < 1 {
		t.Fatalf("deleted users=%d, want at least 1", deletedCount)
	}
}

func TestAuth_DeleteAccountClearsMindstrataSessionCookie(t *testing.T) {
	t.Parallel()

	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)

	code, body := httpJSON(t, ts, "POST", "/api/auth/register", map[string]any{
		"email": uniqueEmail("delete_cookie"), "password": "testpass123",
	})
	if code != http.StatusOK {
		t.Fatalf("register: got %d body=%v", code, body)
	}

	req, err := http.NewRequest(http.MethodDelete, ts.URL("/api/profile"), nil)
	if err != nil {
		t.Fatalf("new delete request: %v", err)
	}
	resp, err := ts.Client.Do(req)
	if err != nil {
		t.Fatalf("delete profile: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("delete profile status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	for _, cookie := range resp.Cookies() {
		if cookie.Name == sessionCookieName && cookie.MaxAge < 0 {
			return
		}
	}
	t.Fatalf("delete profile did not expire %s cookie: %#v", sessionCookieName, resp.Cookies())
}

// TestAuth_Register_CaseInsensitiveDuplicate: different email case → still 409.
func TestAuth_Register_CaseInsensitiveDuplicate(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)

	email := uniqueEmail("CaseTest")
	code, _ := httpJSON(t, ts, "POST", "/api/auth/register", map[string]any{
		"email": strings.ToLower(email), "password": "testpass123",
	})
	if code != http.StatusOK {
		t.Fatalf("lowercase register: %d", code)
	}

	ts2 := NewTestServer(t, env.Pool)
	code2, _ := httpJSON(t, ts2, "POST", "/api/auth/register", map[string]any{
		"email": strings.ToUpper(email), "password": "testpass123",
	})
	if code2 != http.StatusConflict {
		t.Fatalf("uppercase duplicate: expected 409, got %d", code2)
	}
}

// TestAuth_Register_ShortPassword: < 10 chars → 400.
func TestAuth_Register_ShortPassword(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)

	code, body := httpJSON(t, ts, "POST", "/api/auth/register", map[string]any{
		"email": uniqueEmail("short"), "password": "short",
	})
	if code != http.StatusBadRequest {
		t.Fatalf("expected 400 for short password, got %d body=%v", code, body)
	}
}

// TestAuth_Register_InvalidEmail: no @ → 400.
func TestAuth_Register_InvalidEmail(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)

	code, _ := httpJSON(t, ts, "POST", "/api/auth/register", map[string]any{
		"email": "notanemail", "password": "testpass123",
	})
	if code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid email, got %d", code)
	}
}

// TestAuth_Login_HappyPath: existing user, correct credentials → 200 + session.
func TestAuth_Login_HappyPath(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	user := f.CreateUser(TestUserOpts{})

	code, body := httpJSON(t, ts, "POST", "/api/auth/login", map[string]any{
		"email": user.Email, "password": user.Password,
	})
	if code != http.StatusOK {
		t.Fatalf("login: %d body=%v", code, body)
	}

	// Session is valid
	code2, _ := httpJSON(t, ts, "GET", "/api/auth/me", nil)
	if code2 != http.StatusOK {
		t.Fatalf("/me after login: %d", code2)
	}
}

// TestAuth_Login_WrongPassword: correct email, wrong password → 401.
func TestAuth_Login_WrongPassword(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	user := f.CreateUser(TestUserOpts{})

	code, _ := httpJSON(t, ts, "POST", "/api/auth/login", map[string]any{
		"email": user.Email, "password": "wrongpass99",
	})
	if code != http.StatusUnauthorized {
		t.Fatalf("expected 401 on wrong password, got %d", code)
	}
}

// TestAuth_Login_NonexistentEmail: no such user → 401 (no disclosure).
func TestAuth_Login_NonexistentEmail(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)

	code, _ := httpJSON(t, ts, "POST", "/api/auth/login", map[string]any{
		"email": "ghost@test.local", "password": "anything123",
	})
	if code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", code)
	}
}

// TestAuth_Login_BlockedUser: status=blocked → 401.
func TestAuth_Login_BlockedUser(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	user := f.CreateUser(TestUserOpts{Status: "blocked"})

	code, _ := httpJSON(t, ts, "POST", "/api/auth/login", map[string]any{
		"email": user.Email, "password": user.Password,
	})
	if code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for blocked user, got %d", code)
	}
}

// TestAuth_Me_WithoutCookie: GET /me without a session → 401.
func TestAuth_Me_WithoutCookie(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)

	code, _ := httpJSON(t, ts, "GET", "/api/auth/me", nil)
	if code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without cookie, got %d", code)
	}
}

// TestAuth_Me_WithFactorySession: cookie from factory → 200.
func TestAuth_Me_WithFactorySession(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	user := f.CreateUser(TestUserOpts{})
	token := f.CreateSession(user.ID)
	ts.LoginAs(token)

	code, body := httpJSON(t, ts, "GET", "/api/auth/me", nil)
	if code != http.StatusOK {
		t.Fatalf("/me with session: %d body=%v", code, body)
	}
}

// TestAuth_Logout_InvalidatesSession: after logout the same cookie → 401.
func TestAuth_Logout_InvalidatesSession(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	user := f.CreateUser(TestUserOpts{})
	token := f.CreateSession(user.ID)
	ts.LoginAs(token)

	// Before logout /me works
	code, _ := httpJSON(t, ts, "GET", "/api/auth/me", nil)
	if code != http.StatusOK {
		t.Fatalf("pre-logout /me: %d", code)
	}

	// Logout
	code2, _ := httpJSON(t, ts, "POST", "/api/auth/logout", nil)
	if code2 != http.StatusOK {
		t.Fatalf("logout: %d", code2)
	}

	// Check directly in the DB that revoked_at is set.
	var revoked bool
	err := env.Pool.QueryRow(context.Background(), `
		select revoked_at is not null from auth_sessions where token_hash = $1`,
		tokenHash(token)).Scan(&revoked)
	if err != nil {
		t.Fatalf("query revoked: %v", err)
	}
	if !revoked {
		t.Fatalf("session not revoked after logout")
	}

	// Manually re-send the same raw token (the client may have cleared the cookie)
	ts.LoginAs(token)
	code3, _ := httpJSON(t, ts, "GET", "/api/auth/me", nil)
	if code3 != http.StatusUnauthorized {
		t.Fatalf("post-logout /me with same token: expected 401, got %d", code3)
	}
}

// TestOracle_Logout_InvalidatesSessionAcrossProtectedRoutes:
// a revoked token must not pass any auth-required layer.
func TestOracle_Logout_InvalidatesSessionAcrossProtectedRoutes(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	user := f.CreateUser(TestUserOpts{Role: "admin"})
	mode := f.CreateMode(TestModeOpts{})
	f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: mode.ID, DailyMessageLimit: 10})
	dialog := f.CreateDialog(user.ID, mode.ID)
	_, err := env.Pool.Exec(context.Background(),
		`update users set current_mode = $2, current_dialog = $3, accepted_tos = true where id = $1`,
		user.ID, mode.ID, dialog.ID)
	if err != nil {
		t.Fatalf("prepare current dialog: %v", err)
	}

	token := f.CreateSession(user.ID)
	ts.LoginAs(token)

	code, _ := httpJSON(t, ts, "GET", "/api/auth/me", nil)
	if code != http.StatusOK {
		t.Fatalf("pre-logout /me: %d", code)
	}
	code, _ = httpJSON(t, ts, "POST", "/api/auth/logout", nil)
	if code != http.StatusOK {
		t.Fatalf("logout: %d", code)
	}

	protectedRoutes := []struct {
		name   string
		method string
		path   string
		body   any
	}{
		{name: "auth_me", method: "GET", path: "/api/auth/me"},
		{name: "profile", method: "GET", path: "/api/profile"},
		{name: "profile_settings", method: "PATCH", path: "/api/profile/settings", body: map[string]any{"allowMessageAnonymization": false}},
		{name: "access_status", method: "GET", path: "/api/access/status"},
		{name: "promocode_apply", method: "POST", path: "/api/access/promocode/apply", body: map[string]any{"code": "ANY"}},
		{name: "chat_start", method: "POST", path: "/api/chat/start", body: map[string]any{"modeId": mode.ID}},
		{name: "chat_history", method: "GET", path: "/api/chat/history?dialogId=" + fmt.Sprint(dialog.ID)},
		{name: "chat_send", method: "POST", path: "/api/chat/send", body: map[string]any{"dialogId": dialog.ID, "text": "after logout"}},
		{name: "admin_status", method: "GET", path: "/api/admin/status"},
	}

	for _, route := range protectedRoutes {
		route := route
		t.Run(route.name, func(t *testing.T) {
			ts.LoginAs(token)
			status, body := httpJSON(t, ts, route.method, route.path, route.body)
			if status != http.StatusUnauthorized {
				t.Fatalf("%s %s with revoked session: got %d body=%v, want 401",
					route.method, route.path, status, body)
			}
		})
	}
}

func TestOracle_InvalidOrExpiredSessionCookieNeverFallsBackToGuest(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	guest := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{})
	f.GrantAccess(GrantAccessOpts{UserID: guest.ID, ModeID: mode.ID, DailyMessageLimit: 10})
	expiredUser := f.CreateUser(TestUserOpts{})
	expiredToken := f.CreateSession(expiredUser.ID)
	if _, err := env.Pool.Exec(context.Background(),
		`update auth_sessions set expires_at = now() - interval '1 minute' where token_hash = $1`,
		tokenHash(expiredToken)); err != nil {
		t.Fatalf("expire session: %v", err)
	}

	rootReq, err := http.NewRequest("GET", ts.URL("/"), nil)
	if err != nil {
		t.Fatalf("build root request: %v", err)
	}
	cases := []struct {
		name  string
		token string
	}{
		{name: "invalid", token: "token-without-db-row"},
		{name: "expired", token: expiredToken},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			ts.Client.Jar.SetCookies(rootReq.URL, []*http.Cookie{
				{Name: guestCookieName, Value: ts.Handler.guestCookieValue(guest.ID), Path: "/"},
				{Name: sessionCookieName, Value: tc.token, Path: "/"},
			})
			req, err := http.NewRequest("GET", ts.URL("/api/access/status"), nil)
			if err != nil {
				t.Fatalf("build access request: %v", err)
			}
			resp, err := ts.Client.Do(req)
			if err != nil {
				t.Fatalf("access status: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusUnauthorized {
				t.Fatalf("access status with %s auth cookie: got %d want 401", tc.name, resp.StatusCode)
			}
			clearedSession := false
			for _, cookie := range resp.Cookies() {
				if cookie.Name == sessionCookieName && cookie.MaxAge < 0 {
					clearedSession = true
				}
			}
			if !clearedSession {
				t.Fatalf("access status with %s auth cookie did not clear session cookie", tc.name)
			}
		})
	}
}

// TestAuth_Session_DeletedUser: user soft-deleted after the session was created → 401.
func TestAuth_Session_DeletedUser(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	user := f.CreateUser(TestUserOpts{})
	token := f.CreateSession(user.ID)
	ts.LoginAs(token)

	// Before deletion: ok
	code, _ := httpJSON(t, ts, "GET", "/api/auth/me", nil)
	if code != http.StatusOK {
		t.Fatalf("pre-delete /me: %d", code)
	}

	// Soft-delete
	_, err := env.Pool.Exec(context.Background(),
		"update users set deleted_at = now() where id = $1", user.ID)
	if err != nil {
		t.Fatalf("soft-delete user: %v", err)
	}

	code2, _ := httpJSON(t, ts, "GET", "/api/auth/me", nil)
	if code2 != http.StatusUnauthorized {
		t.Fatalf("post-delete /me: expected 401, got %d", code2)
	}
}
