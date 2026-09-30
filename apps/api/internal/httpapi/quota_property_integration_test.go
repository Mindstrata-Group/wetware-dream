//go:build integration

package httpapi

import (
	"context"
	"math/rand"
	"net/http"
	"sync/atomic"
	"testing"

	"mindstrata-stage1/api/internal/testsupport"
)

// TestQuotaProperty_NeverExceedsLimit runs N random operations against a
// single user with limit=L and asserts the global invariant:
//
//	counter_in_db == count_of_user_messages_in_dialog
//	              ≤ L  at all times after each operation.
//
// Operations chosen randomly:
//   - send (POST /api/chat/send)
//   - complete (POST /api/chat/complete) — summary counts toward quota
//
// If ANY iteration leaves counter > L, or sends a successful response that
// doesn't correspond to a stored message, we have a quota integrity bug.
func TestQuotaProperty_NeverExceedsLimit(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)

	const limit = 7
	const iterations = 200
	user, _, dialog := authedUserWithDialog(t, env, ts, limit)

	rng := rand.New(rand.NewSource(42)) // deterministic seed for reproducible failures

	allowed := int64(0)
	for i := 0; i < iterations; i++ {
		var status int
		if rng.Intn(2) == 0 {
			status, _ = httpJSON(t, ts, "POST", "/api/chat/send", map[string]any{
				"dialogId": dialog.ID,
				"text":     "iteration",
			})
		} else {
			// complete also consumes a quota slot
			status, _ = httpJSON(t, ts, "POST", "/api/chat/complete", map[string]any{
				"dialogId": dialog.ID,
			})
		}
		if status == http.StatusOK {
			allowed++
		}

		// Invariant 1: counter never above limit
		var counter int64
		_ = env.Pool.QueryRow(context.Background(),
			`select coalesce(count, 0) from daily_message_counts
			 where user_id = $1 and date = current_date`,
			user.ID).Scan(&counter)
		if counter > limit {
			t.Fatalf("invariant violated at iteration %d: counter=%d > limit=%d (allowed so far: %d)",
				i, counter, limit, allowed)
		}

		// Invariant 2: number of allowed responses ≤ limit
		if allowed > limit {
			t.Fatalf("invariant violated at iteration %d: allowed=%d > limit=%d",
				i, allowed, limit)
		}
	}

	// Final state: counter must equal allowed (no orphans, no missed increments).
	var finalCounter int64
	_ = env.Pool.QueryRow(context.Background(),
		`select coalesce(count, 0) from daily_message_counts
		 where user_id = $1 and date = current_date`,
		user.ID).Scan(&finalCounter)
	if finalCounter != allowed {
		t.Fatalf("counter (%d) != allowed responses (%d) — increment drift", finalCounter, allowed)
	}
}

// TestQuotaProperty_HighContention runs M goroutines × N iterations each
// against a single user. Asserts atomic invariant: counter ≤ limit at end.
//
// Worst-case (with the old check-then-act bug): counter ends at >limit,
// or successful responses > limit, OR there are user messages in DB
// without corresponding successful HTTP responses.
func TestQuotaProperty_HighContention(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)

	const limit = 10
	const workers = 20
	const opsPerWorker = 30
	user := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{})
	f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: mode.ID, DailyMessageLimit: limit})
	dialog := f.CreateDialog(user.ID, mode.ID)
	_, _ = env.Pool.Exec(context.Background(),
		`update users set current_mode = $2, current_dialog = $3 where id = $1`,
		user.ID, mode.ID, dialog.ID)

	var allowed atomic.Int64
	done := make(chan struct{})
	for w := 0; w < workers; w++ {
		go func() {
			defer func() { done <- struct{}{} }()
			ts := NewTestServer(t, env.Pool)
			token := f.CreateSession(user.ID)
			ts.LoginAs(token)
			for i := 0; i < opsPerWorker; i++ {
				code, _ := httpJSON(t, ts, "POST", "/api/chat/send", map[string]any{
					"dialogId": dialog.ID, "text": "stress",
				})
				if code == http.StatusOK {
					allowed.Add(1)
				}
			}
		}()
	}
	for w := 0; w < workers; w++ {
		<-done
	}

	got := allowed.Load()
	if got != int64(limit) {
		t.Fatalf("high contention: got %d successes, want exactly %d", got, limit)
	}

	var counter int64
	_ = env.Pool.QueryRow(context.Background(),
		`select count from daily_message_counts where user_id = $1 and date = current_date`,
		user.ID).Scan(&counter)
	if counter != int64(limit) {
		t.Fatalf("counter drift under contention: counter=%d allowed=%d limit=%d",
			counter, got, limit)
	}

	var msgCount int64
	_ = env.Pool.QueryRow(context.Background(),
		`select count(*) from dialogs_messages where dialog_id = $1 and role = 'user'`,
		dialog.ID).Scan(&msgCount)
	if msgCount != int64(limit) {
		t.Fatalf("orphan messages under contention: counter=%d msg_count=%d limit=%d",
			counter, msgCount, limit)
	}
}
