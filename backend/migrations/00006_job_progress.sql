-- +goose Up
ALTER TABLE jobs ADD COLUMN progress_current_bytes INTEGER NOT NULL DEFAULT 0;
ALTER TABLE jobs ADD COLUMN progress_total_bytes INTEGER NOT NULL DEFAULT 0;
ALTER TABLE jobs ADD COLUMN progress_message TEXT;
ALTER TABLE jobs ADD COLUMN progress_updated_at TEXT;

-- +goose Down
-- SQLite cannot drop columns without rebuilding the table. This migration is intentionally one-way.
SELECT 1;
