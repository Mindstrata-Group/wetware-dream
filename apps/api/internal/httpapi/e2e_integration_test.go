//go:build integration

package httpapi

import (
	"context"
	"net/http"
	"testing"

	"mindstrata-stage1/api/internal/testsupport"
)

// TestE2E_NewUserFullJourney: the full path of a new user.
//  1. POST /api/auth/register   → 200, session cookie
//  2. GET  /api/auth/me         → 200
//  3. POST /api/access/promocode/apply → grants access to mode
//  4. POST /api/chat/select-mode → 200, dialog created
//  5. POST /api/chat/send       → 200, message stored, quota decremented
//  6. POST /api/chat/complete   → 200, summary stored
//  7. GET  /api/chat/history    → 3 messages in chronological order
//  8. POST /api/auth/logout     → 200, subsequent /me fails
func TestE2E_NewUserFullJourney(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	// Setup: one mode + a promo code
	mode := f.CreateMode(TestModeOpts{Name: "Coaching"})
	promo := f.CreatePromocode(TestPromocodeOpts{TargetID: mode.ID, DurationDays: 30})

	// 1. Register
	email := uniqueEmail("e2e_journey")
	code, body := httpJSON(t, ts, "POST", "/api/auth/register", map[string]any{
		"email": email, "password": "testpass123",
	})
	if code != http.StatusOK {
		t.Fatalf("step 1 register: %d body=%v", code, body)
	}

	// 2. /me
	code, body = httpJSON(t, ts, "GET", "/api/auth/me", nil)
	if code != http.StatusOK {
		t.Fatalf("step 2 me: %d body=%v", code, body)
	}
	userMap, _ := body["user"].(map[string]any)
	userIDF, _ := userMap["id"].(float64)
	userID := int64(userIDF)
	if userID == 0 {
		t.Fatalf("step 2: no user.id in /me response")
	}

	// 3. Apply promo
	code, body = httpJSON(t, ts, "POST", "/api/access/promocode/apply", map[string]any{"code": promo.Code})
	if code != http.StatusOK {
		t.Fatalf("step 3 promo: %d body=%v", code, body)
	}

	// 4. Select mode
	code, body = httpJSON(t, ts, "POST", "/api/chat/select-mode", map[string]any{
		"modeId": mode.ID, "newDialog": true,
	})
	if code != http.StatusOK {
		t.Fatalf("step 4 select-mode: %d body=%v", code, body)
	}

	// Get the dialog id that just got created
	var dialogID int64
	_ = env.Pool.QueryRow(context.Background(),
		`select current_dialog from users where id = $1`, userID).Scan(&dialogID)
	if dialogID == 0 {
		t.Fatalf("step 4: no current_dialog set")
	}

	// 5. Send message
	code, body = httpJSON(t, ts, "POST", "/api/chat/send", map[string]any{
		"dialogId": dialogID, "text": "Hello coach",
	})
	if code != http.StatusOK {
		t.Fatalf("step 5 send: %d body=%v", code, body)
	}

	// 6. Complete (summary)
	code, body = httpJSON(t, ts, "POST", "/api/chat/complete", map[string]any{
		"dialogId": dialogID,
	})
	if code != http.StatusOK {
		t.Fatalf("step 6 complete: %d body=%v", code, body)
	}

	// 7. History: must be >= 3 (user, assistant, summary)
	code, body = httpJSON(t, ts, "GET", "/api/chat/history?dialogId="+itoa(dialogID), nil)
	if code != http.StatusOK {
		t.Fatalf("step 7 history: %d body=%v", code, body)
	}
	msgs, _ := body["messages"].([]any)
	if len(msgs) < 3 {
		t.Fatalf("step 7: expected >= 3 messages (user, assistant, summary), got %d", len(msgs))
	}

	// 8. Logout
	code, _ = httpJSON(t, ts, "POST", "/api/auth/logout", nil)
	if code != http.StatusOK {
		t.Fatalf("step 8 logout: %d", code)
	}
	// /me now gives 401 OR creates a new guest, depending on ensureGuestUser.
	// What matters: the current user is no longer returned.
	code, body = httpJSON(t, ts, "GET", "/api/auth/me", nil)
	if code == http.StatusOK {
		if me, _ := body["user"].(map[string]any); me != nil {
			meID, _ := me["id"].(float64)
			if int64(meID) == userID {
				t.Fatalf("step 8: user still authenticated after logout (id=%d)", userID)
			}
		}
	}

	// DB invariants
	var counter int64
	_ = env.Pool.QueryRow(context.Background(),
		`select count from daily_message_counts where user_id = $1 and date = current_date`,
		userID).Scan(&counter)
	if counter != 2 {
		t.Fatalf("daily counter: got %d want 2 (send + summary)", counter)
	}
}

