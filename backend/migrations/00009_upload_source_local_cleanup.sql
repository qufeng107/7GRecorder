-- +goose Up
PRAGMA foreign_keys = ON;

ALTER TABLE upload_sources ADD COLUMN local_cleanup_status TEXT NOT NULL DEFAULT 'AVAILABLE';
ALTER TABLE upload_sources ADD COLUMN local_deleted_at DATETIME;

CREATE INDEX ix_upload_sources_local_cleanup
    ON upload_sources(local_cleanup_status, started_at);

-- +goose Down
SELECT 'forward-only migration; restore SQLite backup for rollback';
