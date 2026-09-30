//go:build integration

package httpapi

import (
	"context"
	"net/http"
	"testing"

	"mindstrata-stage1/api/internal/testsupport"
)

// =============================================================================
// SLAB E — broadcast + summary prompts CRUD
// =============================================================================

// 1. AdminSummaryPrompts GET → list.
func TestAdminSummaryPrompts_GETList(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	_, _ = env.Pool.Exec(context.Background(),
		`insert into admin_summary_prompts (name, prompt, is_default) values ('L1', 'p1', false), ('L2', 'p2', true)`)
	ts.LoginAs(f.CreateSession(admin.ID))

	status, body := httpJSON(t, ts, "GET", "/api/admin/summary-prompts", nil)
	if status != http.StatusOK {
		t.Fatalf("status %d body=%v", status, body)
	}
	prompts, _ := body["prompts"].([]any)
	if len(prompts) < 2 {
		t.Errorf("expected ≥2 prompts, got %d", len(prompts))
	}
}

// 2. AdminSummaryPrompts POST → create + audit.
func TestAdminSummaryPrompts_POST_Creates(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	status, body := httpJSON(t, ts, "POST", "/api/admin/summary-prompts", map[string]any{
		"name":      "MyPrompt",
		"prompt":    "system prompt content",
		"isDefault": false,
	})
	if status != http.StatusCreated {
		t.Fatalf("status=%d body=%v", status, body)
	}
	id, _ := body["promptId"].(float64)
	if id == 0 {
		t.Errorf("no promptId returned")
	}

	// DB
	var cnt int64
	_ = env.Pool.QueryRow(context.Background(),
		`select count(*) from admin_summary_prompts where id=$1 and name='MyPrompt'`, int64(id)).Scan(&cnt)
	if cnt != 1 {
		t.Errorf("prompt not in DB")
	}

	// Audit
	var auditN int64
	_ = env.Pool.QueryRow(context.Background(),
		`select count(*) from admin_audit_log where action='admin.summary_prompt.create' and target_id=$1`,
		int64(id)).Scan(&auditN)
	if auditN == 0 {
		t.Errorf("audit not written")
	}
}

// 3. AdminSummaryPrompts POST empty name → 400.
func TestAdminSummaryPrompts_POST_NoName_400(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	status, _ := httpJSON(t, ts, "POST", "/api/admin/summary-prompts", map[string]any{
		"name":   "",
		"prompt": "p",
	})
	if status != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", status)
	}
}

// 4. AdminSummaryPrompts non-admin → 403.
func TestAdminSummaryPrompts_NonAdmin_403(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	user := f.CreateUser(TestUserOpts{Role: "user"})
	ts.LoginAs(f.CreateSession(user.ID))

	status, _ := httpJSON(t, ts, "GET", "/api/admin/summary-prompts", nil)
	if status != http.StatusForbidden && status != http.StatusUnauthorized {
		t.Errorf("user GET: expected 403/401, got %d", status)
	}
}

// 5. AdminSummaryPromptDetail PATCH update.
func TestAdminSummaryPromptDetail_PATCH(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	var id int64
	_ = env.Pool.QueryRow(context.Background(),
		`insert into admin_summary_prompts (name, prompt, is_default) values ('OldName', 'OldPrompt', false) returning id`).Scan(&id)
	ts.LoginAs(f.CreateSession(admin.ID))

	status, _ := adminPatch(t, ts, "/api/admin/summary-prompts/"+itoa(id), map[string]any{
		"name":      "NewName",
		"prompt":    "NewPrompt",
		"isDefault": false,
	})
	if status != http.StatusOK {
		t.Fatalf("patch: %d", status)
	}

	// DB updated
	var name string
	_ = env.Pool.QueryRow(context.Background(),
		`select name from admin_summary_prompts where id=$1`, id).Scan(&name)
	if name != "NewName" {
		t.Errorf("name not updated: %q", name)
	}
}

// 6. AdminSummaryPromptDetail DELETE — non-default deleted.
func TestAdminSummaryPromptDetail_DELETE_NonDefault(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	var id int64
	_ = env.Pool.QueryRow(context.Background(),
		`insert into admin_summary_prompts (name, prompt, is_default) values ('Disposable', 'p', false) returning id`).Scan(&id)
	ts.LoginAs(f.CreateSession(admin.ID))

	status, _ := httpJSON(t, ts, "DELETE", "/api/admin/summary-prompts/"+itoa(id), nil)
	if status != http.StatusOK {
		t.Fatalf("delete: %d", status)
	}

	var cnt int64
	_ = env.Pool.QueryRow(context.Background(),
		`select count(*) from admin_summary_prompts where id=$1`, id).Scan(&cnt)
	if cnt != 0 {
		t.Errorf("non-default delete should remove row, count=%d", cnt)
	}
}

