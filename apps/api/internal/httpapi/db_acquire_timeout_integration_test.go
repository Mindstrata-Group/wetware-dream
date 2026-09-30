//go:build integration

package httpapi

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"mindstrata-stage1/api/internal/testsupport"
)

// TestBeginTxTimeout_PoolExhausted_FailsFast: K-1. When the connection pool is
// exhausted, beginTxTimeout must quickly return context.DeadlineExceeded instead
// of hanging until the parent context is cancelled (in prod, until the server's
// WriteTimeout=90s).
func TestBeginTxTimeout_PoolExhausted_FailsFast(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)

	cfg, err := pgxpool.ParseConfig(env.DSN)
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	cfg.MaxConns = 1

	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatalf("new pool: %v", err)
	}
	defer pool.Close()

	// Occupy the pool's only connection with a long transaction.
	holder, err := pool.Begin(context.Background())
	if err != nil {
		t.Fatalf("begin holder tx: %v", err)
	}
	defer holder.Rollback(context.Background())

	const timeout = 200 * time.Millisecond
	start := time.Now()
	_, err = beginTxTimeout(context.Background(), pool, timeout)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatalf("expected error when pool exhausted, got nil")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context.DeadlineExceeded, got %v", err)
	}
	// It must finish around the timeout, not hang for minutes.
	if elapsed > 2*time.Second {
		t.Fatalf("beginTxTimeout took too long: %v (want ~%v)", elapsed, timeout)
	}
	if elapsed < timeout {
		t.Fatalf("beginTxTimeout returned too early: %v (want >= %v)", elapsed, timeout)
	}
}

// TestBeginTxTimeout_Success_NotLimitedAfterBegin: after a successful Begin
// (pool not exhausted) an acquireCtx with an expired timeout must not affect the
// rest of the transaction: it uses its own ctx.
func TestBeginTxTimeout_Success_NotLimitedAfterBegin(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)

	tx, err := beginTxTimeout(context.Background(), env.Pool, dbAcquireTimeout)
	if err != nil {
		t.Fatalf("beginTxTimeout: %v", err)
	}
	defer tx.Rollback(context.Background())

	// acquireCtx inside beginTxTimeout is already cancelled (defer cancel()), but the
	// transaction runs on a separate connection and is not bound to it.
	time.Sleep(50 * time.Millisecond)
	if _, err := tx.Exec(context.Background(), "select 1"); err != nil {
		t.Fatalf("exec after beginTxTimeout returned: %v", err)
	}
}
