package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	_ "github.com/lib/pq"
)

// Development-only administrator. Printed on every run so nobody has to
// guess; never used outside `make dev`.
const (
	DevAdminEmail    = "admin@example.com"
	DevAdminPassword = "dev-password-only"
)

func main() {
	dsn := flag.String("database-url", os.Getenv("DATABASE_URL"), "local development database")
	api := flag.String("api", "http://localhost:18080", "local API address")
	token := flag.String("bootstrap-token", os.Getenv("BOOTSTRAP_ADMIN_TOKEN"), "BOOTSTRAP_ADMIN_TOKEN from .env")
	n := flag.Int("clients", 12, "number of synthetic clients")
	seed := flag.Int64("seed", 1, "random seed (same seed, same data)")
	flag.Parse()

	if err := allowed(os.Getenv("DEV_SEED_ALLOWED"), os.Getenv("APP_ENV")); err != nil {
		log.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	created, err := bootstrapAdminWithRetry(ctx, *api, *token, 2*time.Second)
	if err != nil {
		log.Fatalf("admin: %v", err)
	}
	db, err := sql.Open("postgres", *dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	if err := seedClients(ctx, db, Generate(*seed, *n)); err != nil {
		log.Fatalf("clients: %v", err)
	}
	fmt.Printf("dev data ready: %d synthetic clients\n", *n)
	if created {
		fmt.Printf("admin login: %s / %s (development only)\n", DevAdminEmail, DevAdminPassword)
	} else {
		fmt.Println("an administrator already exists: sign in with that account (dev admin was not created)")
	}
}

// allowed keeps the seeder away from anything but a local development setup.
func allowed(flagValue, appEnv string) error {
	if flagValue != "1" {
		return fmt.Errorf("devseed runs only from `make dev` (DEV_SEED_ALLOWED=1)")
	}
	if strings.EqualFold(appEnv, "production") {
		return fmt.Errorf("devseed never runs with APP_ENV=production")
	}
	return nil
}

// bootstrapAdminWithRetry waits for the API to come up: `make dev` starts the
// seeder right after `docker compose up`, while the API may still be booting.
// Connection errors and 502/503/504 are retried every interval until ctx ends;
// any other answer (a refused token, for example) is final.
func bootstrapAdminWithRetry(ctx context.Context, api, token string, interval time.Duration) (bool, error) {
	for {
		created, err := bootstrapAdmin(ctx, api, token)
		var notReady errAPINotReady
		if err == nil || !errors.As(err, &notReady) {
			return created, err
		}
		select {
		case <-ctx.Done():
			return false, fmt.Errorf("API at %s did not come up: %w", api, err)
		case <-time.After(interval):
		}
	}
}

// errAPINotReady marks answers that mean "still starting", not "no".
type errAPINotReady struct{ reason string }

func (e errAPINotReady) Error() string { return "API not ready: " + e.reason }

// bootstrapAdmin creates the development admin. created is false when an
// administrator already existed: the API allows only the first one.
func bootstrapAdmin(ctx context.Context, api, token string) (created bool, err error) {
	body, _ := json.Marshal(map[string]string{"email": DevAdminEmail, "password": DevAdminPassword})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(api, "/")+"/api/bootstrap/admin", bytes.NewReader(body))
	if err != nil {
		return false, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Bootstrap-Token", token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return false, err
		}
		return false, errAPINotReady{reason: err.Error()}
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusCreated:
		return true, nil
	case http.StatusConflict:
		return false, nil
	case http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return false, errAPINotReady{reason: resp.Status}
	default:
		return false, fmt.Errorf("bootstrap returned %s", resp.Status)
	}
}

func seedClients(ctx context.Context, db *sql.DB, clients []Client) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var modeID int64
	err = tx.QueryRowContext(ctx, `select id from modes where hidden_at is null order by id limit 1`).Scan(&modeID)
	if err == sql.ErrNoRows {
		err = tx.QueryRowContext(ctx, `
			insert into modes (name, ai_model, model_temperature, prompt, welcome_message)
			values ('Демо-режим', 'gpt-4o-mini', 0.7,
			        'Ты доброжелательный собеседник. Это демо-режим для локальной разработки.',
			        'Привет! Это демо-режим.')
			returning id`).Scan(&modeID)
	}
	if err != nil {
		return fmt.Errorf("mode: %w", err)
	}
	for _, c := range clients {
		var userID int64
		err := tx.QueryRowContext(ctx, `
			insert into users (email, display_name, phone, accepted_tos, role, status)
			select $1, $2, $3, true, 'user', 'active'
			where not exists (select 1 from users where email = $1)
			returning id`, c.Email, c.DisplayName, c.Phone).Scan(&userID)
		if err == sql.ErrNoRows {
			continue // already seeded
		}
		if err != nil {
			return fmt.Errorf("user %s: %w", c.Email, err)
		}
		var dialogID int64
		if err := tx.QueryRowContext(ctx, `
			insert into users_dialogs (user_id, mode_id, message_count)
			values ($1, $2, $3)
			returning id`, userID, modeID, len(c.Messages)).Scan(&dialogID); err != nil {
			return fmt.Errorf("dialog: %w", err)
		}
		for _, m := range c.Messages {
			if _, err := tx.ExecContext(ctx, `
				insert into dialogs_messages (dialog_id, role, content)
				values ($1, $2, $3)`,
				dialogID, m.Role, m.Content); err != nil {
				return fmt.Errorf("message: %w", err)
			}
		}
	}
	return tx.Commit()
}