// 7. AdminSummaryPromptDetail DELETE default → row stays.
func TestAdminSummaryPromptDetail_DELETE_Default_Preserved(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	var id int64
	_ = env.Pool.QueryRow(context.Background(),
		`insert into admin_summary_prompts (name, prompt, is_default) values ('Default', 'p', true) returning id`).Scan(&id)
	ts.LoginAs(f.CreateSession(admin.ID))

	_, _ = httpJSON(t, ts, "DELETE", "/api/admin/summary-prompts/"+itoa(id), nil)

	// Default prompt cannot be deleted — WHERE is_default = false guards.
	var cnt int64
	_ = env.Pool.QueryRow(context.Background(),
		`select count(*) from admin_summary_prompts where id=$1`, id).Scan(&cnt)
	if cnt != 1 {
		t.Errorf("default prompt deleted, count=%d", cnt)
	}
}

// 8. AdminSummaryPromptDetail invalid id → 400.
func TestAdminSummaryPromptDetail_InvalidID_400(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	status, _ := httpJSON(t, ts, "DELETE", "/api/admin/summary-prompts/not-a-number", nil)
	if status != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", status)
	}
}

// 9. AdminSummaryPromptDetail wrong method → 405.
func TestAdminSummaryPromptDetail_WrongMethod_405(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	status, _ := httpJSON(t, ts, "GET", "/api/admin/summary-prompts/123", nil)
	if status != http.StatusMethodNotAllowed && status != http.StatusBadRequest {
		t.Errorf("GET on detail: got %d (expected 405 or 400 if id invalid path)", status)
	}
}

// 10. AdminBroadcasts POST audience=active_access happy path.
func TestAdminBroadcasts_POST_ActiveAccess(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	owner := f.CreateUser(TestUserOpts{Role: "owner"})
	user := f.CreateUser(TestUserOpts{Role: "user"})
	mode := f.CreateMode(TestModeOpts{})
	f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: mode.ID, DailyMessageLimit: 10})

	ts.LoginAs(f.CreateSession(owner.ID))
	status, body := httpJSON(t, ts, "POST", "/api/admin/broadcasts", map[string]any{
		"audience": "active_access",
		"message":  "hello users",
	})
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%v", status, body)
	}
	cnt, _ := body["plannedRecipients"].(float64)
	if cnt < 1 {
		t.Errorf("expected ≥1 recipient, got %v", cnt)
	}
	if state, _ := body["status"].(string); state != "saved_for_manual_delivery" {
		t.Errorf("status field: %q", state)
	}
}

// 11. AdminBroadcasts POST empty message → 400.
func TestAdminBroadcasts_POST_NoMessage_400(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	owner := f.CreateUser(TestUserOpts{Role: "owner"})
	ts.LoginAs(f.CreateSession(owner.ID))

	status, _ := httpJSON(t, ts, "POST", "/api/admin/broadcasts", map[string]any{
		"audience": "active_access",
	})
	if status != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", status)
	}
}

// 12. AdminBroadcasts POST audience=promocode empty IDs → 400.
func TestAdminBroadcasts_POST_PromocodeNoIDs_400(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	owner := f.CreateUser(TestUserOpts{Role: "owner"})
	ts.LoginAs(f.CreateSession(owner.ID))

	status, _ := httpJSON(t, ts, "POST", "/api/admin/broadcasts", map[string]any{
		"audience":     "promocode",
		"message":      "hi",
		"promocodeIds": []int64{},
	})
	if status != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", status)
	}
}

// 13. AdminBroadcasts POST audience=users with IDs → counts.
func TestAdminBroadcasts_POST_UserIDsAudience(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	owner := f.CreateUser(TestUserOpts{Role: "owner"})
	target := f.CreateUser(TestUserOpts{Role: "user", Status: "active"})
	ts.LoginAs(f.CreateSession(owner.ID))

	status, body := httpJSON(t, ts, "POST", "/api/admin/broadcasts", map[string]any{
		"audience": "users",
		"message":  "ping",
		"userIds":  []int64{target.ID},
	})
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%v", status, body)
	}
	cnt, _ := body["plannedRecipients"].(float64)
	if cnt != 1 {
		t.Errorf("plannedRecipients want 1, got %v", cnt)
	}
}

