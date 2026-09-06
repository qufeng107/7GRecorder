package worker

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/7grecorder/7grecorder/backend/internal/config"
	"github.com/7grecorder/7grecorder/backend/internal/media"
	"github.com/7grecorder/7grecorder/backend/internal/recorder"
	"github.com/7grecorder/7grecorder/backend/internal/recording"
)

type Worker struct {
	db       *sql.DB
	recorder recorder.SyncClient
	cfg      config.Config
	merger   media.Merger
	lockID   string
}

type job struct {
	ID                 int64
	Type               string
	RecordingProfileID int64
	PayloadJSON        string
	Attempts           int
	MaxAttempts        int
}

type mergeJobPayload struct {
	UploadSourceID int64 `json:"upload_source_id"`
}

func New(database *sql.DB, recorderClient recorder.SyncClient, cfgs ...config.Config) Worker {
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "7grecorder"
	}
	cfg := config.Config{}
	if len(cfgs) > 0 {
		cfg = cfgs[0]
	}
	return Worker{
		db:       database,
		recorder: recorderClient,
		cfg:      cfg,
		merger:   media.NewFFmpegMerger(cfg.DataRoot, cfg.TempRoot, cfg.FFmpegPath),
		lockID:   fmt.Sprintf("%s:%d", host, os.Getpid()),
	}
}

func NewWithMerger(database *sql.DB, recorderClient recorder.SyncClient, cfg config.Config, merger media.Merger) Worker {
	worker := New(database, recorderClient, cfg)
	worker.merger = merger
	return worker
}

func (w Worker) Run(ctx context.Context) {
	if err := w.RunOnce(ctx); err != nil && !errors.Is(err, sql.ErrNoRows) {
		log.Printf("worker run once failed: %v", err)
	}
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := w.RunOnce(ctx); err != nil && !errors.Is(err, sql.ErrNoRows) {
				log.Printf("worker run once failed: %v", err)
			}
		}
	}
}

func (w Worker) RunOnce(ctx context.Context) error {
	job, err := w.claimJob(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return w.discoverUploadSources(ctx)
		}
		return err
	}

	switch job.Type {
	case "SYNC_RECORDER_PROFILE":
		return w.runSyncJob(ctx, job)
	case "MERGE_UPLOAD_SOURCE":
		return w.runMergeJob(ctx, job)
	default:
		return w.failJob(ctx, job, "PERMANENT", fmt.Errorf("unknown job type %q", job.Type))
	}
}

func (w Worker) runSyncJob(ctx context.Context, job job) error {
	var desired recorder.DesiredProfile
	if err := json.Unmarshal([]byte(job.PayloadJSON), &desired); err != nil {
		return w.failJob(ctx, job, "PERMANENT", fmt.Errorf("decode sync payload: %w", err))
	}
	status, err := w.recorder.SyncProfile(ctx, desired)
	if err != nil {
		return w.failJob(ctx, job, "TRANSIENT", err)
	}
	return w.succeedJob(ctx, job, status)
}

func (w Worker) runMergeJob(ctx context.Context, job job) error {
	var payload mergeJobPayload
	if err := json.Unmarshal([]byte(job.PayloadJSON), &payload); err != nil {
		return w.failJob(ctx, job, "PERMANENT", fmt.Errorf("decode merge payload: %w", err))
	}
	store := recording.NewStore(w.db, w.cfg)
	source, err := store.UploadSourceForMerge(ctx, payload.UploadSourceID)
	if err != nil {
		return w.failJob(ctx, job, "PERMANENT", err)
	}
	if source.Status == "READY_TO_UPLOAD" {
		return w.succeedJob(ctx, job, recorder.RuntimeStatus{})
	}
	if source.Status != "MERGE_PENDING" && source.Status != "MERGE_FAILED" {
		return w.failJob(ctx, job, "PERMANENT", fmt.Errorf("upload source is not merge pending: %s", source.Status))
	}
	segments := make([]media.Segment, 0, len(source.Segments))
	for _, segment := range source.Segments {
		segments = append(segments, media.Segment{RelativePath: segment.RelativePath})
	}
	outputRelativePath := filepath.ToSlash(filepath.Join("upload-sources", fmt.Sprintf("%d", source.RecordingProfileID), fmt.Sprintf("%d", source.ID), fmt.Sprintf("upload-source-%d.flv", source.ID)))
	result, err := w.merger.Merge(ctx, media.MergeRequest{
		UploadSourceID:     source.ID,
		Segments:           segments,
		OutputRelativePath: outputRelativePath,
	})
	if err != nil {
		terminal := job.Attempts >= job.MaxAttempts
		message := truncateError(err)
		if markErr := store.MarkUploadSourceMergeFailed(ctx, source.ID, terminal, message); markErr != nil {
			return markErr
		}
		return w.failJob(ctx, job, "TRANSIENT", err)
	}
	if err := store.MarkUploadSourceMergeSucceeded(ctx, source.ID, result.RelativePath, result.SizeBytes); err != nil {
		return w.failJob(ctx, job, "PERMANENT", err)
	}
	return w.succeedJob(ctx, job, recorder.RuntimeStatus{})
}

