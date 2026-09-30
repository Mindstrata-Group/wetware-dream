//go:build integration

package httpapi

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"mindstrata-stage1/api/internal/testsupport"
)

// =============================================================================
// SLAB B — promo_admin (HMAC-protected promo summary panel)
// =============================================================================

// promoAdminTokenForCode mirrors the logic of production admin_broadcast_summary.go.
// S-NEW-2: production removed the fallback to OpenAI/Telegram/hard-coded values;
// tests must use the same default as buildTestServer/NewTestServer.
func promoAdminTokenForCode(h Handler, code string) string {
	secret := strings.TrimSpace(h.PromoAdminSecret)
	if secret == "" {
		// Match the buildTestServer/NewTestServer default so that a handler without an
		// explicit PromoAdminSecret still works in tests.
		secret = "test-promo-admin-secret"
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(strings.ToUpper(strings.TrimSpace(code))))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// 1. PromoAdminStatus with a valid key → 200 + promo data.
func TestPromoAdminStatus_ValidKey(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	mode := f.CreateMode(TestModeOpts{})
	code := "PROMOADM_" + itoa(int64(env.Pool.Stat().AcquireCount()))
	promo := f.CreatePromocode(TestPromocodeOpts{Code: code, TargetID: mode.ID})

	key := promoAdminTokenForCode(ts.Handler, code)
	status, body := httpJSON(t, ts, "GET",
		"/api/promo-admin/status?promo="+code+"&key="+key, nil)
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%v", status, body)
	}
	promoMap, _ := body["promo"].(map[string]any)
	if promoMap == nil {
		t.Fatalf("no promo field in body: %v", body)
	}
	if id, _ := promoMap["id"].(float64); int64(id) != promo.ID {
		t.Errorf("promo.id: got %v want %d", id, promo.ID)
	}
	// summaryUsed / summaryRemaining must be present
	if _, ok := promoMap["summaryUsed"]; !ok {
		t.Errorf("summaryUsed missing")
	}
}

// 2. PromoAdminStatus without a key → 403 (invalid token).
func TestPromoAdminStatus_NoKey_403(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	mode := f.CreateMode(TestModeOpts{})
	code := "PROMONOKEY_" + itoa(int64(env.Pool.Stat().AcquireCount()))
	_ = f.CreatePromocode(TestPromocodeOpts{Code: code, TargetID: mode.ID})

	status, _ := httpJSON(t, ts, "GET", "/api/promo-admin/status?promo="+code, nil)
	if status != http.StatusForbidden {
		t.Errorf("no key: expected 403, got %d", status)
	}
}

// 3. PromoAdminStatus without promo → 400.
func TestPromoAdminStatus_NoPromo_400(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)
	_ = env

	status, _ := httpJSON(t, ts, "GET", "/api/promo-admin/status", nil)
	if status != http.StatusBadRequest {
		t.Errorf("no promo: expected 400, got %d", status)
	}
}

// 4. PromoAdminStatus: non-existent promo → 404.
func TestPromoAdminStatus_PromoNotFound_404(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)
	_ = env

	code := "DOESNOTEXIST_XYZ"
	key := promoAdminTokenForCode(ts.Handler, code)
	status, _ := httpJSON(t, ts, "GET",
		"/api/promo-admin/status?promo="+code+"&key="+key, nil)
	if status != http.StatusNotFound {
		t.Errorf("promo not found: expected 404, got %d", status)
	}
}

// 5. PromoAdminStatus: wrong method → 405.
func TestPromoAdminStatus_PutMethod_405(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)
	_ = env

	status, _ := httpJSON(t, ts, "PUT", "/api/promo-admin/status?promo=X&key=Y", nil)
	if status != http.StatusMethodNotAllowed {
		t.Errorf("PUT: expected 405, got %d", status)
	}
}

// 6. PromoAdminSummarize: invalid JSON → 400.
func TestPromoAdminSummarize_BadJSON_400(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)
	_ = env

	// Use bytes.NewBufferString directly.
	req, _ := http.NewRequest("POST", ts.URL("/api/promo-admin/summarize"),
		strings.NewReader("{garbage}}}"))
	req.Header.Set("Content-Type", "application/json")
	resp, err := ts.Client.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("bad json: expected 400, got %d", resp.StatusCode)
	}
}

