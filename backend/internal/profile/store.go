package profile

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/7grecorder/7grecorder/backend/internal/account"
)

var (
	ErrNotFound        = errors.New("profile not found")
	ErrForbidden       = errors.New("profile forbidden")
	ErrValidation      = errors.New("profile validation failed")
	ErrRoomInUse       = errors.New("room is already configured")
	ErrRecordingActive = errors.New("profile has an active recording")
	ErrManagerLimit    = errors.New("manager profile limit reached")
)

type RecordingSettings struct {
	AutoRecord             bool   `json:"auto_record"`
	Quality                string `json:"quality"`
	RecordDanmaku          bool   `json:"record_danmaku"`
	SegmentDurationSec     int64  `json:"segment_duration_sec"`
	FinalizeGracePeriodSec int64  `json:"finalize_grace_period_sec"`
	UpdatedAt              string `json:"updated_at"`
}

type Runtime struct {
	StreamStatus       string `json:"stream_status"`
	RecorderStatus     string `json:"recorder_status"`
	SyncStatus         string `json:"sync_status"`
	CurrentRecordingID *int64 `json:"current_recording_id"`
	LastEventAt        string `json:"last_event_at,omitempty"`
	LastReconciledAt   string `json:"last_reconciled_at,omitempty"`
	LastError          string `json:"last_error,omitempty"`
	UpdatedAt          string `json:"updated_at"`
}

type RecordingProfile struct {
	ID            int64             `json:"id"`
	OwnerUserID   int64             `json:"owner_user_id"`
	Name          string            `json:"name"`
	Platform      string            `json:"platform"`
	RoomID        string            `json:"room_id"`
	StreamerName  string            `json:"streamer_name"`
	StreamerUID   string            `json:"streamer_uid,omitempty"`
	Timezone      string            `json:"timezone"`
	Enabled       bool              `json:"enabled"`
	PublicEnabled bool              `json:"public_enabled"`
	PublicSlug    string            `json:"public_slug,omitempty"`
	ArchivedAt    string            `json:"archived_at,omitempty"`
	CreatedAt     string            `json:"created_at"`
	UpdatedAt     string            `json:"updated_at"`
	Settings      RecordingSettings `json:"recording_settings"`
	Runtime       Runtime           `json:"runtime"`
}

type CreateRequest struct {
	Name          string          `json:"name"`
	RoomID        string          `json:"room_id"`
	StreamerName  string          `json:"streamer_name"`
	StreamerUID   string          `json:"streamer_uid"`
	Timezone      string          `json:"timezone"`
	Enabled       *bool           `json:"enabled"`
	PublicEnabled *bool           `json:"public_enabled"`
	PublicSlug    string          `json:"public_slug"`
	Settings      *SettingsUpsert `json:"recording_settings"`
}

type UpdateRequest struct {
	Name          *string `json:"name"`
	RoomID        *string `json:"room_id"`
	StreamerName  *string `json:"streamer_name"`
	StreamerUID   *string `json:"streamer_uid"`
	Timezone      *string `json:"timezone"`
	Enabled       *bool   `json:"enabled"`
	PublicEnabled *bool   `json:"public_enabled"`
	PublicSlug    *string `json:"public_slug"`
	Archived      *bool   `json:"archived"`
}

type SettingsUpsert struct {
	AutoRecord             *bool   `json:"auto_record"`
	Quality                *string `json:"quality"`
	RecordDanmaku          *bool   `json:"record_danmaku"`
	SegmentDurationSec     *int64  `json:"segment_duration_sec"`
	FinalizeGracePeriodSec *int64  `json:"finalize_grace_period_sec"`
}

type RecorderSyncPayload struct {
	ProfileID              int64  `json:"profile_id"`
	RoomID                 string `json:"room_id"`
	LiveURL                string `json:"live_url"`
	Enabled                bool   `json:"enabled"`
	AutoRecord             bool   `json:"auto_record"`
	Quality                string `json:"quality"`
	RecordDanmaku          bool   `json:"record_danmaku"`
	SegmentDurationSec     int64  `json:"segment_duration_sec"`
	FinalizeGracePeriodSec int64  `json:"finalize_grace_period_sec"`
	OutputRelativeDir      string `json:"output_relative_dir"`
	WebhookPath            string `json:"webhook_path"`
}

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) Store {
	return Store{db: db}
}

