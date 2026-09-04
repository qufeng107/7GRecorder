package db

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/7grecorder/7grecorder/backend/internal/config"
	"github.com/7grecorder/7grecorder/backend/migrations"
	_ "github.com/mattn/go-sqlite3"
	"github.com/pressly/goose/v3"
)

func Migrate(ctx context.Context, cfg config.Config) error {
	database, err := sql.Open("sqlite3", cfg.SQLitePath+"?_foreign_keys=on&_busy_timeout=5000")
	if err != nil {
		return fmt.Errorf("open sqlite: %w", err)
	}
	defer database.Close()

	if _, err := database.ExecContext(ctx, "PRAGMA journal_mode=WAL;"); err != nil {
		return fmt.Errorf("set sqlite wal: %w", err)
	}
	if _, err := database.ExecContext(ctx, "PRAGMA foreign_keys=ON;"); err != nil {
		return fmt.Errorf("enable sqlite foreign keys: %w", err)
	}

	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("sqlite3"); err != nil {
		return fmt.Errorf("set goose dialect: %w", err)
	}
	if err := goose.UpContext(ctx, database, "."); err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}
	return nil
}
