//go:build integration

package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"mindstrata-stage1/api/internal/testsupport"
)

type yookassaEnterpriseFixture struct {
	UserID    int64
	ModeID    int64
	TariffID  int64
	InvoiceID int64
	PaymentID string
}

func seedYooKassaEnterpriseInvoice(t *testing.T, env *testsupport.Env, status string) yookassaEnterpriseFixture {
	t.Helper()
	return seedYooKassaEnterpriseInvoiceMonths(t, env, status, 1)
}

func seedYooKassaEnterpriseInvoiceMonths(t *testing.T, env *testsupport.Env, status string, months int) yookassaEnterpriseFixture {
	t.Helper()
	f := NewFactory(t, env.Pool)
	ctx := context.Background()
	user := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{})
	paymentID := "yk-enterprise-" + mode.Name

	var tariffID int64
	if err := env.Pool.QueryRow(ctx, `
		insert into tariffs (name, monthly_price, daily_message_limit, limit_type, available_for_subscription)
		values ($1, 100.00, 50, 'shared', true)
		returning id`, "tariff-"+mode.Name).Scan(&tariffID); err != nil {
		t.Fatalf("seed tariff: %v", err)
	}
	if _, err := env.Pool.Exec(ctx,
		`insert into tariff_mode (tariff_id, mode_id) values ($1, $2)`,
		tariffID, mode.ID); err != nil {
		t.Fatalf("seed tariff_mode: %v", err)
	}
	if months <= 0 {
		months = 1
	}
	var invoiceID int64
	if err := env.Pool.QueryRow(ctx, `
		insert into invoices (user_id, tariff_id, yookassa_payment_id, amount, currency, status, subscription_months, expires_at)
		values ($1, $2, $3, 100.00, 'RUB', $4, $5, now() + interval '1 hour')
		returning id`,
		user.ID, tariffID, paymentID, status, months).Scan(&invoiceID); err != nil {
		t.Fatalf("seed invoice: %v", err)
	}

	return yookassaEnterpriseFixture{
		UserID:    user.ID,
		ModeID:    mode.ID,
		TariffID:  tariffID,
		InvoiceID: invoiceID,
		PaymentID: paymentID,
	}
}

func newYooKassaEnterpriseServer(t *testing.T, env *testsupport.Env, webhookPath string) *httptest.Server {
	t.Helper()
	handler := Handler{
		DB:                  env.Pool,
		YooKassaWebhookPath: webhookPath,
		HTTPClient:          &http.Client{Timeout: 5 * time.Second},
	}
	srv := httptest.NewServer(NewRouter(handler, []string{"*"}))
	t.Cleanup(srv.Close)
	return srv
}

func newYooKassaEnterpriseServerWithClient(t *testing.T, env *testsupport.Env, webhookPath string, client *http.Client) *httptest.Server {
	t.Helper()
	handler := Handler{
		DB:                  env.Pool,
		YooKassaShopID:      "12345",
		YooKassaSecretKey:   "test_secret",
		YooKassaWebhookPath: webhookPath,
		HTTPClient:          client,
	}
	if handler.HTTPClient == nil {
		handler.HTTPClient = &http.Client{Timeout: 5 * time.Second}
	}
	srv := httptest.NewServer(NewRouter(handler, []string{"*"}))
	t.Cleanup(srv.Close)
	return srv
}

func fakeYooKassaPaymentLookup(t *testing.T, objects map[string]map[string]any, status int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("unexpected YooKassa method: got %s want GET", r.Method)
		}
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Basic ") {
			t.Errorf("missing Basic auth header")
		}
		id := strings.TrimPrefix(r.URL.Path, "/v3/payments/")
		body, ok := objects[id]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]any{"id": id, "status": "not_found"})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func yookassaHandlerForReconciliation(env *testsupport.Env, client *http.Client) Handler {
	return Handler{
		DB:                env.Pool,
		YooKassaShopID:    "12345",
		YooKassaSecretKey: "test_secret",
		HTTPClient:        client,
	}
}

