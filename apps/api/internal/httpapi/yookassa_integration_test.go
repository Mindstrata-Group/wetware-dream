//go:build integration

package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"mindstrata-stage1/api/internal/testsupport"
)

// fakeYooKassa returns an httptest.Server that mocks api.yookassa.ru.
// Tests inject this via Handler.HTTPClient targeting yk.URL via custom Transport.
func fakeYooKassa(t *testing.T, status int, body map[string]any) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Basic ") {
			t.Errorf("missing Basic auth header")
		}
		if r.Header.Get("Idempotence-Key") == "" {
			t.Errorf("missing Idempotence-Key header (ЮKassa requires it)")
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// rewriteToFakeYooKassa returns a RoundTripper that redirects
// api.yookassa.ru → fake httptest server.
type rewriteRT struct {
	target string
}

func (r rewriteRT) RoundTrip(req *http.Request) (*http.Response, error) {
	if strings.Contains(req.URL.Host, "yookassa.ru") {
		// Rewrite to fake server.
		u := *req.URL
		u.Scheme = "http"
		u.Host = strings.TrimPrefix(r.target, "http://")
		req.URL = &u
		req.Host = u.Host
	}
	return http.DefaultTransport.RoundTrip(req)
}

// yookassaTestServer: helper that builds a TestServer with YooKassa env pre-set.
// Handler is captured by value in NewTestServer, so setting fields after
// construction has no effect — must inject before router build.
func yookassaTestServer(t *testing.T, env *testsupport.Env, ykHTTPClient *http.Client) *TestServer {
	t.Helper()
	handler := Handler{
		DB:                  env.Pool,
		YooKassaShopID:      "12345",
		YooKassaSecretKey:   "test_secret",
		YooKassaWebhookPath: "test-secret-path",
		HTTPClient:          ykHTTPClient,
	}
	if handler.HTTPClient == nil {
		handler.HTTPClient = &http.Client{Timeout: 5 * time.Second}
	}
	mux := NewRouter(handler, []string{"*"})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	jar, _ := cookiejar.New(nil)
	return &TestServer{
		t: t, server: srv, Handler: handler,
		Client: &http.Client{
			Timeout: 10 * time.Second,
			Jar:     jar,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

// TestYooKassa_CreatePayment_RequiresBillingAdminOrAbove: user/tester → 403.
func TestYooKassa_CreatePayment_RequiresBillingAdminOrAbove(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := yookassaTestServer(t, env, nil)

	for _, role := range []string{"user", "tester", "expert", "support", "content_admin"} {
		user := f.CreateUser(TestUserOpts{Role: role})
		ts.LoginAs(f.CreateSession(user.ID))
		status, _ := httpJSON(t, ts, "POST", "/api/admin/payments/yookassa/create",
			map[string]any{"amountRub": 100.0})
		if status != http.StatusForbidden && status != http.StatusUnauthorized {
			t.Errorf("role %q: expected 403/401, got %d", role, status)
		}
	}
}

// TestYooKassa_CreatePayment_RequiresConfig: env vars not set → 503.
func TestYooKassa_CreatePayment_RequiresConfig(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)
	// Deliberately not setting ShopID / SecretKey.

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	status, body := httpJSON(t, ts, "POST", "/api/admin/payments/yookassa/create",
		map[string]any{"amountRub": 100.0})
	if status != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 (yookassa not configured), got %d body=%v", status, body)
	}
	if c, _ := body["code"].(string); c != "yookassa_not_configured" {
		t.Fatalf("expected code=yookassa_not_configured, got %v", body["code"])
	}
}

// TestYooKassa_CreatePayment_RejectsNegativeAmount.
func TestYooKassa_CreatePayment_RejectsNegativeAmount(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := yookassaTestServer(t, env, nil)

	admin := f.CreateUser(TestUserOpts{Role: "billing_admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	for _, amt := range []float64{0, -1, -100.50} {
		status, _ := httpJSON(t, ts, "POST", "/api/admin/payments/yookassa/create",
			map[string]any{"amountRub": amt})
		if status != http.StatusBadRequest {
			t.Errorf("amount=%v: expected 400, got %d", amt, status)
		}
	}
}

// TestYooKassa_CreatePayment_BillingAdmin_Success: 200 + confirmationUrl.
//
// The handler makes an HTTP request to api.yookassa.ru. We mock it through
// rewriteRT, intercepting the URL and sending it to a fake server.
func TestYooKassa_CreatePayment_BillingAdmin_Success(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)

	// Mock a YooKassa response with confirmation_url.
	fake := fakeYooKassa(t, 200, map[string]any{
		"id":     "test-payment-id",
		"status": "pending",
		"amount": map[string]string{"value": "100.00", "currency": "RUB"},
		"confirmation": map[string]any{
			"type":             "redirect",
			"confirmation_url": "https://yookassa.ru/checkout/payments/test-payment-id",
		},
	})

	ts := yookassaTestServer(t, env, &http.Client{
		Transport: rewriteRT{target: fake.URL},
		Timeout:   5 * time.Second,
	})

	admin := f.CreateUser(TestUserOpts{Role: "billing_admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	status, body := httpJSON(t, ts, "POST", "/api/admin/payments/yookassa/create",
		map[string]any{
			"amountRub":         150.50,
			"savePaymentMethod": true,
			"description":       "Test payment",
		})
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%v", status, body)
	}
	if ok, _ := body["ok"].(bool); !ok {
		t.Fatalf("ok=false: %v", body)
	}
	confirmationURL, _ := body["confirmationUrl"].(string)
	if !strings.Contains(confirmationURL, "yookassa.ru/checkout") {
		t.Fatalf("missing/wrong confirmationUrl: %v", confirmationURL)
	}
}

// TestYooKassa_CreatePayment_UpstreamError: YooKassa returns 400 → 502.
func TestYooKassa_CreatePayment_UpstreamError(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)

	fake := fakeYooKassa(t, 400, map[string]any{
		"type":        "error",
		"description": "Invalid amount currency",
	})

	ts := yookassaTestServer(t, env, &http.Client{
		Transport: rewriteRT{target: fake.URL},
		Timeout:   5 * time.Second,
	})

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	status, body := httpJSON(t, ts, "POST", "/api/admin/payments/yookassa/create",
		map[string]any{"amountRub": 100.0})
	if status != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d body=%v", status, body)
	}
}

