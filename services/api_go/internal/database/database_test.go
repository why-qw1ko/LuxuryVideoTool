package database

import (
	"context"
	"io/fs"
	"path/filepath"
	"testing"

	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/migrations"
)

func TestOpenRunsMigrationsIdempotently(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.db")
	db, err := Open(context.Background(), path)
	if err != nil { t.Fatalf("Open() error = %v", err) }
	defer db.Close()
	if err := Migrate(context.Background(), db); err != nil { t.Fatalf("second Migrate() error = %v", err) }
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM schema_migrations").Scan(&count); err != nil { t.Fatal(err) }
	entries, err := fs.Glob(migrations.Files, "*.sql")
	if err != nil { t.Fatal(err) }
	if count != len(entries) { t.Fatalf("migration count = %d, want %d", count, len(entries)) }
}

