//go:build integration

package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	"mindstrata-stage1/api/internal/testsupport"
)

var enterpriseRouteInventory = []string{
	"/api/_error",
	// The training game "Department N": public practicum routes, no auth.
	"/api/game/result",
	"/api/game/stats",
	"/api/game/results",
	"/api/game/tasks",
	"/api/game/session",
	"/api/mcp/call",
	"/api/access/promocode/apply",
	"/api/debug/client-log",
	"/api/access/status",
	"/api/admin/access",
	"/api/admin/ai-gateways",
	"/api/admin/ai-gateways/",
	"/api/admin/ai-settings",
	"/api/admin/analytics-settings",
	"/api/admin/broadcasts",
	"/api/admin/dialog-summary-prompt",
	"/api/admin/lead-summary-prompt",
	"/api/admin/dialogs",
	"/api/admin/dialogs/",
	"/api/admin/exports/messages",
	"/api/admin/ai-models",
	"/api/admin/modes",
	"/api/admin/modes/",
	"/api/admin/modes/guardrail",
	"/api/admin/modes/model-stats",
	"/api/admin/notifications/history",
	"/api/admin/notifications/preview",
	"/api/admin/notifications/send",
	"/api/admin/notifications/templates",
	"/api/admin/notifications/test",
	"/api/admin/orchestration-prompt",
	"/api/admin/payments",
	"/api/admin/payments/access-recovery",
	"/api/admin/payments/subscriptions/revoke",
	"/api/admin/payments/yookassa/config",
	"/api/admin/payments/yookassa/create",
	"/api/admin/payments/yookassa/renewals/run",
	"/api/admin/payments/yookassa/run-due-renewals",
	"/api/admin/payments/yookassa/test-charge",
	"/api/admin/client-logs",
	"/api/admin/data-protection",
	"/api/admin/data-protection/access-log",
	"/api/admin/data-protection/gateway-country",
	"/api/admin/promocodes",
	"/api/admin/promocodes/",
	"/api/admin/promocodes/bulk-deactivate",
	"/api/admin/site-content",
	"/api/admin/site-content/flush",
	"/api/admin/site-media",
	"/api/admin/stats",
	"/api/admin/status",
	"/api/admin/summary-prompts",
	"/api/admin/summary-prompts/",
	"/api/admin/tariff-groups",
	"/api/admin/tariff-groups/",
	"/api/admin/tariffs",
	"/api/admin/tariffs/",
	"/api/admin/users",
	"/api/admin/users/",
	"/api/argument-clinic/vote",
	"/api/auth/forgot-password",
	"/api/auth/login",
	"/api/auth/logout",
	"/api/auth/me",
	"/api/auth/oauth/",
	"/api/auth/oauth/providers",
	"/api/auth/register",
	"/api/auth/reset-password",
	"/api/auth/verify-email",
	"/api/bootstrap/admin",
	"/api/billing/autorenew/disable",
	"/api/chat/complete",
	"/api/chat/attachments",
	"/api/chat/history",
	"/api/chat/select-mode",
	"/api/chat/send",
	"/api/chat/start",
	"/api/cookie-consent",
	"/api/expert/status",
	"/api/notifications/inbox",
	"/api/notifications/max/start-link",
	"/api/notifications/max/unlink",
	"/api/notifications/preferences",
	"/api/notifications/telegram/start-link",
	"/api/notifications/telegram/unlink",
	"/api/payments/yookassa/autorenew/disable",
	"/api/payments/yookassa/create",
	"/api/profile",
	"/api/profile/settings",
	"/api/profile/export",
	"/api/privacy/consents",
	"/api/privacy/consents/withdraw",
	"/api/promo-admin/status",
	"/api/promo-admin/summarize",
	"/api/promo/validate",
	"/api/public/analytics",
	"/api/public/demo-modes",
	"/api/public/site-content",
	"/api/public/site-media/",
	"/api/public/tariffs",
	"/api/tester/run-check",
	"/api/tester/status",
	"/api/tester/user-chat-preview",
	"/api/tester/users",
	"/api/tester/users/",
	"/db-check",
	"/health",
	"/health/ai-errors",
	"/health/deep",
	"/health/frontend-errors",
	"/metrics",
	"/webhooks/max/",
	"/webhooks/yookassa/",
}

func TestEnterpriseContract_RouterRoutesMatchInventory(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile("router.go")
	if err != nil {
		t.Fatalf("read router.go: %v", err)
	}
	re := regexp.MustCompile(`mux\.HandleFunc\("([^"]+)"`)
	matches := re.FindAllStringSubmatch(string(raw), -1)
	if len(matches) == 0 {
		t.Fatalf("no mux.HandleFunc routes found in router.go")
	}

	actualSet := map[string]bool{}
	for _, match := range matches {
		actualSet[match[1]] = true
	}
	expectedSet := map[string]bool{}
	for _, path := range enterpriseRouteInventory {
		expectedSet[path] = true
	}

	var missing, stale []string
	for path := range actualSet {
		if !expectedSet[path] {
			missing = append(missing, path)
		}
	}
	for path := range expectedSet {
		if !actualSet[path] {
			stale = append(stale, path)
		}
	}
	sort.Strings(missing)
	sort.Strings(stale)

	if len(missing) > 0 || len(stale) > 0 {
		t.Fatalf("router inventory drift: missing=%v stale=%v", missing, stale)
	}
}

