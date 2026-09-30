//go:build integration

// Package-level breadth coverage: smoke tests for handlers that
// don't have dedicated _integration_test files yet. Goal: drive coverage
// from ~33% toward 80% by exercising the read paths of the largest
// uncovered admin/tester/public/AI-settings endpoints.
//
// These are NOT replacements for proper deep tests — they verify
// "endpoint responds 2xx for the right role" and "writes round-trip
// in the DB". Use them as a regression net; write deeper tests as new
// features land.
package httpapi

import (
	"context"
	"net/http"
	"testing"

	"mindstrata-stage1/api/internal/testsupport"
)

// ============================================================================
// PUBLIC / TESTER ENDPOINTS
// ============================================================================

func TestBreadth_PublicDemoModes_ReturnsList(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	// Seed a mode so the list isn't empty.
	_ = f.CreateMode(TestModeOpts{Name: "Public demo mode"})

	status, body := httpJSON(t, ts, "GET", "/api/public/demo-modes", nil)
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%v", status, body)
	}
}

func TestBreadth_TesterStatus_AsTester(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	tester := f.CreateUser(TestUserOpts{Role: "tester"})
	ts.LoginAs(f.CreateSession(tester.ID))

	status, body := httpJSON(t, ts, "GET", "/api/tester/status", nil)
	if status != http.StatusOK {
		t.Fatalf("status: %d body=%v", status, body)
	}
}

func TestBreadth_TesterUsers_AsSupport(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	support := f.CreateUser(TestUserOpts{Role: "support"})
	ts.LoginAs(f.CreateSession(support.ID))

	status, _ := httpJSON(t, ts, "GET", "/api/tester/users", nil)
	if status != http.StatusOK {
		t.Fatalf("support GET /api/tester/users: %d", status)
	}
}

func TestBreadth_TesterUserChatPreview(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	tester := f.CreateUser(TestUserOpts{Role: "tester"})
	target := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{})
	dialog := f.CreateDialog(target.ID, mode.ID)
	f.AppendMessage(dialog.ID, "user", "preview-test-message")

	ts.LoginAs(f.CreateSession(tester.ID))
	status, _ := httpJSON(t, ts, "GET", "/api/tester/user-chat-preview?userId="+itoa(target.ID), nil)
	if status != http.StatusOK {
		t.Fatalf("preview: %d", status)
	}
}

func TestBreadth_TesterRunCheck_RequiresTester(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	user := f.CreateUser(TestUserOpts{Role: "user"})
	ts.LoginAs(f.CreateSession(user.ID))

	status, _ := httpJSON(t, ts, "POST", "/api/tester/run-check",
		map[string]any{"checkType": "guardrail-prompt-append"})
	if status != http.StatusForbidden && status != http.StatusUnauthorized {
		t.Errorf("non-tester run-check: expected 403/401, got %d", status)
	}
}

// ============================================================================
// ADMIN SYSTEM / STATS / AI / PROMPTS
// ============================================================================

func TestBreadth_AdminStatus_AsAdmin(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	status, body := httpJSON(t, ts, "GET", "/api/admin/status", nil)
	if status != http.StatusOK {
		t.Fatalf("admin/status: %d body=%v", status, body)
	}
}

func TestBreadth_AdminStats_AsBillingAdmin(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	ba := f.CreateUser(TestUserOpts{Role: "billing_admin"})
	ts.LoginAs(f.CreateSession(ba.ID))

	status, _ := httpJSON(t, ts, "GET", "/api/admin/stats", nil)
	if status != http.StatusOK {
		t.Fatalf("billing_admin GET /api/admin/stats: %d", status)
	}
}

func TestBreadth_AdminAISettings_GetAndPatch(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	// GET
	status, _ := httpJSON(t, ts, "GET", "/api/admin/ai-settings", nil)
	if status != http.StatusOK {
		t.Fatalf("GET ai-settings: %d", status)
	}

	// POST update (the struct expects every field as a string; Ollama fields removed 2026-05-28).
	status2, _ := httpJSON(t, ts, "POST", "/api/admin/ai-settings", map[string]any{
		"summaryModel":              "openai/gpt-4o-mini",
		"summaryTemperature":        "0.3",
		"orchestrationModel":        "openai/gpt-4o-mini",
		"orchestrationTemperature":  "0.1",
		"orchestrationHistoryLimit": "12",
		"chatHistoryLimit":          "20",
	})
	if status2 != http.StatusOK {
		t.Fatalf("POST ai-settings: %d", status2)
	}
}