// TestE2E_SessionInvalidationAfterPasswordReset:
//  1. A user + session is created (cookie set).
//  2. A direct password reset through the DB (imitating the full reset flow).
//  3. The old session is no longer valid → /me gives 401.
//
// This is an invariant: a password change must invalidate all sessions.
func TestE2E_SessionInvalidationAfterPasswordReset(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	user := f.CreateUser(TestUserOpts{})
	token := f.CreateSession(user.ID)
	ts.LoginAs(token)

	// Before: ok
	code, _ := httpJSON(t, ts, "GET", "/api/auth/me", nil)
	if code != http.StatusOK {
		t.Fatalf("pre-reset /me: %d", code)
	}

	// Imitation: the reset_password handler does 2 things:
	//   - update users set password_hash = ...
	//   - update auth_sessions set revoked_at = now() where user_id = ...
	_, err := env.Pool.Exec(context.Background(),
		`update auth_sessions set revoked_at = now() where user_id = $1 and revoked_at is null`,
		user.ID)
	if err != nil {
		t.Fatalf("revoke sessions: %v", err)
	}

	// After: 401
	code, _ = httpJSON(t, ts, "GET", "/api/auth/me", nil)
	if code != http.StatusUnauthorized {
		t.Fatalf("post-reset /me: expected 401, got %d", code)
	}
}

// TestE2E_QuotaExhaustedThenAdminGrantsMore_Recovers:
// E2 scenario: the user exhausts the quota → the admin raises the limit → the user can write again.
//
// Semantics context:
//   - resetLimits=true in admin/access zeroes user_mode_access.daily_message_limit
//     (this is a REVOKE, not "reset the counter to zero"). After a reset a new grant is needed.
//   - In the real flow the admin either just GRANTS MORE access (no reset), or
//     does reset + grant in two separate calls. Here we test a plain grant.
func TestE2E_QuotaExhaustedThenAdminGrantsMore_Recovers(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	user, mode, dialog := authedUserWithDialog(t, env, ts, 2)

	// Step 1: use up both slots
	for i := 0; i < 2; i++ {
		code, _ := httpJSON(t, ts, "POST", "/api/chat/send", map[string]any{
			"dialogId": dialog.ID, "text": "msg-" + itoa(int64(i)),
		})
		if code != http.StatusOK {
			t.Fatalf("setup send #%d: %d", i, code)
		}
	}

	// Step 2: the third → 429
	code, _ := httpJSON(t, ts, "POST", "/api/chat/send", map[string]any{
		"dialogId": dialog.ID, "text": "rejected",
	})
	if code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 after exhaustion, got %d", code)
	}

	// Step 3: the admin grants additional access (without resetLimits): adds a
	// new user_mode_access row with limit 100. perModeLimit sums them:
	// the original 2 + the new 100 = 102.
	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	adminTS := NewTestServer(t, env.Pool)
	adminTS.LoginAs(f.CreateSession(admin.ID))
	code, body := httpJSON(t, adminTS, "POST", "/api/admin/access", map[string]any{
		"userId":            user.ID,
		"modeIds":           []int64{mode.ID},
		"days":              7,
		"dailyMessageLimit": 100,
	})
	if code != http.StatusOK {
		t.Fatalf("admin grant: %d body=%v", code, body)
	}

	// Step 4: the 3rd message now passes (limit 102, used 2, 2+1<=102)
	code, _ = httpJSON(t, ts, "POST", "/api/chat/send", map[string]any{
		"dialogId": dialog.ID, "text": "after extension",
	})
	if code != http.StatusOK {
		t.Fatalf("send after admin grant: expected 200, got %d", code)
	}

	// DB: 3 user messages (2 + 1) in this dialog
	var msgCount int64
	_ = env.Pool.QueryRow(context.Background(),
		`select count(*) from dialogs_messages where dialog_id = $1 and role = 'user'`,
		dialog.ID).Scan(&msgCount)
	if msgCount != 3 {
		t.Fatalf("user messages: got %d want 3", msgCount)
	}
}

// TestE2E_SoftDeleteChain: a deleted dialog disappears from history + sending into it fails.
// The E10 scenario from the plan.
func TestE2E_SoftDeleteChain(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)

	user, _, dialog := authedUserWithDialog(t, env, ts, 50)
	_ = user

	// Send a message
	code, _ := httpJSON(t, ts, "POST", "/api/chat/send", map[string]any{
		"dialogId": dialog.ID, "text": "before delete",
	})
	if code != http.StatusOK {
		t.Fatalf("send: %d", code)
	}

	// Delete the dialog
	code, _ = httpJSON(t, ts, "DELETE", "/api/chat/history?dialogId="+itoa(dialog.ID), nil)
	if code != http.StatusOK {
		t.Fatalf("delete: %d", code)
	}

	// This dialog's history must now be either empty or without accessActive.
	code, body := httpJSON(t, ts, "GET", "/api/chat/history?dialogId="+itoa(dialog.ID), nil)
	// Status 200 or 404: both are correct outcomes. What matters: whether messages is empty.
	if code == http.StatusOK {
		msgs, _ := body["messages"].([]any)
		if active, _ := body["accessActive"].(bool); active && len(msgs) > 0 {
			t.Fatalf("deleted dialog still shows %d messages with accessActive=true: %v",
				len(msgs), body)
		}
	}

	// Sending into a deleted dialog must be rejected
	code, _ = httpJSON(t, ts, "POST", "/api/chat/send", map[string]any{
		"dialogId": dialog.ID, "text": "after delete",
	})
	if code == http.StatusOK {
		t.Fatalf("send into deleted dialog succeeded — soft-delete not enforced on send")
	}
}
