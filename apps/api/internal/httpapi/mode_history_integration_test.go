//go:build integration

package httpapi

import (
	"context"
	"net/http"
	"strconv"
	"testing"

	"mindstrata-stage1/api/internal/testsupport"
)

// modeHistoryTestServer: a server with a fake mode history store (no real
// git/SSH); configured decides whether the feature is on.
func modeHistoryTestServer(t testing.TB, env *testsupport.Env, configured bool) (*TestServer, *fakeModeHistoryStore) {
	t.Helper()
	store := newFakeModeHistoryStore(configured)
	ts := NewTestServerWithHandler(t, env.Pool, Handler{ModeHistory: store})
	return ts, store
}

// TestModeHistory_SaveCreatesCommit: both creating and editing a mode in the admin
// commit a snapshot to the git store (POST /api/admin/modes, PATCH .../{id}).
func TestModeHistory_SaveCreatesCommit(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts, store := modeHistoryTestServer(t, env, true)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	// Creation: version #1.
	code, body := httpJSON(t, ts, http.MethodPost, "/api/admin/modes", map[string]any{
		"name":   "История промпта",
		"prompt": "Начальный промпт",
	})
	if code != http.StatusCreated {
		t.Fatalf("create: %d body=%v", code, body)
	}
	modeIDFloat, _ := body["modeId"].(float64)
	modeID := int64(modeIDFloat)
	if modeID == 0 {
		t.Fatalf("no modeId in response: %v", body)
	}

	entries, err := store.history(context.Background(), modeID, 50)
	if err != nil {
		t.Fatalf("history after create: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 version after create, got %d", len(entries))
	}
	if entries[0].AuthorEmail != admin.Email {
		t.Fatalf("author email: got %q want %q", entries[0].AuthorEmail, admin.Email)
	}

	// Edit: version #2.
	code, body = adminPatch(t, ts, "/api/admin/modes/"+strconv.FormatInt(modeID, 10), map[string]any{
		"prompt": "Обновлённый промпт",
	})
	if code != http.StatusOK {
		t.Fatalf("patch: %d body=%v", code, body)
	}
	entries, err = store.history(context.Background(), modeID, 50)
	if err != nil {
		t.Fatalf("history after patch: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 versions after patch, got %d", len(entries))
	}

	// GET /history: the same list over HTTP.
	code, listBody := httpJSON(t, ts, http.MethodGet, "/api/admin/modes/"+strconv.FormatInt(modeID, 10)+"/history", nil)
	if code != http.StatusOK {
		t.Fatalf("get history: %d body=%v", code, listBody)
	}
	versions, _ := listBody["versions"].([]any)
	if len(versions) != 2 {
		t.Fatalf("GET history: expected 2 versions, got %d (%v)", len(versions), listBody)
	}
}

// TestModeHistory_RestoreCreatesNewVersion_NotOverwrite: restore applies old
// fields through the regular UPDATE path and creates a NEW version; history is
// not rewritten (like git revert, not git reset).
func TestModeHistory_RestoreCreatesNewVersion_NotOverwrite(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts, store := modeHistoryTestServer(t, env, true)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	code, body := httpJSON(t, ts, http.MethodPost, "/api/admin/modes", map[string]any{
		"name":   "Восстановление",
		"prompt": "Версия 1",
	})
	if code != http.StatusCreated {
		t.Fatalf("create: %d body=%v", code, body)
	}
	modeID := int64(body["modeId"].(float64))

	entriesAfterCreate, _ := store.history(context.Background(), modeID, 50)
	firstSHA := entriesAfterCreate[0].SHA

	code, body = adminPatch(t, ts, "/api/admin/modes/"+strconv.FormatInt(modeID, 10), map[string]any{
		"prompt": "Версия 2",
	})
	if code != http.StatusOK {
		t.Fatalf("patch: %d body=%v", code, body)
	}

	// Restore version 1.
	code, body = httpJSON(t, ts, http.MethodPost, "/api/admin/modes/"+strconv.FormatInt(modeID, 10)+"/restore/"+firstSHA, nil)
	if code != http.StatusOK {
		t.Fatalf("restore: %d body=%v", code, body)
	}

	var currentPrompt string
	if err := env.Pool.QueryRow(context.Background(), `select prompt from modes where id = $1`, modeID).Scan(&currentPrompt); err != nil {
		t.Fatalf("query prompt: %v", err)
	}
	if currentPrompt != "Версия 1" {
		t.Fatalf("prompt after restore: got %q want %q", currentPrompt, "Версия 1")
	}

	entries, _ := store.history(context.Background(), modeID, 50)
	if len(entries) != 3 {
		t.Fatalf("restore must create a NEW version on top (revert), not overwrite history: expected 3 versions, got %d", len(entries))
	}
	// History is not rewritten: the first (by time) version still exists untouched.
	found := false
	for _, e := range entries {
		if e.SHA == firstSHA {
			found = true
		}
	}
	if !found {
		t.Fatalf("original version %s must still be present in history after restore", firstSHA)
	}
}

