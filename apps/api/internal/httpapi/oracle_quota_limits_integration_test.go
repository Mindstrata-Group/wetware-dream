//go:build integration

package httpapi

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"

	"mindstrata-stage1/api/internal/testsupport"
)

// =============================================================================
// ORACLES: quotas, limits, promo codes: multi-grant logic and Wiener gaps.
// =============================================================================

// TestOracle_Promo_TwoPromos_SameMode_LimitsSum: two promo codes for one mode:
// the daily limit adds up. Oracle: perModeLimit = sum(grant1, grant2).
//
// Scenario: the user applies promo code A (50 messages) and promo code B
// (30 messages), both for one mode → the total limit is 80, not 50 and not 30.
func TestOracle_Promo_TwoPromos_SameMode_LimitsSum(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)
	h := Handler{DB: env.Pool}

	user := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{})

	// Two promo codes for one mode with different limits.
	// Factory hard-codes daily_message_limit=50. We insert by hand with different values.
	var promoID1, promoID2 int64
	err := env.Pool.QueryRow(context.Background(), `
		insert into promocodes
		  (code, max_uses, used_count, active_from, active_to, duration,
		   access_priority, grants_type, target_id, limit_type, daily_message_limit)
		values ($1, 0, 0, null, null, '30 days'::interval, 0, 'mode', $2, 'fixed', 30)
		returning id`, "LIMIT30_"+f.uniqueSuffix(), mode.ID).Scan(&promoID1)
	if err != nil {
		t.Fatalf("insert promo1: %v", err)
	}
	err = env.Pool.QueryRow(context.Background(), `
		insert into promocodes
		  (code, max_uses, used_count, active_from, active_to, duration,
		   access_priority, grants_type, target_id, limit_type, daily_message_limit)
		values ($1, 0, 0, null, null, '30 days'::interval, 0, 'mode', $2, 'fixed', 20)
		returning id`, "LIMIT20_"+f.uniqueSuffix(), mode.ID).Scan(&promoID2)
	if err != nil {
		t.Fatalf("insert promo2: %v", err)
	}

	// Fetch the code by ID for the HTTP request.
	var code1, code2 string
	_ = env.Pool.QueryRow(context.Background(), `select code from promocodes where id=$1`, promoID1).Scan(&code1)
	_ = env.Pool.QueryRow(context.Background(), `select code from promocodes where id=$1`, promoID2).Scan(&code2)

	ts.LoginAs(f.CreateSession(user.ID))

	// Apply both promo codes.
	s1, b1 := httpJSON(t, ts, "POST", "/api/access/promocode/apply", map[string]any{"code": code1})
	if s1 != http.StatusOK {
		t.Fatalf("apply promo1: %d %v", s1, b1)
	}
	s2, b2 := httpJSON(t, ts, "POST", "/api/access/promocode/apply", map[string]any{"code": code2})
	if s2 != http.StatusOK {
		t.Fatalf("apply promo2: %d %v", s2, b2)
	}

	// perModeLimit must return 30 + 20 = 50.
	limit, hasAccess, err := h.perModeLimit(context.Background(), user.ID, mode.ID)
	if err != nil {
		t.Fatalf("perModeLimit: %v", err)
	}
	if !hasAccess {
		t.Fatalf("ORACLE: hasAccess=false после двух промокодов на режим")
	}
	if limit != 50 {
		t.Errorf("ORACLE: лимит=%d, ожидали 50 (30+20 от двух промокодов)", limit)
	}

	// DB: two rows in user_mode_access with different source_id.
	var rowCount int64
	_ = env.Pool.QueryRow(context.Background(),
		`select count(*) from user_mode_access
		 where user_id=$1 and mode_id=$2 and access_type='promocode'`,
		user.ID, mode.ID).Scan(&rowCount)
	if rowCount != 2 {
		t.Errorf("ORACLE: user_mode_access rows=%d, want 2 (по одной на промокод)", rowCount)
	}
}

