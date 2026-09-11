-- +goose Up
PRAGMA foreign_keys = ON;

ALTER TABLE recordings ADD COLUMN upload_review_status TEXT NOT NULL DEFAULT 'NONE';
ALTER TABLE recordings ADD COLUMN upload_review_requested_at DATETIME;
ALTER TABLE recordings ADD COLUMN upload_review_completed_at DATETIME;
ALTER TABLE recordings ADD COLUMN upload_review_notes TEXT;

ALTER TABLE upload_sources ADD COLUMN review_status TEXT NOT NULL DEFAULT 'NONE';
ALTER TABLE upload_sources ADD COLUMN review_requested_at DATETIME;
ALTER TABLE upload_sources ADD COLUMN review_completed_at DATETIME;
ALTER TABLE upload_sources ADD COLUMN review_notes TEXT;
ALTER TABLE upload_sources ADD COLUMN edit_decision_json TEXT;

CREATE INDEX ix_recordings_upload_review_status ON recordings(upload_review_status);
CREATE INDEX ix_upload_sources_review_status ON upload_sources(review_status);

-- +goose Down
SELECT 'forward-only migration; restore SQLite backup for rollback';