func TestBreadth_AdminOrchestrationPrompt_GetAndPost(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	status, _ := httpJSON(t, ts, "GET", "/api/admin/orchestration-prompt", nil)
	if status != http.StatusOK {
		t.Fatalf("GET orchestration-prompt: %d", status)
	}
	status2, _ := httpJSON(t, ts, "POST", "/api/admin/orchestration-prompt", map[string]any{
		"prompt": "Тестовый orchestration prompt.",
	})
	if status2 != http.StatusOK {
		t.Fatalf("POST orchestration-prompt: %d body", status2)
	}
}

func TestBreadth_AdminDialogSummaryPrompt_GetAndPost(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	status, _ := httpJSON(t, ts, "GET", "/api/admin/dialog-summary-prompt", nil)
	if status != http.StatusOK {
		t.Fatalf("GET dialog-summary-prompt: %d", status)
	}
	status2, _ := httpJSON(t, ts, "POST", "/api/admin/dialog-summary-prompt", map[string]any{
		"prompt": "Резюме теста",
	})
	if status2 != http.StatusOK {
		t.Fatalf("POST dialog-summary-prompt: %d", status2)
	}
}

// ============================================================================
// ADMIN USER DETAIL (GET) — list already covered, detail not
// ============================================================================

func TestBreadth_AdminUserDetail_GET(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	target := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{})
	f.GrantAccess(GrantAccessOpts{UserID: target.ID, ModeID: mode.ID, DailyMessageLimit: 50})

	ts.LoginAs(f.CreateSession(admin.ID))

	status, body := httpJSON(t, ts, "GET", "/api/admin/users/"+itoa(target.ID), nil)
	if status != http.StatusOK {
		t.Fatalf("user detail: %d body=%v", status, body)
	}
}

func TestBreadth_AdminUserAccess_GET(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	target := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{})
	f.GrantAccess(GrantAccessOpts{UserID: target.ID, ModeID: mode.ID, DailyMessageLimit: 100})

	ts.LoginAs(f.CreateSession(admin.ID))

	status, _ := httpJSON(t, ts, "GET", "/api/admin/users/"+itoa(target.ID)+"/access", nil)
	if status != http.StatusOK {
		t.Fatalf("user access detail: %d", status)
	}
}

// ============================================================================
// ADMIN MODE DETAIL (GET, PATCH)
// ============================================================================

func TestBreadth_AdminModeDetail_GET(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	mode := f.CreateMode(TestModeOpts{Name: "Detail target"})
	ts.LoginAs(f.CreateSession(admin.ID))

	status, _ := httpJSON(t, ts, "GET", "/api/admin/modes/"+itoa(mode.ID), nil)
	if status != http.StatusOK {
		t.Fatalf("mode detail: %d", status)
	}
}

func TestBreadth_AdminModePatch(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	mode := f.CreateMode(TestModeOpts{Name: "to-patch"})
	ts.LoginAs(f.CreateSession(admin.ID))

	status, _ := adminPatch(t, ts, "/api/admin/modes/"+itoa(mode.ID), map[string]any{
		"name":   "patched-name",
		"prompt": "patched prompt",
	})
	if status != http.StatusOK {
		t.Fatalf("mode patch: %d", status)
	}

	// DB verify
	var name string
	_ = env.Pool.QueryRow(context.Background(),
		`select name from modes where id = $1`, mode.ID).Scan(&name)
	if name != "patched-name" {
		t.Fatalf("name after patch: got %q want patched-name", name)
	}
}

// ============================================================================
// ADMIN PROMOCODES (LIST, DETAIL, CREATE)
// ============================================================================

func TestBreadth_AdminPromocodes_List(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	mode := f.CreateMode(TestModeOpts{})
	_ = f.CreatePromocode(TestPromocodeOpts{TargetID: mode.ID})
	ts.LoginAs(f.CreateSession(admin.ID))

	status, _ := httpJSON(t, ts, "GET", "/api/admin/promocodes", nil)
	if status != http.StatusOK {
		t.Fatalf("promocodes list: %d", status)
	}
}

// AdminPromocodeDetail supports PATCH and DELETE, not GET.
func TestBreadth_AdminPromocodeDetail_PatchAndDelete(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	mode := f.CreateMode(TestModeOpts{})
	promo := f.CreatePromocode(TestPromocodeOpts{TargetID: mode.ID})
	ts.LoginAs(f.CreateSession(admin.ID))

	// PATCH: extend activeTo (if the handler allows it).
	status, _ := adminPatch(t, ts, "/api/admin/promocodes/"+itoa(promo.ID), map[string]any{
		"maxUses": 999,
	})
	if status != http.StatusOK && status != http.StatusBadRequest {
		t.Errorf("PATCH promocode: got %d (expected 200 or 400)", status)
	}

	// DELETE: sets active_to=now(), does not delete physically.
	status2, _ := httpJSON(t, ts, "DELETE", "/api/admin/promocodes/"+itoa(promo.ID), nil)
	if status2 != http.StatusOK {
		t.Fatalf("DELETE promocode: %d", status2)
	}
}

