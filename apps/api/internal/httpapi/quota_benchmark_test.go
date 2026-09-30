//go:build integration

package httpapi

import (
	"context"
	"testing"

	"mindstrata-stage1/api/internal/testsupport"
)

// Benchmarks track perf regressions in hot quota/history paths.
//
// Run:
//   TEST_DATABASE_URL=... go test -tags=integration -bench=Benchmark -benchmem \
//     -run=^$ ./internal/httpapi/...
//
// CI guidance: snapshot baseline, fail if >50% slower (manual until benchstat
// integrated). Numbers are env-dependent — only relative deltas are meaningful.

func BenchmarkQuota_PerModeLimit(b *testing.B) {
	env := testsupport.NewEnv(b)
	f := NewFactory(b, env.Pool)
	h := Handler{DB: env.Pool}

	user := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{})
	f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: mode.ID, DailyMessageLimit: 50})
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, err := h.perModeLimit(ctx, user.ID, mode.ID)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkQuota_GlobalDailyUsed_FastPath(b *testing.B) {
	env := testsupport.NewEnv(b)
	f := NewFactory(b, env.Pool)
	h := Handler{DB: env.Pool}

	user := f.CreateUser(TestUserOpts{})
	// No admin reset → fast path.
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		_, _ = h.incrementDailyMessageCount(ctx, user.ID, 1)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := h.globalDailyUsed(ctx, user.ID)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkQuota_DailyQuota_Combined(b *testing.B) {
	env := testsupport.NewEnv(b)
	f := NewFactory(b, env.Pool)
	h := Handler{DB: env.Pool}

	user := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{})
	f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: mode.ID, DailyMessageLimit: 50})
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := h.dailyQuota(ctx, user.ID, mode.ID)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkQuota_TryIncrement(b *testing.B) {
	env := testsupport.NewEnv(b)
	f := NewFactory(b, env.Pool)
	h := Handler{DB: env.Pool}

	user := f.CreateUser(TestUserOpts{})
	ctx := context.Background()

	// Pre-create the row so we hit the ON CONFLICT path (typical case).
	_, _ = h.incrementDailyMessageCount(ctx, user.ID, 1)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Very high limit, so the increment always passes.
		_, _, err := h.tryIncrementDailyMessageCount(ctx, user.ID, 1_000_000, 1)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkChat_GetDialogMessages_Page10(b *testing.B) {
	env := testsupport.NewEnv(b)
	f := NewFactory(b, env.Pool)

	user := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{})
	dialog := f.CreateDialog(user.ID, mode.ID)
	// Seed 100 messages so pagination is meaningful.
	for i := 0; i < 100; i++ {
		_ = f.AppendMessage(dialog.ID, "user", "msg-content")
	}
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := getDialogMessages(ctx, env.Pool, dialog.ID, 10, 0)
		if err != nil {
			b.Fatal(err)
		}
	}
}
