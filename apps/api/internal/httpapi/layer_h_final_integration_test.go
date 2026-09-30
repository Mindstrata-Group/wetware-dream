//go:build integration

package httpapi

import (
	"context"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"testing"
	"time"

	"mindstrata-stage1/api/internal/testsupport"
)

// =============================================================================
// SLAB H — final push to 80%: Mode CRUD + Access grant + PromoAdmin summary
// =============================================================================

// 1. AdminModeGuardrail POST simple guardrail set.
func TestAdminModeGuardrail_POST_SetsGuardrail(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	custom := "GUARDRAIL_TEXT_XYZ"
	status, body := httpJSON(t, ts, "POST", "/api/admin/modes/guardrail", map[string]any{
		"text": custom,
	})
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%v", status, body)
	}
	if ra, _ := body["runtimeAppend"].(bool); !ra {
		t.Errorf("runtimeAppend=false")
	}

	// DB
	var val string
	_ = env.Pool.QueryRow(context.Background(),
		`select value from system_settings where key='mode_guardrail_default'`).Scan(&val)
	if val != custom {
		t.Errorf("setting not persisted: %q", val)
	}
}

// 2. AdminModeGuardrail POST empty → 400.
func TestAdminModeGuardrail_POST_EmptyText_400(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	status, _ := httpJSON(t, ts, "POST", "/api/admin/modes/guardrail", map[string]any{
		"text": "",
	})
	if status != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", status)
	}
}

// 3. AdminModeGuardrail POST replaceAll missing find → 400.
func TestAdminModeGuardrail_ReplaceAll_NoFind_400(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	status, _ := httpJSON(t, ts, "POST", "/api/admin/modes/guardrail", map[string]any{
		"text":       "x",
		"replaceAll": true,
		"find":       "",
	})
	if status != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", status)
	}
}

func TestAdminModeGuardrail_ReplaceAll_RejectsPromptAsModel(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	mode := f.CreateMode(TestModeOpts{AIModel: "openai/gpt-old"})
	ts.LoginAs(f.CreateSession(admin.ID))

	status, body := httpJSON(t, ts, "POST", "/api/admin/modes/guardrail", map[string]any{
		"text":        "runtime guardrail",
		"replaceAll":  true,
		"find":        "openai/gpt-old",
		"replacement": "Работу я оцениваю на семь. Это текст ответа, а не id модели.",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%v", status, body)
	}

	var aiModel string
	_ = env.Pool.QueryRow(context.Background(),
		`select ai_model from modes where id=$1`, mode.ID).Scan(&aiModel)
	if aiModel != "openai/gpt-old" {
		t.Fatalf("ai_model changed to %q", aiModel)
	}
}

// 4. AdminModeGuardrail GET returns the current runtime block.
func TestAdminModeGuardrail_GET_ReturnsStoredGuardrail(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))
	const stored = "STORED_GUARDRAIL_TEXT"
	if _, err := env.Pool.Exec(context.Background(),
		`insert into system_settings (key, value) values ('mode_guardrail_default', $1)
		 on conflict (key) do update set value=excluded.value`, stored); err != nil {
		t.Fatalf("seed guardrail: %v", err)
	}

	status, body := httpJSON(t, ts, "GET", "/api/admin/modes/guardrail", nil)
	if status != http.StatusOK {
		t.Fatalf("GET: expected 200, got %d body=%v", status, body)
	}
	if body["text"] != stored {
		t.Errorf("text=%q want %q", body["text"], stored)
	}
}

// 5. AdminModeGuardrail non-admin → 403.
func TestAdminModeGuardrail_NonAdmin_403(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	user := f.CreateUser(TestUserOpts{Role: "user"})
	ts.LoginAs(f.CreateSession(user.ID))

	status, _ := httpJSON(t, ts, "POST", "/api/admin/modes/guardrail", map[string]any{"text": "x"})
	if status != http.StatusForbidden && status != http.StatusUnauthorized {
		t.Errorf("user: expected 403/401, got %d", status)
	}
}

// 6. AdminModeModelStats GET.
func TestAdminModeModelStats_GET(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	_ = f.CreateMode(TestModeOpts{Name: "StatsMode1"})
	_ = f.CreateMode(TestModeOpts{Name: "StatsMode2"})
	ts.LoginAs(f.CreateSession(admin.ID))

	status, body := httpJSON(t, ts, "GET", "/api/admin/modes/model-stats", nil)
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%v", status, body)
	}
	models, _ := body["models"].([]any)
	if len(models) == 0 {
		t.Errorf("expected non-empty models stats")
	}
}

