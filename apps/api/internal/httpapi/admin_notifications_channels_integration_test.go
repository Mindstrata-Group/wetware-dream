//go:build integration

package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"mindstrata-stage1/api/internal/testsupport"
)

// notifTestServer: a server with Max and Telegram configured; outgoing HTTP
// is swallowed by noopMaxTransport (always 200).
func notifTestServer(t testing.TB, env *testsupport.Env) *TestServer {
	t.Helper()
	return NewTestServerWithHandler(t, env.Pool, Handler{
		MaxBotToken:      testMaxToken,
		MaxBotUsername:   "testbot",
		TelegramBotToken: "tg-test-token",
		HTTPClient:       &http.Client{Transport: noopMaxTransport{}},
	})
}

func TestAdminNotificationPreview_AdminsAudience(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	admin := f.CreateUser(TestUserOpts{Role: "admin", Email: "admins_aud@test.local"})
	f.CreateUser(TestUserOpts{Email: "admins_aud_user@test.local"})

	ts := notifTestServer(t, env)
	ts.LoginAs(f.CreateSession(admin.ID))

	// Audience "Admins": only the admin, a regular user is not included.
	_, body := httpJSON(t, ts, http.MethodPost, "/api/admin/notifications/preview", map[string]any{
		"audience": "admins",
	})
	if got := body["recipientCount"].(float64); got != 1 {
		t.Fatalf("admins recipientCount=%v, want 1", got)
	}
}

func TestAdminNotificationPreview_CountsAndContacts(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	admin := f.CreateUser(TestUserOpts{Role: "admin", Email: "notif_prev_admin@test.local"})
	linked := f.CreateUser(TestUserOpts{Email: "notif_prev_max@test.local"})
	f.CreateUser(TestUserOpts{Email: "notif_prev_plain@test.local"})
	if _, err := env.Pool.Exec(context.Background(),
		`UPDATE users SET max_chat_id = 111222 WHERE id = $1`, linked.ID); err != nil {
		t.Fatalf("link max: %v", err)
	}
	// The phone comes from OAuth into notification_contacts, NOT users.phone:
	// the preview must see it there too (the real Yandex login case).
	if _, err := env.Pool.Exec(context.Background(),
		`INSERT INTO notification_contacts (user_id, channel, address, verified, source)
		 VALUES ($1, 'phone', '+79001234567', true, 'yandex_id')`, linked.ID); err != nil {
		t.Fatalf("insert phone contact: %v", err)
	}

	ts := notifTestServer(t, env)
	ts.LoginAs(f.CreateSession(admin.ID))

	status, body := httpJSON(t, ts, http.MethodPost, "/api/admin/notifications/preview", map[string]any{
		"audience": "all",
	})
	if status != http.StatusOK || body["ok"] != true {
		t.Fatalf("status=%d body=%v, want 200 ok:true", status, body)
	}
	if got := body["recipientCount"].(float64); got < 3 {
		t.Fatalf("recipientCount=%v, want >= 3", got)
	}
	if got := body["maxLinkedCount"].(float64); got != 1 {
		t.Fatalf("maxLinkedCount=%v, want 1", got)
	}
	if got := body["phoneCount"].(float64); got != 1 {
		t.Fatalf("phoneCount=%v, want 1 (телефон из notification_contacts)", got)
	}
	recipients, _ := body["recipients"].([]any)
	if len(recipients) == 0 {
		t.Fatalf("recipients empty, want list with contacts")
	}
}

