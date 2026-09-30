package httpapi

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

func TestOpenAPIContract_IsValidAndMatchesRouter(t *testing.T) {
	t.Parallel()

	routerRoutes := readRouterRoutes(t)
	spec := readOpenAPISpec(t)

	if got := stringField(t, spec, "openapi"); got == "" || !strings.HasPrefix(got, "3.") {
		t.Fatalf("openapi=%q, want OpenAPI 3.x", got)
	}
	info := objectField(t, spec, "info")
	if stringField(t, info, "title") == "" || stringField(t, info, "version") == "" {
		t.Fatalf("info.title and info.version are required")
	}
	components := objectField(t, spec, "components")
	securitySchemes := objectField(t, components, "securitySchemes")
	if _, ok := securitySchemes["cookieAuth"]; !ok {
		t.Fatalf("components.securitySchemes.cookieAuth is required")
	}
	validateLocalRefs(t, spec)

	paths := objectField(t, spec, "paths")
	documentedRoutes := map[string]string{}
	operationIDs := map[string]string{}
	for path, rawPathItem := range paths {
		pathItem, ok := rawPathItem.(map[string]any)
		if !ok {
			t.Fatalf("paths.%s must be an object", path)
		}
		routerPath := stringField(t, pathItem, "x-router-path")
		if routerPath == "" {
			t.Fatalf("paths.%s is missing x-router-path", path)
		}
		if previousPath, exists := documentedRoutes[routerPath]; exists {
			t.Fatalf("x-router-path %q is duplicated in %s and %s", routerPath, previousPath, path)
		}
		documentedRoutes[routerPath] = path

		hasOperation := false
		for method, rawOperation := range pathItem {
			if !isOpenAPIMethod(method) {
				continue
			}
			hasOperation = true
			operation, ok := rawOperation.(map[string]any)
			if !ok {
				t.Fatalf("%s %s must be an object", strings.ToUpper(method), path)
			}
			operationID := stringField(t, operation, "operationId")
			if operationID == "" {
				t.Fatalf("%s %s is missing operationId", strings.ToUpper(method), path)
			}
			if previousPath, exists := operationIDs[operationID]; exists {
				t.Fatalf("operationId %q is duplicated in %s and %s", operationID, previousPath, path)
			}
			operationIDs[operationID] = path
			if len(arrayField(t, operation, "tags")) == 0 {
				t.Fatalf("%s %s is missing tags", strings.ToUpper(method), path)
			}
			responses := objectField(t, operation, "responses")
			if len(responses) == 0 {
				t.Fatalf("%s %s is missing responses", strings.ToUpper(method), path)
			}
			if !hasSuccessOrRedirectResponse(responses) {
				t.Fatalf("%s %s must document at least one 2xx or 3xx response", strings.ToUpper(method), path)
			}
		}
		if !hasOperation {
			t.Fatalf("paths.%s must document at least one HTTP operation", path)
		}
	}

	missing, stale := diffRoutes(routerRoutes, documentedRoutes)
	if len(missing) > 0 || len(stale) > 0 {
		t.Fatalf("OpenAPI/router drift: missing=%v stale=%v", missing, stale)
	}
}

func TestOpenAPIContract_OperationalSemantics(t *testing.T) {
	t.Parallel()

	spec := readOpenAPISpec(t)
	paths := objectField(t, spec, "paths")

	for path, rawPathItem := range paths {
		pathItem, ok := rawPathItem.(map[string]any)
		if !ok {
			t.Fatalf("paths.%s must be an object", path)
		}
		for method, rawOperation := range pathItem {
			if !isOpenAPIMethod(method) {
				continue
			}
			operation, ok := rawOperation.(map[string]any)
			if !ok {
				t.Fatalf("%s %s must be an object", strings.ToUpper(method), path)
			}
			if operationNeedsRequestBody(path, method) {
				if _, ok := operation["requestBody"]; !ok {
					t.Fatalf("%s %s is a mutation and must document requestBody", strings.ToUpper(method), path)
				}
			}
			if operationRequiresCookieAuth(path, method) && !operationHasCookieAuth(operation) {
				t.Fatalf("%s %s is protected and must require cookieAuth", strings.ToUpper(method), path)
			}
			if operationAllowsAnonymous(path, method) && operationHasCookieAuth(operation) && !operationHasAnonymousAlternative(operation) {
				t.Fatalf("%s %s allows anonymous/guest users and must include an explicit empty security alternative", strings.ToUpper(method), path)
			}
		}
	}
}

