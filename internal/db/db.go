package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"

	_ "github.com/jackc/pgx/v5/stdlib"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

type DB struct {
	*sql.DB
}

func Connect(ctx context.Context, connStr string) (*DB, error) {
	db, err := sql.Open("pgx", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open database connection: %w", err)
	}

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &DB{DB: db}, nil
}

func (db *DB) RunMigrations(ctx context.Context) error {
	migrationSQL, err := migrationFS.ReadFile("migrations/000001_init.up.sql")
	if err != nil {
		return fmt.Errorf("failed to read migration file: %w", err)
	}

	_, err = db.ExecContext(ctx, string(migrationSQL))
	if err != nil {
		return fmt.Errorf("failed to execute migration: %w", err)
	}

	_, _ = db.ExecContext(ctx, "ALTER TABLE tracked_meta_repos ADD COLUMN IF NOT EXISTS submodule_changes_only BOOLEAN NOT NULL DEFAULT TRUE")

	return nil
}
