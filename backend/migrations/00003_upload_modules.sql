-- +goose Up
PRAGMA foreign_keys = ON;

ALTER TABLE publications ADD COLUMN upload_source_id INTEGER REFERENCES upload_sources(id) ON DELETE RESTRICT;

CREATE UNIQUE INDEX ux_publications_upload_source_platform
    ON publications(upload_source_id, platform)
    WHERE upload_source_id IS NOT NULL;

CREATE TABLE upload_source_cos_objects (
    id INTEGER PRIMARY KEY,
    cos_storage_profile_id INTEGER NOT NULL,
    recording_profile_id INTEGER NOT NULL,
    upload_source_id INTEGER NOT NULL,
    object_key TEXT NOT NULL,
    size_bytes INTEGER NOT NULL,
    checksum TEXT,
    etag TEXT,
    status TEXT NOT NULL,
    last_error TEXT,
    uploaded_at DATETIME,
    deleted_at DATETIME,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (cos_storage_profile_id) REFERENCES cos_storage_profiles(id) ON DELETE RESTRICT,
    FOREIGN KEY (recording_profile_id) REFERENCES recording_profiles(id) ON DELETE RESTRICT,
    FOREIGN KEY (upload_source_id) REFERENCES upload_sources(id) ON DELETE RESTRICT
);

CREATE UNIQUE INDEX ux_upload_source_cos_objects_profile_source
    ON upload_source_cos_objects(cos_storage_profile_id, upload_source_id);

CREATE UNIQUE INDEX ux_upload_source_cos_objects_profile_key
    ON upload_source_cos_objects(cos_storage_profile_id, object_key);

CREATE INDEX ix_upload_source_cos_objects_profile_status_uploaded
    ON upload_source_cos_objects(cos_storage_profile_id, status, uploaded_at);

ALTER TABLE jobs ADD COLUMN upload_source_id INTEGER REFERENCES upload_sources(id) ON DELETE RESTRICT;

CREATE INDEX ix_jobs_upload_source_status ON jobs(upload_source_id, status);

-- +goose Down
SELECT 'forward-only migration; restore SQLite backup for rollback';
