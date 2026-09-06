package account

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/7grecorder/7grecorder/backend/internal/auth"
	"github.com/7grecorder/7grecorder/backend/internal/config"
	"github.com/7grecorder/7grecorder/backend/internal/db"
)

func TestSuperAdminCreatesManagerWithDefaultPolicy(t *testing.T) {
	ctx := context.Background()
	database := openTestDB(t, ctx)
	store := NewStore(database)
	admin := bootstrapTestAdmin(t, ctx, database)

	created, err := store.Create(ctx, admin, CreateRequest{
		Username: "manager",
		Password: "correct horse battery staple",
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if created.Role != RoleManager {
		t.Fatalf("expected MANAGER role, got %q", created.Role)
	}
	if created.Policy == nil || !created.Policy.CanEditRecordingProfile || !created.Policy.CanManageLocalFiles {
		t.Fatalf("expected default manager policy, got %#v", created.Policy)
	}
}

func TestManagerCannotListAccounts(t *testing.T) {
	ctx := context.Background()
	database := openTestDB(t, ctx)
	store := NewStore(database)
	manager := insertTestManager(t, ctx, database)

	_, err := store.List(ctx, manager)
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestSuperAdminCanDisableManagerButNotSelf(t *testing.T) {
	ctx := context.Background()
	database := openTestDB(t, ctx)
	store := NewStore(database)
	admin := bootstrapTestAdmin(t, ctx, database)
	manager := insertTestManager(t, ctx, database)

	disabled := false
	updated, err := store.Update(ctx, admin, manager.ID, UpdateRequest{Enabled: &disabled})
	if err != nil {
		t.Fatalf("Update manager returned error: %v", err)
	}
	if updated.Enabled {
		t.Fatal("expected manager to be disabled")
	}

	_, err = store.Update(ctx, admin, admin.ID, UpdateRequest{Enabled: &disabled})
	if !errors.Is(err, ErrCannotDisableSelf) {
		t.Fatalf("expected ErrCannotDisableSelf, got %v", err)
	}
}

func TestSuperAdminCanUpdateManagerPolicy(t *testing.T) {
	ctx := context.Background()
	database := openTestDB(t, ctx)
	store := NewStore(database)
	admin := bootstrapTestAdmin(t, ctx, database)
	manager := insertTestManager(t, ctx, database)

	canEditProfiles := false
	policy, err := store.UpsertPolicy(ctx, admin, manager.ID, PolicyUpsert{
		CanEditRecordingProfile: &canEditProfiles,
	})
	if err != nil {
		t.Fatalf("UpsertPolicy returned error: %v", err)
	}
	if policy.CanEditRecordingProfile {
		t.Fatal("expected profile editing to be disabled")
	}
	if !policy.CanEditCosModule || !policy.CanManageLocalFiles {
		t.Fatalf("expected unrelated policy values to remain enabled, got %#v", policy)
	}
}

func TestResetPasswordChangesLoginPassword(t *testing.T) {
	ctx := context.Background()
	database := openTestDB(t, ctx)
	store := NewStore(database)
	bootstrapTestAdmin(t, ctx, database)

	updated, err := store.ResetPassword(ctx, "admin", "new correct horse battery staple")
	if err != nil {
		t.Fatalf("ResetPassword returned error: %v", err)
	}
	if updated.Username != "admin" {
		t.Fatalf("expected admin, got %q", updated.Username)
	}

	_, _, _, err = store.Login(ctx, "admin", "correct horse battery staple", 0)
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expected old password to fail, got %v", err)
	}
	user, _, _, err := store.Login(ctx, "admin", "new correct horse battery staple", 0)
	if err != nil {
		t.Fatalf("Login with new password returned error: %v", err)
	}
	if user.Username != "admin" {
		t.Fatalf("expected admin login, got %q", user.Username)
	}
}

func TestResetPasswordRejectsMissingUser(t *testing.T) {
	ctx := context.Background()
	database := openTestDB(t, ctx)
	store := NewStore(database)

	_, err := store.ResetPassword(ctx, "missing", "new correct horse battery staple")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func openTestDB(t *testing.T, ctx context.Context) *sql.DB {
	t.Helper()
	root := t.TempDir()
	cfg := config.Config{
		DataRoot:   root,
		SQLitePath: filepath.Join(root, "7grecorder.db"),
		TempRoot:   filepath.Join(root, "temp"),
	}
	if err := db.Migrate(ctx, cfg); err != nil {
		t.Fatalf("Migrate returned error: %v", err)
	}
	database, err := db.Open(ctx, cfg)
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

func bootstrapTestAdmin(t *testing.T, ctx context.Context, database *sql.DB) User {
	t.Helper()
	user, err := NewStore(database).BootstrapSuperAdmin(ctx, "admin", "correct horse battery staple")
	if err != nil {
		t.Fatalf("BootstrapSuperAdmin returned error: %v", err)
	}
	return user
}

func insertTestManager(t *testing.T, ctx context.Context, database *sql.DB) User {
	t.Helper()
	passwordHash, err := authHash("unused manager password")
	if err != nil {
		t.Fatalf("authHash returned error: %v", err)
	}
	result, err := database.ExecContext(ctx, `
		INSERT INTO users (username, password_hash, role, enabled)
		VALUES ('manager', ?, ?, 1)
	`, passwordHash, RoleManager)
	if err != nil {
		t.Fatalf("insert manager returned error: %v", err)
	}
	userID, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("LastInsertId returned error: %v", err)
	}
	user, err := NewStore(database).UserByID(ctx, userID)
	if err != nil {
		t.Fatalf("UserByID returned error: %v", err)
	}
	return user
}

func authHash(password string) (string, error) {
	return auth.HashPassword(password)
}
