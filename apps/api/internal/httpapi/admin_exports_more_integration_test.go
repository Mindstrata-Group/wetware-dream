//go:build integration

package httpapi

import (
	"bytes"
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"mindstrata-stage1/api/internal/testsupport"
)

// Integration tests that fill the gaps in admin_exports_integration_test.go.
// They cover collectAdminExport through the AdminExports HTTP handler.

// TestAdminExports_GET_DateRangeIncludes: dateFrom/dateTo catch fresh messages.
func TestAdminExports_GET_DateRangeIncludes(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	user := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{})
	dialog := f.CreateDialog(user.ID, mode.ID)
	_ = f.AppendMessage(dialog.ID, "user", "today-message")

	ts.LoginAs(f.CreateSession(admin.ID))

	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	tomorrow := time.Now().AddDate(0, 0, 1).Format("2006-01-02")
	status, body := httpJSON(t, ts, "GET",
		"/api/admin/exports/messages?dateFrom="+yesterday+"&dateTo="+tomorrow+"&limit=100", nil)
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%v", status, body)
	}
	msgs, _ := body["messages"].([]any)
	if len(msgs) == 0 {
		t.Errorf("expected today message inside [%s, %s], got 0", yesterday, tomorrow)
	}
}

// TestAdminExports_GET_DateRangeExcludes: dateTo in the past → nothing.
func TestAdminExports_GET_DateRangeExcludes(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	user := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{})
	dialog := f.CreateDialog(user.ID, mode.ID)
	_ = f.AppendMessage(dialog.ID, "user", "current-message")

	ts.LoginAs(f.CreateSession(admin.ID))

	// The window is entirely in the past: the current message will not match.
	status, body := httpJSON(t, ts, "GET",
		"/api/admin/exports/messages?dateFrom=2020-01-01&dateTo=2020-01-02&limit=100", nil)
	if status != http.StatusOK {
		t.Fatalf("status=%d", status)
	}
	cnt, _ := body["messageCount"].(float64)
	if cnt != 0 {
		t.Errorf("date window [2020,2020] should match nothing, got %v", cnt)
	}
}

// TestAdminExports_GET_UserIDFilter: filtering by userIds excludes other users.
func TestAdminExports_GET_UserIDFilter(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	target := f.CreateUser(TestUserOpts{})
	other := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{})
	tdlg := f.CreateDialog(target.ID, mode.ID)
	odlg := f.CreateDialog(other.ID, mode.ID)
	_ = f.AppendMessage(tdlg.ID, "user", "TARGET_USER_MSG_xyz")
	_ = f.AppendMessage(odlg.ID, "user", "OTHER_USER_MSG_xyz")

	ts.LoginAs(f.CreateSession(admin.ID))

	status, body := httpJSON(t, ts, "GET",
		"/api/admin/exports/messages?userIds="+itoa(target.ID), nil)
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%v", status, body)
	}
	// Map by user_id from items
	msgs, _ := body["messages"].([]any)
	for _, m := range msgs {
		row, _ := m.(map[string]any)
		uid, _ := row["userId"].(float64)
		if int64(uid) != target.ID {
			t.Errorf("leaked message from user %d (filter was %d)", int64(uid), target.ID)
		}
	}
}

// TestAdminExports_GET_LimitTruncates.
func TestAdminExports_GET_LimitTruncates(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	user := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{})
	dialog := f.CreateDialog(user.ID, mode.ID)
	for i := 0; i < 7; i++ {
		_ = f.AppendMessage(dialog.ID, "user", "msg")
	}

	ts.LoginAs(f.CreateSession(admin.ID))

	status, body := httpJSON(t, ts, "GET",
		"/api/admin/exports/messages?limit=3&userIds="+itoa(user.ID), nil)
	if status != http.StatusOK {
		t.Fatalf("status=%d", status)
	}
	cnt, _ := body["messageCount"].(float64)
	if cnt != 3 {
		t.Errorf("limit=3 produced messageCount=%v want 3", cnt)
	}
}

