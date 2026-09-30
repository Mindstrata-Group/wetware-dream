//go:build integration

package httpapi

import (
	"context"
	"net/http"
	"testing"

	"mindstrata-stage1/api/internal/testsupport"
)

func TestDeleteAccount_OK(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	user := f.CreateUser(TestUserOpts{Role: "user"})
	ts.LoginAs(f.CreateSession(user.ID))

	// Give the user access to a mode
	_, _ = env.Pool.Exec(context.Background(),
		`insert into user_mode_access (user_id, mode_id, active_from, active_to)
		 values ($1, (select id from modes limit 1), now(), now() + interval '30 days')
		 on conflict do nothing`, user.ID)

	status, body := httpJSON(t, ts, "DELETE", "/api/profile", nil)
	if status != http.StatusOK {
		t.Fatalf("delete: status %d body=%v", status, body)
	}

	// User marked deleted and PII nulled
	var dbStatus, dbEmail, dbDisplayName string
	var dbDeletedAt *string
	_ = env.Pool.QueryRow(context.Background(),
		`select status, coalesce(email, '__null__'), coalesce(display_name, '__null__'), deleted_at::text
		 from users where id = $1`, user.ID).Scan(&dbStatus, &dbEmail, &dbDisplayName, &dbDeletedAt)

	if dbStatus != "blocked" {
		t.Errorf("status: want blocked, got %q", dbStatus)
	}
	if dbEmail != "__null__" {
		t.Errorf("email should be null after delete, got %q", dbEmail)
	}
	if dbDeletedAt == nil {
		t.Errorf("deleted_at must be set")
	}

	// Grants removed
	var accessCnt int64
	_ = env.Pool.QueryRow(context.Background(),
		`select count(*) from user_mode_access where user_id = $1`, user.ID).Scan(&accessCnt)
	if accessCnt != 0 {
		t.Errorf("user_mode_access: want 0, got %d", accessCnt)
	}

	// After deletion the profile is unavailable (session revoked)
	status2, _ := httpJSON(t, ts, "GET", "/api/profile", nil)
	if status2 != http.StatusUnauthorized && status2 != http.StatusForbidden {
		t.Errorf("profile after delete: want 401/403, got %d", status2)
	}
}

func TestDeleteAccount_Unauthenticated_401(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)
	_ = env

	status, _ := httpJSON(t, ts, "DELETE", "/api/profile", nil)
	if status != http.StatusUnauthorized && status != http.StatusForbidden {
		t.Errorf("want 401/403, got %d", status)
	}
}

// The user's dialogs are soft-deleted, not physically erased.
func TestDeleteAccount_DialogsSoftDeleted(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	user := f.CreateUser(TestUserOpts{Role: "user"})
	mode := f.CreateMode(TestModeOpts{})
	dialog := f.CreateDialog(user.ID, mode.ID)
	f.AppendMessage(dialog.ID, "user", "привет")

	ts.LoginAs(f.CreateSession(user.ID))

	status, body := httpJSON(t, ts, "DELETE", "/api/profile", nil)
	if status != http.StatusOK {
		t.Fatalf("delete: %d body=%v", status, body)
	}

	// The dialog must have deleted_at (soft delete), not be physically deleted.
	var deletedAt *string
	_ = env.Pool.QueryRow(t.Context(),
		`select deleted_at::text from users_dialogs where id = $1`, dialog.ID).Scan(&deletedAt)
	if deletedAt == nil {
		t.Errorf("users_dialogs.deleted_at не выставлен после удаления аккаунта")
	}

	// Messages stay in the DB (audit trail).
	var msgCnt int64
	_ = env.Pool.QueryRow(t.Context(),
		`select count(*) from dialogs_messages where dialog_id = $1`, dialog.ID).Scan(&msgCnt)
	if msgCnt == 0 {
		t.Errorf("сообщения должны остаться в dialogs_messages, но их нет")
	}
}

func TestDeleteAccount_RevokesAllUserSessions(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	user := f.CreateUser(TestUserOpts{Role: "user"})
	token1 := f.CreateSession(user.ID)
	_ = f.CreateSession(user.ID)
	_ = f.CreateSession(user.ID)
	ts.LoginAs(token1)

	status, body := httpJSON(t, ts, "DELETE", "/api/profile", nil)
	if status != http.StatusOK {
		t.Fatalf("delete: status=%d body=%v", status, body)
	}

	var activeSessions int64
	if err := env.Pool.QueryRow(t.Context(),
		`select count(*) from auth_sessions where user_id = $1 and revoked_at is null`,
		user.ID).Scan(&activeSessions); err != nil {
		t.Fatalf("count active sessions: %v", err)
	}
	if activeSessions != 0 {
		t.Fatalf("active sessions after delete: got %d want 0", activeSessions)
	}
}

// An administrator can delete their own account: the role does not block it.
func TestDeleteAccount_AdminCanDeleteSelf(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	status, body := httpJSON(t, ts, "DELETE", "/api/profile", nil)
	if status != http.StatusOK {
		t.Fatalf("admin delete self: %d body=%v", status, body)
	}

	var dbDeletedAt *string
	_ = env.Pool.QueryRow(t.Context(),
		`select deleted_at::text from users where id = $1`, admin.ID).Scan(&dbDeletedAt)
	if dbDeletedAt == nil {
		t.Errorf("admin deleted_at не выставлен")
	}
}
