//go:build integration

package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"mindstrata-stage1/api/internal/testsupport"
)

// fakeYandex returns httptest.Server emulating both Yandex OAuth endpoints:
//   - POST /token       → {access_token: "abc"}
//   - GET  /info?...    → user profile JSON
//
// Routed via rewriteRT — same pattern as fakeYooKassa.
type yandexRT struct{ target string }

func (r yandexRT) RoundTrip(req *http.Request) (*http.Response, error) {
	host := req.URL.Host
	if strings.Contains(host, "oauth.yandex.ru") || strings.Contains(host, "login.yandex.ru") {
		u := *req.URL
		u.Scheme = "http"
		u.Host = strings.TrimPrefix(r.target, "http://")
		req.URL = &u
		req.Host = u.Host
	}
	return http.DefaultTransport.RoundTrip(req)
}

func fakeYandex(t *testing.T, profile map[string]any, tokenStatus, profileStatus int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasPrefix(r.URL.Path, "/token") {
			if r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
				t.Errorf("token endpoint: wrong content-type %s", r.Header.Get("Content-Type"))
			}
			w.WriteHeader(tokenStatus)
			if tokenStatus >= 200 && tokenStatus < 300 {
				_, _ = w.Write([]byte(`{"access_token":"fake-yandex-token"}`))
			} else {
				_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
			}
			return
		}
		if strings.HasPrefix(r.URL.Path, "/info") {
			if !strings.HasPrefix(r.Header.Get("Authorization"), "OAuth ") {
				t.Errorf("info endpoint: missing OAuth header")
			}
			w.WriteHeader(profileStatus)
			if profileStatus >= 200 && profileStatus < 300 {
				_ = json.NewEncoder(w).Encode(profile)
			} else {
				_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
			}
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestOAuthE2E_ExchangeYandexProfile_SuccessfulFlow.
func TestOAuthE2E_ExchangeYandexProfile_SuccessfulFlow(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	_ = env
	fake := fakeYandex(t, map[string]any{
		"id":            "yandex-uid-12345",
		"login":         "test_user",
		"display_name":  "Test User",
		"default_email": "test@example.com",
	}, 200, 200)

	h := Handler{
		HTTPClient: &http.Client{
			Transport: yandexRT{target: fake.URL},
			Timeout:   5 * time.Second,
		},
		YandexClientID:     "test-client",
		YandexClientSecret: "test-secret",
		APIPublicBaseURL:   "http://localhost:18080",
	}

	cfg, err := h.oauthConfig("yandex")
	if err != nil {
		t.Fatalf("oauthConfig: %v", err)
	}
	profile, err := h.exchangeYandexProfile(context.Background(), cfg, "fake-code")
	if err != nil {
		t.Fatalf("exchange: %v", err)
	}
	if profile.ProviderUserID != "yandex-uid-12345" {
		t.Errorf("provider_user_id: got %q", profile.ProviderUserID)
	}
	if profile.Email != "test@example.com" {
		t.Errorf("email: got %q", profile.Email)
	}
	if profile.DisplayName != "Test User" {
		t.Errorf("display_name: got %q", profile.DisplayName)
	}
	if profile.Provider != "yandex" {
		t.Errorf("provider: got %q", profile.Provider)
	}
}

// TestOAuthE2E_ExchangeYandexProfile_TokenError: Yandex /token returns 4xx → error.
func TestOAuthE2E_ExchangeYandexProfile_TokenError(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	_ = env
	fake := fakeYandex(t, nil, 401, 200)

	h := Handler{
		HTTPClient: &http.Client{
			Transport: yandexRT{target: fake.URL},
			Timeout:   5 * time.Second,
		},
		YandexClientID:     "test-client",
		YandexClientSecret: "test-secret",
	}

	cfg, _ := h.oauthConfig("yandex")
	_, err := h.exchangeYandexProfile(context.Background(), cfg, "bad-code")
	if err == nil {
		t.Fatalf("expected error on token 401, got nil")
	}
}

// TestOAuthE2E_ExchangeYandexProfile_ProfileMissingID: profile without id → error.
func TestOAuthE2E_ExchangeYandexProfile_ProfileMissingID(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	_ = env
	fake := fakeYandex(t, map[string]any{
		"login": "noid",
		// no id field
	}, 200, 200)

	h := Handler{
		HTTPClient: &http.Client{
			Transport: yandexRT{target: fake.URL},
			Timeout:   5 * time.Second,
		},
		YandexClientID:     "test-client",
		YandexClientSecret: "test-secret",
	}
	cfg, _ := h.oauthConfig("yandex")
	_, err := h.exchangeYandexProfile(context.Background(), cfg, "code")
	if err == nil {
		t.Fatalf("expected error on profile without id")
	}
}

// TestOAuthE2E_FindOrCreateOAuthUser_NewUser:
// the first time a user + identity is created.
func TestOAuthE2E_FindOrCreateOAuthUser_NewUser(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	h := Handler{DB: env.Pool}

	profile := OAuthProfile{
		Provider:       "yandex",
		ProviderUserID: "uid-new-" + itoa(int64(env.Pool.Stat().AcquireCount())),
		Email:          "new_" + uniqueEmail("ya"),
		EmailVerified:  true,
		DisplayName:    "New OAuth User",
	}

	user, err := h.findOrCreateOAuthUser(context.Background(), profile)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if user.ID == 0 {
		t.Fatalf("zero user ID")
	}
	if user.Role != "user" || user.Status != "active" {
		t.Errorf("defaults: role=%q status=%q", user.Role, user.Status)
	}

	// DB: identity row exists
	var idCount int64
	_ = env.Pool.QueryRow(context.Background(),
		`select count(*) from user_identities where provider = 'yandex' and provider_user_id = $1`,
		profile.ProviderUserID).Scan(&idCount)
	if idCount != 1 {
		t.Errorf("identity row count: got %d want 1", idCount)
	}
}

// TestOAuthE2E_FindOrCreateOAuthUser_ExistingIdentity: the same provider_user_id → returns the same user.
func TestOAuthE2E_FindOrCreateOAuthUser_ExistingIdentity(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	h := Handler{DB: env.Pool}

	providerUID := "uid-existing-" + itoa(int64(env.Pool.Stat().AcquireCount()))
	profile := OAuthProfile{
		Provider: "yandex", ProviderUserID: providerUID,
		Email: uniqueEmail("first"), DisplayName: "First",
	}
	first, err := h.findOrCreateOAuthUser(context.Background(), profile)
	if err != nil {
		t.Fatalf("first create: %v", err)
	}

	// Second call with same provider_user_id but updated profile
	profile2 := OAuthProfile{
		Provider: "yandex", ProviderUserID: providerUID,
		Email: uniqueEmail("first"), DisplayName: "Updated Name",
	}
	second, err := h.findOrCreateOAuthUser(context.Background(), profile2)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if second.ID != first.ID {
		t.Errorf("re-find by identity: got user %d want %d", second.ID, first.ID)
	}
}

// TestOAuthE2E_FindOrCreateOAuthUser_AfterAccountDeleteAllowsSameProviderIdentity:
// a deleted user is not restored, but the same OAuth account can register a new user.
func TestOAuthE2E_FindOrCreateOAuthUser_AfterAccountDeleteAllowsSameProviderIdentity(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)
	h := Handler{DB: env.Pool}

	providerUID := "uid-reregister-" + itoa(time.Now().UnixNano())
	profile := OAuthProfile{
		Provider:       "yandex",
		ProviderUserID: providerUID,
		Email:          "oauth_" + uniqueEmail("reregister"),
		EmailVerified:  true,
		DisplayName:    "Deleted OAuth User",
	}
	first, err := h.findOrCreateOAuthUser(context.Background(), profile)
	if err != nil {
		t.Fatalf("first create: %v", err)
	}

	ts.LoginAs(f.CreateSession(first.ID))
	status, body := httpJSON(t, ts, "DELETE", "/api/profile", nil)
	if status != http.StatusOK {
		t.Fatalf("delete profile: status=%d body=%v", status, body)
	}

	profile.DisplayName = "Recreated OAuth User"
	second, err := h.findOrCreateOAuthUser(context.Background(), profile)
	if err != nil {
		t.Fatalf("recreate after delete: %v", err)
	}
	if second.ID == first.ID {
		t.Fatalf("deleted account was reused: user_id=%d", second.ID)
	}

	var oldIdentityCount, newIdentityCount, activeEmailCount int64
	if err := env.Pool.QueryRow(context.Background(),
		`select count(*) from user_identities where user_id = $1`,
		first.ID).Scan(&oldIdentityCount); err != nil {
		t.Fatalf("count old identities: %v", err)
	}
	if err := env.Pool.QueryRow(context.Background(),
		`select count(*) from user_identities where user_id = $1 and provider = $2 and provider_user_id = $3`,
		second.ID, profile.Provider, profile.ProviderUserID).Scan(&newIdentityCount); err != nil {
		t.Fatalf("count new identity: %v", err)
	}
	if err := env.Pool.QueryRow(context.Background(),
		`select count(*) from users where lower(email) = lower($1) and deleted_at is null`,
		profile.Email).Scan(&activeEmailCount); err != nil {
		t.Fatalf("count active email: %v", err)
	}
	if oldIdentityCount != 0 {
		t.Fatalf("deleted user still has oauth identities: got %d want 0", oldIdentityCount)
	}
	if newIdentityCount != 1 {
		t.Fatalf("new oauth identity count: got %d want 1", newIdentityCount)
	}
	if activeEmailCount != 1 {
		t.Fatalf("active users with email: got %d want 1", activeEmailCount)
	}
}

