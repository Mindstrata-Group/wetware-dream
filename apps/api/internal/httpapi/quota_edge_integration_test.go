//go:build integration

package httpapi

import (
	"context"
	"sync"
	"testing"

	"mindstrata-stage1/api/internal/testsupport"
)

// TestQuotaEdge_FastToSlowPathSwitch: the counter switches to the slow path after
// the first admin reset of the day.
//
// Scenario:
//  1. A few messages → fast path, used=N
//  2. Admin reset → flag has_reset = true
//  3. globalDailyUsed must switch to the slow path and count messages
//     AFTER reset_at, ignoring the counter before the reset.
func TestQuotaEdge_FastToSlowPathSwitch(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	h := Handler{DB: env.Pool}

	user := f.CreateUser(TestUserOpts{})
	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	mode := f.CreateMode(TestModeOpts{})
	dialog := f.CreateDialog(user.ID, mode.ID)

	// Inc through the standard handler: counter=3, no reset → the fast path returns 3
	for i := 0; i < 3; i++ {
		_, _ = h.incrementDailyMessageCount(context.Background(), user.ID, 1)
		f.AppendMessage(dialog.ID, "user", "msg-before-reset")
	}
	usedFast, err := h.globalDailyUsed(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("fast-path query: %v", err)
	}
	if usedFast != 3 {
		t.Fatalf("fast-path: got %d want 3", usedFast)
	}

	// Admin reset
	_, err = env.Pool.Exec(context.Background(), `
		insert into admin_mode_usage_resets (user_id, mode_id, actor_user_id, reset_at)
		values ($1, $2, $3, now())`, user.ID, mode.ID, admin.ID)
	if err != nil {
		t.Fatalf("insert reset: %v", err)
	}

	// No new messages after the reset → the slow path returns 0
	usedSlow, err := h.globalDailyUsed(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("slow-path query: %v", err)
	}
	if usedSlow != 0 {
		t.Fatalf("slow-path after reset, no new msgs: got %d want 0", usedSlow)
	}

	// Add 2 messages after the reset → the slow path returns 2 (the old ones are ignored)
	for i := 0; i < 2; i++ {
		f.AppendMessage(dialog.ID, "user", "msg-after-reset")
	}
	usedAfter, err := h.globalDailyUsed(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("slow-path post-msgs: %v", err)
	}
	if usedAfter != 2 {
		t.Fatalf("slow-path with 2 post-reset msgs: got %d want 2", usedAfter)
	}
}

// TestQuotaEdge_TryIncrement_BlockedAtBoundary: limit=3, increment 3 times → ok,
// the 4th increment must return false.
func TestQuotaEdge_TryIncrement_BlockedAtBoundary(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	h := Handler{DB: env.Pool}

	user := f.CreateUser(TestUserOpts{})

	for i := 1; i <= 3; i++ {
		newCount, ok, err := h.tryIncrementDailyMessageCount(context.Background(), user.ID, 3, 1)
		if err != nil {
			t.Fatalf("try inc #%d: %v", i, err)
		}
		if !ok {
			t.Fatalf("try inc #%d: ok=false at count=%d limit=3", i, newCount)
		}
		if newCount != int64(i) {
			t.Fatalf("try inc #%d: newCount=%d want %d", i, newCount, i)
		}
	}

	// 4th inc → ok=false
	_, ok, err := h.tryIncrementDailyMessageCount(context.Background(), user.ID, 3, 1)
	if err != nil {
		t.Fatalf("try inc #4: %v", err)
	}
	if ok {
		t.Fatalf("try inc #4: should be blocked at limit=3, got ok=true")
	}
}

// TestQuotaEdge_TryIncrement_LimitZero: limit=0 → always false.
func TestQuotaEdge_TryIncrement_LimitZero(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	h := Handler{DB: env.Pool}

	user := f.CreateUser(TestUserOpts{})

	_, ok, err := h.tryIncrementDailyMessageCount(context.Background(), user.ID, 0, 1)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if ok {
		t.Fatalf("limit=0 should always be blocked")
	}
}

