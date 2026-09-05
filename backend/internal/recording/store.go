package recording

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/7grecorder/7grecorder/backend/internal/account"
	"github.com/7grecorder/7grecorder/backend/internal/config"
)

var (
	ErrForbidden  = errors.New("recording forbidden")
	ErrNotFound   = errors.New("recording not found")
	ErrNotReady   = errors.New("recording file not ready")
	ErrValidation = errors.New("recording validation failed")
)

const gibibyte int64 = 1024 * 1024 * 1024

type Recording struct {
	ID                 int64  `json:"id"`
	RecordingProfileID int64  `json:"recording_profile_id"`
	ProfileName        string `json:"profile_name"`
	RoomID             string `json:"room_id"`
	StreamerName       string `json:"streamer_name"`
	Title              string `json:"title,omitempty"`
	StartedAt          string `json:"started_at"`
	CompletedAt        string `json:"completed_at,omitempty"`
	DurationMs         int64  `json:"duration_ms"`
	RecordingStatus    string `json:"recording_status"`
	LocalStorageStatus string `json:"local_storage_status"`
	LocalProtected     bool   `json:"local_protected"`
	Files              []File `json:"files"`
}

type File struct {
	ID           int64  `json:"id"`
	RecordingID  int64  `json:"recording_id"`
	RelativePath string `json:"relative_path"`
	OriginalName string `json:"original_name"`
	Kind         string `json:"kind"`
	FileStatus   string `json:"file_status"`
	SizeBytes    int64  `json:"size_bytes"`
	DurationMs   int64  `json:"duration_ms"`
	ClosedAt     string `json:"closed_at,omitempty"`
	UpdatedAt    string `json:"updated_at"`
}

type ReconcileResult struct {
	ScannedFiles int `json:"scanned_files"`
	Imported     int `json:"imported"`
	Updated      int `json:"updated"`
	Skipped      int `json:"skipped"`
}

type LocalStorageStatus struct {
	DataRoot            string               `json:"data_root"`
	DiskTotalBytes      int64                `json:"disk_total_bytes"`
	DiskFreeBytes       int64                `json:"disk_free_bytes"`
	DiskAvailableBytes  int64                `json:"disk_available_bytes"`
	IndexedVideoBytes   int64                `json:"indexed_video_bytes"`
	IndexedVideoFiles   int64                `json:"indexed_video_files"`
	ProtectedRecordings int64                `json:"protected_recordings"`
	CompletedRecordings int64                `json:"completed_recordings"`
	SettingsConfigured  bool                 `json:"settings_configured"`
	Health              string               `json:"health"`
	NeedReclaimBytes    int64                `json:"need_reclaim_bytes"`
	TargetVideoBytes    int64                `json:"target_video_bytes"`
	Settings            LocalStorageSettings `json:"settings"`
}

type LocalStorageSettings struct {
	MaxRecordingBytes           int64   `json:"max_recording_bytes"`
	MinSystemFreeBytes         int64   `json:"min_system_free_bytes"`
	CleanupTargetRatio         float64 `json:"cleanup_target_ratio"`
	AbsoluteEmergencyFreeBytes int64   `json:"absolute_emergency_free_bytes"`
	UpdatedAt                  string  `json:"updated_at,omitempty"`
}

type LocalStorageSettingsUpsert struct {
	MaxRecordingBytes           int64   `json:"max_recording_bytes"`
	MinSystemFreeBytes         int64   `json:"min_system_free_bytes"`
	CleanupTargetRatio         float64 `json:"cleanup_target_ratio"`
	AbsoluteEmergencyFreeBytes int64   `json:"absolute_emergency_free_bytes"`
}

type CleanupCandidate struct {
	RecordingID      int64  `json:"recording_id"`
	ProfileName      string `json:"profile_name"`
	RoomID           string `json:"room_id"`
	StreamerName     string `json:"streamer_name"`
	Title            string `json:"title,omitempty"`
	StartedAt        string `json:"started_at"`
	CompletedAt      string `json:"completed_at,omitempty"`
	DurationMs       int64  `json:"duration_ms"`
	FileCount        int64  `json:"file_count"`
	ReclaimableBytes int64  `json:"reclaimable_bytes"`
}

