//go:build integration

package httpapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"mindstrata-stage1/api/internal/testsupport"
)

type capturedYooKassaServer struct {
	*httptest.Server
	mu       sync.Mutex
	requests []map[string]any
	auths    []string
}

var yookassaRecurringIntegrationMu sync.Mutex

func serializeYooKassaRecurringIntegration(t *testing.T) func() {
	t.Helper()
	// These tests check the full lifecycle of recurring payments:
	// webhooks, due renewal scan, invoice/attempt/subscription updates.
	// Running this group in parallel on a snapshot Postgres overloads the DB with
	// transactions and gives false context deadline exceeded errors instead of
	// product defects.
	yookassaRecurringIntegrationMu.Lock()
	return yookassaRecurringIntegrationMu.Unlock
}

func fakeYooKassaCapture(t *testing.T, paymentID string) *capturedYooKassaServer {
	t.Helper()
	return fakeYooKassaCaptureStatusDelay(t, paymentID, http.StatusOK, 0)
}

func fakeYooKassaCaptureStatus(t *testing.T, paymentID string, status int) *capturedYooKassaServer {
	t.Helper()
	return fakeYooKassaCaptureStatusDelay(t, paymentID, status, 0)
}

func fakeYooKassaCaptureStatusDelay(t *testing.T, paymentID string, status int, delay time.Duration) *capturedYooKassaServer {
	t.Helper()
	cap := &capturedYooKassaServer{}
	cap.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Basic ") {
			t.Errorf("missing Basic auth")
		}
		if r.Header.Get("Idempotence-Key") == "" {
			t.Errorf("missing idempotence key")
		}
		auth := r.Header.Get("Authorization")
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode yookassa request: %v", err)
		}
		cap.mu.Lock()
		cap.requests = append(cap.requests, payload)
		cap.auths = append(cap.auths, auth)
		cap.mu.Unlock()
		if delay > 0 {
			time.Sleep(delay)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":     paymentID,
			"status": "pending",
			"amount": map[string]string{"value": "100.00", "currency": "RUB"},
			"confirmation": map[string]any{
				"type":             "redirect",
				"confirmation_url": "https://yookassa.ru/checkout/payments/" + paymentID,
			},
		})
	}))
	t.Cleanup(cap.Close)
	return cap
}

func (c *capturedYooKassaServer) lastAuthorization(t *testing.T) string {
	t.Helper()
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.auths) == 0 {
		t.Fatalf("fake YooKassa received no authorization headers")
	}
	return c.auths[len(c.auths)-1]
}

func (c *capturedYooKassaServer) lastRequest(t *testing.T) map[string]any {
	t.Helper()
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.requests) == 0 {
		t.Fatalf("fake YooKassa received no requests")
	}
	return c.requests[len(c.requests)-1]
}

func (c *capturedYooKassaServer) requestCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.requests)
}

func seedRecurringTariff(t *testing.T, env *testsupport.Env) (userID, modeID, tariffID int64) {
	t.Helper()
	f := NewFactory(t, env.Pool)
	user := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{})
	ctx := context.Background()
	if err := env.Pool.QueryRow(ctx, `
		insert into tariffs (name, monthly_price, daily_message_limit, limit_type, available_for_subscription)
		values ($1, 100.00, 50, 'shared', true)
		returning id`, "recurring-"+mode.Name).Scan(&tariffID); err != nil {
		t.Fatalf("seed tariff: %v", err)
	}
	if _, err := env.Pool.Exec(ctx, `insert into tariff_mode (tariff_id, mode_id) values ($1, $2)`, tariffID, mode.ID); err != nil {
		t.Fatalf("seed tariff_mode: %v", err)
	}
	return user.ID, mode.ID, tariffID
}

type dueRecurringOpts struct {
	paymentMethodID string
	isEnabled       bool
	status          string
	activeTo        time.Time
	nextRetryAt     *time.Time
	failures        int
	maxRetries      int
	retryHours      int
}

type dueRecurringFixture struct {
	userID             int64
	tariffID           int64
	paymentMethodRowID int64
	subscriptionID     int64
	autopayID          int64
}

func seedDueRecurringAutopay(t *testing.T, env *testsupport.Env, opts dueRecurringOpts) dueRecurringFixture {
	t.Helper()
	userID, _, tariffID := seedRecurringTariff(t, env)
	if opts.paymentMethodID == "" {
		opts.paymentMethodID = "pm-due-oracle"
	}
	if opts.status == "" {
		opts.status = "active"
	}
	if opts.activeTo.IsZero() {
		opts.activeTo = time.Now().Add(-time.Minute)
	}
	if opts.maxRetries == 0 {
		opts.maxRetries = 3
	}
	if opts.retryHours == 0 {
		opts.retryHours = 24
	}
	ctx := context.Background()
	var paymentMethodRowID, subscriptionID, autopayID int64
	if err := env.Pool.QueryRow(ctx, `
		insert into payment_methods (user_id, yookassa_payment_method_id, payment_method_type, status)
		values ($1, $2, 'bank_card', 'active')
		returning id`, userID, opts.paymentMethodID).Scan(&paymentMethodRowID); err != nil {
		t.Fatalf("seed payment method: %v", err)
	}
	if err := env.Pool.QueryRow(ctx, `
		insert into subscriptions (
			user_id, tariff_id, status, active_from, active_to, daily_message_limit,
			access_priority, payment_reference, amount_paid, autopay_renewal_months, is_recurring
		)
		values ($1, $2, $3, now() - interval '1 month', $4, 50, 0, 'initial', 100.00, 1, true)
		returning id`, userID, tariffID, opts.status, opts.activeTo).Scan(&subscriptionID); err != nil {
		t.Fatalf("seed subscription: %v", err)
	}
	if err := env.Pool.QueryRow(ctx, `
		insert into autopay_subscriptions (
			subscription_id, payment_method_id, is_enabled, renewal_period_months,
			max_retry_attempts, retry_interval_hours, consecutive_failures, next_retry_at
		)
		values ($1, $2, $3, 1, $4, $5, $6, $7)
		returning id`, subscriptionID, paymentMethodRowID, opts.isEnabled, opts.maxRetries, opts.retryHours, opts.failures, opts.nextRetryAt).Scan(&autopayID); err != nil {
		t.Fatalf("seed autopay: %v", err)
	}
	return dueRecurringFixture{
		userID:             userID,
		tariffID:           tariffID,
		paymentMethodRowID: paymentMethodRowID,
		subscriptionID:     subscriptionID,
		autopayID:          autopayID,
	}
}

func TestYooKassaRecurring_AdminCreatePaymentRequestsSavedMethodForAutoRenew(t *testing.T) {
	unlockYooKassaRecurring := serializeYooKassaRecurringIntegration(t)
	defer unlockYooKassaRecurring()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	userID, _, tariffID := seedRecurringTariff(t, env)
	fake := fakeYooKassaCapture(t, "yk-recurring-initial")
	ts := yookassaTestServer(t, env, &http.Client{Transport: rewriteRT{target: fake.URL}, Timeout: 5 * time.Second})
	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	status, body := httpJSON(t, ts, http.MethodPost, "/api/admin/payments/yookassa/create", map[string]any{
		"amountRub":         100.00,
		"userId":            userID,
		"tariffId":          tariffID,
		"enableAutoRenew":   true,
		"savePaymentMethod": false,
		"returnUrl":         "https://mindstrata.ru/profile",
	})
	if status != http.StatusOK {
		t.Fatalf("create payment status: got %d body=%v", status, body)
	}
	req := fake.lastRequest(t)
	if req["save_payment_method"] != true {
		t.Fatalf("save_payment_method: got %#v want true", req["save_payment_method"])
	}

	var autorenew bool
	if err := env.Pool.QueryRow(context.Background(),
		`select autorenew_requested from invoices where yookassa_payment_id = 'yk-recurring-initial'`).Scan(&autorenew); err != nil {
		t.Fatalf("query invoice autorenew: %v", err)
	}
	if !autorenew {
		t.Fatalf("invoice autorenew_requested=false, want true")
	}
}

func TestYooKassaRecurring_PublicCreateDefaultsToAutoRenew(t *testing.T) {
	unlockYooKassaRecurring := serializeYooKassaRecurringIntegration(t)
	defer unlockYooKassaRecurring()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	userID, _, tariffID := seedRecurringTariff(t, env)
	fake := fakeYooKassaCapture(t, "yk-recurring-public-default")
	ts := yookassaTestServer(t, env, &http.Client{Transport: rewriteRT{target: fake.URL}, Timeout: 5 * time.Second})
	ts.LoginAs(f.CreateSession(userID))

	status, body := httpJSON(t, ts, http.MethodPost, "/api/payments/yookassa/create", map[string]any{
		"tariffId":  tariffID,
		"returnUrl": "https://mindstrata.ru/profile",
	})
	if status != http.StatusOK {
		t.Fatalf("create public payment status: got %d body=%v", status, body)
	}
	req := fake.lastRequest(t)
	if req["save_payment_method"] != true {
		t.Fatalf("save_payment_method: got %#v want true", req["save_payment_method"])
	}

	var isRecurring, autorenew bool
	if err := env.Pool.QueryRow(context.Background(), `
		select is_recurring, autorenew_requested
		from invoices
		where yookassa_payment_id = 'yk-recurring-public-default'`).Scan(&isRecurring, &autorenew); err != nil {
		t.Fatalf("query invoice recurring flags: %v", err)
	}
	if !isRecurring || !autorenew {
		t.Fatalf("invoice recurring flags: is_recurring=%v autorenew_requested=%v, want both true", isRecurring, autorenew)
	}
}