// 14. AdminBroadcasts non-owner → 403.
func TestAdminBroadcasts_NonOwner_403(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	// admin is ok (in requireOwnerAdmin allow list).
	// But billing_admin should be blocked.
	billing := f.CreateUser(TestUserOpts{Role: "billing_admin"})
	ts.LoginAs(f.CreateSession(billing.ID))

	status, _ := httpJSON(t, ts, "POST", "/api/admin/broadcasts", map[string]any{
		"audience": "active_access",
		"message":  "x",
	})
	if status != http.StatusForbidden && status != http.StatusUnauthorized {
		t.Errorf("billing_admin: expected 403/401, got %d", status)
	}
}

// 15. AdminBroadcasts wrong method → 405.
func TestAdminBroadcasts_WrongMethod_405(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	owner := f.CreateUser(TestUserOpts{Role: "owner"})
	ts.LoginAs(f.CreateSession(owner.ID))

	status, _ := httpJSON(t, ts, "GET", "/api/admin/broadcasts", nil)
	if status != http.StatusMethodNotAllowed {
		t.Errorf("GET: expected 405, got %d", status)
	}
}

// =============================================================================
// AdminOrchestrationPrompt & AdminDialogSummaryPrompt — settings GET/PATCH
// =============================================================================

// 16. AdminOrchestrationPrompt GET returns current setting.
func TestAdminOrchestrationPrompt_GET(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	status, body := httpJSON(t, ts, "GET", "/api/admin/orchestration-prompt", nil)
	if status != http.StatusOK {
		t.Fatalf("status %d body=%v", status, body)
	}
	if _, ok := body["prompt"].(string); !ok {
		t.Errorf("missing prompt field: %v", body)
	}
}

// 17. AdminOrchestrationPrompt PATCH stores in settings.
func TestAdminOrchestrationPrompt_PATCH(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	custom := "CUSTOM_ORCHESTRATION_PROMPT_XYZ"
	status, _ := httpJSON(t, ts, "POST", "/api/admin/orchestration-prompt", map[string]any{
		"prompt": custom,
	})
	if status != http.StatusOK {
		t.Fatalf("patch: %d", status)
	}

	var val string
	_ = env.Pool.QueryRow(context.Background(),
		`select value from system_settings where key='orchestration_prompt_default'`).Scan(&val)
	if val != custom {
		t.Errorf("setting not persisted: %q", val)
	}
}

// 18. AdminDialogSummaryPrompt GET/PATCH.
func TestAdminDialogSummaryPrompt_RoundTrip(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	custom := "DLG_SUMMARY_PROMPT_XYZ"
	status, _ := httpJSON(t, ts, "POST", "/api/admin/dialog-summary-prompt", map[string]any{
		"prompt": custom,
	})
	if status != http.StatusOK {
		t.Fatalf("patch: %d", status)
	}

	status, body := httpJSON(t, ts, "GET", "/api/admin/dialog-summary-prompt", nil)
	if status != http.StatusOK {
		t.Fatalf("get: %d", status)
	}
	if got, _ := body["prompt"].(string); got != custom {
		t.Errorf("prompt: got %q want %q", got, custom)
	}
}

// 19. AdminLeadSummaryPrompt GET/POST: a configurable lead summary prompt
// (see lead_notifications.go), not hard-coded.
func TestAdminLeadSummaryPrompt_RoundTrip(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	custom := "LEAD_SUMMARY_PROMPT_XYZ"
	status, _ := httpJSON(t, ts, "POST", "/api/admin/lead-summary-prompt", map[string]any{
		"prompt": custom,
	})
	if status != http.StatusOK {
		t.Fatalf("patch: %d", status)
	}

	status, body := httpJSON(t, ts, "GET", "/api/admin/lead-summary-prompt", nil)
	if status != http.StatusOK {
		t.Fatalf("get: %d", status)
	}
	if got, _ := body["prompt"].(string); got != custom {
		t.Errorf("prompt: got %q want %q", got, custom)
	}

	var val string
	_ = env.Pool.QueryRow(context.Background(),
		`select value from system_settings where key='lead_summary_prompt_default'`).Scan(&val)
	if val != custom {
		t.Errorf("setting not persisted: %q", val)
	}
}