type CleanupCandidateList struct {
	Items                   []CleanupCandidate `json:"items"`
	Total                   int                `json:"total"`
	PreviewReclaimableBytes int64             `json:"preview_reclaimable_bytes"`
}

type Store struct {
	db  *sql.DB
	cfg config.Config
}

func NewStore(db *sql.DB, cfg config.Config) Store {
	return Store{db: db, cfg: cfg}
}

func (s Store) List(ctx context.Context, actor account.User) ([]Recording, error) {
	query := `
		SELECT rec.id, rec.recording_profile_id, p.name, rec.source_room_id, rec.streamer_name_snapshot,
			COALESCE(rec.title, ''), rec.started_at, COALESCE(rec.completed_at, ''),
			COALESCE(rec.duration_ms, 0), rec.recording_status, rec.local_storage_status, rec.local_protected
		FROM recordings rec
		JOIN recording_profiles p ON p.id = rec.recording_profile_id
	`
	args := []interface{}{}
	if actor.Role != account.RoleSuperAdmin {
		query += " WHERE p.owner_user_id = ?"
		args = append(args, actor.ID)
	}
	query += " ORDER BY rec.started_at DESC, rec.id DESC LIMIT 100"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list recordings: %w", err)
	}
	defer rows.Close()

	items := make([]Recording, 0)
	for rows.Next() {
		var item Recording
		var localProtected int
		if err := rows.Scan(
			&item.ID,
			&item.RecordingProfileID,
			&item.ProfileName,
			&item.RoomID,
			&item.StreamerName,
			&item.Title,
			&item.StartedAt,
			&item.CompletedAt,
			&item.DurationMs,
			&item.RecordingStatus,
			&item.LocalStorageStatus,
			&localProtected,
		); err != nil {
			return nil, fmt.Errorf("scan recording: %w", err)
		}
		item.LocalProtected = localProtected == 1
		item.Files, err = s.files(ctx, item.ID)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate recordings: %w", err)
	}
	return items, nil
}

