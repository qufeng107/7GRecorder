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
	"github.com/7grecorder/7grecorder/backend/internal/sitetls"
	"github.com/7grecorder/7grecorder/backend/internal/songs"
	"github.com/7grecorder/7grecorder/backend/internal/upload"
)

type Worker struct {
	db                   *sql.DB
	recorder             recorder.SyncClient
	cfg                  config.Config
	merger               media.Merger
	packager             media.Packager
	editor               media.Editor
	cos                  upload.COSUploader
	bilibili             upload.BilibiliUploader
	siteTLS              sitetls.Synchronizer
	songSourceDownloader songs.SourceDownloader
	songRecognizer       songs.Recognizer
	songAudioCutter      songs.AudioCutter
	lockID               string
}

type workerJob struct {
	ID                 int64
	Type               string
	RecordingProfileID int64
	PayloadJSON        string
	Attempts           int
	MaxAttempts        int
}

type RecoveryResult struct {
	Retryable int
	Ambiguous int
	Completed int
}

type abandonedJob struct {
	ID                 int64
	Type               string
	RecordingProfileID int64
	PublicationID      int64
	PublicationStatus  string
	PayloadJSON        string
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
		db:                   database,
		recorder:             recorderClient,
		cfg:                  cfg,
		merger:               media.NewFFmpegMerger(cfg.DataRoot, cfg.TempRoot, cfg.FFmpegPath),
		packager:             media.NewFFmpegMerger(cfg.DataRoot, cfg.TempRoot, cfg.FFmpegPath),
		editor:               media.NewFFmpegMerger(cfg.DataRoot, cfg.TempRoot, cfg.FFmpegPath),
		cos:                  upload.NewTencentCOSUploader(cfg.COSUploadMaxBytesPerSec),
		bilibili:             upload.NewBiliupCLIUploader(cfg),
		siteTLS:              sitetls.NewTencentSynchronizer(cfg.DataRoot),
		songSourceDownloader: songs.TencentCOSDownloader{},
		songRecognizer:       songs.NewACRCloudRecognizer(),
		songAudioCutter:      songs.FFmpegAudioCutter{Path: cfg.FFmpegPath},
		lockID:               fmt.Sprintf("%s:%d:%d", host, os.Getpid(), time.Now().UnixNano()),
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

func NewWithEditor(database *sql.DB, recorderClient recorder.SyncClient, cfg config.Config, editor media.Editor) Worker {
	worker := New(database, recorderClient, cfg)
	worker.editor = editor
	return worker
}

func NewWithCOSUploader(database *sql.DB, recorderClient recorder.SyncClient, cfg config.Config, cosUploader upload.COSUploader) Worker {
	worker := New(database, recorderClient, cfg)
	worker.cos = cosUploader
	return worker
}

func NewWithBilibiliUploader(database *sql.DB, recorderClient recorder.SyncClient, cfg config.Config, bilibiliUploader upload.BilibiliUploader) Worker {
	worker := New(database, recorderClient, cfg)
	worker.bilibili = bilibiliUploader
	return worker
}

func NewWithSiteTLSSynchronizer(database *sql.DB, recorderClient recorder.SyncClient, cfg config.Config, synchronizer sitetls.Synchronizer) Worker {
	worker := New(database, recorderClient, cfg)
	worker.siteTLS = synchronizer
	return worker
}

func NewWithSongSourceDownloader(database *sql.DB, recorderClient recorder.SyncClient, cfg config.Config, downloader songs.SourceDownloader) Worker {
	worker := New(database, recorderClient, cfg)
	worker.songSourceDownloader = downloader
	return worker
}

func NewWithSongProcessors(database *sql.DB, recorderClient recorder.SyncClient, cfg config.Config, recognizer songs.Recognizer, cutter songs.AudioCutter, uploader upload.COSUploader) Worker {
	worker := New(database, recorderClient, cfg)
	worker.songRecognizer = recognizer
	worker.songAudioCutter = cutter
	worker.cos = uploader
	return worker
}

func (w Worker) Run(ctx context.Context) {
	recovery, err := w.RecoverAbandonedJobs(ctx)
	if err != nil {
		log.Printf("worker startup recovery failed; worker disabled: %v", err)
		return
	}
	if recovery.Retryable > 0 || recovery.Ambiguous > 0 || recovery.Completed > 0 {
		log.Printf("worker startup recovery completed: retryable=%d ambiguous=%d completed=%d", recovery.Retryable, recovery.Ambiguous, recovery.Completed)
	}
	if err := w.RequeueRecorderSyncJobs(ctx); err != nil {
		log.Printf("worker recorder resync preparation failed; worker disabled: %v", err)
		return
	}
	if err := w.refreshRecorderRuntimes(ctx); err != nil {
		log.Printf("worker recorder runtime refresh failed: %v", err)
	}
	if err := w.discoverUploadSources(ctx); err != nil {
		log.Printf("worker reconcile failed: %v", err)
	}
	go w.runDiscoveryLoop(ctx)
	go w.runResourceLoop(ctx, "LIGHT", 1)
	go w.runResourceLoop(ctx, "MEDIA", 1)
	go w.runResourceLoop(ctx, "AI", 1)
	go w.runResourceLoop(ctx, "NETWORK", 1)
	w.runResourceLoop(ctx, "NETWORK", 2)
}

func (w Worker) RequeueRecorderSyncJobs(ctx context.Context) error {
	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin recorder resync preparation: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `
		UPDATE jobs
		SET status = 'PENDING', attempts = 0, run_after = CURRENT_TIMESTAMP,
			locked_at = NULL, heartbeat_at = NULL, locked_by = NULL,
			last_error_class = NULL, last_error = NULL,
			progress_current_bytes = 0, progress_total_bytes = 0,
			progress_message = NULL, progress_updated_at = NULL, updated_at = CURRENT_TIMESTAMP
		WHERE type = 'SYNC_RECORDER_PROFILE'
	`); err != nil {
		return fmt.Errorf("requeue recorder sync jobs: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE recording_profile_runtime
		SET sync_status = 'PENDING', last_error = NULL, updated_at = CURRENT_TIMESTAMP
		WHERE recording_profile_id IN (
			SELECT recording_profile_id FROM jobs WHERE type = 'SYNC_RECORDER_PROFILE'
		)
	`); err != nil {
		return fmt.Errorf("mark recorder runtimes pending: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit recorder resync preparation: %w", err)
	}
	return nil
}

func (w Worker) RecoverAbandonedJobs(ctx context.Context) (RecoveryResult, error) {
	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		return RecoveryResult{}, fmt.Errorf("begin abandoned job recovery: %w", err)
	}
	defer tx.Rollback()

	rows, err := tx.QueryContext(ctx, `
		SELECT j.id, j.type, COALESCE(j.recording_profile_id, 0), COALESCE(j.publication_id, 0),
			COALESCE(p.status, ''), COALESCE(j.payload_json, '')
		FROM jobs j
		LEFT JOIN publications p ON p.id = j.publication_id
		WHERE j.status = 'RUNNING'
			AND COALESCE(j.locked_by, '') != ?
		ORDER BY j.id ASC
	`, w.lockID)
	if err != nil {
		return RecoveryResult{}, fmt.Errorf("list abandoned jobs: %w", err)
	}
	items := make([]abandonedJob, 0)
	for rows.Next() {
		var item abandonedJob
		if err := rows.Scan(&item.ID, &item.Type, &item.RecordingProfileID, &item.PublicationID, &item.PublicationStatus, &item.PayloadJSON); err != nil {
			rows.Close()
			return RecoveryResult{}, fmt.Errorf("scan abandoned job: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return RecoveryResult{}, fmt.Errorf("iterate abandoned jobs: %w", err)
	}
	if err := rows.Close(); err != nil {
		return RecoveryResult{}, fmt.Errorf("close abandoned jobs: %w", err)
	}

	result := RecoveryResult{}
	for _, item := range items {
		switch item.Type {
		case "UPLOAD_BILIBILI":
			if item.PublicationStatus == "VERIFIED" {
				if err := recoverJobAsSucceeded(ctx, tx, item.ID); err != nil {
					return RecoveryResult{}, err
				}
				result.Completed++
				continue
			}
			message := "upload worker stopped before Bilibili result was persisted; verify Creator Center before retry"
			if item.PublicationID > 0 {
				if _, err := tx.ExecContext(ctx, `
					UPDATE publications
					SET status = 'AMBIGUOUS', last_error = ?, updated_at = CURRENT_TIMESTAMP
					WHERE id = ? AND status != 'VERIFIED'
				`, message, item.PublicationID); err != nil {
					return RecoveryResult{}, fmt.Errorf("mark abandoned Bilibili publication ambiguous: %w", err)
				}
			}
			if err := recoverJobAsAmbiguous(ctx, tx, item.ID, message); err != nil {
				return RecoveryResult{}, err
			}
			result.Ambiguous++
		case "UPLOAD_COS_OBJECT":
			var payload upload.COSJobPayload
			if err := json.Unmarshal([]byte(item.PayloadJSON), &payload); err != nil || payload.COSObjectID <= 0 {
				if err := recoverJobAsAmbiguous(ctx, tx, item.ID, "cannot safely recover COS job with invalid payload"); err != nil {
					return RecoveryResult{}, err
				}
				result.Ambiguous++
				continue
			}
			if _, err := tx.ExecContext(ctx, `
				UPDATE upload_source_cos_objects
				SET status = 'PENDING', compression_status = 'DISABLED', compression_preset = NULL,
					compressed_from_relative_path = NULL, last_error = NULL, updated_at = CURRENT_TIMESTAMP
				WHERE id = ?
			`, payload.COSObjectID); err != nil {
				return RecoveryResult{}, fmt.Errorf("reset abandoned upload-source COS object: %w", err)
			}
			if err := recoverJobAsPending(ctx, tx, item.ID); err != nil {
				return RecoveryResult{}, err
			}
			result.Retryable++
		case "UPLOAD_COS_RECORDING_FILE":
			var payload upload.COSRecordingFileJobPayload
			if err := json.Unmarshal([]byte(item.PayloadJSON), &payload); err != nil || payload.COSObjectID <= 0 {
				if err := recoverJobAsAmbiguous(ctx, tx, item.ID, "cannot safely recover COS recording-file job with invalid payload"); err != nil {
					return RecoveryResult{}, err
				}
				result.Ambiguous++
				continue
			}
			if _, err := tx.ExecContext(ctx, `
				UPDATE cos_objects
				SET status = 'PENDING', last_error = NULL, updated_at = CURRENT_TIMESTAMP
				WHERE id = ?
			`, payload.COSObjectID); err != nil {
				return RecoveryResult{}, fmt.Errorf("reset abandoned recording-file COS object: %w", err)
			}
			if err := recoverJobAsPending(ctx, tx, item.ID); err != nil {
				return RecoveryResult{}, err
			}
			result.Retryable++
		case "DOWNLOAD_SONG_SOURCE", "PROCESS_SONG_ANALYSIS":
			var payload songs.DownloadJobPayload
			if err := json.Unmarshal([]byte(item.PayloadJSON), &payload); err != nil || payload.AnalysisRunID <= 0 {
				if err := recoverJobAsAmbiguous(ctx, tx, item.ID, "cannot safely recover song download job with invalid payload"); err != nil {
					return RecoveryResult{}, err
				}
				result.Ambiguous++
				continue
			}
			var runStatus string
			err := tx.QueryRowContext(ctx, `SELECT status FROM song_analysis_runs WHERE id = ?`, payload.AnalysisRunID).Scan(&runStatus)
			if err != nil {
				if err := recoverJobAsAmbiguous(ctx, tx, item.ID, "cannot safely recover song download job without its analysis run"); err != nil {
					return RecoveryResult{}, err
				}
				result.Ambiguous++
				continue
			}
			if item.Type == "PROCESS_SONG_ANALYSIS" {
				switch runStatus {
				case "COMPLETED", "REVIEW_REQUIRED":
					if err := recoverJobAsSucceeded(ctx, tx, item.ID); err != nil {
						return RecoveryResult{}, err
					}
					result.Completed++
				default:
					if err := recoverJobAsPending(ctx, tx, item.ID); err != nil {
						return RecoveryResult{}, err
					}
					result.Retryable++
				}
				continue
			}
			switch runStatus {
			case "ANALYZING", "RECOGNIZING", "FINALIZING", "GENERATING_AUDIO", "REVIEW_REQUIRED", "COMPLETED":
				if _, err := tx.ExecContext(ctx, `DELETE FROM storage_reservations WHERE job_id = ?`, item.ID); err != nil {
					return RecoveryResult{}, fmt.Errorf("release recovered song reservation: %w", err)
				}
				if err := recoverJobAsSucceeded(ctx, tx, item.ID); err != nil {
					return RecoveryResult{}, err
				}
				result.Completed++
				continue
			case "PENDING", "DOWNLOADING":
				if err := recoverJobAsPending(ctx, tx, item.ID); err != nil {
					return RecoveryResult{}, err
				}
				result.Retryable++
				continue
			default:
				if err := recoverJobAsAmbiguous(ctx, tx, item.ID, fmt.Sprintf("song analysis run is terminal while download job was running: %s", runStatus)); err != nil {
					return RecoveryResult{}, err
				}
				result.Ambiguous++
				continue
			}
		case "SYNC_RECORDER_PROFILE", "MERGE_UPLOAD_SOURCE", "PACKAGE_UPLOAD_SOURCE", "APPLY_UPLOAD_SOURCE_EDIT", "SYNC_SITE_TLS":
			if item.Type == "SYNC_RECORDER_PROFILE" && item.RecordingProfileID > 0 {
				if _, err := tx.ExecContext(ctx, `
					UPDATE recording_profile_runtime
					SET sync_status = 'PENDING', last_error = NULL, updated_at = CURRENT_TIMESTAMP
					WHERE recording_profile_id = ?
				`, item.RecordingProfileID); err != nil {
					return RecoveryResult{}, fmt.Errorf("reset abandoned recorder sync runtime: %w", err)
				}
			}
			if err := recoverJobAsPending(ctx, tx, item.ID); err != nil {
				return RecoveryResult{}, err
			}
			result.Retryable++
		default:
			if err := recoverJobAsAmbiguous(ctx, tx, item.ID, "worker stopped during a job with unknown recovery semantics"); err != nil {
				return RecoveryResult{}, err
			}
			result.Ambiguous++
		}
	}
	if err := tx.Commit(); err != nil {
		return RecoveryResult{}, fmt.Errorf("commit abandoned job recovery: %w", err)
	}
	return result, nil
}

func recoverJobAsSucceeded(ctx context.Context, tx *sql.Tx, jobID int64) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE jobs
		SET status = 'SUCCEEDED', locked_at = NULL, heartbeat_at = NULL, locked_by = NULL,
			last_error_class = NULL, last_error = NULL, progress_message = NULL,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND status = 'RUNNING'
	`, jobID)
	if err != nil {
		return fmt.Errorf("complete abandoned job %d with verified result: %w", jobID, err)
	}
	return nil
}

func recoverJobAsPending(ctx context.Context, tx *sql.Tx, jobID int64) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE jobs
		SET status = 'PENDING', attempts = MAX(attempts - 1, 0), run_after = CURRENT_TIMESTAMP,
			locked_at = NULL, heartbeat_at = NULL, locked_by = NULL,
			last_error_class = NULL, last_error = NULL,
			progress_current_bytes = 0, progress_total_bytes = 0,
			progress_message = NULL, progress_updated_at = NULL, updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND status = 'RUNNING'
	`, jobID)
	if err != nil {
		return fmt.Errorf("reset abandoned job %d: %w", jobID, err)
	}
	return nil
}

func recoverJobAsAmbiguous(ctx context.Context, tx *sql.Tx, jobID int64, message string) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE jobs
		SET status = 'FAILED', locked_at = NULL, heartbeat_at = NULL, locked_by = NULL,
			last_error_class = 'AMBIGUOUS', last_error = ?, progress_message = NULL,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND status = 'RUNNING'
	`, message, jobID)
	if err != nil {
		return fmt.Errorf("freeze abandoned job %d: %w", jobID, err)
	}
	return nil
}

