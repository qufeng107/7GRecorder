package songs

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/7grecorder/7grecorder/backend/internal/account"
	"github.com/7grecorder/7grecorder/backend/internal/config"
	"github.com/7grecorder/7grecorder/backend/internal/secretbox"
)

var (
	ErrForbidden       = errors.New("songs forbidden")
	ErrNotFound        = errors.New("songs resource not found")
	ErrNotReady        = errors.New("songs source not ready")
	ErrWaitingForSpace = errors.New("songs waiting for managed local space")
	ErrValidation      = errors.New("songs validation failed")
)

type Settings struct {
	Enabled                        bool   `json:"enabled"`
	CredentialID                   int64  `json:"credential_id,omitempty"`
	Region                         string `json:"region"`
	ContainerID                    string `json:"container_id"`
	DestinationCOSStorageProfileID int64  `json:"destination_cos_storage_profile_id,omitempty"`
	SongsPrefix                    string `json:"songs_prefix"`
	BoundaryPaddingMs              int64  `json:"boundary_padding_ms"`
	AlgorithmVersion               string `json:"algorithm_version"`
	UpdatedAt                      string `json:"updated_at"`
}

type SettingsUpsert struct {
	Enabled                        bool   `json:"enabled"`
	CredentialID                   int64  `json:"credential_id"`
	Region                         string `json:"region"`
	ContainerID                    string `json:"container_id"`
	DestinationCOSStorageProfileID int64  `json:"destination_cos_storage_profile_id"`
	SongsPrefix                    string `json:"songs_prefix"`
	BoundaryPaddingMs              int64  `json:"boundary_padding_ms"`
	AlgorithmVersion               string `json:"algorithm_version"`
}

type AnalysisSource struct {
	COSObjectID          int64  `json:"cos_object_id"`
	COSStorageProfileID  int64  `json:"cos_storage_profile_id"`
	UploadSourceID       int64  `json:"upload_source_id"`
	OutputID             int64  `json:"output_id"`
	RecordingProfileID   int64  `json:"recording_profile_id"`
	ProfileName          string `json:"profile_name"`
	ObjectKey            string `json:"object_key"`
	ETag                 string `json:"etag,omitempty"`
	SizeBytes            int64  `json:"size_bytes"`
	TimelineStartMs      int64  `json:"timeline_start_ms"`
	TimelineEndMs        int64  `json:"timeline_end_ms"`
	UploadSourceStarted  string `json:"upload_source_started_at"`
	UploadSourceComplete string `json:"upload_source_completed_at"`
}

type AnalysisRun struct {
	ID                 int64  `json:"id"`
	SourceCOSObjectID  int64  `json:"source_cos_object_id"`
	UploadSourceID     int64  `json:"upload_source_id"`
	OutputID           int64  `json:"upload_source_output_id"`
	RecordingProfileID int64  `json:"recording_profile_id"`
	SourceObjectKey    string `json:"source_object_key"`
	SourceETag         string `json:"source_etag,omitempty"`
	SourceSizeBytes    int64  `json:"source_size_bytes"`
	TimelineStartMs    int64  `json:"source_timeline_start_ms"`
	TimelineEndMs      int64  `json:"source_timeline_end_ms"`
	Status             string `json:"status"`
	ProgressMessage    string `json:"progress_message,omitempty"`
	LastErrorClass     string `json:"last_error_class,omitempty"`
	LastError          string `json:"last_error,omitempty"`
	CreatedAt          string `json:"created_at"`
	UpdatedAt          string `json:"updated_at"`
}

type CreateRunRequest struct {
	COSObjectID int64 `json:"cos_object_id"`
}

type DownloadJobPayload struct {
	AnalysisRunID int64 `json:"analysis_run_id"`
}

type DownloadRequest struct {
	AnalysisRunID int64
	Region        string
	Bucket        string
	ObjectKey     string
	ExpectedETag  string
	ExpectedSize  int64
	Destination   string
	SecretID      string
	SecretKey     string
	SessionToken  string
}

type Store struct {
	db  *sql.DB
	cfg config.Config
}

func NewStore(database *sql.DB, cfg config.Config) Store {
	return Store{db: database, cfg: cfg}
}