// ============================================================================
// ADMIN TARIFFS (LIST, DETAIL)
// ============================================================================

func TestBreadth_AdminTariffs_List(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	status, _ := httpJSON(t, ts, "GET", "/api/admin/tariffs", nil)
	if status != http.StatusOK {
		t.Fatalf("tariffs list: %d", status)
	}
}

func TestBreadth_AdminTariffGroups_List(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	status, _ := httpJSON(t, ts, "GET", "/api/admin/tariff-groups", nil)
	if status != http.StatusOK {
		t.Fatalf("tariff-groups list: %d", status)
	}
}

// ============================================================================
// ADMIN BROADCASTS / SUMMARY-PROMPTS
// ============================================================================

// AdminBroadcasts accepts only POST.
func TestBreadth_AdminBroadcasts_POST(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	status, _ := httpJSON(t, ts, "POST", "/api/admin/broadcasts", map[string]any{
		"audience": "active_access",
		"message":  "Тестовая рассылка",
		"every":    "once",
	})
	// 200 ok or 400 if the payload is invalid for the current audience: the point is no 5xx.
	if status >= 500 {
		t.Fatalf("broadcasts 5xx: %d", status)
	}
}

func TestBreadth_AdminSummaryPrompts_GET(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	status, _ := httpJSON(t, ts, "GET", "/api/admin/summary-prompts", nil)
	if status != http.StatusOK {
		t.Fatalf("summary-prompts: %d", status)
	}
}

// ============================================================================
// ADMIN EXPORTS
// ============================================================================

func TestBreadth_AdminExports_RequiresAdmin(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	user := f.CreateUser(TestUserOpts{Role: "user"})
	ts.LoginAs(f.CreateSession(user.ID))

	status, _ := httpJSON(t, ts, "POST", "/api/admin/exports/messages",
		map[string]any{"format": "csv"})
	if status != http.StatusForbidden && status != http.StatusUnauthorized {
		t.Errorf("non-admin exports: expected 403/401, got %d", status)
	}
}

func TestBreadth_AdminExports_AsAdminContextOnly(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	// May return 400/200 depending on payload validation: the point is no 5xx.
	status, _ := httpJSON(t, ts, "POST", "/api/admin/exports/messages",
		map[string]any{"format": "csv"})
	if status >= 500 {
		t.Errorf("exports 5xx: %d", status)
	}
}

// ============================================================================
// ACCESS endpoint (status)
// ============================================================================

func TestBreadth_AccessStatus_Authenticated(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	user := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{})
	f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: mode.ID, DailyMessageLimit: 50})
	ts.LoginAs(f.CreateSession(user.ID))

	status, body := httpJSON(t, ts, "GET", "/api/access/status", nil)
	if status != http.StatusOK {
		t.Fatalf("access/status: %d body=%v", status, body)
	}
	if ha, _ := body["hasAccess"].(bool); !ha {
		t.Errorf("hasAccess=false despite granted access: %v", body)
	}
}

func TestBreadth_AccessStatus_NoAccess(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	user := f.CreateUser(TestUserOpts{})
	ts.LoginAs(f.CreateSession(user.ID))

	status, body := httpJSON(t, ts, "GET", "/api/access/status", nil)
	if status != http.StatusOK {
		t.Fatalf("access/status: %d", status)
	}
	if ha, _ := body["hasAccess"].(bool); ha {
		t.Errorf("hasAccess=true without grants: %v", body)
	}
}

// ============================================================================
// CHAT START / SELECT-MODE (basic chat init paths)
// ============================================================================

func TestBreadth_ChatStart_Authenticated(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	user := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{})
	f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: mode.ID, DailyMessageLimit: 50})
	ts.LoginAs(f.CreateSession(user.ID))

	status, _ := httpJSON(t, ts, "POST", "/api/chat/start", nil)
	if status != http.StatusOK {
		t.Fatalf("chat/start: %d", status)
	}
}

// ============================================================================
// PROFILE
// ============================================================================

func TestBreadth_Profile_RequiresAuth(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)

	status, _ := httpJSON(t, ts, "GET", "/api/profile", nil)
	if status != http.StatusUnauthorized {
		t.Errorf("profile without auth: expected 401, got %d", status)
	}
}

func TestBreadth_Profile_AsUser(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	user := f.CreateUser(TestUserOpts{})
	ts.LoginAs(f.CreateSession(user.ID))

	status, _ := httpJSON(t, ts, "GET", "/api/profile", nil)
	if status != http.StatusOK {
		t.Fatalf("profile: %d", status)
	}
}
