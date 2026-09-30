//go:build integration

package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"mindstrata-stage1/api/internal/testsupport"
)

// =============================================================================
// SLAB A — pure helpers + micro endpoints
// =============================================================================

// 1. /health → 200 + ok+service.
func TestHealth_OK(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)

	status, body := httpJSON(t, ts, "GET", "/health", nil)
	if status != http.StatusOK {
		t.Fatalf("status: %d", status)
	}
	if ok, _ := body["ok"].(bool); !ok {
		t.Errorf("ok=false: %v", body)
	}
	if svc, _ := body["service"].(string); svc != "api" {
		t.Errorf("service=%q", svc)
	}
}

// 2. /db-check happy path.
func TestDBCheck_OK(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)

	status, body := httpJSON(t, ts, "GET", "/db-check", nil)
	if status != http.StatusOK {
		t.Errorf("status %d body=%v", status, body)
	}
}

// 3. /db-check without a DB → 503.
func TestDBCheck_NoDB_503(t *testing.T) {
	t.Parallel()
	h := Handler{DB: nil}
	mux := NewRouter(h, []string{"*"})
	req, _ := http.NewRequest("GET", "/db-check", nil)
	rec := newRecorder()
	mux.ServeHTTP(rec, req)
	if rec.code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rec.code)
	}
}

// 4. ExpertStatus: expert role → 200; user → 403.
func TestExpertStatus_AuthMatrix(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	// Expert may pass.
	expert := f.CreateUser(TestUserOpts{Role: "expert"})
	ts.LoginAs(f.CreateSession(expert.ID))
	status, body := httpJSON(t, ts, "GET", "/api/expert/status", nil)
	if status != http.StatusOK {
		t.Fatalf("expert: status %d", status)
	}
	if msg, _ := body["status"].(string); msg != "stub" {
		t.Errorf("status field: %q", msg)
	}

	// User-role blocked.
	user := f.CreateUser(TestUserOpts{Role: "user"})
	ts.LoginAs(f.CreateSession(user.ID))
	status, _ = httpJSON(t, ts, "GET", "/api/expert/status", nil)
	if status != http.StatusForbidden && status != http.StatusUnauthorized {
		t.Errorf("user-role expert: expected 403/401, got %d", status)
	}

	// Wrong method.
	ts.LoginAs(f.CreateSession(expert.ID))
	status, _ = httpJSON(t, ts, "POST", "/api/expert/status", nil)
	if status != http.StatusMethodNotAllowed {
		t.Errorf("POST: expected 405, got %d", status)
	}
}

// 5. userRole: returns role + ErrNoRows for a missing/deleted user.
func TestUserRole_Returns(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	h := Handler{DB: env.Pool}

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	role, err := h.userRole(context.Background(), admin.ID)
	if err != nil {
		t.Fatalf("userRole: %v", err)
	}
	if role != "admin" {
		t.Errorf("role: got %q want admin", role)
	}

	// Non-existent ID → ErrNoRows.
	_, err = h.userRole(context.Background(), 9_999_999)
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("expected ErrNoRows for missing user, got %v", err)
	}
}

// 6. userCanUseLiveAI: ok if role is not empty, false if the user is not found.
func TestUserCanUseLiveAI(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	h := Handler{DB: env.Pool}

	user := f.CreateUser(TestUserOpts{Role: "user"})
	if !h.userCanUseLiveAI(context.Background(), user.ID) {
		t.Errorf("user with role=user should be allowed")
	}
	if h.userCanUseLiveAI(context.Background(), 9_999_999) {
		t.Errorf("missing user should not be allowed")
	}
}

