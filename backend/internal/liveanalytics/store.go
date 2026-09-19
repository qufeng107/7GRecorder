package liveanalytics

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/7grecorder/7grecorder/backend/internal/account"
	"github.com/7grecorder/7grecorder/backend/internal/config"
	"github.com/7grecorder/7grecorder/backend/internal/secretbox"
)

var (
	ErrForbidden  = errors.New("live analytics forbidden")
	ErrNotFound   = errors.New("live analytics resource not found")
	ErrValidation = errors.New("live analytics validation failed")
)

type Store struct {
	db  *sql.DB
	cfg config.Config
}

func NewStore(database *sql.DB, cfg config.Config) Store { return Store{db: database, cfg: cfg} }

type Config struct {
	RecordingProfileID int64  `json:"recording_profile_id"`
	CredentialID       int64  `json:"credential_id,omitempty"`
	AppID              int64  `json:"app_id"`
	Enabled            bool   `json:"enabled"`
	LastStartedAt      string `json:"last_started_at,omitempty"`
	LastError          string `json:"last_error,omitempty"`
	UpdatedAt          string `json:"updated_at,omitempty"`
}

type ConfigUpsert struct {
	CredentialID int64 `json:"credential_id"`
	AppID        int64 `json:"app_id"`
	Enabled      bool  `json:"enabled"`
}

type Session struct {
	ID                 int64          `json:"id"`
	RecordingProfileID int64          `json:"recording_profile_id"`
	Source             string         `json:"source"`
	Status             string         `json:"status"`
	RoomID             string         `json:"room_id,omitempty"`
	AnchorUID          int64          `json:"anchor_uid,omitempty"`
	AnchorOpenID       string         `json:"anchor_open_id,omitempty"`
	AnchorUnionID      string         `json:"anchor_union_id,omitempty"`
	AnchorName         string         `json:"anchor_name,omitempty"`
	AnchorFaceURL      string         `json:"anchor_face_url,omitempty"`
	RawStatus          string         `json:"raw_status"`
	RawSizeBytes       int64          `json:"raw_size_bytes"`
	RawDeletedAt       string         `json:"raw_deleted_at,omitempty"`
	StartedAt          string         `json:"started_at"`
	ConnectedAt        string         `json:"connected_at,omitempty"`
	EndedAt            string         `json:"ended_at,omitempty"`
	LastEventAt        string         `json:"last_event_at,omitempty"`
	LastHeartbeatAt    string         `json:"last_heartbeat_at,omitempty"`
	EventCount         int64          `json:"event_count"`
	UnknownEventCount  int64          `json:"unknown_event_count"`
	GapCount           int64          `json:"gap_count"`
	EventCounts        map[string]int `json:"event_counts"`
	LastError          string         `json:"last_error,omitempty"`
}

type CaptureRequest struct {
	RecordingProfileID int64
	ExpectedRoomID     string
	CredentialID       int64
	AppID              int64
	AccessKeyID        string
	AccessKeySecret    string
	IdentityCode       string
}

func (s Store) GetConfig(ctx context.Context, actor account.User, profileID int64) (Config, error) {
	if err := s.ensureProfileVisible(ctx, actor, profileID); err != nil {
		return Config{}, err
	}
	var item Config
	var enabled int
	err := s.db.QueryRowContext(ctx, `SELECT recording_profile_id, COALESCE(credential_id, 0), app_id, enabled,
		COALESCE(last_started_at, ''), COALESCE(last_error, ''), updated_at
		FROM live_analytics_configs WHERE recording_profile_id = ?`, profileID).
		Scan(&item.RecordingProfileID, &item.CredentialID, &item.AppID, &enabled, &item.LastStartedAt, &item.LastError, &item.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Config{RecordingProfileID: profileID}, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("get live analytics config: %w", err)
	}
	item.Enabled = enabled == 1
	return item, nil
}

func (s Store) UpsertConfig(ctx context.Context, actor account.User, profileID int64, req ConfigUpsert) (Config, error) {
	if actor.Role != account.RoleSuperAdmin {
		return Config{}, ErrForbidden
	}
	if err := s.ensureProfileVisible(ctx, actor, profileID); err != nil {
		return Config{}, err
	}
	if req.AppID < 0 || req.CredentialID < 0 || (req.Enabled && (req.AppID == 0 || req.CredentialID == 0)) {
		return Config{}, ErrValidation
	}
	if req.CredentialID > 0 {
		var count int
		if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM credentials
			WHERE id = ? AND platform = 'bilibili_open_live' AND purpose = 'LIVE_ANALYTICS'`, req.CredentialID).Scan(&count); err != nil {
			return Config{}, fmt.Errorf("validate OpenLive credential: %w", err)
		}
		if count != 1 {
			return Config{}, ErrValidation
		}
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO live_analytics_configs
		(recording_profile_id, credential_id, app_id, enabled)
		VALUES (?, NULLIF(?, 0), ?, ?)
		ON CONFLICT(recording_profile_id) DO UPDATE SET
			credential_id = NULLIF(excluded.credential_id, 0), app_id = excluded.app_id,
			enabled = excluded.enabled, last_error = NULL, updated_at = CURRENT_TIMESTAMP`,
		profileID, req.CredentialID, req.AppID, boolInt(req.Enabled))
	if err != nil {
		return Config{}, fmt.Errorf("save live analytics config: %w", err)
	}
	return s.GetConfig(ctx, actor, profileID)
}