// 7. AdminModeModelStats wrong method → 405.
func TestAdminModeModelStats_POST_405(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	status, _ := httpJSON(t, ts, "POST", "/api/admin/modes/model-stats", nil)
	if status != http.StatusMethodNotAllowed {
		t.Errorf("POST: expected 405, got %d", status)
	}
}

// =============================================================================
// AdminModeDetail — GET, PATCH, DELETE, detach-paid-tariffs
// =============================================================================

// 8. AdminModeDetail GET → full mode object.
func TestAdminModeDetail_GET(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	mode := f.CreateMode(TestModeOpts{Name: "DetailMode"})
	ts.LoginAs(f.CreateSession(admin.ID))

	status, body := httpJSON(t, ts, "GET", "/api/admin/modes/"+itoa(mode.ID), nil)
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%v", status, body)
	}
	m, _ := body["mode"].(map[string]any)
	if m == nil {
		t.Fatalf("no mode")
	}
	if name, _ := m["name"].(string); name != "DetailMode" {
		t.Errorf("name: %q", name)
	}
}

// 9. AdminModeDetail GET 404.
func TestAdminModeDetail_GET_NotFound(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	status, _ := httpJSON(t, ts, "GET", "/api/admin/modes/999999999", nil)
	if status != http.StatusNotFound {
		t.Errorf("expected 404, got %d", status)
	}
}

// 10. AdminModeDetail invalid id → 400.
func TestAdminModeDetail_InvalidID_400(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	status, _ := httpJSON(t, ts, "GET", "/api/admin/modes/abc", nil)
	if status != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", status)
	}
}

// 11. AdminModeDetail PATCH name + criteria.
func TestAdminModeDetail_PATCH(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	mode := f.CreateMode(TestModeOpts{Name: "PatchModeOriginal"})
	ts.LoginAs(f.CreateSession(admin.ID))

	status, _ := adminPatch(t, ts, "/api/admin/modes/"+itoa(mode.ID), map[string]any{
		"name":     "PatchModeUpdated",
		"criteria": "new criteria",
	})
	if status != http.StatusOK {
		t.Fatalf("patch: %d", status)
	}

	var name, criteria string
	_ = env.Pool.QueryRow(context.Background(),
		`select name, coalesce(criteria,'') from modes where id=$1`, mode.ID).Scan(&name, &criteria)
	if name != "PatchModeUpdated" {
		t.Errorf("name not updated: %q", name)
	}
	if criteria != "new criteria" {
		t.Errorf("criteria not updated: %q", criteria)
	}
}

// 12. AdminModeDetail DELETE → row removed.
func TestAdminModeDetail_DELETE(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	mode := f.CreateMode(TestModeOpts{Name: "DeleteMe"})
	ts.LoginAs(f.CreateSession(admin.ID))

	status, body := httpJSON(t, ts, "DELETE", "/api/admin/modes/"+itoa(mode.ID), nil)
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%v", status, body)
	}
	if name, _ := body["name"].(string); name != "DeleteMe" {
		t.Errorf("name in response: %q", name)
	}

	var cnt int64
	_ = env.Pool.QueryRow(context.Background(),
		`select count(*) from modes where id=$1`, mode.ID).Scan(&cnt)
	if cnt != 0 {
		t.Errorf("mode not deleted, count=%d", cnt)
	}
}

// 13. AdminModeDetail DELETE missing → 400.
func TestAdminModeDetail_DELETE_NotFound(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	status, _ := httpJSON(t, ts, "DELETE", "/api/admin/modes/999999999", nil)
	if status != http.StatusNotFound && status != http.StatusBadRequest {
		t.Errorf("DELETE missing: expected 404/400, got %d", status)
	}
}

