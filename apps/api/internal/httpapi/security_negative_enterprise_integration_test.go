//go:build integration

package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"mindstrata-stage1/api/internal/testsupport"
)

func TestEnterpriseSecurity_MalformedJSONAndOversizedBodies_ReturnClientErrors(t *testing.T) {
	t.Parallel()

	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	adminToken := f.CreateSession(admin.ID)

	cases := []struct {
		name   string
		method string
		path   string
		body   []byte
		auth   bool
	}{
		{"login_malformed", http.MethodPost, "/api/auth/login", []byte(`{"email":`), false},
		{"register_oversized", http.MethodPost, "/api/auth/register", bytes.Repeat([]byte("x"), 40<<10), false},
		{"profile_settings_malformed", http.MethodPatch, "/api/profile/settings", []byte(`{"allowMessageAnonymization":`), true},
		{"chat_send_oversized", http.MethodPost, "/api/chat/send", append([]byte(`{"text":"`), bytes.Repeat([]byte("x"), (1<<20)+1)...), true},
		{"promo_malformed", http.MethodPost, "/api/access/promocode/apply", []byte(`{"code":`), false},
		{"cookie_consent_oversized", http.MethodPost, "/api/cookie-consent", bytes.Repeat([]byte("x"), 5<<10), false},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			ts := NewTestServer(t, env.Pool)
			if tc.auth {
				ts.LoginAs(adminToken)
			}
			status, payload, raw := enterpriseJSONRequest(t, ts, tc.method, tc.path, tc.body)
			if status < 400 || status >= 500 {
				t.Fatalf("%s %s status=%d body=%s, want 4xx", tc.method, tc.path, status, raw)
			}
			if _, ok := payload["error"]; !ok {
				t.Fatalf("%s %s missing error payload: %s", tc.method, tc.path, raw)
			}
		})
	}
}

func TestEnterpriseSecurity_RateLimits_AuthPromoAndChat(t *testing.T) {
	t.Parallel()

	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)
	f := NewFactory(t, env.Pool)

	t.Run("auth_login", func(t *testing.T) {
		for i := 1; i <= 11; i++ {
			status, _, raw := enterpriseJSONRequest(t, ts, http.MethodPost, "/api/auth/login",
				[]byte(`{"email":"missing@example.test","password":"wrong-password"}`))
			if i <= 10 && status == http.StatusTooManyRequests {
				t.Fatalf("login attempt %d rate-limited too early: %s", i, raw)
			}
			if i == 11 && status != http.StatusTooManyRequests {
				t.Fatalf("login attempt %d status=%d body=%s, want 429", i, status, raw)
			}
		}
	})

	t.Run("auth_register", func(t *testing.T) {
		for i := 1; i <= 9; i++ {
			email := fmt.Sprintf("rate_register_%02d@test.local", i)
			status, _, raw := enterpriseJSONRequest(t, ts, http.MethodPost, "/api/auth/register",
				[]byte(fmt.Sprintf(`{"email":%q,"password":"short"}`, email)))
			if i <= 8 && status == http.StatusTooManyRequests {
				t.Fatalf("register attempt %d rate-limited too early: %s", i, raw)
			}
			if i == 9 && status != http.StatusTooManyRequests {
				t.Fatalf("register attempt %d status=%d body=%s, want 429", i, status, raw)
			}
		}
	})

	t.Run("promo", func(t *testing.T) {
		for i := 1; i <= 31; i++ {
			status, _, raw := enterpriseJSONRequest(t, ts, http.MethodPost, "/api/access/promocode/apply",
				[]byte(fmt.Sprintf(`{"code":"NO_SUCH_PROMO_%02d"}`, i)))
			if i <= 30 && status == http.StatusTooManyRequests {
				t.Fatalf("promo attempt %d rate-limited too early: %s", i, raw)
			}
			if i == 31 && status != http.StatusTooManyRequests {
				t.Fatalf("promo attempt %d status=%d body=%s, want 429", i, status, raw)
			}
		}
	})

	t.Run("chat_burst", func(t *testing.T) {
		user := f.CreateUser(TestUserOpts{})
		mode := f.CreateMode(TestModeOpts{})
		f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: mode.ID, DailyMessageLimit: 100})
		dialog := f.CreateDialog(user.ID, mode.ID)
		sessionToken := f.CreateSession(user.ID)

		const attempts = 31
		var ok, limited, unexpected atomic.Int64
		var wg sync.WaitGroup
		start := make(chan struct{})
		errors := make(chan string, attempts)

		for i := 1; i <= attempts; i++ {
			i := i
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start

				body := []byte(fmt.Sprintf(`{"dialogId":%d,"text":"hello %02d","responseMode":"test"}`, dialog.ID, i))
				status, payload, raw, err := enterpriseJSONRequestNoFatalWithSession(ts, http.MethodPost, "/api/chat/send", body, sessionToken)
				if err != nil {
					errors <- fmt.Sprintf("chat attempt %d failed: %v", i, err)
					unexpected.Add(1)
					return
				}
				switch status {
				case http.StatusOK:
					ok.Add(1)
				case http.StatusTooManyRequests:
					if code, _ := payload["code"].(string); code != "chat_burst_limit" {
						errors <- fmt.Sprintf("chat attempt %d 429 code=%q body=%s, want chat_burst_limit", i, code, raw)
						unexpected.Add(1)
						return
					}
					limited.Add(1)
				default:
					errors <- fmt.Sprintf("chat attempt %d status=%d body=%s, want 200/429", i, status, raw)
					unexpected.Add(1)
				}
			}()
		}
		close(start)
		wg.Wait()
		close(errors)

		for err := range errors {
			t.Error(err)
		}
		if got := unexpected.Load(); got != 0 {
			t.Fatalf("chat burst returned %d unexpected responses", got)
		}
		if got := limited.Load(); got < 1 {
			t.Fatalf("chat burst limited=%d ok=%d, want at least one 429", got, ok.Load())
		}
		if got := ok.Load(); got > 30 {
			t.Fatalf("chat burst ok=%d limited=%d, want at most 30 successful requests", got, limited.Load())
		}
	})
}

