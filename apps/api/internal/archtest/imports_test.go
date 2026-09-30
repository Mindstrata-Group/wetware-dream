package archtest

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// goModulePath reads the module path of apps/api from go.mod.
func goModulePath(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(apiRoot, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`(?m)^module\s+(\S+)`).FindStringSubmatch(string(raw))
	if m == nil {
		t.Fatal("no module line in go.mod")
	}
	return m[1]
}

// importRuleViolation checks one import edge. from and to are package paths
// relative to apps/api ("internal/modules/games", "internal/kernel/httpjson").
// It returns "" when the edge is allowed.
//
// Rules (ADR-0001):
//  1. kernel imports neither modules nor the legacy httpapi package;
//  2. a module does not import the legacy httpapi package;
//  3. a module imports another module only through that module's root
//     package (its public API), never its subpackages.
func importRuleViolation(from, to string) string {
	fromKernel := from == "internal/kernel" || strings.HasPrefix(from, "internal/kernel/")
	fromMod, fromIsMod := moduleOfPackage(from)
	toMod, toIsMod := moduleOfPackage(to)
	toLegacy := to == "internal/httpapi" || strings.HasPrefix(to, "internal/httpapi/")

	switch {
	case fromKernel && toIsMod:
		return "kernel must not depend on modules"
	case fromKernel && toLegacy:
		return "kernel must not depend on the legacy httpapi package"
	case fromIsMod && toLegacy:
		return "modules must not depend on the legacy httpapi package (legacy may depend on modules, not the other way)"
	case fromIsMod && toIsMod && fromMod != toMod && to != "internal/modules/"+toMod:
		return fmt.Sprintf("module %s may use module %s only through its root package internal/modules/%s", fromMod, toMod, toMod)
	}
	return ""
}

func moduleOfPackage(pkg string) (string, bool) {
	if !strings.HasPrefix(pkg, "internal/modules/") {
		return "", false
	}
	rest := strings.TrimPrefix(pkg, "internal/modules/")
	if rest == "" {
		return "", false
	}
	return strings.SplitN(rest, "/", 2)[0], true
}

func TestImportRules_Repository(t *testing.T) {
	t.Parallel()
	mod := goModulePath(t) + "/"
	var bad []string
	for _, f := range scanSources(t) {
		from := filepath.ToSlash(filepath.Dir(f.rel))
		for _, imp := range f.imports {
			if !strings.HasPrefix(imp, mod) {
				continue
			}
			to := strings.TrimPrefix(imp, mod)
			if why := importRuleViolation(from, to); why != "" {
				bad = append(bad, fmt.Sprintf("%s imports %s: %s", f.rel, to, why))
			}
		}
	}
	sort.Strings(bad)
	if len(bad) > 0 {
		t.Fatalf("import rule violations:\n  %s", strings.Join(bad, "\n  "))
	}
}

func TestImportRules_Table(t *testing.T) {
	t.Parallel()
	cases := []struct {
		from, to string
		allowed  bool
	}{
		{"internal/httpapi", "internal/modules/games", true},
		{"internal/httpapi", "internal/kernel/httpjson", true},
		{"internal/modules/games", "internal/kernel/httpjson", true},
		{"internal/modules/games/internal/scoring", "internal/modules/games", true},
		{"internal/modules/games", "internal/modules/auth", true},
		{"internal/modules/games", "internal/modules/auth/store", false},
		{"internal/modules/games", "internal/httpapi", false},
		{"internal/kernel/httpjson", "internal/modules/games", false},
		{"internal/kernel", "internal/httpapi", false},
		{"cmd/api", "internal/modules/games", true},
	}
	for _, tc := range cases {
		t.Run(tc.from+"->"+tc.to, func(t *testing.T) {
			t.Parallel()
			why := importRuleViolation(tc.from, tc.to)
			if (why == "") != tc.allowed {
				t.Fatalf("importRuleViolation(%q, %q) = %q, want allowed=%v", tc.from, tc.to, why, tc.allowed)
			}
		})
	}
}