// 7. PromoAdminSummarize: no promptID → 400.
func TestPromoAdminSummarize_NoPromptID_400(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	mode := f.CreateMode(TestModeOpts{})
	code := "PROMOSUM_" + itoa(int64(env.Pool.Stat().AcquireCount()))
	_ = f.CreatePromocode(TestPromocodeOpts{Code: code, TargetID: mode.ID})

	key := promoAdminTokenForCode(ts.Handler, code)
	status, body := httpJSON(t, ts, "POST",
		"/api/promo-admin/summarize?key="+key, map[string]any{
			"code":     code,
			"modeIds":  []int64{mode.ID},
			"promptId": 0, // missing
		})
	if status != http.StatusBadRequest {
		t.Errorf("no prompt: expected 400, got %d body=%v", status, body)
	}
}

// 8. promoAdminTokenValid: hand-crafted token matches.
func TestPromoAdminTokenValid_RoundTrip(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)
	_ = env

	code := "ROUNDTRIP_AAA"
	key := promoAdminTokenForCode(ts.Handler, code)

	req, _ := http.NewRequest("GET", "/?key="+key, nil)
	if !ts.Handler.promoAdminTokenValid(req, code) {
		t.Errorf("valid token rejected")
	}
	if ts.Handler.promoAdminTokenValid(req, "DIFFERENT") {
		t.Errorf("token validated for different code")
	}

	// the token form is accepted too
	req2, _ := http.NewRequest("GET", "/?token="+key, nil)
	if !ts.Handler.promoAdminTokenValid(req2, code) {
		t.Errorf("token query param not accepted")
	}

	// no key/token
	req3, _ := http.NewRequest("GET", "/", nil)
	if ts.Handler.promoAdminTokenValid(req3, code) {
		t.Errorf("empty token accepted")
	}
}

// 9. promoAdminSummaryWindowActive.
func TestPromoAdminSummaryWindowActive(t *testing.T) {
	t.Parallel()
	past := time.Now().Add(-24 * time.Hour)
	future := time.Now().Add(24 * time.Hour)

	// both nil → true
	if !promoAdminSummaryWindowActive(nil, nil) {
		t.Errorf("nil/nil: expected true")
	}
	// activeFrom in future → false
	if promoAdminSummaryWindowActive(&future, nil) {
		t.Errorf("from=future: expected false")
	}
	// activeTo in past → false
	if promoAdminSummaryWindowActive(nil, &past) {
		t.Errorf("to=past: expected false")
	}
	// both in valid window → true
	if !promoAdminSummaryWindowActive(&past, &future) {
		t.Errorf("past..future: expected true")
	}
}

// 10. promoAdminContext.toMap returns all fields.
func TestPromoAdminContext_ToMap(t *testing.T) {
	t.Parallel()
	limit := int64(5)
	ctx := promoAdminContext{id: 42, code: "TEST", maxUses: 10, usedCount: 3, summaryLimit: &limit}
	m := ctx.toMap()
	if id, _ := m["id"].(int64); id != 42 {
		t.Errorf("id: %v", m["id"])
	}
	if code, _ := m["code"].(string); code != "TEST" {
		t.Errorf("code: %v", m["code"])
	}
	if lim, _ := m["summaryLimit"].(*int64); lim == nil || *lim != 5 {
		t.Errorf("summaryLimit: %v", m["summaryLimit"])
	}
}

