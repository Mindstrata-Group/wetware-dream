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

// =============================================================================
// ORACLE TESTS (after Knuth): invariants that must always hold.
// Each test checks a property of the system, not a specific scenario.
// =============================================================================

// TestOracle_Promo_UsedCount_MatchesActualUsages: an integrity invariant.
// After K sequential applications of a promo code by different users:
//   - promocodes.used_count == K
//   - COUNT(*) FROM promocode_usages WHERE promocode_id = X == K
//   - both values are identical (the counter does not drift from the actual records)
func TestOracle_Promo_UsedCount_MatchesActualUsages(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)

	mode := f.CreateMode(TestModeOpts{})
	promo := f.CreatePromocode(TestPromocodeOpts{TargetID: mode.ID, MaxUses: 0})

	const K = 5
	for i := 0; i < K; i++ {
		ts := NewTestServer(t, env.Pool)
		user := f.CreateUser(TestUserOpts{})
		ts.LoginAs(f.CreateSession(user.ID))
		status, body := httpJSON(t, ts, "POST", "/api/access/promocode/apply", map[string]any{"code": promo.Code})
		if status != http.StatusOK {
			t.Fatalf("apply #%d: got %d body=%v", i+1, status, body)
		}
	}

	var usedCount, actualUsages int64
	_ = env.Pool.QueryRow(context.Background(),
		`select used_count from promocodes where id = $1`, promo.ID).Scan(&usedCount)
	_ = env.Pool.QueryRow(context.Background(),
		`select count(*) from promocode_usages where promocode_id = $1`, promo.ID).Scan(&actualUsages)

	if usedCount != K {
		t.Errorf("used_count=%d, want %d", usedCount, K)
	}
	if actualUsages != K {
		t.Errorf("promocode_usages count=%d, want %d", actualUsages, K)
	}
	if usedCount != actualUsages {
		t.Errorf("ORACLE: used_count=%d != actual_usages=%d (расхождение счётчика)", usedCount, actualUsages)
	}
}

// TestOracle_Promo_SameUser_ConcurrentDoubleApply: one user, two concurrent
// requests for one promo code → exactly 1 success, 1 already_used.
// Unlike TestPromo_Apply_Race_MaxUses1: there the users differ, here it is one.
func TestOracle_Promo_SameUser_ConcurrentDoubleApply(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)

	mode := f.CreateMode(TestModeOpts{})
	promo := f.CreatePromocode(TestPromocodeOpts{TargetID: mode.ID})

	user := f.CreateUser(TestUserOpts{})

	// Two independent HTTP clients, one user (two valid tokens).
	ts1 := NewTestServer(t, env.Pool)
	ts1.LoginAs(f.CreateSession(user.ID))
	ts2 := NewTestServer(t, env.Pool)
	ts2.LoginAs(f.CreateSession(user.ID))

	var success, alreadyUsed, other atomic.Int64
	var wg sync.WaitGroup
	var start sync.WaitGroup
	start.Add(1)

	send := func(ts *TestServer) {
		defer wg.Done()
		start.Wait()
		status, body := httpJSON(t, ts, "POST", "/api/access/promocode/apply", map[string]any{"code": promo.Code})
		switch {
		case status == http.StatusOK:
			success.Add(1)
		case status == http.StatusBadRequest && body["errorCode"] == "already_used":
			alreadyUsed.Add(1)
		default:
			other.Add(1)
			t.Logf("unexpected: status=%d body=%v", status, body)
		}
	}

	wg.Add(2)
	go send(ts1)
	go send(ts2)
	start.Done() // start both at once
	wg.Wait()

	if got := success.Load(); got != 1 {
		t.Errorf("same-user double-apply: expected 1 success, got %d (already_used=%d other=%d)",
			got, alreadyUsed.Load(), other.Load())
	}

	var cnt int64
	_ = env.Pool.QueryRow(context.Background(),
		`select count(*) from promocode_usages where user_id = $1 and promocode_id = $2`,
		user.ID, promo.ID).Scan(&cnt)
	if cnt != 1 {
		t.Errorf("ORACLE: promocode_usages=%d для одного пользователя, want 1", cnt)
	}
}

