//go:build integration

package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"mindstrata-stage1/api/internal/testsupport"
)

// mcpPost: POST with the key in a header. A separate helper because the shared
// httpJSON does not take headers, and the MCP key is deliberately not passed in
// the URL: in the query it would end up in proxy logs.
func mcpPost(t testing.TB, ts *TestServer, key string, body map[string]any) (int, map[string]any) {
	t.Helper()
	buf, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req, err := http.NewRequest("POST", ts.URL("/api/mcp/call"), bytes.NewReader(buf))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("X-MCP-Key", key)
	}
	resp, err := ts.Client.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close()
	var parsed map[string]any
	raw, _ := io.ReadAll(resp.Body)
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &parsed)
	}
	return resp.StatusCode, parsed
}

// mcpCall sends a command with the key in a header, the same way the MCP server
// in apps/mcp-admin does.
func mcpCall(t testing.TB, ts *TestServer, key string, body map[string]any) (int, map[string]any) {
	t.Helper()
	return mcpPost(t, ts, key, body)
}

func setMCPKey(t testing.TB, env *testsupport.Env, key string) {
	t.Helper()
	if _, err := env.Pool.Exec(context.Background(),
		`insert into system_settings (key, value) values ('mcp_admin_key', $1)
		 on conflict (key) do update set value = excluded.value`, key); err != nil {
		t.Fatalf("ключ MCP: %v", err)
	}
}

// TestMCP_KeyGuard: no key in the DB → 503 (not 401, otherwise the agent would
// look for a mistake in its key when the server simply has none); a wrong key → 401.
func TestMCP_KeyGuard(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)

	if _, err := env.Pool.Exec(context.Background(), `delete from system_settings where key = 'mcp_admin_key'`); err != nil {
		t.Fatalf("очистка ключа: %v", err)
	}
	if code, _ := mcpCall(t, ts, "whatever", map[string]any{"action": "ping"}); code != http.StatusServiceUnavailable {
		t.Fatalf("без ключа на сервере: %d, want 503", code)
	}
	setMCPKey(t, env, "mcp-secret")
	if code, _ := mcpCall(t, ts, "wrong", map[string]any{"action": "ping"}); code != http.StatusUnauthorized {
		t.Fatalf("чужой ключ: %d, want 401", code)
	}
	if code, _ := mcpCall(t, ts, "", map[string]any{"action": "ping"}); code != http.StatusUnauthorized {
		t.Fatalf("пустой ключ: %d, want 401", code)
	}
	code, resp := mcpCall(t, ts, "mcp-secret", map[string]any{"action": "ping"})
	if code != http.StatusOK || resp["ok"] != true {
		t.Fatalf("свой ключ: %d %+v", code, resp)
	}
	if cmds, _ := resp["commands"].([]any); len(cmds) < 5 {
		t.Fatalf("ping не перечислил команды: %+v", resp)
	}
}

// TestMCP_UnknownActionRejected: a whitelist of commands. A typo in the name must
// give a clear error, not a silent success.
func TestMCP_UnknownActionRejected(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)
	setMCPKey(t, env, "mcp-2")

	code, resp := mcpCall(t, ts, "mcp-2", map[string]any{"action": "users.delete"})
	if code != http.StatusBadRequest {
		t.Fatalf("неизвестная команда: %d %+v, want 400", code, resp)
	}
}

// TestMCP_GrantAndRevokeAccess: granting and closing access are the main
// operations the whole thing was built for. Closing cuts it off by date rather
// than deleting: we check that the record remains and shows as expired.
func TestMCP_GrantAndRevokeAccess(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)
	setMCPKey(t, env, "mcp-3")
	ctx := context.Background()

	f := NewFactory(t, env.Pool)
	user := f.CreateUser(TestUserOpts{}).ID
	modeID := f.CreateMode(TestModeOpts{}).ID

	code, resp := mcpCall(t, ts, "mcp-3", map[string]any{
		"action": "access.grant", "userId": user, "modeIds": []int64{modeID}, "days": 7, "reason": "тест",
	})
	if code != http.StatusOK || resp["granted"] != float64(1) {
		t.Fatalf("выдача: %d %+v", code, resp)
	}

	var live int
	if err := env.Pool.QueryRow(ctx,
		`select count(*) from user_mode_access where user_id = $1 and active_to > now()`, user).Scan(&live); err != nil {
		t.Fatalf("чтение доступа: %v", err)
	}
	if live != 1 {
		t.Fatalf("после выдачи живых доступов %d, want 1", live)
	}

	code, resp = mcpCall(t, ts, "mcp-3", map[string]any{"action": "access.list", "userId": user})
	if code != http.StatusOK {
		t.Fatalf("список доступов: %d %+v", code, resp)
	}
	if rows, _ := resp["access"].([]any); len(rows) != 1 {
		t.Fatalf("список вернул %d строк, want 1: %+v", len(rows), resp)
	}

	code, resp = mcpCall(t, ts, "mcp-3", map[string]any{"action": "access.revoke", "userId": user, "reason": "тест"})
	if code != http.StatusOK || resp["revoked"] != float64(1) {
		t.Fatalf("закрытие: %d %+v", code, resp)
	}
	var total, stillLive int
	if err := env.Pool.QueryRow(ctx, `
		select count(*), count(*) filter (where active_to > now())
		from user_mode_access where user_id = $1`, user).Scan(&total, &stillLive); err != nil {
		t.Fatalf("чтение после закрытия: %v", err)
	}
	if total != 1 || stillLive != 0 {
		t.Fatalf("после закрытия строк %d, живых %d — ожидали 1 и 0 (обрыв сроком, не удаление)", total, stillLive)
	}

	// Every change must leave a trace: otherwise the agent's edits are
	// indistinguishable from manual ones, and there is nothing to investigate an
	// incident by.
	var audit int
	if err := env.Pool.QueryRow(ctx,
		`select count(*) from admin_audit_log where action in ('admin.mcp.access_grant','admin.mcp.access_revoke')`).Scan(&audit); err != nil {
		t.Fatalf("аудит: %v", err)
	}
	if audit != 2 {
		t.Fatalf("в аудите %d записей, want 2", audit)
	}
}

