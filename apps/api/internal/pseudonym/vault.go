package pseudonym

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Entry is one decrypted vault row: alias number and the original value.
type Entry struct {
	Kind   string
	Number int
	Canon  string
	Key    string
}

type entryKey struct {
	kind   string
	number int
}

// Vault stores alias numbers and encrypted originals per user.
type Vault interface {
	Load(ctx context.Context, userID int64) ([]Entry, error)
	Assign(ctx context.Context, userID int64, kind, key, canon string) (int, error)
}

// PGVault is the Postgres implementation (table pii_vault).
type PGVault struct {
	Pool *pgxpool.Pool
	Keys *Keys
}

type sealed struct {
	Canon string `json:"c"`
	Key   string `json:"k"`
}

func (v *PGVault) Load(ctx context.Context, userID int64) ([]Entry, error) {
	rows, err := v.Pool.Query(ctx, `
		select kind, alias_no, nonce, ciphertext
		from pii_vault where user_id = $1 order by kind, alias_no`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Entry
	for rows.Next() {
		var kind string
		var number int
		var nonce, ct []byte
		if err := rows.Scan(&kind, &number, &nonce, &ct); err != nil {
			return nil, err
		}
		plain, err := v.Keys.Open(userID, kind, nonce, ct)
		if err != nil {
			// A row sealed with another key (rotation) or tampered with:
			// skip it rather than fail the whole request.
			continue
		}
		var s sealed
		if err := json.Unmarshal(plain, &s); err != nil {
			continue
		}
		out = append(out, Entry{Kind: kind, Number: number, Canon: s.Canon, Key: s.Key})
	}
	return out, rows.Err()
}

// Assign returns the alias number of (kind, key) for the user, creating the
// row on first sight. Numbers are dense per (user, kind); concurrent first
// sightings of different values race for the same number, and the loser
// retries.
func (v *PGVault) Assign(ctx context.Context, userID int64, kind, key, canon string) (int, error) {
	fp := v.Keys.Fingerprint(userID, kind, key)
	plain, err := json.Marshal(sealed{Canon: canon, Key: key})
	if err != nil {
		return 0, err
	}
	nonce, ct, err := v.Keys.Seal(userID, kind, plain)
	if err != nil {
		return 0, err
	}
	for attempt := 0; attempt < 5; attempt++ {
		var number int
		err = v.Pool.QueryRow(ctx, `
			insert into pii_vault (user_id, kind, alias_no, value_hmac, nonce, ciphertext)
			select $1, $2, coalesce(max(alias_no), 0) + 1, $3, $4, $5
			from pii_vault where user_id = $1 and kind = $2
			on conflict (user_id, kind, value_hmac)
			do update set last_used_at = now()
			returning alias_no`, userID, kind, fp, nonce, ct).Scan(&number)
		if err == nil {
			return number, nil
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			continue // another value took this number first
		}
		return 0, err
	}
	return 0, fmt.Errorf("pseudonym: could not assign an alias: %w", err)
}

// Purge deletes rows not used for ttl and all rows of deleted users.
// Account deletion itself removes rows through the foreign key cascade.
func Purge(ctx context.Context, pool *pgxpool.Pool, ttl time.Duration) (int64, error) {
	var total int64
	tag, err := pool.Exec(ctx, `delete from pii_vault where last_used_at < now() - $1::interval`,
		fmt.Sprintf("%d seconds", int64(ttl.Seconds())))
	if err != nil {
		return 0, err
	}
	total += tag.RowsAffected()
	tag, err = pool.Exec(ctx, `
		delete from pii_vault v using users u
		where u.id = v.user_id and u.deleted_at is not null`)
	if err != nil {
		return total, err
	}
	return total + tag.RowsAffected(), nil
}

// DeleteUser removes a user's whole vault (right to erasure without
// deleting the account itself).
func DeleteUser(ctx context.Context, pool *pgxpool.Pool, userID int64) error {
	_, err := pool.Exec(ctx, `delete from pii_vault where user_id = $1`, userID)
	return err
}

// Profile values of a user that must never leave in clear text.
type Profile struct {
	DisplayName string
	Email       string
	Phone       string
	Telegram    string
}

// ProfileSource loads a user's profile; an interface for tests.
type ProfileSource interface {
	Profile(ctx context.Context, userID int64) (Profile, error)
}

// PGProfiles reads the users table.
type PGProfiles struct{ Pool *pgxpool.Pool }

func (p PGProfiles) Profile(ctx context.Context, userID int64) (Profile, error) {
	var out Profile
	err := p.Pool.QueryRow(ctx, `
		select coalesce(display_name, ''), coalesce(email, ''), coalesce(phone, ''),
		       coalesce(telegram_username, '')
		from users where id = $1`, userID).Scan(&out.DisplayName, &out.Email, &out.Phone, &out.Telegram)
	if errors.Is(err, pgx.ErrNoRows) {
		return Profile{}, nil
	}
	return out, err
}