// TestOracle_Promo_NWinnerRace: max_uses=N, M>N goroutines (different users) →
// exactly N successes. Extends TestPromo_Apply_Race_MaxUses1 to N>1.
func TestOracle_Promo_NWinnerRace(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)

	const maxUses = 3
	const goroutines = 10

	mode := f.CreateMode(TestModeOpts{})
	promo := f.CreatePromocode(TestPromocodeOpts{TargetID: mode.ID, MaxUses: maxUses})

	var success, limitReached, other atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ts := NewTestServer(t, env.Pool)
			user := f.CreateUser(TestUserOpts{})
			ts.LoginAs(f.CreateSession(user.ID))
			status, body := httpJSON(t, ts, "POST", "/api/access/promocode/apply", map[string]any{"code": promo.Code})
			switch {
			case status == http.StatusOK:
				success.Add(1)
			case status == http.StatusBadRequest &&
				(body["code"] == "limit_reached" || body["errorCode"] == "limit_reached"):
				limitReached.Add(1)
			default:
				other.Add(1)
				t.Logf("unexpected: status=%d body=%v", status, body)
			}
		}()
	}
	wg.Wait()

	if got := success.Load(); got != maxUses {
		t.Errorf("N-winner race: expected %d successes, got %d (limit_reached=%d other=%d)",
			maxUses, got, limitReached.Load(), other.Load())
	}

	var usedCount, accessCount int64
	_ = env.Pool.QueryRow(context.Background(),
		`select used_count from promocodes where id = $1`, promo.ID).Scan(&usedCount)
	_ = env.Pool.QueryRow(context.Background(),
		`select count(*) from user_mode_access where source_id = $1 and access_type = 'promocode'`,
		promo.ID).Scan(&accessCount)

	if usedCount != maxUses {
		t.Errorf("ORACLE: used_count=%d, want %d", usedCount, maxUses)
	}
	if accessCount != maxUses {
		t.Errorf("ORACLE: user_mode_access count=%d, want %d", accessCount, maxUses)
	}
}

// TestOracle_Session_BlockedAfterLogin: the user is blocked AFTER the session was
// created → an existing valid token → /api/auth/me → 401.
// The middleware checks u.status='active' on every request.
func TestOracle_Session_BlockedAfterLogin(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	user := f.CreateUser(TestUserOpts{Status: "active"})
	ts.LoginAs(f.CreateSession(user.ID))

	code, _ := httpJSON(t, ts, "GET", "/api/auth/me", nil)
	if code != http.StatusOK {
		t.Fatalf("pre-block /me: expected 200, got %d", code)
	}

	_, err := env.Pool.Exec(context.Background(),
		`update users set status = 'blocked' where id = $1`, user.ID)
	if err != nil {
		t.Fatalf("block user: %v", err)
	}

	code2, _ := httpJSON(t, ts, "GET", "/api/auth/me", nil)
	if code2 != http.StatusUnauthorized {
		t.Errorf("ORACLE: post-block /me: expected 401, got %d", code2)
	}
}

// TestOracle_AccessStatus_ExpiredGrantNotVisible: a grant with active_to in the
// past → GET /api/access/status → hasAccess=false, activeModes=[].
// Tests the HTTP layer (unlike TestQuota_PerModeLimit_ExpiredAccess, which goes through the DB).
func TestOracle_AccessStatus_ExpiredGrantNotVisible(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	user := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{})

	past := time.Now().Add(-48 * time.Hour)
	pastPast := time.Now().Add(-72 * time.Hour)
	f.GrantAccess(GrantAccessOpts{
		UserID:            user.ID,
		ModeID:            mode.ID,
		DailyMessageLimit: 50,
		ActiveFrom:        &pastPast,
		ActiveTo:          &past,
	})

	ts.LoginAs(f.CreateSession(user.ID))
	status, body := httpJSON(t, ts, "GET", "/api/access/status", nil)
	if status != http.StatusOK {
		t.Fatalf("access/status: got %d body=%v", status, body)
	}

	if hasAccess, _ := body["hasAccess"].(bool); hasAccess {
		t.Errorf("ORACLE: expired grant не должен давать доступ, но hasAccess=true")
	}
	if modes, _ := body["activeModes"].([]any); len(modes) > 0 {
		t.Errorf("ORACLE: activeModes должен быть пуст при expired grant, got %v", modes)
	}
}

