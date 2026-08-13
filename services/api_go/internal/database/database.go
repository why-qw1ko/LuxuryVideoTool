package database

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

func Open(ctx context.Context, path string) (*sql.DB, error) {
	if path == "" {
		return nil, fmt.Errorf("database path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve database path: %w", err)
	}
	dsn := "file:" + filepath.ToSlash(absPath) + "?cache=shared&mode=rwc&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}
	if err := os.Chmod(absPath, 0o600); err != nil {
		db.Close()
		return nil, fmt.Errorf("set database permissions: %w", err)
	}
	if err := Migrate(ctx, db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func Ready(ctx context.Context, db *sql.DB, dataDir string) error {
	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("database: %w", err)
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return fmt.Errorf("data directory: %w", err)
	}
	probe, err := os.CreateTemp(dataDir, ".ready-*")
	if err != nil {
		return fmt.Errorf("data directory not writable: %w", err)
	}
	name := probe.Name()
	if err := probe.Close(); err != nil {
		return fmt.Errorf("close readiness probe: %w", err)
	}
	if err := os.Remove(name); err != nil {
		return fmt.Errorf("remove readiness probe: %w", err)
	}
	return nil
}