// 11. listSummaryPrompts.
func TestListSummaryPrompts_OrdersByDefaultThenID(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	h := Handler{DB: env.Pool}

	_, _ = env.Pool.Exec(context.Background(),
		`insert into admin_summary_prompts (name, prompt, is_default) values ('Reg', 'p1', false), ('Def', 'p2', true)`)

	prompts, err := h.listSummaryPrompts(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(prompts) < 2 {
		t.Fatalf("expected ≥2 prompts, got %d", len(prompts))
	}
	first, _ := prompts[0]["isDefault"].(bool)
	if !first {
		t.Errorf("first prompt should be default=true (order is_default desc): got %v", prompts[0])
	}
}

// 12. promoSummaryUsage: limit=nil → remaining=nil; limit=N → N-used.
func TestPromoSummaryUsage(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	h := Handler{DB: env.Pool}

	mode := f.CreateMode(TestModeOpts{})
	promo := f.CreatePromocode(TestPromocodeOpts{TargetID: mode.ID})

	// 0 saved summaries → used=0
	used, remaining := h.promoSummaryUsage(context.Background(), promo.ID, nil)
	if used != 0 {
		t.Errorf("initial used: %d want 0", used)
	}
	if remaining != nil {
		t.Errorf("nil limit: remaining must be nil, got %v", *remaining)
	}

	// Insert 1 fake summary
	_, _ = env.Pool.Exec(context.Background(),
		`insert into admin_export_summaries (promocode_id, prompt, filters, source_message_count, source_bytes, approx_tokens, result) values ($1, 'x', '{}'::jsonb, 0, 0, 0, '')`, promo.ID)

	used, _ = h.promoSummaryUsage(context.Background(), promo.ID, nil)
	if used != 1 {
		t.Errorf("after 1 summary: used=%d want 1", used)
	}

	// With limit=3 → remaining=2
	limit := int64(3)
	_, rem := h.promoSummaryUsage(context.Background(), promo.ID, &limit)
	if rem == nil || *rem != 2 {
		t.Errorf("limit=3 used=1: remaining want 2, got %v", rem)
	}

	// With limit=0 (exhausted) → remaining=0
	zero := int64(0)
	_, rem0 := h.promoSummaryUsage(context.Background(), promo.ID, &zero)
	if rem0 == nil || *rem0 != 0 {
		t.Errorf("limit=0: remaining must be 0 (clamped), got %v", rem0)
	}
}

// 13. promoAdminModes: returns modes associated with the promo via grants_type=mode.
func TestPromoAdminModes_ModeGrant(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	h := Handler{DB: env.Pool}

	mode := f.CreateMode(TestModeOpts{Name: "PromoAdminMode"})
	other := f.CreateMode(TestModeOpts{Name: "OtherMode"})
	promo := f.CreatePromocode(TestPromocodeOpts{GrantsType: "mode", TargetID: mode.ID})

	modes, err := h.promoAdminModes(context.Background(), promo.ID)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	foundTarget := false
	for _, m := range modes {
		if id, _ := m["id"].(int64); id == mode.ID {
			foundTarget = true
		}
		if id, _ := m["id"].(int64); id == other.ID {
			t.Errorf("leaked unrelated mode %d", other.ID)
		}
	}
	if !foundTarget {
		t.Errorf("target mode %d not in result: %v", mode.ID, modes)
	}
}

func TestPromoAdminModes_TargetsTablePreferred(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	h := Handler{DB: env.Pool}

	legacy := f.CreateMode(TestModeOpts{Name: "LegacyMode"})
	mode1 := f.CreateMode(TestModeOpts{Name: "TargetModeA"})
	mode2 := f.CreateMode(TestModeOpts{Name: "TargetModeB"})
	promo := f.CreatePromocode(TestPromocodeOpts{GrantsType: "mode", TargetID: legacy.ID})
	if _, err := env.Pool.Exec(context.Background(),
		`insert into promocode_targets (promocode_id, target_id, created_at) values ($1, $2, now()), ($1, $3, now())`,
		promo.ID, mode1.ID, mode2.ID); err != nil {
		t.Fatalf("seed targets: %v", err)
	}

	modes, err := h.promoAdminModes(context.Background(), promo.ID)
	if err != nil {
		t.Fatalf("promoAdminModes: %v", err)
	}
	ids := map[int64]bool{}
	for _, item := range modes {
		id, _ := item["id"].(int64)
		ids[id] = true
	}
	if !ids[mode1.ID] || !ids[mode2.ID] {
		t.Fatalf("targets table modes missing: got %v want %d and %d", modes, mode1.ID, mode2.ID)
	}
	if ids[legacy.ID] {
		t.Fatalf("legacy target leaked despite promocode_targets rows: %v", modes)
	}
}

func TestPromoAdminSummarize_LimitExhaustedReturnsUsageAndSkipsAI(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	mode := f.CreateMode(TestModeOpts{})
	code := "PROMOLIMIT_" + itoa(int64(env.Pool.Stat().AcquireCount()))

	var promoID int64
	if err := env.Pool.QueryRow(context.Background(),
		`insert into promocodes (code, max_uses, used_count, duration, access_priority, grants_type, target_id, limit_type, daily_message_limit, summary_limit)
		 values ($1, 0, 0, '30 days'::interval, 0, 'mode', $2, 'fixed', 50, 0) returning id`,
		code, mode.ID).Scan(&promoID); err != nil {
		t.Fatalf("insert promo: %v", err)
	}
	var promptID int64
	if err := env.Pool.QueryRow(context.Background(), `insert into admin_summary_prompts (name, prompt, is_default) values ('Limit prompt', 'summarize', true) returning id`).Scan(&promptID); err != nil {
		t.Fatalf("insert prompt: %v", err)
	}

	var calls atomic.Int64
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(fake.Close)
	handler := Handler{DB: env.Pool, OpenAIAPIKey: "test", OpenAIBaseURL: fake.URL, HTTPClient: &http.Client{Timeout: 5 * time.Second}}
	ts := buildTestServer(t, env, handler)

	key := promoAdminTokenForCode(handler, code)
	status, body := httpJSON(t, ts, "POST", "/api/promo-admin/summarize?key="+key, map[string]any{
		"code":     code,
		"promptId": promptID,
		"modeIds":  []int64{mode.ID},
	})
	if status != http.StatusForbidden {
		t.Fatalf("status=%d want 403 body=%v", status, body)
	}
	if got, _ := body["error"].(string); !strings.Contains(got, "Лимит резюмирований") {
		t.Fatalf("error=%q want limit exhausted", got)
	}
	if got := int64(body["used"].(float64)); got != 0 {
		t.Fatalf("used=%d want 0", got)
	}
	if got := int64(body["remaining"].(float64)); got != 0 {
		t.Fatalf("remaining=%d want 0", got)
	}
	if calls.Load() != 0 {
		t.Fatalf("AI provider called despite exhausted summary limit")
	}
}

func TestPromoAdminSummarize_AIErrorReturnsSafeErrorWithoutPersistingHistory(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	_, _ = env.Pool.Exec(context.Background(), `insert into system_settings (key, value) values ('ai_retry_attempts', '1') on conflict (key) do update set value=excluded.value`)

	mode := f.CreateMode(TestModeOpts{Name: "DiagMode"})
	promo := f.CreatePromocode(TestPromocodeOpts{Code: "PROMOAIERR_" + itoa(int64(env.Pool.Stat().AcquireCount())), TargetID: mode.ID})
	var promptID int64
	if err := env.Pool.QueryRow(context.Background(), `insert into admin_summary_prompts (name, prompt, is_default) values ('Diag prompt', 'summarize', true) returning id`).Scan(&promptID); err != nil {
		t.Fatalf("insert prompt: %v", err)
	}
	user := f.CreateUser(TestUserOpts{})
	dialog := f.CreateDialog(user.ID, mode.ID)
	_, _ = env.Pool.Exec(context.Background(), `insert into promocode_usages (user_id, promocode_id, used_at) values ($1, $2, now())`, user.ID, promo.ID)
	f.AppendMessage(dialog.ID, "user", "diagnostic message")

	fake := fakeVseGPT(t, http.StatusInternalServerError, "")
	handler := Handler{DB: env.Pool, OpenAIAPIKey: "test", OpenAIBaseURL: "https://api.vsegpt.ru/v1", HTTPClient: &http.Client{Transport: vsegptRT{target: fake.URL}, Timeout: 5 * time.Second}}
	ts := buildTestServer(t, env, handler)

	key := promoAdminTokenForCode(handler, promo.Code)
	status, body := httpJSON(t, ts, "POST", "/api/promo-admin/summarize?key="+key, map[string]any{
		"code":     promo.Code,
		"promptId": promptID,
		"modeIds":  []int64{mode.ID},
	})
	if status != http.StatusBadGateway {
		t.Fatalf("status=%d want 502 body=%v", status, body)
	}
	if got, _ := body["code"].(string); got != "live_ai_error" {
		t.Fatalf("code=%q want live_ai_error body=%v", got, body)
	}
	if _, hasDebug := body["debug"]; hasDebug {
		t.Fatalf("promo admin AI error leaked debug payload: %v", body)
	}
	if strings.Contains(fmt.Sprint(body), "diagnostic message") {
		t.Fatalf("promo admin AI error leaked raw source message: %v", body)
	}
	if got := int64(body["messageCount"].(float64)); got != 1 {
		t.Fatalf("messageCount=%d want 1", got)
	}
	if got := int64(body["sourceBytes"].(float64)); got <= 0 {
		t.Fatalf("sourceBytes=%d want positive", got)
	}
	if got := int64(body["approxTokens"].(float64)); got <= 0 {
		t.Fatalf("approxTokens=%d want positive", got)
	}
	var saved int64
	if err := env.Pool.QueryRow(context.Background(), `select count(*) from admin_export_summaries where promocode_id=$1`, promo.ID).Scan(&saved); err != nil {
		t.Fatalf("count summaries: %v", err)
	}
	if saved != 0 {
		t.Fatalf("saved summaries after AI error=%d want 0", saved)
	}
}

func TestPromoAdminStatus_MessageStats(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	mode := f.CreateMode(TestModeOpts{})
	promo := f.CreatePromocode(TestPromocodeOpts{Code: "PROMOSTATS_" + itoa(int64(env.Pool.Stat().AcquireCount())), TargetID: mode.ID})
	withMessages := f.CreateUser(TestUserOpts{})
	withoutMessages := f.CreateUser(TestUserOpts{})
	_, _ = env.Pool.Exec(context.Background(), `insert into promocode_usages (user_id, promocode_id, used_at) values ($1, $2, now()), ($3, $2, now())`, withMessages.ID, promo.ID, withoutMessages.ID)
	dialog := f.CreateDialog(withMessages.ID, mode.ID)
	f.AppendMessage(dialog.ID, "user", "hello")
	f.AppendMessage(dialog.ID, "assistant", "hi")

	key := promoAdminTokenForCode(ts.Handler, promo.Code)
	status, body := httpJSON(t, ts, "GET", "/api/promo-admin/status?promo="+promo.Code+"&key="+key, nil)
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%v", status, body)
	}
	promoMap, _ := body["promo"].(map[string]any)
	if got := int64(promoMap["messageCount"].(float64)); got != 2 {
		t.Errorf("messageCount=%d want 2", got)
	}
	if got := int64(promoMap["activationsWithoutMessages"].(float64)); got != 1 {
		t.Errorf("activationsWithoutMessages=%d want 1", got)
	}
}