// 7. guestUserIDFromRequest: parses the cookie or returns 0 (dev mode without a secret).
func TestGuestUserIDFromRequest(t *testing.T) {
	t.Parallel()
	// helper to build an http.Request with a cookie; "" = dev mode without a signature
	req := &http.Request{Header: http.Header{}}
	if got := guestUserIDFromRequest(req, ""); got != 0 {
		t.Errorf("no cookie: got %d want 0", got)
	}

	req.AddCookie(&http.Cookie{Name: guestCookieName, Value: "42"})
	if got := guestUserIDFromRequest(req, ""); got != 42 {
		t.Errorf("valid cookie dev-mode: got %d want 42", got)
	}

	// Malformed value → 0.
	bad := &http.Request{Header: http.Header{}}
	bad.AddCookie(&http.Cookie{Name: guestCookieName, Value: "not-a-number"})
	if got := guestUserIDFromRequest(bad, ""); got != 0 {
		t.Errorf("bad cookie: got %d want 0", got)
	}

	// Negative → 0.
	neg := &http.Request{Header: http.Header{}}
	neg.AddCookie(&http.Cookie{Name: guestCookieName, Value: "-1"})
	if got := guestUserIDFromRequest(neg, ""); got != 0 {
		t.Errorf("negative cookie: got %d want 0", got)
	}

	// L-1: HMAC mode: a signed value is accepted, a plain int is rejected.
	const secret = "test-secret-key"
	signed := signGuestCookie(99, secret)
	hmacReq := &http.Request{Header: http.Header{}}
	hmacReq.AddCookie(&http.Cookie{Name: guestCookieName, Value: signed})
	if got := guestUserIDFromRequest(hmacReq, secret); got != 99 {
		t.Errorf("HMAC valid: got %d want 99", got)
	}

	// Plain int with the secret enabled → 0 (unsigned values are rejected)
	plainReq := &http.Request{Header: http.Header{}}
	plainReq.AddCookie(&http.Cookie{Name: guestCookieName, Value: "99"})
	if got := guestUserIDFromRequest(plainReq, secret); got != 0 {
		t.Errorf("plain int with secret: got %d want 0 (unsigned rejected)", got)
	}

	// Tampered signature → 0
	tampered := signGuestCookie(1, secret)[:len(signed)-2] + "xx"
	tampReq := &http.Request{Header: http.Header{}}
	tampReq.AddCookie(&http.Cookie{Name: guestCookieName, Value: tampered})
	if got := guestUserIDFromRequest(tampReq, secret); got != 0 {
		t.Errorf("tampered HMAC: got %d want 0", got)
	}
}

// 8. scanOptionalNoRows: ErrNoRows → nil; other errors → as is.
func TestScanOptionalNoRows(t *testing.T) {
	t.Parallel()
	if got := scanOptionalNoRows(pgx.ErrNoRows); got != nil {
		t.Errorf("ErrNoRows: got %v want nil", got)
	}
	custom := errors.New("custom")
	if got := scanOptionalNoRows(custom); got != custom {
		t.Errorf("custom error: got %v want pass-through", got)
	}
	if got := scanOptionalNoRows(nil); got != nil {
		t.Errorf("nil: got %v want nil", got)
	}
}

// 9. notFoundIfNoRows passes through.
func TestNotFoundIfNoRows(t *testing.T) {
	t.Parallel()
	// Right now the function returns err on both branches; we check it does not crash and does not replace nil.
	if got := notFoundIfNoRows(pgx.ErrNoRows); !errors.Is(got, pgx.ErrNoRows) {
		t.Errorf("ErrNoRows passthrough: got %v", got)
	}
	custom := errors.New("zzz")
	if got := notFoundIfNoRows(custom); got != custom {
		t.Errorf("custom: %v", got)
	}
}

// 10. tailString.
func TestTailString(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in    string
		limit int
		want  string
	}{
		{"hello", 10, "hello"},
		{"hello world", 5, "world"},
		{"  trim me  ", 100, "trim me"},
		{"", 5, ""},
		{"abcdefg", 3, "efg"},
	}
	for _, c := range cases {
		if got := tailString(c.in, c.limit); got != c.want {
			t.Errorf("tailString(%q, %d) = %q want %q", c.in, c.limit, got, c.want)
		}
	}
}

