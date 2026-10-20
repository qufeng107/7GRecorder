package upload

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path"
	"strings"
	"time"
)

// COSDeleter only receives database-registered keys selected by quota policy.
type COSDeleter interface {
	Delete(context.Context, COSUploadRequest) error
}

func managedKey(prefix, key string) bool {
	return prefix != "" && strings.HasSuffix(prefix, "/") && strings.HasPrefix(key, prefix) && key != prefix && path.Clean(key) == key && !strings.Contains(key, "\\")
}

// AdmitCOSUpload persists an actual-byte reservation atomically before any PUT.
// Failure/recovery keeps it until the remote result is confirmed.
func (s Store) AdmitCOSUpload(ctx context.Context, request COSUploadRequest, raw bool, deleter COSDeleter) error {
	info, err := os.Stat(request.SourcePath)
	if err != nil || info.IsDir() {
		return NewClassifiedError("SOURCE_MISSING", "cos upload source is missing")
	}
	table := "upload_source_cos_objects"
	if raw {
		table = "cos_objects"
	}
	return s.ensureCOSCapacity(ctx, request, table, info.Size(), deleter)
}

func (s Store) ensureCOSCapacity(ctx context.Context, request COSUploadRequest, table string, incoming int64, deleter COSDeleter) error {
	for {
		done, victim, err := s.cosCapacityStep(ctx, request, table, incoming)
		if err != nil || done {
			return err
		}
		if deleter == nil {
			return NewClassifiedError("TRANSIENT", "COS quota requires a delete adapter")
		}
		deletion := request
		deletion.ObjectKey = victim.key
		deleteCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		err = deleter.Delete(deleteCtx, deletion)
		cancel()
		if err != nil {
			return fmt.Errorf("COS quota cleanup: %w", err)
		}
		// DELETING is durable: a crash after remote success retries the idempotent DELETE.
		if _, err = s.db.ExecContext(ctx, `UPDATE `+victim.table+` SET status='DELETED', quota_reserved_bytes=0, deleted_at=CURRENT_TIMESTAMP,
   updated_at=CURRENT_TIMESTAMP WHERE id=? AND status='DELETING'`, victim.id); err != nil {
			return err
		}
	}
}

type cosVictim struct {
	table string
	id    int64
	key   string
}