func TestPromoAdminStatus_MessageStats_TargetsTableFallback(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	legacy := f.CreateMode(TestModeOpts{Name: "LegacyStatsMode"})
	target := f.CreateMode(TestModeOpts{Name: "TargetStatsMode"})
	promo := f.CreatePromocode(TestPromocodeOpts{Code: "PROMOTARGETS_" + itoa(int64(env.Pool.Stat().AcquireCount())), TargetID: legacy.ID})
	if _, err := env.Pool.Exec(context.Background(),
		`insert into promocode_targets (promocode_id, target_id, created_at) values ($1, $2, now())`,
		promo.ID, target.ID); err != nil {
		t.Fatalf("seed targets: %v", err)
	}
	user := f.CreateUser(TestUserOpts{})
	if _, err := env.Pool.Exec(context.Background(),
		`insert into promocode_usages (user_id, promocode_id, used_at) values ($1, $2, now())`,
		user.ID, promo.ID); err != nil {
		t.Fatalf("seed usage: %v", err)
	}
	dialog := f.CreateDialog(user.ID, target.ID)
	f.AppendMessage(dialog.ID, "user", "target table message")
	f.AppendMessage(dialog.ID, "assistant", "target table answer")

	key := promoAdminTokenForCode(ts.Handler, promo.Code)
	status, body := httpJSON(t, ts, "GET", "/api/promo-admin/status?promo="+promo.Code+"&key="+key, nil)
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%v", status, body)
	}
	promoMap, _ := body["promo"].(map[string]any)
	if got := int64(promoMap["messageCount"].(float64)); got != 2 {
		t.Fatalf("messageCount=%d want 2 for promocode_targets fallback", got)
	}
	if got := int64(promoMap["activationsWithoutMessages"].(float64)); got != 0 {
		t.Fatalf("activationsWithoutMessages=%d want 0", got)
	}
}

