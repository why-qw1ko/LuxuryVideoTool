package migrations

import "embed"

// Files contains all forward-only database migrations.
//
//go:embed *.sql
var Files embed.FS