func (s Store) cosCapacityStep(ctx context.Context, request COSUploadRequest, table string, incoming int64) (bool, cosVictim, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, cosVictim{}, err
	}
	defer tx.Rollback()
	// Acquire the SQLite write lock before reading usage; no network I/O in this transaction.
	if _, err = tx.ExecContext(ctx, `UPDATE cos_storage_profiles SET updated_at=updated_at WHERE recording_profile_id=?`, request.RecordingProfileID); err != nil {
		return false, cosVictim{}, err
	}
	var profileID, limit int64
	var region, bucket, prefix string
	err = tx.QueryRowContext(ctx, `SELECT id,max_managed_bytes,region,bucket,prefix FROM cos_storage_profiles
  WHERE recording_profile_id=? AND enabled=1`, request.RecordingProfileID).Scan(&profileID, &limit, &region, &bucket, &prefix)
	if err != nil {
		return false, cosVictim{}, err
	}
	if region != request.Region || bucket != request.Bucket || prefix != request.Prefix || incoming < 0 || limit <= 0 {
		return false, cosVictim{}, NewClassifiedError("PERMANENT", "COS configuration changed or quota is invalid")
	}
	if incoming > limit {
		return false, cosVictim{}, NewClassifiedError("PERMANENT", "COS object exceeds max_managed_bytes")
	}
	if table != "" {
		if table != "song_artifacts" && !managedKey(prefix, request.ObjectKey) {
			return false, cosVictim{}, NewClassifiedError("PERMANENT", "COS object is outside managed prefix")
		}
		allowed := "('PENDING','FAILED')"
		if table == "song_artifacts" {
			allowed = "('GENERATING','FAILED','UPLOADING')"
		}
		var count int
		err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+table+` WHERE id=? AND cos_storage_profile_id=? AND object_key=? AND status IN `+allowed, request.ObjectID, profileID, request.ObjectKey).Scan(&count)
		if err != nil {
			return false, cosVictim{}, err
		}
		if count != 1 {
			return false, cosVictim{}, ErrNotReady
		}
	}
	var usage int64
	err = tx.QueryRowContext(ctx, `SELECT
  (SELECT COALESCE(SUM(CASE WHEN status IN ('AVAILABLE','DELETING') THEN MAX(size_bytes,quota_reserved_bytes) ELSE quota_reserved_bytes END),0) FROM upload_source_cos_objects WHERE cos_storage_profile_id=? AND status!='DELETED') +
  (SELECT COALESCE(SUM(CASE WHEN status IN ('AVAILABLE','DELETING') THEN MAX(size_bytes,quota_reserved_bytes) ELSE quota_reserved_bytes END),0) FROM cos_objects WHERE cos_storage_profile_id=? AND status!='DELETED') +
  (SELECT COALESCE(SUM(size_bytes),0) FROM song_artifacts WHERE cos_storage_profile_id=? AND status != 'DELETED')`, profileID, profileID, profileID).Scan(&usage)
	if err != nil {
		return false, cosVictim{}, err
	}
	if table != "" {
		column := "quota_reserved_bytes"
		if table == "song_artifacts" {
			column = "size_bytes"
		}
		var previous int64
		if err = tx.QueryRowContext(ctx, `SELECT `+column+` FROM `+table+` WHERE id=?`, request.ObjectID).Scan(&previous); err != nil {
			return false, cosVictim{}, err
		}
		usage -= previous
		// Until the replacement PUT is confirmed, the previous remote object may still be larger.
		incoming = max(incoming, previous)
	}
	// Always finish persisted delete intents, even if quota has since increased.
	victim := cosVictim{}
	err = tx.QueryRowContext(ctx, `SELECT tbl,id,object_key FROM (
  SELECT 'upload_source_cos_objects' tbl,id,object_key FROM upload_source_cos_objects WHERE cos_storage_profile_id=? AND status='DELETING'
  UNION ALL SELECT 'cos_objects',id,object_key FROM cos_objects WHERE cos_storage_profile_id=? AND status='DELETING') ORDER BY tbl,id LIMIT 1`, profileID, profileID).Scan(&victim.table, &victim.id, &victim.key)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, victim, err
	}
	if errors.Is(err, sql.ErrNoRows) && usage <= limit-incoming {
		if table != "" {
			column := "quota_reserved_bytes"
			if table == "song_artifacts" {
				column = "size_bytes"
			}
			_, err = tx.ExecContext(ctx, `UPDATE `+table+` SET status='UPLOADING',`+column+`=?,last_error=NULL,updated_at=CURRENT_TIMESTAMP WHERE id=?`, incoming, request.ObjectID)
			if err != nil {
				return false, victim, err
			}
		}
		return true, victim, tx.Commit()
	}
	if errors.Is(err, sql.ErrNoRows) {
		err = tx.QueryRowContext(ctx, `SELECT tbl,id,object_key FROM (
   SELECT 'upload_source_cos_objects' tbl,co.id,co.object_key,us.started_at
   FROM upload_source_cos_objects co JOIN upload_sources us ON us.id=co.upload_source_id
   WHERE co.cos_storage_profile_id=? AND co.status='AVAILABLE' AND us.id != ?
    AND us.status IN ('READY_TO_UPLOAD','REPLACED') AND us.review_status != 'REQUIRED'
    AND COALESCE(us.edit_decision_json,'')=''
    AND EXISTS (SELECT 1 FROM upload_source_segments seg WHERE seg.upload_source_id=us.id)
    AND NOT EXISTS (SELECT 1 FROM upload_source_segments seg JOIN recordings r ON r.id=seg.recording_id
     WHERE seg.upload_source_id=us.id AND (r.recording_status!='COMPLETED' OR r.local_protected=1 OR r.upload_review_status='REQUIRED'))
    AND NOT EXISTS (SELECT 1 FROM upload_source_segments seg JOIN recording_files rf ON rf.recording_id=seg.recording_id WHERE seg.upload_source_id=us.id AND rf.file_status='WRITING')
    AND NOT EXISTS (SELECT 1 FROM song_analysis_runs run WHERE run.upload_source_id=us.id
     AND run.status NOT IN ('COMPLETED','CANCELLED','REVIEW_REQUIRED'))
    AND NOT EXISTS (SELECT 1 FROM upload_source_cos_objects other WHERE other.upload_source_id=us.id AND other.status='UPLOADING')
   UNION ALL
   SELECT 'cos_objects',co.id,co.object_key,r.started_at FROM cos_objects co JOIN recordings r ON r.id=co.recording_id
   WHERE co.cos_storage_profile_id=? AND co.status='AVAILABLE' AND r.recording_status='COMPLETED' AND r.local_protected=0 AND r.upload_review_status!='REQUIRED'
    AND NOT EXISTS (SELECT 1 FROM recording_files rf WHERE rf.recording_id=r.id AND rf.file_status='WRITING')
    AND NOT EXISTS (SELECT 1 FROM upload_source_segments seg WHERE seg.recording_id=r.id AND seg.upload_source_id=?)
  ) WHERE substr(object_key,1,length(?))=? ORDER BY started_at,tbl,id LIMIT 1`, profileID, request.UploadSourceID, profileID, request.UploadSourceID, prefix, prefix).Scan(&victim.table, &victim.id, &victim.key)
		if errors.Is(err, sql.ErrNoRows) {
			return false, victim, NewClassifiedError("TRANSIENT", "COS quota exhausted: no safely reclaimable objects")
		}
		if err != nil {
			return false, victim, err
		}
	}
	if !managedKey(prefix, victim.key) {
		return false, victim, NewClassifiedError("PERMANENT", "COS cleanup key is outside managed prefix")
	}
	if _, err = tx.ExecContext(ctx, `UPDATE `+victim.table+` SET status='DELETING',updated_at=CURRENT_TIMESTAMP WHERE id=?`, victim.id); err != nil {
		return false, victim, err
	}
	return false, victim, tx.Commit()
}

