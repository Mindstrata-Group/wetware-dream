//go:build integration

package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"

	"mindstrata-stage1/api/internal/testsupport"
)

func TestLoadBaseline_AuthSessionsParallelMe(t *testing.T) {
	// Load baseline tests do not run with t.Parallel(): they deliberately create DB
	// contention and on a snapshot DB could slow down neighbouring stress tests.

	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)

	const users = 24
	const rounds = 3

	type authSubject struct {
		userID int64
		token  string
	}
	subjects := make([]authSubject, 0, users)
	for i := 0; i < users; i++ {
		user := f.CreateUser(TestUserOpts{EmailVerified: true})
		subjects = append(subjects, authSubject{
			userID: user.ID,
			token:  f.CreateSession(user.ID),
		})
	}

	var ok, wrongUser, failed atomic.Int64
	var wg sync.WaitGroup
	for _, subject := range subjects {
		subject := subject
		wg.Add(1)
		go func() {
			defer wg.Done()

			ts := NewTestServer(t, env.Pool)
			ts.LoginAs(subject.token)
			for i := 0; i < rounds; i++ {
				status, body := httpJSON(t, ts, http.MethodGet, "/api/auth/me", nil)
				if status != http.StatusOK {
					failed.Add(1)
					t.Logf("auth baseline /me status=%d body=%v", status, body)
					continue
				}
				user, _ := body["user"].(map[string]any)
				gotID, _ := user["id"].(float64)
				if int64(gotID) != subject.userID {
					wrongUser.Add(1)
					t.Logf("auth baseline wrong user: got=%v want=%d body=%v", gotID, subject.userID, body)
					continue
				}
				ok.Add(1)
			}
		}()
	}
	wg.Wait()

	if failed.Load() != 0 || wrongUser.Load() != 0 {
		t.Fatalf("auth baseline: ok=%d failed=%d wrong_user=%d", ok.Load(), failed.Load(), wrongUser.Load())
	}
	if got, want := ok.Load(), int64(users*rounds); got != want {
		t.Fatalf("auth baseline: ok=%d want=%d", got, want)
	}
}

func TestLoadBaseline_PromoApplyConcurrentMaxUses(t *testing.T) {
	// Load baseline tests do not run with t.Parallel(): they deliberately create DB
	// contention and on a snapshot DB could slow down neighbouring stress tests.

	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)

	const maxUses = 4
	const contenders = 8

	mode := f.CreateMode(TestModeOpts{})
	promo := f.CreatePromocode(TestPromocodeOpts{TargetID: mode.ID, MaxUses: maxUses})

	var success, limitReached, failed atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < contenders; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			ts := NewTestServer(t, env.Pool)
			user := f.CreateUser(TestUserOpts{})
			ts.LoginAs(f.CreateSession(user.ID))

			status, body := httpJSON(t, ts, http.MethodPost, "/api/access/promocode/apply", map[string]any{
				"code": promo.Code,
			})
			switch {
			case status == http.StatusOK:
				success.Add(1)
			case status == http.StatusBadRequest &&
				(body["code"] == "limit_reached" || body["errorCode"] == "limit_reached"):
				limitReached.Add(1)
			default:
				failed.Add(1)
				t.Logf("promo baseline unexpected status=%d body=%v", status, body)
			}
		}()
	}
	wg.Wait()

	if failed.Load() != 0 {
		t.Fatalf("promo baseline: unexpected failures=%d success=%d limit_reached=%d",
			failed.Load(), success.Load(), limitReached.Load())
	}
	if got := success.Load(); got != maxUses {
		t.Fatalf("promo baseline: success=%d want=%d limit_reached=%d",
			got, maxUses, limitReached.Load())
	}
	if got, want := limitReached.Load(), int64(contenders-maxUses); got != want {
		t.Fatalf("promo baseline: limit_reached=%d want=%d", got, want)
	}

	var usedCount, usageRows, accessRows int64
	err := env.Pool.QueryRow(context.Background(), `
		select used_count from promocodes where id = $1`, promo.ID).Scan(&usedCount)
	if err != nil {
		t.Fatalf("promo baseline used_count: %v", err)
	}
	err = env.Pool.QueryRow(context.Background(), `
		select count(*) from promocode_usages where promocode_id = $1`, promo.ID).Scan(&usageRows)
	if err != nil {
		t.Fatalf("promo baseline usages: %v", err)
	}
	err = env.Pool.QueryRow(context.Background(), `
		select count(*) from user_mode_access where access_type = 'promocode' and source_id = $1`, promo.ID).Scan(&accessRows)
	if err != nil {
		t.Fatalf("promo baseline access rows: %v", err)
	}
	if usedCount != maxUses || usageRows != maxUses || accessRows != maxUses {
		t.Fatalf("promo baseline counters diverged: used=%d usages=%d access=%d want=%d",
			usedCount, usageRows, accessRows, maxUses)
	}
}