func (s Store) GetSettings(ctx context.Context, actor account.User) (Settings, error) {
	if actor.Role != account.RoleSuperAdmin {
		return Settings{}, ErrForbidden
	}
	return s.getSettings(ctx)
}

func (s Store) UpsertSettings(ctx context.Context, actor account.User, req SettingsUpsert) (Settings, error) {
	if actor.Role != account.RoleSuperAdmin {
		return Settings{}, ErrForbidden
	}
	req.Region = strings.TrimSpace(req.Region)
	req.ContainerID = strings.TrimSpace(req.ContainerID)
	req.SongsPrefix = strings.Trim(strings.TrimSpace(req.SongsPrefix), "/")
	req.AlgorithmVersion = strings.TrimSpace(req.AlgorithmVersion)
	if req.SongsPrefix == "" {
		req.SongsPrefix = "songs"
	}
	if req.AlgorithmVersion == "" {
		req.AlgorithmVersion = "v1"
	}
	if req.BoundaryPaddingMs < 0 || strings.Contains(req.SongsPrefix, "..") {
		return Settings{}, ErrValidation
	}
	if req.Enabled && (req.CredentialID <= 0 || req.Region == "" || req.ContainerID == "" || req.DestinationCOSStorageProfileID <= 0) {
		return Settings{}, ErrValidation
	}
	if req.CredentialID > 0 {
		var count int
		if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM credentials WHERE id = ? AND scope = 'SYSTEM' AND platform = 'acrcloud' AND purpose = 'SONG_RECOGNITION'`, req.CredentialID).Scan(&count); err != nil {
			return Settings{}, err
		}
		if count != 1 {
			return Settings{}, ErrValidation
		}
	}
	if req.DestinationCOSStorageProfileID > 0 {
		var count int
		if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM cos_storage_profiles WHERE id = ? AND enabled = 1`, req.DestinationCOSStorageProfileID).Scan(&count); err != nil {
			return Settings{}, err
		}
		if count != 1 {
			return Settings{}, ErrValidation
		}
	}
	_, err := s.db.ExecContext(ctx, `UPDATE song_settings SET enabled = ?, credential_id = NULLIF(?, 0), region = ?,
		container_id = ?, destination_cos_storage_profile_id = NULLIF(?, 0), songs_prefix = ?, boundary_padding_ms = ?,
		algorithm_version = ?, updated_at = CURRENT_TIMESTAMP WHERE id = 1`, boolInt(req.Enabled), req.CredentialID,
		req.Region, req.ContainerID, req.DestinationCOSStorageProfileID, req.SongsPrefix, req.BoundaryPaddingMs, req.AlgorithmVersion)
	if err != nil {
		return Settings{}, fmt.Errorf("update song settings: %w", err)
	}
	return s.getSettings(ctx)
}

func (s Store) ListSources(ctx context.Context, actor account.User) ([]AnalysisSource, error) {
	if actor.Role != account.RoleSuperAdmin {
		return nil, ErrForbidden
	}
	rows, err := s.db.QueryContext(ctx, `SELECT co.id, co.cos_storage_profile_id, co.upload_source_id, uso.id, co.recording_profile_id,
		p.name, co.object_key, COALESCE(co.etag, ''), co.size_bytes, uso.timeline_start_ms, uso.timeline_end_ms,
		us.started_at, us.completed_at
		FROM upload_source_cos_objects co
		JOIN upload_sources us ON us.id = co.upload_source_id AND us.status = 'READY_TO_UPLOAD'
		JOIN upload_source_outputs uso ON uso.id = co.upload_source_output_id AND uso.status = 'READY_TO_UPLOAD'
		JOIN recording_profiles p ON p.id = co.recording_profile_id
		WHERE co.status = 'AVAILABLE' AND co.deleted_at IS NULL AND uso.timeline_end_ms > uso.timeline_start_ms
		ORDER BY us.started_at DESC, co.id DESC`)
	if err != nil {
		return nil, fmt.Errorf("list song analysis sources: %w", err)
	}
	defer rows.Close()
	items := make([]AnalysisSource, 0)
	for rows.Next() {
		var item AnalysisSource
		if err := rows.Scan(&item.COSObjectID, &item.COSStorageProfileID, &item.UploadSourceID, &item.OutputID, &item.RecordingProfileID,
			&item.ProfileName, &item.ObjectKey, &item.ETag, &item.SizeBytes, &item.TimelineStartMs, &item.TimelineEndMs,
			&item.UploadSourceStarted, &item.UploadSourceComplete); err != nil {
			return nil, fmt.Errorf("scan song analysis source: %w", err)
		}
		if isVideoObject(item.ObjectKey) {
			items = append(items, item)
		}
	}
	return items, rows.Err()
}