// 14. AdminModeDetail/detach-paid-tariffs POST.
func TestAdminModeDetail_DetachPaidTariffs(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	mode := f.CreateMode(TestModeOpts{})

	// Create paid tariff with this mode
	var tariffID int64
	_ = env.Pool.QueryRow(context.Background(),
		`insert into tariffs (name, monthly_price, limit_type, available_for_subscription) values ('PaidT', 100.0, 'shared', true) returning id`).Scan(&tariffID)
	_, _ = env.Pool.Exec(context.Background(),
		`insert into tariff_mode (tariff_id, mode_id) values ($1, $2)`, tariffID, mode.ID)

	ts.LoginAs(f.CreateSession(admin.ID))
	status, body := httpJSON(t, ts, "POST", "/api/admin/modes/"+itoa(mode.ID)+"/detach-paid-tariffs", nil)
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%v", status, body)
	}
	removed, _ := body["removed"].(float64)
	if removed < 1 {
		t.Errorf("expected ≥1 detached, got %v", removed)
	}
}

// 15. AdminModeDetail/detach-paid-tariffs GET → 405.
func TestAdminModeDetail_DetachPaidTariffs_GET_405(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	mode := f.CreateMode(TestModeOpts{})
	ts.LoginAs(f.CreateSession(admin.ID))

	status, _ := httpJSON(t, ts, "GET", "/api/admin/modes/"+itoa(mode.ID)+"/detach-paid-tariffs", nil)
	if status != http.StatusMethodNotAllowed {
		t.Errorf("GET: expected 405, got %d", status)
	}
}

// =============================================================================
// AdminAccess (POST grant) — full flow
// =============================================================================

// 16. AdminAccess POST grant by modeId.
func TestAdminAccess_POST_GrantByModeID(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	target := f.CreateUser(TestUserOpts{Role: "user"})
	mode := f.CreateMode(TestModeOpts{})
	ts.LoginAs(f.CreateSession(admin.ID))

	dailyLimit := int64(50)
	status, body := httpJSON(t, ts, "POST", "/api/admin/access", map[string]any{
		"userId":            target.ID,
		"modeId":            mode.ID,
		"days":              30,
		"dailyMessageLimit": dailyLimit,
	})
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%v", status, body)
	}
	granted, _ := body["granted"].(float64)
	if granted < 1 {
		t.Errorf("granted=%v want ≥1", granted)
	}
}

// 17. AdminAccess POST grant by tariffId expands to modes.
func TestAdminAccess_POST_GrantByTariffID(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	target := f.CreateUser(TestUserOpts{Role: "user"})
	modeA := f.CreateMode(TestModeOpts{})
	modeB := f.CreateMode(TestModeOpts{})
	var tariffID int64
	_ = env.Pool.QueryRow(context.Background(),
		`insert into tariffs (name, monthly_price, limit_type) values ('TariffH', 0, 'shared') returning id`).Scan(&tariffID)
	_, _ = env.Pool.Exec(context.Background(),
		`insert into tariff_mode (tariff_id, mode_id) values ($1, $2), ($1, $3)`, tariffID, modeA.ID, modeB.ID)
	ts.LoginAs(f.CreateSession(admin.ID))

	status, body := httpJSON(t, ts, "POST", "/api/admin/access", map[string]any{
		"userId":   target.ID,
		"tariffId": tariffID,
		"days":     30,
	})
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%v", status, body)
	}
	granted, _ := body["granted"].(float64)
	if granted < 2 {
		t.Errorf("expected 2 grants (both modes), got %v", granted)
	}
}

// 18. AdminAccess POST without mode/tariff → 400.
func TestAdminAccess_POST_NoModes_400(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	target := f.CreateUser(TestUserOpts{Role: "user"})
	ts.LoginAs(f.CreateSession(admin.ID))

	status, _ := httpJSON(t, ts, "POST", "/api/admin/access", map[string]any{
		"userId": target.ID,
		"days":   30,
	})
	if status != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", status)
	}
}

// 19. AdminAccess POST resolves by email.
func TestAdminAccess_POST_ResolveByEmail(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	target := f.CreateUser(TestUserOpts{Role: "user"})
	mode := f.CreateMode(TestModeOpts{})
	ts.LoginAs(f.CreateSession(admin.ID))

	status, _ := httpJSON(t, ts, "POST", "/api/admin/access", map[string]any{
		"email":  target.Email,
		"modeId": mode.ID,
		"days":   30,
	})
	if status != http.StatusOK {
		t.Errorf("status=%d", status)
	}
}

// 20. AdminAccess POST activeTo override.
func TestAdminAccess_POST_ActiveTo(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	target := f.CreateUser(TestUserOpts{Role: "user"})
	mode := f.CreateMode(TestModeOpts{})
	ts.LoginAs(f.CreateSession(admin.ID))

	status, _ := httpJSON(t, ts, "POST", "/api/admin/access", map[string]any{
		"userId":   target.ID,
		"modeId":   mode.ID,
		"activeTo": "2099-12-31T00:00:00Z",
	})
	if status != http.StatusOK {
		t.Errorf("status=%d", status)
	}
}

