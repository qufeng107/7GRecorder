-- +goose Up
ALTER TABLE cos_objects ADD COLUMN quota_reserved_bytes INTEGER NOT NULL DEFAULT 0;
ALTER TABLE upload_source_cos_objects ADD COLUMN quota_reserved_bytes INTEGER NOT NULL DEFAULT 0;
UPDATE cos_objects SET quota_reserved_bytes = MAX(size_bytes, 0) WHERE status IN ('UPLOADING', 'FAILED');
UPDATE upload_source_cos_objects SET quota_reserved_bytes = MAX(size_bytes, 0) WHERE status IN ('UPLOADING', 'FAILED');

-- +goose Down
SELECT 'forward-only migration; restore SQLite backup for rollback';
