package liveanalytics

import (
	"context"
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
		return result, nil
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, raw_relative_path, raw_status
		FROM live_capture_sessions
		WHERE raw_relative_path IS NOT NULL AND raw_status IN ('WRITING', 'AVAILABLE', 'DELETING', 'MISSING')
			AND status NOT IN ('STARTING', 'CONNECTED', 'RECONNECTING')
		ORDER BY COALESCE(ended_at, started_at) ASC, id ASC`)
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
			_, _ = s.db.ExecContext(ctx, `UPDATE live_capture_sessions SET raw_status = ?,
				raw_deleted_at = CASE WHEN ? = 'DELETED' THEN COALESCE(raw_deleted_at, CURRENT_TIMESTAMP) ELSE raw_deleted_at END,
				updated_at = CURRENT_TIMESTAMP WHERE id = ?`, status, status, item.id)
			continue
		}
		if err != nil {
			return result, fmt.Errorf("stat live analytics raw file: %w", err)
		}
		if !info.Mode().IsRegular() {
			return result, fmt.Errorf("live analytics cleanup target is not a regular file")
		}
		claimed, err := s.db.ExecContext(ctx, `UPDATE live_capture_sessions SET raw_status = 'DELETING', raw_size_bytes = ?,
			updated_at = CURRENT_TIMESTAMP WHERE id = ? AND status NOT IN ('STARTING', 'CONNECTED', 'RECONNECTING')
			AND raw_status IN ('WRITING', 'AVAILABLE', 'DELETING', 'MISSING')`, info.Size(), item.id)
		if err != nil {
			return result, fmt.Errorf("claim live analytics raw cleanup: %w", err)
		}
		rowsAffected, err := claimed.RowsAffected()
		if err != nil || rowsAffected != 1 {
			continue
		}
		if err := os.Remove(absolute); err != nil {
			_, _ = s.db.ExecContext(ctx, `UPDATE live_capture_sessions SET raw_status = 'AVAILABLE', updated_at = CURRENT_TIMESTAMP
				WHERE id = ? AND raw_status = 'DELETING'`, item.id)
			return result, fmt.Errorf("delete live analytics raw file: %w", err)
		}
		if _, err := s.db.ExecContext(ctx, `UPDATE live_capture_sessions SET raw_status = 'DELETED', raw_size_bytes = ?,
			raw_deleted_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, info.Size(), item.id); err != nil {
			return result, fmt.Errorf("mark live analytics raw file deleted: %w", err)
		}
		result.AfterBytes -= info.Size()
		result.DeletedBytes += info.Size()
		result.DeletedFiles++
	}
	return result, nil
}

func (s Store) recoverRawCleanupClaims(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx, `SELECT id, raw_relative_path FROM live_capture_sessions
		WHERE raw_status = 'DELETING' AND raw_relative_path IS NOT NULL`)
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
			_, err = s.db.ExecContext(ctx, `UPDATE live_capture_sessions SET raw_status = 'AVAILABLE', updated_at = CURRENT_TIMESTAMP
				WHERE id = ? AND raw_status = 'DELETING'`, item.id)
		case errors.Is(statErr, os.ErrNotExist):
			_, err = s.db.ExecContext(ctx, `UPDATE live_capture_sessions SET raw_status = 'DELETED',
				raw_deleted_at = COALESCE(raw_deleted_at, CURRENT_TIMESTAMP), updated_at = CURRENT_TIMESTAMP
				WHERE id = ? AND raw_status = 'DELETING'`, item.id)
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
	var relative, status string
	if err := s.db.QueryRowContext(ctx, `SELECT COALESCE(raw_relative_path, ''), raw_status FROM live_capture_sessions WHERE id = ?`, id).Scan(&relative, &status); err != nil {
		return err
	}
	if relative == "" || status == "DELETING" || status == "DELETED" {
		return nil
	}
	absolute, err := s.resolveRawPath(relative)
	if err != nil {
		return err
	}
	info, err := os.Lstat(absolute)
	if errors.Is(err, os.ErrNotExist) {
		_, updateErr := s.db.ExecContext(ctx, `UPDATE live_capture_sessions SET raw_status = 'MISSING', updated_at = CURRENT_TIMESTAMP
			WHERE id = ? AND raw_status != 'DELETED'`, id)
		return updateErr
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("live analytics raw path is not a regular file")
	}
	_, err = s.db.ExecContext(ctx, `UPDATE live_capture_sessions SET raw_status = 'AVAILABLE', raw_size_bytes = ?,
		updated_at = CURRENT_TIMESTAMP WHERE id = ? AND raw_status != 'DELETED'`, info.Size(), id)
	return err
}

func (s Store) refreshRawSize(ctx context.Context, id int64) error {
	var relative string
	if err := s.db.QueryRowContext(ctx, `SELECT COALESCE(raw_relative_path, '') FROM live_capture_sessions WHERE id = ?`, id).Scan(&relative); err != nil {
		return err
	}
	if relative == "" {
		return nil
	}
	absolute, err := s.resolveRawPath(relative)
	if err != nil {
		return err
	}
	info, err := os.Lstat(absolute)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("live analytics raw path is not a regular file")
	}
	_, err = s.db.ExecContext(ctx, `UPDATE live_capture_sessions SET raw_size_bytes = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND raw_status = 'WRITING'`, info.Size(), id)
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
	return resolved, nil
}