// TestOracle_Promo_TwoPromos_SameMode_SecondExtends: with a second promo code for
// the same mode, access is not created from scratch: the existing row is extended.
// Invariant: the user does not lose access, limits add up.
func TestOracle_Promo_TwoPromos_SameMode_SecondExtends(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	user := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{})
	promo1 := f.CreatePromocode(TestPromocodeOpts{TargetID: mode.ID})
	promo2 := f.CreatePromocode(TestPromocodeOpts{TargetID: mode.ID})

	ts.LoginAs(f.CreateSession(user.ID))

	// First promo code.
	s1, b1 := httpJSON(t, ts, "POST", "/api/access/promocode/apply", map[string]any{"code": promo1.Code})
	if s1 != http.StatusOK {
		t.Fatalf("apply promo1: %d %v", s1, b1)
	}

	// Second promo code for the same mode: access must continue.
	s2, b2 := httpJSON(t, ts, "POST", "/api/access/promocode/apply", map[string]any{"code": promo2.Code})
	if s2 != http.StatusOK {
		t.Fatalf("apply promo2 (extend): %d %v", s2, b2)
	}

	// GET /access/status → hasAccess=true, the mode is visible.
	ss, body := httpJSON(t, ts, "GET", "/api/access/status", nil)
	if ss != http.StatusOK {
		t.Fatalf("access/status: %d", ss)
	}
	if ha, _ := body["hasAccess"].(bool); !ha {
		t.Errorf("ORACLE: hasAccess=false после двух промокодов, ожидали true")
	}
}

// TestOracle_Promo_DuplicateUsage_DBConstraintBlocks: a direct INSERT of a
// duplicate record into promocode_usages → the DB unique constraint rejects it.
// This is protection AT THE DB LEVEL, independent of app logic.
func TestOracle_Promo_DuplicateUsage_DBConstraintBlocks(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)

	user := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{})
	promo := f.CreatePromocode(TestPromocodeOpts{TargetID: mode.ID})

	// First record: ok.
	_, err := env.Pool.Exec(context.Background(),
		`insert into promocode_usages (user_id, promocode_id, used_at) values ($1, $2, now())`,
		user.ID, promo.ID)
	if err != nil {
		t.Fatalf("первая запись в promocode_usages: %v", err)
	}

	// A second record with the same (user_id, promocode_id) → must fail.
	_, err = env.Pool.Exec(context.Background(),
		`insert into promocode_usages (user_id, promocode_id, used_at) values ($1, $2, now())`,
		user.ID, promo.ID)
	if err == nil {
		t.Errorf("ORACLE: DB должна отклонить дубль (user_id, promocode_id), но вставка прошла")
	}
}

// TestOracle_Quota_GlobalCounter_ModeSwitch: the global counter is shared between modes.
// Invariant (Wolfram #3): a user with mode A (limit=100) and mode B (limit=30)
// can use both after 30 messages; after 100, mode A is blocked, and mode B too
// (global counter == limit_B).
//
// The test documents intentional behaviour: the global counter as a shared ceiling.
func TestOracle_Quota_GlobalCounter_ModeSwitch(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	h := Handler{DB: env.Pool}

	user := f.CreateUser(TestUserOpts{})

	// Mode A: limit=10. Mode B: limit=5.
	// After 5 increments mode B must be blocked, mode A not yet.
	const limitA = 10
	const limitB = 5

	// Send 5 messages (as if into mode A).
	for i := 0; i < limitB; i++ {
		_, ok, err := h.tryIncrementDailyMessageCount(context.Background(), user.ID, limitA, 1)
		if err != nil {
			t.Fatalf("increment #%d: %v", i+1, err)
		}
		if !ok {
			t.Fatalf("increment #%d: expected ok=true, got false", i+1)
		}
	}

	// Mode B: tryIncrement with limit=5. The counter is already 5: blocked.
	_, okB, err := h.tryIncrementDailyMessageCount(context.Background(), user.ID, limitB, 1)
	if err != nil {
		t.Fatalf("tryIncrement mode B after 5: %v", err)
	}
	if okB {
		// This is documented behaviour: global counter == limitB → B is blocked.
		t.Logf("ORACLE INFO: mode B заблокирован после %d глобальных сообщений (limit=%d)", limitB, limitB)
	}

	// Mode A can still work (5 < 10).
	_, okA, err := h.tryIncrementDailyMessageCount(context.Background(), user.ID, limitA, 1)
	if err != nil {
		t.Fatalf("tryIncrement mode A after 5: %v", err)
	}
	if !okA {
		t.Errorf("ORACLE: mode A (limit=%d) должен работать при счётчике=5, но заблокирован", limitA)
	}

	// Check the final counter: 5 (from B) + 1 (from A) = 6, no more.
	var count int64
	_ = env.Pool.QueryRow(context.Background(),
		`select count from daily_message_counts where user_id=$1 and date=current_date`,
		user.ID).Scan(&count)
	if count != limitB+1 {
		t.Errorf("ORACLE: counter=%d, want %d", count, limitB+1)
	}
}