func (s Store) ListSessions(ctx context.Context, actor account.User, profileID int64) ([]Session, error) {
	if err := s.ensureProfileVisible(ctx, actor, profileID); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, recording_profile_id, source, status, COALESCE(room_id, ''),
		COALESCE(anchor_uid, 0), COALESCE(anchor_open_id, ''), COALESCE(anchor_union_id, ''), COALESCE(anchor_name, ''), COALESCE(anchor_face_url, ''),
		raw_status, raw_size_bytes, COALESCE(raw_deleted_at, ''), started_at, COALESCE(connected_at, ''),
		COALESCE(ended_at, ''), COALESCE(last_event_at, ''), COALESCE(last_heartbeat_at, ''), event_count,
		unknown_event_count, gap_count, event_counts_json, COALESCE(last_error, '')
		FROM live_capture_sessions WHERE recording_profile_id = ? ORDER BY started_at DESC, id DESC LIMIT 100`, profileID)
	if err != nil {
		return nil, fmt.Errorf("list live capture sessions: %w", err)
	}
	defer rows.Close()
	items := []Session{}
	for rows.Next() {
		item, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s Store) GetSession(ctx context.Context, actor account.User, id int64) (Session, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, recording_profile_id, source, status, COALESCE(room_id, ''),
		COALESCE(anchor_uid, 0), COALESCE(anchor_open_id, ''), COALESCE(anchor_union_id, ''), COALESCE(anchor_name, ''), COALESCE(anchor_face_url, ''),
		raw_status, raw_size_bytes, COALESCE(raw_deleted_at, ''), started_at, COALESCE(connected_at, ''),
		COALESCE(ended_at, ''), COALESCE(last_event_at, ''), COALESCE(last_heartbeat_at, ''), event_count,
		unknown_event_count, gap_count, event_counts_json, COALESCE(last_error, '')
		FROM live_capture_sessions WHERE id = ?`, id)
	item, err := scanSession(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Session{}, ErrNotFound
	}
	if err != nil {
		return Session{}, err
	}
	if err := s.ensureProfileVisible(ctx, actor, item.RecordingProfileID); err != nil {
		return Session{}, err
	}
	return item, nil
}

type scanner interface{ Scan(...any) error }

func scanSession(row scanner) (Session, error) {
	var item Session
	var counts string
	if err := row.Scan(&item.ID, &item.RecordingProfileID, &item.Source, &item.Status, &item.RoomID,
		&item.AnchorUID, &item.AnchorOpenID, &item.AnchorUnionID, &item.AnchorName, &item.AnchorFaceURL,
		&item.RawStatus, &item.RawSizeBytes, &item.RawDeletedAt, &item.StartedAt, &item.ConnectedAt, &item.EndedAt,
		&item.LastEventAt, &item.LastHeartbeatAt, &item.EventCount, &item.UnknownEventCount,
		&item.GapCount, &counts, &item.LastError); err != nil {
		return Session{}, err
	}
	item.EventCounts = map[string]int{}
	if err := json.Unmarshal([]byte(counts), &item.EventCounts); err != nil {
		return Session{}, fmt.Errorf("decode live event counts: %w", err)
	}
	return item, nil
}

func (s Store) EnabledRequests(ctx context.Context) ([]CaptureRequest, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT c.recording_profile_id, p.room_id, c.credential_id, c.app_id,
		cr.encrypted_secret FROM live_analytics_configs c
		JOIN recording_profiles p ON p.id = c.recording_profile_id
		JOIN credentials cr ON cr.id = c.credential_id
		WHERE c.enabled = 1 AND p.enabled = 1 AND p.archived_at IS NULL
			AND cr.platform = 'bilibili_open_live' AND cr.purpose = 'LIVE_ANALYTICS'`)
	if err != nil {
		return nil, fmt.Errorf("list enabled live analytics configs: %w", err)
	}
	defer rows.Close()
	requests := []CaptureRequest{}
	for rows.Next() {
		var req CaptureRequest
		var encrypted []byte
		if err := rows.Scan(&req.RecordingProfileID, &req.ExpectedRoomID, &req.CredentialID, &req.AppID, &encrypted); err != nil {
			return nil, err
		}
		plain, err := secretbox.Decrypt(s.cfg.MasterKeyPath, encrypted)
		if err != nil {
			s.setConfigError(ctx, req.RecordingProfileID, "decrypt OpenLive credential failed")
			continue
		}
		var secret struct {
			AccessKeyID     string `json:"access_key_id"`
			AccessKeySecret string `json:"access_key_secret"`
			IdentityCode    string `json:"identity_code"`
		}
		if json.Unmarshal(plain, &secret) != nil || strings.TrimSpace(secret.AccessKeyID) == "" || strings.TrimSpace(secret.AccessKeySecret) == "" || strings.TrimSpace(secret.IdentityCode) == "" {
			s.setConfigError(ctx, req.RecordingProfileID, "OpenLive credential is incomplete")
			continue
		}
		req.AccessKeyID, req.AccessKeySecret, req.IdentityCode = secret.AccessKeyID, secret.AccessKeySecret, secret.IdentityCode
		requests = append(requests, req)
	}
	return requests, rows.Err()
}

func (s Store) InterruptActive(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `UPDATE live_capture_sessions SET status = 'INTERRUPTED', ended_at = CURRENT_TIMESTAMP,
		gap_count = gap_count + 1, raw_status = CASE WHEN raw_relative_path IS NULL THEN 'PENDING' ELSE 'AVAILABLE' END,
		last_error = 'collector process stopped before closing the session', updated_at = CURRENT_TIMESTAMP
		WHERE status IN ('STARTING', 'CONNECTED', 'RECONNECTING')`)
	if err != nil {
		return err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM live_capture_sessions
		WHERE status = 'INTERRUPTED' AND raw_status = 'AVAILABLE' AND raw_relative_path IS NOT NULL`)
	if err != nil {
		return err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, id := range ids {
		_ = s.refreshRawMetadata(ctx, id)
	}
	return nil
}

