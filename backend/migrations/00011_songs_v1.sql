-- +goose NO TRANSACTION
-- +goose Up
CREATE TEMP TABLE songs_v1_empty_guard (
    row_count INTEGER NOT NULL CHECK (row_count = 0)
);
INSERT INTO songs_v1_empty_guard (row_count)
SELECT (SELECT COUNT(*) FROM songs) + (SELECT COUNT(*) FROM song_candidates);
DROP TABLE songs_v1_empty_guard;

PRAGMA foreign_keys = OFF;

DROP TABLE IF EXISTS song_candidates;
DROP TABLE IF EXISTS songs;

CREATE TABLE song_settings (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    enabled INTEGER NOT NULL DEFAULT 0,
    credential_id INTEGER,
    region TEXT NOT NULL DEFAULT '',
    container_id TEXT NOT NULL DEFAULT '',
    destination_cos_storage_profile_id INTEGER,
    songs_prefix TEXT NOT NULL DEFAULT 'songs',
    boundary_padding_ms INTEGER NOT NULL DEFAULT 0,
    algorithm_version TEXT NOT NULL DEFAULT 'v1',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (credential_id) REFERENCES credentials(id) ON DELETE RESTRICT,
    FOREIGN KEY (destination_cos_storage_profile_id) REFERENCES cos_storage_profiles(id) ON DELETE RESTRICT
);

INSERT INTO song_settings (id) VALUES (1);

CREATE TABLE song_analysis_runs (
    id INTEGER PRIMARY KEY,
    source_cos_object_id INTEGER NOT NULL,
    upload_source_id INTEGER NOT NULL,
    upload_source_output_id INTEGER NOT NULL,
    recording_profile_id INTEGER NOT NULL,
    source_cos_storage_profile_id INTEGER NOT NULL,
    source_object_key TEXT NOT NULL,
    source_etag TEXT,
    source_size_bytes INTEGER NOT NULL,
    source_timeline_start_ms INTEGER NOT NULL,
    source_timeline_end_ms INTEGER NOT NULL,
    source_relative_path TEXT,
    provider_region TEXT NOT NULL,
    provider_container_id TEXT NOT NULL,
    provider_credential_id INTEGER NOT NULL,
    destination_cos_storage_profile_id INTEGER NOT NULL,
    songs_prefix TEXT NOT NULL,
    boundary_padding_ms INTEGER NOT NULL,
    algorithm_version TEXT NOT NULL,
    status TEXT NOT NULL,
    progress_message TEXT,
    last_error_class TEXT,
    last_error TEXT,
    cancelled_at DATETIME,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (source_cos_object_id) REFERENCES upload_source_cos_objects(id) ON DELETE RESTRICT,
    FOREIGN KEY (upload_source_id) REFERENCES upload_sources(id) ON DELETE RESTRICT,
    FOREIGN KEY (upload_source_output_id) REFERENCES upload_source_outputs(id) ON DELETE RESTRICT,
    FOREIGN KEY (recording_profile_id) REFERENCES recording_profiles(id) ON DELETE RESTRICT,
    FOREIGN KEY (source_cos_storage_profile_id) REFERENCES cos_storage_profiles(id) ON DELETE RESTRICT,
    FOREIGN KEY (provider_credential_id) REFERENCES credentials(id) ON DELETE RESTRICT,
    FOREIGN KEY (destination_cos_storage_profile_id) REFERENCES cos_storage_profiles(id) ON DELETE RESTRICT
);

CREATE INDEX ix_song_analysis_runs_status ON song_analysis_runs(status, created_at);
CREATE INDEX ix_song_analysis_runs_source ON song_analysis_runs(source_cos_object_id, created_at);

CREATE TABLE song_analysis_chunks (
    id INTEGER PRIMARY KEY,
    analysis_run_id INTEGER NOT NULL,
    chunk_index INTEGER NOT NULL,
    core_start_ms INTEGER NOT NULL,
    core_end_ms INTEGER NOT NULL,
    guard_start_ms INTEGER NOT NULL,
    guard_end_ms INTEGER NOT NULL,
    provider_filename TEXT NOT NULL,
    provider_file_id TEXT,
    status TEXT NOT NULL,
    result_json TEXT,
    poll_count INTEGER NOT NULL DEFAULT 0,
    next_poll_at DATETIME,
    last_error TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (analysis_run_id) REFERENCES song_analysis_runs(id) ON DELETE RESTRICT
);

CREATE UNIQUE INDEX ux_song_analysis_chunks_run_index ON song_analysis_chunks(analysis_run_id, chunk_index);

