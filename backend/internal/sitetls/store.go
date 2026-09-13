package sitetls

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/7grecorder/7grecorder/backend/internal/account"
	"github.com/7grecorder/7grecorder/backend/internal/config"
	"github.com/7grecorder/7grecorder/backend/internal/secretbox"
)

var (
	ErrForbidden  = errors.New("site tls forbidden")
	ErrNotFound   = errors.New("site tls resource not found")
	ErrValidation = errors.New("site tls validation failed")
)

var hostnamePattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)+$`)

type Settings struct {
	Enabled               bool     `json:"enabled"`
	CredentialID          int64    `json:"credential_id,omitempty"`
	PrimaryDomain         string   `json:"primary_domain"`
	AdditionalDomains     []string `json:"additional_domains"`
	Status                string   `json:"status"`
	LatestCertificateID   string   `json:"latest_certificate_id,omitempty"`
	LatestNotAfter        string   `json:"latest_not_after,omitempty"`
	StagedCertificateID   string   `json:"staged_certificate_id,omitempty"`
	StagedAt              string   `json:"staged_at,omitempty"`
	DeployedCertificateID string   `json:"deployed_certificate_id,omitempty"`
	DeployedAt            string   `json:"deployed_at,omitempty"`
	LastCheckedAt         string   `json:"last_checked_at,omitempty"`
	LastError             string   `json:"last_error,omitempty"`
	UpdatedAt             string   `json:"updated_at"`
}

type SettingsUpsert struct {
	Enabled           bool     `json:"enabled"`
	CredentialID      int64    `json:"credential_id"`
	PrimaryDomain     string   `json:"primary_domain"`
	AdditionalDomains []string `json:"additional_domains"`
}

type SyncRequest struct {
	SecretID              string
	SecretKey             string
	PrimaryDomain         string
	AdditionalDomains     []string
	DeployedCertificateID string
}

type Store struct {
	db  *sql.DB
	cfg config.Config
}

func NewStore(database *sql.DB, cfg config.Config) Store {
	return Store{db: database, cfg: cfg}
}

func (s Store) Get(ctx context.Context, actor account.User) (Settings, error) {
	if actor.Role != account.RoleSuperAdmin {
		return Settings{}, ErrForbidden
	}
	if err := s.ObserveDeploymentReceipt(ctx); err != nil {
		return Settings{}, err
	}
	return s.get(ctx)
}

func (s Store) Upsert(ctx context.Context, actor account.User, req SettingsUpsert) (Settings, error) {
	if actor.Role != account.RoleSuperAdmin {
		return Settings{}, ErrForbidden
	}
	primary, additional, err := normalizeDomains(req.PrimaryDomain, req.AdditionalDomains)
	if err != nil {
		return Settings{}, err
	}
	if req.Enabled {
		if req.CredentialID <= 0 {
			return Settings{}, ErrValidation
		}
		if err := s.ensureCredential(ctx, req.CredentialID); err != nil {
			return Settings{}, err
		}
	}
	encoded, _ := json.Marshal(additional)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Settings{}, fmt.Errorf("begin site tls settings update: %w", err)
	}
	defer tx.Rollback()
	status := "DISABLED"
	if req.Enabled {
		status = "PENDING"
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE site_tls_settings
		SET credential_id = NULLIF(?, 0), enabled = ?, primary_domain = ?, additional_domains_json = ?,
			status = ?, last_error = NULL, updated_at = CURRENT_TIMESTAMP
		WHERE id = 1
	`, req.CredentialID, boolInt(req.Enabled), primary, string(encoded), status); err != nil {
		return Settings{}, fmt.Errorf("update site tls settings: %w", err)
	}
	if req.Enabled {
		if err := scheduleSyncTx(ctx, tx); err != nil {
			return Settings{}, err
		}
	} else if _, err := tx.ExecContext(ctx, `UPDATE jobs SET status = 'CANCELLED', updated_at = CURRENT_TIMESTAMP WHERE type = 'SYNC_SITE_TLS' AND status = 'PENDING'`); err != nil {
		return Settings{}, fmt.Errorf("cancel site tls sync: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Settings{}, fmt.Errorf("commit site tls settings update: %w", err)
	}
	return s.get(ctx)
}

