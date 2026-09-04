-- +goose Up
PRAGMA foreign_keys = ON;
PRAGMA journal_mode = WAL;

CREATE TABLE users (
    id INTEGER PRIMARY KEY,
    username TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    role TEXT NOT NULL,
    enabled INTEGER NOT NULL DEFAULT 1,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE manager_policies (
    user_id INTEGER PRIMARY KEY,
    can_edit_recording_profile INTEGER NOT NULL DEFAULT 1,
    can_edit_bilibili_module INTEGER NOT NULL DEFAULT 1,
    can_edit_cos_module INTEGER NOT NULL DEFAULT 1,
    can_edit_netease_module INTEGER NOT NULL DEFAULT 1,
    can_manage_local_files INTEGER NOT NULL DEFAULT 1,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE RESTRICT
);

CREATE TABLE recording_profiles (
    id INTEGER PRIMARY KEY,
    owner_user_id INTEGER NOT NULL,
    name TEXT NOT NULL,
    platform TEXT NOT NULL DEFAULT 'bilibili',
    room_id TEXT NOT NULL,
    streamer_name TEXT NOT NULL,
    streamer_uid TEXT,
    timezone TEXT NOT NULL DEFAULT 'Asia/Shanghai',
    enabled INTEGER NOT NULL DEFAULT 1,
    public_enabled INTEGER NOT NULL DEFAULT 0,
    public_slug TEXT,
    archived_at DATETIME,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (owner_user_id) REFERENCES users(id) ON DELETE RESTRICT
);

CREATE UNIQUE INDEX ux_recording_profiles_active_room ON recording_profiles(platform, room_id) WHERE archived_at IS NULL;
CREATE UNIQUE INDEX ux_recording_profiles_public_slug ON recording_profiles(public_slug) WHERE public_slug IS NOT NULL;

CREATE TABLE recording_settings (
    recording_profile_id INTEGER PRIMARY KEY,
    auto_record INTEGER NOT NULL DEFAULT 1,
    quality TEXT NOT NULL DEFAULT 'original',
    record_danmaku INTEGER NOT NULL DEFAULT 1,
    segment_duration_sec INTEGER NOT NULL DEFAULT 1800,
    finalize_grace_period_sec INTEGER NOT NULL DEFAULT 300,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (recording_profile_id) REFERENCES recording_profiles(id) ON DELETE RESTRICT
);

CREATE TABLE recording_profile_runtime (
    recording_profile_id INTEGER PRIMARY KEY,
    stream_status TEXT NOT NULL DEFAULT 'UNKNOWN',
    recorder_status TEXT NOT NULL DEFAULT 'UNKNOWN',
    sync_status TEXT NOT NULL DEFAULT 'PENDING',
    current_recording_id INTEGER,
    last_event_at DATETIME,
    last_reconciled_at DATETIME,
    last_error TEXT,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (recording_profile_id) REFERENCES recording_profiles(id) ON DELETE RESTRICT
);

CREATE TABLE credentials (
    id INTEGER PRIMARY KEY,
    owner_user_id INTEGER,
    scope TEXT NOT NULL,
    platform TEXT NOT NULL,
    purpose TEXT NOT NULL,
    account_label TEXT NOT NULL,
    external_uid TEXT,
    encrypted_secret BLOB NOT NULL,
    secret_format_version INTEGER NOT NULL DEFAULT 1,
    status TEXT NOT NULL DEFAULT 'UNVERIFIED',
    last_verified_at DATETIME,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (owner_user_id) REFERENCES users(id) ON DELETE RESTRICT
);

CREATE TABLE publishing_profiles (
    id INTEGER PRIMARY KEY,
    recording_profile_id INTEGER NOT NULL,
    platform TEXT NOT NULL,
    credential_id INTEGER,
    enabled INTEGER NOT NULL DEFAULT 0,
    settings_json TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (recording_profile_id) REFERENCES recording_profiles(id) ON DELETE RESTRICT,
    FOREIGN KEY (credential_id) REFERENCES credentials(id) ON DELETE RESTRICT
);

CREATE UNIQUE INDEX ux_publishing_profiles_profile_platform ON publishing_profiles(recording_profile_id, platform);

CREATE TABLE cos_storage_profiles (
    id INTEGER PRIMARY KEY,
    recording_profile_id INTEGER NOT NULL UNIQUE,
    credential_id INTEGER NOT NULL,
    enabled INTEGER NOT NULL DEFAULT 0,
    region TEXT NOT NULL,
    bucket TEXT NOT NULL,
    prefix TEXT NOT NULL,
    max_managed_bytes INTEGER NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (recording_profile_id) REFERENCES recording_profiles(id) ON DELETE RESTRICT,
    FOREIGN KEY (credential_id) REFERENCES credentials(id) ON DELETE RESTRICT
);

CREATE TABLE local_storage_settings (
    id INTEGER PRIMARY KEY,
    max_recording_bytes INTEGER NOT NULL,
    min_system_free_bytes INTEGER NOT NULL,
    cleanup_target_ratio REAL NOT NULL,
    absolute_emergency_free_bytes INTEGER NOT NULL,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_by_user_id INTEGER NOT NULL,
    FOREIGN KEY (updated_by_user_id) REFERENCES users(id) ON DELETE RESTRICT
);

CREATE TABLE recordings (
    id INTEGER PRIMARY KEY,
    recording_profile_id INTEGER NOT NULL,
    title TEXT,
    started_at DATETIME NOT NULL,
    ended_at DATETIME,
    completed_at DATETIME,
    finalize_at DATETIME,
    duration_ms INTEGER,
    recording_status TEXT NOT NULL,
    song_processing_status TEXT NOT NULL DEFAULT 'DISABLED',
    local_storage_status TEXT NOT NULL DEFAULT 'AVAILABLE',
    local_protected INTEGER NOT NULL DEFAULT 0,
    local_deleted_at DATETIME,
    source_room_id TEXT NOT NULL,
    streamer_name_snapshot TEXT NOT NULL,
    recording_config_snapshot_json TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (recording_profile_id) REFERENCES recording_profiles(id) ON DELETE RESTRICT
);

CREATE INDEX ix_recordings_profile_started ON recordings(recording_profile_id, started_at);
CREATE INDEX ix_recordings_status_finalize ON recordings(recording_status, finalize_at);
CREATE INDEX ix_recordings_completed ON recordings(completed_at);

CREATE TABLE recorder_sessions (
    id INTEGER PRIMARY KEY,
    recording_id INTEGER NOT NULL,
    external_session_id TEXT,
    started_at DATETIME NOT NULL,
    ended_at DATETIME,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (recording_id) REFERENCES recordings(id) ON DELETE RESTRICT
);

CREATE TABLE recording_files (
    id INTEGER PRIMARY KEY,
    recording_id INTEGER NOT NULL,
    recorder_session_id INTEGER,
    relative_path TEXT NOT NULL UNIQUE,
    original_name TEXT NOT NULL,
    kind TEXT NOT NULL,
    file_status TEXT NOT NULL,
    segment_index INTEGER,
    size_bytes INTEGER,
    duration_ms INTEGER,
    checksum TEXT,
    opened_at DATETIME,
    closed_at DATETIME,
    deleted_at DATETIME,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (recording_id) REFERENCES recordings(id) ON DELETE RESTRICT,
    FOREIGN KEY (recorder_session_id) REFERENCES recorder_sessions(id) ON DELETE RESTRICT
);

CREATE INDEX ix_recording_files_recording ON recording_files(recording_id);
CREATE INDEX ix_recording_files_status_closed ON recording_files(file_status, closed_at);

CREATE TABLE recorder_events (
    id INTEGER PRIMARY KEY,
    event_id TEXT NOT NULL UNIQUE,
    event_type TEXT NOT NULL,
    room_id TEXT NOT NULL,
    recording_profile_id INTEGER,
    payload_json TEXT NOT NULL,
    event_at DATETIME NOT NULL,
    processed_at DATETIME,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (recording_profile_id) REFERENCES recording_profiles(id) ON DELETE RESTRICT
);

CREATE TABLE publications (
    id INTEGER PRIMARY KEY,
    recording_profile_id INTEGER NOT NULL,
    recording_id INTEGER,
    song_id INTEGER,
    platform TEXT NOT NULL,
    credential_id INTEGER,
    external_id TEXT,
    external_url TEXT,
    status TEXT NOT NULL,
    attempts INTEGER NOT NULL DEFAULT 0,
    last_error TEXT,
    request_snapshot_json TEXT,
    published_at DATETIME,
    verified_at DATETIME,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (recording_profile_id) REFERENCES recording_profiles(id) ON DELETE RESTRICT,
    FOREIGN KEY (recording_id) REFERENCES recordings(id) ON DELETE RESTRICT,
    FOREIGN KEY (credential_id) REFERENCES credentials(id) ON DELETE RESTRICT
);

CREATE UNIQUE INDEX ux_publications_recording_platform ON publications(recording_id, platform) WHERE recording_id IS NOT NULL;
CREATE INDEX ix_publications_profile_platform_status ON publications(recording_profile_id, platform, status);

CREATE TABLE cos_objects (
    id INTEGER PRIMARY KEY,
    cos_storage_profile_id INTEGER NOT NULL,
    recording_profile_id INTEGER NOT NULL,
    recording_id INTEGER NOT NULL,
    recording_file_id INTEGER NOT NULL,
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
    FOREIGN KEY (recording_id) REFERENCES recordings(id) ON DELETE RESTRICT,
    FOREIGN KEY (recording_file_id) REFERENCES recording_files(id) ON DELETE RESTRICT
);

CREATE UNIQUE INDEX ux_cos_objects_profile_file ON cos_objects(cos_storage_profile_id, recording_file_id);
CREATE UNIQUE INDEX ux_cos_objects_profile_key ON cos_objects(cos_storage_profile_id, object_key);
CREATE INDEX ix_cos_objects_profile_status_uploaded ON cos_objects(cos_storage_profile_id, status, uploaded_at);
CREATE INDEX ix_cos_objects_recording ON cos_objects(recording_id);

CREATE TABLE songs (
    id INTEGER PRIMARY KEY,
    recording_profile_id INTEGER NOT NULL,
    recording_id INTEGER NOT NULL,
    title TEXT,
    artist TEXT,
    start_ms INTEGER NOT NULL,
    end_ms INTEGER NOT NULL,
    confidence REAL,
    status TEXT NOT NULL,
    local_audio_status TEXT NOT NULL DEFAULT 'NONE',
    audio_relative_path TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (recording_profile_id) REFERENCES recording_profiles(id) ON DELETE RESTRICT,
    FOREIGN KEY (recording_id) REFERENCES recordings(id) ON DELETE RESTRICT
);

CREATE INDEX ix_songs_recording_start ON songs(recording_id, start_ms);

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

CREATE TABLE jobs (
    id INTEGER PRIMARY KEY,
    recording_profile_id INTEGER,
    recording_id INTEGER,
    recording_file_id INTEGER,
    song_id INTEGER,
    publication_id INTEGER,
    cos_object_id INTEGER,
    type TEXT NOT NULL,
    resource_class TEXT NOT NULL,
    business_key TEXT,
    payload_json TEXT,
    status TEXT NOT NULL,
    priority INTEGER NOT NULL DEFAULT 100,
    attempts INTEGER NOT NULL DEFAULT 0,
    max_attempts INTEGER NOT NULL DEFAULT 1,
    run_after DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    locked_at DATETIME,
    heartbeat_at DATETIME,
    locked_by TEXT,
    last_error_class TEXT,
    last_error TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (recording_profile_id) REFERENCES recording_profiles(id) ON DELETE RESTRICT,
    FOREIGN KEY (recording_id) REFERENCES recordings(id) ON DELETE RESTRICT,
    FOREIGN KEY (recording_file_id) REFERENCES recording_files(id) ON DELETE RESTRICT,
    FOREIGN KEY (song_id) REFERENCES songs(id) ON DELETE RESTRICT,
    FOREIGN KEY (publication_id) REFERENCES publications(id) ON DELETE RESTRICT,
    FOREIGN KEY (cos_object_id) REFERENCES cos_objects(id) ON DELETE RESTRICT
);

CREATE UNIQUE INDEX ux_jobs_business_key ON jobs(business_key) WHERE business_key IS NOT NULL;
CREATE INDEX ix_jobs_status_run_after_priority ON jobs(status, run_after, priority);
CREATE INDEX ix_jobs_recording_status ON jobs(recording_id, status);
CREATE INDEX ix_jobs_recording_file_status ON jobs(recording_file_id, status);

CREATE TABLE sessions (
    id INTEGER PRIMARY KEY,
    user_id INTEGER NOT NULL,
    token_digest TEXT NOT NULL UNIQUE,
    expires_at DATETIME NOT NULL,
    last_seen_at DATETIME NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE RESTRICT
);

CREATE TABLE audit_logs (
    id INTEGER PRIMARY KEY,
    actor_user_id INTEGER,
    action TEXT NOT NULL,
    resource_type TEXT NOT NULL,
    resource_id INTEGER,
    summary TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (actor_user_id) REFERENCES users(id) ON DELETE RESTRICT
);

CREATE TABLE system_settings (
    id INTEGER PRIMARY KEY,
    key TEXT NOT NULL UNIQUE,
    value_json TEXT NOT NULL,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- +goose Down
SELECT 'forward-only migration; restore SQLite backup for rollback';
