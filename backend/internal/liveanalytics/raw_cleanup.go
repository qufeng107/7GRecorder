package liveanalytics

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/7grecorder/7grecorder/backend/internal/storagepolicy"
)

type RawCleanupResult struct {
	BeforeBytes  int64
	AfterBytes   int64
	DeletedBytes int64
	DeletedFiles int64
}

func (s Store) EnforceRawQuota(ctx context.Context) (RawCleanupResult, error) {
	if err := s.recoverRawCleanupClaims(ctx); err != nil {
		return RawCleanupResult{}, err
	}
	root := filepath.Join(s.cfg.DataRoot, "live-analytics")
	used, _, err := storagepolicy.DirectoryUsage(root)
	if err != nil {
		return RawCleanupResult{}, fmt.Errorf("measure live analytics storage: %w", err)
	}
	result := RawCleanupResult{BeforeBytes: used, AfterBytes: used}
	if used <= storagepolicy.LiveAnalyticsRawBytes {
		return result, s.refreshRawSummary(ctx)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, relative_path, status
		FROM live_capture_raw_files
		WHERE relative_path IS NOT NULL AND status IN ('AVAILABLE', 'DELETING', 'MISSING')
        ORDER BY id ASC`)
	if err != nil {
		return result, fmt.Errorf("list live analytics cleanup candidates: %w", err)
	}
	type candidate struct {
		id     int64
		path   string
		status string
	}
	var candidates []candidate
	for rows.Next() {
		var item candidate
		if err := rows.Scan(&item.id, &item.path, &item.status); err != nil {
			rows.Close()
			return result, err
		}
		candidates = append(candidates, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return result, err
	}
	if err := rows.Close(); err != nil {
		return result, err
	}
	for _, item := range candidates {
		if result.AfterBytes <= storagepolicy.LiveAnalyticsRawBytes {
			break
		}
		absolute, err := s.resolveRawPath(item.path)
		if err != nil {
			return result, err
		}
		info, err := os.Lstat(absolute)
		if errors.Is(err, os.ErrNotExist) {
			status := "MISSING"
			if item.status == "DELETING" {
				status = "DELETED"
			}
			_, _ = s.db.ExecContext(ctx, `UPDATE live_capture_raw_files SET status = ?,
				deleted_at = CASE WHEN ? = 'DELETED' THEN COALESCE(deleted_at, CURRENT_TIMESTAMP) ELSE deleted_at END WHERE id = ?`, status, status, item.id)
			continue
		}
		if err != nil {
			return result, fmt.Errorf("stat live analytics raw file: %w", err)
		}
		if !info.Mode().IsRegular() {
			return result, fmt.Errorf("live analytics cleanup target is not a regular file")
		}
		claimed, err := s.db.ExecContext(ctx, `UPDATE live_capture_raw_files SET status = 'DELETING', size_bytes = ?,
			closed_at = COALESCE(closed_at, CURRENT_TIMESTAMP) WHERE id = ?
            AND status IN ('AVAILABLE', 'DELETING', 'MISSING')`, info.Size(), item.id)
		if err != nil {
			return result, fmt.Errorf("claim live analytics raw cleanup: %w", err)
		}
		rowsAffected, err := claimed.RowsAffected()
		if err != nil || rowsAffected != 1 {
			continue
		}
		root, err := os.OpenRoot(s.cfg.DataRoot)
		if err != nil {
			return result, err
		}
		removeErr := root.Remove(filepath.FromSlash(item.path))
		root.Close()
		if err := removeErr; err != nil {
			_, _ = s.db.ExecContext(ctx, `UPDATE live_capture_raw_files SET status = 'AVAILABLE'
				WHERE id = ? AND status = 'DELETING'`, item.id)
			return result, fmt.Errorf("delete live analytics raw file: %w", err)
		}
		if _, err := s.db.ExecContext(ctx, `UPDATE live_capture_raw_files SET status = 'DELETED', size_bytes = ?,
			deleted_at = CURRENT_TIMESTAMP WHERE id = ?`, info.Size(), item.id); err != nil {
			return result, fmt.Errorf("mark live analytics raw file deleted: %w", err)
		}
		result.AfterBytes -= info.Size()
		result.DeletedBytes += info.Size()
		result.DeletedFiles++
	}
	return result, s.refreshRawSummary(ctx)
}

func (s Store) recoverRawCleanupClaims(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx, `SELECT id, relative_path FROM live_capture_raw_files
		WHERE status = 'DELETING' AND relative_path IS NOT NULL`)
	if err != nil {
		return fmt.Errorf("list interrupted live analytics cleanup claims: %w", err)
	}
	type claim struct {
		id   int64
		path string
	}
	var claims []claim
	for rows.Next() {
		var item claim
		if err := rows.Scan(&item.id, &item.path); err != nil {
			rows.Close()
			return err
		}
		claims = append(claims, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, item := range claims {
		absolute, err := s.resolveRawPath(item.path)
		if err != nil {
			return err
		}
		_, statErr := os.Lstat(absolute)
		switch {
		case statErr == nil:
			_, err = s.db.ExecContext(ctx, `UPDATE live_capture_raw_files SET status = 'AVAILABLE'
				WHERE id = ? AND status = 'DELETING'`, item.id)
		case errors.Is(statErr, os.ErrNotExist):
			_, err = s.db.ExecContext(ctx, `UPDATE live_capture_raw_files SET status = 'DELETED',
				deleted_at = COALESCE(deleted_at, CURRENT_TIMESTAMP)
				WHERE id = ? AND status = 'DELETING'`, item.id)
		default:
			return fmt.Errorf("recover live analytics raw cleanup: %w", statErr)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (s Store) refreshRawMetadata(ctx context.Context, id int64) error {
	if err := s.refreshRawSize(ctx, id); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE live_capture_raw_files SET status = 'AVAILABLE', closed_at = CURRENT_TIMESTAMP
        WHERE session_id = ? AND status = 'WRITING'`, id); err != nil {
		return err
	}
	return s.refreshRawSummary(ctx, id)
}