func (s Store) ScheduleNow(ctx context.Context, actor account.User) (Settings, error) {
	if actor.Role != account.RoleSuperAdmin {
		return Settings{}, ErrForbidden
	}
	var enabled int
	if err := s.db.QueryRowContext(ctx, `SELECT enabled FROM site_tls_settings WHERE id = 1`).Scan(&enabled); err != nil {
		return Settings{}, err
	}
	if enabled != 1 {
		return Settings{}, ErrValidation
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Settings{}, err
	}
	defer tx.Rollback()
	if err := scheduleSyncTx(ctx, tx); err != nil {
		return Settings{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE site_tls_settings SET status = 'PENDING', last_error = NULL, updated_at = CURRENT_TIMESTAMP WHERE id = 1`); err != nil {
		return Settings{}, err
	}
	if err := tx.Commit(); err != nil {
		return Settings{}, err
	}
	return s.get(ctx)
}

func (s Store) Reconcile(ctx context.Context) error {
	if err := s.ObserveDeploymentReceipt(ctx); err != nil {
		return err
	}
	var due int
	err := s.db.QueryRowContext(ctx, `
		SELECT CASE WHEN enabled = 1 AND credential_id IS NOT NULL
			AND (last_checked_at IS NULL OR last_checked_at <= datetime('now', '-24 hours')) THEN 1 ELSE 0 END
		FROM site_tls_settings WHERE id = 1
	`).Scan(&due)
	if err != nil || due != 1 {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := scheduleSyncTx(ctx, tx); err != nil {
		return err
	}
	return tx.Commit()
}

func (s Store) SyncRequest(ctx context.Context) (SyncRequest, error) {
	var encrypted []byte
	var additionalJSON string
	var enabled int
	var request SyncRequest
	err := s.db.QueryRowContext(ctx, `
		SELECT st.enabled, st.primary_domain, st.additional_domains_json,
			COALESCE(st.deployed_certificate_id, ''), c.encrypted_secret
		FROM site_tls_settings st
		JOIN credentials c ON c.id = st.credential_id
			AND c.scope = 'SYSTEM' AND c.platform = 'tencent_ssl' AND c.purpose = 'TLS'
		WHERE st.id = 1
	`).Scan(&enabled, &request.PrimaryDomain, &additionalJSON, &request.DeployedCertificateID, &encrypted)
	if errors.Is(err, sql.ErrNoRows) || enabled != 1 {
		return SyncRequest{}, ErrValidation
	}
	if err != nil {
		return SyncRequest{}, fmt.Errorf("load site tls sync request: %w", err)
	}
	if err := json.Unmarshal([]byte(additionalJSON), &request.AdditionalDomains); err != nil {
		return SyncRequest{}, fmt.Errorf("decode site tls domains: %w", err)
	}
	plaintext, err := secretbox.Decrypt(s.cfg.MasterKeyPath, encrypted)
	if err != nil {
		return SyncRequest{}, NewClassifiedError("AUTH", "decrypt Tencent SSL credential", err)
	}
	var secret map[string]string
	if err := json.Unmarshal(plaintext, &secret); err != nil {
		return SyncRequest{}, NewClassifiedError("AUTH", "Tencent SSL credential must be JSON", err)
	}
	request.SecretID = firstNonEmpty(secret["secret_id"], secret["secretId"], secret["SecretId"], secret["SecretID"])
	request.SecretKey = firstNonEmpty(secret["secret_key"], secret["secretKey"], secret["SecretKey"])
	if request.SecretID == "" || request.SecretKey == "" {
		return SyncRequest{}, NewClassifiedError("AUTH", "Tencent SSL credential must include secret_id and secret_key", nil)
	}
	return request, nil
}

func (s Store) MarkChecked(ctx context.Context, result SyncResult) error {
	status := "STAGED"
	if result.AlreadyCurrent {
		status = "ACTIVE"
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE site_tls_settings SET status = CASE WHEN enabled = 1 THEN ? ELSE 'DISABLED' END,
			latest_certificate_id = ?, latest_not_after = ?,
			staged_certificate_id = CASE WHEN ? THEN staged_certificate_id ELSE ? END,
			staged_at = CASE WHEN ? THEN staged_at ELSE CURRENT_TIMESTAMP END,
			last_checked_at = CURRENT_TIMESTAMP, last_error = NULL, updated_at = CURRENT_TIMESTAMP WHERE id = 1
	`, status, result.CertificateID, result.NotAfter.UTC(), result.AlreadyCurrent, result.CertificateID, result.AlreadyCurrent)
	if err != nil {
		return fmt.Errorf("mark site tls checked: %w", err)
	}
	return nil
}

func (s Store) MarkError(ctx context.Context, message string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE site_tls_settings SET status = CASE WHEN enabled = 1 THEN 'ERROR' ELSE 'DISABLED' END,
		last_error = ?, last_checked_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP WHERE id = 1`, truncate(message, 1000))
	return err
}

func (s Store) ObserveDeploymentReceipt(ctx context.Context) error {
	settings, err := s.get(ctx)
	if err != nil {
		return err
	}
	receiptPath := filepath.Join(s.cfg.DataRoot, "tls", settings.PrimaryDomain, "deployed-certificate-id")
	value, err := os.ReadFile(receiptPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read site tls deployment receipt: %w", err)
	}
	certificateID := strings.TrimSpace(string(value))
	if certificateID == "" || certificateID == settings.DeployedCertificateID {
		return nil
	}
	_, err = s.db.ExecContext(ctx, `UPDATE site_tls_settings SET deployed_certificate_id = ?, deployed_at = CURRENT_TIMESTAMP,
		status = CASE WHEN enabled = 1 AND staged_certificate_id = ? THEN 'ACTIVE' ELSE status END,
		last_error = NULL, updated_at = CURRENT_TIMESTAMP WHERE id = 1`, certificateID, certificateID)
	return err
}

func (s Store) get(ctx context.Context) (Settings, error) {
	var item Settings
	var enabled int
	var additionalJSON string
	err := s.db.QueryRowContext(ctx, `SELECT enabled, COALESCE(credential_id, 0), primary_domain, additional_domains_json,
		status, COALESCE(latest_certificate_id, ''), COALESCE(latest_not_after, ''), COALESCE(staged_certificate_id, ''),
		COALESCE(staged_at, ''), COALESCE(deployed_certificate_id, ''), COALESCE(deployed_at, ''),
		COALESCE(last_checked_at, ''), COALESCE(last_error, ''), updated_at FROM site_tls_settings WHERE id = 1`).Scan(
		&enabled, &item.CredentialID, &item.PrimaryDomain, &additionalJSON, &item.Status, &item.LatestCertificateID,
		&item.LatestNotAfter, &item.StagedCertificateID, &item.StagedAt, &item.DeployedCertificateID,
		&item.DeployedAt, &item.LastCheckedAt, &item.LastError, &item.UpdatedAt)
	if err != nil {
		return Settings{}, err
	}
	item.Enabled = enabled == 1
	if err := json.Unmarshal([]byte(additionalJSON), &item.AdditionalDomains); err != nil {
		return Settings{}, err
	}
	return item, nil
}

func (s Store) ensureCredential(ctx context.Context, id int64) error {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM credentials WHERE id = ? AND scope = 'SYSTEM' AND platform = 'tencent_ssl' AND purpose = 'TLS'`, id).Scan(&count); err != nil {
		return err
	}
	if count != 1 {
		return ErrValidation
	}
	return nil
}

