//go:build integration

package httpapi

import (
	"net/http"
	"testing"

	"mindstrata-stage1/api/internal/testsupport"
)

// =============================================================================
// SLAB C — tester handlers
// =============================================================================

// 1. testerSafeChecks: returns a fixed set + contains "db".
func TestTesterSafeChecks_ContainsCore(t *testing.T) {
	t.Parallel()
	checks := testerSafeChecks()
	if len(checks) < 5 {
		t.Errorf("expected ≥5 checks, got %d: %v", len(checks), checks)
	}
	wantCore := []string{"db", "users", "modes", "tariffs", "promocodes"}
	for _, c := range wantCore {
		found := false
		for _, x := range checks {
			if x == c {
				found = true
			}
		}
		if !found {
			t.Errorf("missing core check: %q in %v", c, checks)
		}
	}
}

// 2. TesterUserDetail: tester role → user delegated to adminGetUser.
func TestTesterUserDetail_TesterCanRead(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	tester := f.CreateUser(TestUserOpts{Role: "tester"})
	target := f.CreateUser(TestUserOpts{Role: "user"})
	ts.LoginAs(f.CreateSession(tester.ID))

	status, body := httpJSON(t, ts, "GET", "/api/tester/users/"+itoa(target.ID), nil)
	if status != http.StatusOK {
		t.Fatalf("tester GET user: status=%d body=%v", status, body)
	}
	if u, _ := body["user"].(map[string]any); u == nil {
		t.Errorf("user payload missing: %v", body)
	}
}

// 3. TesterUserDetail: invalid id → 400.
func TestTesterUserDetail_BadID_400(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	tester := f.CreateUser(TestUserOpts{Role: "tester"})
	ts.LoginAs(f.CreateSession(tester.ID))

	status, _ := httpJSON(t, ts, "GET", "/api/tester/users/not-a-number", nil)
	if status != http.StatusBadRequest {
		t.Errorf("invalid id: expected 400, got %d", status)
	}
}

// 4. TesterUserDetail: non-tester non-admin → 403.
func TestTesterUserDetail_RegularUser_Blocked(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	user := f.CreateUser(TestUserOpts{Role: "user"})
	other := f.CreateUser(TestUserOpts{Role: "user"})
	ts.LoginAs(f.CreateSession(user.ID))

	status, _ := httpJSON(t, ts, "GET", "/api/tester/users/"+itoa(other.ID), nil)
	if status != http.StatusForbidden && status != http.StatusUnauthorized {
		t.Errorf("user role: expected 403/401, got %d", status)
	}
}

// 5. TesterUserDetail: wrong method → 405.
func TestTesterUserDetail_WrongMethod_405(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	tester := f.CreateUser(TestUserOpts{Role: "tester"})
	other := f.CreateUser(TestUserOpts{})
	ts.LoginAs(f.CreateSession(tester.ID))

	status, _ := httpJSON(t, ts, "DELETE", "/api/tester/users/"+itoa(other.ID), nil)
	if status != http.StatusMethodNotAllowed {
		t.Errorf("DELETE: expected 405, got %d", status)
	}
}

// 6. TesterRunCheck: check=db happy path.
func TestTesterRunCheck_DBCheck(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	tester := f.CreateUser(TestUserOpts{Role: "tester"})
	ts.LoginAs(f.CreateSession(tester.ID))

	status, body := httpJSON(t, ts, "POST", "/api/tester/run-check", map[string]any{
		"check": "db",
	})
	if status != http.StatusOK {
		t.Fatalf("db check: %d body=%v", status, body)
	}
	result, _ := body["result"].(map[string]any)
	if check, _ := result["check"].(string); check != "db" {
		t.Errorf("check field: %q", check)
	}
	if ok, _ := result["ok"].(bool); !ok {
		t.Errorf("db check ok=false: %v", result)
	}
	if _, hasDur := result["durationMs"]; !hasDur {
		t.Errorf("durationMs missing")
	}
}

// 7. TesterRunCheck: unknown check → 400.
func TestTesterRunCheck_Unknown_400(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	tester := f.CreateUser(TestUserOpts{Role: "tester"})
	ts.LoginAs(f.CreateSession(tester.ID))

	status, _ := httpJSON(t, ts, "POST", "/api/tester/run-check", map[string]any{
		"check": "nonexistent_check_zzz",
	})
	if status != http.StatusBadRequest {
		t.Errorf("unknown: expected 400, got %d", status)
	}
}