func TestOAuthE2E_LoginAfterAccountDeleteDoesNotRecreateSameProviderIdentity(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)
	h := Handler{DB: env.Pool}

	profile := OAuthProfile{
		Provider:       "yandex",
		ProviderUserID: "uid-login-after-delete-" + itoa(time.Now().UnixNano()),
		Email:          "oauth_" + uniqueEmail("login_after_delete"),
		EmailVerified:  true,
		DisplayName:    "Deleted OAuth User",
	}
	first, err := h.findOrCreateOAuthUser(context.Background(), profile)
	if err != nil {
		t.Fatalf("first create: %v", err)
	}
	ts.LoginAs(f.CreateSession(first.ID))
	status, body := httpJSON(t, ts, "DELETE", "/api/profile", nil)
	if status != http.StatusOK {
		t.Fatalf("delete profile: status=%d body=%v", status, body)
	}

	if _, err := h.findOAuthUserForLogin(context.Background(), profile); err == nil {
		t.Fatalf("login after delete recreated or accepted deleted OAuth account")
	}

	second, err := h.findOrCreateOAuthUser(context.Background(), profile)
	if err != nil {
		t.Fatalf("register after delete should create a new account: %v", err)
	}
	if second.ID == first.ID {
		t.Fatalf("register after delete reused deleted account: user_id=%d", second.ID)
	}

	var activeEmailCount int64
	if err := env.Pool.QueryRow(context.Background(),
		`select count(*) from users where lower(email) = lower($1) and deleted_at is null`,
		profile.Email).Scan(&activeEmailCount); err != nil {
		t.Fatalf("count active email: %v", err)
	}
	if activeEmailCount != 1 {
		t.Fatalf("active users with email: got %d want 1", activeEmailCount)
	}
}