func (s Store) CreateRun(ctx context.Context, actor account.User, req CreateRunRequest) (AnalysisRun, error) {
	if actor.Role != account.RoleSuperAdmin {
		return AnalysisRun{}, ErrForbidden
	}
	if req.COSObjectID <= 0 {
		return AnalysisRun{}, ErrValidation
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return AnalysisRun{}, err
	}
	defer tx.Rollback()
	var enabled int
	var credentialID, destinationProfileID int64
	var region, containerID, prefix, algorithm string
	var padding int64
	if err := tx.QueryRowContext(ctx, `SELECT enabled, COALESCE(credential_id, 0), region, container_id,
		COALESCE(destination_cos_storage_profile_id, 0), songs_prefix, boundary_padding_ms, algorithm_version
		FROM song_settings WHERE id = 1`).Scan(&enabled, &credentialID, &region, &containerID, &destinationProfileID, &prefix, &padding, &algorithm); err != nil {
		return AnalysisRun{}, err
	}
	if enabled != 1 || credentialID <= 0 || destinationProfileID <= 0 {
		return AnalysisRun{}, ErrNotReady
	}
	var source AnalysisSource
	var sourceCOSProfileID int64
	if err := tx.QueryRowContext(ctx, `SELECT co.id, co.upload_source_id, uso.id, co.recording_profile_id,
		co.cos_storage_profile_id, co.object_key, COALESCE(co.etag, ''), co.size_bytes,
		uso.timeline_start_ms, uso.timeline_end_ms
		FROM upload_source_cos_objects co
		JOIN upload_sources us ON us.id = co.upload_source_id AND us.status = 'READY_TO_UPLOAD'
		JOIN upload_source_outputs uso ON uso.id = co.upload_source_output_id AND uso.status = 'READY_TO_UPLOAD'
		WHERE co.id = ? AND co.status = 'AVAILABLE' AND co.deleted_at IS NULL`, req.COSObjectID).Scan(
		&source.COSObjectID, &source.UploadSourceID, &source.OutputID, &source.RecordingProfileID, &sourceCOSProfileID,
		&source.ObjectKey, &source.ETag, &source.SizeBytes, &source.TimelineStartMs, &source.TimelineEndMs); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return AnalysisRun{}, ErrNotReady
		}
		return AnalysisRun{}, err
	}
	if !isVideoObject(source.ObjectKey) || source.SizeBytes <= 0 || source.TimelineEndMs <= source.TimelineStartMs {
		return AnalysisRun{}, ErrNotReady
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO song_analysis_runs
		(source_cos_object_id, upload_source_id, upload_source_output_id, recording_profile_id, source_cos_storage_profile_id,
		source_object_key, source_etag, source_size_bytes, source_timeline_start_ms, source_timeline_end_ms,
		provider_region, provider_container_id, provider_credential_id, destination_cos_storage_profile_id,
		songs_prefix, boundary_padding_ms, algorithm_version, status, progress_message)
		VALUES (?, ?, ?, ?, ?, ?, NULLIF(?, ''), ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'PENDING', 'Waiting to download source')`,
		source.COSObjectID, source.UploadSourceID, source.OutputID, source.RecordingProfileID, sourceCOSProfileID,
		source.ObjectKey, source.ETag, source.SizeBytes, source.TimelineStartMs, source.TimelineEndMs, region, containerID,
		credentialID, destinationProfileID, prefix, padding, algorithm)
	if err != nil {
		return AnalysisRun{}, fmt.Errorf("create song analysis run: %w", err)
	}
	runID, err := result.LastInsertId()
	if err != nil {
		return AnalysisRun{}, err
	}
	ext := strings.ToLower(filepath.Ext(source.ObjectKey))
	if ext == "" || len(ext) > 10 {
		ext = ".video"
	}
	relativePath := filepath.ToSlash(filepath.Join("songs", "work", "runs", fmt.Sprint(runID), "source"+ext))
	if _, err := tx.ExecContext(ctx, `UPDATE song_analysis_runs SET source_relative_path = ? WHERE id = ?`, relativePath, runID); err != nil {
		return AnalysisRun{}, err
	}
	payload, _ := json.Marshal(DownloadJobPayload{AnalysisRunID: runID})
	if _, err := tx.ExecContext(ctx, `INSERT INTO jobs
		(recording_profile_id, upload_source_id, type, resource_class, business_key, payload_json, status, priority, max_attempts)
		VALUES (?, ?, 'DOWNLOAD_SONG_SOURCE', 'NETWORK', ?, ?, 'PENDING', 120, 5)`, source.RecordingProfileID,
		source.UploadSourceID, fmt.Sprintf("song-analysis:%d:download-source", runID), string(payload)); err != nil {
		return AnalysisRun{}, fmt.Errorf("schedule song source download: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return AnalysisRun{}, err
	}
	return s.GetRun(ctx, actor, runID)
}

func (s Store) ListRuns(ctx context.Context, actor account.User) ([]AnalysisRun, error) {
	if actor.Role != account.RoleSuperAdmin {
		return nil, ErrForbidden
	}
	return s.listRuns(ctx, 0)
}

func (s Store) GetRun(ctx context.Context, actor account.User, id int64) (AnalysisRun, error) {
	if actor.Role != account.RoleSuperAdmin {
		return AnalysisRun{}, ErrForbidden
	}
	items, err := s.listRuns(ctx, id)
	if err != nil {
		return AnalysisRun{}, err
	}
	if len(items) == 0 {
		return AnalysisRun{}, ErrNotFound
	}
	return items[0], nil
}

func (s Store) DownloadRequest(ctx context.Context, runID int64) (DownloadRequest, error) {
	var req DownloadRequest
	var relativePath string
	var encrypted []byte
	err := s.db.QueryRowContext(ctx, `SELECT r.id, csp.region, csp.bucket, r.source_object_key,
		COALESCE(r.source_etag, ''), r.source_size_bytes, r.source_relative_path, c.encrypted_secret
		FROM song_analysis_runs r
		JOIN cos_storage_profiles csp ON csp.id = r.source_cos_storage_profile_id
		JOIN credentials c ON c.id = csp.credential_id
		JOIN upload_source_cos_objects co ON co.id = r.source_cos_object_id
		WHERE r.id = ? AND r.status IN ('PENDING', 'DOWNLOADING') AND co.status = 'AVAILABLE' AND co.deleted_at IS NULL`, runID).Scan(
		&req.AnalysisRunID, &req.Region, &req.Bucket, &req.ObjectKey, &req.ExpectedETag, &req.ExpectedSize, &relativePath, &encrypted)
	if errors.Is(err, sql.ErrNoRows) {
		return DownloadRequest{}, ErrNotReady
	}
	if err != nil {
		return DownloadRequest{}, err
	}
	req.Destination, err = resolveWithinRoot(s.cfg.DataRoot, relativePath)
	if err != nil {
		return DownloadRequest{}, ErrValidation
	}
	plaintext, err := secretbox.Decrypt(s.cfg.MasterKeyPath, encrypted)
	if err != nil {
		return DownloadRequest{}, fmt.Errorf("decrypt source COS credential: %w", err)
	}
	var secret map[string]string
	if err := json.Unmarshal(plaintext, &secret); err != nil {
		return DownloadRequest{}, fmt.Errorf("decode source COS credential: %w", err)
	}
	req.SecretID = first(secret, "secret_id", "secretId", "SecretId", "SecretID")
	req.SecretKey = first(secret, "secret_key", "secretKey", "SecretKey")
	req.SessionToken = first(secret, "session_token", "sessionToken", "SessionToken")
	if req.SecretID == "" || req.SecretKey == "" {
		return DownloadRequest{}, ErrValidation
	}
	return req, nil
}

func (s Store) MarkDownloading(ctx context.Context, runID int64) error {
	result, err := s.db.ExecContext(ctx, `UPDATE song_analysis_runs SET status = 'DOWNLOADING',
		progress_message = 'Downloading source from COS', last_error_class = NULL, last_error = NULL,
		updated_at = CURRENT_TIMESTAMP WHERE id = ? AND status IN ('PENDING', 'DOWNLOADING')`, runID)
	if err != nil {
		return err
	}
	changed, _ := result.RowsAffected()
	if changed != 1 {
		return ErrNotReady
	}
	return nil
}

func (s Store) ReserveSourceDownload(ctx context.Context, jobID, runID int64) error {
	if jobID <= 0 || runID <= 0 {
		return ErrValidation
	}
	availableBytes, err := diskAvailableBytes(s.cfg.DataRoot)
	if err != nil {
		return fmt.Errorf("read Songs work filesystem capacity: %w", err)
	}
	result, err := s.db.ExecContext(ctx, `INSERT INTO storage_reservations
		(job_id, kind, reserved_bytes, heartbeat_at, expires_at)
		SELECT ?, 'SONG_SOURCE_DOWNLOAD', r.source_size_bytes, CURRENT_TIMESTAMP, datetime('now', '+30 minutes')
		FROM song_analysis_runs r
		JOIN local_storage_settings settings ON settings.id = 1
		WHERE r.id = ?
			AND r.source_size_bytes
				+ COALESCE((SELECT SUM(COALESCE(size_bytes, 0)) FROM recording_files WHERE kind = 'video' AND file_status != 'DELETED' AND deleted_at IS NULL), 0)
				+ COALESCE((SELECT SUM(size_bytes) FROM media_cache_entries WHERE status IN ('DOWNLOADING', 'GENERATING', 'AVAILABLE')), 0)
				+ COALESCE((SELECT SUM(reserved_bytes) FROM storage_reservations WHERE expires_at > CURRENT_TIMESTAMP AND job_id != ?), 0)
				<= settings.max_recording_bytes
			AND r.source_size_bytes
				+ COALESCE((SELECT SUM(reserved_bytes) FROM storage_reservations WHERE expires_at > CURRENT_TIMESTAMP AND job_id != ?), 0)
				<= MAX(? - MAX(settings.min_system_free_bytes, settings.absolute_emergency_free_bytes), 0)
		ON CONFLICT(job_id) DO UPDATE SET reserved_bytes = excluded.reserved_bytes,
			heartbeat_at = CURRENT_TIMESTAMP, expires_at = datetime('now', '+30 minutes'), updated_at = CURRENT_TIMESTAMP`, jobID, runID, jobID, jobID, availableBytes)
	if err != nil {
		return fmt.Errorf("reserve song source download space: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed != 1 {
		return ErrWaitingForSpace
	}
	return nil
}

func (s Store) MarkDownloaded(ctx context.Context, jobID, runID int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var relativePath string
	var size int64
	if err := tx.QueryRowContext(ctx, `SELECT source_relative_path, source_size_bytes FROM song_analysis_runs WHERE id = ? AND status = 'DOWNLOADING'`, runID).Scan(&relativePath, &size); err != nil {
		return err
	}
	cacheKey := fmt.Sprintf("song-analysis:%d:source", runID)
	if _, err := tx.ExecContext(ctx, `INSERT INTO media_cache_entries
		(kind, cache_key, relative_path, size_bytes, status, last_accessed_at, grace_until)
		VALUES ('SONG_SOURCE_WORK', ?, ?, ?, 'AVAILABLE', CURRENT_TIMESTAMP, datetime('now', '+24 hours'))
		ON CONFLICT(cache_key) DO UPDATE SET relative_path = excluded.relative_path, size_bytes = excluded.size_bytes,
			status = 'AVAILABLE', last_error = NULL, last_accessed_at = CURRENT_TIMESTAMP,
			grace_until = datetime('now', '+24 hours'), updated_at = CURRENT_TIMESTAMP`, cacheKey, relativePath, size); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM storage_reservations WHERE job_id = ?`, jobID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE song_analysis_runs SET status = 'ANALYZING',
		progress_message = 'Waiting to extract analysis audio', updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND status = 'DOWNLOADING'`, runID); err != nil {
		return err
	}
	payload, _ := json.Marshal(ProcessJobPayload{AnalysisRunID: runID})
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO jobs
		(recording_profile_id, upload_source_id, type, resource_class, business_key, payload_json, status, priority, max_attempts)
		SELECT recording_profile_id, upload_source_id, 'PROCESS_SONG_ANALYSIS', 'AI', ?, ?, 'PENDING', 125, 3
		FROM song_analysis_runs WHERE id = ?`, fmt.Sprintf("song-analysis:%d:process", runID), string(payload), runID); err != nil {
		return fmt.Errorf("schedule song analysis: %w", err)
	}
	return tx.Commit()
}

