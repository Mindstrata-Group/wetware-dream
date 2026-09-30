//go:build integration

package httpapi

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"mindstrata-stage1/api/internal/testsupport"
)

func guestUserIDFromJar(t testing.TB, ts *TestServer) int64 {
	t.Helper()
	baseURL, err := url.Parse(ts.server.URL)
	if err != nil {
		t.Fatalf("parse test server url: %v", err)
	}
	for _, cookie := range ts.Client.Jar.Cookies(baseURL) {
		if cookie.Name != guestCookieName {
			continue
		}
		id, err := strconv.ParseInt(cookie.Value, 10, 64)
		if err != nil {
			t.Fatalf("guest cookie is not a plain test id: %q", cookie.Value)
		}
		if id <= 0 {
			t.Fatalf("guest cookie id must be positive, got %d", id)
		}
		return id
	}
	t.Fatalf("guest cookie %q was not set", guestCookieName)
	return 0
}

func promoGuestCounts(t testing.TB, env *testsupport.Env, promoID int64) (usedCount, usageCount, accessCount int64) {
	t.Helper()
	ctx := context.Background()
	if err := env.Pool.QueryRow(ctx, `select used_count from promocodes where id=$1`, promoID).Scan(&usedCount); err != nil {
		t.Fatalf("select promo used_count: %v", err)
	}
	if err := env.Pool.QueryRow(ctx, `select count(*) from promocode_usages where promocode_id=$1`, promoID).Scan(&usageCount); err != nil {
		t.Fatalf("select promo usages: %v", err)
	}
	if err := env.Pool.QueryRow(ctx, `
		select count(*)
		from user_mode_access
		where source_id=$1 and access_type='promocode'`, promoID).Scan(&accessCount); err != nil {
		t.Fatalf("select promo access rows: %v", err)
	}
	return usedCount, usageCount, accessCount
}

func TestPromoGuestEnterprise_ModePromoWorksForGuestWithoutLogin(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	mode := f.CreateMode(TestModeOpts{Name: "guest_enterprise_mode"})
	promo := f.CreatePromocode(TestPromocodeOpts{TargetID: mode.ID, DurationDays: 7})

	status, body := httpJSON(t, ts, "POST", "/api/access/promocode/apply", map[string]any{"code": promo.Code})
	if status != http.StatusOK {
		t.Fatalf("guest promo apply: got %d body=%v", status, body)
	}
	if ok, _ := body["ok"].(bool); !ok {
		t.Fatalf("guest promo apply returned ok=false: %v", body)
	}
	if requiresAuth, _ := body["requiresAuth"].(bool); requiresAuth {
		t.Fatalf("ordinary mode promo must not require login: %v", body)
	}

	guestID := guestUserIDFromJar(t, ts)
	var accessRows int64
	if err := env.Pool.QueryRow(context.Background(), `
		select count(*)
		from user_mode_access
		where user_id=$1 and mode_id=$2 and source_id=$3 and access_type='promocode'`,
		guestID, mode.ID, promo.ID).Scan(&accessRows); err != nil {
		t.Fatalf("select guest access: %v", err)
	}
	if accessRows != 1 {
		t.Fatalf("guest access rows: got %d want 1", accessRows)
	}

	usedCount, usageCount, accessCount := promoGuestCounts(t, env, promo.ID)
	if usedCount != 1 || usageCount != 1 || accessCount != 1 {
		t.Fatalf("promo counters diverged: used=%d usages=%d access=%d, want all 1",
			usedCount, usageCount, accessCount)
	}
}

func TestPromoGuestEnterprise_AdminRolePromoRequiresAuth(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	promo := f.CreatePromocode(TestPromocodeOpts{GrantsType: "admin_role", TargetID: 0})

	status, body := httpJSON(t, ts, "POST", "/api/access/promocode/apply", map[string]any{"code": promo.Code})
	if status != http.StatusUnauthorized {
		t.Fatalf("anonymous admin-role promo: got %d body=%v, want 401", status, body)
	}
	if code, _ := body["errorCode"].(string); code != "auth_required" {
		t.Fatalf("anonymous admin-role promo errorCode: got %q body=%v, want auth_required", code, body)
	}
	if requiresAuth, _ := body["requiresAuth"].(bool); !requiresAuth {
		t.Fatalf("anonymous admin-role promo must set requiresAuth=true: %v", body)
	}

	usedCount, usageCount, accessCount := promoGuestCounts(t, env, promo.ID)
	if usedCount != 0 || usageCount != 0 || accessCount != 0 {
		t.Fatalf("auth-required admin promo must not mutate counters: used=%d usages=%d access=%d",
			usedCount, usageCount, accessCount)
	}
}