func TestLoadBaseline_ChatSendConcurrentQuotaBoundary(t *testing.T) {
	// Load baseline tests do not run with t.Parallel(): they deliberately create DB
	// contention and on a snapshot DB could slow down neighbouring stress tests.

	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)

	const limit = 12
	const contenders = 18

	user := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{})
	f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: mode.ID, DailyMessageLimit: limit})
	dialog := f.CreateDialog(user.ID, mode.ID)

	var success, quotaDenied, failed atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < contenders; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()

			ts := NewTestServer(t, env.Pool)
			ts.LoginAs(f.CreateSession(user.ID))
			status, body := httpJSON(t, ts, http.MethodPost, "/api/chat/send", map[string]any{
				"dialogId":     dialog.ID,
				"text":         fmt.Sprintf("baseline message %02d", i),
				"responseMode": "test",
			})
			switch {
			case status == http.StatusOK:
				success.Add(1)
			case status == http.StatusTooManyRequests && body["code"] == "daily_quota_exhausted":
				quotaDenied.Add(1)
			default:
				failed.Add(1)
				t.Logf("chat baseline unexpected status=%d body=%v", status, body)
			}
		}()
	}
	wg.Wait()

	if failed.Load() != 0 {
		t.Fatalf("chat baseline: unexpected failures=%d success=%d quota_denied=%d",
			failed.Load(), success.Load(), quotaDenied.Load())
	}
	if got := success.Load(); got != limit {
		t.Fatalf("chat baseline: success=%d want=%d quota_denied=%d", got, limit, quotaDenied.Load())
	}
	if got, want := quotaDenied.Load(), int64(contenders-limit); got != want {
		t.Fatalf("chat baseline: quota_denied=%d want=%d", got, want)
	}

	var dailyCount, userMessages, assistantMessages int64
	err := env.Pool.QueryRow(context.Background(), `
		select count from daily_message_counts where user_id = $1 and date = current_date`, user.ID).Scan(&dailyCount)
	if err != nil {
		t.Fatalf("chat baseline daily count: %v", err)
	}
	err = env.Pool.QueryRow(context.Background(), `
		select count(*) from dialogs_messages where dialog_id = $1 and role = 'user'`, dialog.ID).Scan(&userMessages)
	if err != nil {
		t.Fatalf("chat baseline user messages: %v", err)
	}
	err = env.Pool.QueryRow(context.Background(), `
		select count(*) from dialogs_messages where dialog_id = $1 and role = 'assistant'`, dialog.ID).Scan(&assistantMessages)
	if err != nil {
		t.Fatalf("chat baseline assistant messages: %v", err)
	}
	if dailyCount != limit || userMessages != limit || assistantMessages != limit {
		t.Fatalf("chat baseline counters diverged: daily=%d user_messages=%d assistant_messages=%d want=%d",
			dailyCount, userMessages, assistantMessages, limit)
	}
}
