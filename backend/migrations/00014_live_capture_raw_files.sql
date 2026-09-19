-- +goose Up
CREATE TABLE live_capture_raw_files (
    id INTEGER PRIMARY KEY,
    session_id INTEGER NOT NULL REFERENCES live_capture_sessions(id) ON DELETE RESTRICT,
    relative_path TEXT NOT NULL UNIQUE,
    status TEXT NOT NULL CHECK(status IN ('WRITING','AVAILABLE','DELETING','DELETED','MISSING')),
    size_bytes INTEGER NOT NULL DEFAULT 0 CHECK(size_bytes >= 0),
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    closed_at DATETIME,
    deleted_at DATETIME
);
CREATE UNIQUE INDEX ux_live_capture_raw_writing ON live_capture_raw_files(session_id) WHERE status = 'WRITING';
CREATE INDEX ix_live_capture_raw_session ON live_capture_raw_files(session_id, id);
CREATE INDEX ix_live_capture_raw_cleanup ON live_capture_raw_files(status, id);
INSERT INTO live_capture_raw_files(session_id,relative_path,status,size_bytes,created_at,closed_at,deleted_at)
SELECT id,raw_relative_path,CASE WHEN raw_status = 'PENDING' THEN 'MISSING' ELSE raw_status END,
    raw_size_bytes,started_at,ended_at,raw_deleted_at
FROM live_capture_sessions WHERE raw_relative_path IS NOT NULL;
-- +goose Down
DROP TABLE live_capture_raw_files;
