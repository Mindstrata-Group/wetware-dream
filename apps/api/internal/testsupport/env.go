//go:build integration

package testsupport

import (
	"context"
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const baseDSNEnvVar = "TEST_DATABASE_URL"

// templateDBName is set by InitTemplate so NewSharedEnv can clone it instead
// of replaying schema_base.sql (~3 s → ~0.3 s per DB creation).
var templateDBName string

// sharedEnvInstance is set by UseSharedEnv.  When non-nil, NewEnv returns it
// and registers a per-test Cleanup that truncates all tables.
var sharedEnvInstance *Env

// cachedTableNames memoises the SELECT FROM pg_tables result that truncateAll
// needs. The schema is loaded once into the template DB and cloned into the
// shared DB before tests run; the table set never changes during a test run,
// so a single lookup per process is sufficient (saves one round-trip per test
// — ~466 cleanups × ~1 ms = ~0.5 s, plus query parsing overhead).
//
// We split the result into two sets:
//   - allTables: every public.<tablename> — for the DELETE batch
//   - idTables:  subset with an `id` column — for setval. Calling
//     pg_get_serial_sequence(table, 'id') ERRORS (not returns NULL) when the
//     column doesn't exist, so e.g. public.alembic_version (version_num only)
//     would fail the whole transaction. Filtering at lookup time keeps the
//     batched Exec atomic without per-statement error swallowing.
var (
	cachedAllTables    []string
	cachedIDTables     []string
	cachedTableLookup  sync.Once
	cachedTableLookupE error
)

// Env is a fully-initialized integration test environment.
type Env struct {
	Pool   *pgxpool.Pool
	DSN    string
	DBName string
}

// InitTemplate loads schema_base.sql into a dedicated template database so that
// NewSharedEnv can clone it quickly.  Call once from TestMain before m.Run();
// invoke the returned cleanup after m.Run() returns.
//
// If an error occurs the template is not set and NewSharedEnv falls back to
// replaying the SQL (slower but still correct).
func InitTemplate(baseDSN string) (cleanup func(), err error) {
	name := "mindstrata_tmpl_" + randomSuffix()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	adminConn, err := pgx.Connect(ctx, baseDSN)
	if err != nil {
		return func() {}, err
	}
	defer adminConn.Close(context.Background())

	if _, err = adminConn.Exec(ctx, "create database "+quoteIdent(name)); err != nil {
		return func() {}, err
	}

	targetDSN := replaceDatabase(baseDSN, name)
	loadConn, err := pgx.Connect(ctx, targetDSN)
	if err != nil {
		_ = dropDatabase(baseDSN, name)
		return func() {}, err
	}
	if _, err = loadConn.Exec(ctx, schemaBaseSQL); err != nil {
		_ = loadConn.Close(context.Background())
		_ = dropDatabase(baseDSN, name)
		return func() {}, err
	}
	_ = loadConn.Close(context.Background())

	if err = setDatabaseSearchPath(ctx, baseDSN, name); err != nil {
		_ = dropDatabase(baseDSN, name)
		return func() {}, err
	}

	templateDBName = name
	return func() {
		templateDBName = ""
		_ = dropDatabase(baseDSN, name)
	}, nil
}

// NewSharedEnv creates a single database for the entire test package.  Call
// from TestMain after InitTemplate; pass the result to UseSharedEnv.  The
// returned cleanup drops the database — call it in TestMain after m.Run().
func NewSharedEnv(baseDSN string) (env *Env, cleanup func(), err error) {
	dbName := "mindstrata_shared_" + randomSuffix()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	adminConn, err := pgx.Connect(ctx, baseDSN)
	if err != nil {
		return nil, func() {}, err
	}
	defer adminConn.Close(context.Background())

	if tmpl := templateDBName; tmpl != "" {
		if _, err = adminConn.Exec(ctx, "create database "+quoteIdent(dbName)+" template "+quoteIdent(tmpl)); err != nil {
			return nil, func() {}, err
		}
	} else {
		if _, err = adminConn.Exec(ctx, "create database "+quoteIdent(dbName)); err != nil {
			return nil, func() {}, err
		}
		tdDSN := replaceDatabase(baseDSN, dbName)
		loadConn, err2 := pgx.Connect(ctx, tdDSN)
		if err2 != nil {
			_ = dropDatabase(baseDSN, dbName)
			return nil, func() {}, err2
		}
		if _, err2 = loadConn.Exec(ctx, schemaBaseSQL); err2 != nil {
			_ = loadConn.Close(context.Background())
			_ = dropDatabase(baseDSN, dbName)
			return nil, func() {}, err2
		}
		_ = loadConn.Close(context.Background())
		if err2 = setDatabaseSearchPath(ctx, baseDSN, dbName); err2 != nil {
			_ = dropDatabase(baseDSN, dbName)
			return nil, func() {}, err2
		}
	}

	targetDSN := replaceDatabase(baseDSN, dbName)
	pool, err := pgxpool.New(ctx, targetDSN)
	if err != nil {
		_ = dropDatabase(baseDSN, dbName)
		return nil, func() {}, err
	}

	env = &Env{Pool: pool, DSN: targetDSN, DBName: dbName}
	return env, func() {
		pool.Close()
		_ = dropDatabase(baseDSN, dbName)
	}, nil
}

// UseSharedEnv registers env as the shared instance used by NewEnv.
// After this call, NewEnv returns env and registers a per-test Cleanup that
// truncates all public tables so each test starts with a clean slate.
func UseSharedEnv(env *Env) { sharedEnvInstance = env }

// NewEnv returns an Env for the current test.
//
// Fast path (shared): when UseSharedEnv has been called from TestMain, returns
// the shared Env and registers a Cleanup that deletes all rows between tests.
// One DB is created once; tests share it with a ~10 ms DELETE per test.
//
// Slow path (per-test): when no shared env is configured, creates an isolated
// database from the template (if InitTemplate was called) or from schema SQL.
// Suitable for running a single test in isolation outside of a full suite run.
func NewEnv(t testing.TB) *Env {
	t.Helper()

	if sharedEnvInstance != nil {
		t.Cleanup(func() {
			if err := truncateAll(sharedEnvInstance.Pool); err != nil {
				t.Logf("testsupport: truncate after %s: %v", t.Name(), err)
			}
		})
		return sharedEnvInstance
	}

	// --- Per-test path ---
	baseDSN := strings.TrimSpace(os.Getenv(baseDSNEnvVar))
	if baseDSN == "" {
		t.Skipf("set %s to run integration tests", baseDSNEnvVar)
	}

	dbName := "mindstrata_test_" + randomSuffix()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	adminConn, err := pgx.Connect(ctx, baseDSN)
	if err != nil {
		t.Fatalf("testsupport: connect admin: %v", err)
	}
	defer adminConn.Close(context.Background())

	if tmpl := templateDBName; tmpl != "" {
		if _, err := adminConn.Exec(ctx, "create database "+quoteIdent(dbName)+" template "+quoteIdent(tmpl)); err != nil {
			t.Fatalf("testsupport: create db from template: %v", err)
		}
	} else {
		if _, err := adminConn.Exec(ctx, "create database "+quoteIdent(dbName)); err != nil {
			t.Fatalf("testsupport: create db: %v", err)
		}
		targetDSN := replaceDatabase(baseDSN, dbName)
		loadConn, err := pgx.Connect(ctx, targetDSN)
		if err != nil {
			_ = dropDatabase(baseDSN, dbName)
			t.Fatalf("testsupport: connect target: %v", err)
		}
		if _, err := loadConn.Exec(ctx, loadSchemaSQL(t)); err != nil {
			_ = loadConn.Close(context.Background())
			_ = dropDatabase(baseDSN, dbName)
			t.Fatalf("testsupport: load schema: %v", err)
		}
		_ = loadConn.Close(context.Background())
		if err := setDatabaseSearchPath(ctx, baseDSN, dbName); err != nil {
			_ = dropDatabase(baseDSN, dbName)
			t.Fatalf("testsupport: set search_path: %v", err)
		}
	}

	targetDSN := replaceDatabase(baseDSN, dbName)
	pool, err := pgxpool.New(ctx, targetDSN)
	if err != nil {
		_ = dropDatabase(baseDSN, dbName)
		t.Fatalf("testsupport: open target pool: %v", err)
	}

	env := &Env{Pool: pool, DSN: targetDSN, DBName: dbName}
	t.Cleanup(func() {
		pool.Close()
		if err := dropDatabase(baseDSN, dbName); err != nil {
			t.Logf("testsupport: drop db %s: %v", dbName, err)
		}
	})
	return env
}

// Truncate clears the given tables in CASCADE mode and restarts identities.
// Use between sub-tests to reset state without rebuilding the schema.
func (e *Env) Truncate(t testing.TB, tables ...string) {
	t.Helper()
	if len(tables) == 0 {
		return
	}
	quoted := make([]string, len(tables))
	for i, name := range tables {
		quoted[i] = quoteIdent(name)
	}
	stmt := "truncate " + strings.Join(quoted, ", ") + " restart identity cascade"
	if _, err := e.Pool.Exec(context.Background(), stmt); err != nil {
		t.Fatalf("testsupport: truncate %v: %v", tables, err)
	}
}

// truncateAll removes all rows from every user table in the public schema and
// resets identity sequences.  Used by the shared-env path to isolate tests
// without recreating the database.
//
// Uses DELETE instead of TRUNCATE CASCADE: TRUNCATE acquires ACCESS EXCLUSIVE
// on all FK-related tables (45+ tables cascade from users) even when empty,
// taking ~1.5 s per call.  DELETE with session_replication_role='replica'
// disables FK checks and holds only ROW EXCLUSIVE; on empty tables it is
// essentially free (~1 ms each).  SET LOCAL scopes the role change to the
// transaction, so the connection returns to the pool with the default role.
func truncateAll(pool *pgxpool.Pool) error {
	ctx := context.Background()
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()

	allTables, idTables, err := publicTableNames(ctx, conn.Conn())
	if err != nil {
		return err
	}
	if len(allTables) == 0 {
		return nil
	}

	tx, err := conn.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// Disable FK constraint enforcement for this transaction only.
	if _, err = tx.Exec(ctx, "SET LOCAL session_replication_role = 'replica'"); err != nil {
		return err
	}
	// Build a single multi-statement payload that DELETEs every table and
	// resets the per-table SERIAL sequence in one server round-trip. The
	// pgx default QueryExecMode uses the extended/prepared protocol, which
	// rejects multi-statement SQL — pass QueryExecModeSimpleProtocol so the
	// driver speaks the simple query protocol for this Exec only.
	//
	// Cost model on the shared CI Postgres:
	//   - previously: 1 SELECT pg_tables + N DELETEs + N setval = 2N+1 trips
	//   - now: 1 cached pg_tables (once) + 1 batched DELETE+setval = 1 trip
	//   At ~466 tests this saves several thousand round-trips.
	var b strings.Builder
	b.Grow(len(allTables)*32 + len(idTables)*128)
	// Limit lock waits to 5 seconds: stuck transactions from parallel
	// tests must not hang cleanup forever.
	b.WriteString("SET LOCAL lock_timeout = '5000';")
	for _, name := range allTables {
		b.WriteString("DELETE FROM ")
		b.WriteString(quoteIdent(name))
		b.WriteString(";")
	}
	// setval runs only for tables with an `id` column. pg_get_serial_sequence
	// returns NULL when the column has no sequence (CASE skips); the dangerous
	// case (column doesn't exist at all → ERROR) is filtered upstream.
	for _, name := range idTables {
		b.WriteString("SELECT CASE WHEN seq IS NOT NULL THEN setval(seq, 1, false) END FROM pg_get_serial_sequence('")
		// pg_tables identifiers never contain ', but defend against future
		// schema regressions just in case.
		b.WriteString(strings.ReplaceAll(name, "'", "''"))
		b.WriteString("', 'id') seq;")
	}
	if _, err = tx.Exec(ctx, b.String(), pgx.QueryExecModeSimpleProtocol); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// publicTableNames returns the public-schema tablenames and the subset that
// has an `id` column, looking them up once per process and memoising the
// result. Schema is fixed across the integration run, so repeating the
// pg_tables query for every test cleanup is wasted work.
func publicTableNames(ctx context.Context, conn *pgx.Conn) (all []string, withID []string, err error) {
	cachedTableLookup.Do(func() {
		rows, err := conn.Query(ctx, `
			SELECT t.tablename,
			       EXISTS (
			         SELECT 1 FROM information_schema.columns c
			         WHERE c.table_schema = 'public'
			           AND c.table_name = t.tablename
			           AND c.column_name = 'id'
			       )
			FROM pg_tables t
			WHERE t.schemaname = 'public'
			ORDER BY t.tablename`)
		if err != nil {
			cachedTableLookupE = err
			return
		}
		defer rows.Close()
		for rows.Next() {
			var name string
			var hasID bool
			if err := rows.Scan(&name, &hasID); err != nil {
				cachedTableLookupE = err
				return
			}
			cachedAllTables = append(cachedAllTables, name)
			if hasID {
				cachedIDTables = append(cachedIDTables, name)
			}
		}
		if err := rows.Err(); err != nil {
			cachedTableLookupE = err
			return
		}
	})
	return cachedAllTables, cachedIDTables, cachedTableLookupE
}

// schemaBaseSQL is the production schema dump used to bootstrap each test DB.
//
//go:embed schema_base.sql
var schemaBaseSQL string

func loadSchemaSQL(t testing.TB) string {
	if schemaBaseSQL == "" {
		t.Fatalf("testsupport: schema_base.sql is empty (embed failed)")
	}
	return schemaBaseSQL
}

func randomSuffix() string {
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}

func quoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

func replaceDatabase(dsn, dbName string) string {
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		u, err := url.Parse(dsn)
		if err == nil {
			u.Path = "/" + dbName
			return u.String()
		}
	}
	parts := strings.Fields(dsn)
	found := false
	for i, p := range parts {
		if strings.HasPrefix(p, "dbname=") {
			parts[i] = "dbname=" + dbName
			found = true
		}
	}
	if !found {
		parts = append(parts, "dbname="+dbName)
	}
	return strings.Join(parts, " ")
}

func setDatabaseSearchPath(ctx context.Context, baseDSN, dbName string) error {
	conn, err := pgx.Connect(ctx, baseDSN)
	if err != nil {
		return err
	}
	defer conn.Close(context.Background())
	_, err = conn.Exec(ctx, "alter database "+quoteIdent(dbName)+" set search_path = public, pg_catalog")
	return err
}

func dropDatabase(baseDSN, dbName string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, err := pgx.Connect(ctx, baseDSN)
	if err != nil {
		return err
	}
	defer conn.Close(context.Background())
	_, _ = conn.Exec(ctx, "select pg_terminate_backend(pid) from pg_stat_activity where datname = $1 and pid <> pg_backend_pid()", dbName)
	_, err = conn.Exec(ctx, "drop database if exists "+quoteIdent(dbName))
	return err
}
