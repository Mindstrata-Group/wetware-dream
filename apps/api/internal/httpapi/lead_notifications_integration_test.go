//go:build integration

package httpapi

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"mindstrata-stage1/api/internal/testsupport"
)

// recordingMaxTransport records outgoing requests to botapi.max.ru (URL and
// body) and swallows the rest (AI providers) with an empty ok response: the
// summary then falls back to the user's last messages.
type recordingMaxTransport struct {
	mu   sync.Mutex
	sent []recordedMaxSend
}

type recordedMaxSend struct {
	URL  string
	Body string
}

func (t *recordingMaxTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if strings.Contains(req.URL.Host, "botapi.max.ru") || strings.Contains(req.URL.Host, "api.telegram.org") {
		body := ""
		if req.Body != nil {
			b, _ := io.ReadAll(req.Body)
			body = string(b)
		}
		t.mu.Lock()
		t.sent = append(t.sent, recordedMaxSend{URL: req.URL.String(), Body: body})
		t.mu.Unlock()
	}
	return &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
		Header:     make(http.Header),
	}, nil
}

func (t *recordingMaxTransport) Sent() []recordedMaxSend {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]recordedMaxSend(nil), t.sent...)
}

// failingMaxTransport makes sending to Max (and everything else) fail.
type failingMaxTransport struct{}

func (failingMaxTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: 500,
		Body:       io.NopCloser(strings.NewReader(`{"ok":false}`)),
		Header:     make(http.Header),
	}, nil
}

func leadTestHandler(env *testsupport.Env, transport http.RoundTripper) Handler {
	return Handler{
		DB:               env.Pool,
		MaxBotToken:      testMaxToken,
		TelegramBotToken: "testtelegramtoken",
		HTTPClient:       &http.Client{Transport: transport},
		c:                newHandlerCaches(),
	}
}

// leadTestFixture: a user + a mode with the trigger enabled + a dialog with
// n user replies (and assistant replies between them).
func leadTestFixture(t *testing.T, env *testsupport.Env, threshold int, chatIDs string, userReplies int) (*TestUser, *TestMode, *TestDialog) {
	return leadTestFixtureFull(t, env, threshold, chatIDs, "", userReplies)
}

func leadTestFixtureFull(t *testing.T, env *testsupport.Env, threshold int, chatIDs, telegramIDs string, userReplies int) (*TestUser, *TestMode, *TestDialog) {
	t.Helper()
	f := NewFactory(t, env.Pool)
	user := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{})
	if _, err := env.Pool.Exec(context.Background(), `
		update modes set lead_notify_enabled = true, lead_notify_chat_ids = $2, lead_notify_telegram_ids = $3, lead_notify_threshold = $4
		where id = $1`, mode.ID, chatIDs, telegramIDs, threshold); err != nil {
		t.Fatalf("enable lead notify: %v", err)
	}
	dialog := f.CreateDialog(user.ID, mode.ID)
	for i := 0; i < userReplies; i++ {
		f.AppendMessage(dialog.ID, "user", "реплика пользователя про наболевшее")
		f.AppendMessage(dialog.ID, "assistant", "ответ ассистента")
	}
	return user, mode, dialog
}

func leadNotifiedAt(t *testing.T, env *testsupport.Env, dialogID int64) *string {
	t.Helper()
	var v *string
	if err := env.Pool.QueryRow(context.Background(),
		`select lead_notified_at::text from users_dialogs where id = $1`, dialogID).Scan(&v); err != nil {
		t.Fatalf("query lead_notified_at: %v", err)
	}
	return v
}

func TestLeadNotify_ThresholdReached_SendsOnceWithDetails(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	transport := &recordingMaxTransport{}
	h := leadTestHandler(env, transport)
	user, mode, dialog := leadTestFixture(t, env, 3, "111222333", 3)

	// A promo code activated by the user ends up in the notification.
	var promoID int64
	if err := env.Pool.QueryRow(context.Background(), `
		insert into promocodes (code, max_uses, used_count, duration, daily_message_limit, access_priority, grants_type, target_id, limit_type)
		values ('LEAD-TEST-CODE', 10, 1, interval '30 days', 50, 0, 'mode', $1, 'shared')
		returning id`, mode.ID).Scan(&promoID); err != nil {
		t.Fatalf("insert promocode: %v", err)
	}
	if _, err := env.Pool.Exec(context.Background(),
		`insert into promocode_usages (user_id, promocode_id) values ($1, $2)`, user.ID, promoID); err != nil {
		t.Fatalf("insert promocode usage: %v", err)
	}

	h.maybeSendLeadNotification(context.Background(), user.ID, dialog.ID, mode.ID, mode.Name)

	sent := transport.Sent()
	if len(sent) != 1 {
		t.Fatalf("отправлено %d уведомлений, want 1: %+v", len(sent), sent)
	}
	if !strings.Contains(sent[0].URL, "chat_id=111222333") {
		t.Fatalf("уведомление ушло не туда: %s", sent[0].URL)
	}
	for _, needle := range []string{"#", user.Email, "LEAD-TEST-CODE", mode.Name} {
		if !strings.Contains(sent[0].Body, needle) {
			t.Fatalf("в уведомлении нет %q: %s", needle, sent[0].Body)
		}
	}
	// AI is unavailable in the test → a fallback summary from the user's last messages.
	if !strings.Contains(sent[0].Body, "наболевшее") {
		t.Fatalf("в уведомлении нет фолбек-выжимки: %s", sent[0].Body)
	}
	if leadNotifiedAt(t, env, dialog.ID) == nil {
		t.Fatalf("lead_notified_at не выставлен")
	}

	// A repeated call (the next message): no duplicate.
	h.maybeSendLeadNotification(context.Background(), user.ID, dialog.ID, mode.ID, mode.Name)
	if got := len(transport.Sent()); got != 1 {
		t.Fatalf("после повторного вызова %d уведомлений, want 1 (дедуп)", got)
	}
}