// 8. TesterRunCheck: all_safe → an array of results.
func TestTesterRunCheck_AllSafe(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	tester := f.CreateUser(TestUserOpts{Role: "tester"})
	ts.LoginAs(f.CreateSession(tester.ID))

	status, body := httpJSON(t, ts, "POST", "/api/tester/run-check", map[string]any{
		"check": "all_safe",
	})
	if status != http.StatusOK {
		t.Fatalf("all_safe: %d body=%v", status, body)
	}
	result, _ := body["result"].(map[string]any)
	results, _ := result["results"].([]any)
	if len(results) == 0 {
		t.Errorf("all_safe should return non-empty results")
	}
}

// 9. TesterRunCheck: non-tester → 403.
func TestTesterRunCheck_NonTester_403(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	user := f.CreateUser(TestUserOpts{Role: "support"}) // support is NOT in the tester/admin list
	ts.LoginAs(f.CreateSession(user.ID))

	status, _ := httpJSON(t, ts, "POST", "/api/tester/run-check", map[string]any{"check": "db"})
	if status != http.StatusForbidden && status != http.StatusUnauthorized {
		t.Errorf("support role: expected 403/401, got %d", status)
	}
}

// 10. TesterRunCheck: GET → 405.
func TestTesterRunCheck_WrongMethod_405(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	tester := f.CreateUser(TestUserOpts{Role: "tester"})
	ts.LoginAs(f.CreateSession(tester.ID))

	status, _ := httpJSON(t, ts, "GET", "/api/tester/run-check", nil)
	if status != http.StatusMethodNotAllowed {
		t.Errorf("GET: expected 405, got %d", status)
	}
}

// 11. runGuardrailPromptAppendCheck: ok=true when there is a mode and no OPENAI_API_KEY.
func TestRunGuardrailPromptAppendCheck_NoLiveAI(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	h := Handler{DB: env.Pool}

	_ = f.CreateMode(TestModeOpts{Name: "GuardrailModeForTest"})

	req, _ := http.NewRequest("GET", "/x", nil)
	result := h.runGuardrailPromptAppendCheck(req)
	if ok, _ := result["ok"].(bool); !ok {
		t.Errorf("guardrail check ok=false: %v", result)
	}
	if _, hasPrompt := result["finalPromptTail"]; !hasPrompt {
		t.Errorf("finalPromptTail missing")
	}
	if _, hasGuardrail := result["attachedGuardrail"]; !hasGuardrail {
		t.Errorf("attachedGuardrail missing")
	}
	if msg, _ := result["liveResponse"].(string); msg == "" {
		t.Errorf("liveResponse should describe live-AI-off state")
	}
}

// 12. TesterUserChatPreview: the tester sees the modes for the user.
func TestTesterUserChatPreview_TesterCanPreview(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	tester := f.CreateUser(TestUserOpts{Role: "tester"})
	target := f.CreateUser(TestUserOpts{Role: "user"})
	mode := f.CreateMode(TestModeOpts{Name: "PreviewMode"})
	f.GrantAccess(GrantAccessOpts{UserID: target.ID, ModeID: mode.ID, DailyMessageLimit: 10})

	ts.LoginAs(f.CreateSession(tester.ID))
	status, body := httpJSON(t, ts, "GET", "/api/tester/user-chat-preview?userId="+itoa(target.ID), nil)
	if status != http.StatusOK {
		t.Fatalf("preview: %d body=%v", status, body)
	}
	if ro, _ := body["readOnly"].(bool); !ro {
		t.Errorf("readOnly must be true")
	}
	if scr, _ := body["historyScrubbed"].(bool); !scr {
		t.Errorf("historyScrubbed must be true")
	}
}

// 13. TesterUserChatPreview: no userId → 400.
func TestTesterUserChatPreview_NoUserID_400(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	tester := f.CreateUser(TestUserOpts{Role: "tester"})
	ts.LoginAs(f.CreateSession(tester.ID))

	status, _ := httpJSON(t, ts, "GET", "/api/tester/user-chat-preview", nil)
	if status != http.StatusBadRequest {
		t.Errorf("no userId: expected 400, got %d", status)
	}
}

// 14. TesterUserChatPreview: non-existent user → 404.
func TestTesterUserChatPreview_UserNotFound_404(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	tester := f.CreateUser(TestUserOpts{Role: "tester"})
	ts.LoginAs(f.CreateSession(tester.ID))

	status, _ := httpJSON(t, ts, "GET", "/api/tester/user-chat-preview?userId=99999999", nil)
	if status != http.StatusNotFound {
		t.Errorf("missing user: expected 404, got %d", status)
	}
}
