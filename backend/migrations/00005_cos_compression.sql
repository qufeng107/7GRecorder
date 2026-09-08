-- +goose Up
PRAGMA foreign_keys = ON;

ALTER TABLE upload_source_cos_objects ADD COLUMN source_size_bytes INTEGER NOT NULL DEFAULT 0;
ALTER TABLE upload_source_cos_objects ADD COLUMN compression_status TEXT NOT NULL DEFAULT 'DISABLED';
ALTER TABLE upload_source_cos_objects ADD COLUMN compression_preset TEXT;
ALTER TABLE upload_source_cos_objects ADD COLUMN compressed_from_relative_path TEXT;

UPDATE upload_source_cos_objects
SET source_size_bytes = size_bytes
WHERE source_size_bytes = 0;

-- +goose Down
SELECT 'forward-only migration; restore SQLite backup for rollback';