func TestPromoAdminStatus_MessageStats_ExactAccessAttributionSeparatesPromos(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	mode := f.CreateMode(TestModeOpts{Name: "ExactPromoMode"})
	oldPromo := f.CreatePromocode(TestPromocodeOpts{Code: "PROMOOLD_" + itoa(int64(env.Pool.Stat().AcquireCount())), TargetID: mode.ID})
	newPromo := f.CreatePromocode(TestPromocodeOpts{Code: "PROMONEW_" + itoa(int64(env.Pool.Stat().AcquireCount())), TargetID: mode.ID})
	user := f.CreateUser(TestUserOpts{})
	if _, err := env.Pool.Exec(context.Background(),
		`insert into promocode_usages (user_id, promocode_id, used_at) values ($1, $2, now()), ($1, $3, now())`,
		user.ID, oldPromo.ID, newPromo.ID); err != nil {
		t.Fatalf("seed usages: %v", err)
	}
	oldSource, newSource := oldPromo.ID, newPromo.ID
	f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: mode.ID, DailyMessageLimit: 10, AccessType: "promocode", SourceID: &oldSource})
	f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: mode.ID, DailyMessageLimit: 1, AccessType: "promocode", SourceID: &newSource})
	var newAccessID int64
	if err := env.Pool.QueryRow(context.Background(),
		`select id from user_mode_access where user_id=$1 and mode_id=$2 and access_type='promocode' and source_id=$3`,
		user.ID, mode.ID, newPromo.ID).Scan(&newAccessID); err != nil {
		t.Fatalf("query new access: %v", err)
	}
	dialog := f.CreateDialog(user.ID, mode.ID)
	userMsg := f.AppendMessage(dialog.ID, "user", "belongs to newest promo")
	assistantMsg := f.AppendMessage(dialog.ID, "assistant", "reply belongs to newest promo")
	for _, messageID := range []int64{userMsg, assistantMsg} {
		if _, err := env.Pool.Exec(context.Background(), `
			insert into dialog_message_access_usage (user_id, mode_id, dialog_message_id, access_id, usage_date, usage_kind)
			values ($1, $2, $3, $4, current_date, 'message')`,
			user.ID, mode.ID, messageID, newAccessID); err != nil {
			t.Fatalf("seed message access usage: %v", err)
		}
	}

	oldKey := promoAdminTokenForCode(ts.Handler, oldPromo.Code)
	oldStatus, oldBody := httpJSON(t, ts, "GET", "/api/promo-admin/status?promo="+oldPromo.Code+"&key="+oldKey, nil)
	if oldStatus != http.StatusOK {
		t.Fatalf("old status=%d body=%v", oldStatus, oldBody)
	}
	oldPromoMap, _ := oldBody["promo"].(map[string]any)
	if got := int64(oldPromoMap["messageCount"].(float64)); got != 0 {
		t.Fatalf("old promo messageCount=%d want 0; exact attribution must not fall back to old activation", got)
	}
	if got := int64(oldPromoMap["activationsWithoutMessages"].(float64)); got != 1 {
		t.Fatalf("old promo activationsWithoutMessages=%d want 1", got)
	}

	newKey := promoAdminTokenForCode(ts.Handler, newPromo.Code)
	newStatus, newBody := httpJSON(t, ts, "GET", "/api/promo-admin/status?promo="+newPromo.Code+"&key="+newKey, nil)
	if newStatus != http.StatusOK {
		t.Fatalf("new status=%d body=%v", newStatus, newBody)
	}
	newPromoMap, _ := newBody["promo"].(map[string]any)
	if got := int64(newPromoMap["messageCount"].(float64)); got != 2 {
		t.Fatalf("new promo messageCount=%d want 2", got)
	}
	if got := int64(newPromoMap["activationsWithoutMessages"].(float64)); got != 0 {
		t.Fatalf("new promo activationsWithoutMessages=%d want 0", got)
	}
}