// yooKassaTestIP: an IP in the allowlist, used by every webhook test via
// X-Forwarded-For. Without it a Caddy-style proxied IP cannot be determined.
const yooKassaTestIP = "185.71.76.5"

// postYooKassa: helper: POST with the correct IP (as from YooKassa through Caddy).
func postYooKassa(srvURL, path, body string) (*http.Response, error) {
	req, err := http.NewRequest("POST", srvURL+"/webhooks/yookassa/"+path, strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Forwarded-For", yooKassaTestIP)
	return http.DefaultClient.Do(req)
}

func readYooKassaWebhookOutcome(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()
	defer resp.Body.Close()
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode webhook response: %v", err)
	}
	return body
}

func assertYooKassaWebhookOutcome(t *testing.T, resp *http.Response, wantStatus int, wantOutcome string) {
	t.Helper()
	body := readYooKassaWebhookOutcome(t, resp)
	if resp.StatusCode != wantStatus {
		t.Fatalf("webhook status: got %d want %d body=%v", resp.StatusCode, wantStatus, body)
	}
	if got, _ := body["outcome"].(string); got != wantOutcome {
		t.Fatalf("webhook outcome: got %q want %q body=%v", got, wantOutcome, body)
	}
}

// TestYooKassaWebhook_AcceptsValidPayload: POST with a payload + YK IP → 200.
func TestYooKassaWebhook_AcceptsValidPayload(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	const webhookPath = "test-webhook-secret-abc"
	handler := Handler{
		DB:                  env.Pool,
		YooKassaWebhookPath: webhookPath,
		HTTPClient:          &http.Client{Timeout: 5 * time.Second},
	}
	mux := NewRouter(handler, []string{"*"})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	resp, err := postYooKassa(srv.URL, webhookPath,
		`{"event":"payment.waiting_for_capture","object":{"id":"abc","status":"waiting_for_capture"}}`)
	if err != nil {
		t.Fatalf("webhook POST: %v", err)
	}
	assertYooKassaWebhookOutcome(t, resp, http.StatusOK, "accepted")
}