// TestQuotaEdge_TryIncrement_DeltaGreaterThanLimit: delta > limit → false.
func TestQuotaEdge_TryIncrement_DeltaGreaterThanLimit(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	h := Handler{DB: env.Pool}

	user := f.CreateUser(TestUserOpts{})

	_, ok, err := h.tryIncrementDailyMessageCount(context.Background(), user.ID, 5, 10)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if ok {
		t.Fatalf("delta=10 > limit=5 should be blocked")
	}
}

// TestQuotaEdge_TryIncrement_ParallelStress: 100 goroutines on limit=20 → exactly 20 ok.
func TestQuotaEdge_TryIncrement_ParallelStress(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	h := Handler{DB: env.Pool}

	user := f.CreateUser(TestUserOpts{})

	const N = 100
	const limit = 20

	var ok, denied int64
	var mu sync.Mutex
	var wg sync.WaitGroup
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, allowed, err := h.tryIncrementDailyMessageCount(context.Background(), user.ID, int64(limit), 1)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				t.Errorf("parallel inc: %v", err)
				return
			}
			if allowed {
				ok++
			} else {
				denied++
			}
		}()
	}
	wg.Wait()

	if ok != limit {
		t.Fatalf("parallel: got %d ok, want exactly %d (denied=%d)", ok, limit, denied)
	}

	// DB invariant: the counter is exactly limit
	var count int64
	_ = env.Pool.QueryRow(context.Background(),
		`select count from daily_message_counts where user_id = $1 and date = current_date`,
		user.ID).Scan(&count)
	if count != int64(limit) {
		t.Fatalf("DB counter: got %d want %d", count, limit)
	}
}

// TestQuotaEdge_MultiMode_SharedCounter: the quota is global (a shared counter).
// Two modes with limit 10 each → the user does not get 20 messages but only 10,
// since globalDailyUsed is shared.
//
// This is an explicit design choice (not a bug): the counter is per user per
// day, not per mode. perModeLimit sums the limits over modes, but a shared used
// is subtracted.
func TestQuotaEdge_MultiMode_SharedCounter(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	h := Handler{DB: env.Pool}

	user := f.CreateUser(TestUserOpts{})
	mode1 := f.CreateMode(TestModeOpts{})
	mode2 := f.CreateMode(TestModeOpts{})
	f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: mode1.ID, DailyMessageLimit: 10})
	f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: mode2.ID, DailyMessageLimit: 10})

	q1, err := h.dailyQuota(context.Background(), user.ID, mode1.ID)
	if err != nil {
		t.Fatalf("q1: %v", err)
	}
	q2, err := h.dailyQuota(context.Background(), user.ID, mode2.ID)
	if err != nil {
		t.Fatalf("q2: %v", err)
	}

	// Each mode gives its own limit = 10, Used=0
	if q1.Limit == nil || *q1.Limit != 10 || q1.Used != 0 {
		t.Fatalf("mode1: got %+v want limit=10 used=0", q1)
	}
	if q2.Limit == nil || *q2.Limit != 10 || q2.Used != 0 {
		t.Fatalf("mode2: got %+v want limit=10 used=0", q2)
	}

	// Increment the shared counter 5 times
	for i := 0; i < 5; i++ {
		_, _ = h.incrementDailyMessageCount(context.Background(), user.ID, 1)
	}

	// BOTH modes now show Used=5
	q1, _ = h.dailyQuota(context.Background(), user.ID, mode1.ID)
	q2, _ = h.dailyQuota(context.Background(), user.ID, mode2.ID)
	if q1.Used != 5 || q2.Used != 5 {
		t.Fatalf("after 5 inc: q1.Used=%d q2.Used=%d, want both 5", q1.Used, q2.Used)
	}
	if *q1.Remaining != 5 || *q2.Remaining != 5 {
		t.Fatalf("Remaining: got q1=%d q2=%d, want both 5", *q1.Remaining, *q2.Remaining)
	}
}
