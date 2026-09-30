//go:build integration

package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mindstrata-stage1/api/internal/testsupport"
)

// relaySecretSegment stands for the secret path of a relay gateway URL
// (https://relay/<secret>/v1): knowing it is enough to use the relay.
const relaySecretSegment = "relaysecretQ7Q7Q7Q7Q7"

// brokenRelay accepts the connection and drops it without an answer, so the
// HTTP client fails with a *url.Error that carries the full request URL.
func brokenRelay(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Error("hijack not supported")
			return
		}
		conn, _, err := hj.Hijack()
		if err == nil {
			conn.Close()
		}
	}))
	t.Cleanup(srv.Close)
	return srv.URL + "/" + relaySecretSegment + "/v1"
}

func sendLiveAsRole(t *testing.T, env *testsupport.Env, role string) (int, string) {
	t.Helper()
	ts := aiTestServer(t, env, brokenRelay(t), "test-key-for-live-ai")
	f := NewFactory(t, env.Pool)
	user := f.CreateUser(TestUserOpts{Role: role})
	mode := f.CreateMode(TestModeOpts{AIModel: "openai/gpt-4o-mini"})
	f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: mode.ID, DailyMessageLimit: 50})
	dialog := f.CreateDialog(user.ID, mode.ID)
	ts.LoginAs(f.CreateSession(user.ID))
	status, body := httpJSON(t, ts, "POST", "/api/chat/send", map[string]any{
		"dialogId": dialog.ID, "text": "hello", "responseMode": "live",
	})
	raw, _ := json.Marshal(body)
	return status, string(raw)
}

// Regression (2026-09-30 open-source review): a transport error of the AI
// gateway was returned to the chat as err.Error(), i.e. with the full request
// URL. For a relay gateway that URL contains the relay secret, so any user —
// a guest included — who hit a relay hiccup received a working relay address.
func TestLiveAIError_OrdinaryUserDoesNotSeeGatewayURL(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	status, body := sendLiveAsRole(t, env, "user")
	if status != http.StatusBadGateway {
		t.Fatalf("status=%d want 502 body=%s", status, body)
	}
	if strings.Contains(body, relaySecretSegment) || strings.Contains(body, "127.0.0.1") {
		t.Fatalf("the gateway URL leaked to a user: %s", body)
	}
	if !strings.Contains(body, "live_ai_error") {
		t.Fatalf("the error code must stay for the UI: %s", body)
	}
}

// Staff still get a useful message, but never the secret part of the URL.
func TestLiveAIError_StaffSeeDetailsWithoutSecretPath(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	status, body := sendLiveAsRole(t, env, "tester")
	if status != http.StatusBadGateway {
		t.Fatalf("status=%d want 502 body=%s", status, body)
	}
	if strings.Contains(body, relaySecretSegment) {
		t.Fatalf("the relay secret leaked to staff: %s", body)
	}
}
