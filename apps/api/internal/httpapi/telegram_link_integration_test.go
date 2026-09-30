//go:build integration

package httpapi

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"mindstrata-stage1/api/internal/testsupport"
)

// tgLinkTransport returns the given getUpdates response and 200 ok to all other requests.
type tgLinkTransport struct {
	updatesJSON string
}

func (t tgLinkTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	body := `{"ok":true}`
	if strings.Contains(req.URL.Path, "/getUpdates") && !strings.Contains(req.URL.RawQuery, "offset=") {
		body = t.updatesJSON
	}
	return &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	}, nil
}

func TestTelegramStartLink_LinksViaGetUpdatesAndGrantsBonusOnce(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	user := f.CreateUser(TestUserOpts{Email: "tg_link@test.local"})
	mode := f.CreateMode(TestModeOpts{})
	activeTo := time.Now().Add(24 * time.Hour)
	f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: mode.ID, ActiveTo: &activeTo, DailyMessageLimit: 20})

	seed := Handler{TelegramBotToken: "tg-test-token"}
	payload := seed.tgUserToken(user.ID)
	updates := fmt.Sprintf(`{"ok":true,"result":[{"update_id":10,"message":{"text":"/start %s","from":{"id":555001,"username":"tg_tester"},"chat":{"id":555001}}}]}`, payload)

	ts := NewTestServerWithHandler(t, env.Pool, Handler{
		TelegramBotToken:    "tg-test-token",
		TelegramBotUsername: "testtgbot",
		HTTPClient:          &http.Client{Transport: tgLinkTransport{updatesJSON: updates}},
	})
	ts.LoginAs(f.CreateSession(user.ID))

	// Poll start-link: the backend fetches getUpdates, matches the payload and links.
	status, body := httpJSON(t, ts, http.MethodGet, "/api/notifications/telegram/start-link", nil)
	if status != http.StatusOK || body["ok"] != true {
		t.Fatalf("status=%d body=%v, want 200 ok:true", status, body)
	}
	if link, _ := body["startLink"].(string); !strings.Contains(link, "t.me/testtgbot?start=") {
		t.Fatalf("startLink=%q, want t.me deeplink", link)
	}
	if body["telegramLinked"] != true {
		t.Fatalf("telegramLinked=%v, want true (привязка в этом же запросе)", body["telegramLinked"])
	}

	var tgID *int64
	var limit int64
	if err := env.Pool.QueryRow(context.Background(),
		`SELECT telegram_id FROM users WHERE id = $1`, user.ID).Scan(&tgID); err != nil {
		t.Fatalf("query user: %v", err)
	}
	if tgID == nil || *tgID != 555001 {
		t.Fatalf("telegram_id=%v, want 555001", tgID)
	}
	if err := env.Pool.QueryRow(context.Background(),
		`SELECT daily_message_limit FROM user_mode_access WHERE user_id = $1`, user.ID).Scan(&limit); err != nil {
		t.Fatalf("query limit: %v", err)
	}
	if limit != 22 {
		t.Fatalf("daily_message_limit=%d, want 22 (бонус за привязку)", limit)
	}

	// Unlinking right after linking is blocked for a day (as with Max).
	status, body = httpJSON(t, ts, http.MethodPost, "/api/notifications/telegram/unlink", nil)
	if status != http.StatusOK || body["ok"] != false || body["code"] != "cooldown" {
		t.Fatalf("unlink до суток: status=%d body=%v, want cooldown", status, body)
	}
	// A day later unlinking goes through.
	if _, err := env.Pool.Exec(context.Background(),
		`UPDATE users SET telegram_linked_at = now() - interval '25 hours' WHERE id = $1`, user.ID); err != nil {
		t.Fatalf("shift telegram_linked_at: %v", err)
	}
	status, body = httpJSON(t, ts, http.MethodPost, "/api/notifications/telegram/unlink", nil)
	if status != http.StatusOK || body["ok"] != true || body["telegramLinked"] != false {
		t.Fatalf("unlink: status=%d body=%v", status, body)
	}
	if err := env.Pool.QueryRow(context.Background(),
		`SELECT telegram_id FROM users WHERE id = $1`, user.ID).Scan(&tgID); err != nil {
		t.Fatalf("query user: %v", err)
	}
	if tgID != nil {
		t.Fatalf("telegram_id=%v, want NULL after unlink", tgID)
	}

	// Linking again: the bonus is NOT repeated (ledger).
	_, body = httpJSON(t, ts, http.MethodGet, "/api/notifications/telegram/start-link", nil)
	if body["telegramLinked"] != true {
		t.Fatalf("re-link failed: %v", body)
	}
	if err := env.Pool.QueryRow(context.Background(),
		`SELECT daily_message_limit FROM user_mode_access WHERE user_id = $1`, user.ID).Scan(&limit); err != nil {
		t.Fatalf("query limit: %v", err)
	}
	if limit != 22 {
		t.Fatalf("daily_message_limit=%d, want 22 (без повторного бонуса)", limit)
	}
}

func TestTgParseToken_RejectsForgery(t *testing.T) {
	t.Parallel()
	h := Handler{TelegramBotToken: "tg-test-token"}
	token := h.tgUserToken(42)
	if id, ok := h.tgParseToken(token); !ok || id != 42 {
		t.Fatalf("valid token rejected: id=%d ok=%v", id, ok)
	}
	if _, ok := h.tgParseToken("42_deadbeefdeadbeef"); ok {
		t.Fatalf("forged token accepted")
	}
	if _, ok := h.tgParseToken("mangled"); ok {
		t.Fatalf("mangled token accepted")
	}
	other := Handler{TelegramBotToken: "another-secret"}
	if _, ok := other.tgParseToken(token); ok {
		t.Fatalf("token for different secret accepted")
	}
}

func TestMaskEmailLabel(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"petrov.ivan@example.com": "pe***@example.com",
		"a@b.c":                   "a***@b.c",
		"аккаунт #7":              "аккаунт #7",
	}
	for in, want := range cases {
		if got := maskEmailLabel(in); got != want {
			t.Fatalf("maskEmailLabel(%q)=%q, want %q", in, got, want)
		}
	}
}

func TestIsMessengerBlockedError(t *testing.T) {
	t.Parallel()
	if isMessengerBlockedError(nil) {
		t.Fatalf("nil must not be blocked")
	}
	if !isMessengerBlockedError(fmt.Errorf("telegram sendMessage: HTTP 403: Forbidden: bot was blocked by the user")) {
		t.Fatalf("blocked error not detected")
	}
	if isMessengerBlockedError(fmt.Errorf("timeout")) {
		t.Fatalf("timeout wrongly detected as blocked")
	}
}

func TestUnlinkBlockedMessenger_ClearsLink(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	user := f.CreateUser(TestUserOpts{Email: "blocked_unlink@test.local"})
	if _, err := env.Pool.Exec(context.Background(),
		`UPDATE users SET max_chat_id = 111, telegram_id = 222 WHERE id = $1`, user.ID); err != nil {
		t.Fatalf("link: %v", err)
	}
	h := Handler{DB: env.Pool}
	h.unlinkBlockedMessenger(user.ID, "max")
	h.unlinkBlockedMessenger(user.ID, "telegram")
	var maxID, tgID *int64
	if err := env.Pool.QueryRow(context.Background(),
		`SELECT max_chat_id, telegram_id FROM users WHERE id = $1`, user.ID).Scan(&maxID, &tgID); err != nil {
		t.Fatalf("query: %v", err)
	}
	if maxID != nil || tgID != nil {
		t.Fatalf("links not cleared: max=%v tg=%v", maxID, tgID)
	}
}
