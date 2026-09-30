//go:build integration

package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"mindstrata-stage1/api/internal/testsupport"
)

// testMaxWebhookSecret is a random-looking path/header secret that has
// nothing in common with the bot token (R-035 defect 2).
const testMaxWebhookSecret = "whsec_Qm9Yc2VjcmV0LW5vdC1mcm9tLXRva2VuLTE2"

func maxSecretTestServer(t testing.TB, env *testsupport.Env, secret string) *TestServer {
	t.Helper()
	return NewTestServerWithHandler(t, env.Pool, Handler{
		MaxBotToken:      testMaxToken,
		MaxBotUsername:   "testbot",
		MaxWebhookSecret: secret,
		HTTPClient:       &http.Client{Transport: noopMaxTransport{}},
	})
}

func postMaxWebhook(t *testing.T, ts *TestServer, path, headerSecret string, body any) int {
	t.Helper()
	raw, _ := json.Marshal(body)
	req, err := http.NewRequest(http.MethodPost, ts.URL(path), bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if headerSecret != "" {
		req.Header.Set("X-Max-Bot-Api-Secret", headerSecret)
	}
	resp, err := ts.Client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

// maxWebhookJSON posts an update the way Max does for a subscription
// registered with a secret: the secret is in the path and in the header.
func maxWebhookJSON(t *testing.T, ts *TestServer, path string, body any) (int, map[string]any) {
	t.Helper()
	raw, _ := json.Marshal(body)
	req, err := http.NewRequest(http.MethodPost, ts.URL(path), bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Max-Bot-Api-Secret", testMaxWebhookSecret)
	resp, err := ts.Client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

// Regression (R-035 defect 2): the webhook path used to be the first 20
// characters of the bot token, so every access log line leaked half the token.
func TestMaxWebhook_TokenPrefixPathIsGone(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := maxSecretTestServer(t, env, testMaxWebhookSecret)
	if got := postMaxWebhook(t, ts, "/webhooks/max/"+testMaxToken[:20], testMaxWebhookSecret,
		map[string]any{"update_type": "bot_started"}); got != http.StatusNotFound {
		t.Fatalf("token-prefix path: status=%d, want 404", got)
	}
}

func TestMaxWebhook_SecretPathAndHeaderLinkAccount(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	user := f.CreateUser(TestUserOpts{Email: "max_secret_link@test.local"})
	ts := maxSecretTestServer(t, env, testMaxWebhookSecret)
	payload := Handler{MaxBotToken: testMaxToken}.maxUserToken(user.ID)

	got := postMaxWebhook(t, ts, "/webhooks/max/"+testMaxWebhookSecret, testMaxWebhookSecret, map[string]any{
		"update_type": "bot_started", "chat_id": int64(999101), "payload": payload,
	})
	if got != http.StatusOK {
		t.Fatalf("valid secret: status=%d, want 200", got)
	}
	var chatID *int64
	if err := env.Pool.QueryRow(t.Context(), `SELECT max_chat_id FROM users WHERE id = $1`, user.ID).Scan(&chatID); err != nil {
		t.Fatal(err)
	}
	if chatID == nil || *chatID != 999101 {
		t.Fatalf("max_chat_id=%v, want 999101", chatID)
	}
}

// The path alone is not enough: Max sends the subscription secret in the
// X-Max-Bot-Api-Secret header, so a leaked URL (logs, referer) cannot forge updates.
func TestMaxWebhook_MissingOrWrongHeaderIsForbidden(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	user := f.CreateUser(TestUserOpts{Email: "max_secret_header@test.local"})
	ts := maxSecretTestServer(t, env, testMaxWebhookSecret)
	payload := Handler{MaxBotToken: testMaxToken}.maxUserToken(user.ID)
	body := map[string]any{"update_type": "bot_started", "chat_id": int64(999102), "payload": payload}

	for _, header := range []string{"", "whsec_wrong"} {
		if got := postMaxWebhook(t, ts, "/webhooks/max/"+testMaxWebhookSecret, header, body); got != http.StatusForbidden {
			t.Fatalf("header %q: status=%d, want 403", header, got)
		}
	}
	var chatID *int64
	if err := env.Pool.QueryRow(t.Context(), `SELECT max_chat_id FROM users WHERE id = $1`, user.ID).Scan(&chatID); err != nil {
		t.Fatal(err)
	}
	if chatID != nil {
		t.Fatalf("forged update linked chat %d", *chatID)
	}
}

// Fail closed: without a strong configured secret the route does not exist.
func TestMaxWebhook_NoRouteWithoutStrongSecret(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	for _, secret := range []string{"", "short-secret"} {
		ts := maxSecretTestServer(t, env, secret)
		for _, path := range []string{"/webhooks/max/" + testMaxToken[:20], "/webhooks/max/short-secret"} {
			if got := postMaxWebhook(t, ts, path, secret, map[string]any{"update_type": "bot_started"}); got != http.StatusNotFound {
				t.Fatalf("secret %q path %s: status=%d, want 404", secret, path, got)
			}
		}
	}
}