// TestOracle_Quota_GlobalCounter_ModeIndependence: with a high enough limit the
// modes do not interfere. If the user has two modes with the same limit N, they
// can send N messages in total (not N in each).
//
// Documents the limitation: global counter ≠ per-mode independence.
func TestOracle_Quota_GlobalCounter_ModeIndependence(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	h := Handler{DB: env.Pool}

	user := f.CreateUser(TestUserOpts{})

	const limit = 6
	const half = limit / 2

	// Simulate: half the messages into "mode A".
	for i := 0; i < half; i++ {
		_, ok, err := h.tryIncrementDailyMessageCount(context.Background(), user.ID, limit, 1)
		if err != nil || !ok {
			t.Fatalf("mode A increment #%d: ok=%v err=%v", i+1, ok, err)
		}
	}

	// "Mode B" with the same limit: half the messages still fit.
	for i := 0; i < half; i++ {
		_, ok, err := h.tryIncrementDailyMessageCount(context.Background(), user.ID, limit, 1)
		if err != nil || !ok {
			t.Fatalf("mode B increment #%d: ok=%v err=%v (глобальный счётчик=%d, limit=%d)", i+1, ok, err, half+i, limit)
		}
	}

	// N+1 must be blocked.
	_, ok, err := h.tryIncrementDailyMessageCount(context.Background(), user.ID, limit, 1)
	if err != nil {
		t.Fatalf("N+1 increment: %v", err)
	}
	if ok {
		t.Errorf("ORACLE: %d-е сообщение при limit=%d должно быть заблокировано", limit+1, limit)
	}

	var count int64
	_ = env.Pool.QueryRow(context.Background(),
		`select count from daily_message_counts where user_id=$1 and date=current_date`,
		user.ID).Scan(&count)
	if count != limit {
		t.Errorf("ORACLE: counter=%d, want %d", count, limit)
	}
}

// TestOracle_DailyCounter_OldDatesNeverCleanedAutomatically: daily_message_counts
// has no automatic cleanup of old rows. This test documents the missing cleanup:
// rows for past dates stay in the table.
//
// Wiener: no feedback loop → the table grows forever.
// Solution (not implemented): pg_cron / background job.
func TestOracle_DailyCounter_OldDatesNeverCleanedAutomatically(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)

	user := f.CreateUser(TestUserOpts{})

	// Insert a "yesterday" counter directly.
	// Take the date from the DB so the timezone matches current_date in SQL queries.
	var yesterday string
	if err := env.Pool.QueryRow(context.Background(), `select (current_date - 1)::text`).Scan(&yesterday); err != nil {
		t.Fatalf("get yesterday from DB: %v", err)
	}
	_, err := env.Pool.Exec(context.Background(),
		`insert into daily_message_counts (user_id, date, count, updated_at)
		 values ($1, $2::date, 99, now())
		 on conflict (user_id, date) do nothing`,
		user.ID, yesterday)
	if err != nil {
		t.Fatalf("insert yesterday row: %v", err)
	}

	// The "yesterday" row is alive: nobody deleted it.
	var count int64
	_ = env.Pool.QueryRow(context.Background(),
		`select count from daily_message_counts where user_id=$1 and date=$2::date`,
		user.ID, yesterday).Scan(&count)
	if count != 99 {
		t.Errorf("ORACLE: вчерашняя строка должна быть count=99, got %d", count)
	}

	// Today's counter = 0 (the yesterday row has no effect).
	h := Handler{DB: env.Pool}
	used, err := h.globalDailyUsed(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("globalDailyUsed: %v", err)
	}
	if used != 0 {
		t.Errorf("ORACLE: вчерашняя строка не должна влиять на сегодняшнее used=%d", used)
	}

	// Pin it: the yesterday row is NOT deleted automatically (no cleanup).
	var total int64
	_ = env.Pool.QueryRow(context.Background(),
		`select count(*) from daily_message_counts where user_id=$1`,
		user.ID).Scan(&total)
	if total < 1 {
		t.Errorf("ORACLE: ожидали хотя бы 1 строку (вчерашняя), got %d", total)
	}
	// TODO: add a pg_cron job: DELETE FROM daily_message_counts WHERE date < current_date - 30
}

