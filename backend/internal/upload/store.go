package upload

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/7grecorder/7grecorder/backend/internal/account"
	"github.com/7grecorder/7grecorder/backend/internal/config"
)

var (
	ErrForbidden  = errors.New("upload forbidden")
	ErrNotFound   = errors.New("upload resource not found")
	ErrNotReady   = errors.New("upload resource not ready")
	ErrValidation = errors.New("upload validation failed")
)

type Store struct {
	db  *sql.DB
	cfg config.Config
}

func NewStore(database *sql.DB, cfg config.Config) Store {
	return Store{db: database, cfg: cfg}
}

type Credential struct {
	ID             int64  `json:"id"`
	OwnerUserID    int64  `json:"owner_user_id,omitempty"`
	Scope          string `json:"scope"`
	Platform       string `json:"platform"`
	Purpose        string `json:"purpose"`
	AccountLabel   string `json:"account_label"`
	ExternalUID    string `json:"external_uid,omitempty"`
	Status         string `json:"status"`
	LastVerifiedAt string `json:"last_verified_at,omitempty"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

type CredentialCreate struct {
	Scope        string          `json:"scope"`
	Platform     string          `json:"platform"`
	Purpose      string          `json:"purpose"`
	AccountLabel string          `json:"account_label"`
	ExternalUID  string          `json:"external_uid"`
	Secret       json.RawMessage `json:"secret"`
}

type PublishingConfig struct {
	ID                 int64           `json:"id,omitempty"`
	RecordingProfileID int64           `json:"recording_profile_id"`
	Platform           string          `json:"platform"`
	CredentialID       int64           `json:"credential_id,omitempty"`
	Enabled            bool            `json:"enabled"`
	Settings           json.RawMessage `json:"settings,omitempty"`
	CreatedAt          string          `json:"created_at,omitempty"`
	UpdatedAt          string          `json:"updated_at,omitempty"`
}

type PublishingConfigUpsert struct {
	CredentialID int64           `json:"credential_id"`
	Enabled      bool            `json:"enabled"`
	Settings     json.RawMessage `json:"settings"`
}

type COSConfig struct {
	ID                 int64  `json:"id,omitempty"`
	RecordingProfileID int64  `json:"recording_profile_id"`
	CredentialID       int64  `json:"credential_id,omitempty"`
	Enabled            bool   `json:"enabled"`
	Region             string `json:"region"`
	Bucket             string `json:"bucket"`
	Prefix             string `json:"prefix"`
	MaxManagedBytes    int64  `json:"max_managed_bytes"`
	CreatedAt          string `json:"created_at,omitempty"`
	UpdatedAt          string `json:"updated_at,omitempty"`
}

const COSCompressionPresetH264CRF23MediumMP4 = "h264_crf23_medium_mp4"

type COSConfigUpsert struct {
	CredentialID    int64  `json:"credential_id"`
	Enabled         bool   `json:"enabled"`
	Region          string `json:"region"`
	Bucket          string `json:"bucket"`
	Prefix          string `json:"prefix"`
	MaxManagedBytes int64  `json:"max_managed_bytes"`
}

type ReconcileResult struct {
	PublicationsCreated   int `json:"publications_created"`
	BilibiliJobsCreated   int `json:"bilibili_jobs_created"`
	COSObjectsCreated     int `json:"cos_objects_created"`
	COSJobsCreated        int `json:"cos_jobs_created"`
	COSFileObjectsCreated int `json:"cos_file_objects_created"`
	COSFileJobsCreated    int `json:"cos_file_jobs_created"`
}

type COSJobPayload struct {
	COSObjectID    int64 `json:"cos_object_id"`
	UploadSourceID int64 `json:"upload_source_id"`
	OutputID       int64 `json:"output_id"`
}

const uploadSourceStableCondition = `
			NOT EXISTS (
				SELECT 1
				FROM recordings next
				WHERE next.recording_profile_id = us.recording_profile_id
					AND next.local_storage_status != 'DELETED'
					AND next.local_deleted_at IS NULL
					AND next.started_at > us.completed_at
					AND ((julianday(next.started_at) - julianday(us.completed_at)) * 86400.0) <= us.merge_gap_threshold_seconds
					AND NOT EXISTS (
						SELECT 1
						FROM upload_source_segments same_segment
						WHERE same_segment.upload_source_id = us.id
							AND same_segment.recording_id = next.id
					)
					AND (
						next.recording_status = 'ACTIVE'
						OR EXISTS (
							SELECT 1
							FROM recording_files f
							WHERE f.recording_id = next.id
								AND LOWER(f.kind) = 'video'
								AND f.file_status IN ('CLOSED', 'WRITING')
								AND f.deleted_at IS NULL
						)
					)
			)`

const uploadSourceReviewReadyCondition = `COALESCE(us.review_status, 'NONE') != 'REQUIRED'
			AND COALESCE(us.edit_decision_json, '') = ''
			AND COALESCE(us.local_cleanup_status, 'AVAILABLE') = 'AVAILABLE'`

type COSRecordingFileJobPayload struct {
	COSObjectID     int64 `json:"cos_object_id"`
	RecordingFileID int64 `json:"recording_file_id"`
}

func (s Store) ListCredentials(ctx context.Context, actor account.User) ([]Credential, error) {
	query := `
		SELECT id, COALESCE(owner_user_id, 0), scope, platform, purpose, account_label,
			COALESCE(external_uid, ''), status, COALESCE(last_verified_at, ''), created_at, updated_at
		FROM credentials
	`
	args := []interface{}{}
	if actor.Role != account.RoleSuperAdmin {
		query += " WHERE owner_user_id = ?"
		args = append(args, actor.ID)
	}
	query += " ORDER BY updated_at DESC, id DESC"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list credentials: %w", err)
	}
	defer rows.Close()

	items := make([]Credential, 0)
	for rows.Next() {
		var item Credential
		if err := rows.Scan(&item.ID, &item.OwnerUserID, &item.Scope, &item.Platform, &item.Purpose, &item.AccountLabel, &item.ExternalUID, &item.Status, &item.LastVerifiedAt, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan credential: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate credentials: %w", err)
	}
	return items, nil
}

func (s Store) CreateCredential(ctx context.Context, actor account.User, req CredentialCreate) (Credential, error) {
	scope := strings.ToUpper(strings.TrimSpace(req.Scope))
	platform := strings.ToLower(strings.TrimSpace(req.Platform))
	purpose := strings.ToUpper(strings.TrimSpace(req.Purpose))
	label := strings.TrimSpace(req.AccountLabel)
	secret := strings.TrimSpace(string(req.Secret))
	if scope == "" {
		scope = "USER"
	}
	if actor.Role != account.RoleSuperAdmin && scope != "USER" {
		return Credential{}, ErrForbidden
	}
	if scope != "USER" && scope != "SYSTEM" {
		return Credential{}, ErrValidation
	}
	if platform == "" || purpose == "" || label == "" || secret == "" || secret == "null" {
		return Credential{}, ErrValidation
	}
	if !json.Valid(req.Secret) {
		return Credential{}, ErrValidation
	}
	encrypted, err := s.encryptSecret(req.Secret)
	if err != nil {
		return Credential{}, err
	}
	var ownerID interface{}
	if scope == "USER" {
		ownerID = actor.ID
	}
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO credentials
			(owner_user_id, scope, platform, purpose, account_label, external_uid, encrypted_secret, status)
		VALUES (?, ?, ?, ?, ?, NULLIF(?, ''), ?, 'UNVERIFIED')
	`, ownerID, scope, platform, purpose, label, strings.TrimSpace(req.ExternalUID), encrypted)
	if err != nil {
		return Credential{}, fmt.Errorf("create credential: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return Credential{}, fmt.Errorf("read credential id: %w", err)
	}
	return s.getCredential(ctx, actor, id)
}

