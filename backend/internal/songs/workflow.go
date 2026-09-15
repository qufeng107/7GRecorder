package songs

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/7grecorder/7grecorder/backend/internal/account"
	"github.com/7grecorder/7grecorder/backend/internal/secretbox"
	"github.com/7grecorder/7grecorder/backend/internal/upload"
)

type ProcessJobPayload struct {
	AnalysisRunID int64 `json:"analysis_run_id"`
}

type ProcessRequest struct {
	AnalysisRunID   int64
	SourcePath      string
	AnalysisPath    string
	ProviderRegion  string
	ContainerID     string
	AccessToken     string
	ProviderName    string
	TimelineStartMs int64
	TimelineEndMs   int64
	PaddingMs       int64
}

type AudioArtifactRequest struct {
	SongID              int64
	RecordingProfileID  int64
	Revision            int
	SourcePath          string
	DestinationPath     string
	RelativePath        string
	StartOffsetMs       int64
	EndOffsetMs         int64
	COSStorageProfileID int64
	COSRequest          upload.COSUploadRequest
}

type Song struct {
	ID                  int64   `json:"id"`
	AnalysisRunID       int64   `json:"analysis_run_id"`
	Title               string  `json:"title"`
	Artist              string  `json:"artist"`
	StartMs             int64   `json:"start_ms"`
	EndMs               int64   `json:"end_ms"`
	Confidence          float64 `json:"confidence,omitempty"`
	Status              string  `json:"status"`
	AudioArtifactStatus string  `json:"audio_artifact_status"`
	AudioURL            string  `json:"audio_url,omitempty"`
	CreatedAt           string  `json:"created_at"`
}

type AudioFile struct {
	AbsolutePath string
	Filename     string
}

func (s Store) ProcessRequest(ctx context.Context, runID int64) (ProcessRequest, error) {
	var req ProcessRequest
	var sourceRelative, algorithm string
	var encrypted []byte
	err := s.db.QueryRowContext(ctx, `SELECT r.id, r.source_relative_path, r.provider_region,
		r.provider_container_id, c.encrypted_secret, r.source_timeline_start_ms, r.source_timeline_end_ms,
		r.boundary_padding_ms, r.algorithm_version
		FROM song_analysis_runs r JOIN credentials c ON c.id = r.provider_credential_id
		WHERE r.id = ? AND r.status IN ('ANALYZING', 'RECOGNIZING', 'GENERATING_AUDIO', 'FAILED')`, runID).Scan(
		&req.AnalysisRunID, &sourceRelative, &req.ProviderRegion, &req.ContainerID, &encrypted,
		&req.TimelineStartMs, &req.TimelineEndMs, &req.PaddingMs, &algorithm)
	if errors.Is(err, sql.ErrNoRows) {
		return ProcessRequest{}, ErrNotReady
	}
	if err != nil {
		return ProcessRequest{}, err
	}
	req.SourcePath, err = resolveWithinRoot(s.cfg.DataRoot, sourceRelative)
	if err != nil {
		return ProcessRequest{}, err
	}
	if info, statErr := os.Stat(req.SourcePath); statErr != nil || info.IsDir() {
		return ProcessRequest{}, ErrNotReady
	}
	req.AnalysisPath, err = resolveWithinRoot(s.cfg.DataRoot, filepath.ToSlash(filepath.Join("songs", "work", "runs", fmt.Sprint(runID), "analysis.mp3")))
	if err != nil {
		return ProcessRequest{}, err
	}
	req.ProviderName = fmt.Sprintf("7grecorder-run-%d-%s.mp3", runID, safeFilenamePart(algorithm))
	plaintext, err := secretbox.Decrypt(s.cfg.MasterKeyPath, encrypted)
	if err != nil {
		return ProcessRequest{}, fmt.Errorf("decrypt ACRCloud credential: %w", err)
	}
	var secret map[string]string
	if err := json.Unmarshal(plaintext, &secret); err != nil {
		return ProcessRequest{}, fmt.Errorf("decode ACRCloud credential: %w", err)
	}
	req.AccessToken = first(secret, "access_token", "accessToken", "token")
	if req.AccessToken == "" {
		return ProcessRequest{}, ErrValidation
	}
	return req, nil
}

