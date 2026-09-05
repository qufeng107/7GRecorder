package worker

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/7grecorder/7grecorder/backend/internal/recorder"
)

type Worker struct {
	db       *sql.DB
	recorder recorder.SyncClient
	lockID   string
}

type syncJob struct {
	ID                 int64
	RecordingProfileID int64
	PayloadJSON        string
	Attempts           int
	MaxAttempts        int
}

func New(database *sql.DB, recorderClient recorder.SyncClient) Worker {
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "7grecorder"
	}
	return Worker{
		db:       database,
		recorder: recorderClient,
		lockID:   fmt.Sprintf("%s:%d", host, os.Getpid()),
	}
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
	job, err := w.claimSyncJob(ctx)
	if err != nil {
		return err
	}

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

func (w Worker) claimSyncJob(ctx context.Context) (syncJob, error) {
	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		return syncJob{}, fmt.Errorf("begin sync job claim: %w", err)
	}
	defer tx.Rollback()

	var job syncJob
	err = tx.QueryRowContext(ctx, `
		SELECT id, recording_profile_id, COALESCE(payload_json, ''), attempts, max_attempts
		FROM jobs
		WHERE type = 'SYNC_RECORDER_PROFILE'
			AND status = 'PENDING'
			AND run_after <= CURRENT_TIMESTAMP
		ORDER BY priority ASC, run_after ASC, id ASC
		LIMIT 1
	`).Scan(&job.ID, &job.RecordingProfileID, &job.PayloadJSON, &job.Attempts, &job.MaxAttempts)
	if err != nil {
		return syncJob{}, err
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
		return syncJob{}, fmt.Errorf("claim sync job: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return syncJob{}, fmt.Errorf("read claim rows affected: %w", err)
	}
	if changed != 1 {
		return syncJob{}, sql.ErrNoRows
	}
	if err := tx.Commit(); err != nil {
		return syncJob{}, fmt.Errorf("commit sync job claim: %w", err)
	}
	job.Attempts++
	return job, nil
}

func (w Worker) succeedJob(ctx context.Context, job syncJob, status recorder.RuntimeStatus) error {
	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin sync job success: %w", err)
	}
	defer tx.Rollback()

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
		return fmt.Errorf("mark sync job succeeded: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit sync job success: %w", err)
	}
	return nil
}

func (w Worker) failJob(ctx context.Context, job syncJob, errorClass string, cause error) error {
	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin sync job failure: %w", err)
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
		return fmt.Errorf("mark sync job failed: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit sync job failure: %w", err)
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