func (s Store) List(ctx context.Context, actor account.User) ([]RecordingProfile, error) {
	query := `
		SELECT id, owner_user_id, name, platform, room_id, streamer_name, COALESCE(streamer_uid, ''),
			timezone, enabled, public_enabled, COALESCE(public_slug, ''), COALESCE(archived_at, ''),
			created_at, updated_at
		FROM recording_profiles
	`
	args := []interface{}{}
	if actor.Role != account.RoleSuperAdmin {
		query += " WHERE owner_user_id = ?"
		args = append(args, actor.ID)
	}
	query += " ORDER BY archived_at IS NOT NULL ASC, updated_at DESC, id DESC"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list profiles: %w", err)
	}
	defer rows.Close()

	profiles := make([]RecordingProfile, 0)
	for rows.Next() {
		profile, err := scanProfileRow(rows)
		if err != nil {
			return nil, err
		}
		profile.Settings, err = s.settings(ctx, profile.ID)
		if err != nil {
			return nil, err
		}
		profile.Runtime, err = s.runtime(ctx, profile.ID)
		if err != nil {
			return nil, err
		}
		profiles = append(profiles, profile)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate profiles: %w", err)
	}
	return profiles, nil
}

func (s Store) Create(ctx context.Context, actor account.User, req CreateRequest) (RecordingProfile, error) {
	if err := s.ensureCanEditRecordingProfile(ctx, actor); err != nil {
		return RecordingProfile{}, err
	}

	name := strings.TrimSpace(req.Name)
	roomID := strings.TrimSpace(req.RoomID)
	streamerName := strings.TrimSpace(req.StreamerName)
	timezone := normalizeDefault(req.Timezone, "Asia/Shanghai")
	if name == "" || roomID == "" || streamerName == "" {
		return RecordingProfile{}, ErrValidation
	}
	if actor.Role == account.RoleManager {
		var count int
		err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM recording_profiles WHERE owner_user_id = ? AND archived_at IS NULL", actor.ID).Scan(&count)
		if err != nil {
			return RecordingProfile{}, fmt.Errorf("count manager profiles: %w", err)
		}
		if count >= 1 {
			return RecordingProfile{}, ErrManagerLimit
		}
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return RecordingProfile{}, fmt.Errorf("begin profile create: %w", err)
	}
	defer tx.Rollback()

	enabled := boolDefault(req.Enabled, true)
	publicEnabled := boolDefault(req.PublicEnabled, false)
	result, err := tx.ExecContext(ctx, `
		INSERT INTO recording_profiles
			(owner_user_id, name, platform, room_id, streamer_name, streamer_uid, timezone, enabled, public_enabled, public_slug)
		VALUES (?, ?, 'bilibili', ?, ?, NULLIF(?, ''), ?, ?, ?, NULLIF(?, ''))
	`, actor.ID, name, roomID, streamerName, strings.TrimSpace(req.StreamerUID), timezone, boolInt(enabled), boolInt(publicEnabled), strings.TrimSpace(req.PublicSlug))
	if isUniqueConstraint(err) {
		return RecordingProfile{}, ErrRoomInUse
	}
	if err != nil {
		return RecordingProfile{}, fmt.Errorf("insert profile: %w", err)
	}
	profileID, err := result.LastInsertId()
	if err != nil {
		return RecordingProfile{}, fmt.Errorf("read profile id: %w", err)
	}

	settings := defaultSettings()
	if req.Settings != nil {
		settings = mergeSettings(settings, *req.Settings)
	}
	if err := validateSettings(settings); err != nil {
		return RecordingProfile{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO recording_settings
			(recording_profile_id, auto_record, quality, record_danmaku, segment_duration_sec, finalize_grace_period_sec)
		VALUES (?, ?, ?, ?, ?, ?)
	`, profileID, boolInt(settings.AutoRecord), settings.Quality, boolInt(settings.RecordDanmaku), settings.SegmentDurationSec, settings.FinalizeGracePeriodSec); err != nil {
		return RecordingProfile{}, fmt.Errorf("insert profile settings: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO recording_profile_runtime
			(recording_profile_id, stream_status, recorder_status, sync_status)
		VALUES (?, 'UNKNOWN', 'UNKNOWN', 'PENDING')
	`, profileID); err != nil {
		return RecordingProfile{}, fmt.Errorf("insert profile runtime: %w", err)
	}
	if err := enqueueRecorderSync(ctx, tx, recorderSyncPayload(profileID, roomID, enabled, "", settings)); err != nil {
		return RecordingProfile{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO audit_logs (actor_user_id, action, resource_type, resource_id, summary)
		VALUES (?, 'PROFILE_CREATE', 'recording_profile', ?, ?)
	`, actor.ID, profileID, "Created recording profile"); err != nil {
		return RecordingProfile{}, fmt.Errorf("write audit log: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return RecordingProfile{}, fmt.Errorf("commit profile create: %w", err)
	}
	return s.Get(ctx, actor, profileID)
}

func (s Store) Get(ctx context.Context, actor account.User, id int64) (RecordingProfile, error) {
	profile, err := s.profileByID(ctx, id)
	if err != nil {
		return RecordingProfile{}, err
	}
	if actor.Role != account.RoleSuperAdmin && profile.OwnerUserID != actor.ID {
		return RecordingProfile{}, ErrForbidden
	}
	profile.Settings, err = s.settings(ctx, id)
	if err != nil {
		return RecordingProfile{}, err
	}
	profile.Runtime, err = s.runtime(ctx, id)
	if err != nil {
		return RecordingProfile{}, err
	}
	return profile, nil
}

func (s Store) Update(ctx context.Context, actor account.User, id int64, req UpdateRequest) (RecordingProfile, error) {
	if err := s.ensureCanEditRecordingProfile(ctx, actor); err != nil {
		return RecordingProfile{}, err
	}

	current, err := s.Get(ctx, actor, id)
	if err != nil {
		return RecordingProfile{}, err
	}
	if req.RoomID != nil && strings.TrimSpace(*req.RoomID) != current.RoomID {
		active, err := s.hasActiveRecording(ctx, id)
		if err != nil {
			return RecordingProfile{}, err
		}
		if active {
			return RecordingProfile{}, ErrRecordingActive
		}
	}

	name := current.Name
	roomID := current.RoomID
	streamerName := current.StreamerName
	streamerUID := current.StreamerUID
	timezone := current.Timezone
	enabled := current.Enabled
	publicEnabled := current.PublicEnabled
	publicSlug := current.PublicSlug
	archivedAt := current.ArchivedAt

	if req.Name != nil {
		name = strings.TrimSpace(*req.Name)
	}
	if req.RoomID != nil {
		roomID = strings.TrimSpace(*req.RoomID)
	}
	if req.StreamerName != nil {
		streamerName = strings.TrimSpace(*req.StreamerName)
	}
	if req.StreamerUID != nil {
		streamerUID = strings.TrimSpace(*req.StreamerUID)
	}
	if req.Timezone != nil {
		timezone = normalizeDefault(*req.Timezone, "Asia/Shanghai")
	}
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	if req.PublicEnabled != nil {
		publicEnabled = *req.PublicEnabled
	}
	if req.PublicSlug != nil {
		publicSlug = strings.TrimSpace(*req.PublicSlug)
	}
	if req.Archived != nil {
		if *req.Archived && archivedAt == "" {
			archivedAt = time.Now().UTC().Format(time.RFC3339)
			enabled = false
		}
		if !*req.Archived {
			archivedAt = ""
		}
	}
	if name == "" || roomID == "" || streamerName == "" {
		return RecordingProfile{}, ErrValidation
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return RecordingProfile{}, fmt.Errorf("begin profile update: %w", err)
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, `
		UPDATE recording_profiles
		SET name = ?, room_id = ?, streamer_name = ?, streamer_uid = NULLIF(?, ''), timezone = ?,
			enabled = ?, public_enabled = ?, public_slug = NULLIF(?, ''), archived_at = NULLIF(?, ''),
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, name, roomID, streamerName, streamerUID, timezone, boolInt(enabled), boolInt(publicEnabled), publicSlug, archivedAt, id)
	if isUniqueConstraint(err) {
		return RecordingProfile{}, ErrRoomInUse
	}
	if err != nil {
		return RecordingProfile{}, fmt.Errorf("update profile: %w", err)
	}
	settings := current.Settings
	if err := markRecorderSyncPending(ctx, tx, id); err != nil {
		return RecordingProfile{}, err
	}
	if err := enqueueRecorderSync(ctx, tx, recorderSyncPayload(id, roomID, enabled, archivedAt, settings)); err != nil {
		return RecordingProfile{}, err
	}
	if err := tx.Commit(); err != nil {
		return RecordingProfile{}, fmt.Errorf("commit profile update: %w", err)
	}
	return s.Get(ctx, actor, id)
}

func (s Store) UpsertSettings(ctx context.Context, actor account.User, profileID int64, req SettingsUpsert) (RecordingSettings, error) {
	if err := s.ensureCanEditRecordingProfile(ctx, actor); err != nil {
		return RecordingSettings{}, err
	}

	settings, err := s.settings(ctx, profileID)
	if err != nil {
		return RecordingSettings{}, err
	}
	profile, err := s.Get(ctx, actor, profileID)
	if err != nil {
		return RecordingSettings{}, err
	}
	if coreSettingsChanging(settings, req) {
		active, err := s.hasActiveRecording(ctx, profileID)
		if err != nil {
			return RecordingSettings{}, err
		}
		if active {
			return RecordingSettings{}, ErrRecordingActive
		}
	}
	settings = mergeSettings(settings, req)
	if err := validateSettings(settings); err != nil {
		return RecordingSettings{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return RecordingSettings{}, fmt.Errorf("begin profile settings update: %w", err)
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, `
		UPDATE recording_settings
		SET auto_record = ?, quality = ?, record_danmaku = ?, segment_duration_sec = ?,
			finalize_grace_period_sec = ?, updated_at = CURRENT_TIMESTAMP
		WHERE recording_profile_id = ?
	`, boolInt(settings.AutoRecord), settings.Quality, boolInt(settings.RecordDanmaku), settings.SegmentDurationSec, settings.FinalizeGracePeriodSec, profileID)
	if err != nil {
		return RecordingSettings{}, fmt.Errorf("update profile settings: %w", err)
	}
	if err := markRecorderSyncPending(ctx, tx, profileID); err != nil {
		return RecordingSettings{}, err
	}
	if err := enqueueRecorderSync(ctx, tx, recorderSyncPayload(profileID, profile.RoomID, profile.Enabled, profile.ArchivedAt, settings)); err != nil {
		return RecordingSettings{}, err
	}
	if err := tx.Commit(); err != nil {
		return RecordingSettings{}, fmt.Errorf("commit profile settings update: %w", err)
	}
	return s.settings(ctx, profileID)
}

func (s Store) profileByID(ctx context.Context, id int64) (RecordingProfile, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, owner_user_id, name, platform, room_id, streamer_name, COALESCE(streamer_uid, ''),
			timezone, enabled, public_enabled, COALESCE(public_slug, ''), COALESCE(archived_at, ''),
			created_at, updated_at
		FROM recording_profiles
		WHERE id = ?
	`, id)
	profile, err := scanProfileRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return RecordingProfile{}, ErrNotFound
	}
	return profile, err
}

func (s Store) settings(ctx context.Context, profileID int64) (RecordingSettings, error) {
	var settings RecordingSettings
	var autoRecord int
	var recordDanmaku int
	err := s.db.QueryRowContext(ctx, `
		SELECT auto_record, quality, record_danmaku, segment_duration_sec, finalize_grace_period_sec, updated_at
		FROM recording_settings
		WHERE recording_profile_id = ?
	`, profileID).Scan(&autoRecord, &settings.Quality, &recordDanmaku, &settings.SegmentDurationSec, &settings.FinalizeGracePeriodSec, &settings.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return RecordingSettings{}, ErrNotFound
	}
	if err != nil {
		return RecordingSettings{}, fmt.Errorf("load profile settings: %w", err)
	}
	settings.AutoRecord = autoRecord == 1
	settings.RecordDanmaku = recordDanmaku == 1
	return settings, nil
}

func (s Store) runtime(ctx context.Context, profileID int64) (Runtime, error) {
	var runtime Runtime
	var currentRecordingID sql.NullInt64
	err := s.db.QueryRowContext(ctx, `
		SELECT stream_status, recorder_status, sync_status, current_recording_id,
			COALESCE(last_event_at, ''), COALESCE(last_reconciled_at, ''), COALESCE(last_error, ''), updated_at
		FROM recording_profile_runtime
		WHERE recording_profile_id = ?
	`, profileID).Scan(&runtime.StreamStatus, &runtime.RecorderStatus, &runtime.SyncStatus, &currentRecordingID, &runtime.LastEventAt, &runtime.LastReconciledAt, &runtime.LastError, &runtime.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Runtime{}, ErrNotFound
	}
	if err != nil {
		return Runtime{}, fmt.Errorf("load profile runtime: %w", err)
	}
	if currentRecordingID.Valid {
		runtime.CurrentRecordingID = &currentRecordingID.Int64
	}
	return runtime, nil
}

func (s Store) hasActiveRecording(ctx context.Context, profileID int64) (bool, error) {
	var count int
	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM recordings
		WHERE recording_profile_id = ? AND recording_status IN ('ACTIVE', 'FINALIZING')
	`, profileID).Scan(&count); err != nil {
		return false, fmt.Errorf("count active recordings: %w", err)
	}
	return count > 0, nil
}

func (s Store) ensureCanEditRecordingProfile(ctx context.Context, actor account.User) error {
	if actor.Role != account.RoleManager {
		return nil
	}
	var canEdit int
	err := s.db.QueryRowContext(ctx, `
		SELECT can_edit_recording_profile
		FROM manager_policies
		WHERE user_id = ?
	`, actor.ID).Scan(&canEdit)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("load manager profile policy: %w", err)
	}
	if canEdit != 1 {
		return ErrForbidden
	}
	return nil
}

type txExecutor interface {
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
}

func markRecorderSyncPending(ctx context.Context, exec txExecutor, profileID int64) error {
	if _, err := exec.ExecContext(ctx, `
		UPDATE recording_profile_runtime
		SET sync_status = 'PENDING', last_error = NULL, updated_at = CURRENT_TIMESTAMP
		WHERE recording_profile_id = ?
	`, profileID); err != nil {
		return fmt.Errorf("mark recorder sync pending: %w", err)
	}
	return nil
}

func enqueueRecorderSync(ctx context.Context, exec txExecutor, payload RecorderSyncPayload) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal recorder sync payload: %w", err)
	}
	businessKey := fmt.Sprintf("profile:%d:recorder:sync", payload.ProfileID)
	if _, err := exec.ExecContext(ctx, `
		INSERT INTO jobs
			(recording_profile_id, type, resource_class, business_key, payload_json, status, priority, max_attempts)
		VALUES (?, 'SYNC_RECORDER_PROFILE', 'LIGHT', ?, ?, 'PENDING', 20, 5)
		ON CONFLICT(business_key) WHERE business_key IS NOT NULL DO UPDATE SET
			payload_json = excluded.payload_json,
			status = 'PENDING',
			priority = excluded.priority,
			attempts = 0,
			max_attempts = excluded.max_attempts,
			run_after = CURRENT_TIMESTAMP,
			locked_at = NULL,
			heartbeat_at = NULL,
			locked_by = NULL,
			last_error_class = NULL,
			last_error = NULL,
			updated_at = CURRENT_TIMESTAMP
	`, payload.ProfileID, businessKey, string(body)); err != nil {
		return fmt.Errorf("enqueue recorder sync: %w", err)
	}
	return nil
}