func (s Store) MarkRecognizing(ctx context.Context, runID int64) error {
	result, err := s.db.ExecContext(ctx, `UPDATE song_analysis_runs SET status = 'RECOGNIZING',
		progress_message = 'Recognizing analysis audio with ACRCloud', last_error_class = NULL, last_error = NULL,
		updated_at = CURRENT_TIMESTAMP WHERE id = ? AND status IN ('ANALYZING', 'RECOGNIZING', 'GENERATING_AUDIO', 'FAILED')`, runID)
	if err != nil {
		return err
	}
	changed, _ := result.RowsAffected()
	if changed != 1 {
		return ErrNotReady
	}
	return nil
}

func (s Store) MarkProviderSubmitted(ctx context.Context, runID int64, providerFilename, providerFileID string, pollCount int) error {
	if runID <= 0 || strings.TrimSpace(providerFilename) == "" || strings.TrimSpace(providerFileID) == "" {
		return ErrValidation
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO song_analysis_chunks
		(analysis_run_id, chunk_index, core_start_ms, core_end_ms, guard_start_ms, guard_end_ms,
		provider_filename, provider_file_id, status, poll_count)
		SELECT id, 0, 0, source_timeline_end_ms-source_timeline_start_ms, 0,
			source_timeline_end_ms-source_timeline_start_ms, ?, ?, 'PROCESSING', 0
		FROM song_analysis_runs WHERE id = ?
		ON CONFLICT(analysis_run_id, chunk_index) DO UPDATE SET provider_filename = excluded.provider_filename,
			provider_file_id = excluded.provider_file_id, status = 'PROCESSING', updated_at = CURRENT_TIMESTAMP`,
		providerFilename, providerFileID, runID)
	if err == nil && pollCount > 0 {
		_, err = s.db.ExecContext(ctx, `UPDATE song_analysis_chunks SET poll_count = MAX(poll_count, ?),
			updated_at = CURRENT_TIMESTAMP WHERE analysis_run_id = ? AND chunk_index = 0`, pollCount, runID)
	}
	return err
}

func (s Store) PersistRecognition(ctx context.Context, runID int64, result RecognitionResult) ([]int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var profileID, uploadSourceID, timelineStart, timelineEnd, padding int64
	var providerName string
	if err := tx.QueryRowContext(ctx, `SELECT recording_profile_id, upload_source_id, source_timeline_start_ms,
		source_timeline_end_ms, boundary_padding_ms, 'analysis.mp3' FROM song_analysis_runs
		WHERE id = ? AND status = 'RECOGNIZING'`, runID).Scan(
		&profileID, &uploadSourceID, &timelineStart, &timelineEnd, &padding, &providerName); err != nil {
		return nil, err
	}
	var existing int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM songs WHERE analysis_run_id = ?`, runID).Scan(&existing); err != nil {
		return nil, err
	}
	if existing == 0 {
		_, err := tx.ExecContext(ctx, `INSERT INTO song_analysis_chunks
			(analysis_run_id, chunk_index, core_start_ms, core_end_ms, guard_start_ms, guard_end_ms,
			provider_filename, provider_file_id, status, result_json, poll_count)
			VALUES (?, 0, 0, ?, 0, ?, ?, ?, 'READY', ?, 1)
			ON CONFLICT(analysis_run_id, chunk_index) DO UPDATE SET provider_file_id = excluded.provider_file_id,
				status = 'READY', result_json = excluded.result_json, poll_count = song_analysis_chunks.poll_count + 1,
				updated_at = CURRENT_TIMESTAMP`, runID, timelineEnd-timelineStart,
			timelineEnd-timelineStart, providerName, result.ProviderFileID, result.RawJSON)
		if err != nil {
			return nil, err
		}
		var chunkID int64
		if err := tx.QueryRowContext(ctx, `SELECT id FROM song_analysis_chunks WHERE analysis_run_id = ? AND chunk_index = 0`, runID).Scan(&chunkID); err != nil {
			return nil, err
		}
		for _, match := range result.Matches {
			startOffset := max64(0, match.StartMs-padding)
			endOffset := min64(timelineEnd-timelineStart, match.EndMs+padding)
			if endOffset <= startOffset {
				continue
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO song_recognition_matches
				(analysis_run_id, analysis_chunk_id, engine, acrid, isrc, title, artist, chunk_start_ms,
				chunk_end_ms, global_start_ms, global_end_ms, score, evidence_json)
				VALUES (?, ?, ?, NULLIF(?, ''), NULLIF(?, ''), ?, ?, ?, ?, ?, ?, ?, ?)`, runID, chunkID,
				match.Engine, match.ACRID, match.ISRC, match.Title, match.Artist, match.StartMs, match.EndMs,
				timelineStart+match.StartMs, timelineStart+match.EndMs, match.Score, match.EvidenceJSON); err != nil {
				return nil, err
			}
			inserted, err := tx.ExecContext(ctx, `INSERT INTO songs
				(recording_profile_id, upload_source_id, analysis_run_id, title, artist, detected_start_ms,
				detected_end_ms, start_ms, end_ms, confidence, status, audio_artifact_status)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'DRAFT', 'PENDING')`, profileID, uploadSourceID, runID,
				match.Title, match.Artist, timelineStart+match.StartMs, timelineStart+match.EndMs,
				timelineStart+startOffset, timelineStart+endOffset, match.Score)
			if err != nil {
				return nil, err
			}
			songID, _ := inserted.LastInsertId()
			if _, err := tx.ExecContext(ctx, `INSERT INTO song_candidates (song_id, title, artist, source, score, evidence_json)
				VALUES (?, ?, ?, ?, ?, ?)`, songID, match.Title, match.Artist, match.Engine, match.Score, match.EvidenceJSON); err != nil {
				return nil, err
			}
		}
	}
	rows, err := tx.QueryContext(ctx, `SELECT id FROM songs WHERE analysis_run_id = ? ORDER BY start_ms, id`, runID)
	if err != nil {
		return nil, err
	}
	ids := make([]int64, 0)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	status, message := "GENERATING_AUDIO", "Generating and uploading recognized audio clips"
	if len(ids) == 0 {
		status, message = "COMPLETED", "Recognition completed with no songs found"
	}
	if _, err := tx.ExecContext(ctx, `UPDATE song_analysis_runs SET status = ?, progress_message = ?,
		updated_at = CURRENT_TIMESTAMP WHERE id = ?`, status, message, runID); err != nil {
		return nil, err
	}
	return ids, tx.Commit()
}

func (s Store) AudioArtifactRequest(ctx context.Context, songID int64) (AudioArtifactRequest, error) {
	var req AudioArtifactRequest
	var sourceRelative, prefix, region, bucket string
	var timelineStart int64
	var encrypted []byte
	err := s.db.QueryRowContext(ctx, `SELECT song.id, song.recording_profile_id, song.clip_revision, run.source_relative_path,
		run.source_timeline_start_ms, song.start_ms, song.end_ms, run.destination_cos_storage_profile_id,
		run.songs_prefix, csp.region, csp.bucket, c.encrypted_secret
		FROM songs song JOIN song_analysis_runs run ON run.id = song.analysis_run_id
		JOIN cos_storage_profiles csp ON csp.id = run.destination_cos_storage_profile_id
		JOIN credentials c ON c.id = csp.credential_id
		WHERE song.id = ? AND run.status IN ('GENERATING_AUDIO', 'FAILED')
		AND song.audio_artifact_status IN ('PENDING', 'PROCESSING', 'FAILED')`, songID).Scan(
		&req.SongID, &req.RecordingProfileID, &req.Revision, &sourceRelative, &timelineStart, &req.StartOffsetMs, &req.EndOffsetMs,
		&req.COSStorageProfileID, &prefix, &region, &bucket, &encrypted)
	if errors.Is(err, sql.ErrNoRows) {
		return AudioArtifactRequest{}, ErrNotReady
	}
	if err != nil {
		return AudioArtifactRequest{}, err
	}
	req.StartOffsetMs -= timelineStart
	req.EndOffsetMs -= timelineStart
	req.SourcePath, err = resolveWithinRoot(s.cfg.DataRoot, sourceRelative)
	if err != nil {
		return AudioArtifactRequest{}, err
	}
	req.RelativePath = filepath.ToSlash(filepath.Join("songs", "cache", "audio", fmt.Sprint(songID), fmt.Sprintf("audio-r%d.m4a", req.Revision)))
	req.DestinationPath, err = resolveWithinRoot(s.cfg.DataRoot, req.RelativePath)
	if err != nil {
		return AudioArtifactRequest{}, err
	}
	plaintext, err := secretbox.Decrypt(s.cfg.MasterKeyPath, encrypted)
	if err != nil {
		return AudioArtifactRequest{}, err
	}
	var secret map[string]string
	if err := json.Unmarshal(plaintext, &secret); err != nil {
		return AudioArtifactRequest{}, err
	}
	objectKey := strings.Trim(prefix, "/") + fmt.Sprintf("/%d/%d/audio-r%d.m4a", req.RecordingProfileID, songID, req.Revision)
	req.COSRequest = upload.COSUploadRequest{
		ObjectID: songID, Region: region, Bucket: bucket, ObjectKey: objectKey, SourcePath: req.DestinationPath,
		SourceRelativePath: req.RelativePath, Secret: upload.COSSecret{SecretID: first(secret, "secret_id", "secretId", "SecretId", "SecretID"),
			SecretKey: first(secret, "secret_key", "secretKey", "SecretKey"), SessionToken: first(secret, "session_token", "sessionToken", "SessionToken")},
	}
	if req.COSRequest.Secret.SecretID == "" || req.COSRequest.Secret.SecretKey == "" {
		return AudioArtifactRequest{}, ErrValidation
	}
	return req, nil
}

func (s Store) MarkAudioGenerating(ctx context.Context, req AudioArtifactRequest) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE songs SET audio_artifact_status = 'PROCESSING', updated_at = CURRENT_TIMESTAMP WHERE id = ?`, req.SongID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO song_artifacts
		(song_id, clip_revision, kind, cos_storage_profile_id, object_key, status)
		VALUES (?, ?, 'AUDIO_M4A', ?, ?, 'GENERATING')
		ON CONFLICT(song_id, clip_revision, kind) DO UPDATE SET status = 'GENERATING', last_error = NULL,
		updated_at = CURRENT_TIMESTAMP`, req.SongID, req.Revision, req.COSStorageProfileID, req.COSRequest.ObjectKey); err != nil {
		return err
	}
	return tx.Commit()
}

func (s Store) EnsureAudioCacheCapacity(ctx context.Context, requiredBytes int64) error {
	if requiredBytes <= 0 {
		return ErrValidation
	}
	var maxManaged, usedAudio, usedManaged int64
	if err := s.db.QueryRowContext(ctx, `SELECT settings.max_recording_bytes,
		COALESCE((SELECT SUM(size_bytes) FROM media_cache_entries WHERE kind = 'SONG_AUDIO' AND status IN ('AVAILABLE', 'GENERATING')), 0),
		COALESCE((SELECT SUM(size_bytes) FROM recording_files WHERE kind = 'video' AND file_status != 'DELETED' AND deleted_at IS NULL), 0)
		+ COALESCE((SELECT SUM(size_bytes) FROM media_cache_entries WHERE status IN ('AVAILABLE', 'GENERATING', 'DOWNLOADING')), 0)
		+ COALESCE((SELECT SUM(reserved_bytes) FROM storage_reservations WHERE expires_at > CURRENT_TIMESTAMP), 0)
		FROM local_storage_settings settings WHERE settings.id = 1`).Scan(&maxManaged, &usedAudio, &usedManaged); err != nil {
		return err
	}
	limit := maxManaged * 5 / 100
	if requiredBytes > limit {
		return ErrWaitingForSpace
	}
	for usedAudio+requiredBytes > limit || usedManaged+requiredBytes > maxManaged {
		var id int64
		var relative string
		var size int64
		err := s.db.QueryRowContext(ctx, `SELECT id, relative_path, size_bytes FROM media_cache_entries
			WHERE kind = 'SONG_AUDIO' AND status = 'AVAILABLE' AND lease_job_id IS NULL
			AND (grace_until IS NULL OR grace_until < CURRENT_TIMESTAMP)
			ORDER BY last_accessed_at ASC, id ASC LIMIT 1`).Scan(&id, &relative, &size)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrWaitingForSpace
		}
		if err != nil {
			return err
		}
		claimed, err := s.db.ExecContext(ctx, `UPDATE media_cache_entries SET status = 'EVICTING', updated_at = CURRENT_TIMESTAMP
			WHERE id = ? AND status = 'AVAILABLE' AND lease_job_id IS NULL
			AND (grace_until IS NULL OR grace_until < CURRENT_TIMESTAMP)`, id)
		if err != nil {
			return err
		}
		changed, _ := claimed.RowsAffected()
		if changed != 1 {
			continue
		}
		path, err := resolveWithinRoot(s.cfg.DataRoot, relative)
		if err == nil {
			err = os.Remove(path)
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			_, _ = s.db.ExecContext(ctx, `UPDATE media_cache_entries SET status = 'AVAILABLE', last_error = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, truncate(err.Error(), 500), id)
			return err
		}
		if _, err := s.db.ExecContext(ctx, `DELETE FROM media_cache_entries WHERE id = ? AND status = 'EVICTING'`, id); err != nil {
			return err
		}
		usedAudio -= size
		usedManaged -= size
	}
	return nil
}