// TestOracle_DailyLimit_HardStop: limit=3, 4 sequential attempts →
// the first 3 ok, the 4th blocked. DB: daily_message_counts.count is exactly 3.
func TestOracle_DailyLimit_HardStop(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	h := Handler{DB: env.Pool}

	user := f.CreateUser(TestUserOpts{})

	const limit = 3
	for i := 1; i <= limit; i++ {
		_, ok, err := h.tryIncrementDailyMessageCount(context.Background(), user.ID, limit, 1)
		if err != nil {
			t.Fatalf("tryIncrement #%d: %v", i, err)
		}
		if !ok {
			t.Fatalf("tryIncrement #%d: expected ok=true (в пределах лимита), got false", i)
		}
	}

	_, ok, err := h.tryIncrementDailyMessageCount(context.Background(), user.ID, limit, 1)
	if err != nil {
		t.Fatalf("tryIncrement #%d (сверх лимита): %v", limit+1, err)
	}
	if ok {
		t.Errorf("ORACLE: %d-й инкремент при limit=%d должен быть заблокирован, но ok=true", limit+1, limit)
	}

	var count int64
	_ = env.Pool.QueryRow(context.Background(),
		`select count from daily_message_counts where user_id = $1 and date = current_date`,
		user.ID).Scan(&count)
	if count != limit {
		t.Errorf("ORACLE: daily_message_counts.count=%d, want %d (счётчик не должен превышать лимит)", count, limit)
	}
}

// TestOracle_Access_ActiveGrant_IsVisible: a user with an active grant →
// GET /api/access/status → hasAccess=true, activeModes contains the mode.
func TestOracle_Access_ActiveGrant_IsVisible(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	user := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{})
	f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: mode.ID, DailyMessageLimit: 50})

	ts.LoginAs(f.CreateSession(user.ID))
	status, body := httpJSON(t, ts, "GET", "/api/access/status", nil)
	if status != http.StatusOK {
		t.Fatalf("access/status: got %d body=%v", status, body)
	}

	if hasAccess, _ := body["hasAccess"].(bool); !hasAccess {
		t.Errorf("ORACLE: активный грант должен давать доступ, но hasAccess=false")
	}

	modes, _ := body["activeModes"].([]any)
	if len(modes) == 0 {
		t.Fatalf("ORACLE: activeModes пусто при активном гранте")
	}
	found := false
	for _, m := range modes {
		if mv, ok := m.(map[string]any); ok {
			if id, _ := mv["modeId"].(float64); int64(id) == mode.ID {
				found = true
				break
			}
		}
	}
	if !found {
		t.Errorf("ORACLE: mode_id=%d не найден в activeModes: %v", mode.ID, modes)
	}
}

// TestOracle_Promo_DailyLimit_PropagatesToAccess: after applying a promo code
// user_mode_access.daily_message_limit == 50 (default from promocodes).
// Invariant: the limit is correctly copied from the promo code to the access record.
func TestOracle_Promo_DailyLimit_PropagatesToAccess(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	user := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{})
	promo := f.CreatePromocode(TestPromocodeOpts{TargetID: mode.ID})

	ts.LoginAs(f.CreateSession(user.ID))
	status, body := httpJSON(t, ts, "POST", "/api/access/promocode/apply", map[string]any{"code": promo.Code})
	if status != http.StatusOK {
		t.Fatalf("apply promo: got %d body=%v", status, body)
	}

	var limit int64
	err := env.Pool.QueryRow(context.Background(),
		`select daily_message_limit from user_mode_access
		 where user_id = $1 and mode_id = $2 and access_type = 'promocode'`,
		user.ID, mode.ID).Scan(&limit)
	if err != nil {
		t.Fatalf("query access limit: %v", err)
	}
	if limit != defaultDailyMessageLimit {
		t.Errorf("ORACLE: daily_message_limit=%d в user_mode_access, want %d", limit, defaultDailyMessageLimit)
	}
}

