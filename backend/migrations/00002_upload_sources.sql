-- +goose Up
PRAGMA foreign_keys = ON;

CREATE TABLE upload_sources (
    id INTEGER PRIMARY KEY,
    recording_profile_id INTEGER NOT NULL,
    source_key TEXT NOT NULL UNIQUE,
    title TEXT,
    source_room_id TEXT NOT NULL,
    streamer_name_snapshot TEXT NOT NULL,
    started_at DATETIME NOT NULL,
    completed_at DATETIME NOT NULL,
    duration_ms INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL,
    output_relative_path TEXT,
    output_recording_file_id INTEGER,
    total_bytes INTEGER NOT NULL DEFAULT 0,
    recording_count INTEGER NOT NULL DEFAULT 0,
    file_count INTEGER NOT NULL DEFAULT 0,
    max_gap_seconds INTEGER NOT NULL DEFAULT 0,
    merge_gap_threshold_seconds INTEGER NOT NULL DEFAULT 600,
    metadata_json TEXT,
    ready_at DATETIME,
    last_error TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (recording_profile_id) REFERENCES recording_profiles(id) ON DELETE RESTRICT,
    FOREIGN KEY (output_recording_file_id) REFERENCES recording_files(id) ON DELETE RESTRICT
);

CREATE INDEX ix_upload_sources_profile_started ON upload_sources(recording_profile_id, started_at);
CREATE INDEX ix_upload_sources_status_ready ON upload_sources(status, ready_at);

CREATE TABLE upload_source_segments (
    id INTEGER PRIMARY KEY,
    upload_source_id INTEGER NOT NULL,
    recording_id INTEGER NOT NULL,
    recording_file_id INTEGER NOT NULL,
    sort_order INTEGER NOT NULL,
    source_started_at DATETIME NOT NULL,
    source_completed_at DATETIME NOT NULL,
    timeline_start_ms INTEGER NOT NULL,
    timeline_end_ms INTEGER NOT NULL,
    relative_path TEXT NOT NULL,
    size_bytes INTEGER NOT NULL DEFAULT 0,
    duration_ms INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (upload_source_id) REFERENCES upload_sources(id) ON DELETE RESTRICT,
    FOREIGN KEY (recording_id) REFERENCES recordings(id) ON DELETE RESTRICT,
    FOREIGN KEY (recording_file_id) REFERENCES recording_files(id) ON DELETE RESTRICT
);

CREATE UNIQUE INDEX ux_upload_source_segments_recording ON upload_source_segments(recording_id);
CREATE INDEX ix_upload_source_segments_source_order ON upload_source_segments(upload_source_id, sort_order);

-- +goose Down
SELECT 'forward-only migration; restore SQLite backup for rollback';
