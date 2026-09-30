//go:build integration

package httpapi

import (
	"context"
	"net/http"
	"testing"

	"mindstrata-stage1/api/internal/testsupport"
)

// ============================================================================
// POST /api/admin/tariffs/{id}/set-modes-ai: bulk change of provider and
// model for every mode in the tariff.
// ============================================================================

// TestTariffSetModesAI_UpdatesAllTariffModes: the tariff's modes switch in
// one request, a foreign mode is untouched, the audit is written.
func TestTariffSetModesAI_UpdatesAllTariffModes(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	mode1 := f.CreateMode(TestModeOpts{})
	mode2 := f.CreateMode(TestModeOpts{})
	outsider := f.CreateMode(TestModeOpts{}) // not in the tariff: must stay as it was
	ts.LoginAs(f.CreateSession(admin.ID))

	_, body := httpJSON(t, ts, "POST", "/api/admin/tariffs", map[string]any{
		"name":    "BulkAI",
		"modeIds": []int64{mode1.ID, mode2.ID},
	})
	tid, _ := body["tariffId"].(float64)

	status, resp := httpJSON(t, ts, "POST", "/api/admin/tariffs/"+itoa(int64(tid))+"/set-modes-ai", map[string]any{
		"provider": "anthropic",
		"model":    "claude-haiku-4-5",
	})
	if status != http.StatusOK {
		t.Fatalf("set-modes-ai: %d body=%v", status, resp)
	}
	if updated, _ := resp["updated"].(float64); updated != 2 {
		t.Errorf("updated: got %v want 2", resp["updated"])
	}

	// DB: both tariff modes switched
	for _, modeID := range []int64{mode1.ID, mode2.ID} {
		var provider, model string
		err := env.Pool.QueryRow(context.Background(),
			`select coalesce(ai_provider, ''), ai_model from modes where id = $1`, modeID).Scan(&provider, &model)
		if err != nil {
			t.Fatalf("query mode %d: %v", modeID, err)
		}
		if provider != "anthropic" || model != "claude-haiku-4-5" {
			t.Errorf("mode %d: got %s/%s want anthropic/claude-haiku-4-5", modeID, provider, model)
		}
	}

	// DB: the mode outside the tariff is untouched
	var outModel string
	_ = env.Pool.QueryRow(context.Background(),
		`select ai_model from modes where id = $1`, outsider.ID).Scan(&outModel)
	if outModel != outsider.AIModel {
		t.Errorf("outsider mode changed: got %q want %q", outModel, outsider.AIModel)
	}

	// Audit
	var auditCount int64
	_ = env.Pool.QueryRow(context.Background(),
		`select count(*) from admin_audit_log where action = 'admin.tariff.set_modes_ai' and target_id = $1`,
		int64(tid)).Scan(&auditCount)
	if auditCount == 0 {
		t.Errorf("admin.tariff.set_modes_ai audit not written")
	}
}

// TestTariffSetModesAI_EmptyTariff_ZeroUpdated: tariff without modes → ok, updated=0.
func TestTariffSetModesAI_EmptyTariff_ZeroUpdated(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	_, body := httpJSON(t, ts, "POST", "/api/admin/tariffs", map[string]any{"name": "Empty"})
	tid, _ := body["tariffId"].(float64)

	status, resp := httpJSON(t, ts, "POST", "/api/admin/tariffs/"+itoa(int64(tid))+"/set-modes-ai", map[string]any{
		"provider": "gemini",
		"model":    "gemini-3.1-flash-lite",
	})
	if status != http.StatusOK {
		t.Fatalf("set-modes-ai: %d body=%v", status, resp)
	}
	if updated, _ := resp["updated"].(float64); updated != 0 {
		t.Errorf("updated: got %v want 0", resp["updated"])
	}
}

// TestTariffSetModesAI_UnknownProvider_400: a provider outside the list must
// not silently turn into vsegpt (bulk operation).
func TestTariffSetModesAI_UnknownProvider_400(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	mode := f.CreateMode(TestModeOpts{})
	ts.LoginAs(f.CreateSession(admin.ID))

	_, body := httpJSON(t, ts, "POST", "/api/admin/tariffs", map[string]any{
		"name": "BadProvider", "modeIds": []int64{mode.ID},
	})
	tid, _ := body["tariffId"].(float64)

	status, _ := httpJSON(t, ts, "POST", "/api/admin/tariffs/"+itoa(int64(tid))+"/set-modes-ai", map[string]any{
		"provider": "openai",
		"model":    "gpt-4o",
	})
	if status != http.StatusBadRequest {
		t.Errorf("expected 400 for unknown provider, got %d", status)
	}

	// The mode did not change
	var model string
	_ = env.Pool.QueryRow(context.Background(),
		`select ai_model from modes where id = $1`, mode.ID).Scan(&model)
	if model != mode.AIModel {
		t.Errorf("mode changed on rejected request: got %q", model)
	}
}

// TestTariffSetModesAI_InvalidModel_400: a model with spaces/empty → 400.
func TestTariffSetModesAI_InvalidModel_400(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	_, body := httpJSON(t, ts, "POST", "/api/admin/tariffs", map[string]any{"name": "BadModel"})
	tid, _ := body["tariffId"].(float64)

	for _, model := range []string{"", "модель с пробелами", "has space"} {
		status, _ := httpJSON(t, ts, "POST", "/api/admin/tariffs/"+itoa(int64(tid))+"/set-modes-ai", map[string]any{
			"provider": "anthropic",
			"model":    model,
		})
		if status != http.StatusBadRequest {
			t.Errorf("model %q: expected 400, got %d", model, status)
		}
	}
}

// TestTariffSetModesAI_TariffNotFound_404.
func TestTariffSetModesAI_TariffNotFound_404(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	status, _ := httpJSON(t, ts, "POST", "/api/admin/tariffs/999999/set-modes-ai", map[string]any{
		"provider": "anthropic",
		"model":    "claude-haiku-4-5",
	})
	if status != http.StatusNotFound {
		t.Errorf("expected 404, got %d", status)
	}
}

// TestTariffSetModesAI_GetMethod_405: the suffix accepts only POST.
func TestTariffSetModesAI_GetMethod_405(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	_, body := httpJSON(t, ts, "POST", "/api/admin/tariffs", map[string]any{"name": "MethodCheck"})
	tid, _ := body["tariffId"].(float64)

	status, _ := httpJSON(t, ts, "GET", "/api/admin/tariffs/"+itoa(int64(tid))+"/set-modes-ai", nil)
	if status != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", status)
	}
}

// TestTariffSetModesAI_BillingAdminForbidden: mutating the tariff section is
// forbidden for billing_admin (as is POST /api/admin/tariffs).
func TestTariffSetModesAI_BillingAdminForbidden(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	ba := f.CreateUser(TestUserOpts{Role: "billing_admin"})
	ts.LoginAs(f.CreateSession(ba.ID))

	status, _ := httpJSON(t, ts, "POST", "/api/admin/tariffs/1/set-modes-ai", map[string]any{
		"provider": "anthropic",
		"model":    "claude-haiku-4-5",
	})
	if status != http.StatusForbidden {
		t.Errorf("billing_admin: expected 403, got %d", status)
	}
}