func (s Store) MarkAudioAvailable(ctx context.Context, req AudioArtifactRequest, result upload.COSUploadResult) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE song_artifacts SET size_bytes = ?, etag = NULLIF(?, ''), status = 'AVAILABLE',
		uploaded_at = CURRENT_TIMESTAMP, last_error = NULL, updated_at = CURRENT_TIMESTAMP
		WHERE song_id = ? AND clip_revision = ? AND kind = 'AUDIO_M4A'`, result.SizeBytes, result.ETag, req.SongID, req.Revision); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE songs SET audio_artifact_status = 'AVAILABLE', updated_at = CURRENT_TIMESTAMP WHERE id = ?`, req.SongID); err != nil {
		return err
	}
	cacheKey := fmt.Sprintf("song:%d:audio:%d", req.SongID, req.Revision)
	if _, err := tx.ExecContext(ctx, `INSERT INTO media_cache_entries
		(kind, cache_key, song_id, clip_revision, relative_path, size_bytes, status, grace_until)
		VALUES ('SONG_AUDIO', ?, ?, ?, ?, ?, 'AVAILABLE', datetime('now', '+10 minutes'))
		ON CONFLICT(cache_key) DO UPDATE SET relative_path = excluded.relative_path, size_bytes = excluded.size_bytes,
		status = 'AVAILABLE', last_error = NULL, last_accessed_at = CURRENT_TIMESTAMP,
		grace_until = datetime('now', '+10 minutes'), updated_at = CURRENT_TIMESTAMP`, cacheKey, req.SongID, req.Revision,
		req.RelativePath, result.SizeBytes); err != nil {
		return err
	}
	return tx.Commit()
}

