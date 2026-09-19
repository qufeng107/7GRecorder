-- +goose Up
UPDATE recording_settings SET record_danmaku = 0;

CREATE TABLE live_analytics_configs (
    recording_profile_id INTEGER PRIMARY KEY,
    credential_id INTEGER,
    app_id INTEGER NOT NULL DEFAULT 0,
    enabled INTEGER NOT NULL DEFAULT 0,
    last_started_at DATETIME,
    last_error TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (recording_profile_id) REFERENCES recording_profiles(id) ON DELETE RESTRICT,
    FOREIGN KEY (credential_id) REFERENCES credentials(id) ON DELETE RESTRICT,
    CHECK (enabled IN (0, 1)),
    CHECK (app_id >= 0)
);

CREATE TABLE live_capture_sessions (
    id INTEGER PRIMARY KEY,
    recording_profile_id INTEGER NOT NULL,
    external_game_id TEXT,
    source TEXT NOT NULL DEFAULT 'BILIBILI_OPEN_LIVE',
    status TEXT NOT NULL,
    room_id TEXT,
    anchor_uid INTEGER,
    anchor_open_id TEXT,
    anchor_union_id TEXT,
    anchor_name TEXT,
    anchor_face_url TEXT,
    raw_relative_path TEXT UNIQUE,
    raw_status TEXT NOT NULL DEFAULT 'PENDING',
    raw_size_bytes INTEGER NOT NULL DEFAULT 0,
    raw_deleted_at DATETIME,
    started_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    connected_at DATETIME,
    ended_at DATETIME,
    last_event_at DATETIME,
    last_heartbeat_at DATETIME,
    event_count INTEGER NOT NULL DEFAULT 0,
    unknown_event_count INTEGER NOT NULL DEFAULT 0,
    gap_count INTEGER NOT NULL DEFAULT 0,
    event_counts_json TEXT NOT NULL DEFAULT '{}',
    last_error TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (recording_profile_id) REFERENCES recording_profiles(id) ON DELETE RESTRICT,
    CHECK (source = 'BILIBILI_OPEN_LIVE'),
    CHECK (status IN ('STARTING', 'CONNECTED', 'RECONNECTING', 'ENDED', 'FAILED', 'INTERRUPTED')),
    CHECK (raw_status IN ('PENDING', 'WRITING', 'AVAILABLE', 'DELETING', 'DELETED', 'MISSING')),
    CHECK (raw_size_bytes >= 0),
    CHECK (event_count >= 0 AND unknown_event_count >= 0 AND gap_count >= 0)
);

CREATE INDEX ix_live_capture_sessions_profile_started
    ON live_capture_sessions(recording_profile_id, started_at DESC);
CREATE UNIQUE INDEX ux_live_capture_sessions_profile_active
    ON live_capture_sessions(recording_profile_id)
    WHERE status IN ('STARTING', 'CONNECTED', 'RECONNECTING');

-- +goose Down
DROP INDEX IF EXISTS ux_live_capture_sessions_profile_active;
DROP INDEX IF EXISTS ix_live_capture_sessions_profile_started;
DROP TABLE IF EXISTS live_capture_sessions;
DROP TABLE IF EXISTS live_analytics_configs;