// 21. AdminAccess POST activeTo invalid → 400.
func TestAdminAccess_POST_InvalidActiveTo_400(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	target := f.CreateUser(TestUserOpts{Role: "user"})
	mode := f.CreateMode(TestModeOpts{})
	ts.LoginAs(f.CreateSession(admin.ID))

	status, _ := httpJSON(t, ts, "POST", "/api/admin/access", map[string]any{
		"userId":   target.ID,
		"modeId":   mode.ID,
		"activeTo": "not-a-date",
	})
	if status != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", status)
	}
}

// 22. AdminAccess POST extend existing grant.
func TestAdminAccess_POST_ExtendsExisting(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	target := f.CreateUser(TestUserOpts{Role: "user"})
	mode := f.CreateMode(TestModeOpts{})
	ts.LoginAs(f.CreateSession(admin.ID))

	// First grant
	_, _ = httpJSON(t, ts, "POST", "/api/admin/access", map[string]any{
		"userId": target.ID, "modeId": mode.ID, "days": 30,
	})

	// Second call → should extend
	status, body := httpJSON(t, ts, "POST", "/api/admin/access", map[string]any{
		"userId": target.ID, "modeId": mode.ID, "days": 60,
	})
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%v", status, body)
	}
	extended, _ := body["extended"].(float64)
	if extended < 1 {
		t.Errorf("expected extended=1, got %v", extended)
	}
}

// 23. AdminAccess POST wrong method → 405.
func TestAdminAccess_GET_405(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	status, _ := httpJSON(t, ts, "GET", "/api/admin/access", nil)
	if status != http.StatusMethodNotAllowed {
		t.Errorf("GET: expected 405, got %d", status)
	}
}

// 24. AdminAccess POST resetLimits flow.
func TestAdminAccess_POST_ResetLimits(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	target := f.CreateUser(TestUserOpts{Role: "user"})
	mode := f.CreateMode(TestModeOpts{})
	f.GrantAccess(GrantAccessOpts{UserID: target.ID, ModeID: mode.ID, DailyMessageLimit: 50})
	ts.LoginAs(f.CreateSession(admin.ID))

	status, body := httpJSON(t, ts, "POST", "/api/admin/access", map[string]any{
		"userId":      target.ID,
		"modeId":      mode.ID,
		"resetLimits": true,
	})
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%v", status, body)
	}
	if lr, _ := body["limitsReset"].(bool); !lr {
		t.Errorf("limitsReset=false")
	}
}

// =============================================================================
// PromoAdminSummarize: full happy flow with mock VseGPT
// =============================================================================

// 25. PromoAdminSummarize: happy path with promptID and mock AI.
func TestPromoAdminSummarize_HappyPath(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)

	mode := f.CreateMode(TestModeOpts{})
	code := "SUMM_" + itoa(int64(env.Pool.Stat().AcquireCount()))
	promo := f.CreatePromocode(TestPromocodeOpts{Code: code, TargetID: mode.ID})

	// Create prompt
	var promptID int64
	_ = env.Pool.QueryRow(context.Background(),
		`insert into admin_summary_prompts (name, prompt, is_default) values ('SummPrompt', 'system context', false) returning id`).Scan(&promptID)

	// Mock VseGPT
	fake := fakeVseGPT(t, 200, "promo-summary-result")

	// Build TestServer with VseGPT live + key set
	handler := Handler{
		DB:            env.Pool,
		OpenAIAPIKey:  "test",
		OpenAIBaseURL: "https://api.vsegpt.ru/v1",
		HTTPClient:    &http.Client{Transport: vsegptRT{target: fake.URL}},
	}
	ts := buildTestServer(t, env, handler)

	// Add a message tied to the promo (via promocode_usage).
	user := f.CreateUser(TestUserOpts{})
	dlg := f.CreateDialog(user.ID, mode.ID)
	_, _ = env.Pool.Exec(context.Background(),
		`insert into promocode_usages (user_id, promocode_id, used_at) values ($1, $2, now() - interval '1 minute')`,
		user.ID, promo.ID)
	_ = f.AppendMessage(dlg.ID, "user", "sample user message")

	key := promoAdminTokenForCode(handler, code)
	status, body := httpJSON(t, ts, "POST", "/api/promo-admin/summarize?key="+key, map[string]any{
		"code":     code,
		"promptId": promptID,
		"modeIds":  []int64{mode.ID},
	})
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%v", status, body)
	}
	if res, _ := body["result"].(string); res != "promo-summary-result" {
		t.Errorf("result: %q", res)
	}
	if sid, _ := body["summaryId"].(float64); sid == 0 {
		t.Errorf("missing summaryId")
	}
}

