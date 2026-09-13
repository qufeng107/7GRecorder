package sitetls

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/7grecorder/7grecorder/backend/internal/account"
	"github.com/7grecorder/7grecorder/backend/internal/config"
	"github.com/7grecorder/7grecorder/backend/internal/db"
	"github.com/7grecorder/7grecorder/backend/internal/upload"
)

func TestSiteTLSSettingsRequireSystemCredentialAndScheduleJob(t *testing.T) {
	ctx := context.Background()
	cfg, database := openSiteTLSTestDB(t, ctx)
	admin := account.User{ID: 1, Username: "admin", Role: account.RoleSuperAdmin, Enabled: true}
	manager := account.User{ID: 2, Username: "manager", Role: account.RoleManager, Enabled: true}
	if _, err := database.ExecContext(ctx, `INSERT INTO users (id, username, password_hash, role) VALUES (1, 'admin', 'x', 'SUPER_ADMIN'), (2, 'manager', 'x', 'MANAGER')`); err != nil {
		t.Fatal(err)
	}
	uploadStore := upload.NewStore(database, cfg)
	userCredential, err := uploadStore.CreateCredential(ctx, admin, upload.CredentialCreate{Scope: "USER", Platform: "tencent_ssl", Purpose: "TLS", AccountLabel: "wrong scope", Secret: []byte(`{"secret_id":"id","secret_key":"key"}`)})
	if err != nil {
		t.Fatal(err)
	}
	systemCredential, err := uploadStore.CreateCredential(ctx, admin, upload.CredentialCreate{Scope: "SYSTEM", Platform: "tencent_ssl", Purpose: "TLS", AccountLabel: "site tls", Secret: []byte(`{"secret_id":"id","secret_key":"key"}`)})
	if err != nil {
		t.Fatal(err)
	}
	store := NewStore(database, cfg)
	if _, err := store.Get(ctx, manager); err != ErrForbidden {
		t.Fatalf("manager Get error = %v", err)
	}
	if _, err := store.Upsert(ctx, admin, SettingsUpsert{Enabled: true, CredentialID: userCredential.ID, PrimaryDomain: "7g.chat", AdditionalDomains: []string{"www.7g.chat"}}); err != ErrValidation {
		t.Fatalf("USER credential error = %v", err)
	}
	settings, err := store.Upsert(ctx, admin, SettingsUpsert{Enabled: true, CredentialID: systemCredential.ID, PrimaryDomain: "7G.CHAT.", AdditionalDomains: []string{"www.7g.chat", "www.7g.chat"}})
	if err != nil {
		t.Fatal(err)
	}
	if settings.Status != "PENDING" || settings.PrimaryDomain != "7g.chat" || len(settings.AdditionalDomains) != 1 {
		t.Fatalf("unexpected settings: %#v", settings)
	}
	var jobType, status string
	if err := database.QueryRowContext(ctx, `SELECT type, status FROM jobs WHERE business_key = 'system:site-tls:sync'`).Scan(&jobType, &status); err != nil {
		t.Fatal(err)
	}
	if jobType != "SYNC_SITE_TLS" || status != "PENDING" {
		t.Fatalf("unexpected job %s/%s", jobType, status)
	}
	request, err := store.SyncRequest(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if request.SecretID != "id" || request.SecretKey != "key" {
		t.Fatalf("secret did not decrypt: %#v", request)
	}
}

func TestObserveDeploymentReceiptMarksMatchingStageActive(t *testing.T) {
	ctx := context.Background()
	cfg, database := openSiteTLSTestDB(t, ctx)
	if _, err := database.ExecContext(ctx, `UPDATE site_tls_settings SET enabled = 1, status = 'STAGED', staged_certificate_id = 'cert-1' WHERE id = 1`); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(cfg.DataRoot, "tls", "7g.chat")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "deployed-certificate-id"), []byte("cert-1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := NewStore(database, cfg).ObserveDeploymentReceipt(ctx); err != nil {
		t.Fatal(err)
	}
	var status, deployed string
	if err := database.QueryRowContext(ctx, `SELECT status, deployed_certificate_id FROM site_tls_settings WHERE id = 1`).Scan(&status, &deployed); err != nil {
		t.Fatal(err)
	}
	if status != "ACTIVE" || deployed != "cert-1" {
		t.Fatalf("unexpected deployment %s/%s", status, deployed)
	}
}

func openSiteTLSTestDB(t *testing.T, ctx context.Context) (config.Config, *sql.DB) {
	t.Helper()
	root := t.TempDir()
	masterKeyPath := filepath.Join(root, "master.key")
	if err := os.WriteFile(masterKeyPath, []byte("test-master-key"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{DataRoot: root, SQLitePath: filepath.Join(root, "test.db"), MasterKeyPath: masterKeyPath}
	if err := db.Migrate(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	database, err := db.Open(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return cfg, database
}
