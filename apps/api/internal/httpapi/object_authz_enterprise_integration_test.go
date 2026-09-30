//go:build integration

package httpapi

import (
	"context"
	"net/http"
	"testing"

	"mindstrata-stage1/api/internal/testsupport"
)

func TestObjectAuthz_ForeignDialogReadAndDeleteDoNotLeakOrModify(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	mode := f.CreateMode(TestModeOpts{})
	victim := f.CreateUser(TestUserOpts{})
	attacker := f.CreateUser(TestUserOpts{})
	f.GrantAccess(GrantAccessOpts{UserID: victim.ID, ModeID: mode.ID, DailyMessageLimit: 50})
	f.GrantAccess(GrantAccessOpts{UserID: attacker.ID, ModeID: mode.ID, DailyMessageLimit: 50})

	victimDialog := f.CreateDialog(victim.ID, mode.ID)
	f.AppendMessage(victimDialog.ID, "user", "victim-secret-message")
	attackerDialog := f.CreateDialog(attacker.ID, mode.ID)
	if _, err := env.Pool.Exec(context.Background(),
		`update users set current_mode = $2, current_dialog = $3 where id = $1`,
		attacker.ID, mode.ID, attackerDialog.ID); err != nil {
		t.Fatalf("set attacker current dialog: %v", err)
	}

	ts.LoginAs(f.CreateSession(attacker.ID))

	readStatus, readBody := httpJSON(t, ts, http.MethodGet, "/api/chat/history?dialogId="+itoa(victimDialog.ID), nil)
	if readStatus == http.StatusOK {
		t.Fatalf("foreign history read returned 200 body=%v", readBody)
	}
	if messages, _ := readBody["messages"].([]any); len(messages) > 0 {
		t.Fatalf("foreign history leaked messages: %v", readBody)
	}

	deleteStatus, deleteBody := httpJSON(t, ts, http.MethodDelete, "/api/chat/history?dialogId="+itoa(victimDialog.ID), nil)
	if deleteStatus != http.StatusOK {
		t.Fatalf("foreign delete should be a harmless no-op response, got %d body=%v", deleteStatus, deleteBody)
	}
	if deleted, _ := deleteBody["deleted"].(float64); int(deleted) != 0 {
		t.Fatalf("foreign delete affected %v rows body=%v", deleteBody["deleted"], deleteBody)
	}

	var victimDeleted bool
	if err := env.Pool.QueryRow(context.Background(),
		`select deleted_at is not null from users_dialogs where id = $1`, victimDialog.ID).Scan(&victimDeleted); err != nil {
		t.Fatalf("query victim dialog deleted_at: %v", err)
	}
	if victimDeleted {
		t.Fatalf("foreign delete soft-deleted victim dialog")
	}

	var attackerCurrentDialog *int64
	if err := env.Pool.QueryRow(context.Background(),
		`select current_dialog from users where id = $1`, attacker.ID).Scan(&attackerCurrentDialog); err != nil {
		t.Fatalf("query attacker current_dialog: %v", err)
	}
	if attackerCurrentDialog == nil || *attackerCurrentDialog != attackerDialog.ID {
		t.Fatalf("foreign delete changed attacker current_dialog: got %v want %d", attackerCurrentDialog, attackerDialog.ID)
	}
}