func (s Store) refreshRawSize(ctx context.Context, id int64) error {
	var relative string
	var fileID int64
	err := s.db.QueryRowContext(ctx, `SELECT id,relative_path FROM live_capture_raw_files WHERE session_id = ? AND status = 'WRITING'`, id).Scan(&fileID, &relative)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	absolute, err := s.resolveRawPath(relative)
	if err != nil {
		return err
	}
	info, err := os.Lstat(absolute)
	if errors.Is(err, os.ErrNotExist) {
		_, err = s.db.ExecContext(ctx, `UPDATE live_capture_raw_files SET status = 'MISSING', closed_at = CURRENT_TIMESTAMP WHERE id = ? AND status = 'WRITING'`, fileID)
		return err
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("live analytics raw path is not a regular file")
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE live_capture_raw_files SET size_bytes = ? WHERE id = ? AND status = 'WRITING'`, info.Size(), fileID); err != nil {
		return err
	}
	return s.refreshRawSummary(ctx, id)
}

func (s Store) refreshRawSummary(ctx context.Context, sessionIDs ...int64) error {
	query := `UPDATE live_capture_sessions AS s SET
        raw_size_bytes = (SELECT COALESCE(SUM(size_bytes),0) FROM live_capture_raw_files WHERE session_id = s.id),
        raw_status = CASE
            WHEN EXISTS(SELECT 1 FROM live_capture_raw_files WHERE session_id = s.id AND status = 'WRITING') THEN 'WRITING'
            WHEN EXISTS(SELECT 1 FROM live_capture_raw_files WHERE session_id = s.id AND status = 'AVAILABLE') THEN 'AVAILABLE'
            WHEN EXISTS(SELECT 1 FROM live_capture_raw_files WHERE session_id = s.id AND status = 'DELETING') THEN 'DELETING'
            WHEN EXISTS(SELECT 1 FROM live_capture_raw_files WHERE session_id = s.id AND status = 'MISSING') THEN 'MISSING'
            ELSE 'DELETED' END,
        raw_deleted_at = (SELECT MAX(deleted_at) FROM live_capture_raw_files WHERE session_id = s.id)
        WHERE EXISTS(SELECT 1 FROM live_capture_raw_files WHERE session_id = s.id)`
	var args []any
	if len(sessionIDs) > 0 {
		query += " AND s.id = ?"
		args = append(args, sessionIDs[0])
	}
	_, err := s.db.ExecContext(ctx, query, args...)
	return err
}

func (s Store) resolveRawPath(relative string) (string, error) {
	if filepath.IsAbs(relative) {
		return "", fmt.Errorf("live analytics raw path must be relative")
	}
	root, err := filepath.Abs(filepath.Join(s.cfg.DataRoot, "live-analytics"))
	if err != nil {
		return "", err
	}
	resolved, err := filepath.Abs(filepath.Join(s.cfg.DataRoot, filepath.FromSlash(relative)))
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(root, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("live analytics raw path escapes data root")
	}
	if err := rejectRawSymlinks(s.cfg.DataRoot, relative); err != nil {
		return "", err
	}
	return resolved, nil
}

// Refuse symlinks in every component, including the evidence root itself.
func rejectRawSymlinks(dataRoot, relative string) error {
	current := dataRoot
	for _, part := range strings.Split(filepath.Clean(relative), string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("live analytics path contains a symlink")
		}
	}
	return nil
}