// ReconcileCOSQuota runs independently of uploads and continues after a profile failure.
func (s Store) ReconcileCOSQuota(ctx context.Context, deleter COSDeleter) error {
	rows, err := s.db.QueryContext(ctx, `SELECT csp.recording_profile_id,csp.region,csp.bucket,csp.prefix,c.encrypted_secret
  FROM cos_storage_profiles csp JOIN credentials c ON c.id=csp.credential_id WHERE csp.enabled=1 ORDER BY csp.id`)
	if err != nil {
		return err
	}
	var requests []COSUploadRequest
	var failures []error
	for rows.Next() {
		var req COSUploadRequest
		var encrypted []byte
		if err = rows.Scan(&req.RecordingProfileID, &req.Region, &req.Bucket, &req.Prefix, &encrypted); err != nil {
			rows.Close()
			return err
		}
		req.Secret, err = s.decryptCOSSecret(encrypted)
		if err != nil {
			failures = append(failures, err)
			continue
		}
		requests = append(requests, req)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, req := range requests {
		if err = s.ensureCOSCapacity(ctx, req, "", 0, deleter); err != nil {
			failures = append(failures, fmt.Errorf("COS profile %d: %w", req.RecordingProfileID, err))
		}
	}
	return errors.Join(failures...)
}

// AdmitCOSArtifact applies the same profile budget to independent Songs uploads.
func (s Store) AdmitCOSArtifact(ctx context.Context, request COSUploadRequest, profileID int64, deleter COSDeleter) error {
	var id int64
	err := s.db.QueryRowContext(ctx, `SELECT a.id,csp.recording_profile_id,csp.prefix FROM song_artifacts a
 JOIN cos_storage_profiles csp ON csp.id=a.cos_storage_profile_id
 WHERE a.cos_storage_profile_id=? AND a.object_key=? AND csp.enabled=1`, profileID, request.ObjectKey).Scan(&id, &request.RecordingProfileID, &request.Prefix)
	if err != nil {
		return err
	}
	request.ObjectID = id
	info, err := os.Stat(request.SourcePath)
	if err != nil || info.IsDir() {
		return NewClassifiedError("SOURCE_MISSING", "song artifact source is missing")
	}
	return s.ensureCOSCapacity(ctx, request, "song_artifacts", info.Size(), deleter)
}
