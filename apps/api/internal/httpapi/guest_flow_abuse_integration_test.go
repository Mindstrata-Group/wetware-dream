//go:build integration

package httpapi

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"

	"mindstrata-stage1/api/internal/testsupport"
)

// These tests check abuse scenarios of the guest flow (anonymous session →
// promo code → access transferred to an account via login/register/OAuth) at the
// junction with the M-NEW fix in transferGuestAccess (move, not clone); see
// oauth_guest_access.go.

// guestUserIDFromStatus does GET /api/access/status, creating (or finding) the
// guest user, and returns its userId. The guest cookie is kept in the test
// client's cookiejar.
func guestUserIDFromStatus(t *testing.T, ts *TestServer) int64 {
	t.Helper()
	code, body := httpJSON(t, ts, "GET", "/api/access/status", nil)
	if code != http.StatusOK {
		t.Fatalf("access/status: %d body=%v", code, body)
	}
	id, _ := body["userId"].(float64)
	if id <= 0 {
		t.Fatalf("access/status: invalid userId in %v", body)
	}
	return int64(id)
}

func countPromoAccess(t *testing.T, env *testsupport.Env, userID, modeID, promoID int64) int64 {
	t.Helper()
	var n int64
	if err := env.Pool.QueryRow(context.Background(), `
		select count(*) from user_mode_access
		where user_id = $1 and mode_id = $2 and access_type = 'promocode' and source_id = $3
		  and (active_to is null or active_to >= now())`,
		userID, modeID, promoID).Scan(&n); err != nil {
		t.Fatalf("count promo access: %v", err)
	}
	return n
}

// TestGuestFlowAbuse_PromoReplayAcrossOAuthLogins_DoesNotDuplicateAccess:
// the guest applies a promo code (1 used_count), then logs in via OAuth into
// account1 (access is transferred). If the same guest cookie is replayed on an
// OAuth login into account2, access must NOT be duplicated: used_count stays 1
// (M-NEW: move, not clone).
func TestGuestFlowAbuse_PromoReplayAcrossOAuthLogins_DoesNotDuplicateAccess(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)

	mode := f.CreateMode(TestModeOpts{})
	promo := f.CreatePromocode(TestPromocodeOpts{TargetID: mode.ID, MaxUses: 5})

	guestTS := NewTestServer(t, env.Pool)
	guestID := guestUserIDFromStatus(t, guestTS)

	if code, body := httpJSON(t, guestTS, "POST", "/api/access/promocode/apply", map[string]any{"code": promo.Code}); code != http.StatusOK {
		t.Fatalf("apply promo as guest: %d body=%v", code, body)
	}

	pre := Handler{DB: env.Pool}

	// account1: the first OAuth login with the guest cookie: access is transferred.
	profile1 := OAuthProfile{
		Provider: "yandex", ProviderUserID: "uid-replay-acc1-" + itoa(time.Now().UnixNano()),
		Email: uniqueEmail("replay_acc1"), EmailVerified: true, DisplayName: "Replay Acc1",
	}
	acc1, err := pre.findOrCreateOAuthUser(context.Background(), profile1)
	if err != nil {
		t.Fatalf("precreate acc1: %v", err)
	}
	doOAuthLoginWithGuestCookie(t, env, profile1, guestID, "replay-1-")

	if got := countPromoAccess(t, env, acc1.ID, mode.ID, promo.ID); got != 1 {
		t.Fatalf("acc1 promo access rows: got %d want 1", got)
	}

	// account2: replay the same guest cookie on a SECOND OAuth login.
	profile2 := OAuthProfile{
		Provider: "yandex", ProviderUserID: "uid-replay-acc2-" + itoa(time.Now().UnixNano()),
		Email: uniqueEmail("replay_acc2"), EmailVerified: true, DisplayName: "Replay Acc2",
	}
	acc2, err := pre.findOrCreateOAuthUser(context.Background(), profile2)
	if err != nil {
		t.Fatalf("precreate acc2: %v", err)
	}
	doOAuthLoginWithGuestCookie(t, env, profile2, guestID, "replay-2-")

	if got := countPromoAccess(t, env, acc2.ID, mode.ID, promo.ID); got != 0 {
		t.Fatalf("guest cookie replay duplicated promo access to acc2: got %d want 0", got)
	}

	var usedCount int64
	if err := env.Pool.QueryRow(context.Background(), `select used_count from promocodes where id = $1`, promo.ID).Scan(&usedCount); err != nil {
		t.Fatalf("query used_count: %v", err)
	}
	if usedCount != 1 {
		t.Fatalf("used_count: got %d want 1 (one guest application = one redemption)", usedCount)
	}
}

