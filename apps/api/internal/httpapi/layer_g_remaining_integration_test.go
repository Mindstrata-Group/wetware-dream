//go:build integration

package httpapi

import (
	"context"
	"net/http"
	"testing"

	"mindstrata-stage1/api/internal/testsupport"
)

// =============================================================================
// SLAB G — admin_dialog + chat_history + ensureDefaultChatSelection + remainder
// =============================================================================

// 1. AdminDialogDetail happy path.
func TestAdminDialogDetail_GET(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	user := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{})
	dialog := f.CreateDialog(user.ID, mode.ID)
	_ = f.AppendMessage(dialog.ID, "user", "MSG_FOR_DETAIL_xyz")
	ts.LoginAs(f.CreateSession(admin.ID))

	status, body := httpJSON(t, ts, "GET", "/api/admin/dialogs/"+itoa(dialog.ID), nil)
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%v", status, body)
	}
	dlg, _ := body["dialog"].(map[string]any)
	if dlg == nil {
		t.Fatalf("no dialog: %v", body)
	}
	msgs, _ := dlg["messages"].([]any)
	if len(msgs) == 0 {
		t.Errorf("no messages in detail: %v", dlg)
	}
}

// 2. AdminDialogDetail: not found → 404.
func TestAdminDialogDetail_NotFound_404(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	status, _ := httpJSON(t, ts, "GET", "/api/admin/dialogs/999999999", nil)
	if status != http.StatusNotFound {
		t.Errorf("expected 404, got %d", status)
	}
}

// 3. AdminDialogDetail: invalid id → 400.
func TestAdminDialogDetail_InvalidID_400(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	status, _ := httpJSON(t, ts, "GET", "/api/admin/dialogs/abc", nil)
	if status != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", status)
	}
}

// 4. AdminDialogDetail: POST → 405.
func TestAdminDialogDetail_POST_405(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	user := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{})
	dialog := f.CreateDialog(user.ID, mode.ID)
	ts.LoginAs(f.CreateSession(admin.ID))

	status, _ := httpJSON(t, ts, "POST", "/api/admin/dialogs/"+itoa(dialog.ID), nil)
	if status != http.StatusMethodNotAllowed {
		t.Errorf("POST: expected 405, got %d", status)
	}
}

// 5. AdminDialogs: GET list returns array.
func TestAdminDialogs_GETList(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	user := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{})
	_ = f.CreateDialog(user.ID, mode.ID)
	ts.LoginAs(f.CreateSession(admin.ID))

	status, body := httpJSON(t, ts, "GET", "/api/admin/dialogs", nil)
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%v", status, body)
	}
	if _, ok := body["dialogs"]; !ok {
		t.Errorf("missing dialogs field")
	}
}

// 6. AdminDialogs: non-admin → 403.
func TestAdminDialogs_NonAdmin_403(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	user := f.CreateUser(TestUserOpts{Role: "user"})
	ts.LoginAs(f.CreateSession(user.ID))

	status, _ := httpJSON(t, ts, "GET", "/api/admin/dialogs", nil)
	if status != http.StatusForbidden && status != http.StatusUnauthorized {
		t.Errorf("user GET: expected 403/401, got %d", status)
	}
}

// 7. AdminDialogs wrong method → 405.
func TestAdminDialogs_WrongMethod_405(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	status, _ := httpJSON(t, ts, "POST", "/api/admin/dialogs", nil)
	if status != http.StatusMethodNotAllowed {
		t.Errorf("POST: expected 405, got %d", status)
	}
}

// =============================================================================
// chat_history — DeleteHistory, CompleteDialog
// =============================================================================

// 8. DeleteHistory all=true: marks all dialogs deleted.
func TestDeleteHistory_AllTrue(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	user := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{})
	dlg1 := f.CreateDialog(user.ID, mode.ID)
	dlg2 := f.CreateDialog(user.ID, mode.ID)
	ts.LoginAs(f.CreateSession(user.ID))

	status, body := httpJSON(t, ts, "DELETE", "/api/chat/history?all=true", nil)
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%v", status, body)
	}
	deleted, _ := body["deleted"].(float64)
	if deleted < 2 {
		t.Errorf("expected ≥2 deleted, got %v", deleted)
	}

	// Both dialogs marked
	var open int64
	_ = env.Pool.QueryRow(context.Background(),
		`select count(*) from users_dialogs where user_id=$1 and deleted_at is null and id in ($2,$3)`,
		user.ID, dlg1.ID, dlg2.ID).Scan(&open)
	if open != 0 {
		t.Errorf("dialogs still open after all=true: %d", open)
	}
}