func (s Store) LocalStorageStatus(ctx context.Context, actor account.User) (LocalStorageStatus, error) {
	if actor.Role != account.RoleSuperAdmin {
		return LocalStorageStatus{}, ErrForbidden
	}
	if err := os.MkdirAll(s.cfg.DataRoot, 0o755); err != nil {
		return LocalStorageStatus{}, fmt.Errorf("ensure data root: %w", err)
	}

	var stat syscall.Statfs_t
	if err := syscall.Statfs(s.cfg.DataRoot, &stat); err != nil {
		return LocalStorageStatus{}, fmt.Errorf("stat data root filesystem: %w", err)
	}

	status := LocalStorageStatus{
		DataRoot:           s.cfg.DataRoot,
		DiskTotalBytes:     int64(stat.Blocks) * int64(stat.Bsize),
		DiskFreeBytes:      int64(stat.Bfree) * int64(stat.Bsize),
		DiskAvailableBytes: int64(stat.Bavail) * int64(stat.Bsize),
	}
	if err := s.db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(COALESCE(size_bytes, 0)), 0), COUNT(*)
		FROM recording_files
		WHERE kind = 'video' AND file_status != 'DELETED' AND deleted_at IS NULL
	`).Scan(&status.IndexedVideoBytes, &status.IndexedVideoFiles); err != nil {
		return LocalStorageStatus{}, fmt.Errorf("summarize recording files: %w", err)
	}
	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM recordings
		WHERE local_protected = 1 AND local_deleted_at IS NULL
	`).Scan(&status.ProtectedRecordings); err != nil {
		return LocalStorageStatus{}, fmt.Errorf("count protected recordings: %w", err)
	}
	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM recordings
		WHERE recording_status = 'COMPLETED' AND local_deleted_at IS NULL
	`).Scan(&status.CompletedRecordings); err != nil {
		return LocalStorageStatus{}, fmt.Errorf("count completed recordings: %w", err)
	}
	settings, configured, err := s.localStorageSettings(ctx, status.DiskTotalBytes)
	if err != nil {
		return LocalStorageStatus{}, err
	}
	status.Settings = settings
	status.SettingsConfigured = configured
	status.Health, status.NeedReclaimBytes, status.TargetVideoBytes = storagePolicyPreview(status, settings)
	return status, nil
}

func (s Store) UpsertLocalStorageSettings(ctx context.Context, actor account.User, req LocalStorageSettingsUpsert) (LocalStorageSettings, error) {
	if actor.Role != account.RoleSuperAdmin {
		return LocalStorageSettings{}, ErrForbidden
	}
	if err := validateLocalStorageSettings(req); err != nil {
		return LocalStorageSettings{}, err
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO local_storage_settings
			(id, max_recording_bytes, min_system_free_bytes, cleanup_target_ratio, absolute_emergency_free_bytes, updated_by_user_id)
		VALUES (1, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			max_recording_bytes = excluded.max_recording_bytes,
			min_system_free_bytes = excluded.min_system_free_bytes,
			cleanup_target_ratio = excluded.cleanup_target_ratio,
			absolute_emergency_free_bytes = excluded.absolute_emergency_free_bytes,
			updated_at = CURRENT_TIMESTAMP,
			updated_by_user_id = excluded.updated_by_user_id
	`, req.MaxRecordingBytes, req.MinSystemFreeBytes, req.CleanupTargetRatio, req.AbsoluteEmergencyFreeBytes, actor.ID)
	if err != nil {
		return LocalStorageSettings{}, fmt.Errorf("upsert local storage settings: %w", err)
	}
	settings, _, err := s.localStorageSettings(ctx, 0)
	return settings, err
}