func recorderSyncPayload(profileID int64, roomID string, enabled bool, archivedAt string, settings RecordingSettings) RecorderSyncPayload {
	shouldRecord := enabled && archivedAt == ""
	return RecorderSyncPayload{
		ProfileID:              profileID,
		RoomID:                 roomID,
		LiveURL:                "https://live.bilibili.com/" + roomID,
		Enabled:                shouldRecord,
		AutoRecord:             shouldRecord && settings.AutoRecord,
		Quality:                settings.Quality,
		RecordDanmaku:          settings.RecordDanmaku,
		SegmentDurationSec:     settings.SegmentDurationSec,
		FinalizeGracePeriodSec: settings.FinalizeGracePeriodSec,
		OutputRelativeDir:      fmt.Sprintf("recordings/%d", profileID),
		WebhookPath:            "/internal/v1/recorder/webhook",
	}
}

type profileScanner interface {
	Scan(dest ...interface{}) error
}

func scanProfileRow(row profileScanner) (RecordingProfile, error) {
	var profile RecordingProfile
	var enabled int
	var publicEnabled int
	err := row.Scan(
		&profile.ID,
		&profile.OwnerUserID,
		&profile.Name,
		&profile.Platform,
		&profile.RoomID,
		&profile.StreamerName,
		&profile.StreamerUID,
		&profile.Timezone,
		&enabled,
		&publicEnabled,
		&profile.PublicSlug,
		&profile.ArchivedAt,
		&profile.CreatedAt,
		&profile.UpdatedAt,
	)
	if err != nil {
		return RecordingProfile{}, err
	}
	profile.Enabled = enabled == 1
	profile.PublicEnabled = publicEnabled == 1
	return profile, nil
}

