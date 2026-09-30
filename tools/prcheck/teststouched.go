// Command prcheck holds the repository gates that CI runs on every change:
//
//   - tests-touched:  logic changed but no test changed -> red, unless a
//     maintainer adds the no-tests-needed label;
//   - runner-safety:  a self-hosted runner never executes code in the main
//     repository (pull requests from strangers land there), only in a
//     contributor's fork and only after the fork owner opts in;
//   - runner-compose: the contributor runner is ephemeral, resource-limited
//     and has no access to the host Docker socket;
//   - pinned-actions: every third-party action is pinned to a commit SHA;
//   - hygiene:        no public IP literals and no real e-mail addresses;
//   - comments:       code comments are written in English.
//
// Every rule is a pure function over data, so it is covered by table tests
// without git, Docker or network.
package main

import (
	"fmt"
	"path"
	"sort"
	"strings"
)

// NoTestsLabel lets a maintainer waive the test requirement explicitly
// (for example, a copy change or configuration without logic).
const NoTestsLabel = "no-tests-needed"

// isTestFile reports whether a change to p counts as a change to tests.
func isTestFile(p string) bool {
	base := path.Base(p)
	switch {
	case strings.HasSuffix(base, "_test.go"):
		return true
	case strings.Contains(base, ".test.") || strings.Contains(base, ".spec."):
		return true
	case strings.HasPrefix(p, "apps/web/e2e/"):
		return true
	case strings.HasPrefix(p, "apps/api/internal/testsupport/"):
		return true
	}
	return false
}

// isCodeFile reports whether p contains logic whose change needs a test.
func isCodeFile(p string) bool {
	if isTestFile(p) {
		return false
	}
	switch {
	case strings.HasPrefix(p, "apps/api/") && strings.HasSuffix(p, ".go"):
		return true
	case strings.HasPrefix(p, "tools/") && strings.HasSuffix(p, ".go"):
		return true
	case strings.HasPrefix(p, "apps/web/src/"):
		switch path.Ext(p) {
		case ".ts", ".tsx", ".js", ".jsx", ".mjs":
			return true
		}
	}
	return false
}

// TestsVerdict is the outcome of the tests-touched gate.
type TestsVerdict struct {
	OK     bool
	Reason string
	Code   []string // changed files with logic
}

// DecideTestsTouched decides whether a change set may pass.
func DecideTestsTouched(changed []string, labels []string) TestsVerdict {
	var code []string
	tests := 0
	for _, raw := range changed {
		p := strings.TrimSpace(strings.ReplaceAll(raw, "\\", "/"))
		if p == "" {
			continue
		}
		if isTestFile(p) {
			tests++
		} else if isCodeFile(p) {
			code = append(code, p)
		}
	}
	sort.Strings(code)
	switch {
	case len(code) == 0:
		return TestsVerdict{OK: true, Reason: "no logic changed"}
	case tests > 0:
		return TestsVerdict{OK: true, Reason: fmt.Sprintf("%d file(s) with logic, %d test file(s) changed", len(code), tests), Code: code}
	}
	for _, l := range labels {
		if strings.EqualFold(strings.TrimSpace(l), NoTestsLabel) {
			return TestsVerdict{OK: true, Reason: "waived by maintainer label " + NoTestsLabel, Code: code}
		}
	}
	return TestsVerdict{
		OK: false,
		Reason: fmt.Sprintf("logic changed in %d file(s) but no test changed. Add a test for the new behaviour, "+
			"or ask a maintainer to add the %s label if a test really makes no sense here", len(code), NoTestsLabel),
		Code: code,
	}
}