func TestPromoAdminStatus_MessageStats_UsesMessageAttributionAfterDialogModeSwitch(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	promoMode := f.CreateMode(TestModeOpts{Name: "PromoAdminAttributedMode"})
	currentDialogMode := f.CreateMode(TestModeOpts{Name: "PromoAdminCurrentDialogMode"})
	promo := f.CreatePromocode(TestPromocodeOpts{Code: "PROMOATTR_" + itoa(int64(env.Pool.Stat().AcquireCount())), TargetID: promoMode.ID})
	user := f.CreateUser(TestUserOpts{})
	if _, err := env.Pool.Exec(context.Background(),
		`insert into promocode_usages (user_id, promocode_id, used_at) values ($1, $2, now() - interval '1 minute')`,
		user.ID, promo.ID); err != nil {
		t.Fatalf("seed usage: %v", err)
	}
	sourceID := promo.ID
	f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: promoMode.ID, DailyMessageLimit: 10, AccessType: "promocode", SourceID: &sourceID})
	var accessID int64
	if err := env.Pool.QueryRow(context.Background(),
		`select id from user_mode_access where user_id=$1 and mode_id=$2 and access_type='promocode' and source_id=$3`,
		user.ID, promoMode.ID, promo.ID).Scan(&accessID); err != nil {
		t.Fatalf("query access: %v", err)
	}
	dialog := f.CreateDialog(user.ID, promoMode.ID)
	userMsg := f.AppendMessage(dialog.ID, "user", "promo admin attributed user")
	assistantMsg := f.AppendMessage(dialog.ID, "assistant", "promo admin attributed assistant")
	for _, messageID := range []int64{userMsg, assistantMsg} {
		if _, err := env.Pool.Exec(context.Background(), `
			insert into dialog_message_access_usage (user_id, mode_id, dialog_message_id, access_id, usage_date, usage_kind)
			values ($1, $2, $3, $4, current_date, 'message')`,
			user.ID, promoMode.ID, messageID, accessID); err != nil {
			t.Fatalf("seed message attribution: %v", err)
		}
	}
	if _, err := env.Pool.Exec(context.Background(), `update users_dialogs set mode_id=$2 where id=$1`, dialog.ID, currentDialogMode.ID); err != nil {
		t.Fatalf("switch dialog mode: %v", err)
	}

	key := promoAdminTokenForCode(ts.Handler, promo.Code)
	status, body := httpJSON(t, ts, "GET", "/api/promo-admin/status?promo="+promo.Code+"&key="+key, nil)
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%v", status, body)
	}
	promoMap, _ := body["promo"].(map[string]any)
	if got := int64(promoMap["messageCount"].(float64)); got != 2 {
		t.Fatalf("messageCount=%d want 2 from message attribution after dialog switch", got)
	}
	if got := int64(promoMap["activationsWithoutMessages"].(float64)); got != 0 {
		t.Fatalf("activationsWithoutMessages=%d want 0", got)
	}
}