func TestObjectAuthz_ProfileSettingsIgnoreForeignUserIDs(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	victim := f.CreateUser(TestUserOpts{})
	attacker := f.CreateUser(TestUserOpts{})
	if _, err := env.Pool.Exec(context.Background(), `
		update users
		set allow_message_anonymization = true
		where id in ($1, $2)`, victim.ID, attacker.ID); err != nil {
		t.Fatalf("seed anonymization settings: %v", err)
	}

	// S-3: decodeJSONStrict (DisallowUnknownFields) now rejects a body with an unknown
	// field "userId" entirely (400): the request never reaches the DB write, so
	// neither victim nor attacker change at all.
	ts.LoginAs(f.CreateSession(attacker.ID))
	status, body := httpJSON(t, ts, http.MethodPatch, "/api/profile/settings", map[string]any{
		"userId":                    victim.ID,
		"allowMessageAnonymization": false,
	})
	if status != http.StatusBadRequest {
		t.Fatalf("patch with unknown field userId should be rejected by strict decode: %d body=%v", status, body)
	}

	var victimAllow, attackerAllow bool
	if err := env.Pool.QueryRow(context.Background(),
		`select allow_message_anonymization from users where id = $1`, victim.ID).Scan(&victimAllow); err != nil {
		t.Fatalf("query victim setting: %v", err)
	}
	if err := env.Pool.QueryRow(context.Background(),
		`select allow_message_anonymization from users where id = $1`, attacker.ID).Scan(&attackerAllow); err != nil {
		t.Fatalf("query attacker setting: %v", err)
	}
	if !victimAllow {
		t.Fatalf("rejected request changed victim setting")
	}
	if !attackerAllow {
		t.Fatalf("rejected request changed attacker's own setting")
	}
}

func TestObjectAuthz_AdminSectionRolesDoNotEscalatePastPolicy(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	target := f.CreateUser(TestUserOpts{})

	t.Run("support can read users but cannot mutate users", func(t *testing.T) {
		ts := NewTestServer(t, env.Pool)
		support := f.CreateUser(TestUserOpts{Role: "support"})
		ts.LoginAs(f.CreateSession(support.ID))

		status, body := httpJSON(t, ts, http.MethodGet, "/api/admin/users/"+itoa(target.ID), nil)
		if status != http.StatusOK {
			t.Fatalf("support GET user: %d body=%v", status, body)
		}
		status, body = httpJSON(t, ts, http.MethodPatch, "/api/admin/users/"+itoa(target.ID), map[string]any{
			"status": "blocked",
		})
		if status != http.StatusForbidden {
			t.Fatalf("support PATCH user: got %d body=%v, want 403", status, body)
		}
		var statusValue string
		if err := env.Pool.QueryRow(context.Background(), `select status from users where id = $1`, target.ID).Scan(&statusValue); err != nil {
			t.Fatalf("query target status: %v", err)
		}
		if statusValue != "active" {
			t.Fatalf("support mutation changed target status to %q", statusValue)
		}
	})

	t.Run("support cannot use admin mutation-only routes", func(t *testing.T) {
		ts := NewTestServer(t, env.Pool)
		support := f.CreateUser(TestUserOpts{Role: "support"})
		ts.LoginAs(f.CreateSession(support.ID))

		status, body := httpJSON(t, ts, http.MethodPost, "/api/admin/modes", map[string]any{
			"name":   "support_escalation_mode",
			"prompt": "must not be created",
		})
		if status != http.StatusForbidden {
			t.Fatalf("support POST mode: got %d body=%v, want 403", status, body)
		}
		status, body = httpJSON(t, ts, http.MethodPost, "/api/admin/payments/yookassa/create", map[string]any{
			"amountRub": 100,
		})
		if status != http.StatusForbidden {
			t.Fatalf("support create yookassa payment: got %d body=%v, want 403", status, body)
		}
	})

	t.Run("admin retains admin mutations but cannot assign owner through user patch", func(t *testing.T) {
		ts := NewTestServer(t, env.Pool)
		admin := f.CreateUser(TestUserOpts{Role: "admin"})
		ts.LoginAs(f.CreateSession(admin.ID))

		status, body := httpJSON(t, ts, http.MethodPatch, "/api/admin/users/"+itoa(target.ID), map[string]any{
			"role": "owner",
		})
		if status != http.StatusBadRequest && status != http.StatusForbidden {
			t.Fatalf("admin owner escalation: got %d body=%v, want 400/403", status, body)
		}
		var role string
		if err := env.Pool.QueryRow(context.Background(), `select role from users where id = $1`, target.ID).Scan(&role); err != nil {
			t.Fatalf("query target role: %v", err)
		}
		if role == "owner" {
			t.Fatalf("admin escalated target to owner")
		}
	})
}
