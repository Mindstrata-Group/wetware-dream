//go:build integration

package httpapi

import (
	"context"
	"net/http"
	"testing"
	"time"

	"mindstrata-stage1/api/internal/testsupport"
)

// TestOAuth_ProvidersList_NoConfig: no env vars → yandex=false.
func TestOAuth_ProvidersList_NoConfig(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)

	status, body := httpJSON(t, ts, "GET", "/api/auth/oauth/providers", nil)
	if status != http.StatusOK {
		t.Fatalf("providers: %d body=%v", status, body)
	}
	providers, _ := body["providers"].(map[string]any)
	if yandex, _ := providers["yandex"].(bool); yandex {
		t.Fatalf("expected yandex=false when env missing, got true")
	}
}

// TestOAuth_ProvidersList_Configured: with env vars → yandex=true.
func TestOAuth_ProvidersList_Configured(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServerWithHandler(t, env.Pool, Handler{YandexClientID: "test-client-id", YandexClientSecret: "test-client-secret"})

	status, body := httpJSON(t, ts, "GET", "/api/auth/oauth/providers", nil)
	if status != http.StatusOK {
		t.Fatalf("providers: %d", status)
	}
	providers, _ := body["providers"].(map[string]any)
	if yandex, _ := providers["yandex"].(bool); !yandex {
		t.Fatalf("expected yandex=true when env set, got false body=%v", body)
	}
}

// TestOAuth_Start_NotConfigured: /start without YANDEX_CLIENT_ID → redirect to login?error=oauth_not_configured.
func TestOAuth_Start_NotConfigured(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)

	req, _ := http.NewRequest("GET", ts.URL("/api/auth/oauth/yandex/start"), nil)
	resp, err := ts.Client.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	resp.Body.Close()
	// Should be a redirect (302/303) — handler responds with redirect to /login?error=oauth_not_configured
	if resp.StatusCode != http.StatusFound && resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected redirect, got %d", resp.StatusCode)
	}
	loc := resp.Header.Get("Location")
	if loc == "" || !contains(loc, "oauth_not_configured") {
		t.Fatalf("Location should contain oauth_not_configured, got %q", loc)
	}
}

// TestOAuth_Start_CreatesStateRow: /start with config → an oauth_states row + state cookie.
func TestOAuth_Start_CreatesStateRow(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServerWithHandler(t, env.Pool, Handler{YandexClientID: "test-client-id", YandexClientSecret: "test-client-secret"})

	req, _ := http.NewRequest("GET", ts.URL("/api/auth/oauth/yandex/start?next=/profile"), nil)
	resp, err := ts.Client.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusFound && resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected redirect to Yandex, got %d (Location=%q)",
			resp.StatusCode, resp.Header.Get("Location"))
	}
	loc := resp.Header.Get("Location")
	if !contains(loc, "yandex.ru") && !contains(loc, "oauth.yandex") {
		t.Fatalf("Location should redirect to Yandex, got %q", loc)
	}

	// DB: exactly one oauth_states record
	var count int64
	_ = env.Pool.QueryRow(context.Background(),
		`select count(*) from oauth_states where provider = 'yandex' and used_at is null`).Scan(&count)
	if count != 1 {
		t.Fatalf("oauth_states count: got %d want 1", count)
	}

	// Cookie is set
	var hasStateCookie bool
	for _, c := range resp.Cookies() {
		if c.Name == oauthStateCookieName {
			hasStateCookie = true
			break
		}
	}
	if !hasStateCookie {
		t.Fatalf("oauth state cookie not set on /start response")
	}
}

// TestOAuth_ConsumeState_HappyPath: an inserted record + the correct state → returns redirect_after.
func TestOAuth_ConsumeState_HappyPath(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	h := Handler{DB: env.Pool}

	state := "test-state-token-1"
	_, err := env.Pool.Exec(context.Background(), `
		insert into oauth_states (token_hash, provider, redirect_after, ip_hash, user_agent, expires_at)
		values ($1, 'yandex', '/profile', 'ip', 'ua', now() + interval '5 minutes')`,
		tokenHash(state))
	if err != nil {
		t.Fatalf("insert state: %v", err)
	}

	redir, intent, err := h.consumeOAuthState(context.Background(), "yandex", state)
	if err != nil {
		t.Fatalf("consume: %v", err)
	}
	if redir != "/profile" {
		t.Fatalf("redir: got %q want /profile", redir)
	}
	if intent != "login" {
		t.Fatalf("intent: got %q want login", intent)
	}

	// DB: used_at is set
	var usedAt *time.Time
	_ = env.Pool.QueryRow(context.Background(),
		`select used_at from oauth_states where token_hash = $1`, tokenHash(state)).Scan(&usedAt)
	if usedAt == nil {
		t.Fatalf("used_at not marked")
	}
}