func TestPromoAdminStatus_LoadsPersistedSummaryHistory(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	mode := f.CreateMode(TestModeOpts{Name: "HistoryMode"})
	promo := f.CreatePromocode(TestPromocodeOpts{Code: "PROMOHISTORY_" + itoa(int64(env.Pool.Stat().AcquireCount())), TargetID: mode.ID})
	_, _ = env.Pool.Exec(context.Background(),
		`insert into admin_export_summaries (promocode_id, prompt, filters, source_message_count, source_bytes, approx_tokens, result, created_at) values ($1, 'prompt', $2::jsonb, 3, 42, 11, 'saved summary', now())`,
		promo.ID, `{"modeIds":[`+itoa(mode.ID)+`]}`)

	key := promoAdminTokenForCode(ts.Handler, promo.Code)
	status, body := httpJSON(t, ts, "GET", "/api/promo-admin/status?promo="+promo.Code+"&key="+key, nil)
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%v", status, body)
	}
	history, _ := body["history"].([]any)
	if len(history) != 1 {
		t.Fatalf("history len=%d want 1 body=%v", len(history), body)
	}
	item, _ := history[0].(map[string]any)
	if got, _ := item["result"].(string); got != "saved summary" {
		t.Fatalf("result=%q want saved summary", got)
	}
	if got, _ := item["modeLabel"].(string); got != "HistoryMode" {
		t.Fatalf("modeLabel=%q want HistoryMode", got)
	}
}

