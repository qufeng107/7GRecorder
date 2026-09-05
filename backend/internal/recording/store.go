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
	"time"

	"github.com/7grecorder/7grecorder/backend/internal/account"
	"github.com/7grecorder/7grecorder/backend/internal/config"
)

var ErrForbidden = errors.New("recording forbidden")

type Recording struct {
	ID                 int64  `json:"id"`
	RecordingProfileID int64  `json:"recording_profile_id"`
	ProfileName        string `json:"profile_name"`
	RoomID             string `json:"room_id"`
	StreamerName       string `json:"streamer_name"`
	Title              string `json:"title,omitempty"`
	StartedAt          string `json:"started_at"`
	CompletedAt        string `json:"completed_at,omitempty"`
	RecordingStatus    string `json:"recording_status"`
	LocalStorageStatus string `json:"local_storage_status"`
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
	ClosedAt     string `json:"closed_at,omitempty"`
	UpdatedAt    string `json:"updated_at"`
}

type ReconcileResult struct {
	ScannedFiles int `json:"scanned_files"`
	Imported     int `json:"imported"`
	Updated      int `json:"updated"`
	Skipped      int `json:"skipped"`
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
			rec.recording_status, rec.local_storage_status
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
		if err := rows.Scan(
			&item.ID,
			&item.RecordingProfileID,
			&item.ProfileName,
			&item.RoomID,
			&item.StreamerName,
			&item.Title,
			&item.StartedAt,
			&item.CompletedAt,
			&item.RecordingStatus,
			&item.LocalStorageStatus,
		); err != nil {
			return nil, fmt.Errorf("scan recording: %w", err)
		}
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

func (s Store) files(ctx context.Context, recordingID int64) ([]File, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, recording_id, relative_path, original_name, kind, file_status,
			COALESCE(size_bytes, 0), COALESCE(closed_at, ''), updated_at
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
			SET file_status = ?, size_bytes = ?, closed_at = NULLIF(?, ''), updated_at = CURRENT_TIMESTAMP
			WHERE id = ?
		`, fileStatus, info.Size(), completedAt, fileID); err != nil {
			return false, false, fmt.Errorf("update recording file: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE recordings
			SET recording_status = ?, completed_at = NULLIF(?, ''), local_storage_status = 'AVAILABLE', updated_at = CURRENT_TIMESTAMP
			WHERE id = ?
		`, recordingStatus, completedAt, existingRecordingID); err != nil {
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
			(recording_profile_id, title, started_at, completed_at, recording_status,
				local_storage_status, source_room_id, streamer_name_snapshot)
		VALUES (?, NULLIF(?, ''), ?, NULLIF(?, ''), ?, 'AVAILABLE', ?, ?)
	`, profile.ID, title, startedAt, completedAt, recordingStatus, profile.RoomID, profile.StreamerName)
	if err != nil {
		return false, false, fmt.Errorf("insert recording: %w", err)
	}
	recordingID, err := result.LastInsertId()
	if err != nil {
		return false, false, fmt.Errorf("read recording id: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO recording_files
			(recording_id, relative_path, original_name, kind, file_status, size_bytes, closed_at)
		VALUES (?, ?, ?, 'video', ?, ?, NULLIF(?, ''))
	`, recordingID, relativePath, filepath.Base(path), fileStatus, info.Size(), completedAt); err != nil {
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