func TestOAuth_ConsumeState_RegisterIntent(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	h := Handler{DB: env.Pool}

	state := "test-state-token-register"
	_, err := env.Pool.Exec(context.Background(), `
		insert into oauth_states (token_hash, provider, redirect_after, ip_hash, user_agent, expires_at)
		values ($1, 'yandex', $2, 'ip', 'ua', now() + interval '5 minutes')`,
		tokenHash(state), packOAuthRedirectAfter("/profile", oauthRegisterIntent))
	if err != nil {
		t.Fatalf("insert state: %v", err)
	}

	redir, intent, err := h.consumeOAuthState(context.Background(), "yandex", state)
	if err != nil {
		t.Fatalf("consume: %v", err)
	}
	if redir != "/profile" {
		t.Fatalf("redir: got %q want /profile", redir)
	}
	if intent != oauthRegisterIntent {
		t.Fatalf("intent: got %q want %s", intent, oauthRegisterIntent)
	}
}

// TestOAuth_ConsumeState_Reused: reusing the same state → error.
func TestOAuth_ConsumeState_Reused(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	h := Handler{DB: env.Pool}

	state := "reuse-test-token"
	_, err := env.Pool.Exec(context.Background(), `
		insert into oauth_states (token_hash, provider, redirect_after, ip_hash, user_agent, expires_at)
		values ($1, 'yandex', '/profile', 'ip', 'ua', now() + interval '5 minutes')`,
		tokenHash(state))
	if err != nil {
		t.Fatalf("insert state: %v", err)
	}

	// First: ok
	if _, _, err := h.consumeOAuthState(context.Background(), "yandex", state); err != nil {
		t.Fatalf("first consume: %v", err)
	}

	// Second must fail (used_at already set → not found)
	if _, _, err := h.consumeOAuthState(context.Background(), "yandex", state); err == nil {
		t.Fatalf("expected error on reused state, got nil")
	}
}

// TestOAuth_ConsumeState_Expired: expires_at in the past → error.
func TestOAuth_ConsumeState_Expired(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	h := Handler{DB: env.Pool}

	state := "expired-state-token"
	_, err := env.Pool.Exec(context.Background(), `
		insert into oauth_states (token_hash, provider, redirect_after, ip_hash, user_agent, expires_at)
		values ($1, 'yandex', '/profile', 'ip', 'ua', now() - interval '1 hour')`,
		tokenHash(state))
	if err != nil {
		t.Fatalf("insert state: %v", err)
	}

	if _, _, err := h.consumeOAuthState(context.Background(), "yandex", state); err == nil {
		t.Fatalf("expected error on expired state, got nil")
	}
}

// TestOAuth_ConsumeState_ProviderMismatch: state for yandex, callback on google → error.
func TestOAuth_ConsumeState_ProviderMismatch(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	h := Handler{DB: env.Pool}

	state := "mismatch-state-token"
	_, err := env.Pool.Exec(context.Background(), `
		insert into oauth_states (token_hash, provider, redirect_after, ip_hash, user_agent, expires_at)
		values ($1, 'yandex', '/profile', 'ip', 'ua', now() + interval '5 minutes')`,
		tokenHash(state))
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	if _, _, err := h.consumeOAuthState(context.Background(), "google", state); err == nil {
		t.Fatalf("expected error on provider mismatch, got nil")
	}
}

// TestSafeRedirectAfter_Matrix: checks every case of safeRedirectAfter (unit).
func TestSafeRedirectAfter_Matrix(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in, want string
	}{
		{"", "/profile"},
		{"/profile", "/profile"},
		{"/chat", "/chat"},
		{"//evil.com", "/profile"},       // protocol-relative URL
		{"https://evil.com", "/profile"}, // not starting with /
		{`\\evil`, "/profile"},           // backslash trick
		{"/admin/users", "/profile"},     // privileged paths blocked
		{"/tester/users", "/profile"},
		{"/expert/dashboard", "/profile"},
		{"   /chat   ", "/chat"},
	}
	for _, c := range cases {
		if got := safeRedirectAfter(c.in); got != c.want {
			t.Errorf("safeRedirectAfter(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestOAuth_Callback_StateCookieMismatch: cookie state != query state → redirect /login?error=oauth_state.
func TestOAuth_Callback_StateCookieMismatch(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServerWithHandler(t, env.Pool, Handler{YandexClientID: "test-client-id", YandexClientSecret: "test-client-secret"})

	// Create a state in the DB
	cookieState := "cookie-state-xyz"
	queryState := "different-state-abc"
	_, err := env.Pool.Exec(context.Background(), `
		insert into oauth_states (token_hash, provider, redirect_after, ip_hash, user_agent, expires_at)
		values ($1, 'yandex', '/profile', 'ip', 'ua', now() + interval '5 minutes')`,
		tokenHash(queryState))
	if err != nil {
		t.Fatalf("insert state: %v", err)
	}

	// Callback request with different state in the cookie and the query
	req, _ := http.NewRequest("GET",
		ts.URL("/api/auth/oauth/yandex/callback?code=somecode&state="+queryState), nil)
	req.AddCookie(&http.Cookie{Name: oauthStateCookieName, Value: cookieState})
	resp, err := ts.Client.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	resp.Body.Close()

	// Should redirect to login with error
	if resp.StatusCode != http.StatusFound && resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected redirect, got %d", resp.StatusCode)
	}
	loc := resp.Header.Get("Location")
	if !contains(loc, "oauth_state") {
		t.Fatalf("Location should contain oauth_state error, got %q", loc)
	}
}

// contains: helper avoid importing strings just for one call.
func contains(haystack, needle string) bool {
	if len(needle) > len(haystack) {
		return false
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