// 11. clearOAuthStateCookie sets MaxAge=-1.
func TestClearOAuthStateCookie(t *testing.T) {
	t.Parallel()
	rec := newRecorder()
	rec.hdr = http.Header{}
	req, _ := http.NewRequest("GET", "/foo", nil)
	clearOAuthStateCookie(rec, req)
	setCookies := rec.hdr.Values("Set-Cookie")
	if len(setCookies) == 0 {
		t.Fatalf("no Set-Cookie header")
	}
	if !strings.Contains(setCookies[0], "Max-Age=0") && !strings.Contains(setCookies[0], "Max-Age=-1") {
		// Note: http.SetCookie with MaxAge<0 emits Max-Age=0
		t.Errorf("expected Max-Age=0 (clear), got: %s", setCookies[0])
	}
	if !strings.Contains(setCookies[0], oauthStateCookieName) {
		t.Errorf("wrong cookie name: %s", setCookies[0])
	}
}

// 12. nilResponseWriter: the ResponseWriter interface, no-op methods.
func TestNilResponseWriter(t *testing.T) {
	t.Parallel()
	var w http.ResponseWriter = nilResponseWriter{}
	h := w.Header()
	if h == nil {
		t.Errorf("Header() nil")
	}
	n, err := w.Write([]byte("anything"))
	if n != 0 || err != nil {
		t.Errorf("Write: n=%d err=%v want (0,nil)", n, err)
	}
	w.WriteHeader(http.StatusInternalServerError)
}

// 13. requireAdminOrTester is dead code (no call sites in current codebase) —
// covering its behavior would require calling it directly. Skipping with rationale.
// Kept here as marker for cleanup pass.

// =============================================================================
// chat_modes.go — DB helpers
// =============================================================================

// 14. firstUserAccessibleMode: returns the highest-priority mode.
func TestFirstUserAccessibleMode(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)

	user := f.CreateUser(TestUserOpts{})
	lowPrio := f.CreateMode(TestModeOpts{Name: "Low"})
	hiPrio := f.CreateMode(TestModeOpts{Name: "High"})
	f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: lowPrio.ID, DailyMessageLimit: 10, Priority: 1})
	f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: hiPrio.ID, DailyMessageLimit: 10, Priority: 100})

	mode, err := firstUserAccessibleMode(context.Background(), env.Pool, user.ID)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	if mode.ID != hiPrio.ID {
		t.Errorf("expected highest priority %d, got %d", hiPrio.ID, mode.ID)
	}

	// User without a grant → ErrNoRows.
	loner := f.CreateUser(TestUserOpts{})
	_, err = firstUserAccessibleMode(context.Background(), env.Pool, loner.ID)
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("no grants: expected ErrNoRows, got %v", err)
	}
}

// 15. getModeByID: hidden_at IS NOT NULL → ErrNoRows.
func TestGetModeByID_HiddenExcluded(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)

	mode := f.CreateMode(TestModeOpts{Name: "Visible"})
	got, err := getModeByID(context.Background(), env.Pool, mode.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ID != mode.ID {
		t.Errorf("id mismatch")
	}

	// Hide it
	_, _ = env.Pool.Exec(context.Background(), `update modes set hidden_at = now() where id = $1`, mode.ID)
	_, err = getModeByID(context.Background(), env.Pool, mode.ID)
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("hidden mode: expected ErrNoRows, got %v", err)
	}
}

// 16. modeForKnowledgeSelection: empty knowledge IDs → returns current as the "Basic AI" mode.
func TestModeForKnowledgeSelection_EmptyIDs(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	h := Handler{DB: env.Pool}

	current := modeRow{ID: 7, Name: "OriginalName", Prompt: "p", WelcomeMessage: "w"}
	got, replaced, err := h.modeForKnowledgeSelection(context.Background(), env.Pool, 1, current, []int64{})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !replaced {
		t.Errorf("replaced=false (expected true — meaning 'overridden to base')")
	}
	if got.Name != "Базовый ИИ" || got.Prompt != "" {
		t.Errorf("name/prompt not reset: %+v", got)
	}
}