func (w Worker) runDiscoveryLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := w.refreshRecorderRuntimes(ctx); err != nil {
				log.Printf("worker recorder runtime refresh failed: %v", err)
			}
			if err := w.discoverUploadSources(ctx); err != nil {
				log.Printf("worker reconcile failed: %v", err)
			}
		}
	}
}

func (w Worker) refreshRecorderRuntimes(ctx context.Context) error {
	client, ok := w.recorder.(recorder.RuntimeClient)
	if !ok {
		return nil
	}
	rows, err := w.db.QueryContext(ctx, `
		SELECT id, room_id
		FROM recording_profiles
		WHERE enabled = 1 AND archived_at IS NULL
		ORDER BY id ASC
	`)
	if err != nil {
		return fmt.Errorf("list recorder runtimes to refresh: %w", err)
	}
	type target struct {
		profileID int64
		roomID    string
	}
	targets := make([]target, 0)
	for rows.Next() {
		var item target
		if err := rows.Scan(&item.profileID, &item.roomID); err != nil {
			rows.Close()
			return fmt.Errorf("scan recorder runtime target: %w", err)
		}
		targets = append(targets, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("iterate recorder runtime targets: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close recorder runtime targets: %w", err)
	}

	var refreshErrors []error
	for _, item := range targets {
		status, err := client.ReadRuntimeStatus(ctx, item.roomID)
		if err != nil {
			refreshErrors = append(refreshErrors, fmt.Errorf("read recorder runtime for profile %d: %w", item.profileID, err))
			continue
		}
		if _, err := w.db.ExecContext(ctx, `
			UPDATE recording_profile_runtime
			SET stream_status = ?, recorder_status = ?,
				last_reconciled_at = CURRENT_TIMESTAMP,
				updated_at = CURRENT_TIMESTAMP
			WHERE recording_profile_id = ?
		`, status.StreamStatus, status.RecorderStatus, item.profileID); err != nil {
			refreshErrors = append(refreshErrors, fmt.Errorf("update recorder runtime for profile %d: %w", item.profileID, err))
		}
	}
	return errors.Join(refreshErrors...)
}

func (w Worker) runResourceLoop(ctx context.Context, resourceClass string, slot int) {
	for {
		err := w.runOnceForResource(ctx, resourceClass)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			log.Printf("worker %s-%d failed: %v", resourceClass, slot, err)
		}
		wait := 2 * time.Second
		if errors.Is(err, sql.ErrNoRows) {
			wait = 10 * time.Second
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
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
	return w.runClaimedJob(ctx, job)
}

func (w Worker) runOnceForResource(ctx context.Context, resourceClass string) error {
	job, err := w.claimJobForResource(ctx, resourceClass)
	if err != nil {
		return err
	}
	return w.runClaimedJob(ctx, job)
}

func (w Worker) runClaimedJob(ctx context.Context, job workerJob) error {
	jobCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go w.cancelWhenJobStops(ctx, job.ID, cancel, done)
	err := w.executeClaimedJob(jobCtx, job)
	close(done)
	cancel()
	if err != nil && ctx.Err() == nil {
		status, statusErr := w.jobStatus(ctx, job.ID)
		if statusErr == nil && status == "CANCELLED" {
			return nil
		}
	}
	return err
}

func (w Worker) executeClaimedJob(ctx context.Context, job workerJob) error {
	switch job.Type {
	case "SYNC_RECORDER_PROFILE":
		return w.runSyncJob(ctx, job)
	case "MERGE_UPLOAD_SOURCE":
		return w.runMergeJob(ctx, job)
	case "PACKAGE_UPLOAD_SOURCE":
		return w.runPackageJob(ctx, job)
	case "APPLY_UPLOAD_SOURCE_EDIT":
		return w.runApplyUploadSourceEditJob(ctx, job)
	case "UPLOAD_COS_OBJECT":
		return w.runCOSUploadJob(ctx, job)
	case "UPLOAD_COS_RECORDING_FILE":
		return w.runCOSRecordingFileUploadJob(ctx, job)
	case "UPLOAD_BILIBILI":
		return w.runBilibiliUploadJob(ctx, job)
	case "SYNC_SITE_TLS":
		return w.runSiteTLSSyncJob(ctx, job)
	case "DOWNLOAD_SONG_SOURCE":
		return w.runDownloadSongSourceJob(ctx, job)
	case "PROCESS_SONG_ANALYSIS":
		return w.runProcessSongAnalysisJob(ctx, job)
	default:
		return w.failJob(ctx, job, "PERMANENT", fmt.Errorf("unknown job type %q", job.Type))
	}
}

func (w Worker) runProcessSongAnalysisJob(ctx context.Context, job workerJob) error {
	var payload songs.ProcessJobPayload
	if err := json.Unmarshal([]byte(job.PayloadJSON), &payload); err != nil || payload.AnalysisRunID <= 0 {
		return w.failJob(ctx, job, "PERMANENT", errors.New("invalid song analysis payload"))
	}
	store := songs.NewStore(w.db, w.cfg)
	request, err := store.ProcessRequest(ctx, payload.AnalysisRunID)
	if err != nil {
		class := classifySongError(err)
		_ = store.MarkProcessFailed(ctx, payload.AnalysisRunID, class, truncateError(err))
		return w.failJob(ctx, job, class, err)
	}
	if _, err := os.Stat(request.AnalysisPath); errors.Is(err, os.ErrNotExist) {
		_ = w.updateJobProgress(ctx, job.ID, upload.UploadProgress{Message: "Extracting analysis audio"})
		if err := w.songAudioCutter.ExtractAnalysisAudio(ctx, request.SourcePath, request.AnalysisPath); err != nil {
			_ = store.MarkProcessFailed(ctx, payload.AnalysisRunID, "PERMANENT", truncateError(err))
			return w.failJob(ctx, job, "PERMANENT", err)
		}
	} else if err != nil {
		return w.failJob(ctx, job, "PERMANENT", err)
	}
	analysisInfo, err := os.Stat(request.AnalysisPath)
	if err != nil {
		return w.failJob(ctx, job, "SOURCE_MISSING", err)
	}
	if analysisInfo.Size() >= 500_000_000 {
		err := errors.New("analysis audio exceeds ACRCloud's 500 MB file limit; multi-chunk analysis is required")
		_ = store.MarkProcessFailed(ctx, payload.AnalysisRunID, "PERMANENT", err.Error())
		return w.failJob(ctx, job, "PERMANENT", err)
	}
	if err := store.MarkRecognizing(ctx, payload.AnalysisRunID); err != nil {
		return w.failJob(ctx, job, classifySongError(err), err)
	}
	result, err := w.songRecognizer.Recognize(ctx, songs.RecognitionRequest{
		Region: request.ProviderRegion, ContainerID: request.ContainerID, AccessToken: request.AccessToken,
		AudioPath: request.AnalysisPath, ProviderFilename: request.ProviderName,
	}, func(ctx context.Context, progress songs.RecognitionProgress) error {
		if progress.ProviderFileID != "" {
			if err := store.MarkProviderSubmitted(ctx, payload.AnalysisRunID, request.ProviderName, progress.ProviderFileID, progress.PollCount); err != nil {
				return err
			}
		}
		return w.updateJobProgress(ctx, job.ID, upload.UploadProgress{Message: progress.Message})
	})
	if err != nil {
		class := songs.ProviderErrorClass(err)
		_ = store.MarkProcessFailed(ctx, payload.AnalysisRunID, class, truncateError(err))
		return w.failJob(ctx, job, class, err)
	}
	songIDs, err := store.PersistRecognition(ctx, payload.AnalysisRunID, result)
	if err != nil {
		_ = store.MarkProcessFailed(ctx, payload.AnalysisRunID, "PERMANENT", truncateError(err))
		return w.failJob(ctx, job, "PERMANENT", err)
	}
	for index, songID := range songIDs {
		artifact, err := store.AudioArtifactRequest(ctx, songID)
		if errors.Is(err, songs.ErrNotReady) {
			continue
		}
		if err != nil {
			_ = store.MarkProcessFailed(ctx, payload.AnalysisRunID, classifySongError(err), truncateError(err))
			return w.failJob(ctx, job, classifySongError(err), err)
		}
		_ = w.updateJobProgress(ctx, job.ID, upload.UploadProgress{CurrentBytes: int64(index), TotalBytes: int64(len(songIDs)), Message: fmt.Sprintf("Generating audio clip %d/%d", index+1, len(songIDs))})
		if err := store.MarkAudioGenerating(ctx, artifact); err != nil {
			return w.failJob(ctx, job, "PERMANENT", err)
		}
		estimatedBytes := (artifact.EndOffsetMs-artifact.StartOffsetMs)*24 + 1_048_576
		if err := store.EnsureAudioCacheCapacity(ctx, estimatedBytes); err != nil {
			if errors.Is(err, songs.ErrWaitingForSpace) {
				return w.deferSongJobForSpace(ctx, job)
			}
			return w.failJob(ctx, job, classifySongError(err), err)
		}
		size, err := w.songAudioCutter.CutM4A(ctx, artifact.SourcePath, artifact.DestinationPath, artifact.StartOffsetMs, artifact.EndOffsetMs)
		if err != nil {
			_ = store.MarkProcessFailed(ctx, payload.AnalysisRunID, "PERMANENT", truncateError(err))
			return w.failJob(ctx, job, "PERMANENT", err)
		}
		artifact.COSRequest.SourceSizeBytes = size
		uploadResult, err := w.cos.Upload(ctx, artifact.COSRequest, w.progressReporter(job))
		if err != nil {
			class := classifyUploadError(err)
			_ = store.MarkProcessFailed(ctx, payload.AnalysisRunID, class, truncateError(err))
			return w.failJob(ctx, job, class, err)
		}
		if err := store.MarkAudioAvailable(ctx, artifact, uploadResult); err != nil {
			return w.failJob(ctx, job, "PERMANENT", err)
		}
	}
	if err := store.MarkRunCompleted(ctx, payload.AnalysisRunID); err != nil {
		return w.failJob(ctx, job, "PERMANENT", err)
	}
	if err := store.CleanupRunWork(ctx, payload.AnalysisRunID, request.AnalysisPath); err != nil {
		log.Printf("song analysis run %d cleanup deferred: %v", payload.AnalysisRunID, err)
	}
	return w.succeedJob(ctx, job, recorder.RuntimeStatus{})
}

func (w Worker) runDownloadSongSourceJob(ctx context.Context, job workerJob) error {
	var payload songs.DownloadJobPayload
	if err := json.Unmarshal([]byte(job.PayloadJSON), &payload); err != nil {
		return w.failJob(ctx, job, "PERMANENT", fmt.Errorf("decode song source download payload: %w", err))
	}
	if payload.AnalysisRunID <= 0 {
		return w.failJob(ctx, job, "PERMANENT", errors.New("song source download payload has no analysis run"))
	}
	store := songs.NewStore(w.db, w.cfg)
	request, err := store.DownloadRequest(ctx, payload.AnalysisRunID)
	if err != nil {
		class := classifySongError(err)
		_ = store.MarkDownloadFailed(ctx, payload.AnalysisRunID, class, truncateError(err), job.Attempts >= job.MaxAttempts)
		return w.failJob(ctx, job, class, err)
	}
	if err := store.ReserveSourceDownload(ctx, job.ID, payload.AnalysisRunID); err != nil {
		if errors.Is(err, songs.ErrWaitingForSpace) {
			return w.deferSongJobForSpace(ctx, job)
		}
		return w.failJob(ctx, job, classifySongError(err), err)
	}
	if err := store.MarkDownloading(ctx, payload.AnalysisRunID); err != nil {
		_ = store.ReleaseReservation(ctx, job.ID)
		return w.failJob(ctx, job, classifySongError(err), err)
	}
	reporter := w.progressReporter(job)
	err = w.songSourceDownloader.Download(ctx, request, func(ctx context.Context, progress songs.DownloadProgress) {
		_ = store.HeartbeatReservation(ctx, job.ID)
		reporter(ctx, upload.UploadProgress{CurrentBytes: progress.CurrentBytes, TotalBytes: progress.TotalBytes, Message: progress.Message})
	})
	if err != nil {
		class := songs.DownloadErrorClass(err)
		_ = store.ReleaseReservation(ctx, job.ID)
		_ = store.MarkDownloadFailed(ctx, payload.AnalysisRunID, class, truncateError(err), job.Attempts >= job.MaxAttempts)
		return w.failJob(ctx, job, class, err)
	}
	if err := store.MarkDownloaded(ctx, job.ID, payload.AnalysisRunID); err != nil {
		_ = store.ReleaseReservation(ctx, job.ID)
		return w.failJob(ctx, job, "PERMANENT", err)
	}
	return w.succeedJob(ctx, job, recorder.RuntimeStatus{})
}

func (w Worker) deferSongJobForSpace(ctx context.Context, job workerJob) error {
	_, err := w.db.ExecContext(ctx, `UPDATE jobs SET status = 'PENDING', attempts = MAX(attempts - 1, 0),
		run_after = datetime('now', '+60 seconds'), locked_at = NULL, heartbeat_at = NULL, locked_by = NULL,
		last_error_class = NULL, last_error = NULL, progress_message = 'WAITING_FOR_SPACE',
		progress_updated_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND status = 'RUNNING'`, job.ID)
	return err
}

func classifySongError(err error) string {
	if errors.Is(err, songs.ErrNotReady) {
		return "SOURCE_MISSING"
	}
	if errors.Is(err, songs.ErrValidation) || errors.Is(err, songs.ErrForbidden) || errors.Is(err, songs.ErrNotFound) {
		return "PERMANENT"
	}
	return "TRANSIENT"
}

func (w Worker) cancelWhenJobStops(ctx context.Context, jobID int64, cancel context.CancelFunc, done <-chan struct{}) {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		running, err := w.jobIsRunning(ctx, jobID)
		if err == nil && !running {
			cancel()
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-done:
			return
		case <-ticker.C:
		}
	}
}

func (w Worker) jobIsRunning(ctx context.Context, jobID int64) (bool, error) {
	status, err := w.jobStatus(ctx, jobID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return status == "RUNNING", nil
}

func (w Worker) jobStatus(ctx context.Context, jobID int64) (string, error) {
	var status string
	err := w.db.QueryRowContext(ctx, `SELECT status FROM jobs WHERE id = ?`, jobID).Scan(&status)
	if err != nil {
		return "", fmt.Errorf("read job status: %w", err)
	}
	return status, nil
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
		segments = append(segments, media.Segment{
			RelativePath:    segment.RelativePath,
			SizeBytes:       segment.SizeBytes,
			DurationMs:      segment.DurationMs,
			TimelineStartMs: segment.TimelineStartMs,
			TimelineEndMs:   segment.TimelineEndMs,
		})
	}
	outputBaseName, err := store.UploadSourcePackageBaseName(ctx, source)
	if err != nil {
		return w.failJob(ctx, job, "PERMANENT", err)
	}
	result, err := w.packager.PackageSegments(ctx, media.SegmentPackageRequest{
		UploadSourceID:        source.ID,
		Segments:              segments,
		OutputDirRelativePath: filepath.ToSlash(filepath.Join("upload-sources", fmt.Sprintf("%d", source.RecordingProfileID), fmt.Sprintf("%d", source.ID), "parts")),
		MaxPartBytes:          w.cfg.UploadMaxPartBytes,
		MaxPartDurationSecs:   w.cfg.UploadMaxPartDurationSecs,
		OutputBaseName:        outputBaseName,
	})
	if err != nil {
		terminal := job.Attempts >= job.MaxAttempts
		message := truncateError(err)
		if markErr := store.MarkUploadSourceMergeFailed(ctx, source.ID, terminal, message); markErr != nil {
			return markErr
		}
		return w.failJob(ctx, job, "TRANSIENT", err)
	}
	if err := store.MarkUploadSourcePackageSucceeded(ctx, source.ID, result.Outputs); err != nil {
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

func (w Worker) runApplyUploadSourceEditJob(ctx context.Context, job workerJob) error {
	var payload mergeJobPayload
	if err := json.Unmarshal([]byte(job.PayloadJSON), &payload); err != nil {
		return w.failJob(ctx, job, "PERMANENT", fmt.Errorf("decode edit payload: %w", err))
	}
	store := recording.NewStore(w.db, w.cfg)
	source, cuts, err := store.UploadSourceForEdit(ctx, payload.UploadSourceID)
	if err != nil {
		return w.failJob(ctx, job, "PERMANENT", err)
	}
	outputs := make([]media.EditOutput, 0, len(source.Outputs))
	for _, output := range source.Outputs {
		outputs = append(outputs, media.EditOutput{
			RelativePath:    output.RelativePath,
			SizeBytes:       output.SizeBytes,
			DurationMs:      output.DurationMs,
			TimelineStartMs: output.TimelineStartMs,
			TimelineEndMs:   output.TimelineEndMs,
		})
	}
	outputBaseName, err := store.UploadSourcePackageBaseName(ctx, source)
	if err != nil {
		return w.failJob(ctx, job, "PERMANENT", err)
	}
	result, err := w.editor.ApplyCuts(ctx, media.EditRequest{
		UploadSourceID:        source.ID,
		Outputs:               outputs,
		Cuts:                  cuts,
		OutputDirRelativePath: filepath.ToSlash(filepath.Join("upload-sources", fmt.Sprintf("%d", source.RecordingProfileID), fmt.Sprintf("%d", source.ID), "edited")),
		OutputBaseName:        outputBaseName + "-edited",
	})
	if err != nil {
		message := truncateError(err)
		if markErr := store.MarkUploadSourceEditFailed(ctx, source.ID, message); markErr != nil {
			return markErr
		}
		return w.failJob(ctx, job, "TRANSIENT", err)
	}
	if err := store.MarkUploadSourceEditSucceeded(ctx, source.ID, result.Outputs); err != nil {
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
	result, err := w.cos.Upload(ctx, request, w.progressReporter(job))
	if err != nil {
		return w.failUploadJob(ctx, job, request.ObjectID, classifyUploadError(err), err)
	}
	if err := store.MarkCOSObjectUploaded(ctx, request.ObjectID, result); err != nil {
		return w.failJob(ctx, job, "PERMANENT", err)
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
	result, err := w.cos.Upload(ctx, request, w.progressReporter(job))
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
	result, err := w.bilibili.Upload(ctx, request, w.progressReporter(job))
	if err != nil {
		return w.failBilibiliJob(ctx, job, request.PublicationID, classifyUploadError(err), err)
	}
	if err := store.MarkBilibiliUploaded(ctx, request.PublicationID, result); err != nil {
		return w.failJob(ctx, job, "PERMANENT", err)
	}
	return w.succeedJob(ctx, job, recorder.RuntimeStatus{})
}

func (w Worker) runSiteTLSSyncJob(ctx context.Context, job workerJob) error {
	store := sitetls.NewStore(w.db, w.cfg)
	request, err := store.SyncRequest(ctx)
	if err != nil {
		class := classifySiteTLSError(err)
		_ = store.MarkError(ctx, truncateError(err))
		return w.failJob(ctx, job, class, err)
	}
	result, err := w.siteTLS.Sync(ctx, request)
	if err != nil {
		class := classifySiteTLSError(err)
		_ = store.MarkError(ctx, truncateError(err))
		return w.failJob(ctx, job, class, err)
	}
	if err := store.MarkChecked(ctx, result); err != nil {
		return w.failJob(ctx, job, "PERMANENT", err)
	}
	return w.succeedJob(ctx, job, recorder.RuntimeStatus{})
}

func (w Worker) discoverUploadSources(ctx context.Context) error {
	if err := sitetls.NewStore(w.db, w.cfg).Reconcile(ctx); err != nil {
		log.Printf("site TLS reconcile failed: %v", err)
	}
	recordingStore := recording.NewStore(w.db, w.cfg)
	if _, err := recordingStore.ReconcileLocal(ctx, accountSuperAdmin()); err != nil {
		return err
	}
	if _, err := recordingStore.RepairUploadSources(ctx, accountSuperAdmin()); err != nil {
		return err
	}
	if _, err := recordingStore.DiscoverUploadSources(ctx, recording.DefaultMergeGapThresholdSeconds); err != nil {
		return err
	}
	if _, err := upload.NewStore(w.db, w.cfg).Reconcile(ctx, accountSuperAdmin()); err != nil {
		return err
	}
	cleanup, err := recordingStore.RunAutomaticUploadSourceCleanup(ctx, 10)
	if err == nil && cleanup.DeletedRecordings > 0 {
		log.Printf("automatic local cleanup deleted %d recordings and %d files, estimated reclaimed bytes %d", cleanup.DeletedRecordings, cleanup.DeletedFiles, cleanup.ReclaimedBytes)
	}
	return err
}

func accountSuperAdmin() account.User {
	return account.User{Role: account.RoleSuperAdmin}
}

func (w Worker) claimJob(ctx context.Context) (workerJob, error) {
	return w.claimJobWhere(ctx, "", nil)
}

func (w Worker) claimJobForResource(ctx context.Context, resourceClass string) (workerJob, error) {
	return w.claimJobWhere(ctx, "AND resource_class = ?", []interface{}{resourceClass})
}

func (w Worker) claimJobWhere(ctx context.Context, extraWhere string, extraArgs []interface{}) (workerJob, error) {
	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		return workerJob{}, fmt.Errorf("begin job claim: %w", err)
	}
	defer tx.Rollback()

	var job workerJob
	args := append([]interface{}{}, extraArgs...)
	err = tx.QueryRowContext(ctx, `
		SELECT id, type, COALESCE(recording_profile_id, 0), COALESCE(payload_json, ''), attempts, max_attempts
		FROM jobs
		WHERE type IN ('SYNC_RECORDER_PROFILE', 'MERGE_UPLOAD_SOURCE', 'PACKAGE_UPLOAD_SOURCE', 'APPLY_UPLOAD_SOURCE_EDIT', 'UPLOAD_COS_OBJECT', 'UPLOAD_COS_RECORDING_FILE', 'UPLOAD_BILIBILI', 'SYNC_SITE_TLS', 'DOWNLOAD_SONG_SOURCE', 'PROCESS_SONG_ANALYSIS')
			AND status = 'PENDING'
			AND run_after <= CURRENT_TIMESTAMP
			AND NOT EXISTS (
				SELECT 1 FROM system_settings
				WHERE key = 'worker_drain' AND json_extract(value_json, '$') = 1
			)
			`+extraWhere+`
		ORDER BY priority ASC, run_after ASC, id ASC
		LIMIT 1
	`, args...).Scan(&job.ID, &job.Type, &job.RecordingProfileID, &job.PayloadJSON, &job.Attempts, &job.MaxAttempts)
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
			progress_current_bytes = 0,
			progress_total_bytes = 0,
			progress_message = NULL,
			progress_updated_at = NULL,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND status = 'PENDING'
			AND NOT EXISTS (
				SELECT 1 FROM system_settings
				WHERE key = 'worker_drain' AND json_extract(value_json, '$') = 1
			)
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
			progress_current_bytes = CASE WHEN progress_total_bytes > 0 THEN progress_total_bytes ELSE progress_current_bytes END,
			progress_message = NULL,
			progress_updated_at = CASE WHEN progress_total_bytes > 0 THEN CURRENT_TIMESTAMP ELSE progress_updated_at END,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND status = 'RUNNING'
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

func classifySiteTLSError(err error) string {
	var classified interface{ ErrorClass() string }
	if errors.As(err, &classified) {
		return classified.ErrorClass()
	}
	if errors.Is(err, sitetls.ErrValidation) || errors.Is(err, sitetls.ErrForbidden) || errors.Is(err, sitetls.ErrNotFound) {
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
			progress_message = NULL,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND status = 'RUNNING'
	`, runAfter), nextStatus, errorClass, message, job.ID); err != nil {
		return fmt.Errorf("mark job failed: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit job failure: %w", err)
	}
	return nil
}

func (w Worker) progressReporter(job workerJob) upload.ProgressReporter {
	return func(ctx context.Context, progress upload.UploadProgress) {
		if progress.TotalBytes < 0 || progress.CurrentBytes < 0 {
			return
		}
		if progress.TotalBytes > 0 && progress.CurrentBytes > progress.TotalBytes {
			progress.CurrentBytes = progress.TotalBytes
		}
		if err := w.updateJobProgress(ctx, job.ID, progress); err != nil {
			log.Printf("update job progress failed: %v", err)
		}
	}
}

func (w Worker) updateJobProgress(ctx context.Context, jobID int64, progress upload.UploadProgress) error {
	if jobID <= 0 {
		return nil
	}
	message := strings.TrimSpace(progress.Message)
	if len(message) > 160 {
		message = message[:160]
	}
	_, err := w.db.ExecContext(ctx, `
		UPDATE jobs
		SET progress_current_bytes = ?,
			progress_total_bytes = ?,
			progress_message = NULLIF(?, ''),
			progress_updated_at = CURRENT_TIMESTAMP,
			heartbeat_at = CURRENT_TIMESTAMP,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND status = 'RUNNING'
	`, progress.CurrentBytes, progress.TotalBytes, message, jobID)
	if err != nil {
		return fmt.Errorf("update job progress: %w", err)
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
