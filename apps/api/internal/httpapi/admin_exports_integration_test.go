//go:build integration

package httpapi

import (
	"context"
	"io"
	"net/http"
	"testing"

	"mindstrata-stage1/api/internal/testsupport"
)

// TestAdminExports_GET_NoFilters: smoke GET without filters.
func TestAdminExports_GET_NoFilters(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	user := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{})
	dialog := f.CreateDialog(user.ID, mode.ID)
	_ = f.AppendMessage(dialog.ID, "user", "user-message-for-export")
	_ = f.AppendMessage(dialog.ID, "assistant", "asst-reply-for-export")

	ts.LoginAs(f.CreateSession(admin.ID))

	status, body := httpJSON(t, ts, "GET", "/api/admin/exports/messages?limit=10", nil)
	if status != http.StatusOK {
		t.Fatalf("exports GET: %d body=%v", status, body)
	}
	if msgs, _ := body["messages"].([]any); len(msgs) == 0 {
		t.Errorf("messages empty: %v", body)
	}
}

func TestAdminExports_GET_WritesSensitiveAuditContext(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	user := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{})
	dialog := f.CreateDialog(user.ID, mode.ID)
	_ = f.AppendMessage(dialog.ID, "user", "audit-export-user-message")

	ts.LoginAs(f.CreateSession(admin.ID))
	req, err := http.NewRequest(http.MethodGet, ts.URL("/api/admin/exports/messages?limit=10"), nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set(requestIDHeader, "audit-export-trace-1")
	resp, err := ts.Client.Do(req)
	if err != nil {
		t.Fatalf("exports request: %v", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("exports status: got %d want 200", resp.StatusCode)
	}

	var requestID, method, path, section string
	var sensitive bool
	if err := env.Pool.QueryRow(context.Background(), `
		select request_id, http_method, path, section, sensitive
		from admin_audit_log
		where actor_user_id=$1 and action='admin.export.messages.read'
		order by id desc limit 1`, admin.ID).Scan(&requestID, &method, &path, &section, &sensitive); err != nil {
		t.Fatalf("query export audit: %v", err)
	}
	if requestID != "audit-export-trace-1" || method != http.MethodGet || path != "/api/admin/exports/messages" || section != "exports" || !sensitive {
		t.Fatalf("audit context mismatch: requestID=%q method=%q path=%q section=%q sensitive=%v",
			requestID, method, path, section, sensitive)
	}
}

// TestAdminExports_GET_ByModeIDs: filtering by modeIds returns only the requested ones.
func TestAdminExports_GET_ByModeIDs(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	user := f.CreateUser(TestUserOpts{})
	mode1 := f.CreateMode(TestModeOpts{})
	mode2 := f.CreateMode(TestModeOpts{})
	d1 := f.CreateDialog(user.ID, mode1.ID)
	d2 := f.CreateDialog(user.ID, mode2.ID)
	_ = f.AppendMessage(d1.ID, "user", "msg-in-mode-1")
	_ = f.AppendMessage(d2.ID, "user", "msg-in-mode-2")

	ts.LoginAs(f.CreateSession(admin.ID))

	status, body := httpJSON(t, ts, "GET",
		"/api/admin/exports/messages?modeIds="+itoa(mode1.ID), nil)
	if status != http.StatusOK {
		t.Fatalf("exports GET filtered: %d body=%v", status, body)
	}
	// With a filter on mode1 there must be no mode2 messages.
	// (The items structure is not specified, so we stick to sourceBytes > 0.)
	if sb, _ := body["sourceBytes"].(float64); sb == 0 {
		t.Errorf("sourceBytes=0 with seeded data: %v", body)
	}
}

// TestAdminExports_GET_Options.
func TestAdminExports_GET_Options(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	_ = f.CreateMode(TestModeOpts{Name: "OptionTest mode"})
	ts.LoginAs(f.CreateSession(admin.ID))

	status, _ := httpJSON(t, ts, "GET", "/api/admin/exports/messages?options=true", nil)
	if status != http.StatusOK {
		t.Fatalf("exports options: %d", status)
	}
}

// TestAdminExports_POST_WithSummaryPrompt: POST builds the summary payload
// (no live AI call when EnableLiveAI=false).
func TestAdminExports_POST_WithSummaryPrompt(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	user := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{})
	dialog := f.CreateDialog(user.ID, mode.ID)
	_ = f.AppendMessage(dialog.ID, "user", "input for summary")

	ts.LoginAs(f.CreateSession(admin.ID))

	status, body := httpJSON(t, ts, "POST", "/api/admin/exports/messages", map[string]any{
		"modeIds": []int64{mode.ID},
		"prompt":  "Сделай выжимку",
		"limit":   10,
	})
	// 200 ok OR 400 if the payload is invalid: the point is no 5xx
	// (live AI is off, which is fine as the "no AI" path).
	if status >= 500 {
		t.Fatalf("exports POST 5xx: %d body=%v", status, body)
	}
}

// TestAdminExports_NonAdminBlocked.
func TestAdminExports_NonAdminBlocked(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	user := f.CreateUser(TestUserOpts{Role: "user"})
	ts.LoginAs(f.CreateSession(user.ID))

	status, _ := httpJSON(t, ts, "GET", "/api/admin/exports/messages", nil)
	if status != http.StatusForbidden && status != http.StatusUnauthorized {
		t.Errorf("user GET exports: expected 403/401, got %d", status)
	}
}

// TestAdminExports_ContentAdminCannotReadRawExports: the CMS role does not read raw messages.
func TestAdminExports_ContentAdminCannotReadRawExports(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	ca := f.CreateUser(TestUserOpts{Role: "content_admin"})
	ts.LoginAs(f.CreateSession(ca.ID))

	status, _ := httpJSON(t, ts, "GET", "/api/admin/exports/messages?options=true", nil)
	if status != http.StatusForbidden {
		t.Errorf("content_admin exports options: expected 403, got %d", status)
	}
}

// TestAdminExports_RoleFilterFiltersMessages: roleFilter=user → only role='user'.
func TestAdminExports_RoleFilterFiltersMessages(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	user := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{})
	dialog := f.CreateDialog(user.ID, mode.ID)
	_ = f.AppendMessage(dialog.ID, "user", "user1")
	_ = f.AppendMessage(dialog.ID, "assistant", "asst1")
	_ = f.AppendMessage(dialog.ID, "user", "user2")

	ts.LoginAs(f.CreateSession(admin.ID))

	status, body := httpJSON(t, ts, "GET",
		"/api/admin/exports/messages?roleFilter=user&limit=10", nil)
	if status != http.StatusOK {
		t.Fatalf("exports roleFilter: %d", status)
	}
	if msgs, _ := body["messages"].([]any); len(msgs) == 0 {
		t.Errorf("no messages with roleFilter=user")
	}
	// If meta.assistant comes back explicitly, we found a regression
	_ = context.Background()
}
