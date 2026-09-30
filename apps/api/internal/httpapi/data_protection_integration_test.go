//go:build integration

package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"mindstrata-stage1/api/internal/testsupport"
)

// Integration tests for docs/specs/data-protection.md. Every test owns an
// isolated database (testsupport.NewEnv) and runs in parallel.

const testConsentTextHash = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

// countingOpenAI is an OpenAI-compatible upstream that counts hits, so a test
// can prove that a blocked request never left the server.
func countingOpenAI(t *testing.T, hits *atomic.Int64) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok from upstream"}}],"usage":{"prompt_tokens":3,"completion_tokens":2}}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// seedCountryGateway puts a single enabled gateway first in the chain and
// disables every other one, so the test controls exactly where a call can go.
func seedCountryGateway(t *testing.T, pool *pgxpool.Pool, id, baseURL, country string) {
	t.Helper()
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `update ai_gateways set enabled = false`); err != nil {
		t.Fatalf("disable builtin gateways: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		insert into ai_gateways (id, title, protocol, base_url, api_key, priority, enabled, processing_country)
		values ($1, $1, 'openai', $2, 'test-key', 1, true, $3)`, id, baseURL, country); err != nil {
		t.Fatalf("seed gateway %s: %v", id, err)
	}
}

func grantConsent(t *testing.T, ts *TestServer, document string) {
	t.Helper()
	code, body := httpJSON(t, ts, "POST", "/api/privacy/consents", map[string]any{
		"document":   document,
		"version":    "2026-09-30",
		"textSha256": testConsentTextHash,
	})
	if code != http.StatusOK {
		t.Fatalf("grant consent %s: %d body=%v", document, code, body)
	}
}

// AC-2, AC-3, AC-6: in the RU profile a foreign gateway is reachable only
// while the data subject holds an active cross-border consent, and a
// withdrawal takes effect on the very next call (no cache in between).
func TestDataProtection_RUProfile_ConsentGatesForeignGateway(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	var hits atomic.Int64
	upstream := countingOpenAI(t, &hits)
	seedCountryGateway(t, env.Pool, "foreign-ai", upstream.URL, "US")

	handler := Handler{DataProtectionProfile: dataProtectionProfileRU, HTTPClient: &http.Client{Timeout: 5 * time.Second}}
	ts := NewTestServerWithHandler(t, env.Pool, handler)
	user := f.CreateUser(TestUserOpts{})
	ts.LoginAs(f.CreateSession(user.ID))

	h := Handler{DB: env.Pool, DataProtectionProfile: dataProtectionProfileRU, HTTPClient: &http.Client{Timeout: 5 * time.Second}, c: newHandlerCaches()}
	subjectCtx := withDataSubject(context.Background(), user.ID)
	spec := aiCallSpec{Provider: "foreign-ai", Model: "test-model", Temperature: 0.1, ThinkingMode: "default"}
	msgs := []map[string]string{{"role": "user", "content": "hello"}}

	_, _, _, _, err := h.doAIChatResilientSpec(subjectCtx, spec, msgs, "test", openAIChatOptions{})
	if !errors.Is(err, errCrossBorderConsentRequired) {
		t.Fatalf("without consent: err = %v, want errCrossBorderConsentRequired", err)
	}
	if hits.Load() != 0 {
		t.Fatalf("upstream was called %d times without consent", hits.Load())
	}

	grantConsent(t, ts, consentDocCrossBorder)
	if _, _, _, _, err := h.doAIChatResilientSpec(subjectCtx, spec, msgs, "test", openAIChatOptions{}); err != nil {
		t.Fatalf("with consent: unexpected error %v", err)
	}
	if hits.Load() != 1 {
		t.Fatalf("with consent: upstream hits = %d, want 1", hits.Load())
	}

	code, body := httpJSON(t, ts, "POST", "/api/privacy/consents/withdraw", map[string]any{"document": consentDocCrossBorder})
	if code != http.StatusOK {
		t.Fatalf("withdraw: %d body=%v", code, body)
	}
	_, _, _, _, err = h.doAIChatResilientSpec(subjectCtx, spec, msgs, "test", openAIChatOptions{})
	if !errors.Is(err, errCrossBorderConsentRequired) {
		t.Fatalf("after withdrawal: err = %v, want errCrossBorderConsentRequired", err)
	}
	if hits.Load() != 1 {
		t.Fatalf("after withdrawal: upstream hits = %d, want still 1", hits.Load())
	}
}

// AC-2: calls without a data subject in context (background mechanics) are
// blocked from foreign gateways in the RU profile: fail closed.
func TestDataProtection_RUProfile_NoSubjectFailsClosed(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	var hits atomic.Int64
	upstream := countingOpenAI(t, &hits)
	seedCountryGateway(t, env.Pool, "foreign-ai", upstream.URL, "DE")

	h := Handler{DB: env.Pool, DataProtectionProfile: dataProtectionProfileRU, HTTPClient: &http.Client{Timeout: 5 * time.Second}, c: newHandlerCaches()}
	spec := aiCallSpec{Provider: "foreign-ai", Model: "m", Temperature: 0.1, ThinkingMode: "default"}
	_, _, _, _, err := h.doAIChatResilientSpec(context.Background(), spec, []map[string]string{{"role": "user", "content": "x"}}, "test", openAIChatOptions{})
	if !errors.Is(err, errCrossBorderConsentRequired) {
		t.Fatalf("err = %v, want errCrossBorderConsentRequired", err)
	}
	if hits.Load() != 0 {
		t.Fatalf("upstream hits = %d, want 0", hits.Load())
	}
}

// AC-3: a gateway marked as processing in Russia needs no cross-border consent.
func TestDataProtection_RUProfile_DomesticGatewayAllowed(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	var hits atomic.Int64
	upstream := countingOpenAI(t, &hits)
	seedCountryGateway(t, env.Pool, "local-llm", upstream.URL, "RU")

	h := Handler{DB: env.Pool, DataProtectionProfile: dataProtectionProfileRU, HTTPClient: &http.Client{Timeout: 5 * time.Second}, c: newHandlerCaches()}
	spec := aiCallSpec{Provider: "local-llm", Model: "m", Temperature: 0.1, ThinkingMode: "default"}
	if _, _, _, _, err := h.doAIChatResilientSpec(context.Background(), spec, []map[string]string{{"role": "user", "content": "x"}}, "test", openAIChatOptions{}); err != nil {
		t.Fatalf("domestic gateway: %v", err)
	}
	if hits.Load() != 1 {
		t.Fatalf("upstream hits = %d, want 1", hits.Load())
	}
}

// AC-1: the default profile keeps today's behaviour: no consent needed.
func TestDataProtection_ProfileNone_NoChange(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	var hits atomic.Int64
	upstream := countingOpenAI(t, &hits)
	seedCountryGateway(t, env.Pool, "foreign-ai", upstream.URL, "US")

	h := Handler{DB: env.Pool, HTTPClient: &http.Client{Timeout: 5 * time.Second}, c: newHandlerCaches()}
	spec := aiCallSpec{Provider: "foreign-ai", Model: "m", Temperature: 0.1, ThinkingMode: "default"}
	if _, _, _, _, err := h.doAIChatResilientSpec(context.Background(), spec, []map[string]string{{"role": "user", "content": "x"}}, "test", openAIChatOptions{}); err != nil {
		t.Fatalf("profile none: %v", err)
	}
	if hits.Load() != 1 {
		t.Fatalf("upstream hits = %d, want 1", hits.Load())
	}
}

// AC-5: the chat endpoint turns the policy block into a clear 403 with a
// stable code, gives the quota slot back and never calls the provider.
func TestDataProtection_RUProfile_ChatSendReturnsConsentRequired(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	var hits atomic.Int64
	upstream := countingOpenAI(t, &hits)
	seedCountryGateway(t, env.Pool, "foreign-ai", upstream.URL, "US")

	user := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{})
	if _, err := env.Pool.Exec(context.Background(), `update modes set ai_provider = 'foreign-ai' where id = $1`, mode.ID); err != nil {
		t.Fatalf("point mode to gateway: %v", err)
	}
	f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: mode.ID, DailyMessageLimit: 50})
	dialog := f.CreateDialog(user.ID, mode.ID)
	if _, err := env.Pool.Exec(context.Background(),
		`update users set current_mode=$2, current_dialog=$3, accepted_tos=true where id=$1`,
		user.ID, mode.ID, dialog.ID); err != nil {
		t.Fatalf("seed current dialog: %v", err)
	}

	ts := NewTestServerWithHandler(t, env.Pool, Handler{DataProtectionProfile: dataProtectionProfileRU, DisableRateLimits: true, HTTPClient: &http.Client{Timeout: 5 * time.Second}})
	ts.LoginAs(f.CreateSession(user.ID))

	code, body := httpJSON(t, ts, "POST", "/api/chat/send", map[string]any{
		"dialogId":     dialog.ID,
		"text":         "Hello",
		"responseMode": "live",
	})
	if code != http.StatusForbidden || body["code"] != "cross_border_consent_required" {
		t.Fatalf("send without consent: %d body=%v; want 403 cross_border_consent_required", code, body)
	}
	if hits.Load() != 0 {
		t.Fatalf("upstream hits = %d, want 0", hits.Load())
	}
	var used int64
	_ = env.Pool.QueryRow(context.Background(), `select coalesce(sum(count), 0) from daily_message_counts where user_id = $1`, user.ID).Scan(&used)
	if used != 0 {
		t.Fatalf("quota consumed by a blocked message: %d", used)
	}

	grantConsent(t, ts, consentDocCrossBorder)
	code, body = httpJSON(t, ts, "POST", "/api/chat/send", map[string]any{
		"dialogId":     dialog.ID,
		"text":         "Hello again",
		"responseMode": "live",
	})
	if code != http.StatusOK {
		t.Fatalf("send with consent: %d body=%v", code, body)
	}
	if hits.Load() == 0 {
		t.Fatal("upstream was not called after consent")
	}
}

// AC-7, AC-8: the consent journal records document, version and text hash;
// listing shows only the caller's own records.
func TestDataProtection_ConsentJournal_GrantListWithdraw(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)
	other := f.CreateUser(TestUserOpts{})
	otherTS := NewTestServer(t, env.Pool)
	otherTS.LoginAs(f.CreateSession(other.ID))
	grantConsent(t, otherTS, consentDocPersonalData)

	user := f.CreateUser(TestUserOpts{})
	ts.LoginAs(f.CreateSession(user.ID))
	grantConsent(t, ts, consentDocPersonalData)
	grantConsent(t, ts, consentDocCrossBorder)
	grantConsent(t, ts, consentDocHealthData)

	code, body := httpJSON(t, ts, "GET", "/api/privacy/consents", nil)
	if code != http.StatusOK {
		t.Fatalf("list: %d body=%v", code, body)
	}
	items, _ := body["consents"].([]any)
	if len(items) != 3 {
		t.Fatalf("list returned %d records, want 3 (own only): %v", len(items), body)
	}
	for _, raw := range items {
		item := raw.(map[string]any)
		if item["version"] != "2026-09-30" || item["textSha256"] != testConsentTextHash {
			t.Fatalf("record lost version/hash: %v", item)
		}
		if item["withdrawnAt"] != nil {
			t.Fatalf("fresh record is withdrawn: %v", item)
		}
	}

	code, body = httpJSON(t, ts, "POST", "/api/privacy/consents/withdraw", map[string]any{"document": consentDocCrossBorder})
	if code != http.StatusOK {
		t.Fatalf("withdraw: %d body=%v", code, body)
	}
	_, body = httpJSON(t, ts, "GET", "/api/privacy/consents", nil)
	withdrawn := 0
	for _, raw := range body["consents"].([]any) {
		item := raw.(map[string]any)
		if item["document"] == consentDocCrossBorder && item["withdrawnAt"] != nil {
			withdrawn++
		}
	}
	if withdrawn != 1 {
		t.Fatalf("cross-border record not withdrawn: %v", body)
	}

	// The other user's record is untouched and not visible here.
	var otherActive int
	_ = env.Pool.QueryRow(context.Background(), `select count(*) from consent_records where user_id = $1 and withdrawn_at is null`, other.ID).Scan(&otherActive)
	if otherActive != 1 {
		t.Fatalf("other user's consent changed: active=%d", otherActive)
	}

	// Minimisation: no raw IP and no user agent are stored.
	var ipHash string
	_ = env.Pool.QueryRow(context.Background(), `select coalesce(ip_hash, '') from consent_records where user_id = $1 limit 1`, user.ID).Scan(&ipHash)
	if strings.Contains(ipHash, "127.0.0.1") {
		t.Fatalf("raw IP stored in consent journal: %q", ipHash)
	}
}

// AC-7: bad payloads and anonymous callers are rejected; an invalid session
// cookie yields 401 and clears the cookie (strict auth semantics).
func TestDataProtection_ConsentJournal_Rejections(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	anon := NewTestServer(t, env.Pool)
	if code, _ := httpJSON(t, anon, "GET", "/api/privacy/consents", nil); code != http.StatusUnauthorized {
		t.Fatalf("anonymous list: %d, want 401", code)
	}

	bad := NewTestServer(t, env.Pool)
	bad.LoginAs("not-a-real-session-token")
	req, _ := http.NewRequest("GET", bad.URL("/api/privacy/consents"), nil)
	resp, err := bad.Client.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("invalid cookie: %d, want 401", resp.StatusCode)
	}
	cleared := false
	for _, c := range resp.Cookies() {
		if c.Name == sessionCookieName && c.MaxAge < 0 {
			cleared = true
		}
	}
	if !cleared {
		t.Fatal("invalid session cookie was not cleared")
	}

	ts := NewTestServer(t, env.Pool)
	ts.LoginAs(f.CreateSession(f.CreateUser(TestUserOpts{}).ID))
	code, _ := httpJSON(t, ts, "POST", "/api/privacy/consents", map[string]any{
		"document": "unknown_doc", "version": "v1", "textSha256": testConsentTextHash,
	})
	if code != http.StatusBadRequest {
		t.Fatalf("unknown document: %d, want 400", code)
	}
}

// AC-9: the export contains the caller's data and nothing of another user.
func TestDataProtection_Export_OnlyOwnData(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	mode := f.CreateMode(TestModeOpts{})

	other := f.CreateUser(TestUserOpts{})
	otherDialog := f.CreateDialog(other.ID, mode.ID)
	f.AppendMessage(otherDialog.ID, "user", "other-user-secret-text")

	user := f.CreateUser(TestUserOpts{})
	dialog := f.CreateDialog(user.ID, mode.ID)
	f.AppendMessage(dialog.ID, "user", "my-own-message")
	f.AppendMessage(dialog.ID, "assistant", "my-own-answer")

	ts := NewTestServer(t, env.Pool)
	ts.LoginAs(f.CreateSession(user.ID))
	grantConsent(t, ts, consentDocPersonalData)

	req, _ := http.NewRequest("GET", ts.URL("/api/profile/export?userId="+itoa(other.ID)), nil)
	resp, err := ts.Client.Do(req)
	if err != nil {
		t.Fatalf("export request: %v", err)
	}
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("export: %d body=%s", resp.StatusCode, raw)
	}
	if !strings.Contains(resp.Header.Get("Content-Disposition"), "attachment") {
		t.Fatalf("export is not offered as a file: %q", resp.Header.Get("Content-Disposition"))
	}
	text := string(raw)
	if strings.Contains(text, "other-user-secret-text") {
		t.Fatal("export leaked another user's message")
	}
	for _, want := range []string{"my-own-message", "my-own-answer", consentDocPersonalData, user.Email} {
		if !strings.Contains(text, want) {
			t.Fatalf("export misses %q: %s", want, text)
		}
	}
	var parsed map[string]any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("export is not JSON: %v", err)
	}
	if parsed["format"] != userExportFormat {
		t.Fatalf("export format = %v, want %s", parsed["format"], userExportFormat)
	}

	anon := NewTestServer(t, env.Pool)
	if code, _ := httpJSON(t, anon, "GET", "/api/profile/export", nil); code != http.StatusUnauthorized {
		t.Fatalf("anonymous export: %d, want 401", code)
	}
}

// AC-10, AC-11: deleting the account erases the caller's message texts and
// files, keeps other users intact, and ignores any attempt to target another
// account through query parameters.
func TestDataProtection_DeleteAccount_ErasesOwnContentOnly(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ctx := context.Background()
	mode := f.CreateMode(TestModeOpts{})

	other := f.CreateUser(TestUserOpts{})
	otherDialog := f.CreateDialog(other.ID, mode.ID)
	otherMsg := f.AppendMessage(otherDialog.ID, "user", "other-user-text")

	user := f.CreateUser(TestUserOpts{})
	dialog := f.CreateDialog(user.ID, mode.ID)
	myMsg := f.AppendMessage(dialog.ID, "user", "my-private-text")
	if _, err := env.Pool.Exec(ctx, `
		insert into notification_contacts (user_id, channel, address, verified, source)
		values ($1, 'email', 'me@example.test', true, 'profile')`, user.ID); err != nil {
		t.Fatalf("seed contact: %v", err)
	}

	ts := NewTestServer(t, env.Pool)
	ts.LoginAs(f.CreateSession(user.ID))
	grantConsent(t, ts, consentDocCrossBorder)

	code, body := httpJSON(t, ts, "DELETE", "/api/profile?userId="+itoa(other.ID), nil)
	if code != http.StatusOK {
		t.Fatalf("delete: %d body=%v", code, body)
	}

	var content string
	_ = env.Pool.QueryRow(ctx, `select content from dialogs_messages where id = $1`, myMsg).Scan(&content)
	if content != "" {
		t.Fatalf("own message text survived deletion: %q", content)
	}
	_ = env.Pool.QueryRow(ctx, `select content from dialogs_messages where id = $1`, otherMsg).Scan(&content)
	if content != "other-user-text" {
		t.Fatalf("other user's message changed: %q", content)
	}
	var otherDeleted bool
	_ = env.Pool.QueryRow(ctx, `select deleted_at is not null from users where id = $1`, other.ID).Scan(&otherDeleted)
	if otherDeleted {
		t.Fatal("another user's account was deleted")
	}
	var contacts int
	_ = env.Pool.QueryRow(ctx, `select count(*) from notification_contacts where user_id = $1`, user.ID).Scan(&contacts)
	if contacts != 0 {
		t.Fatalf("notification contacts survived deletion: %d", contacts)
	}
	var activeConsents int
	_ = env.Pool.QueryRow(ctx, `select count(*) from consent_records where user_id = $1 and withdrawn_at is null`, user.ID).Scan(&activeConsents)
	if activeConsents != 0 {
		t.Fatalf("consents stayed active after deletion: %d", activeConsents)
	}
}

// AC-13, AC-14: opening a dialog or a user card in the admin panel leaves an
// access record with the subject id but without any message text; only
// owner/admin can read the access log.
func TestDataProtection_AdminAccessLog(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ctx := context.Background()
	mode := f.CreateMode(TestModeOpts{})
	subject := f.CreateUser(TestUserOpts{})
	dialog := f.CreateDialog(subject.ID, mode.ID)
	f.AppendMessage(dialog.ID, "user", "very-private-dialog-text")

	support := f.CreateUser(TestUserOpts{Role: "support"})
	supportTS := NewTestServer(t, env.Pool)
	supportTS.LoginAs(f.CreateSession(support.ID))
	if code, body := httpJSON(t, supportTS, "GET", "/api/admin/dialogs/"+itoa(dialog.ID), nil); code != http.StatusOK {
		t.Fatalf("support opens dialog: %d body=%v", code, body)
	}
	if code, body := httpJSON(t, supportTS, "GET", "/api/admin/users/"+itoa(subject.ID), nil); code != http.StatusOK {
		t.Fatalf("support opens user card: %d body=%v", code, body)
	}

	var rows int
	var leaked bool
	_ = env.Pool.QueryRow(ctx, `
		select count(*), coalesce(bool_or(meta::text like '%very-private-dialog-text%'), false)
		from admin_audit_log
		where actor_user_id = $1 and action in ('admin.dialog.read', 'admin.user.read')
		  and (meta->>'subjectUserId')::bigint = $2`, support.ID, subject.ID).Scan(&rows, &leaked)
	if rows != 2 {
		t.Fatalf("access records = %d, want 2", rows)
	}
	if leaked {
		t.Fatal("access log stores message text")
	}

	if code, _ := httpJSON(t, supportTS, "GET", "/api/admin/data-protection/access-log", nil); code != http.StatusForbidden {
		t.Fatalf("support reads access log: %d, want 403", code)
	}
	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	adminTS := NewTestServer(t, env.Pool)
	adminTS.LoginAs(f.CreateSession(admin.ID))
	code, body := httpJSON(t, adminTS, "GET", "/api/admin/data-protection/access-log?subjectUserId="+itoa(subject.ID), nil)
	if code != http.StatusOK {
		t.Fatalf("admin reads access log: %d body=%v", code, body)
	}
	entries, _ := body["entries"].([]any)
	if len(entries) != 2 {
		t.Fatalf("access log entries = %d, want 2: %v", len(entries), body)
	}
}

// AC-15, AC-16: admin reads the installation profile and marks where a
// gateway processes data; the mark is validated and audited.
func TestDataProtection_AdminGatewayCountry(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ctx := context.Background()
	seedGateway(t, env.Pool, "local-llm", "openai", 50)

	ts := NewTestServerWithHandler(t, env.Pool, Handler{DataProtectionProfile: dataProtectionProfileRU, DisableRateLimits: true})
	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	code, body := httpJSON(t, ts, "POST", "/api/admin/data-protection/gateway-country", map[string]any{"gatewayId": "local-llm", "country": "ru"})
	if code != http.StatusOK {
		t.Fatalf("set country: %d body=%v", code, body)
	}
	var country string
	_ = env.Pool.QueryRow(ctx, `select processing_country from ai_gateways where id = 'local-llm'`).Scan(&country)
	if country != "RU" {
		t.Fatalf("stored country = %q, want RU", country)
	}
	if code, _ := httpJSON(t, ts, "POST", "/api/admin/data-protection/gateway-country", map[string]any{"gatewayId": "local-llm", "country": "Russia"}); code != http.StatusBadRequest {
		t.Fatalf("invalid country: %d, want 400", code)
	}
	if code, _ := httpJSON(t, ts, "POST", "/api/admin/data-protection/gateway-country", map[string]any{"gatewayId": "no-such", "country": "RU"}); code != http.StatusNotFound {
		t.Fatalf("unknown gateway: %d, want 404", code)
	}

	code, body = httpJSON(t, ts, "GET", "/api/admin/data-protection", nil)
	if code != http.StatusOK || body["profile"] != dataProtectionProfileRU {
		t.Fatalf("overview: %d body=%v", code, body)
	}
	// No storage country declared: the RU profile must say so (AC-24).
	warnings, _ := body["warnings"].([]any)
	if len(warnings) != 1 || warnings[0] != "storage_country_unknown" {
		t.Fatalf("overview warnings = %v, want [storage_country_unknown]", body["warnings"])
	}

	user := NewTestServer(t, env.Pool)
	user.LoginAs(f.CreateSession(f.CreateUser(TestUserOpts{}).ID))
	if code, _ := httpJSON(t, user, "GET", "/api/admin/data-protection", nil); code != http.StatusForbidden {
		t.Fatalf("plain user reads overview: %d, want 403", code)
	}
}

// Spec-only acceptance criteria (see docs/specs/data-protection.md).

func TestSpecDataProtection_AC12_BackupExpiry(t *testing.T) {
	t.Parallel()
	t.Skip("SPEC data-protection AC-12: not implemented")
}

func TestSpecDataProtection_AC18_RetentionPolicies(t *testing.T) {
	t.Parallel()
	t.Skip("SPEC data-protection AC-18: not implemented")
}

func TestSpecDataProtection_AC19_ConsentUI(t *testing.T) {
	t.Parallel()
	t.Skip("SPEC data-protection AC-19: not implemented")
}

func TestSpecDataProtection_AC22_BreachRegister(t *testing.T) {
	t.Parallel()
	t.Skip("SPEC data-protection AC-22: not implemented")
}

func TestSpecDataProtection_AC23_AIDisclosure(t *testing.T) {
	t.Parallel()
	t.Skip("SPEC data-protection AC-23: not implemented")
}

func TestSpecDataProtection_AC25_HealthDataConsentRequired(t *testing.T) {
	t.Parallel()
	t.Skip("SPEC data-protection AC-25: not implemented")
}

func TestSpecDataProtection_AC26_CrisisProtocol(t *testing.T) {
	t.Parallel()
	t.Skip("SPEC data-protection AC-26: not implemented")
}

func TestSpecDataProtection_AC27_CrisisHelplinesPerCountry(t *testing.T) {
	t.Parallel()
	t.Skip("SPEC data-protection AC-27: not implemented")
}

func TestSpecDataProtection_AC28_AgeGate(t *testing.T) {
	t.Parallel()
	t.Skip("SPEC data-protection AC-28: not implemented")
}

func TestSpecDataProtection_AC29_MinorsPsychologicalModes(t *testing.T) {
	t.Parallel()
	t.Skip("SPEC data-protection AC-29: not implemented")
}
