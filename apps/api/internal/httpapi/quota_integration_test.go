//go:build integration

package httpapi

import (
	"context"
	"sync"
	"testing"

	"mindstrata-stage1/api/internal/testsupport"
)

// TestQuota_PerModeLimit_NoAccess: user without a grant → hasAccess=false.
func TestQuota_PerModeLimit_NoAccess(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	h := Handler{DB: env.Pool}

	user := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{})

	limit, hasAccess, err := h.perModeLimit(context.Background(), user.ID, mode.ID)
	if err != nil {
		t.Fatalf("perModeLimit: %v", err)
	}
	if hasAccess {
		t.Fatalf("expected no access, got hasAccess=true (limit=%d)", limit)
	}
}

// TestQuota_PerModeLimit_SingleSource: one grant of 50 → limit=50.
func TestQuota_PerModeLimit_SingleSource(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	h := Handler{DB: env.Pool}

	user := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{})
	f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: mode.ID, DailyMessageLimit: 50})

	limit, hasAccess, err := h.perModeLimit(context.Background(), user.ID, mode.ID)
	if err != nil {
		t.Fatalf("perModeLimit: %v", err)
	}
	if !hasAccess {
		t.Fatalf("expected access, got hasAccess=false")
	}
	if limit != 50 {
		t.Fatalf("limit: got %d want 50", limit)
	}
}

// TestQuota_PerModeLimit_TwoDifferentSources: two different sources → summed.
func TestQuota_PerModeLimit_TwoDifferentSources(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	h := Handler{DB: env.Pool}

	user := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{})
	src1 := int64(101)
	src2 := int64(202)
	f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: mode.ID, DailyMessageLimit: 50, AccessType: "promocode", SourceID: &src1})
	f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: mode.ID, DailyMessageLimit: 30, AccessType: "subscription", SourceID: &src2})

	limit, hasAccess, err := h.perModeLimit(context.Background(), user.ID, mode.ID)
	if err != nil {
		t.Fatalf("perModeLimit: %v", err)
	}
	if !hasAccess || limit != 80 {
		t.Fatalf("got hasAccess=%v limit=%d, want hasAccess=true limit=80", hasAccess, limit)
	}
}

// TestQuota_PerModeLimit_NullSourceRowsCountSeparately:
// DB UNIQUE (user_id, mode_id, access_type, source_id): a NULL source_id allows
// several rows (PG treats NULL ≠ NULL for UNIQUE). The perModeLimit query
// deduplicates by id for NULL source_id, so two manual rows without a source
// add up.
func TestQuota_PerModeLimit_NullSourceRowsCountSeparately(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	h := Handler{DB: env.Pool}

	user := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{})
	// Two manual rows without source_id: both are allowed and both must be summed
	f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: mode.ID, DailyMessageLimit: 50, AccessType: "manual"})
	f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: mode.ID, DailyMessageLimit: 30, AccessType: "manual"})

	limit, hasAccess, err := h.perModeLimit(context.Background(), user.ID, mode.ID)
	if err != nil {
		t.Fatalf("perModeLimit: %v", err)
	}
	if !hasAccess || limit != 80 {
		t.Fatalf("got limit=%d, want 80 (two NULL-source rows sum); hasAccess=%v", limit, hasAccess)
	}
}

// TestQuota_PerModeLimit_DuplicateSourceConstraint:
// a DB constraint protects against double counting of promocode/subscription
// (same source_id + access_type → INSERT rejected).
func TestQuota_PerModeLimit_DuplicateSourceConstraint(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)

	user := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{})
	src := int64(777)
	f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: mode.ID, DailyMessageLimit: 50, AccessType: "promocode", SourceID: &src})

	// A second INSERT with the same keys must return an error: checked directly,
	// without the factory (the factory would call t.Fatalf).
	_, err := env.Pool.Exec(context.Background(), `
		insert into user_mode_access
		  (user_id, mode_id, source_id, active_from, active_to, daily_message_limit, priority, access_type)
		values ($1, $2, $3, now() - interval '1 hour', now() + interval '365 days', 50, 0, 'promocode')`,
		user.ID, mode.ID, src)
	if err == nil {
		t.Fatalf("expected unique constraint violation on duplicate (user, mode, access_type, source_id)")
	}
}

// TestQuota_PerModeLimit_ExpiredAccess: active_to in the past → no access.
func TestQuota_PerModeLimit_ExpiredAccess(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	h := Handler{DB: env.Pool}

	user := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{})
	past := timeAgo(48)
	pastPast := timeAgo(72)
	f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: mode.ID, DailyMessageLimit: 50, ActiveFrom: &pastPast, ActiveTo: &past})

	_, hasAccess, err := h.perModeLimit(context.Background(), user.ID, mode.ID)
	if err != nil {
		t.Fatalf("perModeLimit: %v", err)
	}
	if hasAccess {
		t.Fatalf("expired access leaked: hasAccess=true")
	}
}

// TestQuota_IncrementCounter_Sequential: 5 increments in a row → count=5.
func TestQuota_IncrementCounter_Sequential(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	h := Handler{DB: env.Pool}

	user := f.CreateUser(TestUserOpts{})

	for i := 1; i <= 5; i++ {
		got, err := h.incrementDailyMessageCount(context.Background(), user.ID, 1)
		if err != nil {
			t.Fatalf("increment #%d: %v", i, err)
		}
		if got != int64(i) {
			t.Fatalf("increment #%d: got count=%d want %d", i, got, i)
		}
	}
}