func TestYooKassaRecurring_PublicCreateAppliesStorefrontPeriodDiscount(t *testing.T) {
	unlockYooKassaRecurring := serializeYooKassaRecurringIntegration(t)
	defer unlockYooKassaRecurring()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	userID, _, tariffID := seedRecurringTariff(t, env)
	if _, err := env.Pool.Exec(context.Background(), `update tariffs set monthly_price = 1000 where id = $1`, tariffID); err != nil {
		t.Fatalf("seed tariff price: %v", err)
	}
	if _, err := env.Pool.Exec(context.Background(), `
		insert into site_content (key, value, updated_at)
		values ('shop.periods', '[{"id":"3m","label":"3 месяца","months":3,"discountPercent":10,"enabled":true}]'::jsonb, now())
		on conflict (key) do update set value = excluded.value, updated_at = now()`); err != nil {
		t.Fatalf("seed shop periods: %v", err)
	}
	fake := fakeYooKassaCapture(t, "yk-recurring-public-discount")
	ts := yookassaTestServer(t, env, &http.Client{Transport: rewriteRT{target: fake.URL}, Timeout: 5 * time.Second})
	ts.LoginAs(f.CreateSession(userID))

	status, body := httpJSON(t, ts, http.MethodPost, "/api/payments/yookassa/create", map[string]any{
		"tariffId":           tariffID,
		"subscriptionMonths": 3,
		"returnUrl":          "https://mindstrata.ru/profile",
	})
	if status != http.StatusOK {
		t.Fatalf("create public discounted payment status: got %d body=%v", status, body)
	}
	req := fake.lastRequest(t)
	amount, _ := req["amount"].(map[string]any)
	if amount["value"] != "2700.00" {
		t.Fatalf("amount=%v, want 2700.00 with 10%% discount", amount["value"])
	}

	var invoiceAmount string
	if err := env.Pool.QueryRow(context.Background(),
		`select amount::text from invoices where yookassa_payment_id = 'yk-recurring-public-discount'`).Scan(&invoiceAmount); err != nil {
		t.Fatalf("query discounted invoice: %v", err)
	}
	if invoiceAmount != "2700" && invoiceAmount != "2700.00" {
		t.Fatalf("invoice amount=%s, want 2700", invoiceAmount)
	}
}

func TestYooKassaConfig_AdminCanEnableTestCredentialsWithoutLeakingSecret(t *testing.T) {
	unlockYooKassaRecurring := serializeYooKassaRecurringIntegration(t)
	defer unlockYooKassaRecurring()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)
	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	secret := "test-secret-123456"
	status, body := httpJSON(t, ts, http.MethodPost, "/api/admin/payments/yookassa/config", map[string]any{
		"testModeEnabled": true,
		"testShopId":      "test-shop-42",
		"testSecretKey":   secret,
	})
	if status != http.StatusOK {
		t.Fatalf("POST config status: got %d body=%v", status, body)
	}
	if body["activeMode"] != "test" || body["testConfigured"] != true || body["activeConfigured"] != true {
		t.Fatalf("config response: %#v", body)
	}
	if _, leaked := body["testSecretKey"]; leaked {
		t.Fatalf("config leaked raw testSecretKey: %#v", body)
	}
	if masked, _ := body["testSecretKeyMasked"].(string); masked != "****3456" {
		t.Fatalf("masked secret=%q want ****3456", masked)
	}

	var stored string
	if err := env.Pool.QueryRow(context.Background(), `select value from system_settings where key=$1`, yooKassaTestSecretSetting).Scan(&stored); err != nil {
		t.Fatalf("query stored secret: %v", err)
	}
	if stored != secret {
		t.Fatalf("stored secret=%q want original value", stored)
	}
}

func TestYooKassaConfig_BillingAdminCannotReadOrChangeCredentials(t *testing.T) {
	unlockYooKassaRecurring := serializeYooKassaRecurringIntegration(t)
	defer unlockYooKassaRecurring()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)
	billingAdmin := f.CreateUser(TestUserOpts{Role: "billing_admin"})
	ts.LoginAs(f.CreateSession(billingAdmin.ID))

	status, body := httpJSON(t, ts, http.MethodGet, "/api/admin/payments/yookassa/config", nil)
	if status != http.StatusForbidden && status != http.StatusUnauthorized {
		t.Fatalf("GET config status: got %d body=%v, want 403/401", status, body)
	}
	status, _ = httpJSON(t, ts, http.MethodPost, "/api/admin/payments/yookassa/config", map[string]any{
		"testModeEnabled": true,
		"testShopId":      "should-not-save",
		"testSecretKey":   "should-not-save",
	})
	if status != http.StatusForbidden && status != http.StatusUnauthorized {
		t.Fatalf("POST config as billing_admin status=%d, want 403/401", status)
	}
}

func TestYooKassaConfig_PublicCreatePaymentUsesTestCredentialsWhenEnabled(t *testing.T) {
	unlockYooKassaRecurring := serializeYooKassaRecurringIntegration(t)
	defer unlockYooKassaRecurring()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	userID, _, tariffID := seedRecurringTariff(t, env)
	ctx := context.Background()
	if _, err := env.Pool.Exec(ctx, `
		insert into system_settings (key, value) values
		  ($1, '1'),
		  ($2, 'test-shop-id'),
		  ($3, 'test-secret-key')
		on conflict (key) do update set value=excluded.value`,
		yooKassaTestModeSetting, yooKassaTestShopIDSetting, yooKassaTestSecretSetting); err != nil {
		t.Fatalf("seed yookassa config: %v", err)
	}
	fake := fakeYooKassaCapture(t, "yk-test-mode-public")
	ts := yookassaTestServer(t, env, &http.Client{Transport: rewriteRT{target: fake.URL}, Timeout: 5 * time.Second})
	ts.LoginAs(f.CreateSession(userID))

	status, body := httpJSON(t, ts, http.MethodPost, "/api/payments/yookassa/create", map[string]any{
		"tariffId": tariffID,
	})
	if status != http.StatusOK {
		t.Fatalf("create public payment status: got %d body=%v", status, body)
	}
	wantAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("test-shop-id:test-secret-key"))
	if got := fake.lastAuthorization(t); got != wantAuth {
		t.Fatalf("authorization=%q want %q", got, wantAuth)
	}
}

func TestYooKassaRecurring_PaymentSucceededSavesMethodAndEnablesAutopay(t *testing.T) {
	unlockYooKassaRecurring := serializeYooKassaRecurringIntegration(t)
	defer unlockYooKassaRecurring()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	userID, modeID, tariffID := seedRecurringTariff(t, env)
	ctx := context.Background()
	suffix := f.uniqueSuffix()
	paymentID := "yk-recurring-success-" + suffix
	paymentMethodID := "pm-recurring-" + suffix
	var invoiceID int64
	if err := env.Pool.QueryRow(ctx, `
		insert into invoices (
			user_id, tariff_id, yookassa_payment_id, amount, currency, status,
			subscription_months, expires_at, is_recurring, autorenew_requested
		)
		values ($1, $2, $3, 100.00, 'RUB', 'pending', 1, now() + interval '1 hour', true, true)
		returning id`, userID, tariffID, paymentID).Scan(&invoiceID); err != nil {
		t.Fatalf("seed invoice: %v", err)
	}
	webhookPath := "yk-recurring-enable-" + suffix
	srv := newYooKassaEnterpriseServer(t, env, webhookPath)

	resp, err := postYooKassa(srv.URL, webhookPath,
		`{"event":"payment.succeeded","object":{"id":"`+paymentID+`","status":"succeeded","payment_method":{"id":"`+paymentMethodID+`","type":"bank_card","saved":true}}}`)
	if err != nil {
		t.Fatalf("webhook POST: %v", err)
	}
	assertYooKassaWebhookOK(t, resp, "payment.succeeded")

	var storedPaymentMethodID string
	var subscriptionCount, autopayCount, activeAccessCount int64
	if err := env.Pool.QueryRow(ctx, `
		select i.yookassa_payment_method_id,
		       (select count(*) from subscriptions where user_id = $2 and tariff_id = $3 and is_recurring = true and status = 'active'),
		       (select count(*) from autopay_subscriptions a join subscriptions s on s.id = a.subscription_id where s.user_id = $2 and a.is_enabled = true),
		       (select count(*) from user_mode_access where user_id = $2 and mode_id = $4 and source_id = $1 and active_to > now())
		from invoices i where i.id = $1`, invoiceID, userID, tariffID, modeID).Scan(&storedPaymentMethodID, &subscriptionCount, &autopayCount, &activeAccessCount); err != nil {
		t.Fatalf("query recurring state: %v", err)
	}
	if storedPaymentMethodID != paymentMethodID || subscriptionCount != 1 || autopayCount != 1 || activeAccessCount != 1 {
		t.Fatalf("recurring state: paymentMethod=%q subscriptions=%d autopay=%d access=%d",
			storedPaymentMethodID, subscriptionCount, autopayCount, activeAccessCount)
	}
}