func TestOpenAPIContract_P0ResponsesAreConcrete(t *testing.T) {
	t.Parallel()

	spec := readOpenAPISpec(t)
	paths := objectField(t, spec, "paths")
	p0 := map[string]struct{}{
		"GET /api/auth/me":                   {},
		"GET /api/profile":                   {},
		"GET /api/access/status":             {},
		"POST /api/chat/start":               {},
		"POST /api/chat/send":                {},
		"POST /api/payments/yookassa/create": {},
		"GET /api/public/demo-modes":         {},
	}
	for path, rawPathItem := range paths {
		pathItem, ok := rawPathItem.(map[string]any)
		if !ok {
			t.Fatalf("paths.%s must be an object", path)
		}
		for method, rawOperation := range pathItem {
			if !isOpenAPIMethod(method) {
				continue
			}
			key := strings.ToUpper(method) + " " + path
			if _, ok := p0[key]; !ok {
				continue
			}
			operation, ok := rawOperation.(map[string]any)
			if !ok {
				t.Fatalf("%s must be an object", key)
			}
			response := successResponseRef(t, operation)
			if response == "#/components/responses/JSONOK" {
				t.Fatalf("%s must not use generic JSONOK response", key)
			}
			if response == "#/components/responses/List" {
				t.Fatalf("%s must not use generic List response", key)
			}
			if response == "" {
				t.Fatalf("%s must document a concrete success response $ref", key)
			}
		}
	}
}

func readRouterRoutes(t *testing.T) map[string]struct{} {
	t.Helper()

	raw, err := os.ReadFile("router.go")
	if err != nil {
		t.Fatalf("read router.go: %v", err)
	}
	re := regexp.MustCompile(`mux\.HandleFunc\("([^"]+)"`)
	matches := re.FindAllStringSubmatch(string(raw), -1)
	if len(matches) == 0 {
		t.Fatalf("no mux.HandleFunc routes found in router.go")
	}
	routes := make(map[string]struct{}, len(matches))
	for _, match := range matches {
		routes[match[1]] = struct{}{}
	}
	return routes
}

func readOpenAPISpec(t *testing.T) map[string]any {
	t.Helper()

	raw, err := os.ReadFile("../../../../docs/openapi/openapi.json")
	if err != nil {
		t.Fatalf("read docs/openapi/openapi.json: %v", err)
	}
	var spec map[string]any
	if err := json.Unmarshal(raw, &spec); err != nil {
		t.Fatalf("parse docs/openapi/openapi.json: %v", err)
	}
	return spec
}

func objectField(t *testing.T, obj map[string]any, key string) map[string]any {
	t.Helper()

	value, ok := obj[key].(map[string]any)
	if !ok || value == nil {
		t.Fatalf("%s must be an object", key)
	}
	return value
}

func stringField(t *testing.T, obj map[string]any, key string) string {
	t.Helper()

	value, _ := obj[key].(string)
	return value
}

func arrayField(t *testing.T, obj map[string]any, key string) []any {
	t.Helper()

	value, _ := obj[key].([]any)
	return value
}

func validateLocalRefs(t *testing.T, spec map[string]any) {
	t.Helper()

	var walk func(path string, value any)
	walk = func(path string, value any) {
		switch typed := value.(type) {
		case map[string]any:
			if ref, ok := typed["$ref"].(string); ok {
				if !strings.HasPrefix(ref, "#/") {
					t.Fatalf("%s has non-local $ref %q", path, ref)
				}
				if !localRefExists(spec, ref) {
					t.Fatalf("%s has unresolved $ref %q", path, ref)
				}
			}
			for key, child := range typed {
				walk(path+"/"+key, child)
			}
		case []any:
			for i, child := range typed {
				walk(fmt.Sprintf("%s/%d", path, i), child)
			}
		}
	}
	walk("#", spec)
}

func localRefExists(root map[string]any, ref string) bool {
	var current any = root
	for _, part := range strings.Split(strings.TrimPrefix(ref, "#/"), "/") {
		part = strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
		object, ok := current.(map[string]any)
		if !ok {
			return false
		}
		current, ok = object[part]
		if !ok {
			return false
		}
	}
	return true
}

func isOpenAPIMethod(method string) bool {
	switch method {
	case "get", "put", "post", "delete", "options", "head", "patch", "trace":
		return true
	default:
		return false
	}
}

func hasSuccessOrRedirectResponse(responses map[string]any) bool {
	for code := range responses {
		if len(code) == 3 && (code[0] == '2' || code[0] == '3') {
			return true
		}
	}
	return false
}