func (s Store) CleanupCandidates(ctx context.Context, actor account.User, limit int) (CleanupCandidateList, error) {
	if actor.Role != account.RoleSuperAdmin {
		return CleanupCandidateList{}, ErrForbidden
	}
	if limit <= 0 {
		limit = 10
	}
	if limit > 50 {
		limit = 50
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT rec.id, p.name, rec.source_room_id, rec.streamer_name_snapshot,
			COALESCE(rec.title, ''), rec.started_at, COALESCE(rec.completed_at, ''),
			COALESCE(rec.duration_ms, 0),
			COUNT(CASE WHEN f.kind = 'video' AND f.file_status = 'CLOSED' AND f.deleted_at IS NULL THEN 1 END),
			COALESCE(SUM(CASE
				WHEN f.kind = 'video' AND f.file_status = 'CLOSED' AND f.deleted_at IS NULL THEN COALESCE(f.size_bytes, 0)
				ELSE 0
			END), 0)
		FROM recordings rec
		JOIN recording_profiles p ON p.id = rec.recording_profile_id
		LEFT JOIN recording_files f ON f.recording_id = rec.id
		WHERE rec.recording_status = 'COMPLETED'
			AND rec.local_storage_status != 'DELETED'
			AND rec.local_deleted_at IS NULL
			AND rec.local_protected = 0
		GROUP BY rec.id, p.name, rec.source_room_id, rec.streamer_name_snapshot,
			rec.title, rec.started_at, rec.completed_at, rec.duration_ms
		HAVING SUM(CASE
			WHEN f.kind = 'video' AND f.file_status = 'CLOSED' AND f.deleted_at IS NULL THEN COALESCE(f.size_bytes, 0)
			ELSE 0
		END) > 0
		ORDER BY rec.completed_at ASC, rec.started_at ASC, rec.id ASC
		LIMIT ?
	`, limit)
	if err != nil {
		return CleanupCandidateList{}, fmt.Errorf("list cleanup candidates: %w", err)
	}
	defer rows.Close()

	items := make([]CleanupCandidate, 0)
	var previewReclaimableBytes int64
	for rows.Next() {
		var item CleanupCandidate
		if err := rows.Scan(
			&item.RecordingID,
			&item.ProfileName,
			&item.RoomID,
			&item.StreamerName,
			&item.Title,
			&item.StartedAt,
			&item.CompletedAt,
			&item.DurationMs,
			&item.FileCount,
			&item.ReclaimableBytes,
		); err != nil {
			return CleanupCandidateList{}, fmt.Errorf("scan cleanup candidate: %w", err)
		}
		previewReclaimableBytes += item.ReclaimableBytes
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return CleanupCandidateList{}, fmt.Errorf("iterate cleanup candidates: %w", err)
	}
	return CleanupCandidateList{
		Items:                   items,
		Total:                   len(items),
		PreviewReclaimableBytes: previewReclaimableBytes,
	}, nil
}

func (s Store) localStorageSettings(ctx context.Context, diskTotalBytes int64) (LocalStorageSettings, bool, error) {
	var settings LocalStorageSettings
	err := s.db.QueryRowContext(ctx, `
		SELECT max_recording_bytes, min_system_free_bytes, cleanup_target_ratio,
			absolute_emergency_free_bytes, updated_at
		FROM local_storage_settings
		WHERE id = 1
	`).Scan(
		&settings.MaxRecordingBytes,
		&settings.MinSystemFreeBytes,
		&settings.CleanupTargetRatio,
		&settings.AbsoluteEmergencyFreeBytes,
		&settings.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return defaultLocalStorageSettings(diskTotalBytes), false, nil
	}
	if err != nil {
		return LocalStorageSettings{}, false, fmt.Errorf("load local storage settings: %w", err)
	}
	return settings, true, nil
}

func defaultLocalStorageSettings(diskTotalBytes int64) LocalStorageSettings {
	if diskTotalBytes <= 0 {
		diskTotalBytes = 50 * gibibyte
	}
	minFree := diskTotalBytes / 10
	if minFree < 5*gibibyte {
		minFree = 5 * gibibyte
	}
	emergencyFree := diskTotalBytes / 20
	if emergencyFree < 2*gibibyte {
		emergencyFree = 2 * gibibyte
	}
	return LocalStorageSettings{
		MaxRecordingBytes:          diskTotalBytes * 70 / 100,
		MinSystemFreeBytes:        minFree,
		CleanupTargetRatio:        0.85,
		AbsoluteEmergencyFreeBytes: emergencyFree,
	}
}

func validateLocalStorageSettings(req LocalStorageSettingsUpsert) error {
	if req.MaxRecordingBytes <= 0 ||
		req.MinSystemFreeBytes <= 0 ||
		req.AbsoluteEmergencyFreeBytes <= 0 ||
		req.CleanupTargetRatio <= 0 ||
		req.CleanupTargetRatio >= 1 {
		return ErrValidation
	}
	if req.AbsoluteEmergencyFreeBytes > req.MinSystemFreeBytes {
		return ErrValidation
	}
	return nil
}

func storagePolicyPreview(status LocalStorageStatus, settings LocalStorageSettings) (string, int64, int64) {
	targetVideoBytes := int64(float64(settings.MaxRecordingBytes) * settings.CleanupTargetRatio)
	recordingNeed := status.IndexedVideoBytes - targetVideoBytes
	if status.IndexedVideoBytes <= settings.MaxRecordingBytes {
		recordingNeed = 0
	}
	freeNeed := settings.MinSystemFreeBytes - status.DiskAvailableBytes
	emergencyNeed := settings.AbsoluteEmergencyFreeBytes - status.DiskAvailableBytes
	need := maxInt64(recordingNeed, freeNeed, emergencyNeed, 0)

	health := "HEALTHY"
	if emergencyNeed > 0 {
		health = "CRITICAL"
	} else if need > 0 {
		health = "WARNING"
	}
	return health, need, targetVideoBytes
}

func maxInt64(values ...int64) int64 {
	var result int64
	for _, value := range values {
		if value > result {
			result = value
		}
	}
	return result
}

func (s Store) SetLocalProtected(ctx context.Context, actor account.User, recordingID int64, protected bool) (Recording, error) {
	query := `
		UPDATE recordings
		SET local_protected = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
			AND local_deleted_at IS NULL
	`
	args := []interface{}{boolInt(protected), recordingID}
	if actor.Role != account.RoleSuperAdmin {
		query += `
			AND recording_profile_id IN (
				SELECT id FROM recording_profiles WHERE owner_user_id = ?
			)
		`
		args = append(args, actor.ID)
	}
	result, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return Recording{}, fmt.Errorf("set local protected: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return Recording{}, fmt.Errorf("read local protected update count: %w", err)
	}
	if affected == 0 {
		return Recording{}, ErrNotFound
	}
	return s.get(ctx, actor, recordingID)
}

func (s Store) get(ctx context.Context, actor account.User, recordingID int64) (Recording, error) {
	query := `
		SELECT rec.id, rec.recording_profile_id, p.name, rec.source_room_id, rec.streamer_name_snapshot,
			COALESCE(rec.title, ''), rec.started_at, COALESCE(rec.completed_at, ''),
			COALESCE(rec.duration_ms, 0), rec.recording_status, rec.local_storage_status, rec.local_protected
		FROM recordings rec
		JOIN recording_profiles p ON p.id = rec.recording_profile_id
		WHERE rec.id = ?
	`
	args := []interface{}{recordingID}
	if actor.Role != account.RoleSuperAdmin {
		query += " AND p.owner_user_id = ?"
		args = append(args, actor.ID)
	}

	var item Recording
	var localProtected int
	err := s.db.QueryRowContext(ctx, query, args...).Scan(
		&item.ID,
		&item.RecordingProfileID,
		&item.ProfileName,
		&item.RoomID,
		&item.StreamerName,
		&item.Title,
		&item.StartedAt,
		&item.CompletedAt,
		&item.DurationMs,
		&item.RecordingStatus,
		&item.LocalStorageStatus,
		&localProtected,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Recording{}, ErrNotFound
	}
	if err != nil {
		return Recording{}, fmt.Errorf("get recording: %w", err)
	}
	item.LocalProtected = localProtected == 1
	item.Files, err = s.files(ctx, item.ID)
	if err != nil {
		return Recording{}, err
	}
	return item, nil
}

func (s Store) ReconcileLocal(ctx context.Context, actor account.User) (ReconcileResult, error) {
	if actor.Role != account.RoleSuperAdmin {
		return ReconcileResult{}, ErrForbidden
	}
	profiles, err := s.activeProfiles(ctx)
	if err != nil {
		return ReconcileResult{}, err
	}

	root := filepath.Join(s.cfg.DataRoot, "recordings")
	result := ReconcileResult{}
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if !isVideoFile(entry.Name()) {
			result.Skipped++
			return nil
		}
		result.ScannedFiles++
		imported, updated, err := s.reconcileFile(ctx, root, path, profiles)
		if err != nil {
			return err
		}
		switch {
		case imported:
			result.Imported++
		case updated:
			result.Updated++
		default:
			result.Skipped++
		}
		return nil
	})
	if errors.Is(err, fs.ErrNotExist) {
		return result, nil
	}
	if err != nil {
		return ReconcileResult{}, fmt.Errorf("reconcile local recordings: %w", err)
	}
	return result, nil
}

func (s Store) FileForDownload(ctx context.Context, actor account.User, fileID int64) (File, error) {
	query := `
		SELECT f.id, f.recording_id, f.relative_path, f.original_name, f.kind, f.file_status,
			COALESCE(f.size_bytes, 0), COALESCE(f.duration_ms, 0), COALESCE(f.closed_at, ''), f.updated_at
		FROM recording_files f
		JOIN recordings rec ON rec.id = f.recording_id
		JOIN recording_profiles p ON p.id = rec.recording_profile_id
		WHERE f.id = ? AND f.deleted_at IS NULL
	`
	args := []interface{}{fileID}
	if actor.Role != account.RoleSuperAdmin {
		query += " AND p.owner_user_id = ?"
		args = append(args, actor.ID)
	}

	var item File
	err := s.db.QueryRowContext(ctx, query, args...).Scan(
		&item.ID,
		&item.RecordingID,
		&item.RelativePath,
		&item.OriginalName,
		&item.Kind,
		&item.FileStatus,
		&item.SizeBytes,
		&item.DurationMs,
		&item.ClosedAt,
		&item.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return File{}, ErrNotFound
	}
	if err != nil {
		return File{}, fmt.Errorf("lookup recording file for download: %w", err)
	}
	if item.Kind != "video" || item.FileStatus == "WRITING" {
		return File{}, ErrNotReady
	}
	return item, nil
}

func (s Store) files(ctx context.Context, recordingID int64) ([]File, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, recording_id, relative_path, original_name, kind, file_status,
			COALESCE(size_bytes, 0), COALESCE(duration_ms, 0), COALESCE(closed_at, ''), updated_at
		FROM recording_files
		WHERE recording_id = ?
		ORDER BY id ASC
	`, recordingID)
	if err != nil {
		return nil, fmt.Errorf("list recording files: %w", err)
	}
	defer rows.Close()

	items := make([]File, 0)
	for rows.Next() {
		var item File
		if err := rows.Scan(
			&item.ID,
			&item.RecordingID,
			&item.RelativePath,
			&item.OriginalName,
			&item.Kind,
			&item.FileStatus,
			&item.SizeBytes,
			&item.DurationMs,
			&item.ClosedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan recording file: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate recording files: %w", err)
	}
	return items, nil
}

type profileRef struct {
	ID           int64
	Name         string
	RoomID       string
	StreamerName string
}

func (s Store) activeProfiles(ctx context.Context) (map[string]profileRef, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, room_id, streamer_name
		FROM recording_profiles
		WHERE platform = 'bilibili' AND archived_at IS NULL
	`)
	if err != nil {
		return nil, fmt.Errorf("load active profiles: %w", err)
	}
	defer rows.Close()

	items := map[string]profileRef{}
	for rows.Next() {
		var item profileRef
		if err := rows.Scan(&item.ID, &item.Name, &item.RoomID, &item.StreamerName); err != nil {
			return nil, fmt.Errorf("scan active profile: %w", err)
		}
		items[item.RoomID] = item
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate active profiles: %w", err)
	}
	return items, nil
}

func (s Store) reconcileFile(ctx context.Context, root string, path string, profiles map[string]profileRef) (bool, bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		return false, false, fmt.Errorf("stat recording file: %w", err)
	}
	relative, err := filepath.Rel(s.cfg.DataRoot, path)
	if err != nil {
		return false, false, fmt.Errorf("resolve relative recording path: %w", err)
	}
	relativePath := filepath.ToSlash(relative)
	roomID := roomIDFromPath(root, path)
	if roomID == "" {
		result := recordingNamePattern.FindStringSubmatch(filepath.Base(path))
		if len(result) > 1 {
			roomID = result[1]
		}
	}
	profile, ok := profiles[roomID]
	if !ok {
		return false, false, nil
	}

	fileStatus := "CLOSED"
	recordingStatus := "COMPLETED"
	completedAt := info.ModTime().UTC().Format(time.RFC3339)
	if time.Since(info.ModTime()) < 2*time.Minute {
		fileStatus = "WRITING"
		recordingStatus = "ACTIVE"
		completedAt = ""
	}
	startedAt, title := recordingInfoFromName(filepath.Base(path), info.ModTime())
	durationMs := durationMillis(startedAt, completedAt)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, false, fmt.Errorf("begin recording reconcile: %w", err)
	}
	defer tx.Rollback()

	var fileID int64
	var existingRecordingID int64
	err = tx.QueryRowContext(ctx, `
		SELECT id, recording_id
		FROM recording_files
		WHERE relative_path = ?
	`, relativePath).Scan(&fileID, &existingRecordingID)
	if err == nil {
		if _, err := tx.ExecContext(ctx, `
			UPDATE recording_files
			SET file_status = ?, size_bytes = ?, duration_ms = ?, closed_at = NULLIF(?, ''), updated_at = CURRENT_TIMESTAMP
			WHERE id = ?
		`, fileStatus, info.Size(), durationMs, completedAt, fileID); err != nil {
			return false, false, fmt.Errorf("update recording file: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE recordings
			SET recording_status = ?, completed_at = NULLIF(?, ''), duration_ms = ?, local_storage_status = 'AVAILABLE', updated_at = CURRENT_TIMESTAMP
			WHERE id = ?
		`, recordingStatus, completedAt, durationMs, existingRecordingID); err != nil {
			return false, false, fmt.Errorf("update recording: %w", err)
		}
		if err := tx.Commit(); err != nil {
			return false, false, fmt.Errorf("commit recording update: %w", err)
		}
		return false, true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return false, false, fmt.Errorf("lookup recording file: %w", err)
	}

	result, err := tx.ExecContext(ctx, `
		INSERT INTO recordings
			(recording_profile_id, title, started_at, completed_at, duration_ms, recording_status,
				local_storage_status, source_room_id, streamer_name_snapshot)
		VALUES (?, NULLIF(?, ''), ?, NULLIF(?, ''), ?, ?, 'AVAILABLE', ?, ?)
	`, profile.ID, title, startedAt, completedAt, durationMs, recordingStatus, profile.RoomID, profile.StreamerName)
	if err != nil {
		return false, false, fmt.Errorf("insert recording: %w", err)
	}
	recordingID, err := result.LastInsertId()
	if err != nil {
		return false, false, fmt.Errorf("read recording id: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO recording_files
			(recording_id, relative_path, original_name, kind, file_status, size_bytes, duration_ms, closed_at)
		VALUES (?, ?, ?, 'video', ?, ?, ?, NULLIF(?, ''))
	`, recordingID, relativePath, filepath.Base(path), fileStatus, info.Size(), durationMs, completedAt); err != nil {
		return false, false, fmt.Errorf("insert recording file: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return false, false, fmt.Errorf("commit recording insert: %w", err)
	}
	return true, false, nil
}

var recordingNamePattern = regexp.MustCompile(`^[^-]+-([0-9]+)-([0-9]{8})-([0-9]{6})-[0-9]+-(.*)\.[^.]+$`)

func recordingInfoFromName(name string, modTime time.Time) (string, string) {
	result := recordingNamePattern.FindStringSubmatch(name)
	if len(result) != 5 {
		return modTime.UTC().Format(time.RFC3339), strings.TrimSuffix(name, filepath.Ext(name))
	}
	parsed, err := time.ParseInLocation("20060102-150405", result[2]+"-"+result[3], time.FixedZone("Asia/Shanghai", 8*60*60))
	if err != nil {
		return modTime.UTC().Format(time.RFC3339), strings.TrimSuffix(name, filepath.Ext(name))
	}
	return parsed.UTC().Format(time.RFC3339), strings.TrimSpace(result[4])
}

func durationMillis(startedAt string, completedAt string) interface{} {
	if startedAt == "" || completedAt == "" {
		return nil
	}
	started, err := time.Parse(time.RFC3339, startedAt)
	if err != nil {
		return nil
	}
	completed, err := time.Parse(time.RFC3339, completedAt)
	if err != nil {
		return nil
	}
	duration := completed.Sub(started)
	if duration <= 0 {
		return nil
	}
	return duration.Milliseconds()
}

func roomIDFromPath(root string, path string) string {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return ""
	}
	parts := strings.Split(filepath.ToSlash(relative), "/")
	if len(parts) < 2 {
		return ""
	}
	first := parts[0]
	roomID, _, _ := strings.Cut(first, "-")
	return roomID
}

func isVideoFile(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".flv", ".mp4", ".mkv":
		return true
	default:
		return false
	}
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