func TestYooKassaRecurring_PaymentSucceededAllowsThreeMonthAutopay(t *testing.T) {
	unlockYooKassaRecurring := serializeYooKassaRecurringIntegration(t)
	defer unlockYooKassaRecurring()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	userID, _, tariffID := seedRecurringTariff(t, env)
	ctx := context.Background()
	suffix := f.uniqueSuffix()
	paymentID := "yk-recurring-success-3m-" + suffix
	paymentMethodID := "pm-recurring-3m-" + suffix
	if _, err := env.Pool.Exec(ctx, `
		insert into invoices (
			user_id, tariff_id, yookassa_payment_id, amount, currency, status,
			subscription_months, expires_at, is_recurring, autorenew_requested
		)
		values ($1, $2, $3, 270.00, 'RUB', 'pending', 3, now() + interval '1 hour', true, true)`,
		userID, tariffID, paymentID); err != nil {
		t.Fatalf("seed invoice: %v", err)
	}
	webhookPath := "yk-recurring-enable-3m-" + suffix
	srv := newYooKassaEnterpriseServer(t, env, webhookPath)

	resp, err := postYooKassa(srv.URL, webhookPath,
		`{"event":"payment.succeeded","object":{"id":"`+paymentID+`","status":"succeeded","payment_method":{"id":"`+paymentMethodID+`","type":"bank_card","saved":true}}}`)
	if err != nil {
		t.Fatalf("webhook POST: %v", err)
	}
	assertYooKassaWebhookOK(t, resp, "payment.succeeded")

	var periodMonths int
	if err := env.Pool.QueryRow(ctx, `
		select a.renewal_period_months
		from autopay_subscriptions a
		join subscriptions s on s.id = a.subscription_id
		where s.user_id = $1 and s.tariff_id = $2 and a.is_enabled = true
		order by a.id desc
		limit 1`, userID, tariffID).Scan(&periodMonths); err != nil {
		t.Fatalf("query autopay period: %v", err)
	}
	if periodMonths != 3 {
		t.Fatalf("renewal period: got %d want 3", periodMonths)
	}
}

func TestYooKassaRecurring_ProcessDueRenewalCreatesPaymentAndPendingAttempt(t *testing.T) {
	unlockYooKassaRecurring := serializeYooKassaRecurringIntegration(t)
	defer unlockYooKassaRecurring()
	env := testsupport.NewEnv(t)
	userID, _, tariffID := seedRecurringTariff(t, env)
	ctx := context.Background()
	var paymentMethodRowID, subscriptionID, autopayID int64
	if err := env.Pool.QueryRow(ctx, `
		insert into payment_methods (user_id, yookassa_payment_method_id, payment_method_type, status)
		values ($1, 'pm-due-renewal', 'bank_card', 'active')
		returning id`, userID).Scan(&paymentMethodRowID); err != nil {
		t.Fatalf("seed payment method: %v", err)
	}
	if err := env.Pool.QueryRow(ctx, `
		insert into subscriptions (
			user_id, tariff_id, status, active_from, active_to, daily_message_limit,
			access_priority, payment_reference, amount_paid, autopay_renewal_months, is_recurring
		)
		values ($1, $2, 'active', now() - interval '1 month', now() - interval '1 minute', 50, 0, 'initial', 100.00, 1, true)
		returning id`, userID, tariffID).Scan(&subscriptionID); err != nil {
		t.Fatalf("seed subscription: %v", err)
	}
	if err := env.Pool.QueryRow(ctx, `
		insert into autopay_subscriptions (
			subscription_id, payment_method_id, is_enabled, renewal_period_months,
			max_retry_attempts, retry_interval_hours, consecutive_failures
		)
		values ($1, $2, true, 1, 3, 24, 0)
		returning id`, subscriptionID, paymentMethodRowID).Scan(&autopayID); err != nil {
		t.Fatalf("seed autopay: %v", err)
	}
	fake := fakeYooKassaCapture(t, "yk-renewal-created")
	h := Handler{
		DB:                env.Pool,
		YooKassaShopID:    "12345",
		YooKassaSecretKey: "secret",
		HTTPClient:        &http.Client{Transport: rewriteRT{target: fake.URL}, Timeout: 5 * time.Second},
	}

	processed, err := h.ProcessDueYooKassaRenewals(ctx, time.Now())
	if err != nil {
		t.Fatalf("process renewals: %v", err)
	}
	if processed != 1 {
		t.Fatalf("processed: got %d want 1", processed)
	}
	req := fake.lastRequest(t)
	if req["payment_method_id"] != "pm-due-renewal" {
		t.Fatalf("payment_method_id: got %#v want pm-due-renewal", req["payment_method_id"])
	}
	var invoiceStatus, attemptStatus string
	if err := env.Pool.QueryRow(ctx, `
		select i.status, r.status
		from invoices i
		join recurring_payment_attempts r on r.yookassa_payment_id = i.yookassa_payment_id
		where i.yookassa_payment_id = 'yk-renewal-created' and r.autopay_subscription_id = $1`, autopayID).Scan(&invoiceStatus, &attemptStatus); err != nil {
		t.Fatalf("query renewal records: %v", err)
	}
	if invoiceStatus != "pending" || attemptStatus != "pending" {
		t.Fatalf("renewal records: invoice=%q attempt=%q, want pending/pending", invoiceStatus, attemptStatus)
	}
}

func TestYooKassaRecurring_ProcessDueRenewalParallelRunsClaimOnce(t *testing.T) {
	unlockYooKassaRecurring := serializeYooKassaRecurringIntegration(t)
	defer unlockYooKassaRecurring()
	env := testsupport.NewEnv(t)
	ctx := context.Background()
	fx := seedDueRecurringAutopay(t, env, dueRecurringOpts{
		paymentMethodID: "pm-due-renewal-lock",
		isEnabled:       true,
		activeTo:        time.Now().Add(-time.Hour),
	})
	fake := fakeYooKassaCaptureStatusDelay(t, "yk-renewal-locked-once", http.StatusOK, 250*time.Millisecond)
	h := Handler{
		DB:                env.Pool,
		YooKassaShopID:    "12345",
		YooKassaSecretKey: "secret",
		HTTPClient:        &http.Client{Transport: rewriteRT{target: fake.URL}, Timeout: 5 * time.Second},
	}
	now := time.Now()
	start := make(chan struct{})
	results := make(chan struct {
		processed int
		err       error
	}, 2)

	for i := 0; i < 2; i++ {
		go func() {
			<-start
			processed, err := h.ProcessDueYooKassaRenewals(ctx, now)
			results <- struct {
				processed int
				err       error
			}{processed: processed, err: err}
		}()
	}
	close(start)

	totalProcessed := 0
	for i := 0; i < 2; i++ {
		result := <-results
		if result.err != nil {
			t.Fatalf("parallel renewal run %d: %v", i+1, result.err)
		}
		totalProcessed += result.processed
	}
	if totalProcessed != 1 {
		t.Fatalf("parallel renewal processed=%d, want exactly 1", totalProcessed)
	}
	if got := fake.requestCount(); got != 1 {
		t.Fatalf("parallel renewal YooKassa requests=%d, want exactly 1", got)
	}

	var invoices, attempts int64
	if err := env.Pool.QueryRow(ctx, `
		select
		  (select count(*) from invoices where yookassa_payment_id = 'yk-renewal-locked-once'),
		  (select count(*) from recurring_payment_attempts where autopay_subscription_id = $1 and status = 'pending')`,
		fx.autopayID).Scan(&invoices, &attempts); err != nil {
		t.Fatalf("query parallel renewal records: %v", err)
	}
	if invoices != 1 || attempts != 1 {
		t.Fatalf("parallel renewal records: invoices=%d attempts=%d, want 1/1", invoices, attempts)
	}
}

func TestYooKassaRecurring_ProcessDueRenewalSkipsUserDisabledAutoRenew(t *testing.T) {
	unlockYooKassaRecurring := serializeYooKassaRecurringIntegration(t)
	defer unlockYooKassaRecurring()
	env := testsupport.NewEnv(t)
	ctx := context.Background()
	seedDueRecurringAutopay(t, env, dueRecurringOpts{
		paymentMethodID: "pm-disabled-before-due",
		isEnabled:       false,
		activeTo:        time.Now().Add(-time.Hour),
	})
	fake := fakeYooKassaCapture(t, "yk-disabled-should-not-exist")
	h := Handler{
		DB:                env.Pool,
		YooKassaShopID:    "12345",
		YooKassaSecretKey: "secret",
		HTTPClient:        &http.Client{Transport: rewriteRT{target: fake.URL}, Timeout: 5 * time.Second},
	}

	processed, err := h.ProcessDueYooKassaRenewals(ctx, time.Now())
	if err != nil {
		t.Fatalf("process renewals: %v", err)
	}
	if processed != 0 {
		t.Fatalf("processed: got %d want 0 for disabled autorenew", processed)
	}
	if got := fake.requestCount(); got != 0 {
		t.Fatalf("disabled autorenew created YooKassa requests: got %d want 0", got)
	}
	var invoices, attempts int64
	if err := env.Pool.QueryRow(ctx, `
		select
		  (select count(*) from invoices where yookassa_payment_id = 'yk-disabled-should-not-exist'),
		  (select count(*) from recurring_payment_attempts where yookassa_payment_id = 'yk-disabled-should-not-exist')`,
	).Scan(&invoices, &attempts); err != nil {
		t.Fatalf("query disabled renewal side effects: %v", err)
	}
	if invoices != 0 || attempts != 0 {
		t.Fatalf("disabled autorenew side effects: invoices=%d attempts=%d, want 0/0", invoices, attempts)
	}
}