func TestPromoGuestEnterprise_RepeatedUseBySameGuestIsIdempotentlyRejected(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	mode := f.CreateMode(TestModeOpts{})
	promo := f.CreatePromocode(TestPromocodeOpts{TargetID: mode.ID})

	status, body := httpJSON(t, ts, "POST", "/api/access/promocode/apply", map[string]any{"code": promo.Code})
	if status != http.StatusOK {
		t.Fatalf("first guest promo apply: got %d body=%v", status, body)
	}
	firstGuestID := guestUserIDFromJar(t, ts)

	status, body = httpJSON(t, ts, "POST", "/api/access/promocode/apply", map[string]any{"code": promo.Code})
	if status != http.StatusBadRequest {
		t.Fatalf("repeated guest promo apply: got %d body=%v, want 400", status, body)
	}
	if code, _ := body["errorCode"].(string); code != "already_used" {
		t.Fatalf("repeated guest promo errorCode: got %q body=%v, want already_used", code, body)
	}
	if secondGuestID := guestUserIDFromJar(t, ts); secondGuestID != firstGuestID {
		t.Fatalf("same guest browser changed identity: first=%d second=%d", firstGuestID, secondGuestID)
	}

	usedCount, usageCount, accessCount := promoGuestCounts(t, env, promo.ID)
	if usedCount != 1 || usageCount != 1 || accessCount != 1 {
		t.Fatalf("repeated use must not double-count: used=%d usages=%d access=%d, want all 1",
			usedCount, usageCount, accessCount)
	}
}

func TestPromoGuestEnterprise_MaxUsesStopsSecondGuest(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)

	mode := f.CreateMode(TestModeOpts{})
	promo := f.CreatePromocode(TestPromocodeOpts{TargetID: mode.ID, MaxUses: 1})

	first := NewTestServer(t, env.Pool)
	status, body := httpJSON(t, first, "POST", "/api/access/promocode/apply", map[string]any{"code": promo.Code})
	if status != http.StatusOK {
		t.Fatalf("first guest promo apply: got %d body=%v", status, body)
	}

	second := NewTestServer(t, env.Pool)
	status, body = httpJSON(t, second, "POST", "/api/access/promocode/apply", map[string]any{"code": promo.Code})
	if status != http.StatusBadRequest {
		t.Fatalf("second guest over max_uses: got %d body=%v, want 400", status, body)
	}
	if code, _ := body["code"].(string); code != "limit_reached" {
		t.Fatalf("second guest over max_uses code: got %q body=%v, want limit_reached", code, body)
	}

	usedCount, usageCount, accessCount := promoGuestCounts(t, env, promo.ID)
	if usedCount != 1 || usageCount != 1 || accessCount != 1 {
		t.Fatalf("max_uses=1 counters diverged: used=%d usages=%d access=%d, want all 1",
			usedCount, usageCount, accessCount)
	}
}

// Regression (2026-09-30): ApplyPromocode opened its transaction, locked the
// promo row and only then created the guest user through the pool. That
// needs a second connection while the first one is held, so when concurrent
// applies occupy the whole pool every request waits for the others until the
// acquire timeout. It showed up on 2-CPU CI machines (pgxpool defaults to
// max(4, NumCPU) connections); in production it is the same deadlock at 30
// concurrent requests. A pool of one connection makes it deterministic: a
// single guest apply must not need a second connection.
func TestPromoGuestEnterprise_ApplyWorksWithSingleConnectionPool(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	mode := f.CreateMode(TestModeOpts{})
	promo := f.CreatePromocode(TestPromocodeOpts{TargetID: mode.ID, MaxUses: 5})

	cfg, err := pgxpool.ParseConfig(env.DSN)
	if err != nil {
		t.Fatal(err)
	}
	cfg.MaxConns = 1
	cfg.MinConns = 0
	one, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(one.Close)

	ts := NewTestServer(t, one)
	status, body := httpJSON(t, ts, "POST", "/api/access/promocode/apply", map[string]any{"code": promo.Code})
	if status != http.StatusOK {
		t.Fatalf("guest apply with a one-connection pool: got %d body=%v, want 200", status, body)
	}
}

func TestPromoGuestEnterprise_RaceOnMaxUsesAllowsExactlyNWinners(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)

	const maxUses = 3
	const contenders = 8

	mode := f.CreateMode(TestModeOpts{})
	promo := f.CreatePromocode(TestPromocodeOpts{TargetID: mode.ID, MaxUses: maxUses})

	var success, limitReached, other atomic.Int64
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < contenders; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ts := NewTestServer(t, env.Pool)
			<-start
			status, body := httpJSON(t, ts, "POST", "/api/access/promocode/apply", map[string]any{"code": promo.Code})
			switch {
			case status == http.StatusOK:
				success.Add(1)
			case status == http.StatusBadRequest && body["code"] == "limit_reached":
				limitReached.Add(1)
			default:
				other.Add(1)
				t.Logf("unexpected race result: status=%d body=%v", status, body)
			}
		}()
	}
	close(start)
	wg.Wait()

	if got := success.Load(); got != maxUses {
		t.Fatalf("race winners: got %d want %d (limit_reached=%d other=%d)",
			got, maxUses, limitReached.Load(), other.Load())
	}
	if got := limitReached.Load(); got != contenders-maxUses {
		t.Fatalf("race losers: got %d want %d (success=%d other=%d)",
			got, contenders-maxUses, success.Load(), other.Load())
	}
	if got := other.Load(); got != 0 {
		t.Fatalf("race had %d unexpected responses", got)
	}

	usedCount, usageCount, accessCount := promoGuestCounts(t, env, promo.ID)
	if usedCount != maxUses || usageCount != maxUses || accessCount != maxUses {
		t.Fatalf("race counters diverged: used=%d usages=%d access=%d, want all %d",
			usedCount, usageCount, accessCount, maxUses)
	}
}