func (s Store) GetBilibiliConfig(ctx context.Context, actor account.User, profileID int64) (PublishingConfig, error) {
	if err := s.ensureProfileVisible(ctx, actor, profileID); err != nil {
		return PublishingConfig{}, err
	}
	item, err := s.getPublishingConfig(ctx, profileID, "bilibili")
	if errors.Is(err, sql.ErrNoRows) {
		return PublishingConfig{RecordingProfileID: profileID, Platform: "bilibili", Settings: json.RawMessage("{}")}, nil
	}
	if err != nil {
		return PublishingConfig{}, err
	}
	return item, nil
}

func (s Store) UpsertBilibiliConfig(ctx context.Context, actor account.User, profileID int64, req PublishingConfigUpsert) (PublishingConfig, error) {
	if err := s.ensureCanEditModule(ctx, actor, profileID, "bilibili"); err != nil {
		return PublishingConfig{}, err
	}
	settings := normalizeJSON(req.Settings)
	if !json.Valid([]byte(settings)) {
		return PublishingConfig{}, ErrValidation
	}
	normalizedSettings, err := NormalizeBilibiliSettings([]byte(settings))
	if err != nil {
		return PublishingConfig{}, err
	}
	if req.Enabled && req.CredentialID <= 0 {
		return PublishingConfig{}, ErrValidation
	}
	if req.CredentialID > 0 {
		if err := s.ensureCredentialVisible(ctx, actor, req.CredentialID, "bilibili", "PUBLISHER"); err != nil {
			return PublishingConfig{}, err
		}
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO publishing_profiles
			(recording_profile_id, platform, credential_id, enabled, settings_json)
		VALUES (?, 'bilibili', NULLIF(?, 0), ?, ?)
		ON CONFLICT(recording_profile_id, platform) DO UPDATE SET
			credential_id = NULLIF(excluded.credential_id, 0),
			enabled = excluded.enabled,
			settings_json = excluded.settings_json,
			updated_at = CURRENT_TIMESTAMP
	`, profileID, req.CredentialID, boolInt(req.Enabled), string(normalizedSettings))
	if err != nil {
		return PublishingConfig{}, fmt.Errorf("upsert bilibili config: %w", err)
	}
	return s.GetBilibiliConfig(ctx, actor, profileID)
}

func (s Store) GetCOSConfig(ctx context.Context, actor account.User, profileID int64) (COSConfig, error) {
	if err := s.ensureProfileVisible(ctx, actor, profileID); err != nil {
		return COSConfig{}, err
	}
	item, err := s.getCOSConfig(ctx, profileID)
	if errors.Is(err, sql.ErrNoRows) {
		return COSConfig{RecordingProfileID: profileID, Prefix: fmt.Sprintf("7grecorder/%d/", profileID)}, nil
	}
	if err != nil {
		return COSConfig{}, err
	}
	return item, nil
}

func (s Store) UpsertCOSConfig(ctx context.Context, actor account.User, profileID int64, req COSConfigUpsert) (COSConfig, error) {
	if err := s.ensureCanEditModule(ctx, actor, profileID, "cos"); err != nil {
		return COSConfig{}, err
	}
	if !req.Enabled {
		if _, err := s.db.ExecContext(ctx, `
			UPDATE cos_storage_profiles
			SET enabled = 0, updated_at = CURRENT_TIMESTAMP
			WHERE recording_profile_id = ?
		`, profileID); err != nil {
			return COSConfig{}, fmt.Errorf("disable cos config: %w", err)
		}
		return s.GetCOSConfig(ctx, actor, profileID)
	}
	region := strings.TrimSpace(req.Region)
	bucket := strings.TrimSpace(req.Bucket)
	prefix := normalizePrefix(req.Prefix, profileID)
	if req.CredentialID <= 0 || region == "" || bucket == "" || req.MaxManagedBytes <= 0 {
		return COSConfig{}, ErrValidation
	}
	if err := s.ensureCredentialVisible(ctx, actor, req.CredentialID, "tencent_cos", "STORAGE"); err != nil {
		return COSConfig{}, err
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO cos_storage_profiles
			(recording_profile_id, credential_id, enabled, region, bucket, prefix, max_managed_bytes)
		VALUES (?, ?, 1, ?, ?, ?, ?)
		ON CONFLICT(recording_profile_id) DO UPDATE SET
			credential_id = excluded.credential_id,
			enabled = excluded.enabled,
			region = excluded.region,
			bucket = excluded.bucket,
			prefix = excluded.prefix,
			max_managed_bytes = excluded.max_managed_bytes,
			updated_at = CURRENT_TIMESTAMP
	`, profileID, req.CredentialID, region, bucket, prefix, req.MaxManagedBytes)
	if err != nil {
		return COSConfig{}, fmt.Errorf("upsert cos config: %w", err)
	}
	return s.GetCOSConfig(ctx, actor, profileID)
}