func (s Store) ReleaseReservation(ctx context.Context, jobID int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM storage_reservations WHERE job_id = ?`, jobID)
	return err
}

func (s Store) HeartbeatReservation(ctx context.Context, jobID int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE storage_reservations SET heartbeat_at = CURRENT_TIMESTAMP,
		expires_at = datetime('now', '+30 minutes'), updated_at = CURRENT_TIMESTAMP WHERE job_id = ?`, jobID)
	return err
}

func (s Store) MarkDownloadFailed(ctx context.Context, runID int64, class, message string, terminal bool) error {
	status := "PENDING"
	if terminal || class == "AUTH" || class == "SOURCE_MISSING" || class == "PERMANENT" {
		status = "FAILED"
	}
	_, err := s.db.ExecContext(ctx, `UPDATE song_analysis_runs SET status = ?, progress_message = NULL,
		last_error_class = ?, last_error = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, status, class, truncate(message, 1000), runID)
	return err
}

func (s Store) getSettings(ctx context.Context) (Settings, error) {
	var item Settings
	var enabled int
	err := s.db.QueryRowContext(ctx, `SELECT enabled, COALESCE(credential_id, 0), region, container_id,
		COALESCE(destination_cos_storage_profile_id, 0), songs_prefix, boundary_padding_ms, algorithm_version, updated_at
		FROM song_settings WHERE id = 1`).Scan(&enabled, &item.CredentialID, &item.Region, &item.ContainerID,
		&item.DestinationCOSStorageProfileID, &item.SongsPrefix, &item.BoundaryPaddingMs, &item.AlgorithmVersion, &item.UpdatedAt)
	item.Enabled = enabled == 1
	return item, err
}

func (s Store) listRuns(ctx context.Context, id int64) ([]AnalysisRun, error) {
	query := `SELECT id, source_cos_object_id, upload_source_id, upload_source_output_id, recording_profile_id,
		source_object_key, COALESCE(source_etag, ''), source_size_bytes, source_timeline_start_ms, source_timeline_end_ms,
		status, COALESCE(progress_message, ''), COALESCE(last_error_class, ''), COALESCE(last_error, ''), created_at, updated_at
		FROM song_analysis_runs`
	args := []any{}
	if id > 0 {
		query += " WHERE id = ?"
		args = append(args, id)
	}
	query += " ORDER BY created_at DESC, id DESC"
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]AnalysisRun, 0)
	for rows.Next() {
		var item AnalysisRun
		if err := rows.Scan(&item.ID, &item.SourceCOSObjectID, &item.UploadSourceID, &item.OutputID, &item.RecordingProfileID,
			&item.SourceObjectKey, &item.SourceETag, &item.SourceSizeBytes, &item.TimelineStartMs, &item.TimelineEndMs,
			&item.Status, &item.ProgressMessage, &item.LastErrorClass, &item.LastError, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func isVideoObject(key string) bool {
	switch strings.ToLower(filepath.Ext(key)) {
	case ".flv", ".mp4", ".mkv", ".mov", ".ts", ".m4v", ".webm":
		return true
	default:
		return false
	}
}

func resolveWithinRoot(root, relative string) (string, error) {
	if root == "" || relative == "" || filepath.IsAbs(relative) {
		return "", ErrValidation
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	target, err := filepath.Abs(filepath.Join(rootAbs, filepath.FromSlash(relative)))
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(rootAbs, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", ErrValidation
	}
	return target, nil
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
func first(values map[string]string, keys ...string) string {
	for _, key := range keys {
		if strings.TrimSpace(values[key]) != "" {
			return strings.TrimSpace(values[key])
		}
	}
	return ""
}
func truncate(value string, max int) string {
	value = strings.TrimSpace(value)
	if len(value) > max {
		return value[:max]
	}
	return value
}