func (s Store) MarkProcessFailed(ctx context.Context, runID int64, class, message string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE song_analysis_runs SET status = 'FAILED', progress_message = NULL,
		last_error_class = ?, last_error = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND status != 'CANCELLED'`, class, truncate(message, 1000), runID)
	return err
}

func (s Store) MarkRunCompleted(ctx context.Context, runID int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE song_analysis_runs SET
		status = CASE WHEN EXISTS (SELECT 1 FROM songs WHERE analysis_run_id = ?) THEN 'REVIEW_REQUIRED' ELSE 'COMPLETED' END,
		progress_message = CASE WHEN EXISTS (SELECT 1 FROM songs WHERE analysis_run_id = ?) THEN 'Recognition and audio generation completed; review required' ELSE 'Recognition completed with no songs found' END,
		last_error_class = NULL, last_error = NULL,
		updated_at = CURRENT_TIMESTAMP WHERE id = ? AND status = 'GENERATING_AUDIO'`, runID, runID, runID)
	return err
}

func (s Store) CleanupRunWork(ctx context.Context, runID int64, analysisPath string) error {
	var relative string
	if err := s.db.QueryRowContext(ctx, `SELECT source_relative_path FROM song_analysis_runs WHERE id = ?`, runID).Scan(&relative); err != nil {
		return err
	}
	source, err := resolveWithinRoot(s.cfg.DataRoot, relative)
	if err != nil {
		return err
	}
	for _, path := range []string{analysisPath, source} {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	_, err = s.db.ExecContext(ctx, `DELETE FROM media_cache_entries WHERE cache_key = ?`, fmt.Sprintf("song-analysis:%d:source", runID))
	return err
}

func (s Store) ListSongs(ctx context.Context, actor account.User) ([]Song, error) {
	if actor.Role != account.RoleSuperAdmin {
		return nil, ErrForbidden
	}
	rows, err := s.db.QueryContext(ctx, `SELECT song.id, song.analysis_run_id, COALESCE(song.title, ''), COALESCE(song.artist, ''),
		song.start_ms, song.end_ms, COALESCE(song.confidence, 0), song.status, song.audio_artifact_status, song.created_at,
		EXISTS (SELECT 1 FROM media_cache_entries cache WHERE cache.song_id = song.id
			AND cache.clip_revision = song.clip_revision AND cache.kind = 'SONG_AUDIO' AND cache.status = 'AVAILABLE')
		FROM songs song ORDER BY song.created_at DESC, song.start_ms, song.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Song, 0)
	for rows.Next() {
		var item Song
		var cached int
		if err := rows.Scan(&item.ID, &item.AnalysisRunID, &item.Title, &item.Artist, &item.StartMs, &item.EndMs,
			&item.Confidence, &item.Status, &item.AudioArtifactStatus, &item.CreatedAt, &cached); err != nil {
			return nil, err
		}
		if item.AudioArtifactStatus == "AVAILABLE" && cached == 1 {
			item.AudioURL = fmt.Sprintf("/api/v1/songs/%d/audio", item.ID)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s Store) AudioForPlayback(ctx context.Context, actor account.User, songID int64) (AudioFile, error) {
	if actor.Role != account.RoleSuperAdmin {
		return AudioFile{}, ErrForbidden
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return AudioFile{}, err
	}
	defer tx.Rollback()
	claimed, err := tx.ExecContext(ctx, `UPDATE media_cache_entries SET last_accessed_at = CURRENT_TIMESTAMP,
		grace_until = datetime('now', '+10 minutes'), updated_at = CURRENT_TIMESTAMP
		WHERE song_id = ? AND kind = 'SONG_AUDIO' AND status = 'AVAILABLE'
		AND clip_revision = (SELECT clip_revision FROM songs WHERE id = ?)`, songID, songID)
	if err != nil {
		return AudioFile{}, err
	}
	changed, _ := claimed.RowsAffected()
	if changed != 1 {
		return AudioFile{}, ErrNotReady
	}
	var relative string
	err = tx.QueryRowContext(ctx, `SELECT cache.relative_path FROM songs song
		JOIN media_cache_entries cache ON cache.song_id = song.id AND cache.clip_revision = song.clip_revision
		WHERE song.id = ? AND song.audio_artifact_status = 'AVAILABLE' AND cache.kind = 'SONG_AUDIO'
		AND cache.status = 'AVAILABLE'`, songID).Scan(&relative)
	if errors.Is(err, sql.ErrNoRows) {
		return AudioFile{}, ErrNotReady
	}
	if err != nil {
		return AudioFile{}, err
	}
	if err := tx.Commit(); err != nil {
		return AudioFile{}, err
	}
	path, err := resolveWithinRoot(s.cfg.DataRoot, relative)
	if err != nil {
		return AudioFile{}, err
	}
	if info, err := os.Stat(path); err != nil || info.IsDir() {
		return AudioFile{}, ErrNotReady
	}
	return AudioFile{AbsolutePath: path, Filename: filepath.Base(path)}, nil
}

func safeFilenamePart(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "v1"
	}
	var builder strings.Builder
	for _, char := range value {
		if char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || char == '-' || char == '_' {
			builder.WriteRune(char)
		}
	}
	if builder.Len() == 0 {
		return "v1"
	}
	return builder.String()
}

func min64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