func (s Store) Reconcile(ctx context.Context, actor account.User) (ReconcileResult, error) {
	if actor.Role != account.RoleSuperAdmin {
		return ReconcileResult{}, ErrForbidden
	}
	var result ReconcileResult
	created, err := s.createBilibiliPublications(ctx)
	if err != nil {
		return ReconcileResult{}, err
	}
	result.PublicationsCreated = created
	jobs, err := s.createBilibiliJobs(ctx)
	if err != nil {
		return ReconcileResult{}, err
	}
	result.BilibiliJobsCreated = jobs
	objects, err := s.createCOSObjects(ctx)
	if err != nil {
		return ReconcileResult{}, err
	}
	result.COSObjectsCreated = objects
	cosJobs, err := s.createCOSJobs(ctx)
	if err != nil {
		return ReconcileResult{}, err
	}
	result.COSJobsCreated = cosJobs
	fileObjects, err := s.createCOSRecordingFileObjects(ctx)
	if err != nil {
		return ReconcileResult{}, err
	}
	result.COSFileObjectsCreated = fileObjects
	fileJobs, err := s.createCOSRecordingFileJobs(ctx)
	if err != nil {
		return ReconcileResult{}, err
	}
	result.COSFileJobsCreated = fileJobs
	return result, nil
}

