//go:build integration

package httpapi

import (
	"os"
	"regexp"
	"sort"
	"testing"
)

var privilegedRouteInventory = []string{
	"/api/admin/access",
	"/api/admin/ai-gateways",
	"/api/admin/ai-gateways/",
	"/api/admin/ai-models",
	"/api/admin/ai-settings",
	"/api/admin/analytics-settings",
	"/api/admin/broadcasts",
	"/api/admin/dialog-summary-prompt",
	"/api/admin/lead-summary-prompt",
	"/api/admin/dialogs",
	"/api/admin/dialogs/",
	"/api/admin/exports/messages",
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
	"/api/expert/status",
	"/api/tester/run-check",
	"/api/tester/status",
	"/api/tester/user-chat-preview",
	"/api/tester/users",
	"/api/tester/users/",
}

func TestContract_PrivilegedRoutes_AreClassifiedInInventory(t *testing.T) {
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
		path := match[1]
		if isPrivilegedRoute(path) {
			actualSet[path] = true
		}
	}
	expectedSet := map[string]bool{}
	for _, path := range privilegedRouteInventory {
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
		t.Fatalf("privileged route inventory drift: missing=%v stale=%v", missing, stale)
	}
}

func isPrivilegedRoute(path string) bool {
	return len(path) >= len("/api/admin/") && path[:len("/api/admin/")] == "/api/admin/" ||
		len(path) >= len("/api/tester/") && path[:len("/api/tester/")] == "/api/tester/" ||
		len(path) >= len("/api/expert/") && path[:len("/api/expert/")] == "/api/expert/"
}