func TestOAuthE2E_FindOrCreateOAuthUser_AfterAccountDeleteAllowsSameVerifiedEmail(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)
	h := Handler{DB: env.Pool}

	email := "oauth_" + uniqueEmail("same_email")
	firstProfile := OAuthProfile{
		Provider:       "yandex",
		ProviderUserID: "uid-old-email-" + itoa(time.Now().UnixNano()),
		Email:          email,
		EmailVerified:  true,
		DisplayName:    "Deleted OAuth User",
	}
	first, err := h.findOrCreateOAuthUser(context.Background(), firstProfile)
	if err != nil {
		t.Fatalf("first create: %v", err)
	}
	ts.LoginAs(f.CreateSession(first.ID))
	status, body := httpJSON(t, ts, "DELETE", "/api/profile", nil)
	if status != http.StatusOK {
		t.Fatalf("delete profile: status=%d body=%v", status, body)
	}

	secondProfile := OAuthProfile{
		Provider:       "yandex",
		ProviderUserID: "uid-new-email-" + itoa(time.Now().UnixNano()),
		Email:          email,
		EmailVerified:  true,
		DisplayName:    "New OAuth User",
	}
	second, err := h.findOrCreateOAuthUser(context.Background(), secondProfile)
	if err != nil {
		t.Fatalf("second create same verified email after delete: %v", err)
	}
	if second.ID == first.ID {
		t.Fatalf("deleted account was reused by email link: user_id=%d", second.ID)
	}

	var activeEmailCount int64
	if err := env.Pool.QueryRow(context.Background(),
		`select count(*) from users where lower(email) = lower($1) and deleted_at is null`,
		email).Scan(&activeEmailCount); err != nil {
		t.Fatalf("count active email: %v", err)
	}
	if activeEmailCount != 1 {
		t.Fatalf("active users with email: got %d want 1", activeEmailCount)
	}
}