func (s Store) createBilibiliPublications(ctx context.Context) (int, error) {
	query := `
		INSERT OR IGNORE INTO publications
			(recording_profile_id, upload_source_id, platform, credential_id, status, request_snapshot_json)
		SELECT us.recording_profile_id,
			us.id,
			'bilibili',
			pp.credential_id,
			'PENDING',
			us.metadata_json
		FROM upload_sources us
		JOIN publishing_profiles pp ON pp.recording_profile_id = us.recording_profile_id
			AND pp.platform = 'bilibili'
			AND pp.enabled = 1
			AND pp.credential_id IS NOT NULL
		WHERE us.status = 'READY_TO_UPLOAD'
			AND ` + uploadSourceStableCondition + `
			AND ` + uploadSourceReviewReadyCondition + `
			AND EXISTS (
				SELECT 1 FROM upload_source_outputs uso
				WHERE uso.upload_source_id = us.id AND uso.status = 'READY_TO_UPLOAD'
			)
	`
	result, err := s.db.ExecContext(ctx, query)
	if err != nil {
		return 0, fmt.Errorf("create bilibili publications: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("read bilibili publication count: %w", err)
	}
	return int(changed), nil
}

func (s Store) createBilibiliJobs(ctx context.Context) (int, error) {
	query := `
		INSERT OR IGNORE INTO jobs
			(recording_profile_id, upload_source_id, publication_id, type, resource_class, business_key, payload_json, status, priority, max_attempts)
		SELECT p.recording_profile_id,
			p.upload_source_id,
			p.id,
			'UPLOAD_BILIBILI',
			'NETWORK',
			'upload-source:' || p.upload_source_id || ':bilibili:upload',
			'{"publication_id":' || p.id || ',"upload_source_id":' || p.upload_source_id || '}',
			'PENDING',
			80,
			3
		FROM publications p
		JOIN upload_sources us ON us.id = p.upload_source_id
		WHERE p.platform = 'bilibili'
			AND p.status = 'PENDING'
			AND us.status = 'READY_TO_UPLOAD'
			AND ` + uploadSourceStableCondition + `
			AND ` + uploadSourceReviewReadyCondition + `
			AND EXISTS (
				SELECT 1 FROM upload_source_outputs uso
				WHERE uso.upload_source_id = us.id AND uso.status = 'READY_TO_UPLOAD'
			)
			AND p.upload_source_id IS NOT NULL
	`
	result, err := s.db.ExecContext(ctx, query)
	if err != nil {
		return 0, fmt.Errorf("create bilibili jobs: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("read bilibili job count: %w", err)
	}
	reset, err := s.resetPendingBilibiliJobs(ctx)
	if err != nil {
		return 0, err
	}
	return int(changed) + reset, nil
}

func (s Store) resetPendingBilibiliJobs(ctx context.Context) (int, error) {
	result, err := s.db.ExecContext(ctx, `
		UPDATE jobs
		SET status = 'PENDING',
			attempts = 0,
			run_after = CURRENT_TIMESTAMP,
			locked_at = NULL,
			heartbeat_at = NULL,
			locked_by = NULL,
			last_error_class = NULL,
			last_error = NULL,
			updated_at = CURRENT_TIMESTAMP
		WHERE type = 'UPLOAD_BILIBILI'
			AND status IN ('FAILED', 'CANCELLED')
			AND publication_id IN (
				SELECT p.id
				FROM publications p
				JOIN upload_sources us ON us.id = p.upload_source_id
				WHERE p.platform = 'bilibili'
					AND p.status = 'PENDING'
					AND us.status = 'READY_TO_UPLOAD'
					AND `+uploadSourceStableCondition+`
					AND `+uploadSourceReviewReadyCondition+`
					AND EXISTS (
						SELECT 1 FROM upload_source_outputs uso
						WHERE uso.upload_source_id = us.id
							AND uso.status = 'READY_TO_UPLOAD'
					)
			)
	`)
	if err != nil {
		return 0, fmt.Errorf("reset pending bilibili jobs: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("read reset bilibili job count: %w", err)
	}
	return int(changed), nil
}

func (s Store) createCOSObjects(ctx context.Context) (int, error) {
	compressionStatus := "DISABLED"
	compressionPreset := ""
	if s.cfg.COSCompressionEnabled {
		compressionStatus = "PENDING"
		compressionPreset = configuredCOSCompressionPreset(s.cfg.COSCompressionPreset)
	}
	query := `
		INSERT OR IGNORE INTO upload_source_cos_objects
			(cos_storage_profile_id, recording_profile_id, upload_source_id, upload_source_output_id, object_key,
				size_bytes, source_size_bytes, compression_status, compression_preset, status)
		SELECT csp.id,
			us.recording_profile_id,
			us.id,
			uso.id,
			csp.prefix || uso.relative_path,
			uso.size_bytes,
			uso.size_bytes,
			?,
			NULLIF(?, ''),
			'PENDING'
		FROM upload_sources us
		JOIN upload_source_outputs uso ON uso.upload_source_id = us.id
			AND uso.status = 'READY_TO_UPLOAD'
		JOIN cos_storage_profiles csp ON csp.recording_profile_id = us.recording_profile_id
			AND csp.enabled = 1
		WHERE us.status = 'READY_TO_UPLOAD'
			AND ` + uploadSourceStableCondition + `
			AND ` + uploadSourceReviewReadyCondition + `
	`
	result, err := s.db.ExecContext(ctx, query, compressionStatus, compressionPreset)
	if err != nil {
		return 0, fmt.Errorf("create cos objects: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("read cos object count: %w", err)
	}
	return int(changed), nil
}

func (s Store) createCOSJobs(ctx context.Context) (int, error) {
	query := `
		INSERT OR IGNORE INTO jobs
			(recording_profile_id, upload_source_id, type, resource_class, business_key, payload_json, status, priority, max_attempts)
		SELECT co.recording_profile_id,
			co.upload_source_id,
			'UPLOAD_COS_OBJECT',
			'NETWORK',
			'upload-source:' || co.upload_source_id || ':output:' || co.upload_source_output_id || ':cos:' || co.cos_storage_profile_id,
			'{"cos_object_id":' || co.id || ',"upload_source_id":' || co.upload_source_id || ',"output_id":' || co.upload_source_output_id || '}',
			'PENDING',
			90,
			5
		FROM upload_source_cos_objects co
		JOIN upload_sources us ON us.id = co.upload_source_id
		WHERE co.status = 'PENDING'
			AND co.upload_source_output_id IS NOT NULL
			AND us.status = 'READY_TO_UPLOAD'
			AND ` + uploadSourceStableCondition + `
			AND ` + uploadSourceReviewReadyCondition + `
	`
	result, err := s.db.ExecContext(ctx, query)
	if err != nil {
		return 0, fmt.Errorf("create cos jobs: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("read cos job count: %w", err)
	}
	reset, err := s.resetPendingCOSJobs(ctx)
	if err != nil {
		return 0, err
	}
	return int(changed) + reset, nil
}

func (s Store) resetPendingCOSJobs(ctx context.Context) (int, error) {
	result, err := s.db.ExecContext(ctx, `
		UPDATE jobs
		SET status = 'PENDING',
			attempts = 0,
			run_after = CURRENT_TIMESTAMP,
			locked_at = NULL,
			heartbeat_at = NULL,
			locked_by = NULL,
			last_error_class = NULL,
			last_error = NULL,
			updated_at = CURRENT_TIMESTAMP
		WHERE type = 'UPLOAD_COS_OBJECT'
			AND status IN ('FAILED', 'CANCELLED')
			AND EXISTS (
				SELECT 1
				FROM upload_source_cos_objects co
				JOIN upload_sources us ON us.id = co.upload_source_id
				JOIN upload_source_outputs uso ON uso.id = co.upload_source_output_id
					AND uso.status = 'READY_TO_UPLOAD'
				WHERE co.id = json_extract(jobs.payload_json, '$.cos_object_id')
					AND co.status = 'PENDING'
					AND us.status = 'READY_TO_UPLOAD'
					AND `+uploadSourceStableCondition+`
					AND `+uploadSourceReviewReadyCondition+`
			)
	`)
	if err != nil {
		return 0, fmt.Errorf("reset pending cos jobs: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("read reset cos job count: %w", err)
	}
	return int(changed), nil
}

func (s Store) createCOSRecordingFileObjects(ctx context.Context) (int, error) {
	result, err := s.db.ExecContext(ctx, `
		INSERT OR IGNORE INTO cos_objects
			(cos_storage_profile_id, recording_profile_id, recording_id, recording_file_id, object_key, size_bytes, status)
		SELECT csp.id,
			rec.recording_profile_id,
			rf.recording_id,
			rf.id,
			csp.prefix || 'raw/' || rf.relative_path,
			rf.size_bytes,
			'PENDING'
		FROM recording_files rf
		JOIN recordings rec ON rec.id = rf.recording_id
			AND rec.local_storage_status <> 'DELETED'
		JOIN cos_storage_profiles csp ON csp.recording_profile_id = rec.recording_profile_id
			AND csp.enabled = 1
		WHERE rf.kind = 'danmaku'
			AND rf.file_status = 'CLOSED'
			AND rf.deleted_at IS NULL
	`)
	if err != nil {
		return 0, fmt.Errorf("create cos recording file objects: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("read cos recording file object count: %w", err)
	}
	return int(changed), nil
}

func (s Store) createCOSRecordingFileJobs(ctx context.Context) (int, error) {
	result, err := s.db.ExecContext(ctx, `
		INSERT OR IGNORE INTO jobs
			(recording_profile_id, recording_id, recording_file_id, cos_object_id, type, resource_class, business_key, payload_json, status, priority, max_attempts)
		SELECT co.recording_profile_id,
			co.recording_id,
			co.recording_file_id,
			co.id,
			'UPLOAD_COS_RECORDING_FILE',
			'NETWORK',
			'recording-file:' || co.recording_file_id || ':cos:' || co.cos_storage_profile_id,
			'{"cos_object_id":' || co.id || ',"recording_file_id":' || co.recording_file_id || '}',
			'PENDING',
			95,
			5
		FROM cos_objects co
		JOIN recording_files rf ON rf.id = co.recording_file_id
		JOIN cos_storage_profiles csp ON csp.id = co.cos_storage_profile_id
			AND csp.enabled = 1
		WHERE co.status = 'PENDING'
			AND co.deleted_at IS NULL
			AND rf.kind = 'danmaku'
			AND rf.file_status = 'CLOSED'
			AND rf.deleted_at IS NULL
	`)
	if err != nil {
		return 0, fmt.Errorf("create cos recording file jobs: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("read cos recording file job count: %w", err)
	}
	return int(changed), nil
}

func (s Store) COSUploadRequest(ctx context.Context, payload COSJobPayload) (COSUploadRequest, error) {
	if payload.COSObjectID <= 0 {
		return COSUploadRequest{}, ErrValidation
	}
	var request COSUploadRequest
	var encryptedSecret []byte
	var sourceRelativePath string
	err := s.db.QueryRowContext(ctx, `
		SELECT co.id,
			co.upload_source_id,
			co.upload_source_output_id,
			co.recording_profile_id,
			csp.region,
			csp.bucket,
			csp.prefix,
			co.object_key,
			uso.relative_path,
			uso.size_bytes,
			c.encrypted_secret
		FROM upload_source_cos_objects co
		JOIN upload_sources us ON us.id = co.upload_source_id
		JOIN upload_source_outputs uso ON uso.id = co.upload_source_output_id
			AND uso.status = 'READY_TO_UPLOAD'
		JOIN cos_storage_profiles csp ON csp.id = co.cos_storage_profile_id
		JOIN credentials c ON c.id = csp.credential_id
		WHERE co.id = ?
			AND co.status IN ('PENDING', 'FAILED')
			AND us.status = 'READY_TO_UPLOAD'
			AND COALESCE(us.review_status, 'NONE') != 'REQUIRED'
			AND COALESCE(us.edit_decision_json, '') = ''
			AND COALESCE(us.local_cleanup_status, 'AVAILABLE') = 'AVAILABLE'
			AND csp.enabled = 1
	`, payload.COSObjectID).Scan(
		&request.ObjectID,
		&request.UploadSourceID,
		&request.OutputID,
		&request.RecordingProfileID,
		&request.Region,
		&request.Bucket,
		&request.Prefix,
		&request.ObjectKey,
		&sourceRelativePath,
		&request.SourceSizeBytes,
		&encryptedSecret,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return COSUploadRequest{}, ErrNotFound
	}
	if err != nil {
		return COSUploadRequest{}, fmt.Errorf("load cos upload request: %w", err)
	}
	if payload.UploadSourceID > 0 && payload.UploadSourceID != request.UploadSourceID {
		return COSUploadRequest{}, ErrValidation
	}
	if payload.OutputID > 0 {
		var outputID int64
		if err := s.db.QueryRowContext(ctx, `SELECT upload_source_output_id FROM upload_source_cos_objects WHERE id = ?`, payload.COSObjectID).Scan(&outputID); err != nil {
			return COSUploadRequest{}, fmt.Errorf("load cos output id: %w", err)
		}
		if outputID != payload.OutputID {
			return COSUploadRequest{}, ErrValidation
		}
	}
	sourcePath, err := resolveWithinRoot(s.cfg.DataRoot, sourceRelativePath)
	if err != nil {
		return COSUploadRequest{}, fmt.Errorf("resolve cos upload source: %w", err)
	}
	info, err := os.Stat(sourcePath)
	if errors.Is(err, os.ErrNotExist) {
		return COSUploadRequest{}, NewClassifiedError("SOURCE_MISSING", "cos upload source file is missing")
	}
	if err != nil {
		return COSUploadRequest{}, fmt.Errorf("stat cos upload source: %w", err)
	}
	if info.IsDir() {
		return COSUploadRequest{}, NewClassifiedError("SOURCE_MISSING", "cos upload source path is a directory")
	}
	request.SourcePath = sourcePath
	request.SourceRelativePath = sourceRelativePath
	request.Secret, err = s.decryptCOSSecret(encryptedSecret)
	if err != nil {
		return COSUploadRequest{}, err
	}
	return request, nil
}

func (s Store) COSRecordingFileUploadRequest(ctx context.Context, payload COSRecordingFileJobPayload) (COSUploadRequest, error) {
	if payload.COSObjectID <= 0 || payload.RecordingFileID <= 0 {
		return COSUploadRequest{}, ErrValidation
	}
	var request COSUploadRequest
	var encryptedSecret []byte
	var sourceRelativePath string
	err := s.db.QueryRowContext(ctx, `
		SELECT co.id,
			co.recording_profile_id,
			csp.region,
			csp.bucket,
			csp.prefix,
			co.object_key,
			rf.relative_path,
			rf.size_bytes,
			c.encrypted_secret
		FROM cos_objects co
		JOIN recording_files rf ON rf.id = co.recording_file_id
			AND rf.kind = 'danmaku'
			AND rf.file_status = 'CLOSED'
			AND rf.deleted_at IS NULL
		JOIN recordings rec ON rec.id = co.recording_id
			AND rec.local_storage_status <> 'DELETED'
		JOIN cos_storage_profiles csp ON csp.id = co.cos_storage_profile_id
		JOIN credentials c ON c.id = csp.credential_id
		WHERE co.id = ?
			AND co.recording_file_id = ?
			AND co.status IN ('PENDING', 'FAILED')
			AND co.deleted_at IS NULL
			AND csp.enabled = 1
	`, payload.COSObjectID, payload.RecordingFileID).Scan(
		&request.ObjectID,
		&request.RecordingProfileID,
		&request.Region,
		&request.Bucket,
		&request.Prefix,
		&request.ObjectKey,
		&sourceRelativePath,
		&request.SourceSizeBytes,
		&encryptedSecret,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return COSUploadRequest{}, ErrNotFound
	}
	if err != nil {
		return COSUploadRequest{}, fmt.Errorf("load cos recording file upload request: %w", err)
	}
	sourcePath, err := resolveWithinRoot(s.cfg.DataRoot, sourceRelativePath)
	if err != nil {
		return COSUploadRequest{}, fmt.Errorf("resolve cos recording file source: %w", err)
	}
	info, err := os.Stat(sourcePath)
	if errors.Is(err, os.ErrNotExist) {
		return COSUploadRequest{}, NewClassifiedError("SOURCE_MISSING", "cos recording file source is missing")
	}
	if err != nil {
		return COSUploadRequest{}, fmt.Errorf("stat cos recording file source: %w", err)
	}
	if info.IsDir() {
		return COSUploadRequest{}, NewClassifiedError("SOURCE_MISSING", "cos recording file source path is a directory")
	}
	request.SourcePath = sourcePath
	request.SourceRelativePath = sourceRelativePath
	request.Secret, err = s.decryptCOSSecret(encryptedSecret)
	if err != nil {
		return COSUploadRequest{}, err
	}
	return request, nil
}

func (s Store) MarkCOSObjectUploading(ctx context.Context, objectID int64) error {
	if objectID <= 0 {
		return ErrValidation
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE upload_source_cos_objects
		SET status = 'UPLOADING',
			last_error = NULL,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, objectID)
	if err != nil {
		return fmt.Errorf("mark cos object uploading: %w", err)
	}
	return nil
}

func (s Store) MarkCOSObjectCompressing(ctx context.Context, objectID int64) error {
	if objectID <= 0 {
		return ErrValidation
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE upload_source_cos_objects
		SET compression_status = 'COMPRESSING',
			last_error = NULL,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, objectID)
	if err != nil {
		return fmt.Errorf("mark cos object compressing: %w", err)
	}
	return nil
}

func (s Store) MarkCOSObjectCompressed(ctx context.Context, objectID int64, compressedRelativePath string, compressedSizeBytes int64, preset string, objectKey string) error {
	if objectID <= 0 || compressedRelativePath == "" || compressedSizeBytes <= 0 || objectKey == "" {
		return ErrValidation
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE upload_source_cos_objects
		SET compression_status = 'COMPRESSED',
			compression_preset = NULLIF(?, ''),
			compressed_from_relative_path = ?,
			object_key = ?,
			size_bytes = ?,
			last_error = NULL,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, preset, compressedRelativePath, objectKey, compressedSizeBytes, objectID)
	if err != nil {
		return fmt.Errorf("mark cos object compressed: %w", err)
	}
	return nil
}

func (s Store) MarkCOSObjectCompressionFailed(ctx context.Context, objectID int64, message string) error {
	if objectID <= 0 {
		return ErrValidation
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE upload_source_cos_objects
		SET compression_status = 'FAILED',
			status = 'FAILED',
			last_error = ?,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, message, objectID)
	if err != nil {
		return fmt.Errorf("mark cos object compression failed: %w", err)
	}
	return nil
}

func (s Store) MarkCOSObjectUploaded(ctx context.Context, objectID int64, result COSUploadResult) error {
	if objectID <= 0 {
		return ErrValidation
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE upload_source_cos_objects
		SET status = 'AVAILABLE',
			etag = NULLIF(?, ''),
			size_bytes = CASE WHEN ? > 0 THEN ? ELSE size_bytes END,
			last_error = NULL,
			uploaded_at = CURRENT_TIMESTAMP,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, result.ETag, result.SizeBytes, result.SizeBytes, objectID)
	if err != nil {
		return fmt.Errorf("mark cos object uploaded: %w", err)
	}
	return nil
}

func (s Store) MarkCOSObjectUploadFailed(ctx context.Context, objectID int64, errorClass string, message string) error {
	if objectID <= 0 {
		return ErrValidation
	}
	status := "FAILED"
	if errorClass == "SOURCE_MISSING" {
		status = "SOURCE_MISSING"
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE upload_source_cos_objects
		SET status = ?,
			last_error = ?,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, status, message, objectID)
	if err != nil {
		return fmt.Errorf("mark cos object failed: %w", err)
	}
	return nil
}

func (s Store) MarkCOSRecordingFileUploading(ctx context.Context, objectID int64) error {
	if objectID <= 0 {
		return ErrValidation
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE cos_objects
		SET status = 'UPLOADING',
			last_error = NULL,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, objectID)
	if err != nil {
		return fmt.Errorf("mark cos recording file uploading: %w", err)
	}
	return nil
}

func (s Store) MarkCOSRecordingFileUploaded(ctx context.Context, objectID int64, result COSUploadResult) error {
	if objectID <= 0 {
		return ErrValidation
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE cos_objects
		SET status = 'AVAILABLE',
			etag = NULLIF(?, ''),
			size_bytes = CASE WHEN ? > 0 THEN ? ELSE size_bytes END,
			last_error = NULL,
			uploaded_at = CURRENT_TIMESTAMP,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, result.ETag, result.SizeBytes, result.SizeBytes, objectID)
	if err != nil {
		return fmt.Errorf("mark cos recording file uploaded: %w", err)
	}
	return nil
}

func (s Store) MarkCOSRecordingFileUploadFailed(ctx context.Context, objectID int64, errorClass string, message string) error {
	if objectID <= 0 {
		return ErrValidation
	}
	status := "FAILED"
	if errorClass == "SOURCE_MISSING" {
		status = "SOURCE_MISSING"
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE cos_objects
		SET status = ?,
			last_error = ?,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, status, message, objectID)
	if err != nil {
		return fmt.Errorf("mark cos recording file failed: %w", err)
	}
	return nil
}

func (s Store) COSDownloadURLRequest(ctx context.Context, actor account.User, uploadSourceID int64, outputID int64) (COSDownloadURLRequest, error) {
	if uploadSourceID <= 0 || outputID <= 0 {
		return COSDownloadURLRequest{}, ErrValidation
	}

	var request COSDownloadURLRequest
	var encryptedSecret []byte
	var profileOwnerID int64
	var sourceStatus string
	var outputStatus string
	var objectStatus string
	err := s.db.QueryRowContext(ctx, `
		SELECT co.id,
			co.upload_source_id,
			co.upload_source_output_id,
			co.recording_profile_id,
			csp.region,
			csp.bucket,
			co.object_key,
			c.encrypted_secret,
			p.owner_user_id,
			us.status,
			uso.status,
			co.status
		FROM upload_source_cos_objects co
		JOIN upload_sources us ON us.id = co.upload_source_id
		JOIN recording_profiles p ON p.id = co.recording_profile_id
		JOIN upload_source_outputs uso ON uso.id = co.upload_source_output_id
		JOIN cos_storage_profiles csp ON csp.id = co.cos_storage_profile_id
		JOIN credentials c ON c.id = csp.credential_id
		WHERE co.upload_source_id = ?
			AND co.upload_source_output_id = ?
			AND csp.enabled = 1
	`, uploadSourceID, outputID).Scan(
		&request.ObjectID,
		&request.UploadSourceID,
		&request.UploadSourceOutputID,
		&request.RecordingProfileID,
		&request.Region,
		&request.Bucket,
		&request.ObjectKey,
		&encryptedSecret,
		&profileOwnerID,
		&sourceStatus,
		&outputStatus,
		&objectStatus,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return COSDownloadURLRequest{}, ErrNotFound
	}
	if err != nil {
		return COSDownloadURLRequest{}, fmt.Errorf("load cos download request: %w", err)
	}
	if err := s.ensureCanDownloadUploadSourceOutput(ctx, actor, profileOwnerID); err != nil {
		return COSDownloadURLRequest{}, err
	}
	if sourceStatus != "READY_TO_UPLOAD" || outputStatus != "READY_TO_UPLOAD" || objectStatus != "AVAILABLE" {
		return COSDownloadURLRequest{}, ErrNotReady
	}
	secret, err := s.decryptCOSSecret(encryptedSecret)
	if err != nil {
		return COSDownloadURLRequest{}, err
	}
	request.Secret = secret
	return request, nil
}

func (s Store) COSRecordingFileDownloadURLRequest(ctx context.Context, actor account.User, fileID int64) (COSDownloadURLRequest, error) {
	if fileID <= 0 {
		return COSDownloadURLRequest{}, ErrValidation
	}

	var request COSDownloadURLRequest
	var encryptedSecret []byte
	var profileOwnerID int64
	var fileStatus string
	var objectStatus string
	err := s.db.QueryRowContext(ctx, `
		SELECT co.id,
			co.recording_profile_id,
			csp.region,
			csp.bucket,
			co.object_key,
			c.encrypted_secret,
			p.owner_user_id,
			rf.file_status,
			co.status
		FROM cos_objects co
		JOIN recording_files rf ON rf.id = co.recording_file_id
			AND rf.kind = 'danmaku'
			AND rf.deleted_at IS NULL
		JOIN recordings rec ON rec.id = co.recording_id
		JOIN recording_profiles p ON p.id = co.recording_profile_id
		JOIN cos_storage_profiles csp ON csp.id = co.cos_storage_profile_id
		JOIN credentials c ON c.id = csp.credential_id
		WHERE co.recording_file_id = ?
			AND co.deleted_at IS NULL
			AND csp.enabled = 1
		ORDER BY co.updated_at DESC, co.id DESC
		LIMIT 1
	`, fileID).Scan(
		&request.ObjectID,
		&request.RecordingProfileID,
		&request.Region,
		&request.Bucket,
		&request.ObjectKey,
		&encryptedSecret,
		&profileOwnerID,
		&fileStatus,
		&objectStatus,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return COSDownloadURLRequest{}, ErrNotFound
	}
	if err != nil {
		return COSDownloadURLRequest{}, fmt.Errorf("load cos recording file download request: %w", err)
	}
	if err := s.ensureCanDownloadUploadSourceOutput(ctx, actor, profileOwnerID); err != nil {
		return COSDownloadURLRequest{}, err
	}
	if fileStatus != "CLOSED" || objectStatus != "AVAILABLE" {
		return COSDownloadURLRequest{}, ErrNotReady
	}
	secret, err := s.decryptCOSSecret(encryptedSecret)
	if err != nil {
		return COSDownloadURLRequest{}, err
	}
	request.Secret = secret
	return request, nil
}

func (s Store) getCredential(ctx context.Context, actor account.User, id int64) (Credential, error) {
	items, err := s.ListCredentials(ctx, actor)
	if err != nil {
		return Credential{}, err
	}
	for _, item := range items {
		if item.ID == id {
			return item, nil
		}
	}
	return Credential{}, ErrNotFound
}

func (s Store) getPublishingConfig(ctx context.Context, profileID int64, platform string) (PublishingConfig, error) {
	var item PublishingConfig
	var enabled int
	var settings string
	err := s.db.QueryRowContext(ctx, `
		SELECT id, recording_profile_id, platform, COALESCE(credential_id, 0), enabled,
			COALESCE(settings_json, '{}'), created_at, updated_at
		FROM publishing_profiles
		WHERE recording_profile_id = ? AND platform = ?
	`, profileID, platform).Scan(&item.ID, &item.RecordingProfileID, &item.Platform, &item.CredentialID, &enabled, &settings, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		return PublishingConfig{}, err
	}
	item.Enabled = enabled == 1
	item.Settings = json.RawMessage(settings)
	return item, nil
}

func (s Store) getCOSConfig(ctx context.Context, profileID int64) (COSConfig, error) {
	var item COSConfig
	var enabled int
	err := s.db.QueryRowContext(ctx, `
		SELECT id, recording_profile_id, credential_id, enabled, region, bucket, prefix,
			max_managed_bytes, created_at, updated_at
		FROM cos_storage_profiles
		WHERE recording_profile_id = ?
	`, profileID).Scan(&item.ID, &item.RecordingProfileID, &item.CredentialID, &enabled, &item.Region, &item.Bucket, &item.Prefix, &item.MaxManagedBytes, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		return COSConfig{}, err
	}
	item.Enabled = enabled == 1
	return item, nil
}

func (s Store) ensureProfileVisible(ctx context.Context, actor account.User, profileID int64) error {
	if profileID <= 0 {
		return ErrValidation
	}
	var ownerID int64
	err := s.db.QueryRowContext(ctx, `
		SELECT owner_user_id FROM recording_profiles WHERE id = ?
	`, profileID).Scan(&ownerID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("load profile owner: %w", err)
	}
	if actor.Role != account.RoleSuperAdmin && ownerID != actor.ID {
		return ErrForbidden
	}
	return nil
}

func (s Store) ensureCanEditModule(ctx context.Context, actor account.User, profileID int64, module string) error {
	if err := s.ensureProfileVisible(ctx, actor, profileID); err != nil {
		return err
	}
	if actor.Role == account.RoleSuperAdmin {
		return nil
	}
	policy, err := account.NewStore(s.db).Policy(ctx, actor, actor.ID)
	if err != nil {
		return err
	}
	switch module {
	case "bilibili":
		if !policy.CanEditBilibiliModule {
			return ErrForbidden
		}
	case "cos":
		if !policy.CanEditCosModule {
			return ErrForbidden
		}
	default:
		return ErrValidation
	}
	return nil
}

func (s Store) ensureCanDownloadUploadSourceOutput(ctx context.Context, actor account.User, profileOwnerID int64) error {
	if actor.Role == account.RoleSuperAdmin {
		return nil
	}
	if profileOwnerID != actor.ID {
		return ErrForbidden
	}
	policy, err := account.NewStore(s.db).Policy(ctx, actor, actor.ID)
	if err != nil {
		return err
	}
	// Download quotas and time-window policies should attach here so every client
	// receives the same COS signed URL decision.
	if !policy.CanManageLocalFiles {
		return ErrForbidden
	}
	return nil
}

func (s Store) ensureCredentialVisible(ctx context.Context, actor account.User, credentialID int64, platform string, purpose string) error {
	var ownerID int64
	var scope string
	var storedPlatform string
	var storedPurpose string
	err := s.db.QueryRowContext(ctx, `
		SELECT COALESCE(owner_user_id, 0), scope, platform, purpose
		FROM credentials
		WHERE id = ?
	`, credentialID).Scan(&ownerID, &scope, &storedPlatform, &storedPurpose)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrValidation
	}
	if err != nil {
		return fmt.Errorf("load credential: %w", err)
	}
	if storedPlatform != platform || storedPurpose != purpose {
		return ErrValidation
	}
	if actor.Role != account.RoleSuperAdmin && (scope != "USER" || ownerID != actor.ID) {
		return ErrForbidden
	}
	return nil
}

func (s Store) encryptSecret(secret []byte) ([]byte, error) {
	master, err := os.ReadFile(s.cfg.MasterKeyPath)
	if err != nil {
		return nil, fmt.Errorf("read master key: %w", err)
	}
	key := sha256.Sum256([]byte(strings.TrimSpace(string(master))))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create gcm: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("read nonce: %w", err)
	}
	ciphertext := gcm.Seal(nil, nonce, secret, nil)
	envelope := map[string]string{
		"alg":        "AES-256-GCM",
		"nonce":      base64.StdEncoding.EncodeToString(nonce),
		"ciphertext": base64.StdEncoding.EncodeToString(ciphertext),
	}
	encoded, err := json.Marshal(envelope)
	if err != nil {
		return nil, fmt.Errorf("encode encrypted secret: %w", err)
	}
	return encoded, nil
}

func (s Store) decryptSecret(encrypted []byte) ([]byte, error) {
	var envelope struct {
		Alg        string `json:"alg"`
		Nonce      string `json:"nonce"`
		Ciphertext string `json:"ciphertext"`
	}
	if err := json.Unmarshal(encrypted, &envelope); err != nil {
		return nil, fmt.Errorf("decode encrypted secret envelope: %w", err)
	}
	if envelope.Alg != "AES-256-GCM" || envelope.Nonce == "" || envelope.Ciphertext == "" {
		return nil, ErrValidation
	}
	nonce, err := base64.StdEncoding.DecodeString(envelope.Nonce)
	if err != nil {
		return nil, fmt.Errorf("decode encrypted secret nonce: %w", err)
	}
	ciphertext, err := base64.StdEncoding.DecodeString(envelope.Ciphertext)
	if err != nil {
		return nil, fmt.Errorf("decode encrypted secret ciphertext: %w", err)
	}
	master, err := os.ReadFile(s.cfg.MasterKeyPath)
	if err != nil {
		return nil, fmt.Errorf("read master key: %w", err)
	}
	key := sha256.Sum256([]byte(strings.TrimSpace(string(master))))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create gcm: %w", err)
	}
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt secret: %w", err)
	}
	return plaintext, nil
}

func (s Store) decryptCOSSecret(encrypted []byte) (COSSecret, error) {
	plaintext, err := s.decryptSecret(encrypted)
	if err != nil {
		return COSSecret{}, err
	}
	var flexible map[string]string
	if err := json.Unmarshal(plaintext, &flexible); err != nil {
		return COSSecret{}, ErrValidation
	}
	secret := COSSecret{
		SecretID:     firstNonEmpty(flexible["secret_id"], flexible["secretId"], flexible["SecretId"], flexible["SecretID"]),
		SecretKey:    firstNonEmpty(flexible["secret_key"], flexible["secretKey"], flexible["SecretKey"], flexible["SecretKEY"]),
		SessionToken: firstNonEmpty(flexible["session_token"], flexible["sessionToken"], flexible["Token"]),
	}
	if secret.SecretID == "" || secret.SecretKey == "" {
		return COSSecret{}, NewClassifiedError("AUTH", "cos credential must include secret_id and secret_key")
	}
	return secret, nil
}

func normalizeJSON(raw json.RawMessage) string {
	value := strings.TrimSpace(string(raw))
	if value == "" || value == "null" {
		return "{}"
	}
	return value
}

func normalizePrefix(value string, profileID int64) string {
	prefix := strings.TrimSpace(filepath.ToSlash(value))
	prefix = strings.TrimLeft(prefix, "/")
	if prefix == "" {
		prefix = fmt.Sprintf("7grecorder/%d/", profileID)
	}
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	return prefix
}

func resolveWithinRoot(root string, relativePath string) (string, error) {
	if relativePath == "" || filepath.IsAbs(relativePath) {
		return "", ErrValidation
	}
	cleaned := filepath.Clean(relativePath)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(os.PathSeparator)) {
		return "", ErrValidation
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	candidate := filepath.Join(rootAbs, cleaned)
	candidateAbs, err := filepath.Abs(candidate)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(rootAbs, candidateAbs)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", ErrValidation
	}
	return candidateAbs, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func configuredCOSCompressionPreset(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return COSCompressionPresetH264CRF23MediumMP4
	}
	return value
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