// TestAdminExports_GET_RoleAssistantFiltersUserOut.
func TestAdminExports_GET_RoleAssistantFiltersUserOut(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	user := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{})
	dialog := f.CreateDialog(user.ID, mode.ID)
	_ = f.AppendMessage(dialog.ID, "user", "user-side")
	_ = f.AppendMessage(dialog.ID, "assistant", "assistant-side")

	ts.LoginAs(f.CreateSession(admin.ID))

	status, body := httpJSON(t, ts, "GET",
		"/api/admin/exports/messages?roleFilter=assistant&userIds="+itoa(user.ID), nil)
	if status != http.StatusOK {
		t.Fatalf("status=%d", status)
	}
	msgs, _ := body["messages"].([]any)
	for _, m := range msgs {
		row, _ := m.(map[string]any)
		role, _ := row["role"].(string)
		if role != "assistant" {
			t.Errorf("leaked role=%q with roleFilter=assistant", role)
		}
	}
}

// TestAdminExports_GET_WithSummaryProducesPayload: withSummary=true → summaryPayload present.
func TestAdminExports_GET_WithSummaryProducesPayload(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	user := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{})
	dialog := f.CreateDialog(user.ID, mode.ID)
	_ = f.AppendMessage(dialog.ID, "user", "ABCDEF_payload_marker")

	ts.LoginAs(f.CreateSession(admin.ID))

	status, body := httpJSON(t, ts, "GET",
		"/api/admin/exports/messages?withSummary=true&userIds="+itoa(user.ID), nil)
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%v", status, body)
	}
	payload, _ := body["summaryPayload"].(string)
	if payload == "" {
		t.Fatalf("withSummary=true did not return summaryPayload")
	}
	if !strings.Contains(payload, "ABCDEF_payload_marker") {
		t.Errorf("summaryPayload missing message content: %s", payload[:min(200, len(payload))])
	}
	// the default prompt must be present (if summaryPrompt is not set)
	if !strings.Contains(payload, "Задача") {
		t.Errorf("default prompt missing")
	}
}

// TestAdminExports_GET_WithSummaryCustomPrompt: a custom summaryPrompt is used.
func TestAdminExports_GET_WithSummaryCustomPrompt(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	user := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{})
	dialog := f.CreateDialog(user.ID, mode.ID)
	_ = f.AppendMessage(dialog.ID, "user", "x")

	ts.LoginAs(f.CreateSession(admin.ID))

	customPrompt := "MY_CUSTOM_SUMMARY_PROMPT_ABCDEF"
	status, body := httpJSON(t, ts, "GET",
		"/api/admin/exports/messages?withSummary=true&summaryPrompt="+customPrompt+"&userIds="+itoa(user.ID), nil)
	if status != http.StatusOK {
		t.Fatalf("status=%d", status)
	}
	payload, _ := body["summaryPayload"].(string)
	if !strings.Contains(payload, customPrompt) {
		t.Errorf("custom summaryPrompt not echoed in payload")
	}
}

// TestAdminExports_GET_ApproxTokensIsPositive.
func TestAdminExports_GET_ApproxTokensIsPositive(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	user := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{})
	dialog := f.CreateDialog(user.ID, mode.ID)
	_ = f.AppendMessage(dialog.ID, "user", strings.Repeat("token ", 50))

	ts.LoginAs(f.CreateSession(admin.ID))

	status, body := httpJSON(t, ts, "GET",
		"/api/admin/exports/messages?userIds="+itoa(user.ID), nil)
	if status != http.StatusOK {
		t.Fatalf("status=%d", status)
	}
	tokens, _ := body["approxTokens"].(float64)
	if tokens == 0 {
		t.Errorf("approxTokens should be > 0 with 300-char content, got 0")
	}
	bytesField, _ := body["sourceBytes"].(float64)
	if bytesField == 0 {
		t.Errorf("sourceBytes=0 with seeded content")
	}
}