// 9. DeleteHistory dialogId=N: marks single.
func TestDeleteHistory_SingleDialog(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	user := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{})
	dlg1 := f.CreateDialog(user.ID, mode.ID)
	dlg2 := f.CreateDialog(user.ID, mode.ID)
	ts.LoginAs(f.CreateSession(user.ID))

	status, _ := httpJSON(t, ts, "DELETE", "/api/chat/history?dialogId="+itoa(dlg1.ID), nil)
	if status != http.StatusOK {
		t.Fatalf("status=%d", status)
	}

	var deleted1, deleted2 bool
	_ = env.Pool.QueryRow(context.Background(),
		`select (select deleted_at is not null from users_dialogs where id=$1),
		        (select deleted_at is null from users_dialogs where id=$2)`, dlg1.ID, dlg2.ID).Scan(&deleted1, &deleted2)
	if !deleted1 {
		t.Errorf("dlg1 not deleted")
	}
	if !deleted2 {
		t.Errorf("dlg2 should remain active")
	}
}

// 10. DeleteHistory no dialogId → 400.
func TestDeleteHistory_NoArgs_400(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	user := f.CreateUser(TestUserOpts{})
	ts.LoginAs(f.CreateSession(user.ID))

	status, _ := httpJSON(t, ts, "DELETE", "/api/chat/history", nil)
	if status != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", status)
	}
}

// 11. CompleteDialog wrong method → 405.
func TestCompleteDialog_WrongMethod_405(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	user := f.CreateUser(TestUserOpts{})
	ts.LoginAs(f.CreateSession(user.ID))

	status, _ := httpJSON(t, ts, "GET", "/api/chat/complete", nil)
	if status != http.StatusMethodNotAllowed {
		t.Errorf("GET: expected 405, got %d", status)
	}
}

// 12. CompleteDialog no active dialog → 400.
func TestCompleteDialog_NoActiveDialog_400(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	user := f.CreateUser(TestUserOpts{})
	ts.LoginAs(f.CreateSession(user.ID))

	status, _ := httpJSON(t, ts, "POST", "/api/chat/complete", map[string]any{})
	if status != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", status)
	}
}

// 13. CompleteDialog with dialogId: ok when there is active access.
func TestCompleteDialog_WithDialogID_Ok(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	user := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{})
	dialog := f.CreateDialog(user.ID, mode.ID)
	f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: mode.ID, DailyMessageLimit: 10})
	ts.LoginAs(f.CreateSession(user.ID))

	status, _ := httpJSON(t, ts, "POST", "/api/chat/complete", map[string]any{
		"dialogId": dialog.ID,
	})
	if status != http.StatusOK && status != http.StatusBadRequest {
		// May 400 if no transcript yet, but should not 5xx.
		t.Errorf("status=%d not 200/400", status)
	}
}

// 14. ensureDefaultChatSelection: creates dialog when none.
func TestEnsureDefaultChatSelection_CreatesDialog(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	h := Handler{DB: env.Pool}

	user := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{})
	f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: mode.ID, DailyMessageLimit: 10})

	modes := []ModeOption{{ID: mode.ID, Name: mode.Name}}
	resultMode, resultDialog := h.ensureDefaultChatSelection(
		context.Background(), user.ID, nil, nil, modes)
	if resultMode == nil || *resultMode != mode.ID {
		t.Errorf("mode: got %v want %d", resultMode, mode.ID)
	}
	if resultDialog == nil || *resultDialog == 0 {
		t.Errorf("dialog ID not assigned")
	}
}

// 15. ensureDefaultChatSelection: empty modes → nil, nil.
func TestEnsureDefaultChatSelection_EmptyModes(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	h := Handler{DB: env.Pool}

	mode, dialog := h.ensureDefaultChatSelection(
		context.Background(), 1, nil, nil, []ModeOption{})
	if mode != nil || dialog != nil {
		t.Errorf("expected nil,nil for empty modes; got %v,%v", mode, dialog)
	}
}

