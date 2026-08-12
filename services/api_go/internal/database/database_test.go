package database

import (
	"context"
	"path/filepath"
	"testing"
)

func TestOpenRunsMigrationsIdempotently(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.db")
	db, err := Open(context.Background(), path)
	if err != nil { t.Fatalf("Open() error = %v", err) }
	defer db.Close()
	if err := Migrate(context.Background(), db); err != nil { t.Fatalf("second Migrate() error = %v", err) }
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM schema_migrations").Scan(&count); err != nil { t.Fatal(err) }
	if count != 1 { t.Fatalf("migration count = %d", count) }
}