func TestYooKassaRecurring_ProcessDueRenewalFailureSchedulesRetryIn24Hours(t *testing.T) {
	unlockYooKassaRecurring := serializeYooKassaRecurringIntegration(t)
	defer unlockYooKassaRecurring()
	env := testsupport.NewEnv(t)
	ctx := context.Background()
	fx := seedDueRecurringAutopay(t, env, dueRecurringOpts{
		paymentMethodID: "pm-retry-next-day",
		isEnabled:       true,
		activeTo:        time.Now().Add(-time.Hour),
		retryHours:      24,
	})
	fake := fakeYooKassaCaptureStatus(t, "yk-failed-create", http.StatusInternalServerError)
	h := Handler{
		DB:                env.Pool,
		YooKassaShopID:    "12345",
		YooKassaSecretKey: "secret",
		HTTPClient:        &http.Client{Transport: rewriteRT{target: fake.URL}, Timeout: 5 * time.Second},
	}
	before := time.Now()

	processed, err := h.ProcessDueYooKassaRenewals(ctx, before)
	if err != nil {
		t.Fatalf("process renewals: %v", err)
	}
	if processed != 0 {
		t.Fatalf("processed after create failure: got %d want 0", processed)
	}
	var failures int
	var nextRetryAt, graceUntil time.Time
	var lastError, subscriptionStatus string
	if err := env.Pool.QueryRow(ctx, `
		select a.consecutive_failures, a.next_retry_at, a.grace_until, coalesce(a.last_error, ''), s.status
		from autopay_subscriptions a
		join subscriptions s on s.id = a.subscription_id
		where a.id = $1`, fx.autopayID).Scan(&failures, &nextRetryAt, &graceUntil, &lastError, &subscriptionStatus); err != nil {
		t.Fatalf("query retry state: %v", err)
	}
	if failures != 1 || subscriptionStatus != "past_due" {
		t.Fatalf("retry state: failures=%d subscription=%q, want 1/past_due", failures, subscriptionStatus)
	}
	if nextRetryAt.Before(before.Add(23*time.Hour)) || nextRetryAt.After(before.Add(25*time.Hour)) {
		t.Fatalf("next retry at %s, want about +24h from %s", nextRetryAt, before)
	}
	if graceUntil.Before(before.Add(yookassaRenewalGrace - time.Hour)) {
		t.Fatalf("grace_until=%s, want renewal grace from %s", graceUntil, before)
	}
	if !strings.Contains(lastError, "yookassa status 500") {
		t.Fatalf("last_error=%q, want YooKassa status 500", lastError)
	}
	var invoices, attempts int64
	if err := env.Pool.QueryRow(ctx, `
		select
		  (select count(*) from invoices where yookassa_payment_id = 'yk-failed-create'),
		  (select count(*) from recurring_payment_attempts where yookassa_payment_id = 'yk-failed-create')`,
	).Scan(&invoices, &attempts); err != nil {
		t.Fatalf("query failed renewal records: %v", err)
	}
	if invoices != 0 || attempts != 0 {
		t.Fatalf("failed create persisted payment records: invoices=%d attempts=%d, want 0/0", invoices, attempts)
	}
}

func TestYooKassaRecurring_ProcessDueRenewalDoesNotDuplicatePendingAttempt(t *testing.T) {
	unlockYooKassaRecurring := serializeYooKassaRecurringIntegration(t)
	defer unlockYooKassaRecurring()
	env := testsupport.NewEnv(t)
	ctx := context.Background()
	fx := seedDueRecurringAutopay(t, env, dueRecurringOpts{
		paymentMethodID: "pm-pending-attempt",
		isEnabled:       true,
		activeTo:        time.Now().Add(-time.Hour),
	})
	if _, err := env.Pool.Exec(ctx, `
		insert into recurring_payment_attempts (
			autopay_subscription_id, yookassa_payment_id, amount, currency, attempt_number,
			status, renewal_start_date, renewal_end_date, original_renewal_start_date
		)
		values ($1, 'yk-pending-existing', 100.00, 'RUB', 1, 'pending', now() - interval '1 hour', now() + interval '1 month', now() - interval '1 hour')`,
		fx.autopayID); err != nil {
		t.Fatalf("seed pending attempt: %v", err)
	}
	fake := fakeYooKassaCapture(t, "yk-duplicate-should-not-exist")
	h := Handler{
		DB:                env.Pool,
		YooKassaShopID:    "12345",
		YooKassaSecretKey: "secret",
		HTTPClient:        &http.Client{Transport: rewriteRT{target: fake.URL}, Timeout: 5 * time.Second},
	}

	processed, err := h.ProcessDueYooKassaRenewals(ctx, time.Now())
	if err != nil {
		t.Fatalf("process renewals: %v", err)
	}
	if processed != 0 || fake.requestCount() != 0 {
		t.Fatalf("pending attempt duplicate: processed=%d requests=%d, want 0/0", processed, fake.requestCount())
	}
}

func TestYooKassaRecurring_ProcessDueRenewalWaitsForNextRetryEvenWhenSubscriptionExpired(t *testing.T) {
	unlockYooKassaRecurring := serializeYooKassaRecurringIntegration(t)
	defer unlockYooKassaRecurring()
	env := testsupport.NewEnv(t)
	ctx := context.Background()
	nextRetryAt := time.Now().Add(23 * time.Hour)
	seedDueRecurringAutopay(t, env, dueRecurringOpts{
		paymentMethodID: "pm-wait-next-retry",
		isEnabled:       true,
		status:          "past_due",
		activeTo:        time.Now().Add(-24 * time.Hour),
		nextRetryAt:     &nextRetryAt,
		failures:        1,
		maxRetries:      3,
	})
	fake := fakeYooKassaCapture(t, "yk-early-retry-should-not-exist")
	h := Handler{
		DB:                env.Pool,
		YooKassaShopID:    "12345",
		YooKassaSecretKey: "secret",
		HTTPClient:        &http.Client{Transport: rewriteRT{target: fake.URL}, Timeout: 5 * time.Second},
	}

	processed, err := h.ProcessDueYooKassaRenewals(ctx, time.Now())
	if err != nil {
		t.Fatalf("process renewals: %v", err)
	}
	if processed != 0 || fake.requestCount() != 0 {
		t.Fatalf("early retry: processed=%d requests=%d, want 0/0", processed, fake.requestCount())
	}
}

func TestYooKassaRecurring_ProcessDueRenewalStopsWhenMaxRetryAttemptsExhausted(t *testing.T) {
	unlockYooKassaRecurring := serializeYooKassaRecurringIntegration(t)
	defer unlockYooKassaRecurring()
	env := testsupport.NewEnv(t)
	ctx := context.Background()
	retryAt := time.Now().Add(-time.Minute)
	seedDueRecurringAutopay(t, env, dueRecurringOpts{
		paymentMethodID: "pm-max-retry-exhausted",
		isEnabled:       true,
		status:          "past_due",
		activeTo:        time.Now().Add(-24 * time.Hour),
		nextRetryAt:     &retryAt,
		failures:        3,
		maxRetries:      3,
	})
	fake := fakeYooKassaCapture(t, "yk-exhausted-should-not-exist")
	h := Handler{
		DB:                env.Pool,
		YooKassaShopID:    "12345",
		YooKassaSecretKey: "secret",
		HTTPClient:        &http.Client{Transport: rewriteRT{target: fake.URL}, Timeout: 5 * time.Second},
	}

	processed, err := h.ProcessDueYooKassaRenewals(ctx, time.Now())
	if err != nil {
		t.Fatalf("process renewals: %v", err)
	}
	if processed != 0 {
		t.Fatalf("processed after max retry exhaustion: got %d want 0", processed)
	}
	if got := fake.requestCount(); got != 0 {
		t.Fatalf("max retry exhausted created YooKassa requests: got %d want 0", got)
	}
	var invoices, attempts int64
	if err := env.Pool.QueryRow(ctx, `
		select
		  (select count(*) from invoices where yookassa_payment_id = 'yk-exhausted-should-not-exist'),
		  (select count(*) from recurring_payment_attempts where yookassa_payment_id = 'yk-exhausted-should-not-exist')`,
	).Scan(&invoices, &attempts); err != nil {
		t.Fatalf("query exhausted renewal records: %v", err)
	}
	if invoices != 0 || attempts != 0 {
		t.Fatalf("max retry exhausted side effects: invoices=%d attempts=%d, want 0/0", invoices, attempts)
	}
}