func TestAdminNotificationPreview_ChannelFilterNarrowsAudience(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	admin := f.CreateUser(TestUserOpts{Role: "admin", Email: "notif_chf_admin@test.local"})
	linked := f.CreateUser(TestUserOpts{Email: "notif_chf_max@test.local"})
	f.CreateUser(TestUserOpts{Email: "notif_chf_plain@test.local"})
	if _, err := env.Pool.Exec(context.Background(),
		`UPDATE users SET max_chat_id = 777888 WHERE id = $1`, linked.ID); err != nil {
		t.Fatalf("link max: %v", err)
	}

	ts := notifTestServer(t, env)
	ts.LoginAs(f.CreateSession(admin.ID))

	// Max-only channel: the "all" audience narrows to users who linked Max.
	status, body := httpJSON(t, ts, http.MethodPost, "/api/admin/notifications/preview", map[string]any{
		"audience": "all",
		"channels": []string{"max"},
	})
	if status != http.StatusOK || body["ok"] != true {
		t.Fatalf("status=%d body=%v, want 200 ok:true", status, body)
	}
	if got := body["recipientCount"].(float64); got != 1 {
		t.Fatalf("recipientCount=%v, want 1 (только привязавший Max)", got)
	}
	recipients, _ := body["recipients"].([]any)
	if len(recipients) != 1 {
		t.Fatalf("recipients len=%d, want 1", len(recipients))
	}
	first, _ := recipients[0].(map[string]any)
	if first["maxLinked"] != true {
		t.Fatalf("recipient not maxLinked: %v", first)
	}

	// inbox among channels: the filter does not narrow (inbox is available to everyone).
	_, body = httpJSON(t, ts, http.MethodPost, "/api/admin/notifications/preview", map[string]any{
		"audience": "all",
		"channels": []string{"inbox", "max"},
	})
	if got := body["recipientCount"].(float64); got < 3 {
		t.Fatalf("recipientCount=%v, want >= 3 (inbox не сужает)", got)
	}
}

func TestAdminNotificationSend_ChannelsDeliverAndRecordHistory(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	admin := f.CreateUser(TestUserOpts{Role: "admin", Email: "notif_ch_admin@test.local"})
	maxUser := f.CreateUser(TestUserOpts{Email: "notif_ch_max@test.local"})
	tgUser := f.CreateUser(TestUserOpts{Email: "notif_ch_tg@test.local"})
	if _, err := env.Pool.Exec(context.Background(),
		`UPDATE users SET max_chat_id = 333444 WHERE id = $1`, maxUser.ID); err != nil {
		t.Fatalf("link max: %v", err)
	}
	if _, err := env.Pool.Exec(context.Background(),
		`UPDATE users SET telegram_id = 555666 WHERE id = $1`, tgUser.ID); err != nil {
		t.Fatalf("link telegram: %v", err)
	}

	ts := notifTestServer(t, env)
	ts.LoginAs(f.CreateSession(admin.ID))

	status, body := httpJSON(t, ts, http.MethodPost, "/api/admin/notifications/send", map[string]any{
		"audience": "all",
		"title":    "Тест каналов",
		"body":     "**Жирный** и [ссылка](https://mindstrata.ru)",
		"channels": []string{"inbox", "max", "telegram"},
	})
	if status != http.StatusOK || body["ok"] != true {
		t.Fatalf("status=%d body=%v, want 200 ok:true", status, body)
	}
	delivered, _ := body["delivered"].(map[string]any)
	if delivered == nil {
		t.Fatalf("delivered missing in response: %v", body)
	}
	if got := delivered["inbox"].(float64); got < 3 {
		t.Fatalf("delivered.inbox=%v, want >= 3", got)
	}
	if got := delivered["max"].(float64); got != 1 {
		t.Fatalf("delivered.max=%v, want 1 (только привязанный)", got)
	}
	if got := delivered["telegram"].(float64); got != 1 {
		t.Fatalf("delivered.telegram=%v, want 1 (только привязанный)", got)
	}

	// History is recorded.
	var count int
	if err := env.Pool.QueryRow(context.Background(),
		`SELECT count(*) FROM admin_notification_sends WHERE actor_id = $1 AND title = 'Тест каналов'`, admin.ID).Scan(&count); err != nil {
		t.Fatalf("query history: %v", err)
	}
	if count != 1 {
		t.Fatalf("history rows=%d, want 1", count)
	}

	// GET history returns the record.
	status, histBody := httpJSON(t, ts, http.MethodGet, "/api/admin/notifications/history", nil)
	if status != http.StatusOK || histBody["ok"] != true {
		t.Fatalf("history status=%d body=%v, want 200", status, histBody)
	}
	items, _ := histBody["items"].([]any)
	if len(items) == 0 {
		t.Fatalf("history items empty, want >= 1")
	}
}

