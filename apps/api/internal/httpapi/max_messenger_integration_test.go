//go:build integration

package httpapi

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"mindstrata-stage1/api/internal/testsupport"
)

// noopMaxTransport swallows outgoing requests to botapi.max.ru without network calls.
type noopMaxTransport struct{}

func (noopMaxTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
		Header:     make(http.Header),
	}, nil
}

const testMaxToken = "testmaxtoken12345678" // exactly 20 characters → secret = the whole token

// maxTestServer creates a TestServer with MaxBotToken and noopMaxTransport configured.
func maxTestServer(t testing.TB, env *testsupport.Env) *TestServer {
	t.Helper()
	return NewTestServerWithHandler(t, env.Pool, Handler{
		MaxBotToken:      testMaxToken,
		MaxBotUsername:   "testbot",
		MaxWebhookSecret: testMaxWebhookSecret,
		HTTPClient:       &http.Client{Transport: noopMaxTransport{}},
	})
}

func TestMaxNotificationsStartLink_RequiresAuth(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := maxTestServer(t, env)

	status, body := httpJSON(t, ts, http.MethodGet, "/api/notifications/max/start-link", nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("unauthenticated: status=%d body=%v, want 401", status, body)
	}
}

func TestMaxNotificationsStartLink_AuthenticatedUser(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	user := f.CreateUser(TestUserOpts{Email: "max_link@test.local"})
	ts := maxTestServer(t, env)
	ts.LoginAs(f.CreateSession(user.ID))

	status, body := httpJSON(t, ts, http.MethodGet, "/api/notifications/max/start-link", nil)
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%v, want 200", status, body)
	}
	if body["ok"] != true {
		t.Fatalf("ok=%v, want true", body["ok"])
	}
	link, _ := body["startLink"].(string)
	if !strings.Contains(link, "max.ru/testbot?start=") {
		t.Fatalf("startLink=%q, want max.ru/testbot?start= deeplink", link)
	}
	if body["maxLinked"] != false {
		t.Fatalf("maxLinked=%v, want false (not yet linked)", body["maxLinked"])
	}
}

func TestMaxWebhook_InvalidSecret_ReturnsForbidden(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := maxTestServer(t, env)

	status, _ := httpJSON(t, ts, http.MethodPost, "/webhooks/max/wrongsecret", map[string]any{
		"update_type": "bot_started",
	})
	if status != http.StatusNotFound && status != http.StatusForbidden {
		t.Fatalf("wrong secret: status=%d, want 404 or 403", status)
	}
}

func TestMaxWebhook_BotStarted_LinksAccountAndGrantsBonus(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	user := f.CreateUser(TestUserOpts{Email: "max_bonus@test.local"})
	mode := f.CreateMode(TestModeOpts{})
	now := time.Now()
	activeTo := now.Add(24 * time.Hour)
	// daily_message_limit=20 → bonus=ceil(20*0.1)=2 → new limit=22
	f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: mode.ID, ActiveTo: &activeTo, DailyMessageLimit: 20})

	ts := maxTestServer(t, env)

	// Generate a valid payload token for the user
	h := Handler{MaxBotToken: testMaxToken}
	payload := h.maxUserToken(user.ID)

	webhookPath := "/webhooks/max/" + testMaxWebhookSecret
	status, body := maxWebhookJSON(t, ts, webhookPath, map[string]any{
		"update_type": "bot_started",
		"chat_id":     int64(999001),
		"user":        map[string]any{"user_id": 999001, "name": "Test User"},
		"payload":     payload,
	})
	if status != http.StatusOK {
		t.Fatalf("webhook status=%d body=%v, want 200", status, body)
	}

	// Check that max_chat_id is linked
	var chatID *int64
	var bonusGranted bool
	if err := env.Pool.QueryRow(context.Background(),
		`SELECT max_chat_id, max_bonus_granted FROM users WHERE id = $1`, user.ID,
	).Scan(&chatID, &bonusGranted); err != nil {
		t.Fatalf("query user: %v", err)
	}
	if chatID == nil || *chatID != 999001 {
		t.Fatalf("max_chat_id=%v, want 999001", chatID)
	}
	if !bonusGranted {
		t.Fatalf("max_bonus_granted=false, want true")
	}

	// Check that daily_message_limit increased by 10% (min 2)
	var newLimit int64
	if err := env.Pool.QueryRow(context.Background(),
		`SELECT daily_message_limit FROM user_mode_access WHERE user_id = $1 AND mode_id = $2`, user.ID, mode.ID,
	).Scan(&newLimit); err != nil {
		t.Fatalf("query daily_message_limit: %v", err)
	}
	if newLimit != 22 {
		t.Fatalf("daily_message_limit=%d, want 22 (20 + 10%% = +2 bonus)", newLimit)
	}
}

