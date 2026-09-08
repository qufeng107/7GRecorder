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

	"github.com/7grecorder/7grecorder/backend/internal/account"
	"github.com/7grecorder/7grecorder/backend/internal/config"
	"github.com/7grecorder/7grecorder/backend/internal/media"
	"github.com/7grecorder/7grecorder/backend/internal/recorder"
	"github.com/7grecorder/7grecorder/backend/internal/recording"
	"github.com/7grecorder/7grecorder/backend/internal/upload"
)

type Worker struct {
	db         *sql.DB
	recorder   recorder.SyncClient
	cfg        config.Config
	merger     media.Merger
	packager   media.Packager
	compressor media.Compressor
	cos        upload.COSUploader
	bilibili   upload.BilibiliUploader
	lockID     string
}

type workerJob struct {
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
		db:         database,
		recorder:   recorderClient,
		cfg:        cfg,
		merger:     media.NewFFmpegMerger(cfg.DataRoot, cfg.TempRoot, cfg.FFmpegPath),
		packager:   media.NewFFmpegMerger(cfg.DataRoot, cfg.TempRoot, cfg.FFmpegPath),
		compressor: media.NewFFmpegMerger(cfg.DataRoot, cfg.TempRoot, cfg.FFmpegPath),
		cos:        upload.NewTencentCOSUploader(),
		bilibili:   upload.NewBiliupCLIUploader(cfg),
		lockID:     fmt.Sprintf("%s:%d", host, os.Getpid()),
	}
}

func NewWithMerger(database *sql.DB, recorderClient recorder.SyncClient, cfg config.Config, merger media.Merger) Worker {
	worker := New(database, recorderClient, cfg)
	worker.merger = merger
	return worker
}

func NewWithPackager(database *sql.DB, recorderClient recorder.SyncClient, cfg config.Config, packager media.Packager) Worker {
	worker := New(database, recorderClient, cfg)
	worker.packager = packager
	return worker
}

func NewWithCOSUploader(database *sql.DB, recorderClient recorder.SyncClient, cfg config.Config, cosUploader upload.COSUploader) Worker {
	worker := New(database, recorderClient, cfg)
	worker.cos = cosUploader
	return worker
}

func NewWithCOSUploaderAndCompressor(database *sql.DB, recorderClient recorder.SyncClient, cfg config.Config, cosUploader upload.COSUploader, compressor media.Compressor) Worker {
	worker := NewWithCOSUploader(database, recorderClient, cfg, cosUploader)
	worker.compressor = compressor
	return worker
}

func NewWithBilibiliUploader(database *sql.DB, recorderClient recorder.SyncClient, cfg config.Config, bilibiliUploader upload.BilibiliUploader) Worker {
	worker := New(database, recorderClient, cfg)
	worker.bilibili = bilibiliUploader
	return worker
}

func (w Worker) Run(ctx context.Context) {
	if err := w.discoverUploadSources(ctx); err != nil {
		log.Printf("worker reconcile failed: %v", err)
	}
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
			if err := w.discoverUploadSources(ctx); err != nil {
				log.Printf("worker reconcile failed: %v", err)
			}
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
	case "PACKAGE_UPLOAD_SOURCE":
		return w.runPackageJob(ctx, job)
	case "UPLOAD_COS_OBJECT":
		return w.runCOSUploadJob(ctx, job)
	case "UPLOAD_COS_RECORDING_FILE":
		return w.runCOSRecordingFileUploadJob(ctx, job)
	case "UPLOAD_BILIBILI":
		return w.runBilibiliUploadJob(ctx, job)
	default:
		return w.failJob(ctx, job, "PERMANENT", fmt.Errorf("unknown job type %q", job.Type))
	}
}