func postYooKassaFromIP(t *testing.T, srvURL, path, body, ip string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, srvURL+"/webhooks/yookassa/"+path, strings.NewReader(body))
	if err != nil {
		t.Fatalf("build webhook request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Forwarded-For", ip)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("webhook POST: %v", err)
	}
	return resp
}

func assertYooKassaInvoiceAndAccess(t *testing.T, env *testsupport.Env, fx yookassaEnterpriseFixture, wantStatus string, wantAccessCount int64) {
	t.Helper()
	ctx := context.Background()
	var invoiceStatus string
	if err := env.Pool.QueryRow(ctx,
		`select status from invoices where yookassa_payment_id = $1`,
		fx.PaymentID).Scan(&invoiceStatus); err != nil {
		t.Fatalf("query invoice status: %v", err)
	}
	if invoiceStatus != wantStatus {
		t.Fatalf("invoice status: got %q want %q", invoiceStatus, wantStatus)
	}

	var accessCount int64
	if err := env.Pool.QueryRow(ctx, `
		select count(*) from user_mode_access
		where user_id = $1 and mode_id = $2 and access_type = 'subscription'`,
		fx.UserID, fx.ModeID).Scan(&accessCount); err != nil {
		t.Fatalf("query access count: %v", err)
	}
	if accessCount != wantAccessCount {
		t.Fatalf("access grants: got %d want %d", accessCount, wantAccessCount)
	}
}

func assertYooKassaWebhookOK(t *testing.T, resp *http.Response, context string) {
	t.Helper()
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("%s status: got %d want 200", context, resp.StatusCode)
	}
}

func TestYooKassaEnterprise_InvalidRemoteIPDoesNotMutateInvoiceOrAccess(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	fx := seedYooKassaEnterpriseInvoice(t, env, "pending")
	const webhookPath = "yk-enterprise-bad-ip"
	srv := newYooKassaEnterpriseServer(t, env, webhookPath)

	resp := postYooKassaFromIP(t, srv.URL, webhookPath,
		`{"event":"payment.succeeded","object":{"id":"`+fx.PaymentID+`","status":"succeeded"}}`,
		"8.8.8.8")
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("foreign IP status: got %d want 403", resp.StatusCode)
	}

	assertYooKassaInvoiceAndAccess(t, env, fx, "pending", 0)
	var eventCount int64
	if err := env.Pool.QueryRow(context.Background(),
		`select count(*) from yookassa_webhook_events where object_id = $1`,
		fx.PaymentID).Scan(&eventCount); err != nil {
		t.Fatalf("query webhook events: %v", err)
	}
	if eventCount != 0 {
		t.Fatalf("foreign IP persisted webhook events: got %d want 0", eventCount)
	}
}

