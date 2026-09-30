//go:build integration

package httpapi

import (
	"context"
	"net/http"
	"testing"
	"time"

	"mindstrata-stage1/api/internal/testsupport"
)

// TestCompleteDialog_QuotaExhausted_429 (User expectations: the declared
// behaviour is that an exhausted daily limit also blocks completing a dialog,
// not only regular messages; the summary record itself must NOT appear in the
// DB: the quota is checked before writing).
func TestCompleteDialog_QuotaExhausted_429(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)
	_, _, dialog := authedUserWithDialog(t, env, ts, 1)

	code1, body1 := httpJSON(t, ts, "POST", "/api/chat/send", map[string]any{
		"dialogId": dialog.ID, "text": "исчерпываем единственный слот",
	})
	if code1 != http.StatusOK {
		t.Fatalf("send to exhaust quota: %d body=%v", code1, body1)
	}

	code, body := httpJSON(t, ts, "POST", "/api/chat/complete", map[string]any{
		"dialogId": dialog.ID,
	})
	if code != http.StatusTooManyRequests {
		t.Fatalf("complete after quota exhausted: %d body=%v, want 429", code, body)
	}
	if body["code"] != "daily_quota_exhausted" {
		t.Fatalf("code = %v, want daily_quota_exhausted", body["code"])
	}

	var summaryCount int64
	if err := env.Pool.QueryRow(context.Background(),
		`select count(*) from dialogs_messages where dialog_id=$1 and role='summary'`, dialog.ID).Scan(&summaryCount); err != nil {
		t.Fatalf("count summary: %v", err)
	}
	if summaryCount != 0 {
		t.Fatalf("summary must not be created when quota is exhausted, got count=%d", summaryCount)
	}
}

// TestCompleteDialog_DialogBelongsToAnotherUser_404 (Standards: object-level
// authorisation: another user's dialog must not be completable/visible,
// regardless of the userID coming from a valid session).
func TestCompleteDialog_DialogBelongsToAnotherUser_404(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	owner := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{})
	f.GrantAccess(GrantAccessOpts{UserID: owner.ID, ModeID: mode.ID, DailyMessageLimit: 10})
	dialog := f.CreateDialog(owner.ID, mode.ID)

	intruder := f.CreateUser(TestUserOpts{})
	f.GrantAccess(GrantAccessOpts{UserID: intruder.ID, ModeID: mode.ID, DailyMessageLimit: 10})
	ts.LoginAs(f.CreateSession(intruder.ID))

	status, body := httpJSON(t, ts, "POST", "/api/chat/complete", map[string]any{
		"dialogId": dialog.ID,
	})
	if status != http.StatusNotFound {
		t.Fatalf("complete someone else's dialog: %d body=%v, want 404", status, body)
	}
}

// TestCompleteDialog_ResponseModeLive_CallsAIAndRecordsUsage (Purpose:
// responseMode=live is declared as a real AI call, not a template summary; we
// check that the summary contains text from the fake AI server and that
// message_usage is really recorded).
func TestCompleteDialog_ResponseModeLive_CallsAIAndRecordsUsage(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)

	user := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{})
	f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: mode.ID, DailyMessageLimit: 10})
	dialog := f.CreateDialog(user.ID, mode.ID)
	f.AppendMessage(dialog.ID, "user", "вопрос про режим")
	if _, err := env.Pool.Exec(context.Background(),
		`update users set current_mode=$2, current_dialog=$3, accepted_tos=true where id=$1`,
		user.ID, mode.ID, dialog.ID); err != nil {
		t.Fatalf("seed current dialog: %v", err)
	}

	fake := fakeVseGPT(t, http.StatusOK, "живое summary от AI")
	ts := NewTestServerWithHandler(t, env.Pool, Handler{
		OpenAIAPIKey:  "k",
		OpenAIBaseURL: "https://api.vsegpt.ru/v1",
		HTTPClient:    &http.Client{Transport: vsegptRT{target: fake.URL}, Timeout: 5 * time.Second},
	})
	ts.LoginAs(f.CreateSession(user.ID))

	status, body := httpJSON(t, ts, "POST", "/api/chat/complete", map[string]any{
		"dialogId":     dialog.ID,
		"responseMode": "live",
	})
	if status != http.StatusOK {
		t.Fatalf("complete live: %d body=%v", status, body)
	}
	if body["usedLive"] != true {
		t.Fatalf("usedLive = %v, want true", body["usedLive"])
	}
	summary, _ := body["summary"].(map[string]any)
	if summary["content"] != "живое summary от AI" {
		t.Fatalf("summary content = %v, want the fake AI response (proves live path was actually taken)", summary["content"])
	}

	var usageCount int64
	if err := env.Pool.QueryRow(context.Background(),
		`select count(*) from message_usage where user_id=$1 and mode_id=$2`, user.ID, mode.ID).Scan(&usageCount); err != nil {
		t.Fatalf("count usage: %v", err)
	}
	if usageCount != 1 {
		t.Fatalf("message_usage rows = %d, want 1", usageCount)
	}
}