func enterpriseJSONRequestNoFatalWithSession(ts *TestServer, method, path string, body []byte, rawToken string) (int, map[string]any, string, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, ts.URL(path), reader)
	if err != nil {
		return 0, nil, "", fmt.Errorf("new request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: rawToken, Path: "/"})

	// The burst test checks the chat limit, not the cookie jar state between goroutines.
	client := *ts.Client
	client.Jar = nil
	return enterpriseDoJSONNoFatal(&client, req, method, path)
}

func enterpriseJSONRequestNoFatal(ts *TestServer, method, path string, body []byte) (int, map[string]any, string, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, ts.URL(path), reader)
	if err != nil {
		return 0, nil, "", fmt.Errorf("new request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	return enterpriseDoJSONNoFatal(ts.Client, req, method, path)
}

func enterpriseDoJSONNoFatal(client *http.Client, req *http.Request, method, path string) (int, map[string]any, string, error) {
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, "", fmt.Errorf("do %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	rawBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, "", fmt.Errorf("read response: %w", err)
	}
	raw := strings.TrimSpace(string(rawBytes))

	var payload map[string]any
	if err := json.Unmarshal(rawBytes, &payload); err != nil {
		return resp.StatusCode, nil, raw, fmt.Errorf("non-json body: %w", err)
	}
	return resp.StatusCode, payload, raw, nil
}

func TestEnterpriseSecurity_IDOR_ChatSendCannotUseAnotherUsersDialog(t *testing.T) {
	t.Parallel()

	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	owner := f.CreateUser(TestUserOpts{})
	attacker := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{})
	f.GrantAccess(GrantAccessOpts{UserID: owner.ID, ModeID: mode.ID})
	f.GrantAccess(GrantAccessOpts{UserID: attacker.ID, ModeID: mode.ID})
	foreignDialog := f.CreateDialog(owner.ID, mode.ID)

	ts := NewTestServer(t, env.Pool)
	ts.LoginAs(f.CreateSession(attacker.ID))

	status, _, raw := enterpriseJSONRequest(t, ts, http.MethodPost, "/api/chat/send",
		[]byte(fmt.Sprintf(`{"dialogId":%d,"text":"steal this","responseMode":"test"}`, foreignDialog.ID)))
	if status != http.StatusForbidden && status != http.StatusBadRequest {
		t.Fatalf("cross-user dialog send status=%d body=%s, want 403/400", status, raw)
	}

	var count int
	err := env.Pool.QueryRow(context.Background(), `
		select count(*)
		from dialogs_messages
		where dialog_id = $1
		  and content = 'steal this'`, foreignDialog.ID).Scan(&count)
	if err != nil {
		t.Fatalf("count foreign messages: %v", err)
	}
	if count != 0 {
		t.Fatalf("IDOR inserted %d messages into foreign dialog", count)
	}
}

func TestEnterpriseSecurity_ErrorPayloads_DoNotEchoSecretsOrPII(t *testing.T) {
	t.Parallel()

	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)

	secretEmail := "victim.enterprise@example.test"
	secretPassword := "CorrectHorseBatteryStapleSecret"
	secretToken := "sk-live-enterprise-secret-token"

	cases := []struct {
		name   string
		method string
		path   string
		body   []byte
	}{
		{
			name:   "login_invalid_credentials",
			method: http.MethodPost,
			path:   "/api/auth/login",
			body:   []byte(fmt.Sprintf(`{"email":%q,"password":%q}`, secretEmail, secretPassword)),
		},
		{
			name:   "register_malformed_json",
			method: http.MethodPost,
			path:   "/api/auth/register",
			body:   []byte(fmt.Sprintf(`{"email":%q,"password":"%s"`, secretEmail, secretToken)),
		},
		{
			name:   "promo_not_found",
			method: http.MethodPost,
			path:   "/api/access/promocode/apply",
			body:   []byte(fmt.Sprintf(`{"code":%q}`, secretToken)),
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			status, payload, raw := enterpriseJSONRequest(t, ts, tc.method, tc.path, tc.body)
			if status < 400 || status >= 500 {
				t.Fatalf("%s %s status=%d body=%s, want 4xx", tc.method, tc.path, status, raw)
			}
			lower := strings.ToLower(raw)
			for _, forbidden := range []string{
				strings.ToLower(secretEmail),
				strings.ToLower(secretPassword),
				strings.ToLower(secretToken),
				"password_hash",
				"postgres://",
				"sqlstate",
				"token_hash",
			} {
				if strings.Contains(lower, forbidden) {
					t.Fatalf("%s %s leaked %q in payload=%v", tc.method, tc.path, forbidden, payload)
				}
			}
			if _, hasDebug := payload["debug"]; hasDebug {
				t.Fatalf("%s %s included debug payload for negative public request: %s", tc.method, tc.path, raw)
			}
		})
	}
}