func TestEnterpriseContract_PublicRoutes_StatusAndPayloadShape(t *testing.T) {
	t.Parallel()

	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)

	cases := []struct {
		name       string
		method     string
		path       string
		body       []byte
		wantStatus int
		required   []string
		arrays     []string
	}{
		{
			name:       "health",
			method:     http.MethodGet,
			path:       "/health",
			wantStatus: http.StatusOK,
			required:   []string{"ok", "service"},
		},
		{
			name:       "db_check",
			method:     http.MethodGet,
			path:       "/db-check",
			wantStatus: http.StatusOK,
			required:   []string{"ok"},
		},
		{
			name:       "public_demo_modes",
			method:     http.MethodGet,
			path:       "/api/public/demo-modes",
			wantStatus: http.StatusOK,
			required:   []string{"ok", "modes"},
			arrays:     []string{"modes"},
		},
		{
			name:       "public_tariffs",
			method:     http.MethodGet,
			path:       "/api/public/tariffs",
			wantStatus: http.StatusOK,
			required:   []string{"ok", "tariffs"},
			arrays:     []string{"tariffs"},
		},
		{
			name:       "oauth_providers",
			method:     http.MethodGet,
			path:       "/api/auth/oauth/providers",
			wantStatus: http.StatusOK,
			required:   []string{"ok", "providers"},
		},
		{
			name:       "auth_me_requires_auth",
			method:     http.MethodGet,
			path:       "/api/auth/me",
			wantStatus: http.StatusUnauthorized,
			required:   []string{"ok", "error"},
		},
		{
			name:       "access_status_guest",
			method:     http.MethodGet,
			path:       "/api/access/status",
			wantStatus: http.StatusOK,
			required:   []string{"ok", "userId", "hasAccess", "activeModes"},
			arrays:     []string{"activeModes"},
		},
		{
			name:       "cookie_consent",
			method:     http.MethodPost,
			path:       "/api/cookie-consent",
			body:       []byte(`{"consentId":"enterprise-contract","sourcePath":"/"}`),
			wantStatus: http.StatusOK,
			required:   []string{"ok"},
		},
		{
			name:       "login_method_not_allowed",
			method:     http.MethodGet,
			path:       "/api/auth/login",
			wantStatus: http.StatusMethodNotAllowed,
			required:   []string{"ok", "error"},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			status, payload, raw := enterpriseJSONRequest(t, ts, tc.method, tc.path, tc.body)
			if status != tc.wantStatus {
				t.Fatalf("%s %s status=%d body=%s, want %d", tc.method, tc.path, status, raw, tc.wantStatus)
			}
			for _, key := range tc.required {
				if _, ok := payload[key]; !ok {
					t.Fatalf("%s %s missing key %q in body=%s", tc.method, tc.path, key, raw)
				}
			}
			for _, key := range tc.arrays {
				if _, ok := payload[key].([]any); !ok {
					t.Fatalf("%s %s key %q is %T, want array; body=%s", tc.method, tc.path, key, payload[key], raw)
				}
			}
		})
	}
}

func TestEnterpriseContract_ProtectedRoutes_UnauthorizedPayloadShape(t *testing.T) {
	t.Parallel()

	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)

	cases := []struct {
		name   string
		method string
		path   string
		body   []byte
	}{
		{name: "profile", method: http.MethodGet, path: "/api/profile"},
		{name: "profile_settings", method: http.MethodPatch, path: "/api/profile/settings", body: []byte(`{"allowMessageAnonymization":true}`)},
		{name: "profile_export", method: http.MethodGet, path: "/api/profile/export"},
		{name: "privacy_consents", method: http.MethodGet, path: "/api/privacy/consents"},
		{name: "admin_data_protection", method: http.MethodGet, path: "/api/admin/data-protection"},
		{name: "notification_preferences", method: http.MethodGet, path: "/api/notifications/preferences"},
		{name: "notification_inbox", method: http.MethodGet, path: "/api/notifications/inbox"},
		{name: "auth_me", method: http.MethodGet, path: "/api/auth/me"},
		{name: "admin_users", method: http.MethodGet, path: "/api/admin/users"},
		{name: "admin_modes", method: http.MethodGet, path: "/api/admin/modes"},
		{name: "tester_status", method: http.MethodGet, path: "/api/tester/status"},
		{name: "tester_run_check", method: http.MethodPost, path: "/api/tester/run-check", body: []byte(`{}`)},
		{name: "expert_status", method: http.MethodGet, path: "/api/expert/status"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			status, payload, raw := enterpriseJSONRequest(t, ts, tc.method, tc.path, tc.body)
			if status != http.StatusUnauthorized {
				t.Fatalf("%s %s status=%d body=%s, want 401", tc.method, tc.path, status, raw)
			}
			if ok, _ := payload["ok"].(bool); ok {
				t.Fatalf("%s %s ok=true for unauthorized response: %s", tc.method, tc.path, raw)
			}
			if _, ok := payload["error"].(string); !ok {
				t.Fatalf("%s %s missing string error in unauthorized response: %s", tc.method, tc.path, raw)
			}
			if _, hasDebug := payload["debug"]; hasDebug {
				t.Fatalf("%s %s leaked debug in unauthorized response: %s", tc.method, tc.path, raw)
			}
		})
	}
}

func enterpriseJSONRequest(t *testing.T, ts *TestServer, method, path string, body []byte) (int, map[string]any, string) {
	t.Helper()

	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, ts.URL(path), reader)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := ts.Client.Do(req)
	if err != nil {
		t.Fatalf("do %s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	rawBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	raw := strings.TrimSpace(string(rawBytes))

	var payload map[string]any
	if err := json.Unmarshal(rawBytes, &payload); err != nil {
		t.Fatalf("%s %s returned non-json status=%d body=%s: %v", method, path, resp.StatusCode, raw, err)
	}
	return resp.StatusCode, payload, raw
}
