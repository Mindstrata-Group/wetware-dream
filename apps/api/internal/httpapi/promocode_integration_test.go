//go:build integration

package httpapi

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"mindstrata-stage1/api/internal/testsupport"
)

// applyPromoOK: helper that asserts apply succeeded; returns parsed body.
func applyPromoOK(t *testing.T, ts *TestServer, code string) map[string]any {
	t.Helper()
	status, body := httpJSON(t, ts, "POST", "/api/access/promocode/apply", map[string]any{"code": code})
	if status != http.StatusOK {
		t.Fatalf("apply %q: got %d body=%v", code, status, body)
	}
	return body
}

// TestPromo_Apply_HappyPath_GrantMode: a new code → grant created, used_count++.
func TestPromo_Apply_HappyPath_GrantMode(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	user := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{})
	promo := f.CreatePromocode(TestPromocodeOpts{TargetID: mode.ID, DurationDays: 30})

	token := f.CreateSession(user.ID)
	ts.LoginAs(token)

	body := applyPromoOK(t, ts, promo.Code)
	if ok, _ := body["ok"].(bool); !ok {
		t.Fatalf("apply: ok=false: %v", body)
	}

	// DB: user_mode_access created
	var accessCount int64
	_ = env.Pool.QueryRow(context.Background(),
		`select count(*) from user_mode_access where user_id = $1 and mode_id = $2 and access_type = 'promocode'`,
		user.ID, mode.ID).Scan(&accessCount)
	if accessCount == 0 {
		t.Fatalf("user_mode_access not created")
	}

	// DB: used_count incremented
	var usedCount int64
	_ = env.Pool.QueryRow(context.Background(),
		`select used_count from promocodes where id = $1`, promo.ID).Scan(&usedCount)
	if usedCount != 1 {
		t.Fatalf("promo used_count: got %d want 1", usedCount)
	}

	// DB: a record in promocode_usages
	var usageCount int64
	_ = env.Pool.QueryRow(context.Background(),
		`select count(*) from promocode_usages where user_id = $1 and promocode_id = $2`,
		user.ID, promo.ID).Scan(&usageCount)
	if usageCount != 1 {
		t.Fatalf("promocode_usages: got %d want 1", usageCount)
	}
}

// TestPromo_Apply_AlreadyUsedByUser: a second time by the same user → already_used.
func TestPromo_Apply_AlreadyUsedByUser(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	user := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{})
	promo := f.CreatePromocode(TestPromocodeOpts{TargetID: mode.ID})

	token := f.CreateSession(user.ID)
	ts.LoginAs(token)

	applyPromoOK(t, ts, promo.Code)

	// Second time
	status, body := httpJSON(t, ts, "POST", "/api/access/promocode/apply", map[string]any{"code": promo.Code})
	if status != http.StatusBadRequest {
		t.Fatalf("second apply: expected 400, got %d body=%v", status, body)
	}
	if errCode, _ := body["errorCode"].(string); errCode != "already_used" {
		t.Fatalf("expected errorCode=already_used, got %v", body["errorCode"])
	}
}

// TestPromo_Apply_NotFound: non-existent code → 404.
func TestPromo_Apply_NotFound(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	user := f.CreateUser(TestUserOpts{})
	token := f.CreateSession(user.ID)
	ts.LoginAs(token)

	status, body := httpJSON(t, ts, "POST", "/api/access/promocode/apply", map[string]any{"code": "DOESNOTEXIST"})
	if status != http.StatusNotFound {
		t.Fatalf("expected 404, got %d body=%v", status, body)
	}
}

// TestPromo_Apply_Expired: active_to in the past → 400 expired.
func TestPromo_Apply_Expired(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	user := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{})
	past := time.Now().Add(-24 * time.Hour)
	pastFrom := time.Now().Add(-48 * time.Hour)
	promo := f.CreatePromocode(TestPromocodeOpts{
		TargetID: mode.ID, ActiveFrom: &pastFrom, ActiveTo: &past,
	})
	token := f.CreateSession(user.ID)
	ts.LoginAs(token)

	status, body := httpJSON(t, ts, "POST", "/api/access/promocode/apply", map[string]any{"code": promo.Code})
	if status != http.StatusBadRequest {
		t.Fatalf("expected 400 for expired, got %d body=%v", status, body)
	}
	if c, _ := body["code"].(string); c != "expired" {
		t.Fatalf("expected code=expired, got %v", body["code"])
	}
}

// TestPromo_Apply_NotStarted: active_from in the future → 400 not_started.
func TestPromo_Apply_NotStarted(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	user := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{})
	future := time.Now().Add(24 * time.Hour)
	farFuture := time.Now().Add(72 * time.Hour)
	promo := f.CreatePromocode(TestPromocodeOpts{
		TargetID: mode.ID, ActiveFrom: &future, ActiveTo: &farFuture,
	})
	token := f.CreateSession(user.ID)
	ts.LoginAs(token)

	status, body := httpJSON(t, ts, "POST", "/api/access/promocode/apply", map[string]any{"code": promo.Code})
	if status != http.StatusBadRequest {
		t.Fatalf("expected 400 for not_started, got %d body=%v", status, body)
	}
	if c, _ := body["errorCode"].(string); c != "not_started" {
		t.Fatalf("expected errorCode=not_started, got %v body=%v", body["errorCode"], body)
	}
}