func TestMaxWebhook_BotStarted_NoBonusTwice(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	user := f.CreateUser(TestUserOpts{Email: "max_nodup@test.local"})
	mode := f.CreateMode(TestModeOpts{})
	now := time.Now()
	activeTo := now.Add(24 * time.Hour)
	// daily_message_limit=20 → bonus=2 → after 2 webhooks the limit must be 22 (not 24)
	f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: mode.ID, ActiveTo: &activeTo, DailyMessageLimit: 20})

	ts := maxTestServer(t, env)
	h := Handler{MaxBotToken: testMaxToken}
	payload := h.maxUserToken(user.ID)
	webhookPath := "/webhooks/max/" + testMaxWebhookSecret

	// First webhook
	maxWebhookJSON(t, ts, webhookPath, map[string]any{
		"update_type": "bot_started", "chat_id": int64(999002), "payload": payload,
	})

	// Second webhook (re-linking: the bonus must not be duplicated)
	maxWebhookJSON(t, ts, webhookPath, map[string]any{
		"update_type": "bot_started", "chat_id": int64(999002), "payload": payload,
	})

	// daily_message_limit must grow by exactly 2 (not 4)
	var newLimit int64
	if err := env.Pool.QueryRow(context.Background(),
		`SELECT daily_message_limit FROM user_mode_access WHERE user_id = $1 AND mode_id = $2`, user.ID, mode.ID,
	).Scan(&newLimit); err != nil {
		t.Fatalf("query daily_message_limit: %v", err)
	}
	if newLimit != 22 {
		t.Fatalf("daily_message_limit=%d, want 22 (bonus granted only once)", newLimit)
	}
}

func TestMaxWebhook_SameChatSecondAccount_NoBonus(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	first := f.CreateUser(TestUserOpts{Email: "max_multi_1@test.local"})
	second := f.CreateUser(TestUserOpts{Email: "max_multi_2@test.local"})
	mode := f.CreateMode(TestModeOpts{})
	activeTo := time.Now().Add(24 * time.Hour)
	f.GrantAccess(GrantAccessOpts{UserID: first.ID, ModeID: mode.ID, ActiveTo: &activeTo, DailyMessageLimit: 20})
	f.GrantAccess(GrantAccessOpts{UserID: second.ID, ModeID: mode.ID, ActiveTo: &activeTo, DailyMessageLimit: 20})

	ts := maxTestServer(t, env)
	h := Handler{MaxBotToken: testMaxToken}
	webhookPath := "/webhooks/max/" + testMaxWebhookSecret
	sharedChat := int64(999020)

	// First account: the bonus is granted (20 → 22).
	maxWebhookJSON(t, ts, webhookPath, map[string]any{
		"update_type": "bot_started", "chat_id": sharedChat, "payload": h.maxUserToken(first.ID),
	})
	// A second account links THE SAME Max chat: linking works, no bonus.
	maxWebhookJSON(t, ts, webhookPath, map[string]any{
		"update_type": "bot_started", "chat_id": sharedChat, "payload": h.maxUserToken(second.ID),
	})

	var firstLimit, secondLimit int64
	var secondChat *int64
	if err := env.Pool.QueryRow(context.Background(),
		`SELECT daily_message_limit FROM user_mode_access WHERE user_id = $1`, first.ID).Scan(&firstLimit); err != nil {
		t.Fatalf("first limit: %v", err)
	}
	if err := env.Pool.QueryRow(context.Background(),
		`SELECT daily_message_limit FROM user_mode_access WHERE user_id = $1`, second.ID).Scan(&secondLimit); err != nil {
		t.Fatalf("second limit: %v", err)
	}
	if err := env.Pool.QueryRow(context.Background(),
		`SELECT max_chat_id FROM users WHERE id = $1`, second.ID).Scan(&secondChat); err != nil {
		t.Fatalf("second chat: %v", err)
	}
	if firstLimit != 22 {
		t.Fatalf("first daily_message_limit=%d, want 22 (бонус первому)", firstLimit)
	}
	if secondLimit != 20 {
		t.Fatalf("second daily_message_limit=%d, want 20 (без бонуса за тот же чат)", secondLimit)
	}
	if secondChat == nil || *secondChat != sharedChat {
		t.Fatalf("second max_chat_id=%v, want %d (привязка разрешена)", secondChat, sharedChat)
	}
}

