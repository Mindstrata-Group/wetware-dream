//go:build integration

package httpapi

import (
	"net/http"
	"testing"

	"mindstrata-stage1/api/internal/testsupport"
)

// TestResolveDirectNotificationRecipients_ModeHasVsWrote (World: "has" and
// "wrote" are really different recipient sets, not just different strings in
// the query. The existing TestAdminNotificationSend_ModeAudience_Has/Wrote in
// max_messenger_integration_test.go check each filter separately with
// recipientCount=1; here both filters run on the SAME two users, with an
// explicit check that the filter excludes the other segment).
func TestResolveDirectNotificationRecipients_ModeHasVsWrote(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)
	mode := f.CreateMode(TestModeOpts{})

	grantedOnly := f.CreateUser(TestUserOpts{})
	f.GrantAccess(GrantAccessOpts{UserID: grantedOnly.ID, ModeID: mode.ID})

	wroteUser := f.CreateUser(TestUserOpts{})
	f.GrantAccess(GrantAccessOpts{UserID: wroteUser.ID, ModeID: mode.ID})
	dialog := f.CreateDialog(wroteUser.ID, mode.ID)
	f.AppendMessage(dialog.ID, "user", "У меня есть вопрос про режим")

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	code, hasBody := httpJSON(t, ts, "POST", "/api/admin/notifications/preview", map[string]any{
		"audience": "mode",
		"modeIds":  []int64{mode.ID},
		"channels": []string{"inbox"},
	})
	if code != http.StatusOK {
		t.Fatalf("preview has: %d body=%v", code, hasBody)
	}
	assertRecipientIDs(t, hasBody, []int64{grantedOnly.ID, wroteUser.ID}, nil)

	code, wroteBody := httpJSON(t, ts, "POST", "/api/admin/notifications/preview", map[string]any{
		"audience":   "mode",
		"modeIds":    []int64{mode.ID},
		"modeFilter": "wrote",
		"channels":   []string{"inbox"},
	})
	if code != http.StatusOK {
		t.Fatalf("preview wrote: %d body=%v", code, wroteBody)
	}
	assertRecipientIDs(t, wroteBody, []int64{wroteUser.ID}, []int64{grantedOnly.ID})
}

func assertRecipientIDs(t *testing.T, body map[string]any, wantIn []int64, wantOut []int64) {
	t.Helper()
	recipients, _ := body["recipients"].([]any)
	ids := map[int64]bool{}
	for _, r := range recipients {
		rec, _ := r.(map[string]any)
		ids[int64(rec["id"].(float64))] = true
	}
	for _, id := range wantIn {
		if !ids[id] {
			t.Errorf("expected recipient %d to be included, got ids=%v", id, ids)
		}
	}
	for _, id := range wantOut {
		if ids[id] {
			t.Errorf("expected recipient %d to be excluded, got ids=%v", id, ids)
		}
	}
}
