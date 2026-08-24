// Package db embeds the SQL migrations so the server binary, the CLI, and the
// test harness all apply exactly the same schema without depending on files
// being present on disk next to the binary.
package db

import "embed"

// MigrationsFS holds every goose migration.
//
//go:embed migrations/*.sql
var MigrationsFS embed.FS

// MigrationsDir is the path to the migrations inside MigrationsFS.
const MigrationsDir = "migrations"
