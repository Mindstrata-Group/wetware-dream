//go:build integration

package httpapi

import (
	"context"
	"net/http"
	"testing"

	"mindstrata-stage1/api/internal/testsupport"
)

func TestNotifications_YandexContactsDoNotGrantConsent(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	user := f.CreateUser(TestUserOpts{Email: "yandex_contact@test.local"})
	h := Handler{DB: env.Pool}

	profile := OAuthProfile{
		Provider:       "yandex",
		ProviderUserID: "yandex-contact-1",
		Email:          user.Email,
		EmailVerified:  true,
		Phone:          "+7 (900) 000-00-18",
		PhoneVerified:  true,
		Raw:            map[string]any{"default_email": user.Email, "default_phone": map[string]any{"number": "+79000000018"}},
	}
	if _, err := h.findOrCreateOAuthUser(context.Background(), profile); err != nil {
		t.Fatalf("findOrCreateOAuthUser: %v", err)
	}

	var contacts, consents int
	if err := env.Pool.QueryRow(context.Background(), `select count(*) from notification_contacts where user_id = $1`, user.ID).Scan(&contacts); err != nil {
		t.Fatalf("contacts count: %v", err)
	}
	if contacts != 2 {
		t.Fatalf("contacts = %d, want 2", contacts)
	}
	if err := env.Pool.QueryRow(context.Background(), `select count(*) from notification_channel_consents where user_id = $1 and channel in ('email', 'sms', 'push')`, user.ID).Scan(&consents); err != nil {
		t.Fatalf("consents count: %v", err)
	}
	if consents != 0 {
		t.Fatalf("Yandex contacts must not grant channel consents, got %d", consents)
	}
}

func TestNotifications_YandexContactStillRequiresExplicitExternalConsent(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	user := f.CreateUser(TestUserOpts{Email: "yandex_external_gate@test.local"})
	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	h := Handler{DB: env.Pool}
	ctx := context.Background()

	profile := OAuthProfile{
		Provider:       "yandex",
		ProviderUserID: "yandex-external-gate",
		Email:          user.Email,
		EmailVerified:  true,
		Phone:          "+7 (900) 000-00-18",
		PhoneVerified:  true,
	}
	if _, err := h.findOrCreateOAuthUser(ctx, profile); err != nil {
		t.Fatalf("findOrCreateOAuthUser: %v", err)
	}

	ts := NewTestServer(t, env.Pool)
	ts.LoginAs(f.CreateSession(admin.ID))
	status, body := httpJSON(t, ts, http.MethodPost, "/api/admin/notifications/test", map[string]any{
		"userId":      user.ID,
		"templateKey": "service.reminder_30m",
		"channel":     "email",
		"variables":   map[string]string{"name": "Анна"},
	})
	if status != http.StatusOK {
		t.Fatalf("admin test status=%d body=%v", status, body)
	}
	if body["attemptStatus"] != "skipped_no_consent" {
		t.Fatalf("attemptStatus=%v, want skipped_no_consent for Yandex email without explicit consent", body["attemptStatus"])
	}

	var attempts int
	if err := env.Pool.QueryRow(ctx, `
		select count(*)
		from notification_delivery_attempts
		where user_id = $1 and channel = 'email' and status = 'skipped_no_consent'`, user.ID).Scan(&attempts); err != nil {
		t.Fatalf("query delivery attempts: %v", err)
	}
	if attempts != 1 {
		t.Fatalf("skipped email attempts=%d, want 1", attempts)
	}
}

func TestNotifications_PreferencesConsentAndInbox(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	user := f.CreateUser(TestUserOpts{Email: "notify_user@test.local"})
	ts := NewTestServer(t, env.Pool)
	ts.LoginAs(f.CreateSession(user.ID))

	status, body := httpJSON(t, ts, http.MethodGet, "/api/notifications/preferences", nil)
	if status != http.StatusOK {
		t.Fatalf("GET preferences status=%d body=%v", status, body)
	}
	reach := body["reachability"].(map[string]any)
	if reach["bestChannel"] != "in_site" {
		t.Fatalf("bestChannel = %v, want in_site", reach["bestChannel"])
	}

	status, body = httpJSON(t, ts, http.MethodPatch, "/api/notifications/preferences", map[string]any{
		"channel":     "email",
		"consentType": "service",
		"granted":     true,
		"reason":      "Напомнить за 30 минут",
	})
	if status != http.StatusOK {
		t.Fatalf("PATCH preferences status=%d body=%v", status, body)
	}
	consents := body["consents"].([]any)
	foundEmailService := false
	for _, raw := range consents {
		item := raw.(map[string]any)
		if item["channel"] == "email" && item["consentType"] == "service" && item["status"] == "granted" {
			foundEmailService = true
		}
	}
	if !foundEmailService {
		t.Fatalf("email service consent not returned: %v", consents)
	}

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))
	status, body = httpJSON(t, ts, http.MethodPost, "/api/admin/notifications/test", map[string]any{
		"userId":      user.ID,
		"templateKey": "service.reminder_30m",
		"channel":     "in_site",
		"variables":   map[string]string{"name": "Анна"},
	})
	if status != http.StatusOK {
		t.Fatalf("admin test status=%d body=%v", status, body)
	}
	if body["attemptStatus"] != "sent" {
		t.Fatalf("attemptStatus=%v, want sent", body["attemptStatus"])
	}

	ts.LoginAs(f.CreateSession(user.ID))
	status, body = httpJSON(t, ts, http.MethodGet, "/api/notifications/inbox", nil)
	if status != http.StatusOK {
		t.Fatalf("GET inbox status=%d body=%v", status, body)
	}
	items := body["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("inbox items=%d, want 1", len(items))
	}
	first := items[0].(map[string]any)
	if first["title"] != "Напомнить за 30 минут" {
		t.Fatalf("inbox title=%v", first["title"])
	}

	status, body = httpJSON(t, ts, http.MethodGet, "/api/notifications/preferences", nil)
	if status != http.StatusOK {
		t.Fatalf("GET preferences after inbox status=%d body=%v", status, body)
	}
	prefInbox := body["inbox"].([]any)
	if len(prefInbox) != 1 {
		t.Fatalf("preferences inbox items=%d, want 1", len(prefInbox))
	}
}