func TestLeadNotify_MultipleRecipients(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	transport := &recordingMaxTransport{}
	h := leadTestHandler(env, transport)
	user, mode, dialog := leadTestFixture(t, env, 2, "111, 222", 2)

	h.maybeSendLeadNotification(context.Background(), user.ID, dialog.ID, mode.ID, mode.Name)

	sent := transport.Sent()
	if len(sent) != 2 {
		t.Fatalf("отправлено %d, want 2 (оба получателя): %+v", len(sent), sent)
	}
	both := sent[0].URL + " " + sent[1].URL
	if !strings.Contains(both, "chat_id=111") || !strings.Contains(both, "chat_id=222") {
		t.Fatalf("не все получатели: %s", both)
	}
}

func TestLeadNotify_TelegramOnly(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	transport := &recordingMaxTransport{}
	h := leadTestHandler(env, transport)
	user, mode, dialog := leadTestFixtureFull(t, env, 2, "", "555666777", 2)

	h.maybeSendLeadNotification(context.Background(), user.ID, dialog.ID, mode.ID, mode.Name)

	sent := transport.Sent()
	if len(sent) != 1 {
		t.Fatalf("отправлено %d, want 1 (только telegram): %+v", len(sent), sent)
	}
	if !strings.Contains(sent[0].URL, "api.telegram.org") || !strings.Contains(sent[0].Body, "555666777") {
		t.Fatalf("уведомление ушло не в telegram или не тому chat_id: %s / %s", sent[0].URL, sent[0].Body)
	}
	if leadNotifiedAt(t, env, dialog.ID) == nil {
		t.Fatalf("lead_notified_at не выставлен")
	}
}

func TestLeadNotify_BothChannels(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	transport := &recordingMaxTransport{}
	h := leadTestHandler(env, transport)
	user, mode, dialog := leadTestFixtureFull(t, env, 2, "111222333", "555666777", 2)

	h.maybeSendLeadNotification(context.Background(), user.ID, dialog.ID, mode.ID, mode.Name)

	sent := transport.Sent()
	if len(sent) != 2 {
		t.Fatalf("отправлено %d, want 2 (max + telegram): %+v", len(sent), sent)
	}
	hosts := sent[0].URL + " " + sent[1].URL
	if !strings.Contains(hosts, "botapi.max.ru") || !strings.Contains(hosts, "api.telegram.org") {
		t.Fatalf("не оба канала сработали: %s", hosts)
	}
}

func TestLeadNotify_BelowThreshold_NoSend(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	transport := &recordingMaxTransport{}
	h := leadTestHandler(env, transport)
	user, mode, dialog := leadTestFixture(t, env, 4, "111222333", 3)

	h.maybeSendLeadNotification(context.Background(), user.ID, dialog.ID, mode.ID, mode.Name)

	if got := len(transport.Sent()); got != 0 {
		t.Fatalf("отправлено %d уведомлений до порога, want 0", got)
	}
	if leadNotifiedAt(t, env, dialog.ID) != nil {
		t.Fatalf("lead_notified_at выставлен до порога")
	}
}

func TestLeadNotify_DisabledOrNoRecipients_NoSend(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	transport := &recordingMaxTransport{}
	h := leadTestHandler(env, transport)

	// Disabled.
	user, mode, dialog := leadTestFixture(t, env, 2, "111222333", 3)
	if _, err := env.Pool.Exec(context.Background(),
		`update modes set lead_notify_enabled = false where id = $1`, mode.ID); err != nil {
		t.Fatalf("disable: %v", err)
	}
	h.maybeSendLeadNotification(context.Background(), user.ID, dialog.ID, mode.ID, mode.Name)

	// Enabled, but there are no recipients.
	user2, mode2, dialog2 := leadTestFixture(t, env, 2, "", 3)
	h.maybeSendLeadNotification(context.Background(), user2.ID, dialog2.ID, mode2.ID, mode2.Name)

	if got := len(transport.Sent()); got != 0 {
		t.Fatalf("отправлено %d уведомлений, want 0", got)
	}
}

func TestLeadNotify_DeliveryFailed_ReleasesDedupeMark(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	h := leadTestHandler(env, failingMaxTransport{})
	user, mode, dialog := leadTestFixture(t, env, 2, "111222333", 2)

	h.maybeSendLeadNotification(context.Background(), user.ID, dialog.ID, mode.ID, mode.Name)

	if leadNotifiedAt(t, env, dialog.ID) != nil {
		t.Fatalf("после провала доставки пометка должна сниматься, чтобы лид не потерялся")
	}
}
