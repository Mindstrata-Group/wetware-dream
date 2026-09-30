package httpapi

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

const redactTestToken = "123456:SECRET-token-value"

// newConnDroppingServer returns a server that closes every connection without
// answering, so the client gets a *url.Error that carries the request URL
// (with /bot<token>/ in the path for Telegram).
func newConnDroppingServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Error("response writer does not support hijacking")
			return
		}
		conn, _, err := hj.Hijack()
		if err != nil {
			t.Errorf("hijack: %v", err)
			return
		}
		_ = conn.Close()
	}))
	t.Cleanup(srv.Close)
	return srv
}

func assertNoToken(t *testing.T, where string, err error) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s: expected an error from the dropped connection", where)
	}
	msg := err.Error()
	if strings.Contains(msg, redactTestToken) || strings.Contains(msg, "SECRET-token") {
		t.Fatalf("%s: bot token leaked into the error text: %q", where, msg)
	}
	if !strings.Contains(msg, "<redacted>") {
		t.Fatalf("%s: expected the token to be replaced with <redacted>, got %q", where, msg)
	}
}

// Regression (R-035, defect 1): a network error from the Telegram Bot API is
// a *url.Error whose text includes the full URL with the bot token. Callers
// log it (lead_notifications.go) and store it (notification_channel_queue
// failure reason), so the returned error must never contain the token.
func TestSendTelegramMessage_NetworkErrorDoesNotLeakToken(t *testing.T) {
	t.Parallel()
	srv := newConnDroppingServer(t)
	h := Handler{TelegramBotToken: redactTestToken, TelegramAPIBaseURL: srv.URL, HTTPClient: srv.Client()}

	err := h.sendTelegramMessage(context.Background(), "42", "hello")
	assertNoToken(t, "sendTelegramMessage", err)

	// The error keeps its type so timeout/transport checks still work.
	var ue *url.Error
	if !errors.As(err, &ue) {
		t.Fatalf("expected *url.Error to survive redaction, got %T", err)
	}
}

func TestForwardUserMessageToTelegram_NetworkErrorDoesNotLeakToken(t *testing.T) {
	t.Parallel()
	srv := newConnDroppingServer(t)
	h := Handler{
		TelegramBotToken:   redactTestToken,
		TelegramChatID:     "-100500",
		TelegramAPIBaseURL: srv.URL,
		HTTPClient:         srv.Client(),
	}

	err := h.forwardUserMessageToTelegram(context.Background(), 1, 2, "Mode", "text")
	assertNoToken(t, "forwardUserMessageToTelegram", err)
}

// A malformed base URL makes http.NewRequest fail with a parse error that
// also quotes the whole URL, token included.
func TestSendTelegramMessage_BadBaseURLDoesNotLeakToken(t *testing.T) {
	t.Parallel()
	h := Handler{TelegramBotToken: redactTestToken, TelegramAPIBaseURL: "http://bad host\x7f"}

	err := h.sendTelegramMessage(context.Background(), "42", "hello")
	assertNoToken(t, "sendTelegramMessage with bad base URL", err)
}

// echoAuthTransport answers every request with 401 and echoes the
// Authorization header in the body, like a misconfigured proxy error page.
type echoAuthTransport struct{}

func (echoAuthTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusUnauthorized,
		Body:       io.NopCloser(strings.NewReader("bad token: " + r.Header.Get("Authorization"))),
		Header:     make(http.Header),
		Request:    r,
	}, nil
}

// Max sends the token in the Authorization header, so a network error does
// not carry it; an error body that echoes the header must not leak it either.
func TestSendMaxMessage_EchoedTokenDoesNotLeak(t *testing.T) {
	t.Parallel()
	h := Handler{MaxBotToken: redactTestToken, HTTPClient: &http.Client{Transport: echoAuthTransport{}}}

	err := h.sendMaxMessage(context.Background(), 42, "hello")
	assertNoToken(t, "sendMaxMessage", err)
	if !strings.Contains(err.Error(), "HTTP 401") {
		t.Fatalf("status must stay in the error text for blocked-bot detection: %q", err.Error())
	}
}

// Blocked-bot detection reads the error text; redaction must keep it intact.
func TestRedactSecretInError_KeepsTextAroundSecret(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		err    error
		secret string
		want   string
	}{
		{"nil error", nil, "s", ""},
		{"empty secret", errors.New("telegram sendMessage: HTTP 403: bot was blocked"), "", "telegram sendMessage: HTTP 403: bot was blocked"},
		{"no secret inside", errors.New("telegram sendMessage: HTTP 403: bot was blocked"), redactTestToken, "telegram sendMessage: HTTP 403: bot was blocked"},
		{"plain error", errors.New("GET /bot" + redactTestToken + "/getMe: boom"), redactTestToken, "GET /bot<redacted>/getMe: boom"},
		{
			"url error",
			&url.Error{Op: "Post", URL: "https://api.example/bot" + redactTestToken + "/sendMessage", Err: errors.New("EOF")},
			redactTestToken,
			`Post "https://api.example/bot<redacted>/sendMessage": EOF`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := redactSecretInError(tc.err, tc.secret)
			if tc.err == nil {
				if got != nil {
					t.Fatalf("nil in → %v out", got)
				}
				return
			}
			if got.Error() != tc.want {
				t.Fatalf("got %q, want %q", got.Error(), tc.want)
			}
		})
	}
}