func TestMessengerPlainText_SimplifiesMarkdown(t *testing.T) {
	t.Parallel()
	got := messengerPlainText("Заголовок", "**Жирный** текст и [ссылка](https://a.b)")
	want := "Заголовок\n\nЖирный текст и ссылка (https://a.b)"
	if got != want {
		t.Fatalf("messengerPlainText=%q, want %q", got, want)
	}
}

func TestNotificationConsentBonus_GrantedOnceOnFirstSubscribe(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	user := f.CreateUser(TestUserOpts{Email: "consent_bonus@test.local"})
	mode := f.CreateMode(TestModeOpts{})
	activeTo := time.Now().Add(24 * time.Hour)
	f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: mode.ID, ActiveTo: &activeTo, DailyMessageLimit: 20})

	ts := notifTestServer(t, env)
	ts.LoginAs(f.CreateSession(user.ID))

	// First time a subscription is turned on: +2 bonus (10% of 20, min 2).
	status, body := httpJSON(t, ts, http.MethodPatch, "/api/notifications/preferences", map[string]any{
		"channel": "email", "consentType": "marketing", "granted": true, "reason": "тест",
	})
	if status != http.StatusOK || body["ok"] != true {
		t.Fatalf("status=%d body=%v, want 200 ok:true", status, body)
	}
	if got := body["grantedBonus"].(float64); got != 2 {
		t.Fatalf("grantedBonus=%v, want 2", got)
	}
	var limit int64
	if err := env.Pool.QueryRow(context.Background(),
		`SELECT daily_message_limit FROM user_mode_access WHERE user_id = $1 AND mode_id = $2`, user.ID, mode.ID).Scan(&limit); err != nil {
		t.Fatalf("query limit: %v", err)
	}
	if limit != 22 {
		t.Fatalf("daily_message_limit=%d, want 22", limit)
	}

	// Turned off and on again: the bonus is not granted twice.
	httpJSON(t, ts, http.MethodPatch, "/api/notifications/preferences", map[string]any{
		"channel": "email", "consentType": "marketing", "granted": false, "reason": "тест",
	})
	_, body = httpJSON(t, ts, http.MethodPatch, "/api/notifications/preferences", map[string]any{
		"channel": "email", "consentType": "marketing", "granted": true, "reason": "тест",
	})
	if got := body["grantedBonus"].(float64); got != 0 {
		t.Fatalf("grantedBonus при повторном включении=%v, want 0", got)
	}
	if err := env.Pool.QueryRow(context.Background(),
		`SELECT daily_message_limit FROM user_mode_access WHERE user_id = $1 AND mode_id = $2`, user.ID, mode.ID).Scan(&limit); err != nil {
		t.Fatalf("query limit: %v", err)
	}
	if limit != 22 {
		t.Fatalf("daily_message_limit=%d, want 22 (без повторного бонуса)", limit)
	}

	// A different subscription: a separate bonus.
	_, body = httpJSON(t, ts, http.MethodPatch, "/api/notifications/preferences", map[string]any{
		"channel": "in_site", "consentType": "marketing", "granted": true, "reason": "тест",
	})
	if got := body["grantedBonus"].(float64); got < 2 {
		t.Fatalf("grantedBonus за другую подписку=%v, want >= 2", got)
	}
}

