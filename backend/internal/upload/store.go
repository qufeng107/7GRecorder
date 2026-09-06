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

type COSConfigUpsert struct {
	CredentialID    int64  `json:"credential_id"`
	Enabled         bool   `json:"enabled"`
	Region          string `json:"region"`
	Bucket          string `json:"bucket"`
	Prefix          string `json:"prefix"`
	MaxManagedBytes int64  `json:"max_managed_bytes"`
}

type ReconcileResult struct {
	PublicationsCreated int `json:"publications_created"`
	BilibiliJobsCreated int `json:"bilibili_jobs_created"`
	COSObjectsCreated   int `json:"cos_objects_created"`
	COSJobsCreated      int `json:"cos_jobs_created"`
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
	if req.Enabled && req.CredentialID <= 0 {
		return PublishingConfig{}, ErrValidation
	}
	if req.CredentialID > 0 {
		if err := s.ensureCredentialVisible(ctx, actor, req.CredentialID, "bilibili", "PUBLISHER"); err != nil {
			return PublishingConfig{}, err
		}
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO publishing_profiles
			(recording_profile_id, platform, credential_id, enabled, settings_json)
		VALUES (?, 'bilibili', NULLIF(?, 0), ?, ?)
		ON CONFLICT(recording_profile_id, platform) DO UPDATE SET
			credential_id = NULLIF(excluded.credential_id, 0),
			enabled = excluded.enabled,
			settings_json = excluded.settings_json,
			updated_at = CURRENT_TIMESTAMP
	`, profileID, req.CredentialID, boolInt(req.Enabled), settings)
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
	return result, nil
}

func (s Store) createBilibiliPublications(ctx context.Context) (int, error) {
	result, err := s.db.ExecContext(ctx, `
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
			AND COALESCE(us.output_relative_path, '') != ''
	`)
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
	result, err := s.db.ExecContext(ctx, `
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
			AND p.upload_source_id IS NOT NULL
	`)
	if err != nil {
		return 0, fmt.Errorf("create bilibili jobs: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("read bilibili job count: %w", err)
	}
	return int(changed), nil
}

func (s Store) createCOSObjects(ctx context.Context) (int, error) {
	result, err := s.db.ExecContext(ctx, `
		INSERT OR IGNORE INTO upload_source_cos_objects
			(cos_storage_profile_id, recording_profile_id, upload_source_id, object_key, size_bytes, status)
		SELECT csp.id,
			us.recording_profile_id,
			us.id,
			csp.prefix || 'upload-sources/' || us.id || '/upload-source-' || us.id || '.flv',
			us.total_bytes,
			'PENDING'
		FROM upload_sources us
		JOIN cos_storage_profiles csp ON csp.recording_profile_id = us.recording_profile_id
			AND csp.enabled = 1
		WHERE us.status = 'READY_TO_UPLOAD'
			AND COALESCE(us.output_relative_path, '') != ''
	`)
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
	result, err := s.db.ExecContext(ctx, `
		INSERT OR IGNORE INTO jobs
			(recording_profile_id, upload_source_id, cos_object_id, type, resource_class, business_key, payload_json, status, priority, max_attempts)
		SELECT co.recording_profile_id,
			co.upload_source_id,
			co.id,
			'UPLOAD_COS_OBJECT',
			'NETWORK',
			'upload-source:' || co.upload_source_id || ':cos:' || co.cos_storage_profile_id,
			'{"cos_object_id":' || co.id || ',"upload_source_id":' || co.upload_source_id || '}',
			'PENDING',
			90,
			5
		FROM upload_source_cos_objects co
		JOIN upload_sources us ON us.id = co.upload_source_id
		WHERE co.status = 'PENDING'
			AND us.status = 'READY_TO_UPLOAD'
	`)
	if err != nil {
		return 0, fmt.Errorf("create cos jobs: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("read cos job count: %w", err)
	}
	return int(changed), nil
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

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
