package database

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"sort"

	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/migrations"
)

func Migrate(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version TEXT PRIMARY KEY,
		applied_at INTEGER NOT NULL
	)`); err != nil {
		return fmt.Errorf("create schema migrations: %w", err)
	}

	entries, err := fs.Glob(migrations.Files, "*.sql")
	if err != nil {
		return fmt.Errorf("list migrations: %w", err)
	}
	sort.Strings(entries)
	for _, path := range entries {
		if err := applyMigration(ctx, db, path); err != nil {
			return err
		}
	}
	return nil
}

func applyMigration(ctx context.Context, db *sql.DB, path string) error {
	var exists int
	if err := db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = ?)", path).Scan(&exists); err != nil {
		return fmt.Errorf("check migration %s: %w", path, err)
	}
	if exists == 1 {
		return nil
	}
	body, err := migrations.Files.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read migration %s: %w", path, err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migration %s: %w", path, err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, string(body)); err != nil {
		return fmt.Errorf("execute migration %s: %w", path, err)
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO schema_migrations(version, applied_at) VALUES (?, unixepoch('subsec') * 1000)", path); err != nil {
		return fmt.Errorf("record migration %s: %w", path, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration %s: %w", path, err)
	}
	return nil
}