func TestYooKassaRecurring_RenewalSucceededExtendsSubscriptionAndClearsRetryState(t *testing.T) {
	unlockYooKassaRecurring := serializeYooKassaRecurringIntegration(t)
	defer unlockYooKassaRecurring()
	env := testsupport.NewEnv(t)
	userID, _, tariffID := seedRecurringTariff(t, env)
	ctx := context.Background()
	var paymentMethodRowID, subscriptionID, autopayID int64
	if err := env.Pool.QueryRow(ctx, `
		insert into payment_methods (user_id, yookassa_payment_method_id, payment_method_type, status)
		values ($1, 'pm-renewal-success', 'bank_card', 'active')
		returning id`, userID).Scan(&paymentMethodRowID); err != nil {
		t.Fatalf("seed payment method: %v", err)
	}
	initialActiveTo := time.Now().Add(-time.Hour)
	if err := env.Pool.QueryRow(ctx, `
		insert into subscriptions (
			user_id, tariff_id, status, active_from, active_to, daily_message_limit,
			access_priority, payment_reference, amount_paid, autopay_renewal_months, is_recurring
		)
		values ($1, $2, 'past_due', now() - interval '1 month', $3, 50, 0, 'initial', 100.00, 1, true)
		returning id`, userID, tariffID, initialActiveTo).Scan(&subscriptionID); err != nil {
		t.Fatalf("seed subscription: %v", err)
	}
	if err := env.Pool.QueryRow(ctx, `
		insert into autopay_subscriptions (
			subscription_id, payment_method_id, is_enabled, renewal_period_months,
			max_retry_attempts, retry_interval_hours, consecutive_failures,
			next_retry_at, grace_until, last_error
		)
		values ($1, $2, true, 1, 3, 24, 2, now() - interval '1 hour', now() + interval '2 days', 'previous failure')
		returning id`, subscriptionID, paymentMethodRowID).Scan(&autopayID); err != nil {
		t.Fatalf("seed autopay: %v", err)
	}
	renewalEnd := time.Now().AddDate(0, 1, 0)
	if _, err := env.Pool.Exec(ctx, `
		insert into invoices (
			user_id, tariff_id, yookassa_payment_id, amount, currency, status,
			subscription_months, expires_at, is_recurring, autorenew_requested, yookassa_payment_method_id, renewal_attempt
		)
		values ($1, $2, 'yk-renewal-success', 100.00, 'RUB', 'pending', 1, now() + interval '1 hour', true, true, 'pm-renewal-success', 3)`,
		userID, tariffID); err != nil {
		t.Fatalf("seed invoice: %v", err)
	}
	if _, err := env.Pool.Exec(ctx, `
		insert into recurring_payment_attempts (
			autopay_subscription_id, yookassa_payment_id, amount, currency, attempt_number,
			status, renewal_start_date, renewal_end_date, original_renewal_start_date
		)
		values ($1, 'yk-renewal-success', 100.00, 'RUB', 3, 'pending', $2, $3, $2)`,
		autopayID, initialActiveTo, renewalEnd); err != nil {
		t.Fatalf("seed attempt: %v", err)
	}
	const webhookPath = "yk-recurring-renewal-success"
	srv := newYooKassaEnterpriseServer(t, env, webhookPath)

	resp, err := postYooKassa(srv.URL, webhookPath,
		`{"event":"payment.succeeded","object":{"id":"yk-renewal-success","status":"succeeded","payment_method":{"id":"pm-renewal-success","type":"bank_card","saved":true}}}`)
	if err != nil {
		t.Fatalf("webhook POST: %v", err)
	}
	assertYooKassaWebhookOK(t, resp, "payment.succeeded")

	var invoiceStatus, attemptStatus, subscriptionStatus, paymentReference string
	var activeTo time.Time
	var failures int
	var nextRetryAt, graceUntil *time.Time
	var lastError string
	if err := env.Pool.QueryRow(ctx, `
		select i.status, r.status, s.status, s.active_to, s.payment_reference,
		       a.consecutive_failures, a.next_retry_at, a.grace_until, coalesce(a.last_error, '')
		from invoices i
		join recurring_payment_attempts r on r.yookassa_payment_id = i.yookassa_payment_id
		join autopay_subscriptions a on a.id = r.autopay_subscription_id
		join subscriptions s on s.id = a.subscription_id
		where i.yookassa_payment_id = 'yk-renewal-success'`,
	).Scan(&invoiceStatus, &attemptStatus, &subscriptionStatus, &activeTo, &paymentReference, &failures, &nextRetryAt, &graceUntil, &lastError); err != nil {
		t.Fatalf("query renewal success state: %v", err)
	}
	if invoiceStatus != "paid" || attemptStatus != "succeeded" || subscriptionStatus != "active" {
		t.Fatalf("renewal statuses: invoice=%q attempt=%q subscription=%q, want paid/succeeded/active", invoiceStatus, attemptStatus, subscriptionStatus)
	}
	if activeTo.Before(renewalEnd.Add(-2 * time.Second)) {
		t.Fatalf("subscription active_to=%s, want at least renewal_end=%s", activeTo, renewalEnd)
	}
	if paymentReference != "yk-renewal-success" || failures != 0 || nextRetryAt != nil || graceUntil != nil || lastError != "" {
		t.Fatalf("renewal cleanup: reference=%q failures=%d next=%v grace=%v error=%q",
			paymentReference, failures, nextRetryAt, graceUntil, lastError)
	}
}

func TestYooKassaRecurring_PermissionRevokedDisablesAutopayAndPaymentMethod(t *testing.T) {
	unlockYooKassaRecurring := serializeYooKassaRecurringIntegration(t)
	defer unlockYooKassaRecurring()
	env := testsupport.NewEnv(t)
	userID, _, tariffID := seedRecurringTariff(t, env)
	ctx := context.Background()
	var paymentMethodRowID, subscriptionID, autopayID int64
	if err := env.Pool.QueryRow(ctx, `
		insert into payment_methods (user_id, yookassa_payment_method_id, payment_method_type, status)
		values ($1, 'pm-revoked', 'yoo_money', 'active')
		returning id`, userID).Scan(&paymentMethodRowID); err != nil {
		t.Fatalf("seed payment method: %v", err)
	}
	if err := env.Pool.QueryRow(ctx, `
		insert into subscriptions (
			user_id, tariff_id, status, active_from, active_to, daily_message_limit,
			access_priority, payment_reference, amount_paid, autopay_renewal_months, is_recurring
		)
		values ($1, $2, 'active', now() - interval '1 month', now(), 50, 0, 'initial', 100.00, 1, true)
		returning id`, userID, tariffID).Scan(&subscriptionID); err != nil {
		t.Fatalf("seed subscription: %v", err)
	}
	if err := env.Pool.QueryRow(ctx, `
		insert into autopay_subscriptions (
			subscription_id, payment_method_id, is_enabled, renewal_period_months,
			max_retry_attempts, retry_interval_hours, consecutive_failures
		)
		values ($1, $2, true, 1, 3, 24, 0)
		returning id`, subscriptionID, paymentMethodRowID).Scan(&autopayID); err != nil {
		t.Fatalf("seed autopay: %v", err)
	}
	if _, err := env.Pool.Exec(ctx, `
		insert into invoices (
			user_id, tariff_id, yookassa_payment_id, amount, currency, status,
			subscription_months, expires_at, is_recurring, autorenew_requested, yookassa_payment_method_id
		)
		values ($1, $2, 'yk-permission-revoked', 100.00, 'RUB', 'pending', 1, now() + interval '1 hour', true, true, 'pm-revoked')`,
		userID, tariffID); err != nil {
		t.Fatalf("seed invoice: %v", err)
	}
	if _, err := env.Pool.Exec(ctx, `
		insert into recurring_payment_attempts (
			autopay_subscription_id, yookassa_payment_id, amount, currency, attempt_number,
			status, renewal_start_date, renewal_end_date, original_renewal_start_date
		)
		values ($1, 'yk-permission-revoked', 100.00, 'RUB', 1, 'pending', now(), now() + interval '1 month', now())`, autopayID); err != nil {
		t.Fatalf("seed attempt: %v", err)
	}
	const webhookPath = "yk-recurring-revoked"
	srv := newYooKassaEnterpriseServer(t, env, webhookPath)

	resp, err := postYooKassa(srv.URL, webhookPath,
		`{"event":"payment.canceled","object":{"id":"yk-permission-revoked","status":"canceled","cancellation_details":{"party":"yoo_money","reason":"permission_revoked"}}}`)
	if err != nil {
		t.Fatalf("webhook POST: %v", err)
	}
	assertYooKassaWebhookOK(t, resp, "payment.canceled")

	var invoiceStatus, methodStatus, subscriptionStatus string
	var autopayEnabled bool
	if err := env.Pool.QueryRow(ctx, `
		select i.status, pm.status, s.status, a.is_enabled
		from invoices i
		join recurring_payment_attempts r on r.yookassa_payment_id = i.yookassa_payment_id
		join autopay_subscriptions a on a.id = r.autopay_subscription_id
		join subscriptions s on s.id = a.subscription_id
		join payment_methods pm on pm.id = a.payment_method_id
		where i.yookassa_payment_id = 'yk-permission-revoked'`,
	).Scan(&invoiceStatus, &methodStatus, &subscriptionStatus, &autopayEnabled); err != nil {
		t.Fatalf("query revoked state: %v", err)
	}
	if invoiceStatus != "canceled" || methodStatus != "disabled" || subscriptionStatus != "canceled" || autopayEnabled {
		t.Fatalf("revoked state: invoice=%q method=%q subscription=%q enabled=%v",
			invoiceStatus, methodStatus, subscriptionStatus, autopayEnabled)
	}
}