// TestOracle_DailyLimit_ParallelNoOverflow: limit=5, 10 concurrent goroutines →
// the final counter in the DB does not exceed 5 (atomicity guaranteed by ON CONFLICT).
func TestOracle_DailyLimit_ParallelNoOverflow(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	h := Handler{DB: env.Pool}

	user := f.CreateUser(TestUserOpts{})

	const limit = 5
	const goroutines = 10

	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, _ = h.tryIncrementDailyMessageCount(context.Background(), user.ID, limit, 1)
		}()
	}
	wg.Wait()

	var count int64
	_ = env.Pool.QueryRow(context.Background(),
		`select count from daily_message_counts where user_id = $1 and date = current_date`,
		user.ID).Scan(&count)
	if count > limit {
		t.Errorf("ORACLE: parallel overflow! count=%d > limit=%d", count, limit)
	}
}

// TestOracle_Promo_UsedCount_NeverExceedsMaxUses: concurrent N-winner →
// used_count == count(promocode_usages) AND both == maxUses.
// A double check: the counter and the real records always match after the race.
func TestOracle_Promo_UsedCount_NeverExceedsMaxUses(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)

	const maxUses = 4
	const goroutines = 8

	mode := f.CreateMode(TestModeOpts{})
	promo := f.CreatePromocode(TestPromocodeOpts{TargetID: mode.ID, MaxUses: maxUses})

	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ts := NewTestServer(t, env.Pool)
			user := f.CreateUser(TestUserOpts{})
			ts.LoginAs(f.CreateSession(user.ID))
			httpJSON(t, ts, "POST", "/api/access/promocode/apply", map[string]any{"code": promo.Code})
		}()
	}
	wg.Wait()

	var usedCount, actualUsages int64
	_ = env.Pool.QueryRow(context.Background(),
		`select used_count from promocodes where id = $1`, promo.ID).Scan(&usedCount)
	_ = env.Pool.QueryRow(context.Background(),
		`select count(*) from promocode_usages where promocode_id = $1`, promo.ID).Scan(&actualUsages)

	if usedCount > maxUses {
		t.Errorf("ORACLE: used_count=%d > maxUses=%d (превышение недопустимо)", usedCount, maxUses)
	}
	if actualUsages > maxUses {
		t.Errorf("ORACLE: actual_usages=%d > maxUses=%d", actualUsages, maxUses)
	}
	if usedCount != actualUsages {
		t.Errorf("ORACLE: расхождение после гонки: used_count=%d != actual_usages=%d", usedCount, actualUsages)
	}
	if usedCount != maxUses {
		t.Errorf("ORACLE: ожидали ровно %d успехов, получили used_count=%d", maxUses, usedCount)
	}
}

// TestOracle_Session_ExpiredSession: a session with expires_at in the past →
// GET /api/auth/me → 401. The middleware rejects expired tokens.
func TestOracle_Session_ExpiredSession(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	user := f.CreateUser(TestUserOpts{})

	// Create the session directly with expires_at in the past.
	token, err := randomToken(32)
	if err != nil {
		t.Fatalf("random token: %v", err)
	}
	_, err = env.Pool.Exec(context.Background(), `
		insert into auth_sessions (user_id, token_hash, user_agent, ip_hash, expires_at)
		values ($1, $2, 'test-agent', 'test-ip-hash', $3)`,
		user.ID, tokenHash(token), time.Now().Add(-time.Hour),
	)
	if err != nil {
		t.Fatalf("insert expired session: %v", err)
	}

	ts.LoginAs(token)
	code, _ := httpJSON(t, ts, "GET", "/api/auth/me", nil)
	if code != http.StatusUnauthorized {
		t.Errorf("ORACLE: expired session должна возвращать 401, got %d", code)
	}
}