// 17. modeForKnowledgeSelection: primary is accessible → take it.
func TestModeForKnowledgeSelection_PrimaryAccessible(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	h := Handler{DB: env.Pool}

	user := f.CreateUser(TestUserOpts{})
	primary := f.CreateMode(TestModeOpts{Name: "Primary"})
	f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: primary.ID, DailyMessageLimit: 10})

	current := modeRow{ID: 999, Name: "Other"}
	got, replaced, err := h.modeForKnowledgeSelection(context.Background(), env.Pool, user.ID, current, []int64{primary.ID})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got.ID != primary.ID {
		t.Errorf("got mode %d want primary %d", got.ID, primary.ID)
	}
	if replaced {
		t.Errorf("replaced should be false when primary is accessible (no fallback to base)")
	}
}

func TestModeForKnowledgeSelection_KeepsCurrentSelectedMode(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	h := Handler{DB: env.Pool}

	user := f.CreateUser(TestUserOpts{})
	primary := f.CreateMode(TestModeOpts{Name: "Первый режим промокода"})
	current := f.CreateMode(TestModeOpts{Name: "Текущий после оркестрации"})
	f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: primary.ID, DailyMessageLimit: 10})
	f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: current.ID, DailyMessageLimit: 10})

	currentRow := modeRow{ID: current.ID, Name: current.Name}
	got, replaced, err := h.modeForKnowledgeSelection(context.Background(), env.Pool, user.ID, currentRow, []int64{primary.ID, current.ID})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got.ID != current.ID {
		t.Fatalf("got mode %d want current selected mode %d", got.ID, current.ID)
	}
	if replaced {
		t.Fatalf("replaced should be false")
	}
}

// 18. modeForKnowledgeSelection: primary not accessible, current in the list → current is used.
func TestModeForKnowledgeSelection_CurrentInList_Allowed(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	h := Handler{DB: env.Pool}

	user := f.CreateUser(TestUserOpts{})
	current := f.CreateMode(TestModeOpts{Name: "Cur"})
	f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: current.ID, DailyMessageLimit: 10})

	currentRow := modeRow{ID: current.ID, Name: "Cur"}
	// primaryID=99999 does not exist, current.ID is in knowledgeIDs.
	got, _, err := h.modeForKnowledgeSelection(context.Background(), env.Pool, user.ID, currentRow, []int64{99999, current.ID})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got.ID != current.ID {
		t.Errorf("current should be allowed, got %d", got.ID)
	}
}

// 19. modeForKnowledgeSelection: primary not accessible and current not in the list → error.
func TestModeForKnowledgeSelection_Forbidden(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	h := Handler{DB: env.Pool}

	current := modeRow{ID: 1, Name: "x"}
	_, _, err := h.modeForKnowledgeSelection(context.Background(), env.Pool, 1, current, []int64{99999})
	if err == nil {
		t.Fatalf("expected error when no mode is accessible")
	}
	if !strings.Contains(err.Error(), "недоступен") {
		t.Errorf("error text: %q", err.Error())
	}
}

// 20. baseDialogMode: requestedModeID > 0 + a grant exists → returns it.
func TestBaseDialogMode_WithRequestedAndAccess(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	h := Handler{DB: env.Pool}

	user := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{Name: "BaseDialogTarget"})
	f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: mode.ID, DailyMessageLimit: 5})

	got, err := h.baseDialogMode(context.Background(), env.Pool, user.ID, mode.ID)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got.ID != mode.ID {
		t.Errorf("got %d want %d", got.ID, mode.ID)
	}
}