// doOAuthLoginWithGuestCookie runs the OAuth callback (intent=login) for an
// existing account, attaching a guest cookie with the given guestUserID.
// Used to emulate a "replayed" guest cookie on a repeated login.
func doOAuthLoginWithGuestCookie(t *testing.T, env *testsupport.Env, profile OAuthProfile, guestUserID int64, statePrefix string) {
	t.Helper()

	fake := fakeYandex(t, map[string]any{
		"id":            profile.ProviderUserID,
		"display_name":  profile.DisplayName,
		"default_email": profile.Email,
	}, 200, 200)
	ts := NewTestServerWithHandler(t, env.Pool, Handler{
		HTTPClient: &http.Client{
			Transport: yandexRT{target: fake.URL},
			Timeout:   5 * time.Second,
		},
		YandexClientID:     "test-client",
		YandexClientSecret: "test-secret",
	})

	state := statePrefix + itoa(time.Now().UnixNano())
	redirectAfter := packOAuthRedirectAfter("/chat", "login")
	if _, err := env.Pool.Exec(context.Background(), `
		insert into oauth_states (token_hash, provider, redirect_after, ip_hash, user_agent, expires_at)
		values ($1, 'yandex', $2, 'ip', 'ua', now() + interval '5 minutes')`,
		tokenHash(state), redirectAfter); err != nil {
		t.Fatalf("insert oauth state: %v", err)
	}

	req, err := http.NewRequest("GET", ts.URL("/api/auth/oauth/yandex/callback?code=fake-code&state="+state), nil)
	if err != nil {
		t.Fatalf("build callback request: %v", err)
	}
	req.AddCookie(&http.Cookie{Name: oauthStateCookieName, Value: state})
	req.AddCookie(&http.Cookie{Name: guestCookieName, Value: ts.Handler.guestCookieValue(guestUserID)})
	resp, err := ts.Client.Do(req)
	if err != nil {
		t.Fatalf("callback: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("callback status: got %d want 302", resp.StatusCode)
	}
}

// TestGuestFlowAbuse_ReapplyPromoAfterTransfer_BoundedByMaxUses: after the access
// transfer the guest is "reset" (its promocode_usages are removed by the M-NEW
// fix) and may apply the same promo code again, but no more than max_uses times
// in total. Each application = exactly one account with access.
func TestGuestFlowAbuse_ReapplyPromoAfterTransfer_BoundedByMaxUses(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	h := Handler{DB: env.Pool}

	mode := f.CreateMode(TestModeOpts{})
	promo := f.CreatePromocode(TestPromocodeOpts{TargetID: mode.ID, MaxUses: 2})
	target1 := f.CreateUser(TestUserOpts{Role: "user"})
	target2 := f.CreateUser(TestUserOpts{Role: "user"})

	ts := NewTestServer(t, env.Pool)
	guestID := guestUserIDFromStatus(t, ts)

	// 1st application: success, used_count=1.
	if code, body := httpJSON(t, ts, "POST", "/api/access/promocode/apply", map[string]any{"code": promo.Code}); code != http.StatusOK {
		t.Fatalf("1st apply: %d body=%v", code, body)
	}
	if err := h.transferGuestAccess(context.Background(), guestID, target1.ID); err != nil {
		t.Fatalf("transfer to target1: %v", err)
	}

	// 2nd application with the same guest cookie (the guest was "reset" by the transfer):
	// must pass, used_count=2.
	if code, body := httpJSON(t, ts, "POST", "/api/access/promocode/apply", map[string]any{"code": promo.Code}); code != http.StatusOK {
		t.Fatalf("2nd apply after transfer should succeed (bounded by max_uses): %d body=%v", code, body)
	}
	if err := h.transferGuestAccess(context.Background(), guestID, target2.ID); err != nil {
		t.Fatalf("transfer to target2: %v", err)
	}

	// 3rd application: the limit is exhausted.
	code, body := httpJSON(t, ts, "POST", "/api/access/promocode/apply", map[string]any{"code": promo.Code})
	if code != http.StatusBadRequest {
		t.Fatalf("3rd apply: got %d want 400 body=%v", code, body)
	}
	if errCode, _ := body["code"].(string); errCode != "limit_reached" {
		t.Fatalf("3rd apply errorCode: got %q want limit_reached body=%v", errCode, body)
	}

	if got := countPromoAccess(t, env, target1.ID, mode.ID, promo.ID); got != 1 {
		t.Fatalf("target1 promo access rows: got %d want 1", got)
	}
	if got := countPromoAccess(t, env, target2.ID, mode.ID, promo.ID); got != 1 {
		t.Fatalf("target2 promo access rows: got %d want 1", got)
	}

	var usedCount int64
	if err := env.Pool.QueryRow(context.Background(), `select used_count from promocodes where id = $1`, promo.ID).Scan(&usedCount); err != nil {
		t.Fatalf("query used_count: %v", err)
	}
	if usedCount != 2 {
		t.Fatalf("used_count: got %d want 2", usedCount)
	}
}

// TestGuestFlowAbuse_ReplayedCookieAfterTransfer_NoLeftoverAccess: after
// transferGuestAccess a repeated request with the old guest cookie must not
// show any leftover paid access.
func TestGuestFlowAbuse_ReplayedCookieAfterTransfer_NoLeftoverAccess(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	h := Handler{DB: env.Pool}

	mode := f.CreateMode(TestModeOpts{})
	promo := f.CreatePromocode(TestPromocodeOpts{TargetID: mode.ID, MaxUses: 1})
	target := f.CreateUser(TestUserOpts{Role: "user"})

	ts := NewTestServer(t, env.Pool)
	guestID := guestUserIDFromStatus(t, ts)

	if code, body := httpJSON(t, ts, "POST", "/api/access/promocode/apply", map[string]any{"code": promo.Code}); code != http.StatusOK {
		t.Fatalf("apply promo: %d body=%v", code, body)
	}
	if err := h.transferGuestAccess(context.Background(), guestID, target.ID); err != nil {
		t.Fatalf("transfer: %v", err)
	}

	// The same guest cookie, repeated access/status.
	code, body := httpJSON(t, ts, "GET", "/api/access/status", nil)
	if code != http.StatusOK {
		t.Fatalf("access/status replay: %d body=%v", code, body)
	}
	if hasAccess, _ := body["hasAccess"].(bool); hasAccess {
		t.Fatalf("stale guest cookie still reports hasAccess=true: %v", body)
	}
	if modes, ok := body["activeModes"].([]any); ok && len(modes) != 0 {
		t.Fatalf("stale guest cookie still has active modes: %v", modes)
	}
}

// TestGuestFlowAbuse_ConcurrentOAuthTransferReplays_NoDuplicateGrant:
// several parallel transferGuestAccess calls for ONE guest to ONE target
// (imitating a race of parallel OAuth callbacks from one browser) must not
// duplicate access/usage and must not fail with an error.
func TestGuestFlowAbuse_ConcurrentOAuthTransferReplays_NoDuplicateGrant(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	h := Handler{DB: env.Pool}

	mode := f.CreateMode(TestModeOpts{})
	promo := f.CreatePromocode(TestPromocodeOpts{TargetID: mode.ID, MaxUses: 1})
	target := f.CreateUser(TestUserOpts{Role: "user"})

	ts := NewTestServer(t, env.Pool)
	guestID := guestUserIDFromStatus(t, ts)
	if code, body := httpJSON(t, ts, "POST", "/api/access/promocode/apply", map[string]any{"code": promo.Code}); code != http.StatusOK {
		t.Fatalf("apply promo: %d body=%v", code, body)
	}

	const n = 5
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := h.transferGuestAccess(context.Background(), guestID, target.ID); err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("concurrent transfer: %v", err)
	}

	if got := countPromoAccess(t, env, target.ID, mode.ID, promo.ID); got != 1 {
		t.Fatalf("target promo access rows: got %d want 1", got)
	}
	var targetUsages int64
	if err := env.Pool.QueryRow(context.Background(),
		`select count(*) from promocode_usages where user_id = $1 and promocode_id = $2`,
		target.ID, promo.ID).Scan(&targetUsages); err != nil {
		t.Fatalf("count target usages: %v", err)
	}
	if targetUsages != 1 {
		t.Fatalf("target promo usages: got %d want 1", targetUsages)
	}

	if got := countPromoAccess(t, env, guestID, mode.ID, promo.ID); got != 0 {
		t.Fatalf("guest still has active access after transfer: got %d want 0", got)
	}
}

