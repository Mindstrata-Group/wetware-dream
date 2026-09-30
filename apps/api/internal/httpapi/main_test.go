//go:build integration

package httpapi

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"mindstrata-stage1/api/internal/testsupport"
)

// TestMain bootstraps the integration test suite.
//
// After the caches were refactored to per-Handler isolation, tests run safely in
// parallel. Each test creates its own NewTestServer with a separate
// newHandlerCaches(): there is no global state.
//
// DB strategy: InitTemplate loads the schema into a template once (~30s), and
// each test clones it through testsupport.NewEnv (~0.3s). truncateAll (the
// shared env path) is NOT used: parallel DELETEs from different tests conflict
// on locks. Instead each test gets an isolated DB that is dropped in Cleanup.
func TestMain(m *testing.M) {
	// Lower the PBKDF2 cost for the integration run.
	// The production binary uses the defaults from auth_security.go.
	passwordPBKDF2Iter = 1000
	passwordPBKDF2IterMin = 1000

	baseDSN := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	if baseDSN == "" {
		// No DB configured: a unit-only run; integration tests skip themselves.
		os.Exit(m.Run())
	}

	// Load the schema into the template DB once (~30s).
	tmplCleanup, err := testsupport.InitTemplate(baseDSN)
	if err != nil {
		fmt.Fprintf(os.Stderr, "testsupport: InitTemplate: %v (falling back to slow path)\n", err)
		tmplCleanup = func() {}
	}

	code := m.Run()

	tmplCleanup()
	os.Exit(code)
}
