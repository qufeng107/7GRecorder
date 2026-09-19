-- +goose Up
CREATE TABLE live_capture_minutes (
 session_id INTEGER NOT NULL REFERENCES live_capture_sessions(id) ON DELETE RESTRICT,
 minute TEXT NOT NULL,
 cmd TEXT NOT NULL,
 event_count INTEGER NOT NULL CHECK(event_count >= 0),
 PRIMARY KEY(session_id, minute, cmd)
);
-- +goose Down
DROP TABLE live_capture_minutes;