func TestAdminNotificationTemplatesAndConsentGate(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	user := f.CreateUser(TestUserOpts{Email: "marketing_gate@test.local"})
	ts := NewTestServer(t, env.Pool)
	ts.LoginAs(f.CreateSession(admin.ID))

	status, body := httpJSON(t, ts, http.MethodPost, "/api/admin/notifications/templates", map[string]any{
		"key":            "marketing.test_offer",
		"name":           "Тестовый оффер",
		"consentType":    "marketing",
		"defaultChannel": "email",
		"title":          "Оффер для {{name}}",
		"body":           "Тестовый текст",
		"active":         true,
	})
	if status != http.StatusOK {
		t.Fatalf("upsert template status=%d body=%v", status, body)
	}
	tpl := body["template"].(map[string]any)
	if tpl["title"] != "Оффер для {{name}}" {
		t.Fatalf("template title=%v", tpl["title"])
	}

	status, body = httpJSON(t, ts, http.MethodPost, "/api/admin/notifications/test", map[string]any{
		"userId":      user.ID,
		"templateKey": "marketing.test_offer",
		"channel":     "email",
		"variables":   map[string]string{"name": "Иван"},
	})
	if status != http.StatusOK {
		t.Fatalf("test notification status=%d body=%v", status, body)
	}
	if body["attemptStatus"] != "skipped_no_consent" {
		t.Fatalf("attemptStatus=%v, want skipped_no_consent", body["attemptStatus"])
	}
}

func TestAdminNotifications_BulkRecipientsDedupesPromocodesAndExplicitUsers(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	first := f.CreateUser(TestUserOpts{Email: "notify_dedupe_first@test.local"})
	second := f.CreateUser(TestUserOpts{Email: "notify_dedupe_second@test.local"})
	promoA := f.CreatePromocode(TestPromocodeOpts{Code: "NOTIFY_A"})
	promoB := f.CreatePromocode(TestPromocodeOpts{Code: "NOTIFY_B"})
	ctx := context.Background()
	if _, err := env.Pool.Exec(ctx, `
		insert into promocode_usages (user_id, promocode_id, used_at)
		values ($1, $3, now()), ($2, $3, now()), ($2, $4, now())`,
		first.ID, second.ID, promoA.ID, promoB.ID); err != nil {
		t.Fatalf("insert promocode usages: %v", err)
	}

	ts := NewTestServer(t, env.Pool)
	ts.LoginAs(f.CreateSession(admin.ID))
	status, body := httpJSON(t, ts, http.MethodPost, "/api/admin/notifications/test", map[string]any{
		"userIds":      []int64{first.ID},
		"promocodeIds": []int64{promoA.ID, promoB.ID},
		"templateKey":  "service.reminder_30m",
		"channel":      "in_site",
		"variables":    map[string]string{"name": "Тест"},
	})
	if status != http.StatusOK {
		t.Fatalf("bulk notification status=%d body=%v", status, body)
	}
	if body["recipientCount"] != float64(2) {
		t.Fatalf("recipientCount=%v, want 2", body["recipientCount"])
	}
	if body["dedupedRecipientHit"] != float64(2) {
		t.Fatalf("dedupedRecipientHit=%v, want 2", body["dedupedRecipientHit"])
	}

	for _, userID := range []int64{first.ID, second.ID} {
		var inboxCount int
		if err := env.Pool.QueryRow(ctx, `
			select count(*)
			from notification_inbox
			where user_id = $1 and template_key = 'service.reminder_30m'`, userID).Scan(&inboxCount); err != nil {
			t.Fatalf("query inbox count: %v", err)
		}
		if inboxCount != 1 {
			t.Fatalf("user %d inboxCount=%d, want 1", userID, inboxCount)
		}
	}
}

func TestNotifications_RejectsUnreadyPaidChannels(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	user := f.CreateUser(TestUserOpts{Email: "notify_sms_user@test.local"})
	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts := NewTestServer(t, env.Pool)

	ts.LoginAs(f.CreateSession(user.ID))
	status, body := httpJSON(t, ts, http.MethodPatch, "/api/notifications/preferences", map[string]any{
		"channel":     "sms",
		"consentType": "service",
		"granted":     true,
		"reason":      "SMS не включаем в базовом контуре",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("PATCH sms consent status=%d body=%v, want 400", status, body)
	}

	ts.LoginAs(f.CreateSession(admin.ID))
	status, body = httpJSON(t, ts, http.MethodPost, "/api/admin/notifications/templates", map[string]any{
		"key":            "service.sms_blocked",
		"name":           "SMS blocked",
		"consentType":    "service",
		"defaultChannel": "sms",
		"title":          "SMS",
		"body":           "SMS body",
		"active":         true,
	})
	if status != http.StatusBadRequest {
		t.Fatalf("upsert sms template status=%d body=%v, want 400", status, body)
	}

	status, body = httpJSON(t, ts, http.MethodPost, "/api/admin/notifications/test", map[string]any{
		"userId":      user.ID,
		"templateKey": "service.reminder_30m",
		"channel":     "sms",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("admin sms test status=%d body=%v, want 400", status, body)
	}
}