func TestAdminNotificationPreview_PromoSubgroups(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	admin := f.CreateUser(TestUserOpts{Role: "admin", Email: "promo_sub_admin@test.local"})
	wrote := f.CreateUser(TestUserOpts{Email: "promo_sub_wrote@test.local"})
	silent := f.CreateUser(TestUserOpts{Email: "promo_sub_silent@test.local"})
	mode := f.CreateMode(TestModeOpts{})
	promo := f.CreatePromocode(TestPromocodeOpts{})
	for _, uid := range []int64{wrote.ID, silent.ID} {
		if _, err := env.Pool.Exec(context.Background(),
			`insert into promocode_usages (user_id, promocode_id, used_at) values ($1, $2, now())`,
			uid, promo.ID); err != nil {
			t.Fatalf("use promocode: %v", err)
		}
	}
	// wrote really wrote: a dialog + a message with role=user.
	dialog := f.CreateDialog(wrote.ID, mode.ID)
	f.AppendMessage(dialog.ID, "user", "привет")

	ts := notifTestServer(t, env)
	ts.LoginAs(f.CreateSession(admin.ID))

	check := func(filters []string, wantIDs ...int64) {
		t.Helper()
		_, body := httpJSON(t, ts, http.MethodPost, "/api/admin/notifications/preview", map[string]any{
			"audience": "promocode", "promocodeIds": []int64{promo.ID}, "promoFilters": filters,
		})
		recipients, _ := body["recipients"].([]any)
		got := map[int64]bool{}
		for _, r := range recipients {
			m, _ := r.(map[string]any)
			got[int64(m["id"].(float64))] = true
		}
		if len(got) != len(wantIDs) {
			t.Fatalf("filters=%v: got %d recipients (%v), want %d", filters, len(got), got, len(wantIDs))
		}
		for _, id := range wantIDs {
			if !got[id] {
				t.Fatalf("filters=%v: missing user %d in %v", filters, id, got)
			}
		}
	}
	check([]string{"wrote"}, wrote.ID)
	check([]string{"not_wrote"}, silent.ID)
	check([]string{"wrote", "not_wrote"}, wrote.ID, silent.ID)
	check(nil, wrote.ID, silent.ID)
}

func TestNotificationQueue_ProcessDueSendsAndMarks(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	user := f.CreateUser(TestUserOpts{Email: "queue_user@test.local"})
	if _, err := env.Pool.Exec(context.Background(),
		`UPDATE users SET max_chat_id = 424242 WHERE id = $1`, user.ID); err != nil {
		t.Fatalf("link max: %v", err)
	}
	ts := notifTestServer(t, env)
	h := ts.Handler

	// A due max message + a not-yet-due telegram one.
	if _, err := env.Pool.Exec(context.Background(), `
		INSERT INTO notification_channel_queue (user_id, channel, text, send_after) VALUES
		($1, 'max', 'due now', now() - interval '1 minute'),
		($1, 'telegram', 'later', now() + interval '1 hour')`, user.ID); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	sent, err := h.processDueNotificationQueue(context.Background())
	if err != nil {
		t.Fatalf("processDue: %v", err)
	}
	if sent != 1 {
		t.Fatalf("sent=%d, want 1 (только созревшее)", sent)
	}
	var sentCount, pendingCount int
	if err := env.Pool.QueryRow(context.Background(),
		`SELECT count(*) FILTER (WHERE sent_at IS NOT NULL), count(*) FILTER (WHERE sent_at IS NULL AND failed IS NULL)
		 FROM notification_channel_queue WHERE user_id = $1`, user.ID).Scan(&sentCount, &pendingCount); err != nil {
		t.Fatalf("query queue: %v", err)
	}
	if sentCount != 1 || pendingCount != 1 {
		t.Fatalf("sent=%d pending=%d, want 1/1", sentCount, pendingCount)
	}
}