// TestPromo_Apply_LimitReached: max_uses=1, already used → 400 limit_reached.
func TestPromo_Apply_LimitReached(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	// Promo code with limit 1
	mode := f.CreateMode(TestModeOpts{})
	promo := f.CreatePromocode(TestPromocodeOpts{TargetID: mode.ID, MaxUses: 1})

	// First user: success
	user1 := f.CreateUser(TestUserOpts{})
	token1 := f.CreateSession(user1.ID)
	ts.LoginAs(token1)
	applyPromoOK(t, ts, promo.Code)

	// Second user: limit_reached
	ts2 := NewTestServer(t, env.Pool)
	user2 := f.CreateUser(TestUserOpts{})
	token2 := f.CreateSession(user2.ID)
	ts2.LoginAs(token2)
	status, body := httpJSON(t, ts2, "POST", "/api/access/promocode/apply", map[string]any{"code": promo.Code})
	if status != http.StatusBadRequest {
		t.Fatalf("expected 400 limit_reached, got %d body=%v", status, body)
	}
	if c, _ := body["code"].(string); c != "limit_reached" {
		t.Fatalf("expected code=limit_reached, got %v body=%v", body["code"], body)
	}
}

// TestPromo_Apply_CaseInsensitive: the code entered in lowercase is found (UPPER).
func TestPromo_Apply_CaseInsensitive(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	user := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{})
	promo := f.CreatePromocode(TestPromocodeOpts{Code: "MYPROMO123", TargetID: mode.ID})

	token := f.CreateSession(user.ID)
	ts.LoginAs(token)

	// Apply in lowercase: the handler does strings.ToUpper.
	status, body := httpJSON(t, ts, "POST", "/api/access/promocode/apply", map[string]any{"code": "mypromo123"})
	if status != http.StatusOK {
		t.Fatalf("case-insensitive apply: %d body=%v", status, body)
	}
	_ = promo
}

// TestPromo_Apply_AdminRole_RequiresAuth: anonymous + grants_type=admin_role → 401 auth_required.
func TestPromo_Apply_AdminRole_RequiresAuth(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	mode := f.CreateMode(TestModeOpts{})
	promo := f.CreatePromocode(TestPromocodeOpts{TargetID: mode.ID, GrantsType: "admin_role"})

	// No session
	status, body := httpJSON(t, ts, "POST", "/api/access/promocode/apply", map[string]any{"code": promo.Code})
	if status != http.StatusUnauthorized {
		t.Fatalf("expected 401 for anon admin promo, got %d body=%v", status, body)
	}
	if c, _ := body["errorCode"].(string); c != "auth_required" {
		t.Fatalf("expected errorCode=auth_required, got %v", body["errorCode"])
	}
}

// TestPromo_Apply_AdminRole_UpgradesRole: authed user + admin_role promo → role=admin.
func TestPromo_Apply_AdminRole_UpgradesRole(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	user := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{})
	promo := f.CreatePromocode(TestPromocodeOpts{TargetID: mode.ID, GrantsType: "admin_role"})

	token := f.CreateSession(user.ID)
	ts.LoginAs(token)

	body := applyPromoOK(t, ts, promo.Code)
	if upgraded, _ := body["roleUpgraded"].(bool); !upgraded {
		t.Fatalf("expected roleUpgraded=true: %v", body)
	}

	// DB: role upgraded to admin
	var role string
	_ = env.Pool.QueryRow(context.Background(),
		`select role from users where id = $1`, user.ID).Scan(&role)
	if role != "admin" {
		t.Fatalf("user.role: got %q want admin", role)
	}
}

// TestPromo_Apply_Race_MaxUses1: 2 users at once on max_uses=1 → exactly 1 success.
// The inner transaction with FOR UPDATE must serialise.
func TestPromo_Apply_Race_MaxUses1(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)

	mode := f.CreateMode(TestModeOpts{})
	promo := f.CreatePromocode(TestPromocodeOpts{TargetID: mode.ID, MaxUses: 1})

	const N = 5

	var success, limitReached, other atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ts := NewTestServer(t, env.Pool)
			user := f.CreateUser(TestUserOpts{})
			token := f.CreateSession(user.ID)
			ts.LoginAs(token)
			status, body := httpJSON(t, ts, "POST", "/api/access/promocode/apply", map[string]any{"code": promo.Code})
			switch {
			case status == http.StatusOK:
				success.Add(1)
			case status == http.StatusBadRequest && body["code"] == "limit_reached":
				limitReached.Add(1)
			default:
				other.Add(1)
				t.Logf("unexpected: status=%d body=%v", status, body)
			}
		}()
	}
	wg.Wait()

	// CRITICAL INVARIANT: exactly 1 success
	if got := success.Load(); got != 1 {
		t.Fatalf("promo race: expected exactly 1 success, got %d (limit_reached=%d other=%d)",
			got, limitReached.Load(), other.Load())
	}
	if got := limitReached.Load(); got < int64(N-1) {
		t.Logf("WARN: only %d/%d got limit_reached (expected ~%d). Others=%d", got, N, N-1, other.Load())
	}

	// DB invariant: used_count = 1
	var used int64
	_ = env.Pool.QueryRow(context.Background(),
		`select used_count from promocodes where id = $1`, promo.ID).Scan(&used)
	if used != 1 {
		t.Fatalf("promo used_count race: got %d want 1", used)
	}
}