CREATE TABLE song_recognition_matches (
    id INTEGER PRIMARY KEY,
    analysis_run_id INTEGER NOT NULL,
    analysis_chunk_id INTEGER NOT NULL,
    engine TEXT NOT NULL,
    acrid TEXT,
    isrc TEXT,
    title TEXT,
    artist TEXT,
    chunk_start_ms INTEGER NOT NULL,
    chunk_end_ms INTEGER NOT NULL,
    global_start_ms INTEGER NOT NULL,
    global_end_ms INTEGER NOT NULL,
    score REAL,
    evidence_json TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (analysis_run_id) REFERENCES song_analysis_runs(id) ON DELETE RESTRICT,
    FOREIGN KEY (analysis_chunk_id) REFERENCES song_analysis_chunks(id) ON DELETE RESTRICT
);

CREATE INDEX ix_song_matches_run_time ON song_recognition_matches(analysis_run_id, global_start_ms);

CREATE TABLE songs (
    id INTEGER PRIMARY KEY,
    recording_profile_id INTEGER NOT NULL,
    recording_id INTEGER,
    upload_source_id INTEGER NOT NULL,
    analysis_run_id INTEGER NOT NULL,
    title TEXT,
    artist TEXT,
    detected_start_ms INTEGER NOT NULL,
    detected_end_ms INTEGER NOT NULL,
    start_ms INTEGER NOT NULL,
    end_ms INTEGER NOT NULL,
    confidence REAL,
    status TEXT NOT NULL DEFAULT 'DRAFT',
    clip_revision INTEGER NOT NULL DEFAULT 1,
    audio_artifact_status TEXT NOT NULL DEFAULT 'NONE',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (recording_profile_id) REFERENCES recording_profiles(id) ON DELETE RESTRICT,
    FOREIGN KEY (recording_id) REFERENCES recordings(id) ON DELETE RESTRICT,
    FOREIGN KEY (upload_source_id) REFERENCES upload_sources(id) ON DELETE RESTRICT,
    FOREIGN KEY (analysis_run_id) REFERENCES song_analysis_runs(id) ON DELETE RESTRICT,
    CHECK (start_ms >= 0 AND end_ms > start_ms),
    CHECK (detected_start_ms >= 0 AND detected_end_ms > detected_start_ms)
);

CREATE INDEX ix_songs_run_start ON songs(analysis_run_id, start_ms);
CREATE INDEX ix_songs_upload_source_start ON songs(upload_source_id, start_ms);

CREATE TABLE song_candidates (
    id INTEGER PRIMARY KEY,
    song_id INTEGER NOT NULL,
    title TEXT,
    artist TEXT,
    source TEXT NOT NULL,
    score REAL,
    evidence_json TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (song_id) REFERENCES songs(id) ON DELETE RESTRICT
);

CREATE TABLE song_artifacts (
    id INTEGER PRIMARY KEY,
    song_id INTEGER NOT NULL,
    clip_revision INTEGER NOT NULL,
    kind TEXT NOT NULL DEFAULT 'AUDIO_M4A',
    cos_storage_profile_id INTEGER NOT NULL,
    object_key TEXT NOT NULL,
    size_bytes INTEGER NOT NULL DEFAULT 0,
    etag TEXT,
    status TEXT NOT NULL,
    replaces_artifact_id INTEGER,
    last_error TEXT,
    uploaded_at DATETIME,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (song_id) REFERENCES songs(id) ON DELETE RESTRICT,
    FOREIGN KEY (cos_storage_profile_id) REFERENCES cos_storage_profiles(id) ON DELETE RESTRICT,
    FOREIGN KEY (replaces_artifact_id) REFERENCES song_artifacts(id) ON DELETE RESTRICT
);

CREATE UNIQUE INDEX ux_song_artifacts_revision_kind ON song_artifacts(song_id, clip_revision, kind);
CREATE UNIQUE INDEX ux_song_artifacts_profile_key ON song_artifacts(cos_storage_profile_id, object_key);

CREATE TABLE media_cache_entries (
    id INTEGER PRIMARY KEY,
    kind TEXT NOT NULL,
    cache_key TEXT NOT NULL UNIQUE,
    song_id INTEGER,
    clip_revision INTEGER,
    relative_path TEXT NOT NULL,
    size_bytes INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL,
    last_accessed_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    grace_until DATETIME,
    lease_job_id INTEGER,
    lease_until DATETIME,
    last_error TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (song_id) REFERENCES songs(id) ON DELETE RESTRICT,
    FOREIGN KEY (lease_job_id) REFERENCES jobs(id) ON DELETE RESTRICT
);

CREATE INDEX ix_media_cache_reclaim ON media_cache_entries(kind, status, last_accessed_at);

CREATE TABLE storage_reservations (
    id INTEGER PRIMARY KEY,
    job_id INTEGER NOT NULL UNIQUE,
    kind TEXT NOT NULL,
    reserved_bytes INTEGER NOT NULL,
    heartbeat_at DATETIME NOT NULL,
    expires_at DATETIME NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (job_id) REFERENCES jobs(id) ON DELETE RESTRICT,
    CHECK (reserved_bytes > 0)
);

PRAGMA foreign_keys = ON;

-- +goose Down
SELECT 'forward-only migration; restore SQLite backup for rollback';