// TestGuestFlowAbuse_RegisterThenOAuthLoginWithStaleGuestCookie_NoCrossAccountLeak:
// the guest applies a promo code → registers with email/password (access moves
// to account1, the server "resets" the guest cookie). If an attacker manually
// replays the OLD guest cookie value on an OAuth login into ANOTHER existing
// account2, account2 must not get access.
func TestGuestFlowAbuse_RegisterThenOAuthLoginWithStaleGuestCookie_NoCrossAccountLeak(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)

	mode := f.CreateMode(TestModeOpts{})
	promo := f.CreatePromocode(TestPromocodeOpts{TargetID: mode.ID, MaxUses: 5})

	profile2 := OAuthProfile{
		Provider: "yandex", ProviderUserID: "uid-stale-" + itoa(time.Now().UnixNano()),
		Email: uniqueEmail("stale_acc2"), EmailVerified: true, DisplayName: "Stale Acc2",
	}
	pre := Handler{DB: env.Pool}
	acc2, err := pre.findOrCreateOAuthUser(context.Background(), profile2)
	if err != nil {
		t.Fatalf("precreate acc2: %v", err)
	}

	fake := fakeYandex(t, map[string]any{
		"id":            profile2.ProviderUserID,
		"display_name":  profile2.DisplayName,
		"default_email": profile2.Email,
	}, 200, 200)
	ts := NewTestServerWithHandler(t, env.Pool, Handler{
		HTTPClient: &http.Client{
			Transport: yandexRT{target: fake.URL},
			Timeout:   5 * time.Second,
		},
		YandexClientID:     "test-client",
		YandexClientSecret: "test-secret",
	})

	guestID := guestUserIDFromStatus(t, ts)
	guestCookieVal := ts.Handler.guestCookieValue(guestID)

	if code, body := httpJSON(t, ts, "POST", "/api/access/promocode/apply", map[string]any{"code": promo.Code}); code != http.StatusOK {
		t.Fatalf("apply promo: %d body=%v", code, body)
	}

	// Email/password registration moves access to account1 and cancels the guest access.
	email := uniqueEmail("stale_acc1")
	if code, body := httpJSON(t, ts, "POST", "/api/auth/register", map[string]any{
		"email": email, "password": "testpass123",
	}); code != http.StatusOK {
		t.Fatalf("register: %d body=%v", code, body)
	}

	var acc1ID int64
	if err := env.Pool.QueryRow(context.Background(),
		`select id from users where lower(email) = lower($1) and deleted_at is null`, email).Scan(&acc1ID); err != nil {
		t.Fatalf("query acc1 id: %v", err)
	}
	if got := countPromoAccess(t, env, acc1ID, mode.ID, promo.ID); got != 1 {
		t.Fatalf("acc1 promo access rows: got %d want 1", got)
	}

	// Replay the OLD guest cookie value on an OAuth login into account2.
	state := "stale-cookie-" + itoa(time.Now().UnixNano())
	redirectAfter := packOAuthRedirectAfter("/chat", "login")
	if _, err := env.Pool.Exec(context.Background(), `
		insert into oauth_states (token_hash, provider, redirect_after, ip_hash, user_agent, expires_at)
		values ($1, 'yandex', $2, 'ip', 'ua', now() + interval '5 minutes')`,
		tokenHash(state), redirectAfter); err != nil {
		t.Fatalf("insert oauth state: %v", err)
	}
	req, err := http.NewRequest("GET", ts.URL("/api/auth/oauth/yandex/callback?code=fake-code&state="+state), nil)
	if err != nil {
		t.Fatalf("build callback request: %v", err)
	}
	req.AddCookie(&http.Cookie{Name: oauthStateCookieName, Value: state})
	req.AddCookie(&http.Cookie{Name: guestCookieName, Value: guestCookieVal})
	resp, err := ts.Client.Do(req)
	if err != nil {
		t.Fatalf("callback: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("callback status: got %d want 302", resp.StatusCode)
	}

	if got := countPromoAccess(t, env, acc2.ID, mode.ID, promo.ID); got != 0 {
		t.Fatalf("stale guest cookie leaked promo access to acc2: got %d want 0", got)
	}
}