func TestYooKassaRecurring_UserCanDisableOwnAutoRenew(t *testing.T) {
	unlockYooKassaRecurring := serializeYooKassaRecurringIntegration(t)
	defer unlockYooKassaRecurring()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	userID, _, tariffID := seedRecurringTariff(t, env)
	ctx := context.Background()
	var paymentMethodRowID, subscriptionID int64
	if err := env.Pool.QueryRow(ctx, `
		insert into payment_methods (user_id, yookassa_payment_method_id, payment_method_type, status)
		values ($1, 'pm-user-disable', 'bank_card', 'active')
		returning id`, userID).Scan(&paymentMethodRowID); err != nil {
		t.Fatalf("seed payment method: %v", err)
	}
	if err := env.Pool.QueryRow(ctx, `
		insert into subscriptions (
			user_id, tariff_id, status, active_from, active_to, daily_message_limit,
			access_priority, payment_reference, amount_paid, autopay_renewal_months, is_recurring
		)
		values ($1, $2, 'active', now(), now() + interval '1 month', 50, 0, 'initial', 100.00, 1, true)
		returning id`, userID, tariffID).Scan(&subscriptionID); err != nil {
		t.Fatalf("seed subscription: %v", err)
	}
	if _, err := env.Pool.Exec(ctx, `
		insert into autopay_subscriptions (
			subscription_id, payment_method_id, is_enabled, renewal_period_months,
			max_retry_attempts, retry_interval_hours, consecutive_failures
		)
		values ($1, $2, true, 1, 3, 24, 0)`, subscriptionID, paymentMethodRowID); err != nil {
		t.Fatalf("seed autopay: %v", err)
	}
	ts := NewTestServer(t, env.Pool)
	ts.LoginAs(f.CreateSession(userID))

	status, body := httpJSON(t, ts, http.MethodPost, "/api/billing/autorenew/disable", nil)
	if status != http.StatusOK {
		t.Fatalf("disable status: got %d body=%v", status, body)
	}
	var enabled bool
	var reason string
	if err := env.Pool.QueryRow(ctx,
		`select is_enabled, cancel_reason from autopay_subscriptions where subscription_id = $1`, subscriptionID).Scan(&enabled, &reason); err != nil {
		t.Fatalf("query disabled autopay: %v", err)
	}
	if enabled || reason != "user_disabled" {
		t.Fatalf("autopay disable: enabled=%v reason=%q, want false/user_disabled", enabled, reason)
	}
	var methodStatus, methodToken string
	if err := env.Pool.QueryRow(ctx,
		`select status, yookassa_payment_method_id from payment_methods where id = $1`, paymentMethodRowID).Scan(&methodStatus, &methodToken); err != nil {
		t.Fatalf("query disabled payment method: %v", err)
	}
	if methodStatus != "disabled" || methodToken == "pm-user-disable" || !strings.HasPrefix(methodToken, "disabled:") {
		t.Fatalf("payment method after disable: status=%q token=%q, want disabled/redacted", methodStatus, methodToken)
	}
	// History: incident 2026-07-05: is_recurring stayed true after autopay was
	// turned off (only autopay_subscriptions.is_enabled changed), although both
	// columns must read as one state: "is it renewing".
	var isRecurring bool
	if err := env.Pool.QueryRow(ctx,
		`select is_recurring from subscriptions where id = $1`, subscriptionID).Scan(&isRecurring); err != nil {
		t.Fatalf("query subscription is_recurring: %v", err)
	}
	if isRecurring {
		t.Fatalf("subscriptions.is_recurring = true after disable, want false (must stay in sync with autopay_subscriptions.is_enabled)")
	}
}

func TestYooKassaRecurring_AdminCanRevokeSubscriptionAccess(t *testing.T) {
	unlockYooKassaRecurring := serializeYooKassaRecurringIntegration(t)
	defer unlockYooKassaRecurring()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	userID, modeID, tariffID := seedRecurringTariff(t, env)
	ctx := context.Background()
	var paymentMethodRowID, subscriptionID, invoiceID int64
	if err := env.Pool.QueryRow(ctx, `
		insert into payment_methods (user_id, yookassa_payment_method_id, payment_method_type, status)
		values ($1, 'pm-admin-revoke', 'bank_card', 'active')
		returning id`, userID).Scan(&paymentMethodRowID); err != nil {
		t.Fatalf("seed payment method: %v", err)
	}
	if err := env.Pool.QueryRow(ctx, `
		insert into subscriptions (
			user_id, tariff_id, status, active_from, active_to, daily_message_limit,
			access_priority, payment_reference, amount_paid, autopay_renewal_months, is_recurring
		)
		values ($1, $2, 'active', now(), now() + interval '1 month', 50, 0, 'yk-admin-revoke', 100.00, 1, true)
		returning id`, userID, tariffID).Scan(&subscriptionID); err != nil {
		t.Fatalf("seed subscription: %v", err)
	}
	if _, err := env.Pool.Exec(ctx, `
		insert into autopay_subscriptions (
			subscription_id, payment_method_id, is_enabled, renewal_period_months,
			max_retry_attempts, retry_interval_hours, consecutive_failures
		)
		values ($1, $2, true, 1, 3, 24, 0)`, subscriptionID, paymentMethodRowID); err != nil {
		t.Fatalf("seed autopay: %v", err)
	}
	if err := env.Pool.QueryRow(ctx, `
		insert into invoices (
			user_id, tariff_id, yookassa_payment_id, amount, currency, status,
			subscription_months, expires_at, paid_at, is_recurring, autorenew_requested
		)
		values ($1, $2, 'yk-admin-revoke', 100.00, 'RUB', 'paid', 1, now() + interval '1 month', now(), true, true)
		returning id`, userID, tariffID).Scan(&invoiceID); err != nil {
		t.Fatalf("seed invoice: %v", err)
	}
	if _, err := env.Pool.Exec(ctx, `
		insert into user_mode_access (user_id, mode_id, active_from, active_to, daily_message_limit, priority, access_type, source_id)
		values ($1, $2, now(), now() + interval '1 month', 50, 0, 'subscription', $3)`, userID, modeID, invoiceID); err != nil {
		t.Fatalf("seed access: %v", err)
	}
	owner := f.CreateUser(TestUserOpts{Role: "owner"})
	ts := NewTestServer(t, env.Pool)
	ts.LoginAs(f.CreateSession(owner.ID))

	status, body := httpJSON(t, ts, http.MethodPost, "/api/admin/payments/subscriptions/revoke", map[string]any{
		"subscriptionId": subscriptionID,
		"reason":         "fraud_review",
	})
	if status != http.StatusOK {
		t.Fatalf("revoke status: got %d body=%v", status, body)
	}

	var subscriptionStatus, methodStatus, methodToken string
	var accessActive, autopayEnabled bool
	if err := env.Pool.QueryRow(ctx, `
		select
		  (select status from subscriptions where id = $1),
		  exists(select 1 from user_mode_access where user_id = $2 and access_type = 'subscription' and active_to > now()),
		  coalesce((select is_enabled from autopay_subscriptions where subscription_id = $1), false),
		  (select status from payment_methods where id = $3),
		  (select yookassa_payment_method_id from payment_methods where id = $3)`,
		subscriptionID, userID, paymentMethodRowID).Scan(&subscriptionStatus, &accessActive, &autopayEnabled, &methodStatus, &methodToken); err != nil {
		t.Fatalf("query revoked state: %v", err)
	}
	if subscriptionStatus != "canceled" || accessActive || autopayEnabled || methodStatus != "disabled" || methodToken == "pm-admin-revoke" {
		t.Fatalf("revoked state: subscription=%q accessActive=%v autopay=%v method=%q token=%q",
			subscriptionStatus, accessActive, autopayEnabled, methodStatus, methodToken)
	}
}

func TestYooKassaRecurring_AdminTestAutoChargeUsesSavedMethodAndWritesInvoice(t *testing.T) {
	unlockYooKassaRecurring := serializeYooKassaRecurringIntegration(t)
	defer unlockYooKassaRecurring()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	fake := fakeYooKassaCapture(t, "yk-admin-test-charge")
	ts := yookassaTestServer(t, env, &http.Client{Transport: rewriteRT{target: fake.URL}, Timeout: 5 * time.Second})
	fx := seedDueRecurringAutopay(t, env, dueRecurringOpts{paymentMethodID: "pm-admin-test-charge", isEnabled: true, activeTo: time.Now().Add(24 * time.Hour)})
	owner := f.CreateUser(TestUserOpts{Role: "owner"})
	ts.LoginAs(f.CreateSession(owner.ID))

	status, body := httpJSON(t, ts, http.MethodPost, "/api/admin/payments/yookassa/test-charge", map[string]any{
		"userId":    fx.userID,
		"amountRub": 199.50,
	})
	if status != http.StatusOK {
		t.Fatalf("test charge status: got %d body=%v", status, body)
	}
	req := fake.lastRequest(t)
	if req["payment_method_id"] != "pm-admin-test-charge" {
		t.Fatalf("payment_method_id: got %#v want saved method", req["payment_method_id"])
	}
	amount, _ := req["amount"].(map[string]any)
	if amount["value"] != "199.50" {
		t.Fatalf("amount value: got %#v want 199.50", amount["value"])
	}
	var invoiceCount int
	if err := env.Pool.QueryRow(context.Background(), `
		select count(*) from invoices
		where yookassa_payment_id = 'yk-admin-test-charge'
		  and user_id = $1
		  and cancellation_reason = $2`, fx.userID, "admin_test_charge:"+fmt.Sprint(owner.ID)).Scan(&invoiceCount); err != nil {
		t.Fatalf("query test charge invoice: %v", err)
	}
	if invoiceCount != 1 {
		t.Fatalf("test charge invoices=%d, want 1", invoiceCount)
	}
}