func (s Store) CreateSession(ctx context.Context, req CaptureRequest, start StartResult) (int64, error) {
	result, err := s.db.ExecContext(ctx, `INSERT INTO live_capture_sessions
		(recording_profile_id, external_game_id, status, room_id, anchor_uid, anchor_open_id, anchor_union_id, anchor_name, anchor_face_url)
		VALUES (?, ?, 'STARTING', ?, NULLIF(?, 0), NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''))`, req.RecordingProfileID, start.GameID,
		strconv.FormatInt(start.RoomID, 10), start.AnchorUID, start.OpenID, start.UnionID, start.Name, start.FaceURL)
	if err != nil {
		return 0, fmt.Errorf("create live capture session: %w", err)
	}
	id, err := result.LastInsertId()
	if err == nil {
		_, _ = s.db.ExecContext(ctx, `UPDATE live_analytics_configs SET last_started_at = CURRENT_TIMESTAMP,
			last_error = NULL, updated_at = CURRENT_TIMESTAMP WHERE recording_profile_id = ?`, req.RecordingProfileID)
	}
	return id, err
}

func (s Store) SetRawPath(ctx context.Context, id int64, path string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE live_capture_sessions SET raw_relative_path = ?, raw_status = 'WRITING',
		updated_at = CURRENT_TIMESTAMP WHERE id = ?`, path, id)
	return err
}

func (s Store) MarkConnected(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE live_capture_sessions SET status = 'CONNECTED', connected_at = CURRENT_TIMESTAMP,
		last_heartbeat_at = CURRENT_TIMESTAMP, last_error = NULL, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, id)
	return err
}

func (s Store) MarkHeartbeat(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE live_capture_sessions SET last_heartbeat_at = CURRENT_TIMESTAMP,
		updated_at = CURRENT_TIMESTAMP WHERE id = ?`, id)
	if err != nil {
		return err
	}
	_ = s.refreshRawSize(ctx, id)
	return nil
}

func (s Store) UpdateStats(ctx context.Context, id int64, counts map[string]int, total, unknown, gaps int64, lastEvent time.Time) error {
	encoded, _ := json.Marshal(counts)
	lastEventValue := any(nil)
	if !lastEvent.IsZero() {
		lastEventValue = lastEvent.UTC().Format(time.RFC3339Nano)
	}
	_, err := s.db.ExecContext(ctx, `UPDATE live_capture_sessions SET event_count = ?, unknown_event_count = ?,
		gap_count = ?, event_counts_json = ?, last_event_at = COALESCE(?, last_event_at),
		updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		total, unknown, gaps, string(encoded), lastEventValue, id)
	return err
}

func (s Store) FinishSession(ctx context.Context, id int64, status, message string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE live_capture_sessions SET status = ?, ended_at = CURRENT_TIMESTAMP,
		last_error = NULLIF(?, ''), updated_at = CURRENT_TIMESTAMP WHERE id = ?`, status, message, id)
	if err != nil {
		return err
	}
	_ = s.refreshRawMetadata(ctx, id)
	return nil
}

func (s Store) RecordConfigError(ctx context.Context, profileID int64, message string) {
	s.setConfigError(ctx, profileID, message)
}

func (s Store) setConfigError(ctx context.Context, profileID int64, message string) {
	_, _ = s.db.ExecContext(ctx, `UPDATE live_analytics_configs SET last_error = ?, updated_at = CURRENT_TIMESTAMP WHERE recording_profile_id = ?`, message, profileID)
}

func (s Store) ensureProfileVisible(ctx context.Context, actor account.User, profileID int64) error {
	var ownerID int64
	if err := s.db.QueryRowContext(ctx, `SELECT owner_user_id FROM recording_profiles WHERE id = ? AND archived_at IS NULL`, profileID).Scan(&ownerID); errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return err
	}
	if actor.Role != account.RoleSuperAdmin && ownerID != actor.ID {
		return ErrForbidden
	}
	return nil
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
