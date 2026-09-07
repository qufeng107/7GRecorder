-- +goose Up
PRAGMA foreign_keys = ON;

CREATE TABLE upload_source_outputs (
    id INTEGER PRIMARY KEY,
    upload_source_id INTEGER NOT NULL,
    sort_order INTEGER NOT NULL,
    relative_path TEXT NOT NULL,
    size_bytes INTEGER NOT NULL DEFAULT 0,
    duration_ms INTEGER NOT NULL DEFAULT 0,
    timeline_start_ms INTEGER NOT NULL DEFAULT 0,
    timeline_end_ms INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'READY_TO_UPLOAD',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (upload_source_id) REFERENCES upload_sources(id) ON DELETE RESTRICT
);

CREATE UNIQUE INDEX ux_upload_source_outputs_source_order
    ON upload_source_outputs(upload_source_id, sort_order);

CREATE INDEX ix_upload_source_outputs_source_status
    ON upload_source_outputs(upload_source_id, status);

ALTER TABLE upload_source_cos_objects ADD COLUMN upload_source_output_id INTEGER REFERENCES upload_source_outputs(id) ON DELETE RESTRICT;

DROP INDEX ux_upload_source_cos_objects_profile_source;

CREATE UNIQUE INDEX ux_upload_source_cos_objects_profile_output
    ON upload_source_cos_objects(cos_storage_profile_id, upload_source_output_id)
    WHERE upload_source_output_id IS NOT NULL;

-- +goose Down
SELECT 'forward-only migration; restore SQLite backup for rollback';
