//go:build integration

package httpapi

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"mindstrata-stage1/api/internal/testsupport"
)

// TestMigrations_SchemaBaseLoadsOnEmptyDB:
// schema_base.sql (a snapshot of the prod schema with all migrations
// already applied) must apply to an empty DB without errors.
// testsupport.NewEnv already exercises this on every integration test,
// so a successful Env construction here doubles as the assertion.
//
// What this catches: drift between the committed schema_base.sql and
// what PostgreSQL accepts (broken syntax, missing extensions, etc).
func TestMigrations_SchemaBaseLoadsOnEmptyDB(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)

	// Sanity: at least the canonical tables must be queryable on this fresh DB.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	for _, tbl := range []string{"users", "users_dialogs", "dialogs_messages",
		"user_mode_access", "promocodes", "auth_sessions", "modes",
		"daily_message_counts", "admin_mode_usage_resets"} {
		var n int64
		if err := env.Pool.QueryRow(ctx, "select count(*) from "+tbl).Scan(&n); err != nil {
			t.Errorf("table %s not queryable after schema_base load: %v", tbl, err)
		}
	}
}

// TestMigrations_FilesParseable:
// every apps/api/sql/*.sql file must be named by UTC timestamp and contain SQL.
// This is a fast static check: timestamp naming plus non-empty SQL statements.
// Full execution is covered by deploy-server.sh via tools/migrate.go.
//
// We do NOT replay them all on top of schema_base — schema_base already
// reflects the cumulative effect of all migrations, so re-applying earlier
// migrations on a fully-migrated DB hits constraints added by LATER
// migrations (e.g. 20260531_215503 seed insert vs 20260531_215510's NOT NULL daily_message_limit).
//
// Replay correctness is verified manually via deploy-server.sh
// (INIT_DB_SCHEMA=true) once per environment.
func TestMigrations_FilesParseable(t *testing.T) {
	t.Parallel()
	_, thisFile, _, _ := runtime.Caller(0)
	migrationsDir := filepath.Join(filepath.Dir(thisFile), "..", "..", "sql")
	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		t.Fatalf("read sql dir: %v", err)
	}

	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".sql") || strings.HasPrefix(name, "_") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(migrationsDir, name))
		if err != nil {
			t.Errorf("read %s: %v", name, err)
			continue
		}
		text := string(raw)
		// Minimum viable check: file is non-empty and ends with semicolon / contains a statement.
		trimmed := strings.TrimSpace(text)
		if trimmed == "" {
			t.Errorf("%s is empty", name)
			continue
		}
		if !strings.Contains(trimmed, ";") {
			t.Errorf("%s contains no SQL statements (no semicolons)", name)
		}
	}
}

// TestMigrations_LatestMigrationReflectedInSchema:
// schema_base.sql must contain artifacts from the latest migration.
// Catches stale schema_base — e.g., if someone adds a migration but
// forgets to re-dump schema_base from prod.
//
// We pick a known recent artifact: daily_message_counts table (added in 20260531_215515).
// Update this assertion when new migrations introduce new tables/columns.
func TestMigrations_LatestMigrationReflectedInSchema(t *testing.T) {
	t.Parallel()
	_, thisFile, _, _ := runtime.Caller(0)
	// schema_base.sql moved 2026-05-29 to testsupport/ for //go:embed support.
	schemaPath := filepath.Join(filepath.Dir(thisFile), "..", "testsupport", "schema_base.sql")
	raw, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatalf("read schema_base.sql: %v", err)
	}
	schema := string(raw)
	for _, marker := range []string{
		"daily_message_counts",        // 20260531_215515
		"admin_mode_usage_resets",     // 20260531_215512
		"promocode_targets",           // 20260531_215509
		"cookie_consents",             // 20260531_215516
		"site_content",                // 20260531_215517
		"yookassa_webhook_events",     // 20260531_215518
		"idx_message_usage_access_id", // 20260531_215524
		"notification_templates",      // 20260605_130000
		"site_media",                  // 20260605_130001
		"normalize_modes_markdown",    // 20260612_120000
		"argument_clinic_votes",       // 20260616_120000
		"email text",                  // 20260616_130000
		"dialog_message_access_usage", // 20260618_143900
		"orchestration_decision_logs", // 20260624_020000
		"game_tasks_kind_valid",       // 20260924_120000
		"client_log jsonb",            // 20260924_150000
		"consent_records",             // 20260930_120000
		"processing_country",          // 20260930_120000
		"pii_vault_user_id_fkey",      // 20260930_130000
	} {
		if !strings.Contains(schema, marker) {
			t.Errorf("schema_base.sql does not contain %q — it's stale; re-dump from prod", marker)
		}
	}
}

// TestMigrations_FileNamesSorted: migration filenames must sort in apply order.
// This catches non-padded timestamp prefixes that would not sort in chronological order.
func TestMigrations_FileNamesSorted(t *testing.T) {
	t.Parallel()
	_, thisFile, _, _ := runtime.Caller(0)
	migrationsDir := filepath.Join(filepath.Dir(thisFile), "..", "..", "sql")
	entries, _ := os.ReadDir(migrationsDir)
	var names []string
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() || !strings.HasSuffix(n, ".sql") || strings.HasPrefix(n, "_") {
			continue
		}
		names = append(names, n)
	}
	sorted := append([]string{}, names...)
	sort.Strings(sorted)
	// UTC timestamp prefix expected: YYYYMMDD_HHMMSS_description.sql.
	// Fixed-width timestamps keep lexicographic order identical to apply order.
	previous := ""
	for _, n := range sorted {
		if len(n) < 16 || n[8] != '_' || n[15] != '_' {
			t.Errorf("migration %q does not follow YYYYMMDD_HHMMSS_*.sql pattern (sort risk)", n)
			continue
		}
		prefix := n[:15]
		if previous != "" && prefix <= previous {
			t.Errorf("migration %q timestamp prefix must be unique and increasing", n)
		}
		previous = prefix
	}
}

// requireOptionalPGConnection lets future tests open a fresh DB connection
// if they need to bypass the pool. Currently unused; retained for the
// replay-on-fresh-DB suite when handoff_db_sample is added to the repo.
func requireOptionalPGConnection(t *testing.T, dsn string) *pgx.Conn {
	t.Helper()
	conn, err := pgx.Connect(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	return conn
}