func (w Worker) runSyncJob(ctx context.Context, job workerJob) error {
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

func (w Worker) runMergeJob(ctx context.Context, job workerJob) error {
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

func (w Worker) runPackageJob(ctx context.Context, job workerJob) error {
	var payload mergeJobPayload
	if err := json.Unmarshal([]byte(job.PayloadJSON), &payload); err != nil {
		return w.failJob(ctx, job, "PERMANENT", fmt.Errorf("decode package payload: %w", err))
	}
	store := recording.NewStore(w.db, w.cfg)
	source, err := store.UploadSourceForPackage(ctx, payload.UploadSourceID)
	if err != nil {
		return w.failJob(ctx, job, "PERMANENT", err)
	}
	if source.Status == "READY_TO_UPLOAD" && len(source.Outputs) > 0 {
		return w.succeedJob(ctx, job, recorder.RuntimeStatus{})
	}
	if source.OutputRelativePath == "" {
		return w.failJob(ctx, job, "PERMANENT", errors.New("upload source has no package input"))
	}
	outputBaseName, err := store.UploadSourcePackageBaseName(ctx, source)
	if err != nil {
		return w.failJob(ctx, job, "PERMANENT", err)
	}
	result, err := w.packager.Package(ctx, media.PackageRequest{
		UploadSourceID:        source.ID,
		InputRelativePath:     source.OutputRelativePath,
		OutputDirRelativePath: filepath.ToSlash(filepath.Join("upload-sources", fmt.Sprintf("%d", source.RecordingProfileID), fmt.Sprintf("%d", source.ID), "parts")),
		DurationMs:            source.DurationMs,
		SizeBytes:             source.TotalBytes,
		MaxPartBytes:          w.cfg.UploadMaxPartBytes,
		MaxPartDurationSecs:   w.cfg.UploadMaxPartDurationSecs,
		OutputBaseName:        outputBaseName,
	})
	if err != nil {
		terminal := job.Attempts >= job.MaxAttempts
		message := truncateError(err)
		if markErr := store.MarkUploadSourcePackageFailed(ctx, source.ID, terminal, message); markErr != nil {
			return markErr
		}
		return w.failJob(ctx, job, "TRANSIENT", err)
	}
	if err := store.MarkUploadSourcePackageSucceeded(ctx, source.ID, result.Outputs); err != nil {
		return w.failJob(ctx, job, "PERMANENT", err)
	}
	return w.succeedJob(ctx, job, recorder.RuntimeStatus{})
}

func (w Worker) runCOSUploadJob(ctx context.Context, job workerJob) error {
	var payload upload.COSJobPayload
	if err := json.Unmarshal([]byte(job.PayloadJSON), &payload); err != nil {
		return w.failJob(ctx, job, "PERMANENT", fmt.Errorf("decode cos upload payload: %w", err))
	}
	store := upload.NewStore(w.db, w.cfg)
	request, err := store.COSUploadRequest(ctx, payload)
	if err != nil {
		return w.failUploadJob(ctx, job, payload.COSObjectID, classifyUploadError(err), err)
	}
	if err := store.MarkCOSObjectUploading(ctx, request.ObjectID); err != nil {
		return w.failJob(ctx, job, "PERMANENT", err)
	}
	compressedForCOS := false
	if w.cfg.COSCompressionEnabled {
		compressedRelativePath := cosCompressedRelativePath(request)
		compressedObjectKey := request.Prefix + compressedRelativePath
		if err := store.MarkCOSObjectCompressing(ctx, request.ObjectID); err != nil {
			return w.failJob(ctx, job, "PERMANENT", err)
		}
		compressed, err := w.compressor.Compress(ctx, media.CompressionRequest{
			UploadSourceID:     request.UploadSourceID,
			InputRelativePath:  request.SourceRelativePath,
			OutputRelativePath: compressedRelativePath,
			Preset:             w.cfg.COSCompressionPreset,
		})
		if err != nil {
			message := truncateError(err)
			if markErr := store.MarkCOSObjectCompressionFailed(ctx, request.ObjectID, message); markErr != nil {
				return markErr
			}
			return w.failJob(ctx, job, "TRANSIENT", err)
		}
		if err := store.MarkCOSObjectCompressed(ctx, request.ObjectID, compressed.RelativePath, compressed.SizeBytes, compressed.Preset, compressedObjectKey); err != nil {
			return w.failJob(ctx, job, "PERMANENT", err)
		}
		compressedPath, err := resolveWorkerPath(w.cfg.DataRoot, compressed.RelativePath)
		if err != nil {
			return w.failUploadJob(ctx, job, request.ObjectID, "PERMANENT", err)
		}
		request.SourcePath = compressedPath
		request.SourceRelativePath = compressed.RelativePath
		request.SourceSizeBytes = compressed.SizeBytes
		request.ObjectKey = compressedObjectKey
		compressedForCOS = true
	}
	result, err := w.cos.Upload(ctx, request)
	if err != nil {
		return w.failUploadJob(ctx, job, request.ObjectID, classifyUploadError(err), err)
	}
	if err := store.MarkCOSObjectUploaded(ctx, request.ObjectID, result); err != nil {
		return w.failJob(ctx, job, "PERMANENT", err)
	}
	if compressedForCOS {
		_ = removeCOSCompressedFile(w.cfg.DataRoot, request.SourceRelativePath)
	}
	return w.succeedJob(ctx, job, recorder.RuntimeStatus{})
}

func (w Worker) runCOSRecordingFileUploadJob(ctx context.Context, job workerJob) error {
	var payload upload.COSRecordingFileJobPayload
	if err := json.Unmarshal([]byte(job.PayloadJSON), &payload); err != nil {
		return w.failJob(ctx, job, "PERMANENT", fmt.Errorf("decode cos recording file upload payload: %w", err))
	}
	store := upload.NewStore(w.db, w.cfg)
	request, err := store.COSRecordingFileUploadRequest(ctx, payload)
	if err != nil {
		return w.failCOSRecordingFileJob(ctx, job, payload.COSObjectID, classifyUploadError(err), err)
	}
	if err := store.MarkCOSRecordingFileUploading(ctx, request.ObjectID); err != nil {
		return w.failJob(ctx, job, "PERMANENT", err)
	}
	result, err := w.cos.Upload(ctx, request)
	if err != nil {
		return w.failCOSRecordingFileJob(ctx, job, request.ObjectID, classifyUploadError(err), err)
	}
	if err := store.MarkCOSRecordingFileUploaded(ctx, request.ObjectID, result); err != nil {
		return w.failJob(ctx, job, "PERMANENT", err)
	}
	return w.succeedJob(ctx, job, recorder.RuntimeStatus{})
}

func (w Worker) runBilibiliUploadJob(ctx context.Context, job workerJob) error {
	var payload upload.BilibiliJobPayload
	if err := json.Unmarshal([]byte(job.PayloadJSON), &payload); err != nil {
		return w.failJob(ctx, job, "PERMANENT", fmt.Errorf("decode bilibili upload payload: %w", err))
	}
	store := upload.NewStore(w.db, w.cfg)
	request, err := store.BilibiliUploadRequest(ctx, payload)
	if err != nil {
		return w.failBilibiliJob(ctx, job, payload.PublicationID, classifyUploadError(err), err)
	}
	if err := store.MarkBilibiliUploading(ctx, request.PublicationID, request); err != nil {
		return w.failJob(ctx, job, "PERMANENT", err)
	}
	result, err := w.bilibili.Upload(ctx, request)
	if err != nil {
		return w.failBilibiliJob(ctx, job, request.PublicationID, classifyUploadError(err), err)
	}
	if err := store.MarkBilibiliUploaded(ctx, request.PublicationID, result); err != nil {
		return w.failJob(ctx, job, "PERMANENT", err)
	}
	return w.succeedJob(ctx, job, recorder.RuntimeStatus{})
}

func (w Worker) discoverUploadSources(ctx context.Context) error {
	recordingStore := recording.NewStore(w.db, w.cfg)
	if _, err := recordingStore.ReconcileLocal(ctx, accountSuperAdmin()); err != nil {
		return err
	}
	if _, err := recordingStore.DiscoverUploadSources(ctx, recording.DefaultMergeGapThresholdSeconds); err != nil {
		return err
	}
	_, err := upload.NewStore(w.db, w.cfg).Reconcile(ctx, accountSuperAdmin())
	return err
}

func accountSuperAdmin() account.User {
	return account.User{Role: account.RoleSuperAdmin}
}

func (w Worker) claimJob(ctx context.Context) (workerJob, error) {
	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		return workerJob{}, fmt.Errorf("begin job claim: %w", err)
	}
	defer tx.Rollback()

	var job workerJob
	err = tx.QueryRowContext(ctx, `
		SELECT id, type, COALESCE(recording_profile_id, 0), COALESCE(payload_json, ''), attempts, max_attempts
		FROM jobs
		WHERE type IN ('SYNC_RECORDER_PROFILE', 'MERGE_UPLOAD_SOURCE', 'PACKAGE_UPLOAD_SOURCE', 'UPLOAD_COS_OBJECT', 'UPLOAD_COS_RECORDING_FILE', 'UPLOAD_BILIBILI')
			AND status = 'PENDING'
			AND run_after <= CURRENT_TIMESTAMP
		ORDER BY priority ASC, run_after ASC, id ASC
		LIMIT 1
	`).Scan(&job.ID, &job.Type, &job.RecordingProfileID, &job.PayloadJSON, &job.Attempts, &job.MaxAttempts)
	if err != nil {
		return workerJob{}, err
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
		return workerJob{}, fmt.Errorf("claim job: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return workerJob{}, fmt.Errorf("read claim rows affected: %w", err)
	}
	if changed != 1 {
		return workerJob{}, sql.ErrNoRows
	}
	if err := tx.Commit(); err != nil {
		return workerJob{}, fmt.Errorf("commit job claim: %w", err)
	}
	job.Attempts++
	return job, nil
}

func (w Worker) succeedJob(ctx context.Context, job workerJob, status recorder.RuntimeStatus) error {
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

func (w Worker) failUploadJob(ctx context.Context, job workerJob, objectID int64, errorClass string, cause error) error {
	if objectID > 0 {
		if err := upload.NewStore(w.db, w.cfg).MarkCOSObjectUploadFailed(ctx, objectID, errorClass, truncateError(cause)); err != nil {
			return err
		}
	}
	return w.failJob(ctx, job, errorClass, cause)
}

func (w Worker) failCOSRecordingFileJob(ctx context.Context, job workerJob, objectID int64, errorClass string, cause error) error {
	if objectID > 0 {
		if err := upload.NewStore(w.db, w.cfg).MarkCOSRecordingFileUploadFailed(ctx, objectID, errorClass, truncateError(cause)); err != nil {
			return err
		}
	}
	return w.failJob(ctx, job, errorClass, cause)
}

func (w Worker) failBilibiliJob(ctx context.Context, job workerJob, publicationID int64, errorClass string, cause error) error {
	if publicationID > 0 {
		if err := upload.NewStore(w.db, w.cfg).MarkBilibiliUploadFailed(ctx, publicationID, errorClass, truncateError(cause)); err != nil {
			return err
		}
	}
	return w.failJob(ctx, job, errorClass, cause)
}

func classifyUploadError(err error) string {
	var classified interface {
		ErrorClass() string
	}
	if errors.As(err, &classified) {
		return classified.ErrorClass()
	}
	if errors.Is(err, upload.ErrValidation) || errors.Is(err, upload.ErrNotFound) || errors.Is(err, upload.ErrForbidden) {
		return "PERMANENT"
	}
	return "TRANSIENT"
}

func (w Worker) failJob(ctx context.Context, job workerJob, errorClass string, cause error) error {
	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin job failure: %w", err)
	}
	defer tx.Rollback()

	nextStatus := "PENDING"
	runAfter := "CURRENT_TIMESTAMP"
	if job.Attempts >= job.MaxAttempts || isTerminalErrorClass(errorClass) {
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

func isTerminalErrorClass(errorClass string) bool {
	return errorClass == "PERMANENT" || errorClass == "AUTH" || errorClass == "SOURCE_MISSING"
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

func cosCompressedRelativePath(request upload.COSUploadRequest) string {
	base := strings.TrimSuffix(filepath.Base(request.SourceRelativePath), filepath.Ext(request.SourceRelativePath))
	if base == "" {
		base = fmt.Sprintf("output-%d", request.OutputID)
	}
	return filepath.ToSlash(filepath.Join(
		"upload-sources",
		fmt.Sprintf("%d", request.RecordingProfileID),
		fmt.Sprintf("%d", request.UploadSourceID),
		"cos",
		base+".mp4",
	))
}

func resolveWorkerPath(root string, relativePath string) (string, error) {
	if root == "" || relativePath == "" || filepath.IsAbs(relativePath) {
		return "", fmt.Errorf("unsafe path")
	}
	cleaned := filepath.Clean(relativePath)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("unsafe path")
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	candidateAbs, err := filepath.Abs(filepath.Join(rootAbs, cleaned))
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(rootAbs, candidateAbs)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("unsafe path")
	}
	return candidateAbs, nil
}

func removeCOSCompressedFile(root string, relativePath string) error {
	normalized := filepath.ToSlash(relativePath)
	if !strings.Contains(normalized, "/cos/") {
		return fmt.Errorf("not a cos compression file")
	}
	path, err := resolveWorkerPath(root, relativePath)
	if err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("cos compression path is a directory")
	}
	return os.Remove(path)
}