func scheduleSyncTx(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO jobs (type, resource_class, business_key, payload_json, status, priority, max_attempts)
		VALUES ('SYNC_SITE_TLS', 'NETWORK', 'system:site-tls:sync', '{}', 'PENDING', 90, 5)
		ON CONFLICT(business_key) DO UPDATE SET status = 'PENDING', attempts = 0, run_after = CURRENT_TIMESTAMP,
			locked_at = NULL, heartbeat_at = NULL, locked_by = NULL, last_error_class = NULL, last_error = NULL,
			updated_at = CURRENT_TIMESTAMP
		WHERE jobs.status != 'RUNNING'
	`)
	if err != nil {
		return fmt.Errorf("schedule site tls sync: %w", err)
	}
	return nil
}

func normalizeDomains(primary string, additional []string) (string, []string, error) {
	primary = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(primary), "."))
	if !validHostname(primary) {
		return "", nil, ErrValidation
	}
	seen := map[string]bool{primary: true}
	normalized := make([]string, 0, len(additional))
	for _, domain := range additional {
		domain = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(domain), "."))
		if domain == "" || seen[domain] {
			continue
		}
		if !validHostname(domain) {
			return "", nil, ErrValidation
		}
		seen[domain] = true
		normalized = append(normalized, domain)
	}
	sort.Strings(normalized)
	if primary != "7g.chat" || len(normalized) != 1 || normalized[0] != "www.7g.chat" {
		return "", nil, ErrValidation
	}
	return primary, normalized, nil
}

func validHostname(value string) bool {
	return len(value) <= 253 && net.ParseIP(value) == nil && hostnamePattern.MatchString(value)
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
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