func TestOAuthE2E_Callback_TransfersGuestAccessAndHistoryForLoginAndRegister(t *testing.T) {
	t.Parallel()

	for _, intent := range []string{"login", oauthRegisterIntent} {
		intent := intent
		t.Run(intent, func(t *testing.T) {
			env := testsupport.NewEnv(t)
			f := NewFactory(t, env.Pool)
			mode := f.CreateMode(TestModeOpts{})
			guest := f.CreateUser(TestUserOpts{Role: "user"})
			src := int64(7000 + idCounter.Add(1))
			f.GrantAccess(GrantAccessOpts{
				UserID: guest.ID, ModeID: mode.ID, DailyMessageLimit: 15,
				AccessType: "promocode", SourceID: &src,
			})
			dialog := f.CreateDialog(guest.ID, mode.ID)
			messageID := f.AppendMessage(dialog.ID, "user", "guest history before oauth")
			if _, err := env.Pool.Exec(context.Background(),
				`update users set current_mode = $2, current_dialog = $3 where id = $1`,
				guest.ID, mode.ID, dialog.ID); err != nil {
				t.Fatalf("set guest current dialog: %v", err)
			}

			providerUID := "uid-callback-transfer-" + intent + "-" + itoa(time.Now().UnixNano())
			email := "oauth_" + uniqueEmail("callback_transfer_"+intent)
			profile := OAuthProfile{
				Provider:       "yandex",
				ProviderUserID: providerUID,
				Email:          email,
				EmailVerified:  true,
				DisplayName:    "OAuth Transfer User",
			}
			precreatedID := int64(0)
			precreateHandler := Handler{DB: env.Pool}
			if intent == "login" {
				precreated, err := precreateHandler.findOrCreateOAuthUser(context.Background(), profile)
				if err != nil {
					t.Fatalf("precreate oauth user: %v", err)
				}
				precreatedID = precreated.ID
			}

			fake := fakeYandex(t, map[string]any{
				"id":            providerUID,
				"display_name":  profile.DisplayName,
				"default_email": email,
			}, 200, 200)
			ts := NewTestServerWithHandler(t, env.Pool, Handler{
				HTTPClient: &http.Client{
					Transport: yandexRT{target: fake.URL},
					Timeout:   5 * time.Second,
				},
				YandexClientID:     "test-client",
				YandexClientSecret: "test-secret",
			})

			state := "callback-transfer-" + intent + "-" + itoa(time.Now().UnixNano())
			redirectAfter := packOAuthRedirectAfter("/chat", intent)
			if _, err := env.Pool.Exec(context.Background(), `
				insert into oauth_states (token_hash, provider, redirect_after, ip_hash, user_agent, expires_at)
				values ($1, 'yandex', $2, 'ip', 'ua', now() + interval '5 minutes')`,
				tokenHash(state), redirectAfter); err != nil {
				t.Fatalf("insert oauth state: %v", err)
			}

			req, err := http.NewRequest("GET",
				ts.URL("/api/auth/oauth/yandex/callback?code=fake-code&state="+state), nil)
			if err != nil {
				t.Fatalf("build callback request: %v", err)
			}
			req.AddCookie(&http.Cookie{Name: oauthStateCookieName, Value: state})
			req.AddCookie(&http.Cookie{Name: guestCookieName, Value: ts.Handler.guestCookieValue(guest.ID)})
			resp, err := ts.Client.Do(req)
			if err != nil {
				t.Fatalf("callback: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusFound {
				t.Fatalf("callback status: got %d want 302", resp.StatusCode)
			}
			if loc := resp.Header.Get("Location"); !strings.HasSuffix(loc, "/chat") {
				t.Fatalf("callback redirect: got %q want /chat", loc)
			}

			var targetID int64
			if err := env.Pool.QueryRow(context.Background(), `
				select user_id
				from user_identities
				where provider = 'yandex' and provider_user_id = $1`, providerUID).Scan(&targetID); err != nil {
				t.Fatalf("query oauth identity: %v", err)
			}
			if intent == "login" && targetID != precreatedID {
				t.Fatalf("login target: got %d want existing %d", targetID, precreatedID)
			}
			if intent == oauthRegisterIntent && targetID == guest.ID {
				t.Fatalf("register reused guest user as oauth target")
			}

			var dialogOwner, messageCount, accessRows int64
			if err := env.Pool.QueryRow(context.Background(),
				`select user_id from users_dialogs where id = $1`, dialog.ID).Scan(&dialogOwner); err != nil {
				t.Fatalf("query dialog owner: %v", err)
			}
			if err := env.Pool.QueryRow(context.Background(),
				`select count(*) from dialogs_messages where id = $1 and dialog_id = $2`, messageID, dialog.ID).Scan(&messageCount); err != nil {
				t.Fatalf("query transferred message: %v", err)
			}
			if err := env.Pool.QueryRow(context.Background(), `
				select count(*)
				from user_mode_access
				where user_id = $1 and mode_id = $2 and access_type = 'promocode' and source_id = $3`,
				targetID, mode.ID, src).Scan(&accessRows); err != nil {
				t.Fatalf("count transferred access: %v", err)
			}
			if dialogOwner != targetID {
				t.Fatalf("dialog owner: got %d want oauth user %d", dialogOwner, targetID)
			}
			if messageCount != 1 {
				t.Fatalf("transferred message count: got %d want 1", messageCount)
			}
			if accessRows != 1 {
				t.Fatalf("transferred access rows: got %d want 1", accessRows)
			}
		})
	}
}

// TestOAuthE2E_FindOrCreateOAuthUser_LinkByEmail:
// no identity, but a user with that email + role=user + status=active → link.
// K-NEW4-1: linking requires EmailVerified=true (account hijack protection).
func TestOAuthE2E_FindOrCreateOAuthUser_LinkByEmail(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	h := Handler{DB: env.Pool}

	existing := f.CreateUser(TestUserOpts{Role: "user", Status: "active"})

	profile := OAuthProfile{
		Provider: "yandex", ProviderUserID: "uid-link-" + itoa(int64(env.Pool.Stat().AcquireCount())),
		Email:         existing.Email,
		EmailVerified: true, // K-NEW4-1: otherwise linking-by-email refuses
		DisplayName:   "Linked",
	}
	got, err := h.findOrCreateOAuthUser(context.Background(), profile)
	if err != nil {
		t.Fatalf("link: %v", err)
	}
	if got.ID != existing.ID {
		t.Errorf("did not link: got user %d want %d", got.ID, existing.ID)
	}
	// DB: a new identity row for the existing user
	var n int64
	_ = env.Pool.QueryRow(context.Background(),
		`select count(*) from user_identities where user_id = $1 and provider_user_id = $2`,
		existing.ID, profile.ProviderUserID).Scan(&n)
	if n != 1 {
		t.Errorf("identity not created: %d rows", n)
	}
}

// TestOAuthE2E_FindOrCreateOAuthUser_RefusesPrivilegedLink:
// if the existing user is admin/owner/tester/etc → link by email is refused.
func TestOAuthE2E_FindOrCreateOAuthUser_RefusesPrivilegedLink(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	h := Handler{DB: env.Pool}

	admin := f.CreateUser(TestUserOpts{Role: "admin", Status: "active"})

	profile := OAuthProfile{
		Provider: "yandex", ProviderUserID: "uid-priv-" + itoa(int64(env.Pool.Stat().AcquireCount())),
		Email:         admin.Email,
		EmailVerified: true, // K-NEW4-1: linking-by-email requires verified
	}
	_, err := h.findOrCreateOAuthUser(context.Background(), profile)
	if err == nil {
		t.Fatalf("expected error linking to admin account, got nil")
	}
}

// TestOAuthE2E_ProvidersEndpoint_WithConfig: the providers endpoint sees yandex as configured.
func TestOAuthE2E_ProvidersEndpoint_WithConfig(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServerWithHandler(t, env.Pool, Handler{YandexClientID: "test-client", YandexClientSecret: "test-secret"})

	status, body := httpJSON(t, ts, "GET", "/api/auth/oauth/providers", nil)
	if status != http.StatusOK {
		t.Fatalf("status: %d", status)
	}
	providers, _ := body["providers"].(map[string]any)
	if yandex, _ := providers["yandex"].(bool); !yandex {
		t.Errorf("expected yandex=true")
	}
}
