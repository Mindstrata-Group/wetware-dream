package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "github.com/lib/pq"
)

func main() {
	dir := flag.String("dir", "apps/api/sql", "directory with *.sql migrations")
	dsn := flag.String("database-url", os.Getenv("DATABASE_URL"), "PostgreSQL connection string (or DATABASE_URL)")
	dryRun := flag.Bool("dry-run", false, "print pending migrations without applying them")
	flag.Parse()

	if strings.TrimSpace(*dsn) == "" && !*dryRun {
		log.Fatal("DATABASE_URL or -database-url is required")
	}

	migrations, err := readMigrations(*dir)
	if err != nil {
		log.Fatal(err)
	}
	if len(migrations) == 0 {
		log.Printf("no migrations found in %s", *dir)
		return
	}

	if *dryRun {
		for _, migration := range migrations {
			fmt.Println(filepath.Base(migration))
		}
		return
	}

	db, err := sql.Open("postgres", *dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		log.Fatal(err)
	}
	if err := ensureSchemaMigrations(ctx, db); err != nil {
		log.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `select pg_advisory_lock(hashtext('mindstrata_schema_migrations'))`); err != nil {
		log.Fatal(err)
	}
	defer db.ExecContext(context.Background(), `select pg_advisory_unlock(hashtext('mindstrata_schema_migrations'))`)

	applied, err := appliedMigrations(ctx, db)
	if err != nil {
		log.Fatal(err)
	}

	for _, migration := range migrations {
		name := filepath.Base(migration)
		if applied[name] {
			log.Printf("skip %s", name)
			continue
		}
		if err := applyMigration(ctx, db, migration, name); err != nil {
			log.Fatalf("apply %s: %v", name, err)
		}
		log.Printf("applied %s", name)
	}
}

func readMigrations(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", dir, err)
	}
	migrations := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || strings.HasPrefix(name, "_") || !strings.HasSuffix(name, ".sql") {
			continue
		}
		if len(name) < len("20060102_150405_x.sql") || name[8] != '_' || name[15] != '_' {
			return nil, fmt.Errorf("migration %q must use UTC timestamp format YYYYMMDD_HHMMSS_name.sql", name)
		}
		migrations = append(migrations, filepath.Join(dir, name))
	}
	sort.Strings(migrations)
	return migrations, nil
}

func ensureSchemaMigrations(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
		create table if not exists schema_migrations (
			filename text primary key,
			applied_at timestamptz not null default now()
		)`)
	return err
}

func appliedMigrations(ctx context.Context, db *sql.DB) (map[string]bool, error) {
	rows, err := db.QueryContext(ctx, `select filename from schema_migrations`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	applied := map[string]bool{}
	for rows.Next() {
		var filename string
		if err := rows.Scan(&filename); err != nil {
			return nil, err
		}
		applied[filename] = true
	}
	return applied, rows.Err()
}

func applyMigration(ctx context.Context, db *sql.DB, path, name string) error {
	sqlText, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, string(sqlText)); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `insert into schema_migrations (filename) values ($1)`, name); err != nil {
		return err
	}
	return tx.Commit()
}