// TestModeHistory_NotConfigured_NoOp: without a key (MODE_HISTORY_DEPLOY_KEY
// empty) saving does not fail, history is empty.
func TestModeHistory_NotConfigured_NoOp(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts, store := modeHistoryTestServer(t, env, false)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	code, body := httpJSON(t, ts, http.MethodPost, "/api/admin/modes", map[string]any{
		"name":   "Без версий",
		"prompt": "Промпт",
	})
	if code != http.StatusCreated {
		t.Fatalf("create should succeed even with history disabled: %d body=%v", code, body)
	}
	modeID := int64(body["modeId"].(float64))

	if store.configured() {
		t.Fatalf("store should be unconfigured for this test")
	}
	entries, err := store.history(context.Background(), modeID, 50)
	if err != nil || len(entries) != 0 {
		t.Fatalf("history must be empty when not configured: entries=%v err=%v", entries, err)
	}

	code, listBody := httpJSON(t, ts, http.MethodGet, "/api/admin/modes/"+strconv.FormatInt(modeID, 10)+"/history", nil)
	if code != http.StatusOK {
		t.Fatalf("get history (disabled): %d body=%v", code, listBody)
	}
	versions, _ := listBody["versions"].([]any)
	if len(versions) != 0 {
		t.Fatalf("expected empty versions list when history disabled, got %v", versions)
	}
}

// TestModeHistory_PushFailureDoesNotFailSave: a git push error must not break an
// already completed save to the DB (like notification.history_write_failed).
func TestModeHistory_PushFailureDoesNotFailSave(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	store := newFakeModeHistoryStore(true)
	store.CommitErr = errFakeGitPush
	ts := NewTestServerWithHandler(t, env.Pool, Handler{ModeHistory: store})

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	code, body := httpJSON(t, ts, http.MethodPost, "/api/admin/modes", map[string]any{
		"name":   "Пуш падает",
		"prompt": "Промпт",
	})
	if code != http.StatusCreated {
		t.Fatalf("create must still succeed when git push fails: %d body=%v", code, body)
	}
	modeID := int64(body["modeId"].(float64))

	var count int64
	if err := env.Pool.QueryRow(context.Background(), `select count(*) from modes where id = $1`, modeID).Scan(&count); err != nil {
		t.Fatalf("query: %v", err)
	}
	if count != 1 {
		t.Fatalf("mode must be persisted in DB despite git push failure")
	}

	var auditCount int64
	_ = env.Pool.QueryRow(context.Background(),
		`select count(*) from admin_audit_log where actor_user_id = $1 and action = 'admin.mode.history_write_failed' and target_id = $2`,
		admin.ID, modeID).Scan(&auditCount)
	if auditCount == 0 {
		t.Fatalf("expected admin.mode.history_write_failed audit entry")
	}
}

var errFakeGitPush = fakeGitPushError{}

type fakeGitPushError struct{}

func (fakeGitPushError) Error() string { return "имитация ошибки git push" }