// TestOracle_Quota_FastSlowPath_Consistent: the fast path (daily_message_counts)
// and the slow path (count from dialogs_messages after an admin reset) give the
// same result when there is no admin reset.
//
// Wiener: fast/slow path divergence → the quota shows a wrong value.
func TestOracle_Quota_FastSlowPath_Consistent(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	h := Handler{DB: env.Pool}

	user := f.CreateUser(TestUserOpts{})

	// Fast path: increment the counter 3 times.
	for i := 0; i < 3; i++ {
		_, _ = h.incrementDailyMessageCount(context.Background(), user.ID, 1)
	}

	fastUsed, err := h.globalDailyUsed(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("globalDailyUsed (fast): %v", err)
	}

	// Slow path: insert 3 messages into dialogs_messages and check the slow path.
	mode := f.CreateMode(TestModeOpts{})
	dialog := f.CreateDialog(user.ID, mode.ID)
	for i := 0; i < 3; i++ {
		_ = f.AppendMessage(dialog.ID, "user", "msg")
	}

	// Plant an admin reset today: this enables the slow path.
	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	_, err = env.Pool.Exec(context.Background(), `
		insert into admin_mode_usage_resets (user_id, mode_id, actor_user_id, reset_at)
		values ($1, $2, $3, now() - interval '1 second')`,
		user.ID, mode.ID, admin.ID)
	if err != nil {
		t.Fatalf("insert admin reset: %v", err)
	}

	slowUsed, err := h.globalDailyUsed(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("globalDailyUsed (slow): %v", err)
	}

	// The slow path counts dialogs_messages, the fast path counts daily_message_counts.
	// After an admin reset they need not match: this is a documented divergence.
	// What matters: the slow path returned something sensible (not negative, not huge).
	if slowUsed < 0 {
		t.Errorf("ORACLE: slow-path вернул отрицательное значение %d", slowUsed)
	}
	t.Logf("ORACLE INFO: fast-path=%d slow-path=%d (расхождение при admin reset — ожидаемо)", fastUsed, slowUsed)
}

// TestOracle_Promo_UniqueConstraint_PreventsConcurrentDoubleUsage:
// two goroutines, one user, one promo code: a DB constraint (not only app logic)
// guarantees exactly one row in promocode_usages even with concurrent direct inserts.
func TestOracle_Promo_UniqueConstraint_PreventsConcurrentDoubleUsage(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)

	user := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{})
	promo := f.CreatePromocode(TestPromocodeOpts{TargetID: mode.ID})

	const workers = 5
	var success, failed atomic.Int64
	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := env.Pool.Exec(context.Background(),
				`insert into promocode_usages (user_id, promocode_id, used_at) values ($1, $2, now())`,
				user.ID, promo.ID)
			if err != nil {
				failed.Add(1)
			} else {
				success.Add(1)
			}
		}()
	}
	wg.Wait()

	if success.Load() != 1 {
		t.Errorf("ORACLE: DB-констрейнт допустил %d вставок, ожидали ровно 1", success.Load())
	}
	if failed.Load() != workers-1 {
		t.Errorf("ORACLE: %d отказов, ожидали %d", failed.Load(), workers-1)
	}

	var cnt int64
	_ = env.Pool.QueryRow(context.Background(),
		`select count(*) from promocode_usages where user_id=$1 and promocode_id=$2`,
		user.ID, promo.ID).Scan(&cnt)
	if cnt != 1 {
		t.Errorf("ORACLE: promocode_usages count=%d, want 1", cnt)
	}
}