func operationNeedsRequestBody(path, method string) bool {
	switch strings.ToLower(method) {
	case "post", "put", "patch":
	default:
		return false
	}
	noBody := map[string]struct{}{
		"POST /api/auth/logout":                              {},
		"POST /api/billing/autorenew/disable":                {},
		"POST /api/notifications/max/unlink":                 {},
		"POST /api/notifications/telegram/unlink":            {},
		"POST /api/chat/start":                               {},
		"POST /api/admin/site-content/flush":                 {},
		"POST /api/admin/payments/yookassa/renewals/run":     {},
		"POST /api/admin/payments/yookassa/run-due-renewals": {},
	}
	_, skip := noBody[strings.ToUpper(method)+" "+path]
	return !skip
}

func operationRequiresCookieAuth(path, method string) bool {
	if operationAllowsAnonymous(path, method) {
		return false
	}
	if strings.HasPrefix(path, "/api/admin/") || strings.HasPrefix(path, "/api/tester/") || strings.HasPrefix(path, "/api/expert/") {
		return true
	}
	switch path {
	case "/api/auth/me", "/api/auth/logout", "/api/profile", "/api/profile/settings", "/api/profile/export",
		"/api/billing/autorenew/disable", "/api/notifications/preferences", "/api/notifications/inbox",
		"/api/notifications/max/start-link":
		return true
	default:
		return false
	}
}

func operationAllowsAnonymous(path, method string) bool {
	anonymous := map[string]struct{}{
		"GET /health":                                 {},
		"GET /health/deep":                            {},
		"GET /health/frontend-errors":                 {},
		"GET /health/ai-errors":                       {},
		"GET /db-check":                               {},
		"GET /metrics":                                {},
		"POST /api/_error":                            {},
		"GET /api/public/demo-modes":                  {},
		"GET /api/public/site-content":                {},
		"GET /api/public/site-media/{id}/{filename}":  {},
		"HEAD /api/public/site-media/{id}/{filename}": {},
		"GET /api/public/tariffs":                     {},
		"POST /api/cookie-consent":                    {},
		"GET /api/argument-clinic/vote":               {},
		"POST /api/argument-clinic/vote":              {},
		"POST /api/auth/login":                        {},
		"POST /api/auth/forgot-password":              {},
		"POST /api/auth/reset-password":               {},
		"POST /api/auth/register":                     {},
		"GET /api/auth/verify-email":                  {},
		"POST /api/auth/verify-email":                 {},
		"GET /api/auth/oauth/providers":               {},
		"GET /api/auth/oauth/{provider}/{action}":     {},
		"POST /api/bootstrap/admin":                   {},
		"GET /api/access/status":                      {},
		"POST /api/access/promocode/apply":            {},
		"POST /api/promo/validate":                    {},
		"GET /api/promo-admin/status":                 {},
		"POST /api/promo-admin/summarize":             {},
		"POST /api/chat/start":                        {},
		"POST /api/chat/attachments":                  {},
		"POST /api/chat/select-mode":                  {},
		"POST /api/chat/send":                         {},
		"GET /api/chat/history":                       {},
		"POST /api/chat/complete":                     {},
		"POST /api/payments/yookassa/create":          {},
		"POST /webhooks/yookassa/{webhookPath}":       {},
		"POST /api/debug/client-log":                  {},
	}
	_, ok := anonymous[strings.ToUpper(method)+" "+path]
	return ok
}

func operationHasCookieAuth(operation map[string]any) bool {
	rawItems, _ := operation["security"].([]any)
	for _, rawSecurity := range rawItems {
		security, ok := rawSecurity.(map[string]any)
		if !ok {
			continue
		}
		if _, ok := security["cookieAuth"]; ok {
			return true
		}
	}
	return false
}

func operationHasAnonymousAlternative(operation map[string]any) bool {
	rawItems, _ := operation["security"].([]any)
	for _, rawSecurity := range rawItems {
		security, ok := rawSecurity.(map[string]any)
		if ok && len(security) == 0 {
			return true
		}
	}
	return false
}

func successResponseRef(t *testing.T, operation map[string]any) string {
	t.Helper()

	responses := objectField(t, operation, "responses")
	for _, status := range []string{"200", "201", "202", "204", "302"} {
		rawResponse, ok := responses[status]
		if !ok {
			continue
		}
		response, ok := rawResponse.(map[string]any)
		if !ok {
			t.Fatalf("responses.%s must be an object", status)
		}
		return stringField(t, response, "$ref")
	}
	return ""
}

func diffRoutes(routerRoutes map[string]struct{}, documentedRoutes map[string]string) ([]string, []string) {
	var missing, stale []string
	for route := range routerRoutes {
		if _, ok := documentedRoutes[route]; !ok {
			missing = append(missing, route)
		}
	}
	for route, path := range documentedRoutes {
		if _, ok := routerRoutes[route]; !ok {
			stale = append(stale, fmt.Sprintf("%s (paths.%s)", route, path))
		}
	}
	sort.Strings(missing)
	sort.Strings(stale)
	return missing, stale
}
