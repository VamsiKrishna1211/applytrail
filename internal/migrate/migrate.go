// Package migrate applies the embedded schema migrations.
//
// Both the CLI and the test harness go through here, so a test can never run
// against a schema the deployment would not produce.
package migrate

import (
	"context"
	"database/sql"
	"fmt"

	_ "github.com/jackc/pgx/v5/stdlib" // database/sql driver for goose
	"github.com/pressly/goose/v3"

	appdb "github.com/VamsiKrishna1211/applytrail/db"
)

const driverName = "pgx"

// Up applies every pending migration.
func Up(ctx context.Context, databaseURL string) error {
	return withDB(ctx, databaseURL, func(db *sql.DB) error {
		return goose.UpContext(ctx, db, appdb.MigrationsDir)
	})
}

// Down rolls back the most recent migration.
func Down(ctx context.Context, databaseURL string) error {
	return withDB(ctx, databaseURL, func(db *sql.DB) error {
		return goose.DownContext(ctx, db, appdb.MigrationsDir)
	})
}

// Status reports the migration state through goose's logger.
func Status(ctx context.Context, databaseURL string) error {
	return withDB(ctx, databaseURL, func(db *sql.DB) error {
		return goose.StatusContext(ctx, db, appdb.MigrationsDir)
	})
}

// Version returns the current schema version.
func Version(ctx context.Context, databaseURL string) (int64, error) {
	var version int64
	err := withDB(ctx, databaseURL, func(db *sql.DB) error {
		v, err := goose.GetDBVersionContext(ctx, db)
		version = v
		return err
	})
	return version, err
}

func withDB(ctx context.Context, databaseURL string, fn func(*sql.DB) error) error {
	goose.SetBaseFS(appdb.MigrationsFS)
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("set goose dialect: %w", err)
	}

	db, err := sql.Open(driverName, databaseURL)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping database: %w", err)
	}
	return fn(db)
}