// TestFunctional_SendTextLimitBoundary: the exact channel limit boundary.
// title(N) + body(M) + 4 <= limit passes; +1 character → 400.
func TestFunctional_SendTextLimitBoundary(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	admin := f.CreateUser(TestUserOpts{Role: "admin", Email: "limit_admin@test.local"})
	linked := f.CreateUser(TestUserOpts{Email: "limit_max@test.local"})
	if _, err := env.Pool.Exec(context.Background(),
		`UPDATE users SET max_chat_id = 121212 WHERE id = $1`, linked.ID); err != nil {
		t.Fatalf("link: %v", err)
	}
	ts := notifTestServer(t, env)
	ts.LoginAs(f.CreateSession(admin.ID))

	title := "T"
	fits := strings.Repeat("а", maxTextLimit-len([]rune(title))-4)
	status, body := httpJSON(t, ts, http.MethodPost, "/api/admin/notifications/send", map[string]any{
		"audience": "all", "channels": []string{"max"}, "title": title, "body": fits,
	})
	if status != http.StatusOK || body["ok"] != true {
		t.Fatalf("текст ровно в лимит должен пройти: status=%d body=%v", status, map[string]any{"ok": body["ok"], "error": body["error"]})
	}
	status, body = httpJSON(t, ts, http.MethodPost, "/api/admin/notifications/send", map[string]any{
		"audience": "all", "channels": []string{"max"}, "title": title, "body": fits + "б",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("лимит+1 должен дать 400: status=%d body=%v", status, body)
	}
}

// TestFunctional_SecondChannelQueue: the recipient has both messengers: the
// first channel goes at once, the second is queued with the given delay;
// delay=0 sends both at once.
func TestFunctional_SecondChannelQueue(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	admin := f.CreateUser(TestUserOpts{Role: "admin", Email: "queue2_admin@test.local"})
	both := f.CreateUser(TestUserOpts{Email: "queue2_both@test.local"})
	if _, err := env.Pool.Exec(context.Background(),
		`UPDATE users SET max_chat_id = 131313, telegram_id = 141414 WHERE id = $1`, both.ID); err != nil {
		t.Fatalf("link both: %v", err)
	}
	ts := notifTestServer(t, env)
	ts.LoginAs(f.CreateSession(admin.ID))

	status, body := httpJSON(t, ts, http.MethodPost, "/api/admin/notifications/send", map[string]any{
		"audience": "all", "channels": []string{"max", "telegram"},
		"secondChannelDelayHours": 6.0,
		"title":                   "Очередь", "body": "тест",
	})
	if status != http.StatusOK {
		t.Fatalf("send: %d %v", status, body)
	}
	delivered := body["delivered"].(map[string]any)
	if delivered["max"].(float64) != 1 {
		t.Fatalf("первый канал (max) должен уйти сразу: %v", delivered)
	}
	if q, ok := delivered["queued"].(float64); !ok || q != 1 {
		t.Fatalf("второй канал должен встать в очередь: %v", delivered)
	}
	if tg, ok := delivered["telegram"].(float64); ok && tg != 0 {
		t.Fatalf("telegram не должен уйти сразу: %v", delivered)
	}
	var channel string
	var hoursToSend float64
	if err := env.Pool.QueryRow(context.Background(), `
		SELECT channel, EXTRACT(EPOCH FROM (send_after - now()))/3600
		FROM notification_channel_queue WHERE user_id = $1 AND sent_at IS NULL`, both.ID).
		Scan(&channel, &hoursToSend); err != nil {
		t.Fatalf("queue row: %v", err)
	}
	if channel != "telegram" {
		t.Fatalf("в очереди канал %q, want telegram (второй по порядку выбора)", channel)
	}
	if hoursToSend < 5.9 || hoursToSend > 6.1 {
		t.Fatalf("send_after через %.2f ч, want ~6", hoursToSend)
	}

	// delay=0: both channels at once, nothing is added to the queue.
	if _, err := env.Pool.Exec(context.Background(),
		`DELETE FROM notification_channel_queue WHERE user_id = $1`, both.ID); err != nil {
		t.Fatalf("clean queue: %v", err)
	}
	_, body = httpJSON(t, ts, http.MethodPost, "/api/admin/notifications/send", map[string]any{
		"audience": "all", "channels": []string{"max", "telegram"},
		"secondChannelDelayHours": 0.0,
		"title":                   "Сразу", "body": "тест",
	})
	delivered = body["delivered"].(map[string]any)
	if delivered["max"].(float64) != 1 || delivered["telegram"].(float64) != 1 {
		t.Fatalf("delay=0: оба канала должны уйти сразу: %v", delivered)
	}
	var queued int
	_ = env.Pool.QueryRow(context.Background(),
		`SELECT count(*) FROM notification_channel_queue WHERE user_id = $1 AND sent_at IS NULL`, both.ID).Scan(&queued)
	if queued != 0 {
		t.Fatalf("delay=0: очередь должна быть пуста, queued=%d", queued)
	}
}

// TestFunctional_MaxUnlinkCooldownBoundary: both sides of the 24-hour boundary.
func TestFunctional_MaxUnlinkCooldownBoundary(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	user := f.CreateUser(TestUserOpts{Email: "cooldown_b@test.local"})
	if _, err := env.Pool.Exec(context.Background(),
		`UPDATE users SET max_chat_id = 151515, max_linked_at = now() - interval '23 hours' WHERE id = $1`, user.ID); err != nil {
		t.Fatalf("link: %v", err)
	}
	ts := notifTestServer(t, env)
	ts.LoginAs(f.CreateSession(user.ID))

	_, body := httpJSON(t, ts, http.MethodPost, "/api/notifications/max/unlink", nil)
	if body["ok"] != false || body["code"] != "cooldown" {
		t.Fatalf("23ч после привязки: %v, want cooldown", body)
	}
	if hl := body["hoursLeft"].(float64); hl != 1 {
		t.Fatalf("hoursLeft=%v, want 1 (ceil остатка)", hl)
	}
	if _, err := env.Pool.Exec(context.Background(),
		`UPDATE users SET max_linked_at = now() - interval '24 hours 1 minute' WHERE id = $1`, user.ID); err != nil {
		t.Fatalf("shift: %v", err)
	}
	_, body = httpJSON(t, ts, http.MethodPost, "/api/notifications/max/unlink", nil)
	if body["ok"] != true {
		t.Fatalf("24ч+1мин: %v, want ok", body)
	}
}

// TestFunctional_HistoryPaginationAndDates: total/limit/offset and a date range.
func TestFunctional_HistoryPaginationAndDates(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	admin := f.CreateUser(TestUserOpts{Role: "admin", Email: "hist_admin@test.local"})
	for i, daysAgo := range []int{0, 1, 5} {
		if _, err := env.Pool.Exec(context.Background(), `
			INSERT INTO admin_notification_sends (actor_id, audience, channels, title, body, recipient_count, created_at)
			VALUES ($1, 'all', '{inbox}', $2, 'b', 1, now() - make_interval(days => $3))`,
			admin.ID, fmt.Sprintf("H%d", i), daysAgo); err != nil {
			t.Fatalf("seed history: %v", err)
		}
	}
	ts := notifTestServer(t, env)
	ts.LoginAs(f.CreateSession(admin.ID))

	status, body := httpJSON(t, ts, http.MethodGet, "/api/admin/notifications/history?limit=2&offset=0", nil)
	if status != http.StatusOK {
		t.Fatalf("history: %d %v", status, body)
	}
	if total := body["total"].(float64); total != 3 {
		t.Fatalf("total=%v, want 3", total)
	}
	if items := body["items"].([]any); len(items) != 2 {
		t.Fatalf("page1 len=%d, want 2", len(items))
	}
	_, body = httpJSON(t, ts, http.MethodGet, "/api/admin/notifications/history?limit=2&offset=2", nil)
	if items := body["items"].([]any); len(items) != 1 {
		t.Fatalf("page2 len=%d, want 1", len(items))
	}
	// Date range: the last 2 days (cuts off the 5-day-old record).
	from := time.Now().AddDate(0, 0, -2).Format("2006-01-02")
	_, body = httpJSON(t, ts, http.MethodGet, "/api/admin/notifications/history?from="+from, nil)
	if total := body["total"].(float64); total != 2 {
		t.Fatalf("from-фильтр: total=%v, want 2", total)
	}
	// A window that only the 5-day-old record falls into.
	to := time.Now().AddDate(0, 0, -3).Format("2006-01-02")
	fromOld := time.Now().AddDate(0, 0, -10).Format("2006-01-02")
	_, body = httpJSON(t, ts, http.MethodGet, "/api/admin/notifications/history?from="+fromOld+"&to="+to, nil)
	if total := body["total"].(float64); total != 1 {
		t.Fatalf("from+to окно: total=%v, want 1", total)
	}
}

// TestFunctional_QueueSkipsUnlinkedRecipient: by the time the queue sends,
// the recipient has unlinked: mark it failed, do not send.
func TestFunctional_QueueSkipsUnlinkedRecipient(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	user := f.CreateUser(TestUserOpts{Email: "queue_unlinked@test.local"})
	// A telegram message is queued, but the link is already gone.
	if _, err := env.Pool.Exec(context.Background(), `
		INSERT INTO notification_channel_queue (user_id, channel, text, send_after)
		VALUES ($1, 'telegram', 'x', now() - interval '1 minute')`, user.ID); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	ts := notifTestServer(t, env)
	sent, err := ts.Handler.processDueNotificationQueue(context.Background())
	if err != nil {
		t.Fatalf("processDue: %v", err)
	}
	if sent != 0 {
		t.Fatalf("sent=%d, want 0 (получатель отвязан)", sent)
	}
	var failed string
	if err := env.Pool.QueryRow(context.Background(),
		`SELECT COALESCE(failed, '') FROM notification_channel_queue WHERE user_id = $1`, user.ID).Scan(&failed); err != nil {
		t.Fatalf("query: %v", err)
	}
	if failed != "unlinked" {
		t.Fatalf("failed=%q, want unlinked", failed)
	}
}

func TestMarkdownToTelegramHTML(t *testing.T) {
	t.Parallel()
	got := markdownToTelegramHTML("**Жирный** <script> & [ссылка](https://a.b/?x=1&y=2)")
	want := `<b>Жирный</b> &lt;script&gt; &amp; <a href="https://a.b/?x=1&amp;y=2">ссылка</a>`
	if got != want {
		t.Fatalf("markdownToTelegramHTML=%q, want %q", got, want)
	}
}

// TestNotificationQueue_UnknownChannelMarkedFailed (World: the queue claims
// only max/telegram; a row with an unexpected channel value (for example, from
// schema drift after a future refactoring) must be marked failed, not silently
// skipped, and must not bring down processing of the whole batch).
func TestNotificationQueue_UnknownChannelMarkedFailed(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	user := f.CreateUser(TestUserOpts{Email: "queue_unknown_channel@test.local"})
	if _, err := env.Pool.Exec(context.Background(), `
		INSERT INTO notification_channel_queue (user_id, channel, text, send_after)
		VALUES ($1, 'sms', 'x', now() - interval '1 minute')`, user.ID); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	ts := notifTestServer(t, env)
	sent, err := ts.Handler.processDueNotificationQueue(context.Background())
	if err != nil {
		t.Fatalf("processDue: %v", err)
	}
	if sent != 0 {
		t.Fatalf("sent=%d, want 0 (unknown channel must not be delivered)", sent)
	}
	var failed string
	if err := env.Pool.QueryRow(context.Background(),
		`SELECT COALESCE(failed, '') FROM notification_channel_queue WHERE user_id = $1`, user.ID).Scan(&failed); err != nil {
		t.Fatalf("query: %v", err)
	}
	if failed != "unknown channel" {
		t.Fatalf("failed=%q, want %q", failed, "unknown channel")
	}
}