func TestMaxNotificationsUnlink_CooldownBlocks(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	user := f.CreateUser(TestUserOpts{Email: "max_unlink_cd@test.local"})
	ts := maxTestServer(t, env)

	h := Handler{MaxBotToken: testMaxToken}
	payload := h.maxUserToken(user.ID)
	// Linking via webhook sets max_linked_at to now().
	maxWebhookJSON(t, ts, "/webhooks/max/"+testMaxWebhookSecret, map[string]any{
		"update_type": "bot_started", "chat_id": int64(999010), "payload": payload,
	})

	ts.LoginAs(f.CreateSession(user.ID))
	status, body := httpJSON(t, ts, http.MethodPost, "/api/notifications/max/unlink", nil)
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%v, want 200", status, body)
	}
	if body["ok"] != false {
		t.Fatalf("ok=%v, want false (cooldown)", body["ok"])
	}
	if body["code"] != "cooldown" {
		t.Fatalf("code=%v, want cooldown", body["code"])
	}
	// The link is kept during the cooldown.
	var chatID *int64
	if err := env.Pool.QueryRow(context.Background(),
		`SELECT max_chat_id FROM users WHERE id = $1`, user.ID).Scan(&chatID); err != nil {
		t.Fatalf("query user: %v", err)
	}
	if chatID == nil {
		t.Fatalf("max_chat_id nil, want still linked during cooldown")
	}
}

func TestMaxNotificationsUnlink_AfterCooldown(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	user := f.CreateUser(TestUserOpts{Email: "max_unlink_ok@test.local"})
	ts := maxTestServer(t, env)

	h := Handler{MaxBotToken: testMaxToken}
	payload := h.maxUserToken(user.ID)
	maxWebhookJSON(t, ts, "/webhooks/max/"+testMaxWebhookSecret, map[string]any{
		"update_type": "bot_started", "chat_id": int64(999011), "payload": payload,
	})
	// Move the link one day back: the cooldown has expired.
	if _, err := env.Pool.Exec(context.Background(),
		`UPDATE users SET max_linked_at = now() - interval '25 hours' WHERE id = $1`, user.ID); err != nil {
		t.Fatalf("shift max_linked_at: %v", err)
	}

	ts.LoginAs(f.CreateSession(user.ID))
	status, body := httpJSON(t, ts, http.MethodPost, "/api/notifications/max/unlink", nil)
	if status != http.StatusOK || body["ok"] != true {
		t.Fatalf("status=%d body=%v, want 200 ok:true", status, body)
	}
	// max_chat_id is removed, but the bonus flag is kept (no second bonus).
	var chatID *int64
	var bonusGranted bool
	if err := env.Pool.QueryRow(context.Background(),
		`SELECT max_chat_id, max_bonus_granted FROM users WHERE id = $1`, user.ID).Scan(&chatID, &bonusGranted); err != nil {
		t.Fatalf("query user: %v", err)
	}
	if chatID != nil {
		t.Fatalf("max_chat_id=%v, want nil after unlink", chatID)
	}
	if !bonusGranted {
		t.Fatalf("max_bonus_granted=false, want true (persist to block re-bonus)")
	}
}