// TestAdminExports_POST_MalformedJSON: invalid JSON → 400.
func TestAdminExports_POST_MalformedJSON(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	// Not using httpJSON so we can send a raw body, but using ts.Client (with jar+cookie).
	req, _ := http.NewRequest("POST", ts.URL("/api/admin/exports/messages"),
		bytes.NewBufferString("{not-json:::"))
	req.Header.Set("Content-Type", "application/json")
	resp, err := ts.Client.Do(req)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("malformed POST: expected 400, got %d", resp.StatusCode)
	}
}

// TestAdminExports_POST_LimitClampedToZeroOnOversized.
func TestAdminExports_POST_LimitClampedToZeroOnOversized(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	// limit=9999999 → must reset to 0 (no limit) via adminExportLimitInt
	// → the request still returns no 5xx.
	status, body := httpJSON(t, ts, "POST", "/api/admin/exports/messages", map[string]any{
		"limit": 9999999,
	})
	if status >= 500 {
		t.Errorf("oversized limit produced 5xx: %d body=%v", status, body)
	}
}

// TestAdminExports_GET_UnsupportedMethod.
func TestAdminExports_GET_UnsupportedMethod(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	status, _ := httpJSON(t, ts, "DELETE", "/api/admin/exports/messages", nil)
	if status != http.StatusMethodNotAllowed && status != http.StatusForbidden {
		t.Errorf("DELETE: expected 405/403, got %d", status)
	}
}

// TestCollectAdminExport_PromoIDFilter:
// checked directly: create a promo code + usage, filter by promoIDs,
// get only the messages after used_at.
func TestCollectAdminExport_PromoIDFilter(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	h := Handler{DB: env.Pool}

	user := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{})
	promo := f.CreatePromocode(TestPromocodeOpts{
		GrantsType: "mode",
		TargetID:   mode.ID,
	})

	dialog := f.CreateDialog(user.ID, mode.ID)
	_ = f.AppendMessage(dialog.ID, "user", "PROMO_MARKER_after")

	// Record a promo code usage now → the message above is already after used_at.
	_, err := env.Pool.Exec(context.Background(),
		`insert into promocode_usages (user_id, promocode_id, used_at) values ($1, $2, now() - interval '1 minute')`,
		user.ID, promo.ID)
	if err != nil {
		t.Fatalf("insert promo usage: %v", err)
	}

	items, exportText, err := h.collectAdminExport(context.Background(),
		nil, nil, []int64{promo.ID}, "all", 100, "", "")
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if len(items) == 0 {
		t.Fatalf("expected at least 1 message tied to promo")
	}
	if !strings.Contains(exportText, "PROMO_MARKER_after") {
		t.Errorf("exportText missing PROMO_MARKER_after")
	}
}

// TestCollectAdminExport_PromoIDFilter_BeforeUsageExcluded:
// a message OLDER than used_at must not end up in the export.
func TestCollectAdminExport_PromoIDFilter_BeforeUsageExcluded(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	h := Handler{DB: env.Pool}

	user := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{})
	promo := f.CreatePromocode(TestPromocodeOpts{GrantsType: "mode", TargetID: mode.ID})

	dialog := f.CreateDialog(user.ID, mode.ID)
	// Old message (created_at one hour back)
	_, err := env.Pool.Exec(context.Background(),
		`insert into dialogs_messages (dialog_id, role, content, created_at) values ($1, 'user', 'OLD_MARKER', now() - interval '1 hour')`,
		dialog.ID)
	if err != nil {
		t.Fatalf("seed old msg: %v", err)
	}

	// Usage now → the old message must be filtered out.
	_, err = env.Pool.Exec(context.Background(),
		`insert into promocode_usages (user_id, promocode_id, used_at) values ($1, $2, now())`,
		user.ID, promo.ID)
	if err != nil {
		t.Fatalf("insert promo usage: %v", err)
	}

	_, exportText, err := h.collectAdminExport(context.Background(),
		nil, nil, []int64{promo.ID}, "all", 100, "", "")
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if strings.Contains(exportText, "OLD_MARKER") {
		t.Errorf("OLD_MARKER leaked despite being before used_at: %s", exportText)
	}
}