func defaultSettings() RecordingSettings {
	return RecordingSettings{
		AutoRecord:             true,
		Quality:                "original",
		RecordDanmaku:          true,
		SegmentDurationSec:     1800,
		FinalizeGracePeriodSec: 300,
	}
}

func mergeSettings(settings RecordingSettings, req SettingsUpsert) RecordingSettings {
	if req.AutoRecord != nil {
		settings.AutoRecord = *req.AutoRecord
	}
	if req.Quality != nil {
		settings.Quality = strings.TrimSpace(*req.Quality)
	}
	if req.RecordDanmaku != nil {
		settings.RecordDanmaku = *req.RecordDanmaku
	}
	if req.SegmentDurationSec != nil {
		settings.SegmentDurationSec = *req.SegmentDurationSec
	}
	if req.FinalizeGracePeriodSec != nil {
		settings.FinalizeGracePeriodSec = *req.FinalizeGracePeriodSec
	}
	return settings
}

func validateSettings(settings RecordingSettings) error {
	if settings.Quality == "" || settings.SegmentDurationSec < 60 || settings.FinalizeGracePeriodSec < 0 {
		return ErrValidation
	}
	return nil
}

func coreSettingsChanging(current RecordingSettings, req SettingsUpsert) bool {
	if req.AutoRecord != nil && *req.AutoRecord != current.AutoRecord {
		return true
	}
	if req.Quality != nil && strings.TrimSpace(*req.Quality) != current.Quality {
		return true
	}
	if req.RecordDanmaku != nil && *req.RecordDanmaku != current.RecordDanmaku {
		return true
	}
	if req.SegmentDurationSec != nil && *req.SegmentDurationSec != current.SegmentDurationSec {
		return true
	}
	if req.FinalizeGracePeriodSec != nil && *req.FinalizeGracePeriodSec != current.FinalizeGracePeriodSec {
		return true
	}
	return false
}

func boolDefault(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}
	return *value
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func normalizeDefault(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func isUniqueConstraint(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "unique constraint")
}