func TestMaxNotificationsUnlink_NoRebonusAfterRelink(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	user := f.CreateUser(TestUserOpts{Email: "max_relink@test.local"})
	mode := f.CreateMode(TestModeOpts{})
	activeTo := time.Now().Add(24 * time.Hour)
	f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: mode.ID, ActiveTo: &activeTo, DailyMessageLimit: 20})

	ts := maxTestServer(t, env)
	h := Handler{MaxBotToken: testMaxToken}
	payload := h.maxUserToken(user.ID)
	webhookPath := "/webhooks/max/" + testMaxWebhookSecret

	// First link: bonus +2 → limit 22.
	maxWebhookJSON(t, ts, webhookPath, map[string]any{
		"update_type": "bot_started", "chat_id": int64(999012), "payload": payload,
	})
	// The cooldown expires, we unlink.
	if _, err := env.Pool.Exec(context.Background(),
		`UPDATE users SET max_linked_at = now() - interval '25 hours' WHERE id = $1`, user.ID); err != nil {
		t.Fatalf("shift max_linked_at: %v", err)
	}
	ts.LoginAs(f.CreateSession(user.ID))
	if _, body := httpJSON(t, ts, http.MethodPost, "/api/notifications/max/unlink", nil); body["ok"] != true {
		t.Fatalf("unlink failed: %v", body)
	}

	// Linking again: the bonus must NOT be granted again.
	maxWebhookJSON(t, ts, webhookPath, map[string]any{
		"update_type": "bot_started", "chat_id": int64(999013), "payload": payload,
	})

	var newLimit int64
	if err := env.Pool.QueryRow(context.Background(),
		`SELECT daily_message_limit FROM user_mode_access WHERE user_id = $1 AND mode_id = $2`, user.ID, mode.ID).Scan(&newLimit); err != nil {
		t.Fatalf("query daily_message_limit: %v", err)
	}
	if newLimit != 22 {
		t.Fatalf("daily_message_limit=%d, want 22 (no re-bonus after relink)", newLimit)
	}
	// And linked again.
	var chatID *int64
	if err := env.Pool.QueryRow(context.Background(),
		`SELECT max_chat_id FROM users WHERE id = $1`, user.ID).Scan(&chatID); err != nil {
		t.Fatalf("query user: %v", err)
	}
	if chatID == nil || *chatID != 999013 {
		t.Fatalf("max_chat_id=%v, want 999013 (relinked)", chatID)
	}
}

func TestAdminNotificationSend_AllAudience(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	user1 := f.CreateUser(TestUserOpts{Email: "notif_all_1@test.local"})
	user2 := f.CreateUser(TestUserOpts{Email: "notif_all_2@test.local"})
	ts := NewTestServer(t, env.Pool)
	ts.LoginAs(f.CreateSession(admin.ID))

	status, body := httpJSON(t, ts, http.MethodPost, "/api/admin/notifications/send", map[string]any{
		"audience": "all",
		"title":    "Тест всем",
		"body":     "Тестовое уведомление для всех",
	})
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%v, want 200", status, body)
	}
	if body["ok"] != true {
		t.Fatalf("ok=%v, want true", body["ok"])
	}
	recipientCount, _ := body["recipientCount"].(float64)
	if recipientCount < 2 {
		t.Fatalf("recipientCount=%v, want >= 2 (created %d, %d)", recipientCount, user1.ID, user2.ID)
	}
	// Check that the inbox is filled for both users
	ctx := context.Background()
	for _, uid := range []int64{user1.ID, user2.ID} {
		var cnt int
		if err := env.Pool.QueryRow(ctx,
			`SELECT count(*) FROM notification_inbox WHERE user_id = $1 AND title = 'Тест всем'`, uid,
		).Scan(&cnt); err != nil {
			t.Fatalf("query inbox user %d: %v", uid, err)
		}
		if cnt != 1 {
			t.Fatalf("user %d inbox count=%d, want 1", uid, cnt)
		}
	}
}