func (w Worker) discoverUploadSources(ctx context.Context) error {
	_, err := recording.NewStore(w.db, w.cfg).DiscoverUploadSources(ctx, recording.DefaultMergeGapThresholdSeconds)
	return err
}

func (w Worker) claimJob(ctx context.Context) (job, error) {
	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		return job{}, fmt.Errorf("begin job claim: %w", err)
	}
	defer tx.Rollback()

	var job job
	err = tx.QueryRowContext(ctx, `
		SELECT id, type, COALESCE(recording_profile_id, 0), COALESCE(payload_json, ''), attempts, max_attempts
		FROM jobs
		WHERE type IN ('SYNC_RECORDER_PROFILE', 'MERGE_UPLOAD_SOURCE')
			AND status = 'PENDING'
			AND run_after <= CURRENT_TIMESTAMP
		ORDER BY priority ASC, run_after ASC, id ASC
		LIMIT 1
	`).Scan(&job.ID, &job.Type, &job.RecordingProfileID, &job.PayloadJSON, &job.Attempts, &job.MaxAttempts)
	if err != nil {
		return job{}, err
	}

	result, err := tx.ExecContext(ctx, `
		UPDATE jobs
		SET status = 'RUNNING',
			attempts = attempts + 1,
			locked_at = CURRENT_TIMESTAMP,
			heartbeat_at = CURRENT_TIMESTAMP,
			locked_by = ?,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND status = 'PENDING'
	`, w.lockID, job.ID)
	if err != nil {
		return job{}, fmt.Errorf("claim job: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return job{}, fmt.Errorf("read claim rows affected: %w", err)
	}
	if changed != 1 {
		return job{}, sql.ErrNoRows
	}
	if err := tx.Commit(); err != nil {
		return job{}, fmt.Errorf("commit job claim: %w", err)
	}
	job.Attempts++
	return job, nil
}

func (w Worker) succeedJob(ctx context.Context, job job, status recorder.RuntimeStatus) error {
	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin job success: %w", err)
	}
	defer tx.Rollback()

	if job.Type == "SYNC_RECORDER_PROFILE" {
		if _, err := tx.ExecContext(ctx, `
			UPDATE recording_profile_runtime
			SET stream_status = ?,
				recorder_status = ?,
				sync_status = 'SYNCED',
				last_reconciled_at = CURRENT_TIMESTAMP,
				last_error = NULL,
				updated_at = CURRENT_TIMESTAMP
			WHERE recording_profile_id = ?
		`, status.StreamStatus, status.RecorderStatus, job.RecordingProfileID); err != nil {
			return fmt.Errorf("update synced runtime: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE jobs
		SET status = 'SUCCEEDED',
			locked_at = NULL,
			heartbeat_at = NULL,
			locked_by = NULL,
			last_error_class = NULL,
			last_error = NULL,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, job.ID); err != nil {
		return fmt.Errorf("mark job succeeded: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit job success: %w", err)
	}
	return nil
}

func (w Worker) failJob(ctx context.Context, job job, errorClass string, cause error) error {
	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin job failure: %w", err)
	}
	defer tx.Rollback()

	nextStatus := "PENDING"
	runAfter := "CURRENT_TIMESTAMP"
	if job.Attempts >= job.MaxAttempts || errorClass == "PERMANENT" {
		nextStatus = "FAILED"
	} else {
		runAfter = fmt.Sprintf("datetime('now', '+%d seconds')", retryDelaySeconds(job.Attempts))
	}
	message := truncateError(cause)
	if job.Type == "SYNC_RECORDER_PROFILE" {
		if _, err := tx.ExecContext(ctx, `
			UPDATE recording_profile_runtime
			SET sync_status = 'ERROR',
				recorder_status = 'ERROR',
				last_reconciled_at = CURRENT_TIMESTAMP,
				last_error = ?,
				updated_at = CURRENT_TIMESTAMP
			WHERE recording_profile_id = ?
		`, message, job.RecordingProfileID); err != nil {
			return fmt.Errorf("update failed runtime: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, fmt.Sprintf(`
		UPDATE jobs
		SET status = ?,
			run_after = %s,
			locked_at = NULL,
			heartbeat_at = NULL,
			locked_by = NULL,
			last_error_class = ?,
			last_error = ?,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, runAfter), nextStatus, errorClass, message, job.ID); err != nil {
		return fmt.Errorf("mark job failed: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit job failure: %w", err)
	}
	return nil
}

func truncateError(err error) string {
	if err == nil {
		return ""
	}
	message := strings.TrimSpace(err.Error())
	if len(message) > 1000 {
		return message[:1000]
	}
	return message
}

func retryDelaySeconds(attempts int) int {
	switch attempts {
	case 0, 1:
		return 5
	case 2:
		return 30
	case 3:
		return 60
	case 4:
		return 300
	default:
		return 900
	}
}