// TestQuota_IncrementCounter_Atomic: 50 parallel increments → count=50.
// The most important invariant of the quota system: without atomicity a hyper-quota race is possible.
func TestQuota_IncrementCounter_Atomic(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	h := Handler{DB: env.Pool}

	user := f.CreateUser(TestUserOpts{})

	const N = 50
	var wg sync.WaitGroup
	errs := make(chan error, N)
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := h.incrementDailyMessageCount(context.Background(), user.ID, 1); err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent increment failed: %v", err)
		}
	}

	var final int64
	err := env.Pool.QueryRow(context.Background(),
		"select count from daily_message_counts where user_id = $1 and date = current_date",
		user.ID).Scan(&final)
	if err != nil {
		t.Fatalf("query final: %v", err)
	}
	if final != N {
		t.Fatalf("atomicity broken: got count=%d want %d", final, N)
	}
}

// TestQuota_GlobalUsed_FastPath: no admin reset → reads from daily_message_counts.
func TestQuota_GlobalUsed_FastPath(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	h := Handler{DB: env.Pool}

	user := f.CreateUser(TestUserOpts{})

	// No increments: 0
	used, err := h.globalDailyUsed(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("globalDailyUsed: %v", err)
	}
	if used != 0 {
		t.Fatalf("got used=%d, want 0 for new user", used)
	}

	// After 3 increments: 3
	for i := 0; i < 3; i++ {
		_, _ = h.incrementDailyMessageCount(context.Background(), user.ID, 1)
	}
	used, err = h.globalDailyUsed(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("globalDailyUsed: %v", err)
	}
	if used != 3 {
		t.Fatalf("got used=%d, want 3", used)
	}
}

// TestQuota_GlobalUsed_SlowPath_AfterAdminReset:
// after an admin reset the counter is computed from dialogs_messages starting at reset_at.
func TestQuota_GlobalUsed_SlowPath_AfterAdminReset(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	h := Handler{DB: env.Pool}

	user := f.CreateUser(TestUserOpts{})
	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	mode := f.CreateMode(TestModeOpts{})
	dialog := f.CreateDialog(user.ID, mode.ID)

	// Simulate: 3 messages before the reset, then the reset, then 2 messages after.
	_ = f.AppendMessage(dialog.ID, "user", "before reset 1")
	_ = f.AppendMessage(dialog.ID, "user", "before reset 2")
	_ = f.AppendMessage(dialog.ID, "user", "before reset 3")

	// Record an admin reset (mode_id=mode.ID; reset_at = now)
	_, err := env.Pool.Exec(context.Background(), `
		insert into admin_mode_usage_resets (user_id, mode_id, actor_user_id, reset_at)
		values ($1, $2, $3, now())`,
		user.ID, mode.ID, admin.ID)
	if err != nil {
		t.Fatalf("insert admin reset: %v", err)
	}

	_ = f.AppendMessage(dialog.ID, "user", "after reset 1")
	_ = f.AppendMessage(dialog.ID, "user", "after reset 2")

	used, err := h.globalDailyUsed(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("globalDailyUsed: %v", err)
	}
	// Slow path: counts only user/summary messages AFTER reset_at.
	if used != 2 {
		t.Fatalf("got used=%d, want 2 (only post-reset messages should count)", used)
	}
}

// TestQuota_DailyQuota_RemainingNeverNegative:
// if Used > Limit (e.g. after a manual INSERT), Remaining = 0, not negative.
func TestQuota_DailyQuota_RemainingNeverNegative(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	h := Handler{DB: env.Pool}

	user := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{})
	f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: mode.ID, DailyMessageLimit: 3})

	// Increment the counter to 5 times the limit.
	for i := 0; i < 15; i++ {
		_, _ = h.incrementDailyMessageCount(context.Background(), user.ID, 1)
	}

	q, err := h.dailyQuota(context.Background(), user.ID, mode.ID)
	if err != nil {
		t.Fatalf("dailyQuota: %v", err)
	}
	if q == nil {
		t.Fatalf("dailyQuota returned nil despite access")
	}
	if q.Remaining == nil {
		t.Fatalf("Remaining is nil")
	}
	if *q.Remaining != 0 {
		t.Fatalf("Remaining=%d, want 0 (never negative)", *q.Remaining)
	}
	if q.Used != 15 {
		t.Fatalf("Used=%d, want 15", q.Used)
	}
}

// TestQuota_DailyQuota_HappyPath: Limit=10, Used=4 → Remaining=6.
func TestQuota_DailyQuota_HappyPath(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	h := Handler{DB: env.Pool}

	user := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{})
	f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: mode.ID, DailyMessageLimit: 10})

	for i := 0; i < 4; i++ {
		_, _ = h.incrementDailyMessageCount(context.Background(), user.ID, 1)
	}

	q, err := h.dailyQuota(context.Background(), user.ID, mode.ID)
	if err != nil {
		t.Fatalf("dailyQuota: %v", err)
	}
	if q == nil || q.Limit == nil || q.Remaining == nil {
		t.Fatalf("dailyQuota incomplete: %+v", q)
	}
	if *q.Limit != 10 {
		t.Fatalf("Limit=%d want 10", *q.Limit)
	}
	if q.Used != 4 {
		t.Fatalf("Used=%d want 4", q.Used)
	}
	if *q.Remaining != 6 {
		t.Fatalf("Remaining=%d want 6", *q.Remaining)
	}
}

// TestQuota_DailyQuota_NoAccess: user without a grant → nil quota.
func TestQuota_DailyQuota_NoAccess(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	h := Handler{DB: env.Pool}

	user := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{})

	q, err := h.dailyQuota(context.Background(), user.ID, mode.ID)
	if err != nil {
		t.Fatalf("dailyQuota: %v", err)
	}
	if q != nil {
		t.Fatalf("expected nil quota without access, got %+v", q)
	}
}