// 21. baseDialogMode: requestedModeID=0 + current_mode exists → returns it.
func TestBaseDialogMode_FallsBackToCurrent(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	h := Handler{DB: env.Pool}

	user := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{Name: "CurrentMode"})
	f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: mode.ID, DailyMessageLimit: 5})
	_, _ = env.Pool.Exec(context.Background(),
		`update users set current_mode = $1 where id = $2`, mode.ID, user.ID)

	got, err := h.baseDialogMode(context.Background(), env.Pool, user.ID, 0)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got.ID != mode.ID {
		t.Errorf("got %d want current %d", got.ID, mode.ID)
	}
}

// =============================================================================
// admin_export_promo_modes.go — orchestrator
// =============================================================================

// 22. adminPromoModeIndex: all three structures get filled.
func TestAdminPromoModeIndex_PopulatesAll(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	h := Handler{DB: env.Pool}

	modeA := f.CreateMode(TestModeOpts{})
	modeB := f.CreateMode(TestModeOpts{})

	// tariff group + tariff + tariff_mode
	var groupID int64
	if err := env.Pool.QueryRow(context.Background(),
		`insert into tariff_groups (name, sort_order) values ('IndexGroup', 0) returning id`).Scan(&groupID); err != nil {
		t.Fatalf("seed group: %v", err)
	}
	var tariffID int64
	if err := env.Pool.QueryRow(context.Background(),
		`insert into tariffs (name, group_id, monthly_price, limit_type) values ('IndexTariff', $1, 0, 'shared') returning id`, groupID).Scan(&tariffID); err != nil {
		t.Fatalf("seed tariff: %v", err)
	}
	_, _ = env.Pool.Exec(context.Background(),
		`insert into tariff_mode (tariff_id, mode_id) values ($1, $2), ($1, $3)`,
		tariffID, modeA.ID, modeB.ID)

	all, tariffModes, groupModes, err := h.adminPromoModeIndex(context.Background())
	if err != nil {
		t.Fatalf("index: %v", err)
	}
	if !containsInt64(all, modeA.ID) || !containsInt64(all, modeB.ID) {
		t.Errorf("all modes missing %d/%d: %v", modeA.ID, modeB.ID, all)
	}
	if !containsInt64(tariffModes[tariffID], modeA.ID) {
		t.Errorf("tariffModes[%d] missing modeA %d: %v", tariffID, modeA.ID, tariffModes[tariffID])
	}
	if !containsInt64(groupModes[groupID], modeB.ID) {
		t.Errorf("groupModes[%d] missing modeB %d: %v", groupID, modeB.ID, groupModes[groupID])
	}
}

// 23. promocodeModeIDs: a thin wrapper over promocodeModeIDsForTargets.
func TestPromocodeModeIDs_WrapperBehavior(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	h := Handler{DB: env.Pool}

	mode := f.CreateMode(TestModeOpts{})

	got, err := h.promocodeModeIDs(context.Background(), "mode", mode.ID)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(got) != 1 || got[0] != mode.ID {
		t.Errorf("mode wrapper: got %v want [%d]", got, mode.ID)
	}

	// targetID=0 + grants_type=mode → empty result
	got0, err := h.promocodeModeIDs(context.Background(), "mode", 0)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(got0) != 0 {
		t.Errorf("targetID=0: got %v want []", got0)
	}
}

// 24. promocodeModeIDsForTargets: grants_type=all → returns all visible modes.
func TestPromocodeModeIDsForTargets_AllReturnsAllVisible(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	h := Handler{DB: env.Pool}

	visible := f.CreateMode(TestModeOpts{Name: "Vis"})
	hidden := f.CreateMode(TestModeOpts{Name: "Hidden"})
	_, _ = env.Pool.Exec(context.Background(), `update modes set hidden_at = now() where id = $1`, hidden.ID)

	got, err := h.promocodeModeIDsForTargets(context.Background(), "all", nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !containsInt64(got, visible.ID) {
		t.Errorf("visible mode missing: %v", got)
	}
	if containsInt64(got, hidden.ID) {
		t.Errorf("hidden mode leaked: %v", got)
	}
}