func TestPromoAdminStatus_HistoryReturnsNewestLimited(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	mode := f.CreateMode(TestModeOpts{Name: "NewestHistoryMode"})
	promo := f.CreatePromocode(TestPromocodeOpts{Code: "PROMOHISTORYLIMIT_" + itoa(int64(env.Pool.Stat().AcquireCount())), TargetID: mode.ID})
	for i := 0; i < 25; i++ {
		_, err := env.Pool.Exec(context.Background(),
			`insert into admin_export_summaries (promocode_id, prompt, filters, source_message_count, source_bytes, approx_tokens, result, created_at)
			 values ($1, 'prompt', $2::jsonb, $3, 42, 11, $4, now() + ($5::text || ' seconds')::interval)`,
			promo.ID, `{"modeIds":[`+itoa(mode.ID)+`]}`, int64(i+1), "summary-"+itoa(int64(i)), itoa(int64(i)))
		if err != nil {
			t.Fatalf("insert history %d: %v", i, err)
		}
	}

	key := promoAdminTokenForCode(ts.Handler, promo.Code)
	status, body := httpJSON(t, ts, "GET", "/api/promo-admin/status?promo="+promo.Code+"&key="+key, nil)
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%v", status, body)
	}
	history, _ := body["history"].([]any)
	if len(history) != 20 {
		t.Fatalf("history len=%d want 20", len(history))
	}
	first, _ := history[0].(map[string]any)
	if got, _ := first["result"].(string); got != "summary-24" {
		t.Fatalf("first result=%q want newest summary-24", got)
	}
	last, _ := history[len(history)-1].(map[string]any)
	if got, _ := last["result"].(string); got != "summary-5" {
		t.Fatalf("last result=%q want oldest retained summary-5", got)
	}
}

func TestPromoAdminSummarize_NoMessages_400(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	mode := f.CreateMode(TestModeOpts{})
	promo := f.CreatePromocode(TestPromocodeOpts{Code: "PROMOEMPTY_" + itoa(int64(env.Pool.Stat().AcquireCount())), TargetID: mode.ID})
	var promptID int64
	if err := env.Pool.QueryRow(context.Background(), `insert into admin_summary_prompts (name, prompt, is_default) values ('No messages guard', 'summarize', true) returning id`).Scan(&promptID); err != nil {
		t.Fatalf("insert prompt: %v", err)
	}

	key := promoAdminTokenForCode(ts.Handler, promo.Code)
	status, body := httpJSON(t, ts, "POST", "/api/promo-admin/summarize?key="+key, map[string]any{
		"code":     promo.Code,
		"modeIds":  []int64{mode.ID},
		"promptId": promptID,
	})
	if status != http.StatusBadRequest {
		t.Fatalf("status=%d want 400 body=%v", status, body)
	}
	if got, _ := body["error"].(string); !strings.Contains(got, "Нет сообщений") {
		t.Fatalf("error=%q want no messages guard", got)
	}
	if got := int64(body["messageCount"].(float64)); got != 0 {
		t.Fatalf("messageCount=%d want 0", got)
	}
}

// 14. promoAdminSigningSecret: only an explicit PromoAdminSecret.
// S-NEW-2: the fallback to OpenAIAPIKey/TelegramBotToken/hard-coded was removed:
// the hard-coded "mindstrata-local-promo-admin-dev-secret" used to be public in
// git, and anyone could generate a valid token. An explicit secret is now required.
func TestPromoAdminSigningSecret_FallbackChain(t *testing.T) {
	t.Parallel()
	// Explicit PromoAdminSecret returned.
	h1 := Handler{PromoAdminSecret: "primary"}
	if h1.promoAdminSigningSecret() != "primary" {
		t.Errorf("PromoAdminSecret should be returned as-is, got %q", h1.promoAdminSigningSecret())
	}
	// Without PromoAdminSecret: an empty string (NO fallback to other fields).
	h2 := Handler{OpenAIAPIKey: "openai-key", TelegramBotToken: "tg-token", TelegramChatID: "tg-chat"}
	if h2.promoAdminSigningSecret() != "" {
		t.Errorf("S-NEW-2: fallback запрещён, ожидали пустую строку, got %q", h2.promoAdminSigningSecret())
	}
	// Empty Handler: an empty string (NO hard-coded default).
	h3 := Handler{}
	if h3.promoAdminSigningSecret() != "" {
		t.Errorf("S-NEW-2: hardcoded default запрещён, ожидали пустую строку, got %q", h3.promoAdminSigningSecret())
	}
	// Whitespace trimmed.
	h4 := Handler{PromoAdminSecret: "   "}
	if h4.promoAdminSigningSecret() != "" {
		t.Errorf("whitespace not trimmed: %q", h4.promoAdminSigningSecret())
	}
}