func TestYooKassaEnterprise_DuplicateProcessedWebhookDoesNotDoubleGrantAccess(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	fx := seedYooKassaEnterpriseInvoice(t, env, "pending")
	const webhookPath = "yk-enterprise-duplicate"
	srv := newYooKassaEnterpriseServer(t, env, webhookPath)

	payload := `{"event":"payment.succeeded","object":{"id":"` + fx.PaymentID + `","status":"succeeded"}}`
	for i := 1; i <= 2; i++ {
		resp, err := postYooKassa(srv.URL, webhookPath, payload)
		if err != nil {
			t.Fatalf("webhook POST #%d: %v", i, err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("webhook POST #%d status: got %d want 200", i, resp.StatusCode)
		}
	}

	assertYooKassaInvoiceAndAccess(t, env, fx, "paid", 1)
	var eventCount int64
	var processStatus string
	if err := env.Pool.QueryRow(context.Background(), `
		select count(*), max(process_status)
		from yookassa_webhook_events
		where event = 'payment.succeeded' and object_id = $1`, fx.PaymentID).Scan(&eventCount, &processStatus); err != nil {
		t.Fatalf("query webhook event: %v", err)
	}
	if eventCount != 1 || processStatus != "accepted" {
		t.Fatalf("duplicate webhook event: count=%d process_status=%q, want 1/accepted", eventCount, processStatus)
	}
}

func TestYooKassaEnterprise_PaymentSucceededStatusMismatchDoesNotGrantAccess(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	fx := seedYooKassaEnterpriseInvoice(t, env, "pending")
	const webhookPath = "yk-enterprise-bad-status"
	srv := newYooKassaEnterpriseServer(t, env, webhookPath)

	resp, err := postYooKassa(srv.URL, webhookPath,
		`{"event":"payment.succeeded","object":{"id":"`+fx.PaymentID+`","status":"canceled"}}`)
	if err != nil {
		t.Fatalf("webhook POST: %v", err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status mismatch response: got %d want 200", resp.StatusCode)
	}

	assertYooKassaInvoiceAndAccess(t, env, fx, "pending", 0)
	var processStatus string
	if err := env.Pool.QueryRow(context.Background(), `
		select process_status from yookassa_webhook_events
		where event = 'payment.succeeded' and object_id = $1`, fx.PaymentID).Scan(&processStatus); err != nil {
		t.Fatalf("query webhook process_status: %v", err)
	}
	if processStatus != "suspicious" {
		t.Fatalf("status mismatch process_status: got %q want suspicious", processStatus)
	}
}

func TestYooKassaEnterprise_RemoteVerificationMismatchDoesNotGrantAccess(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	fx := seedYooKassaEnterpriseInvoice(t, env, "pending")
	fake := fakeYooKassaPaymentLookup(t, map[string]map[string]any{
		fx.PaymentID: {"id": fx.PaymentID, "status": "canceled"},
	}, http.StatusOK)
	const webhookPath = "yk-enterprise-remote-mismatch"
	srv := newYooKassaEnterpriseServerWithClient(t, env, webhookPath, &http.Client{
		Transport: rewriteRT{target: fake.URL},
		Timeout:   5 * time.Second,
	})

	resp, err := postYooKassa(srv.URL, webhookPath,
		`{"event":"payment.succeeded","object":{"id":"`+fx.PaymentID+`","status":"succeeded"}}`)
	if err != nil {
		t.Fatalf("webhook POST: %v", err)
	}
	assertYooKassaWebhookOK(t, resp, "remote status mismatch")
	assertYooKassaInvoiceAndAccess(t, env, fx, "pending", 0)

	var processStatus string
	if err := env.Pool.QueryRow(context.Background(), `
		select process_status from yookassa_webhook_events
		where event = 'payment.succeeded' and object_id = $1`, fx.PaymentID).Scan(&processStatus); err != nil {
		t.Fatalf("query webhook process_status: %v", err)
	}
	if processStatus != "suspicious" {
		t.Fatalf("remote mismatch process_status: got %q want suspicious", processStatus)
	}
}

func TestYooKassaEnterprise_RemoteVerificationFailureDoesNotGrantAccess(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	fx := seedYooKassaEnterpriseInvoice(t, env, "pending")
	fake := fakeYooKassaPaymentLookup(t, map[string]map[string]any{
		fx.PaymentID: {"id": fx.PaymentID, "status": "succeeded"},
	}, http.StatusInternalServerError)
	const webhookPath = "yk-enterprise-remote-failed"
	srv := newYooKassaEnterpriseServerWithClient(t, env, webhookPath, &http.Client{
		Transport: rewriteRT{target: fake.URL},
		Timeout:   5 * time.Second,
	})

	resp, err := postYooKassa(srv.URL, webhookPath,
		`{"event":"payment.succeeded","object":{"id":"`+fx.PaymentID+`","status":"succeeded"}}`)
	if err != nil {
		t.Fatalf("webhook POST: %v", err)
	}
	assertYooKassaWebhookOutcome(t, resp, http.StatusInternalServerError, "retryable")
	assertYooKassaInvoiceAndAccess(t, env, fx, "pending", 0)

	var processStatus string
	if err := env.Pool.QueryRow(context.Background(), `
		select process_status from yookassa_webhook_events
		where event = 'payment.succeeded' and object_id = $1`, fx.PaymentID).Scan(&processStatus); err != nil {
		t.Fatalf("query webhook process_status: %v", err)
	}
	if processStatus != "retryable" {
		t.Fatalf("remote failure process_status: got %q want retryable", processStatus)
	}
}

func TestYooKassaEnterprise_RemoteVerificationSucceededGrantsAccess(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	fx := seedYooKassaEnterpriseInvoice(t, env, "pending")
	fake := fakeYooKassaPaymentLookup(t, map[string]map[string]any{
		fx.PaymentID: {"id": fx.PaymentID, "status": "succeeded"},
	}, http.StatusOK)
	const webhookPath = "yk-enterprise-remote-success"
	srv := newYooKassaEnterpriseServerWithClient(t, env, webhookPath, &http.Client{
		Transport: rewriteRT{target: fake.URL},
		Timeout:   5 * time.Second,
	})

	resp, err := postYooKassa(srv.URL, webhookPath,
		`{"event":"payment.succeeded","object":{"id":"`+fx.PaymentID+`","status":"succeeded"}}`)
	if err != nil {
		t.Fatalf("webhook POST: %v", err)
	}
	assertYooKassaWebhookOK(t, resp, "remote verify succeeded")
	assertYooKassaInvoiceAndAccess(t, env, fx, "paid", 1)
}

func TestYooKassaEnterprise_ReconcilePendingInvoicesFromRemoteState(t *testing.T) {
	t.Run("remote_succeeded_grants_access", func(t *testing.T) {
		t.Parallel()
		env := testsupport.NewEnv(t)
		fx := seedYooKassaEnterpriseInvoice(t, env, "pending")
		fake := fakeYooKassaPaymentLookup(t, map[string]map[string]any{
			fx.PaymentID: {"id": fx.PaymentID, "status": "succeeded"},
		}, http.StatusOK)
		handler := yookassaHandlerForReconciliation(env, &http.Client{Transport: rewriteRT{target: fake.URL}, Timeout: 5 * time.Second})

		result, err := handler.ReconcilePendingYooKassaInvoices(context.Background(), 10)
		if err != nil {
			t.Fatalf("reconcile pending invoices: %v", err)
		}
		if result["checked"] != 1 || result["paid"] != 1 {
			t.Fatalf("reconcile result: got %+v want checked=1 paid=1", result)
		}
		assertYooKassaInvoiceAndAccess(t, env, fx, "paid", 1)
	})

	// World: YooKassa does not keep a payment forever: for very old/non-existent
	// payment_id, GET /v3/payments/{id} returns 404. This used to be treated as a
	// regular transient error (result["failed"]++, the invoice stays pending), so
	// the invoice landed in the same batch again on the next iteration of the loop
	// in DailyYooKassaRenewalsWorkflow (workflow.go: `for { ... if
	// batch.Counts["checked"] == 0 { break } }`), which never saw checked=0 and
	// burned the whole ScheduleToCloseTimeout without reaching the real
	// subscription renewals (real incident 2026-07-04/05: 13 such invoices on prod
	// put both nightly runs, prod and staging, into a loop).
	t.Run("remote_not_found_marks_canceled_so_reconcile_loop_terminates", func(t *testing.T) {
		t.Parallel()
		env := testsupport.NewEnv(t)
		fx := seedYooKassaEnterpriseInvoice(t, env, "pending")
		// We deliberately do NOT register fx.PaymentID in objects: fakeYooKassaPaymentLookup
		// then answers 404, as the real YooKassa does for stale payment_id.
		fake := fakeYooKassaPaymentLookup(t, map[string]map[string]any{}, http.StatusOK)
		handler := yookassaHandlerForReconciliation(env, &http.Client{Transport: rewriteRT{target: fake.URL}, Timeout: 5 * time.Second})

		result, err := handler.ReconcilePendingYooKassaInvoices(context.Background(), 10)
		if err != nil {
			t.Fatalf("reconcile pending invoices: %v", err)
		}
		if result["checked"] != 1 || result["not_found"] != 1 || result["failed"] != 0 {
			t.Fatalf("reconcile result: got %+v want checked=1 not_found=1 failed=0", result)
		}
		assertYooKassaInvoiceAndAccess(t, env, fx, "canceled", 0)

		// A repeated pass with the same limit must now return checked=0: exactly the
		// condition that stops the loop in DailyYooKassaRenewalsWorkflow.
		second, err := handler.ReconcilePendingYooKassaInvoices(context.Background(), 10)
		if err != nil {
			t.Fatalf("reconcile pending invoices (second pass): %v", err)
		}
		if second["checked"] != 0 {
			t.Fatalf("second pass: got checked=%d, want 0 (invoice must no longer be pending)", second["checked"])
		}
	})

	t.Run("remote_canceled_updates_invoice_without_access", func(t *testing.T) {
		t.Parallel()
		env := testsupport.NewEnv(t)
		fx := seedYooKassaEnterpriseInvoice(t, env, "pending")
		fake := fakeYooKassaPaymentLookup(t, map[string]map[string]any{
			fx.PaymentID: {
				"id":     fx.PaymentID,
				"status": "canceled",
				"cancellation_details": map[string]string{
					"party":  "payment_network",
					"reason": "expired_on_confirmation",
				},
			},
		}, http.StatusOK)
		handler := yookassaHandlerForReconciliation(env, &http.Client{Transport: rewriteRT{target: fake.URL}, Timeout: 5 * time.Second})

		result, err := handler.ReconcilePendingYooKassaInvoices(context.Background(), 10)
		if err != nil {
			t.Fatalf("reconcile pending invoices: %v", err)
		}
		if result["checked"] != 1 || result["canceled"] != 1 {
			t.Fatalf("reconcile result: got %+v want checked=1 canceled=1", result)
		}
		assertYooKassaInvoiceAndAccess(t, env, fx, "canceled", 0)
	})
}

func TestYooKassaEnterprise_InvoiceStateMachine(t *testing.T) {
	t.Run("pending_to_paid_grants_access", func(t *testing.T) {
		t.Parallel()
		env := testsupport.NewEnv(t)
		fx := seedYooKassaEnterpriseInvoice(t, env, "pending")
		const webhookPath = "yk-enterprise-paid"
		srv := newYooKassaEnterpriseServer(t, env, webhookPath)

		resp, err := postYooKassa(srv.URL, webhookPath,
			`{"event":"payment.succeeded","object":{"id":"`+fx.PaymentID+`","status":"succeeded"}}`)
		if err != nil {
			t.Fatalf("webhook POST: %v", err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("pending->paid status: got %d want 200", resp.StatusCode)
		}

		assertYooKassaInvoiceAndAccess(t, env, fx, "paid", 1)
	})

	t.Run("pending_to_canceled_does_not_grant_access", func(t *testing.T) {
		t.Parallel()
		env := testsupport.NewEnv(t)
		fx := seedYooKassaEnterpriseInvoice(t, env, "pending")
		const webhookPath = "yk-enterprise-canceled"
		srv := newYooKassaEnterpriseServer(t, env, webhookPath)

		resp, err := postYooKassa(srv.URL, webhookPath,
			`{"event":"payment.canceled","object":{"id":"`+fx.PaymentID+`","status":"canceled"}}`)
		if err != nil {
			t.Fatalf("webhook POST: %v", err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("pending->canceled status: got %d want 200", resp.StatusCode)
		}

		assertYooKassaInvoiceAndAccess(t, env, fx, "canceled", 0)
	})

	t.Run("expired_invoice_is_terminal_for_success_webhook", func(t *testing.T) {
		t.Parallel()
		env := testsupport.NewEnv(t)
		fx := seedYooKassaEnterpriseInvoice(t, env, "expired")
		const webhookPath = "yk-enterprise-expired"
		srv := newYooKassaEnterpriseServer(t, env, webhookPath)

		resp, err := postYooKassa(srv.URL, webhookPath,
			`{"event":"payment.succeeded","object":{"id":"`+fx.PaymentID+`","status":"succeeded"}}`)
		if err != nil {
			t.Fatalf("webhook POST: %v", err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusInternalServerError {
			t.Fatalf("expired success webhook status: got %d want 500", resp.StatusCode)
		}

		assertYooKassaInvoiceAndAccess(t, env, fx, "expired", 0)
	})
}

func TestYooKassaEnterprise_AdminCreatePaymentCreatesInvoiceWithBillingFields(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ctx := context.Background()

	target := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{})
	var tariffID int64
	if err := env.Pool.QueryRow(ctx, `
		insert into tariffs (name, monthly_price, daily_message_limit, limit_type, available_for_subscription)
		values ($1, 321.45, 50, 'shared', true)
		returning id`, "yk-enterprise-admin-"+mode.Name).Scan(&tariffID); err != nil {
		t.Fatalf("seed tariff: %v", err)
	}
	if _, err := env.Pool.Exec(ctx,
		`insert into tariff_mode (tariff_id, mode_id) values ($1, $2)`,
		tariffID, mode.ID); err != nil {
		t.Fatalf("seed tariff_mode: %v", err)
	}

	fake := fakeYooKassa(t, http.StatusOK, map[string]any{
		"id":     "yk-enterprise-admin-create-001",
		"status": "pending",
		"confirmation": map[string]any{
			"type":             "redirect",
			"confirmation_url": "https://yookassa.ru/checkout/payments/yk-enterprise-admin-create-001",
		},
	})
	ts := yookassaTestServer(t, env, &http.Client{
		Transport: rewriteRT{target: fake.URL},
		Timeout:   5 * time.Second,
	})
	admin := f.CreateUser(TestUserOpts{Role: "billing_admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	status, body := httpJSON(t, ts, http.MethodPost, "/api/admin/payments/yookassa/create", map[string]any{
		"amountRub":          321.45,
		"userId":             target.ID,
		"tariffId":           tariffID,
		"subscriptionMonths": 3,
		"description":        "Проверка lifecycle invoice",
		"savePaymentMethod":  true,
		"returnUrl":          "https://mindstrata.ru/profile",
	})
	if status != http.StatusOK {
		t.Fatalf("create payment status: got %d body=%v", status, body)
	}

	var userID, gotTariffID int64
	var amount string
	var invoiceStatus string
	var months int
	var expiresInFuture bool
	if err := env.Pool.QueryRow(ctx, `
		select user_id, tariff_id, amount::text, status, subscription_months, expires_at > now()
		from invoices
		where yookassa_payment_id = 'yk-enterprise-admin-create-001'`,
	).Scan(&userID, &gotTariffID, &amount, &invoiceStatus, &months, &expiresInFuture); err != nil {
		t.Fatalf("query created invoice: %v", err)
	}
	if userID != target.ID || gotTariffID != tariffID || amount != "321.45" || invoiceStatus != "pending" || months != 3 || !expiresInFuture {
		t.Fatalf("invoice fields: user=%d tariff=%d amount=%s status=%q months=%d future=%v",
			userID, gotTariffID, amount, invoiceStatus, months, expiresInFuture)
	}
}

func TestYooKassaEnterprise_PaymentSucceededUsesInvoiceSubscriptionMonths(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	fx := seedYooKassaEnterpriseInvoiceMonths(t, env, "pending", 3)
	const webhookPath = "yk-enterprise-months"
	srv := newYooKassaEnterpriseServer(t, env, webhookPath)

	before := time.Now()
	resp, err := postYooKassa(srv.URL, webhookPath,
		`{"event":"payment.succeeded","object":{"id":"`+fx.PaymentID+`","status":"succeeded"}}`)
	if err != nil {
		t.Fatalf("webhook POST: %v", err)
	}
	assertYooKassaWebhookOK(t, resp, "payment.succeeded")

	var activeTo time.Time
	if err := env.Pool.QueryRow(context.Background(), `
		select active_to from user_mode_access
		where user_id = $1 and mode_id = $2 and access_type = 'subscription' and source_id = $3`,
		fx.UserID, fx.ModeID, fx.InvoiceID).Scan(&activeTo); err != nil {
		t.Fatalf("query subscription access: %v", err)
	}
	if activeTo.Before(before.AddDate(0, 2, 20)) || activeTo.After(before.AddDate(0, 3, 10)) {
		t.Fatalf("subscription active_to=%s, want about 3 months from payment", activeTo.Format(time.RFC3339))
	}
}

func TestYooKassaEnterprise_RefundSucceededRevokesPaidInvoiceAccess(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	fx := seedYooKassaEnterpriseInvoiceMonths(t, env, "paid", 1)
	ctx := context.Background()
	if _, err := env.Pool.Exec(ctx, `
		insert into user_mode_access
			(user_id, mode_id, active_from, active_to, access_type, source_id, priority, created_at, updated_at)
		values ($1, $2, now() - interval '1 hour', now() + interval '1 month', 'subscription', $3, 0, now(), now())`,
		fx.UserID, fx.ModeID, fx.InvoiceID); err != nil {
		t.Fatalf("seed subscription access: %v", err)
	}
	const webhookPath = "yk-enterprise-refund"
	srv := newYooKassaEnterpriseServer(t, env, webhookPath)

	resp, err := postYooKassa(srv.URL, webhookPath,
		`{"event":"refund.succeeded","object":{"id":"refund-`+fx.PaymentID+`","status":"succeeded","payment_id":"`+fx.PaymentID+`"}}`)
	if err != nil {
		t.Fatalf("webhook POST: %v", err)
	}
	assertYooKassaWebhookOK(t, resp, "refund.succeeded")

	var invoiceStatus string
	var accessActive bool
	var processStatus string
	if err := env.Pool.QueryRow(ctx, `
		select i.status,
		       exists(
		         select 1 from user_mode_access
		         where user_id = $1 and mode_id = $2 and access_type = 'subscription' and source_id = $3 and active_to > now()
		       ),
		       e.process_status
		from invoices i
		join yookassa_webhook_events e on e.event = 'refund.succeeded' and e.object_id = $4
		where i.id = $3`,
		fx.UserID, fx.ModeID, fx.InvoiceID, "refund-"+fx.PaymentID).Scan(&invoiceStatus, &accessActive, &processStatus); err != nil {
		t.Fatalf("query refund lifecycle: %v", err)
	}
	if invoiceStatus != "refunded" || accessActive || processStatus != "accepted" {
		t.Fatalf("refund lifecycle: invoice=%q active=%v process_status=%q, want refunded/false/accepted",
			invoiceStatus, accessActive, processStatus)
	}
}

func TestYooKassaEnterprise_PaymentCanceledAfterPaidDoesNotRevokeAccess(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	fx := seedYooKassaEnterpriseInvoiceMonths(t, env, "paid", 1)
	ctx := context.Background()
	if _, err := env.Pool.Exec(ctx, `
		insert into user_mode_access
			(user_id, mode_id, active_from, active_to, access_type, source_id, priority, created_at, updated_at)
		values ($1, $2, now() - interval '1 hour', now() + interval '1 month', 'subscription', $3, 0, now(), now())`,
		fx.UserID, fx.ModeID, fx.InvoiceID); err != nil {
		t.Fatalf("seed subscription access: %v", err)
	}
	const webhookPath = "yk-enterprise-cancel-after-paid"
	srv := newYooKassaEnterpriseServer(t, env, webhookPath)

	resp, err := postYooKassa(srv.URL, webhookPath,
		`{"event":"payment.canceled","object":{"id":"`+fx.PaymentID+`","status":"canceled"}}`)
	if err != nil {
		t.Fatalf("webhook POST: %v", err)
	}
	assertYooKassaWebhookOK(t, resp, "payment.canceled")

	assertYooKassaInvoiceAndAccess(t, env, fx, "paid", 1)
}
