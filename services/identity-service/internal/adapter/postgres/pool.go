package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ParseConfig parses a DSN and applies the connection rules from
// docs/architecture/04-data.md §4.2 (statement timeout 5 s,
// idle-in-transaction timeout 10 s, application_name).
func ParseConfig(dsn string) (*pgxpool.Config, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}
	rp := cfg.ConnConfig.RuntimeParams
	if rp["application_name"] == "" {
		rp["application_name"] = "identity-service"
	}
	if rp["statement_timeout"] == "" {
		rp["statement_timeout"] = "5000"
	}
	if rp["idle_in_transaction_session_timeout"] == "" {
		rp["idle_in_transaction_session_timeout"] = "10000"
	}
	return cfg, nil
}

// Open creates a connection pool.
func Open(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	cfg, err := ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("open pool: %w", err)
	}
	return pool, nil
}
