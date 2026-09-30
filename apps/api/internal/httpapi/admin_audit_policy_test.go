package httpapi

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestAdminAuditRedactsSensitiveMeta(t *testing.T) {
	t.Parallel()
	meta := sanitizeAuditJSONMap(map[string]any{
		"email":                   "admin@example.test",
		"token":                   "raw-token",
		"cookieValue":             "session-cookie",
		"yookassaPaymentMethodID": "pm-123",
		"nested": map[string]any{
			"api_key": "secret-key",
			"count":   3,
		},
		"items": []any{map[string]any{"password": "raw-password"}},
	})

	if meta["email"] != "admin@example.test" {
		t.Fatalf("safe field redacted: %#v", meta)
	}
	for _, key := range []string{"token", "cookieValue", "yookassaPaymentMethodID"} {
		if meta[key] != "[redacted]" {
			t.Fatalf("%s = %#v, want redacted", key, meta[key])
		}
	}
	nested := meta["nested"].(map[string]any)
	if nested["api_key"] != "[redacted]" || nested["count"] != 3 {
		t.Fatalf("nested redaction broken: %#v", nested)
	}
	items := meta["items"].([]any)
	first := items[0].(map[string]any)
	if first["password"] != "[redacted]" {
		t.Fatalf("slice redaction broken: %#v", first)
	}
}

func TestAdminAuditGuard_AllWrittenActionsClassified(t *testing.T) {
	t.Parallel()
	files, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read httpapi dir: %v", err)
	}
	actionRe := regexp.MustCompile(`writeAdminAudit\([^\n]*"(admin\.[^"]+)"`)
	for _, file := range files {
		name := file.Name()
		if file.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		raw, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		for _, match := range actionRe.FindAllStringSubmatch(string(raw), -1) {
			action := match[1]
			class, ok := adminAuditActionClassifications[action]
			if !ok {
				t.Fatalf("%s uses unclassified audit action %q", name, action)
			}
			if class.Section == "" {
				t.Fatalf("%s action %q has empty audit section", name, action)
			}
		}
	}
}

func TestAdminAuditGuard_AllAdminRoutesClassified(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("router.go")
	if err != nil {
		t.Fatalf("read router.go: %v", err)
	}
	routeRe := regexp.MustCompile(`mux\.HandleFunc\("(\/api\/admin[^"]*)"`)
	for _, match := range routeRe.FindAllStringSubmatch(string(raw), -1) {
		path := match[1]
		class, ok := adminAuditRouteClassifications[path]
		if !ok {
			t.Fatalf("admin route %q has no audit classification", path)
		}
		if class.Section == "" {
			t.Fatalf("admin route %q has empty audit section", path)
		}
	}
}