func TestYooKassaRecurring_AdminTestAutoChargeLimitsAmountAndDailyCount(t *testing.T) {
	unlockYooKassaRecurring := serializeYooKassaRecurringIntegration(t)
	defer unlockYooKassaRecurring()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	fake := fakeYooKassaCapture(t, "yk-admin-test-limit")
	ts := yookassaTestServer(t, env, &http.Client{Transport: rewriteRT{target: fake.URL}, Timeout: 5 * time.Second})
	fx := seedDueRecurringAutopay(t, env, dueRecurringOpts{paymentMethodID: "pm-admin-test-limit", isEnabled: true, activeTo: time.Now().Add(24 * time.Hour)})
	owner := f.CreateUser(TestUserOpts{Role: "owner"})
	ts.LoginAs(f.CreateSession(owner.ID))

	status, body := httpJSON(t, ts, http.MethodPost, "/api/admin/payments/yookassa/test-charge", map[string]any{
		"userId":    fx.userID,
		"amountRub": 501,
	})
	if status != http.StatusBadRequest {
		t.Fatalf("amount limit status=%d body=%v, want 400", status, body)
	}

	for i := 0; i < 3; i++ {
		if _, err := env.Pool.Exec(context.Background(), `
			insert into invoices (
				user_id, tariff_id, yookassa_payment_id, amount, currency, status,
				subscription_months, expires_at, is_recurring, autorenew_requested, cancellation_reason
			)
			values ($1, $2, $3, 10.00, 'RUB', 'pending', 1, now() + interval '1 hour', true, true, $4)`,
			fx.userID, fx.tariffID, fmt.Sprintf("yk-admin-test-limit-%d", i), "admin_test_charge:"+fmt.Sprint(owner.ID)); err != nil {
			t.Fatalf("seed daily test charge %d: %v", i, err)
		}
	}
	status, body = httpJSON(t, ts, http.MethodPost, "/api/admin/payments/yookassa/test-charge", map[string]any{
		"userId":    fx.userID,
		"amountRub": 100,
	})
	if status != http.StatusTooManyRequests {
		t.Fatalf("daily limit status=%d body=%v, want 429", status, body)
	}
	if fake.requestCount() != 0 {
		t.Fatalf("fake YooKassa requests=%d, want 0 when validation blocks", fake.requestCount())
	}
}

func TestYooKassaRecurring_RunDueRenewalsRejectsRegularUserAndAllowsOwner(t *testing.T) {
	unlockYooKassaRecurring := serializeYooKassaRecurringIntegration(t)
	defer unlockYooKassaRecurring()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	fake := fakeYooKassaCapture(t, "yk-authz-run")
	ts := yookassaTestServer(t, env, &http.Client{Transport: rewriteRT{target: fake.URL}, Timeout: 5 * time.Second})
	user := f.CreateUser(TestUserOpts{})
	owner := f.CreateUser(TestUserOpts{Role: "owner"})

	ts.LoginAs(f.CreateSession(user.ID))
	status, body := httpJSON(t, ts, http.MethodPost, "/api/admin/payments/yookassa/run-due-renewals", nil)
	if status != http.StatusForbidden {
		t.Fatalf("regular user run renewals status=%d body=%v, want 403", status, body)
	}

	ts.LoginAs(f.CreateSession(owner.ID))
	status, body = httpJSON(t, ts, http.MethodPost, "/api/admin/payments/yookassa/run-due-renewals", nil)
	if status != http.StatusOK {
		t.Fatalf("owner run renewals status=%d body=%v, want 200", status, body)
	}
	if body["processed"] != float64(0) {
		t.Fatalf("owner processed=%v, want 0 without due subscriptions", body["processed"])
	}
}

func TestYooKassaRecurring_AdminDryRunCreatesVisibleBillingRunWithoutExternalCharge(t *testing.T) {
	unlockYooKassaRecurring := serializeYooKassaRecurringIntegration(t)
	defer unlockYooKassaRecurring()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	fake := fakeYooKassaCapture(t, "yk-dry-run-should-not-exist")
	ts := yookassaTestServer(t, env, &http.Client{Transport: rewriteRT{target: fake.URL}, Timeout: 5 * time.Second})
	fx := seedDueRecurringAutopay(t, env, dueRecurringOpts{
		paymentMethodID: "pm-admin-dry-run",
		isEnabled:       true,
		activeTo:        time.Now().Add(-time.Hour),
	})
	if _, err := env.Pool.Exec(context.Background(), `
		insert into invoices (
			user_id, tariff_id, yookassa_payment_id, amount, currency, status,
			subscription_months, expires_at, is_recurring, autorenew_requested
		)
		values ($1, $2, 'yk-admin-dry-run-pending', 100.00, 'RUB', 'pending', 1, now() + interval '1 hour', true, true)`,
		fx.userID, fx.tariffID); err != nil {
		t.Fatalf("seed pending invoice: %v", err)
	}
	owner := f.CreateUser(TestUserOpts{Role: "owner"})
	ts.LoginAs(f.CreateSession(owner.ID))

	status, body := httpJSON(t, ts, http.MethodPost, "/api/admin/payments/yookassa/run-due-renewals", map[string]any{
		"dryRun": true,
		"runKey": "admin-dry-run-report",
	})
	if status != http.StatusOK {
		t.Fatalf("dry-run status=%d body=%v, want 200", status, body)
	}
	if fake.requestCount() != 0 {
		t.Fatalf("dry-run created YooKassa requests=%d, want 0", fake.requestCount())
	}

	status, body = httpJSON(t, ts, http.MethodPost, "/api/admin/payments/yookassa/run-due-renewals", map[string]any{
		"dryRun": true,
		"runKey": "admin-dry-run-report",
	})
	if status != http.StatusOK || body["alreadySucceeded"] != true {
		t.Fatalf("second dry-run status=%d body=%v, want alreadySucceeded", status, body)
	}

	var runs, items int
	if err := env.Pool.QueryRow(context.Background(), `
		select
		  (select count(*) from billing_runs where run_key = 'admin-dry-run-report' and dry_run = true and status = 'succeeded'),
		  (select count(*) from billing_run_items bri join billing_runs br on br.id = bri.billing_run_id where br.run_key = 'admin-dry-run-report')`).Scan(&runs, &items); err != nil {
		t.Fatalf("query billing run: %v", err)
	}
	if runs != 1 || items != 3 {
		t.Fatalf("billing run rows: runs=%d items=%d, want 1/3", runs, items)
	}

	status, body = httpJSON(t, ts, http.MethodGet, "/api/admin/payments", nil)
	if status != http.StatusOK {
		t.Fatalf("admin payments status=%d body=%v, want 200", status, body)
	}
	billingRuns, _ := body["billingRuns"].([]any)
	if len(billingRuns) == 0 {
		t.Fatalf("admin payments did not include billingRuns: %v", body)
	}
}

func TestYooKassaRecurring_AdminRunMarksExhaustedRenewals(t *testing.T) {
	unlockYooKassaRecurring := serializeYooKassaRecurringIntegration(t)
	defer unlockYooKassaRecurring()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	fake := fakeYooKassaCapture(t, "yk-exhausted-admin-should-not-exist")
	ts := yookassaTestServer(t, env, &http.Client{Transport: rewriteRT{target: fake.URL}, Timeout: 5 * time.Second})
	retryAt := time.Now().Add(-time.Hour)
	fx := seedDueRecurringAutopay(t, env, dueRecurringOpts{
		paymentMethodID: "pm-admin-exhausted",
		isEnabled:       true,
		status:          "past_due",
		activeTo:        time.Now().Add(-24 * time.Hour),
		nextRetryAt:     &retryAt,
		failures:        3,
		maxRetries:      3,
	})
	owner := f.CreateUser(TestUserOpts{Role: "owner"})
	ts.LoginAs(f.CreateSession(owner.ID))

	status, body := httpJSON(t, ts, http.MethodPost, "/api/admin/payments/yookassa/run-due-renewals", map[string]any{
		"runKey": "admin-exhausted-run",
	})
	if status != http.StatusOK {
		t.Fatalf("admin run status=%d body=%v, want 200", status, body)
	}
	if fake.requestCount() != 0 {
		t.Fatalf("exhausted-only run created YooKassa requests=%d, want 0", fake.requestCount())
	}

	var enabled bool
	var cancelReason string
	if err := env.Pool.QueryRow(context.Background(), `
		select is_enabled, coalesce(cancel_reason, '')
		from autopay_subscriptions
		where id = $1`, fx.autopayID).Scan(&enabled, &cancelReason); err != nil {
		t.Fatalf("query exhausted autopay: %v", err)
	}
	if enabled || cancelReason != "max_retries_exhausted" {
		t.Fatalf("exhausted autopay enabled=%v reason=%q, want false/max_retries_exhausted", enabled, cancelReason)
	}
	// History: incident 2026-07-05: markExhaustedYooKassaRenewals disabled only
	// autopay_subscriptions.is_enabled, and subscriptions.is_recurring stayed a
	// stale true.
	var isRecurring bool
	if err := env.Pool.QueryRow(context.Background(),
		`select is_recurring from subscriptions where id = $1`, fx.subscriptionID).Scan(&isRecurring); err != nil {
		t.Fatalf("query subscription is_recurring: %v", err)
	}
	if isRecurring {
		t.Fatalf("subscriptions.is_recurring = true after exhausted-retries disable, want false")
	}
}