func TestCollectAdminExport_ModeFilterUsesMessageAttributionAfterDialogSwitch(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	h := Handler{DB: env.Pool}

	user := f.CreateUser(TestUserOpts{})
	originalMode := f.CreateMode(TestModeOpts{Name: "Original attributed mode"})
	currentDialogMode := f.CreateMode(TestModeOpts{Name: "Current dialog mode"})
	f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: originalMode.ID, DailyMessageLimit: 50})
	dialog := f.CreateDialog(user.ID, originalMode.ID)
	messageID := f.AppendMessage(dialog.ID, "user", "ATTRIBUTED_MODE_MARKER")

	var accessID int64
	if err := env.Pool.QueryRow(context.Background(), `
		select id
		from user_mode_access
		where user_id = $1 and mode_id = $2
		order by id desc
		limit 1`, user.ID, originalMode.ID).Scan(&accessID); err != nil {
		t.Fatalf("query access id: %v", err)
	}
	if _, err := env.Pool.Exec(context.Background(), `
		insert into dialog_message_access_usage (user_id, mode_id, dialog_message_id, access_id, usage_date, usage_kind)
		values ($1, $2, $3, $4, current_date, 'message')`,
		user.ID, originalMode.ID, messageID, accessID); err != nil {
		t.Fatalf("insert attribution: %v", err)
	}
	if _, err := env.Pool.Exec(context.Background(), `update users_dialogs set mode_id = $2 where id = $1`, dialog.ID, currentDialogMode.ID); err != nil {
		t.Fatalf("switch dialog mode: %v", err)
	}

	items, exportText, err := h.collectAdminExport(context.Background(),
		[]int64{originalMode.ID}, []int64{user.ID}, nil, "all", 100, "", "")
	if err != nil {
		t.Fatalf("collect original mode: %v", err)
	}
	if len(items) != 1 || !strings.Contains(exportText, "ATTRIBUTED_MODE_MARKER") {
		t.Fatalf("attributed original mode export mismatch: items=%v text=%q", items, exportText)
	}
	if got, _ := items[0]["modeId"].(int64); got != originalMode.ID {
		t.Fatalf("export modeId=%d, want attributed %d", got, originalMode.ID)
	}
	if got, _ := items[0]["modeName"].(string); got != originalMode.Name {
		t.Fatalf("export modeName=%q, want %q", got, originalMode.Name)
	}

	items, exportText, err = h.collectAdminExport(context.Background(),
		[]int64{currentDialogMode.ID}, []int64{user.ID}, nil, "all", 100, "", "")
	if err != nil {
		t.Fatalf("collect current dialog mode: %v", err)
	}
	if len(items) != 0 || strings.Contains(exportText, "ATTRIBUTED_MODE_MARKER") {
		t.Fatalf("message leaked through current dialog mode filter: items=%v text=%q", items, exportText)
	}
}

