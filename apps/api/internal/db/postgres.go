package db

import (
	"context"
	"os"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func OpenPool(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, err
	}

	cfg.MaxConns = int32Env("PG_MAX_CONNS", 30)
	cfg.MinConns = int32Env("PG_MIN_CONNS", 5)
	cfg.MaxConnLifetime = time.Hour
	cfg.MaxConnIdleTime = 15 * time.Minute
	cfg.HealthCheckPeriod = 30 * time.Second

	return pgxpool.NewWithConfig(ctx, cfg)
}

func int32Env(key string, fallback int32) int32 {
	value, err := strconv.ParseInt(os.Getenv(key), 10, 32)
	if err != nil || value <= 0 {
		return fallback
	}
	return int32(value)
}