func TestYooKassaRecurring_ProfileShowsBillingHistoryAndPaymentMethod(t *testing.T) {
	unlockYooKassaRecurring := serializeYooKassaRecurringIntegration(t)
	defer unlockYooKassaRecurring()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	userID, _, tariffID := seedRecurringTariff(t, env)
	ctx := context.Background()
	var methodID, subscriptionID int64
	if err := env.Pool.QueryRow(ctx, `
		insert into payment_methods (user_id, yookassa_payment_method_id, payment_method_type, status, last_used_at)
		values ($1, 'pm-profile', 'bank_card', 'active', now())
		returning id`, userID).Scan(&methodID); err != nil {
		t.Fatalf("seed method: %v", err)
	}
	if err := env.Pool.QueryRow(ctx, `
		insert into subscriptions (
			user_id, tariff_id, status, active_from, active_to, daily_message_limit,
			access_priority, payment_reference, amount_paid, autopay_renewal_months, is_recurring
		)
		values ($1, $2, 'active', now(), now() + interval '1 month', 50, 0, 'yk-profile', 100.00, 1, true)
		returning id`, userID, tariffID).Scan(&subscriptionID); err != nil {
		t.Fatalf("seed subscription: %v", err)
	}
	if _, err := env.Pool.Exec(ctx, `
		insert into autopay_subscriptions (
			subscription_id, payment_method_id, is_enabled, renewal_period_months,
			max_retry_attempts, retry_interval_hours, consecutive_failures
		)
		values ($1, $2, true, 1, 3, 24, 0)`, subscriptionID, methodID); err != nil {
		t.Fatalf("seed autopay: %v", err)
	}
	if _, err := env.Pool.Exec(ctx, `
		insert into invoices (
			user_id, tariff_id, yookassa_payment_id, amount, currency, status,
			subscription_months, expires_at, paid_at, is_recurring, autorenew_requested, yookassa_payment_method_id
		)
		values ($1, $2, 'yk-profile', 100.00, 'RUB', 'paid', 1, now() + interval '1 hour', now(), true, true, 'pm-profile')`,
		userID, tariffID); err != nil {
		t.Fatalf("seed invoice: %v", err)
	}
	ts := NewTestServer(t, env.Pool)
	ts.LoginAs(f.CreateSession(userID))

	status, body := httpJSON(t, ts, http.MethodGet, "/api/profile", nil)
	if status != http.StatusOK {
		t.Fatalf("profile status: got %d body=%v", status, body)
	}
	billing, _ := body["billing"].(map[string]any)
	subscription, _ := billing["subscription"].(map[string]any)
	if subscription["autoRenewEnabled"] != true || subscription["paymentMethodStatus"] != "active" {
		t.Fatalf("profile subscription billing: %#v", subscription)
	}
	payments, _ := billing["payments"].([]any)
	if len(payments) != 1 {
		t.Fatalf("profile payments count: got %d body=%v", len(payments), billing)
	}
}

func TestYooKassaRecurring_AdminPaymentsShowsLifecycle(t *testing.T) {
	unlockYooKassaRecurring := serializeYooKassaRecurringIntegration(t)
	defer unlockYooKassaRecurring()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	userID, _, tariffID := seedRecurringTariff(t, env)
	ctx := context.Background()
	var methodID, subscriptionID int64
	if err := env.Pool.QueryRow(ctx, `
		insert into payment_methods (user_id, yookassa_payment_method_id, payment_method_type, status)
		values ($1, 'pm-admin-list', 'bank_card', 'active')
		returning id`, userID).Scan(&methodID); err != nil {
		t.Fatalf("seed method: %v", err)
	}
	if err := env.Pool.QueryRow(ctx, `
		insert into subscriptions (
			user_id, tariff_id, status, active_from, active_to, daily_message_limit,
			access_priority, payment_reference, amount_paid, autopay_renewal_months, is_recurring
		)
		values ($1, $2, 'active', now(), now() + interval '1 month', 50, 0, 'yk-admin-list', 100.00, 1, true)
		returning id`, userID, tariffID).Scan(&subscriptionID); err != nil {
		t.Fatalf("seed subscription: %v", err)
	}
	if _, err := env.Pool.Exec(ctx, `
		insert into autopay_subscriptions (
			subscription_id, payment_method_id, is_enabled, renewal_period_months,
			max_retry_attempts, retry_interval_hours, consecutive_failures
		)
		values ($1, $2, true, 1, 3, 24, 0)`, subscriptionID, methodID); err != nil {
		t.Fatalf("seed autopay: %v", err)
	}
	if _, err := env.Pool.Exec(ctx, `
		insert into invoices (
			user_id, tariff_id, yookassa_payment_id, amount, currency, status,
			subscription_months, expires_at, is_recurring, autorenew_requested, renewal_attempt
		)
		values ($1, $2, 'yk-admin-list', 100.00, 'RUB', 'pending', 1, now() + interval '1 hour', true, true, 2)`,
		userID, tariffID); err != nil {
		t.Fatalf("seed invoice: %v", err)
	}
	admin := f.CreateUser(TestUserOpts{Role: "billing_admin"})
	ts := NewTestServer(t, env.Pool)
	ts.LoginAs(f.CreateSession(admin.ID))

	status, body := httpJSON(t, ts, http.MethodGet, "/api/admin/payments", nil)
	if status != http.StatusOK {
		t.Fatalf("admin payments status: got %d body=%v", status, body)
	}
	payments, _ := body["payments"].([]any)
	if len(payments) == 0 {
		t.Fatalf("payments empty: %v", body)
	}
	first, _ := payments[0].(map[string]any)
	if first["paymentId"] != "yk-admin-list" || first["isRecurring"] != true || first["autoRenewRequested"] != true {
		t.Fatalf("admin lifecycle row: %#v", first)
	}
	if first["autoRenewEnabled"] != true || first["paymentMethodStatus"] != "active" || first["subscriptionStatus"] != "active" {
		t.Fatalf("admin lifecycle state: %#v", first)
	}
}

func TestYooKassaRecurring_GuestBillingTransfersToRegisteredAccount(t *testing.T) {
	unlockYooKassaRecurring := serializeYooKassaRecurringIntegration(t)
	defer unlockYooKassaRecurring()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	h := Handler{DB: env.Pool}
	guestID, _, tariffID := seedRecurringTariff(t, env)
	target := f.CreateUser(TestUserOpts{})
	ctx := context.Background()
	var methodID, subscriptionID int64
	if err := env.Pool.QueryRow(ctx, `
		insert into payment_methods (user_id, yookassa_payment_method_id, payment_method_type, status)
		values ($1, 'pm-guest-transfer', 'bank_card', 'active')
		returning id`, guestID).Scan(&methodID); err != nil {
		t.Fatalf("seed method: %v", err)
	}
	if err := env.Pool.QueryRow(ctx, `
		insert into subscriptions (
			user_id, tariff_id, status, active_from, active_to, daily_message_limit,
			access_priority, payment_reference, amount_paid, autopay_renewal_months, is_recurring
		)
		values ($1, $2, 'active', now(), now() + interval '1 month', 50, 0, 'yk-guest-transfer', 100.00, 1, true)
		returning id`, guestID, tariffID).Scan(&subscriptionID); err != nil {
		t.Fatalf("seed subscription: %v", err)
	}
	if _, err := env.Pool.Exec(ctx, `
		insert into autopay_subscriptions (
			subscription_id, payment_method_id, is_enabled, renewal_period_months,
			max_retry_attempts, retry_interval_hours, consecutive_failures
		)
		values ($1, $2, true, 1, 3, 24, 0)`, subscriptionID, methodID); err != nil {
		t.Fatalf("seed autopay: %v", err)
	}
	if _, err := env.Pool.Exec(ctx, `
		insert into invoices (
			user_id, tariff_id, yookassa_payment_id, amount, currency, status,
			subscription_months, expires_at, is_recurring, autorenew_requested
		)
		values ($1, $2, 'yk-guest-transfer', 100.00, 'RUB', 'paid', 1, now() + interval '1 hour', true, true)`,
		guestID, tariffID); err != nil {
		t.Fatalf("seed invoice: %v", err)
	}

	if err := h.transferGuestAccess(ctx, guestID, target.ID); err != nil {
		t.Fatalf("transfer billing: %v", err)
	}
	var invoiceUser, methodUser, subscriptionUser int64
	if err := env.Pool.QueryRow(ctx, `
		select
		  (select user_id from invoices where yookassa_payment_id = 'yk-guest-transfer'),
		  (select user_id from payment_methods where yookassa_payment_method_id = 'pm-guest-transfer'),
		  (select user_id from subscriptions where payment_reference = 'yk-guest-transfer')`,
	).Scan(&invoiceUser, &methodUser, &subscriptionUser); err != nil {
		t.Fatalf("query transferred billing: %v", err)
	}
	if invoiceUser != target.ID || methodUser != target.ID || subscriptionUser != target.ID {
		t.Fatalf("billing ownership: invoice=%d method=%d subscription=%d target=%d", invoiceUser, methodUser, subscriptionUser, target.ID)
	}
}
