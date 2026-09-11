-- +goose Up
PRAGMA foreign_keys = ON;

CREATE TRIGGER trg_bilibili_publication_review_gate_insert
BEFORE INSERT ON publications
FOR EACH ROW
WHEN NEW.platform = 'bilibili'
  AND NEW.status IN ('UPLOADING', 'VERIFYING', 'VERIFIED')
  AND EXISTS (
    SELECT 1
    FROM upload_sources us
    WHERE us.id = NEW.upload_source_id
      AND (us.review_status = 'REQUIRED' OR COALESCE(us.edit_decision_json, '') != '')
  )
BEGIN
  SELECT RAISE(ABORT, 'upload source is waiting for review');
END;

CREATE TRIGGER trg_bilibili_publication_review_gate_update
BEFORE UPDATE OF status ON publications
FOR EACH ROW
WHEN NEW.platform = 'bilibili'
  AND NEW.status IN ('UPLOADING', 'VERIFYING', 'VERIFIED')
  AND EXISTS (
    SELECT 1
    FROM upload_sources us
    WHERE us.id = NEW.upload_source_id
      AND (us.review_status = 'REQUIRED' OR COALESCE(us.edit_decision_json, '') != '')
  )
BEGIN
  SELECT RAISE(ABORT, 'upload source is waiting for review');
END;

CREATE TRIGGER trg_cos_upload_source_review_gate_insert
BEFORE INSERT ON upload_source_cos_objects
FOR EACH ROW
WHEN NEW.upload_source_output_id IS NOT NULL
  AND NEW.status IN ('UPLOADING', 'AVAILABLE')
  AND EXISTS (
    SELECT 1
    FROM upload_sources us
    WHERE us.id = NEW.upload_source_id
      AND (us.review_status = 'REQUIRED' OR COALESCE(us.edit_decision_json, '') != '')
  )
BEGIN
  SELECT RAISE(ABORT, 'upload source is waiting for review');
END;

CREATE TRIGGER trg_cos_upload_source_review_gate_update
BEFORE UPDATE OF status ON upload_source_cos_objects
FOR EACH ROW
WHEN NEW.upload_source_output_id IS NOT NULL
  AND NEW.status IN ('UPLOADING', 'AVAILABLE')
  AND EXISTS (
    SELECT 1
    FROM upload_sources us
    WHERE us.id = NEW.upload_source_id
      AND (us.review_status = 'REQUIRED' OR COALESCE(us.edit_decision_json, '') != '')
  )
BEGIN
  SELECT RAISE(ABORT, 'upload source is waiting for review');
END;

-- +goose Down
DROP TRIGGER IF EXISTS trg_cos_upload_source_review_gate_update;
DROP TRIGGER IF EXISTS trg_cos_upload_source_review_gate_insert;
DROP TRIGGER IF EXISTS trg_bilibili_publication_review_gate_update;
DROP TRIGGER IF EXISTS trg_bilibili_publication_review_gate_insert;
