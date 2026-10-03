package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"log/slog"

	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/db"
)

// MigrateUp applies the embedded goose migrations using the migrator role DSN.
func MigrateUp(ctx context.Context, dsn string, log *slog.Logger) error {
	cfg, err := ParseConfig(dsn)
	if err != nil {
		return err
	}
	// DDL must not inherit the app's 5 s statement timeout.
	cfg.ConnConfig.RuntimeParams["statement_timeout"] = "0"
	cfg.ConnConfig.RuntimeParams["lock_timeout"] = "30000"
	sqldb := stdlib.OpenDB(*cfg.ConnConfig)
	defer func() { _ = sqldb.Close() }()
	return migrateUp(ctx, sqldb, log)
}

func migrateUp(ctx context.Context, sqldb *sql.DB, log *slog.Logger) error {
	sub, err := fs.Sub(db.Migrations, "migrations")
	if err != nil {
		return err
	}
	provider, err := goose.NewProvider(goose.DialectPostgres, sqldb, sub)
	if err != nil {
		return fmt.Errorf("goose provider: %w", err)
	}
	results, err := provider.Up(ctx)
	if err != nil {
		return fmt.Errorf("migrate up: %w", err)
	}
	for _, r := range results {
		log.InfoContext(ctx, "migration applied", "version", r.Source.Version, "duration_ms", r.Duration.Milliseconds())
	}
	return nil
}