func TestCollectAdminExport_PromoFilterReturnsAttributedModeAfterDialogSwitch(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	h := Handler{DB: env.Pool}

	user := f.CreateUser(TestUserOpts{})
	promoMode := f.CreateMode(TestModeOpts{Name: "Promo attributed mode"})
	currentDialogMode := f.CreateMode(TestModeOpts{Name: "Current dialog mode"})
	promo := f.CreatePromocode(TestPromocodeOpts{GrantsType: "mode", TargetID: promoMode.ID})
	sourceID := promo.ID
	f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: promoMode.ID, DailyMessageLimit: 50, AccessType: "promocode", SourceID: &sourceID})
	dialog := f.CreateDialog(user.ID, promoMode.ID)
	messageID := f.AppendMessage(dialog.ID, "assistant", "ATTRIBUTED_PROMO_MARKER")

	var accessID int64
	if err := env.Pool.QueryRow(context.Background(), `
		select id
		from user_mode_access
		where user_id = $1 and mode_id = $2 and access_type = 'promocode' and source_id = $3
		order by id desc
		limit 1`, user.ID, promoMode.ID, promo.ID).Scan(&accessID); err != nil {
		t.Fatalf("query promo access id: %v", err)
	}
	if _, err := env.Pool.Exec(context.Background(), `
		insert into dialog_message_access_usage (user_id, mode_id, dialog_message_id, access_id, usage_date, usage_kind)
		values ($1, $2, $3, $4, current_date, 'message')`,
		user.ID, promoMode.ID, messageID, accessID); err != nil {
		t.Fatalf("insert attribution: %v", err)
	}
	if _, err := env.Pool.Exec(context.Background(), `
		insert into promocode_usages (user_id, promocode_id, used_at)
		values ($1, $2, now() - interval '1 minute')`, user.ID, promo.ID); err != nil {
		t.Fatalf("insert promo usage: %v", err)
	}
	if _, err := env.Pool.Exec(context.Background(), `update users_dialogs set mode_id = $2 where id = $1`, dialog.ID, currentDialogMode.ID); err != nil {
		t.Fatalf("switch dialog mode: %v", err)
	}

	items, exportText, err := h.collectAdminExport(context.Background(),
		nil, []int64{user.ID}, []int64{promo.ID}, "all", 100, "", "")
	if err != nil {
		t.Fatalf("collect promo: %v", err)
	}
	if len(items) != 1 || !strings.Contains(exportText, "ATTRIBUTED_PROMO_MARKER") {
		t.Fatalf("promo export mismatch: items=%v text=%q", items, exportText)
	}
	if got, _ := items[0]["modeId"].(int64); got != promoMode.ID {
		t.Fatalf("promo export modeId=%d, want attributed %d", got, promoMode.ID)
	}
	if got, _ := items[0]["modeName"].(string); got != promoMode.Name {
		t.Fatalf("promo export modeName=%q, want %q", got, promoMode.Name)
	}
}

// TestCollectAdminExport_ZeroLimitMeansNoLimit:
// limit=0 → collects everything (no LIMIT clause).
func TestCollectAdminExport_ZeroLimitMeansNoLimit(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	h := Handler{DB: env.Pool}

	user := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{})
	dialog := f.CreateDialog(user.ID, mode.ID)
	for i := 0; i < 10; i++ {
		_ = f.AppendMessage(dialog.ID, "user", "X")
	}
	items, _, err := h.collectAdminExport(context.Background(),
		nil, []int64{user.ID}, nil, "all", 0, "", "")
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if len(items) < 10 {
		t.Errorf("limit=0 should return all 10+, got %d", len(items))
	}
}

// =============================================================================
// adminSummaryModel: the model comes only from AI settings or a stable default.
// =============================================================================

func TestAdminSummaryModel_DefaultWhenNoModes(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	h := Handler{DB: env.Pool}

	model, temp := h.adminSummaryModel(context.Background())
	if model != defaultAIFallbackModel {
		t.Errorf("model=%q want %q", model, defaultAIFallbackModel)
	}
	if temp != 0.3 {
		t.Errorf("temperature=%v want 0.3", temp)
	}
}

func TestAdminSummaryModel_DoesNotFallbackToVisibleModeModel(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	h := Handler{DB: env.Pool}

	mode := f.CreateMode(TestModeOpts{Name: "SummaryModeTest"})
	_, err := env.Pool.Exec(context.Background(),
		`update modes set ai_model = 'openai/gpt-test-summary', model_temperature = 0.42 where id = $1`, mode.ID)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	model, temp := h.adminSummaryModel(context.Background())
	if model != defaultAIFallbackModel {
		t.Errorf("model=%q want %q", model, defaultAIFallbackModel)
	}
	if temp != 0.3 {
		t.Errorf("temperature=%v want 0.3", temp)
	}
}

func TestAdminSummaryModel_SettingsOverride(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	h := Handler{DB: env.Pool}
	upsertSetting(t, h, "ai_summary_model", "openai/summary-configured")
	upsertSetting(t, h, "ai_summary_temperature", "0.42")

	model, temp := h.adminSummaryModel(context.Background())
	if model != "openai/summary-configured" {
		t.Errorf("model=%q want configured", model)
	}
	if temp != 0.42 {
		t.Errorf("temperature=%v want 0.42", temp)
	}
}

// (min is a builtin since Go 1.21; httpJSON/ts.URL/ts.Client.Do are shared helpers from integration_factory_test.go.)
var _ = time.Now