// TestYooKassaWebhook_RejectsMalformedJSON (correct IP, but broken JSON).
func TestYooKassaWebhook_RejectsMalformedJSON(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	const webhookPath = "test-webhook-secret-abc"
	handler := Handler{
		DB:                  env.Pool,
		YooKassaWebhookPath: webhookPath,
		HTTPClient:          &http.Client{Timeout: 5 * time.Second},
	}
	mux := NewRouter(handler, []string{"*"})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	resp, err := postYooKassa(srv.URL, webhookPath, "not a json at all }{")
	if err != nil {
		t.Fatalf("webhook POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

// TestYooKassaWebhook_NotRegisteredWhenPathEmpty: without env → the route does not exist.
func TestYooKassaWebhook_NotRegisteredWhenPathEmpty(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	handler := Handler{
		DB:                  env.Pool,
		YooKassaWebhookPath: "", // deliberately empty
		HTTPClient:          &http.Client{Timeout: 5 * time.Second},
	}
	mux := NewRouter(handler, []string{"*"})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	resp, err := http.Post(srv.URL+"/webhooks/yookassa/any-path",
		"application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	// http.ServeMux: unregistered path → 404.
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 (not registered), got %d", resp.StatusCode)
	}
}

// REGRESSION: all 5 events from the real YooKassa account settings must be
// accepted. If someone adds an event-type filter (whitelist) and forgets one of
// these, this test shows it.
func TestYooKassaWebhook_AcceptsAll5ProductionEvents(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ctx := context.Background()
	const webhookPath = "production-events-test"
	handler := Handler{
		DB:                  env.Pool,
		YooKassaWebhookPath: webhookPath,
		HTTPClient:          &http.Client{Timeout: 5 * time.Second},
	}
	mux := NewRouter(handler, []string{"*"})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	user := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{})
	var tariffID int64
	if err := env.Pool.QueryRow(ctx, `
		insert into tariffs (name, monthly_price, daily_message_limit, limit_type, available_for_subscription)
		values ($1, 100.00, 50, 'shared', true)
		returning id`, "yk-events-"+mode.Name).Scan(&tariffID); err != nil {
		t.Fatalf("seed tariff: %v", err)
	}
	if _, err := env.Pool.Exec(ctx, `insert into tariff_mode (tariff_id, mode_id) values ($1, $2)`, tariffID, mode.ID); err != nil {
		t.Fatalf("seed tariff_mode: %v", err)
	}
	if _, err := env.Pool.Exec(ctx, `
		insert into invoices (user_id, tariff_id, yookassa_payment_id, amount, currency, status, subscription_months, expires_at)
		values ($1, $2, 'events-paid-001', 100.00, 'RUB', 'pending', 1, now() + interval '1 hour')`,
		user.ID, tariffID); err != nil {
		t.Fatalf("seed invoice: %v", err)
	}

	// The list is taken 1:1 from the YooKassa settings.
	events := []struct {
		name    string
		payload string
	}{
		{"payment.succeeded", `{"event":"payment.succeeded","object":{"id":"events-paid-001","status":"succeeded"}}`},
		{"payment.waiting_for_capture", `{"event":"payment.waiting_for_capture","object":{"id":"test-payment.waiting_for_capture"}}`},
		{"payment.canceled", `{"event":"payment.canceled","object":{"id":"test-payment.canceled"}}`},
		{"payment_method.active", `{"event":"payment_method.active","object":{"id":"test-payment_method.active"}}`},
		{"refund.succeeded", `{"event":"refund.succeeded","object":{"id":"test-refund.succeeded"}}`},
	}

	for _, ev := range events {
		t.Run(ev.name, func(t *testing.T) {
			resp, err := postYooKassa(srv.URL, webhookPath, ev.payload)
			if err != nil {
				t.Fatalf("POST %s: %v", ev.name, err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				raw, _ := io.ReadAll(resp.Body)
				t.Errorf("event %s: status %d (expected 200), body=%s", ev.name, resp.StatusCode, raw)
			}
		})
	}
}

func TestYooKassaWebhook_OutcomeModel(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ctx := context.Background()
	const webhookPath = "outcome-model-test"
	handler := Handler{
		DB:                  env.Pool,
		YooKassaWebhookPath: webhookPath,
		HTTPClient:          &http.Client{Timeout: 5 * time.Second},
	}
	mux := NewRouter(handler, []string{"*"})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	user := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{})
	var tariffID int64
	if err := env.Pool.QueryRow(ctx, `
		insert into tariffs (name, monthly_price, daily_message_limit, limit_type, available_for_subscription)
		values ($1, 100.00, 50, 'shared', true)
		returning id`, "yk-outcome-"+mode.Name).Scan(&tariffID); err != nil {
		t.Fatalf("seed tariff: %v", err)
	}
	if _, err := env.Pool.Exec(ctx, `insert into tariff_mode (tariff_id, mode_id) values ($1, $2)`, tariffID, mode.ID); err != nil {
		t.Fatalf("seed tariff_mode: %v", err)
	}
	for _, seed := range []struct {
		paymentID string
		status    string
	}{
		{"outcome-paid-001", "pending"},
		{"outcome-canceled-001", "pending"},
		{"outcome-refunded-payment-001", "paid"},
	} {
		if _, err := env.Pool.Exec(ctx, `
			insert into invoices (user_id, tariff_id, yookassa_payment_id, amount, currency, status, subscription_months, expires_at)
			values ($1, $2, $3, 100.00, 'RUB', $4, 1, now() + interval '1 hour')`,
			user.ID, tariffID, seed.paymentID, seed.status); err != nil {
			t.Fatalf("seed invoice %s: %v", seed.paymentID, err)
		}
	}

	cases := []struct {
		name          string
		payload       string
		ip            string
		wantHTTP      int
		wantOutcome   string
		wantDBStatus  string
		wantPersisted bool
	}{
		{
			name:          "payment succeeded accepted",
			payload:       `{"event":"payment.succeeded","object":{"id":"outcome-paid-001","status":"succeeded"}}`,
			ip:            yooKassaTestIP,
			wantHTTP:      http.StatusOK,
			wantOutcome:   "accepted",
			wantDBStatus:  "accepted",
			wantPersisted: true,
		},
		{
			name:          "payment canceled accepted",
			payload:       `{"event":"payment.canceled","object":{"id":"outcome-canceled-001","status":"canceled"}}`,
			ip:            yooKassaTestIP,
			wantHTTP:      http.StatusOK,
			wantOutcome:   "accepted",
			wantDBStatus:  "accepted",
			wantPersisted: true,
		},
		{
			name:          "waiting for capture accepted",
			payload:       `{"event":"payment.waiting_for_capture","object":{"id":"outcome-waiting-001","status":"waiting_for_capture"}}`,
			ip:            yooKassaTestIP,
			wantHTTP:      http.StatusOK,
			wantOutcome:   "accepted",
			wantDBStatus:  "accepted",
			wantPersisted: true,
		},
		{
			name:          "payment method active accepted",
			payload:       `{"event":"payment_method.active","object":{"id":"outcome-method-001","status":"active"}}`,
			ip:            yooKassaTestIP,
			wantHTTP:      http.StatusOK,
			wantOutcome:   "accepted",
			wantDBStatus:  "accepted",
			wantPersisted: true,
		},
		{
			name:          "refund succeeded accepted",
			payload:       `{"event":"refund.succeeded","object":{"id":"outcome-refund-001","status":"succeeded","payment_id":"outcome-refunded-payment-001"}}`,
			ip:            yooKassaTestIP,
			wantHTTP:      http.StatusOK,
			wantOutcome:   "accepted",
			wantDBStatus:  "accepted",
			wantPersisted: true,
		},
		{
			name:          "invoice missing retryable",
			payload:       `{"event":"payment.succeeded","object":{"id":"outcome-missing-invoice-001","status":"succeeded"}}`,
			ip:            yooKassaTestIP,
			wantHTTP:      http.StatusInternalServerError,
			wantOutcome:   "retryable",
			wantDBStatus:  "retryable",
			wantPersisted: true,
		},
		{
			name:          "malformed json",
			payload:       `{"event":`,
			ip:            yooKassaTestIP,
			wantHTTP:      http.StatusBadRequest,
			wantOutcome:   "malformed",
			wantPersisted: false,
		},
		{
			name:          "non yookassa ip suspicious",
			payload:       `{"event":"payment.succeeded","object":{"id":"outcome-bad-ip-001","status":"succeeded"}}`,
			ip:            "8.8.8.8",
			wantHTTP:      http.StatusForbidden,
			wantOutcome:   "suspicious",
			wantPersisted: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest("POST", srv.URL+"/webhooks/yookassa/"+webhookPath, strings.NewReader(tc.payload))
			if err != nil {
				t.Fatalf("request: %v", err)
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Forwarded-For", tc.ip)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("POST: %v", err)
			}
			assertYooKassaWebhookOutcome(t, resp, tc.wantHTTP, tc.wantOutcome)

			var count int
			var status string
			objectID := extractYooKassaObjectIDForTest(t, tc.payload)
			if objectID != "" {
				err = env.Pool.QueryRow(ctx, `
					select count(*), coalesce(max(process_status), '')
					from yookassa_webhook_events
					where object_id = $1`, objectID).Scan(&count, &status)
				if err != nil {
					t.Fatalf("query webhook event: %v", err)
				}
				if tc.wantPersisted {
					if count != 1 || status != tc.wantDBStatus {
						t.Fatalf("webhook db state: count=%d status=%q want 1/%q", count, status, tc.wantDBStatus)
					}
				} else if count != 0 {
					t.Fatalf("webhook db state: count=%d want 0", count)
				}
			}
		})
	}

	duplicatePayload := `{"event":"payment_method.active","object":{"id":"outcome-duplicate-001","status":"active"}}`
	resp1, err := postYooKassa(srv.URL, webhookPath, duplicatePayload)
	if err != nil {
		t.Fatalf("duplicate first POST: %v", err)
	}
	assertYooKassaWebhookOutcome(t, resp1, http.StatusOK, "accepted")
	resp2, err := postYooKassa(srv.URL, webhookPath, duplicatePayload)
	if err != nil {
		t.Fatalf("duplicate second POST: %v", err)
	}
	assertYooKassaWebhookOutcome(t, resp2, http.StatusOK, "duplicate")
}

func extractYooKassaObjectIDForTest(t *testing.T, payload string) string {
	t.Helper()
	var decoded yooKassaWebhookPayload
	if err := json.Unmarshal([]byte(payload), &decoded); err != nil {
		return ""
	}
	return decoded.Object.ID
}

// =============================================================================
// SECURITY TESTS (2026-05-29) — IP allowlist + idempotency + signature
// =============================================================================

// TestYooKassaWebhook_RejectsForeignIP: POST from a NON-YooKassa IP → 403.
func TestYooKassaWebhook_RejectsForeignIP(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	const webhookPath = "ip-test"
	handler := Handler{
		DB:                  env.Pool,
		YooKassaWebhookPath: webhookPath,
		HTTPClient:          &http.Client{Timeout: 5 * time.Second},
	}
	mux := NewRouter(handler, []string{"*"})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	// An IP outside the allowlist (Google DNS).
	req, _ := http.NewRequest("POST", srv.URL+"/webhooks/yookassa/"+webhookPath,
		strings.NewReader(`{"event":"payment.succeeded","object":{"id":"x"}}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Forwarded-For", "8.8.8.8")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("foreign IP: expected 403, got %d", resp.StatusCode)
	}
}

// TestYooKassaWebhook_AcceptsAllAllowlistRanges: check that each official
// YooKassa range is let through. If someone mistakenly edits a CIDR, this test
// shows it.
func TestYooKassaWebhook_AcceptsAllAllowlistRanges(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	// Warm up the pool: the first connection to a fresh test DB is established
	// lazily and on a loaded CI can take >3 s, exceeding the handler timeout.
	if err := env.Pool.Ping(context.Background()); err != nil {
		t.Fatalf("pool ping: %v", err)
	}
	const webhookPath = "allowlist-test"
	handler := Handler{
		DB:                  env.Pool,
		YooKassaWebhookPath: webhookPath,
		HTTPClient:          &http.Client{Timeout: 5 * time.Second},
	}
	mux := NewRouter(handler, []string{"*"})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	// One sample from each range + the exact addresses.
	samples := []string{
		"185.71.76.1",   // 185.71.76.0/27
		"185.71.77.30",  // 185.71.77.0/27
		"77.75.153.50",  // 77.75.153.0/25
		"77.75.154.200", // 77.75.154.128/25
		"77.75.156.11",  // exact
		"77.75.156.35",  // exact
		"2a02:5180::1",  // 2a02:5180::/32
	}
	for _, ip := range samples {
		t.Run(ip, func(t *testing.T) {
			req, _ := http.NewRequest("POST", srv.URL+"/webhooks/yookassa/"+webhookPath,
				strings.NewReader(`{"event":"payment_method.active","object":{"id":"allowlist-`+ip+`"}}`))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Forwarded-For", ip)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("POST: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				body, _ := io.ReadAll(resp.Body)
				t.Errorf("IP %s rejected: status=%d body=%s", ip, resp.StatusCode, body)
			}
		})
	}
}

// TestYooKassaWebhook_Idempotent: redelivery of the same event+object_id must
// not create a second DB record, but returns 200 (as YooKassa requires).
func TestYooKassaWebhook_Idempotent(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	const webhookPath = "idem-test"
	handler := Handler{
		DB:                  env.Pool,
		YooKassaWebhookPath: webhookPath,
		HTTPClient:          &http.Client{Timeout: 5 * time.Second},
	}
	mux := NewRouter(handler, []string{"*"})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	payload := `{"event":"payment_method.active","object":{"id":"idem-test-id-001"}}`

	// 1st POST
	resp1, err := postYooKassa(srv.URL, webhookPath, payload)
	if err != nil {
		t.Fatalf("1st POST: %v", err)
	}
	_ = resp1.Body.Close()
	if resp1.StatusCode != http.StatusOK {
		t.Fatalf("1st status: %d", resp1.StatusCode)
	}

	// 2nd POST: duplicate
	resp2, err := postYooKassa(srv.URL, webhookPath, payload)
	if err != nil {
		t.Fatalf("2nd POST: %v", err)
	}
	_ = resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("2nd status: %d (idempotent должен быть 200)", resp2.StatusCode)
	}

	// Exactly one record in the DB.
	var cnt int64
	_ = env.Pool.QueryRow(context.Background(),
		`select count(*) from yookassa_webhook_events
		 where event = 'payment_method.active' and object_id = 'idem-test-id-001'`).Scan(&cnt)
	if cnt != 1 {
		t.Errorf("expected exactly 1 row after duplicate, got %d", cnt)
	}
}

// TestYooKassaWebhook_PaymentSucceeded_DuplicateDoesNotDoubleGrantAccess:
// redelivery of an already processed payment.succeeded must not grant access
// again or change the invoice's business status.
func TestYooKassaWebhook_PaymentSucceeded_DuplicateDoesNotDoubleGrantAccess(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ctx := context.Background()
	const webhookPath = "paid-idem-test"
	const paymentID = "paid-idem-test-001"

	user := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{})

	var tariffID int64
	if err := env.Pool.QueryRow(ctx, `
		insert into tariffs (name, monthly_price, daily_message_limit, limit_type, available_for_subscription)
		values ($1, 100.00, 50, 'shared', true)
		returning id`, "yk-idem-"+mode.Name).Scan(&tariffID); err != nil {
		t.Fatalf("seed tariff: %v", err)
	}
	if _, err := env.Pool.Exec(ctx,
		`insert into tariff_mode (tariff_id, mode_id) values ($1, $2)`,
		tariffID, mode.ID); err != nil {
		t.Fatalf("seed tariff_mode: %v", err)
	}
	if _, err := env.Pool.Exec(ctx, `
		insert into invoices (user_id, tariff_id, yookassa_payment_id, amount, currency, status, subscription_months, expires_at)
		values ($1, $2, $3, 100.00, 'RUB', 'pending', 1, now() + interval '1 hour')`,
		user.ID, tariffID, paymentID); err != nil {
		t.Fatalf("seed invoice: %v", err)
	}

	handler := Handler{
		DB:                  env.Pool,
		YooKassaWebhookPath: webhookPath,
		HTTPClient:          &http.Client{Timeout: 5 * time.Second},
	}
	mux := NewRouter(handler, []string{"*"})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	payload := `{"event":"payment.succeeded","object":{"id":"` + paymentID + `","status":"succeeded"}}`
	for i := 1; i <= 2; i++ {
		resp, err := postYooKassa(srv.URL, webhookPath, payload)
		if err != nil {
			t.Fatalf("POST #%d: %v", i, err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("POST #%d status: got %d want 200", i, resp.StatusCode)
		}
	}

	var accessCount int64
	if err := env.Pool.QueryRow(ctx, `
		select count(*) from user_mode_access
		where user_id = $1
		  and mode_id = $2
		  and access_type = 'subscription'
		  and source_id = (select id from invoices where yookassa_payment_id = $3)`,
		user.ID, mode.ID, paymentID).Scan(&accessCount); err != nil {
		t.Fatalf("query access count: %v", err)
	}
	if accessCount != 1 {
		t.Fatalf("duplicate payment.succeeded granted access %d times, want exactly 1", accessCount)
	}

	var invoiceStatus string
	var paidAtPresent bool
	if err := env.Pool.QueryRow(ctx, `
		select status, paid_at is not null
		from invoices
		where yookassa_payment_id = $1`, paymentID).Scan(&invoiceStatus, &paidAtPresent); err != nil {
		t.Fatalf("query invoice status: %v", err)
	}
	if invoiceStatus != "paid" || !paidAtPresent {
		t.Fatalf("invoice after payment.succeeded: status=%q paid_at_present=%v, want paid/true", invoiceStatus, paidAtPresent)
	}

	var eventCount int64
	var processStatus string
	if err := env.Pool.QueryRow(ctx, `
		select count(*), max(process_status)
		from yookassa_webhook_events
		where event = 'payment.succeeded' and object_id = $1`, paymentID).Scan(&eventCount, &processStatus); err != nil {
		t.Fatalf("query webhook event: %v", err)
	}
	if eventCount != 1 || processStatus != "accepted" {
		t.Fatalf("webhook event after duplicate: count=%d process_status=%q, want 1/accepted", eventCount, processStatus)
	}
}

// TestYooKassaWebhook_PersistsEventToBD: a record with remote_ip and event.
func TestYooKassaWebhook_PersistsEventToBD(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	const webhookPath = "persist-test"
	handler := Handler{
		DB:                  env.Pool,
		YooKassaWebhookPath: webhookPath,
		HTTPClient:          &http.Client{Timeout: 5 * time.Second},
	}
	mux := NewRouter(handler, []string{"*"})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	resp, err := postYooKassa(srv.URL, webhookPath,
		`{"event":"refund.succeeded","object":{"id":"persist-001"}}`)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: %d", resp.StatusCode)
	}

	var ip string
	var event string
	_ = env.Pool.QueryRow(context.Background(),
		`select event, remote_ip from yookassa_webhook_events where object_id = 'persist-001'`).Scan(&event, &ip)
	if event != "refund.succeeded" {
		t.Errorf("event field: %q", event)
	}
	if ip != yooKassaTestIP {
		t.Errorf("remote_ip: got %q want %q", ip, yooKassaTestIP)
	}
}

// A signature / Signing Secret for HTTP Basic Auth does NOT exist in YooKassa;
// see https://yookassa.ru/developers/using-api/webhooks (the "Authenticity check"
// section). Official methods: IP allowlist + checking the object status via the API.
// The IP allowlist is already covered above. Status checking is an optional step
// (see yookassa_status_check_test.go, if we enable it in the future).