func TestAdminNotificationSend_PromocodeAudience(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	user := f.CreateUser(TestUserOpts{Email: "notif_promo@test.local"})
	promo := f.CreatePromocode(TestPromocodeOpts{Code: "NOTIF_PROMO_TEST"})
	ctx := context.Background()
	if _, err := env.Pool.Exec(ctx,
		`INSERT INTO promocode_usages (user_id, promocode_id, used_at) VALUES ($1, $2, now())`,
		user.ID, promo.ID); err != nil {
		t.Fatalf("insert promocode usage: %v", err)
	}

	ts := NewTestServer(t, env.Pool)
	ts.LoginAs(f.CreateSession(admin.ID))

	status, body := httpJSON(t, ts, http.MethodPost, "/api/admin/notifications/send", map[string]any{
		"audience":     "promocode",
		"promocodeIds": []int64{promo.ID},
		"title":        "Промо уведомление",
		"body":         "Текст промо уведомления",
	})
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%v, want 200", status, body)
	}
	if body["recipientCount"] != float64(1) {
		t.Fatalf("recipientCount=%v, want 1", body["recipientCount"])
	}
	var cnt int
	if err := env.Pool.QueryRow(ctx,
		`SELECT count(*) FROM notification_inbox WHERE user_id = $1 AND title = 'Промо уведомление'`, user.ID,
	).Scan(&cnt); err != nil {
		t.Fatalf("query inbox: %v", err)
	}
	if cnt != 1 {
		t.Fatalf("inbox count=%d, want 1", cnt)
	}
}

func TestAdminNotificationSend_ModeAudience_Has(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	user := f.CreateUser(TestUserOpts{Email: "notif_mode_has@test.local"})
	mode := f.CreateMode(TestModeOpts{})
	f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: mode.ID})
	ts := NewTestServer(t, env.Pool)
	ts.LoginAs(f.CreateSession(admin.ID))

	status, body := httpJSON(t, ts, http.MethodPost, "/api/admin/notifications/send", map[string]any{
		"audience":   "mode",
		"modeIds":    []int64{mode.ID},
		"modeFilter": "has",
		"title":      "Режим уведомление",
		"body":       "Текст режим уведомления",
	})
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%v, want 200", status, body)
	}
	if body["recipientCount"] != float64(1) {
		t.Fatalf("recipientCount=%v, want 1", body["recipientCount"])
	}
	ctx := context.Background()
	var cnt int
	if err := env.Pool.QueryRow(ctx,
		`SELECT count(*) FROM notification_inbox WHERE user_id = $1 AND title = 'Режим уведомление'`, user.ID,
	).Scan(&cnt); err != nil {
		t.Fatalf("query inbox: %v", err)
	}
	if cnt != 1 {
		t.Fatalf("inbox count=%d, want 1", cnt)
	}
}

func TestAdminNotificationSend_ModeAudience_Wrote(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	user := f.CreateUser(TestUserOpts{Email: "notif_mode_wrote@test.local"})
	mode := f.CreateMode(TestModeOpts{})
	f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: mode.ID})
	dialog := f.CreateDialog(user.ID, mode.ID)
	f.AppendMessage(dialog.ID, "user", "Привет!")
	ts := NewTestServer(t, env.Pool)
	ts.LoginAs(f.CreateSession(admin.ID))

	status, body := httpJSON(t, ts, http.MethodPost, "/api/admin/notifications/send", map[string]any{
		"audience":   "mode",
		"modeIds":    []int64{mode.ID},
		"modeFilter": "wrote",
		"title":      "Написал уведомление",
		"body":       "Текст для написавших",
	})
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%v, want 200", status, body)
	}
	if body["recipientCount"] != float64(1) {
		t.Fatalf("recipientCount=%v, want 1", body["recipientCount"])
	}
}