// TestMCP_GrantValidations: invalid terms and empty targets must not reach the database.
func TestMCP_GrantValidations(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)
	setMCPKey(t, env, "mcp-4")
	user := NewFactory(t, env.Pool).CreateUser(TestUserOpts{}).ID

	cases := []map[string]any{
		{"action": "access.grant", "userId": user, "days": 7},                          // no modes
		{"action": "access.grant", "userId": user, "modeIds": []int64{1}, "days": 0},   // zero term
		{"action": "access.grant", "userId": user, "modeIds": []int64{1}, "days": 400}, // term out of range
		{"action": "access.grant", "modeIds": []int64{1}, "days": 7},                   // no user
		{"action": "access.revoke"},                                                    // no user
	}
	for i, body := range cases {
		if code, resp := mcpCall(t, ts, "mcp-4", body); code != http.StatusBadRequest {
			t.Fatalf("случай %d: код %d, want 400 (%+v)", i, code, resp)
		}
	}
}

// TestMCP_PromoCreateAndUpdate: a promo code is created and edited, but an
// activated one does not change: the person got access on the original terms.
func TestMCP_PromoCreateAndUpdate(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)
	setMCPKey(t, env, "mcp-5")
	ctx := context.Background()

	var tariffID int64
	if err := env.Pool.QueryRow(ctx, `
		insert into tariffs (name, monthly_price, limit_type, daily_message_limit)
		values ('Тариф MCP', 100, 'shared', 50) returning id`).Scan(&tariffID); err != nil {
		t.Fatalf("тариф: %v", err)
	}

	code, resp := mcpCall(t, ts, "mcp-5", map[string]any{
		"action": "promo.create", "tariffId": tariffID, "days": 3, "maxUses": 5, "comment": "из MCP",
	})
	if code != http.StatusOK {
		t.Fatalf("создание промокода: %d %+v", code, resp)
	}
	codeStr, _ := resp["code"].(string)
	if codeStr == "" {
		t.Fatalf("промокод без кода: %+v", resp)
	}

	code, resp = mcpCall(t, ts, "mcp-5", map[string]any{"action": "promo.update", "code": codeStr, "days": 30})
	if code != http.StatusOK {
		t.Fatalf("правка промокода: %d %+v", code, resp)
	}
	var duration string
	if err := env.Pool.QueryRow(ctx, `select duration::text from promocodes where code = $1`, codeStr).Scan(&duration); err != nil {
		t.Fatalf("чтение промокода: %v", err)
	}
	if duration != "30 days" {
		t.Fatalf("срок промокода %q, want «30 days»", duration)
	}

	// Mark it used: editing must stop working.
	if _, err := env.Pool.Exec(ctx, `update promocodes set used_count = 1 where code = $1`, codeStr); err != nil {
		t.Fatalf("отметка использования: %v", err)
	}
	if code, resp := mcpCall(t, ts, "mcp-5", map[string]any{"action": "promo.update", "code": codeStr, "days": 90}); code != http.StatusNotFound {
		t.Fatalf("правка активированного промокода: %d %+v, want 404", code, resp)
	}
}

// TestMCP_ReadOnlyCommandsLeaveNoAudit: reads must not clutter the journal,
// otherwise real changes drown in it.
func TestMCP_ReadOnlyCommandsLeaveNoAudit(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)
	setMCPKey(t, env, "mcp-6")

	for _, action := range []string{"stats.summary", "tariffs.list", "promo.list", "access.list", "tir.results"} {
		if code, resp := mcpCall(t, ts, "mcp-6", map[string]any{"action": action}); code != http.StatusOK {
			t.Fatalf("%s: %d %+v", action, code, resp)
		}
	}
	var audit int
	if err := env.Pool.QueryRow(context.Background(),
		`select count(*) from admin_audit_log where action like 'admin.mcp.%'`).Scan(&audit); err != nil {
		t.Fatalf("аудит: %v", err)
	}
	if audit != 0 {
		t.Fatalf("чтение оставило %d записей в аудите, want 0", audit)
	}
}
