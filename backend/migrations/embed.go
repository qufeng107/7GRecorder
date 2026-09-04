package migrations

import "embed"

// FS contains forward-only SQLite migrations.
//
//go:embed *.sql
var FS embed.FS
