//go:build integration

package httpapi

import (
	"net/http"
	"testing"

	"mindstrata-stage1/api/internal/testsupport"
)

// Careful: clientLogEnabled is a shared package global (see the comment in
// debug_client_log_test.go). These tests NEVER turn logging back off (that would
// wipe the buffer of parallel tests); they only turn it on and look for THEIR OWN
// entries by a unique UserID, like the other tests in this file.

// TestDebugClientLog_AppendsWhenEnabled (Claims: the handler comment declares a
// silent no-op when logging is off and a real write when it is on; here we check
// the "on" branch through HTTP, not a direct clientLogAppend call).
func TestDebugClientLog_AppendsWhenEnabled(t *testing.T) {
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)
	owner := f.CreateUser(TestUserOpts{Role: "owner"})
	user := f.CreateUser(TestUserOpts{})

	ts.LoginAs(f.CreateSession(owner.ID))
	status, body := httpJSON(t, ts, "POST", "/api/admin/client-logs", map[string]any{"enabled": true})
	if status != http.StatusOK || body["enabled"] != true {
		t.Fatalf("enable client logs: %d body=%v", status, body)
	}

	ts.LoginAs(f.CreateSession(user.ID))
	status, body = httpJSON(t, ts, "POST", "/api/debug/client-log", map[string]any{
		"events": []map[string]any{{"type": "onChange", "domLen": 5}},
		"ua":     "TestAgent/http",
		"path":   "/chat",
	})
	if status != http.StatusOK || body["ok"] != true {
		t.Fatalf("post client-log: %d body=%v", status, body)
	}

	ts.LoginAs(f.CreateSession(owner.ID))
	status, body = httpJSON(t, ts, "GET", "/api/admin/client-logs", nil)
	if status != http.StatusOK {
		t.Fatalf("get client-logs: %d body=%v", status, body)
	}
	entries, _ := body["entries"].([]any)
	found := false
	for _, e := range entries {
		m, _ := e.(map[string]any)
		if int64(m["userId"].(float64)) == user.ID {
			found = true
			if m["path"] != "/chat" || m["ua"] != "TestAgent/http" {
				t.Fatalf("entry content mismatch: %+v", m)
			}
		}
	}
	if !found {
		t.Fatalf("posted entry for user %d not found in GET response: %v", user.ID, entries)
	}
}

// TestDebugClientLog_EmptyEventsIsNoOpButOK: an empty batch.events is a silent
// 200, nothing breaks.
func TestDebugClientLog_EmptyEventsIsNoOpButOK(t *testing.T) {
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)
	owner := f.CreateUser(TestUserOpts{Role: "owner"})
	ts.LoginAs(f.CreateSession(owner.ID))
	_, _ = httpJSON(t, ts, "POST", "/api/admin/client-logs", map[string]any{"enabled": true})

	user := f.CreateUser(TestUserOpts{})
	ts.LoginAs(f.CreateSession(user.ID))
	status, body := httpJSON(t, ts, "POST", "/api/debug/client-log", map[string]any{"events": []any{}})
	if status != http.StatusOK || body["ok"] != true {
		t.Fatalf("empty events: %d body=%v", status, body)
	}
}

// TestDebugClientLog_WrongMethod_405.
func TestDebugClientLog_WrongMethod_405(t *testing.T) {
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)
	ts.LoginAs(f.CreateSession(f.CreateUser(TestUserOpts{}).ID))
	req, err := http.NewRequest(http.MethodGet, ts.URL("/api/debug/client-log"), nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := ts.Client.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("GET /api/debug/client-log: %d, want 405", resp.StatusCode)
	}
}

// TestAdminClientLogs_NonOwnerAdmin_Forbidden (Standards: admin/owner only;
// content_admin must not get access to other users' client logs).
func TestAdminClientLogs_NonOwnerAdmin_Forbidden(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)
	ts.LoginAs(f.CreateSession(f.CreateUser(TestUserOpts{Role: "content_admin"}).ID))

	status, _ := httpJSON(t, ts, "GET", "/api/admin/client-logs", nil)
	if status != http.StatusForbidden {
		t.Fatalf("content_admin GET client-logs: %d, want 403", status)
	}
}

// TestAdminClientLogs_InvalidJSON_400.
func TestAdminClientLogs_InvalidJSON_400(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)
	ts.LoginAs(f.CreateSession(f.CreateUser(TestUserOpts{Role: "owner"}).ID))

	req, err := http.NewRequest(http.MethodPost, ts.URL("/api/admin/client-logs"), nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Body = nil
	resp, err := ts.Client.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("POST with empty body: %d, want 400", resp.StatusCode)
	}
}