// 26. PromoAdminSummarize: limit exhausted → 403.
func TestPromoAdminSummarize_LimitExhausted_403(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	mode := f.CreateMode(TestModeOpts{})
	code := "EXH_" + itoa(int64(env.Pool.Stat().AcquireCount()))
	// Promo with summary_limit=0
	var promoID int64
	_ = env.Pool.QueryRow(context.Background(),
		`insert into promocodes (code, max_uses, used_count, duration, access_priority, grants_type, target_id, limit_type, daily_message_limit, summary_limit) values ($1, 0, 0, '30 days'::interval, 0, 'mode', $2, 'fixed', 50, 0) returning id`,
		code, mode.ID).Scan(&promoID)
	_ = promoID

	// Create a prompt to pass past prompt-check.
	var promptID int64
	_ = env.Pool.QueryRow(context.Background(),
		`insert into admin_summary_prompts (name, prompt, is_default) values ('LimExhPrompt', 'p', false) returning id`).Scan(&promptID)

	key := promoAdminTokenForCode(ts.Handler, code)
	status, body := httpJSON(t, ts, "POST", "/api/promo-admin/summarize?key="+key, map[string]any{
		"code":     code,
		"promptId": promptID,
		"modeIds":  []int64{mode.ID},
	})
	if status != http.StatusForbidden {
		t.Errorf("expected 403 (limit exhausted), got %d body=%v", status, body)
	}
}

// =============================================================================
// queryAdminExportPromocodes via /api/admin/exports?options=true
// (boost coverage of options branch)
// =============================================================================

// 27. AdminExportsOptions populates promocodes list.
func TestAdminExports_OptionsPromocodesListed(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	mode := f.CreateMode(TestModeOpts{})
	promo := f.CreatePromocode(TestPromocodeOpts{TargetID: mode.ID})
	_ = promo
	ts.LoginAs(f.CreateSession(admin.ID))

	// Clear cache so we hit the live query
	clearAdminExportOptionsCache()
	status, body := httpJSON(t, ts, "GET", "/api/admin/exports/messages?options=true", nil)
	if status != http.StatusOK {
		t.Fatalf("status=%d", status)
	}
	if _, ok := body["promocodes"]; !ok {
		t.Errorf("promocodes field missing: keys=%v", mapKeys(body))
	}
}

// =============================================================================
// withCORS middleware: OPTIONS short-circuits
// =============================================================================

// 28. withCORS OPTIONS → 204.
func TestWithCORS_Options204(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)
	_ = env

	req, _ := http.NewRequest("OPTIONS", ts.URL("/health"), nil)
	req.Header.Set("Origin", "http://example.com")
	resp, err := ts.Client.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		t.Errorf("OPTIONS: status %d (expected 204/200)", resp.StatusCode)
	}
}

// =============================================================================
// helpers
// =============================================================================

func mapKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// buildTestServer wraps a custom Handler into httptest server with cookie jar.
func buildTestServer(t *testing.T, env *testsupport.Env, handler Handler) *TestServer {
	t.Helper()
	if handler.DB == nil {
		handler.DB = env.Pool
	}
	if handler.HTTPClient == nil {
		handler.HTTPClient = &http.Client{Timeout: 5 * time.Second}
	}
	if handler.PromoAdminSecret == "" {
		handler.PromoAdminSecret = "test-promo-admin-secret" // S-NEW-2: explicit secret
	}
	if handler.c == nil {
		handler.c = newHandlerCaches()
	}
	mux := NewRouter(handler, []string{"*"})
	srv := httptest.NewServer(mux)
	jar, _ := cookiejar.New(nil)
	t.Cleanup(srv.Close)
	return &TestServer{
		t: t, server: srv, Handler: handler,
		Client: &http.Client{
			Timeout: 10 * time.Second,
			Jar:     jar,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}