// 16. ensureDefaultChatSelection: existing current dialog → reused.
func TestEnsureDefaultChatSelection_ExistingDialog(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	h := Handler{DB: env.Pool}

	user := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{})
	f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: mode.ID, DailyMessageLimit: 10})
	dialog := f.CreateDialog(user.ID, mode.ID)

	dlgID := dialog.ID
	modes := []ModeOption{{ID: mode.ID, Name: mode.Name}}
	gotMode, gotDialog := h.ensureDefaultChatSelection(
		context.Background(), user.ID, nil, &dlgID, modes)
	if gotMode == nil || *gotMode != mode.ID {
		t.Errorf("mode: got %v want %d", gotMode, mode.ID)
	}
	if gotDialog == nil || *gotDialog != dlgID {
		t.Errorf("dialog reuse: got %v want %d", gotDialog, dlgID)
	}
}

// =============================================================================
// StartChat
// =============================================================================

// 17. StartChat: GET → 405.
func TestStartChat_WrongMethod_405(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)
	_ = env

	status, _ := httpJSON(t, ts, "GET", "/api/chat/start", nil)
	if status != http.StatusMethodNotAllowed {
		t.Errorf("GET: expected 405, got %d", status)
	}
}

// 18. StartChat: guest without modes → 403 access_required.
func TestStartChat_GuestNoAccess_403(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)
	_ = env

	status, body := httpJSON(t, ts, "POST", "/api/chat/start", map[string]any{})
	if status != http.StatusForbidden {
		t.Errorf("expected 403 access_required, got %d body=%v", status, body)
	}
	if code, _ := body["code"].(string); code != "access_required" {
		t.Errorf("missing code=access_required: %v", body)
	}
}

func TestStartChat_ReturnsChatMessageMaxChars(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)
	user := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{})
	f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: mode.ID, DailyMessageLimit: 50})
	_, _ = env.Pool.Exec(context.Background(),
		`insert into system_settings (key, value) values ($1, $2)
		 on conflict (key) do update set value=excluded.value`,
		"chat_message_max_chars", "12345")
	ts.LoginAs(f.CreateSession(user.ID))

	status, body := httpJSON(t, ts, "POST", "/api/chat/start", map[string]any{})
	if status != http.StatusOK {
		t.Fatalf("POST /api/chat/start expected 200, got %d body=%v", status, body)
	}

	if got, ok := body["chatMessageMaxChars"].(float64); !ok || int(got) != 12345 {
		t.Fatalf("chatMessageMaxChars=%v (type %T), want 12345", body["chatMessageMaxChars"], body["chatMessageMaxChars"])
	}
}

// 19. SelectMode: GET → 405.
func TestSelectMode_WrongMethod_405(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)
	_ = env

	status, _ := httpJSON(t, ts, "GET", "/api/chat/select-mode", nil)
	if status != http.StatusMethodNotAllowed {
		t.Errorf("GET: expected 405, got %d", status)
	}
}

// 20. SelectMode: invalid JSON → 400.
func TestSelectMode_InvalidJSON_400(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	user := f.CreateUser(TestUserOpts{})
	ts.LoginAs(f.CreateSession(user.ID))

	// Use sendRaw
	req, _ := http.NewRequest("POST", ts.URL("/api/chat/select-mode"),
		nil) // no body → JSON decoder fails
	req.Header.Set("Content-Type", "application/json")
	resp, err := ts.Client.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close()
	// EOF on decode → "invalid payload" → 400, OR access_required → 403 (if access check first)
	if resp.StatusCode != http.StatusBadRequest && resp.StatusCode != http.StatusForbidden {
		t.Errorf("expected 400/403, got %d", resp.StatusCode)
	}
}

// 21. SelectMode without access → 403 access_required.
func TestSelectMode_NoAccess_403(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	user := f.CreateUser(TestUserOpts{})
	ts.LoginAs(f.CreateSession(user.ID))

	status, body := httpJSON(t, ts, "POST", "/api/chat/select-mode", map[string]any{
		"modeId": 1,
	})
	if status != http.StatusForbidden {
		t.Errorf("expected 403, got %d", status)
	}
	if code, _ := body["code"].(string); code != "access_required" {
		t.Errorf("missing access_required code: %v", body)
	}
}
