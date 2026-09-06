package job

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/7grecorder/7grecorder/backend/internal/account"
	"github.com/7grecorder/7grecorder/backend/internal/config"
	"github.com/7grecorder/7grecorder/backend/internal/db"
	"github.com/7grecorder/7grecorder/backend/internal/profile"
)

func TestSuperAdminListsJobs(t *testing.T) {
	ctx := context.Background()
	database := openTestDB(t, ctx)
	admin := bootstrapTestAdmin(t, ctx, database)
	created := createTestProfile(t, ctx, database, admin, "7G", "1741048619")
	insertTestJob(t, ctx, database, created.ID, "FAILED")

	items, err := NewStore(database).List(ctx, admin, 10)
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected profile sync job and inserted job, got %#v", items)
	}
	if items[0].ProfileName == "" {
		t.Fatalf("expected profile metadata, got %#v", items[0])
	}
}

func TestManagerListsOnlyOwnJobs(t *testing.T) {
	ctx := context.Background()
	database := openTestDB(t, ctx)
	admin := bootstrapTestAdmin(t, ctx, database)
	manager := createTestManager(t, ctx, database, admin, "manager")
	other := createTestManager(t, ctx, database, admin, "other-manager")
	ownProfile := createTestProfile(t, ctx, database, manager, "own", "1741048619")
	otherProfile := createTestProfile(t, ctx, database, other, "other", "1741048620")
	insertTestJob(t, ctx, database, ownProfile.ID, "FAILED")
	insertTestJob(t, ctx, database, otherProfile.ID, "FAILED")

	items, err := NewStore(database).List(ctx, manager, 20)
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	for _, item := range items {
		if item.RecordingProfileID != ownProfile.ID {
			t.Fatalf("manager saw another profile job: %#v", item)
		}
	}
	if len(items) == 0 {
		t.Fatalf("expected manager to see own jobs")
	}
}

func TestRetryFailedJobResetsState(t *testing.T) {
	ctx := context.Background()
	database := openTestDB(t, ctx)
	admin := bootstrapTestAdmin(t, ctx, database)
	created := createTestProfile(t, ctx, database, admin, "7G", "1741048619")
	id := insertTestJob(t, ctx, database, created.ID, "FAILED")

	updated, err := NewStore(database).Retry(ctx, admin, id)
	if err != nil {
		t.Fatalf("Retry returned error: %v", err)
	}
	if updated.Status != "PENDING" || updated.Attempts != 0 || updated.LastError != "" {
		t.Fatalf("unexpected retried job: %#v", updated)
	}
}

func TestCancelRejectsRunningJob(t *testing.T) {
	ctx := context.Background()
	database := openTestDB(t, ctx)
	admin := bootstrapTestAdmin(t, ctx, database)
	created := createTestProfile(t, ctx, database, admin, "7G", "1741048619")
	id := insertTestJob(t, ctx, database, created.ID, "RUNNING")

	_, err := NewStore(database).Cancel(ctx, admin, id)
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("expected ErrValidation, got %v", err)
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

func bootstrapTestAdmin(t *testing.T, ctx context.Context, database *sql.DB) account.User {
	t.Helper()
	user, err := account.NewStore(database).BootstrapSuperAdmin(ctx, "admin", "correct horse battery staple")
	if err != nil {
		t.Fatalf("BootstrapSuperAdmin returned error: %v", err)
	}
	return user
}

func createTestManager(t *testing.T, ctx context.Context, database *sql.DB, admin account.User, username string) account.User {
	t.Helper()
	created, err := account.NewStore(database).Create(ctx, admin, account.CreateRequest{
		Username: username,
		Password: "correct horse battery staple",
	})
	if err != nil {
		t.Fatalf("Create manager returned error: %v", err)
	}
	return created.User
}

func createTestProfile(t *testing.T, ctx context.Context, database *sql.DB, actor account.User, name string, roomID string) profile.RecordingProfile {
	t.Helper()
	created, err := profile.NewStore(database).Create(ctx, actor, profile.CreateRequest{
		Name:         name,
		RoomID:       roomID,
		StreamerName: name,
	})
	if err != nil {
		t.Fatalf("Create profile returned error: %v", err)
	}
	return created
}

func insertTestJob(t *testing.T, ctx context.Context, database *sql.DB, profileID int64, status string) int64 {
	t.Helper()
	result, err := database.ExecContext(ctx, `
		INSERT INTO jobs
			(recording_profile_id, type, resource_class, business_key, payload_json, status, attempts, max_attempts, last_error_class, last_error)
		VALUES (?, 'TEST_JOB', 'LIGHT', ?, '{}', ?, 2, 3, 'TRANSIENT', 'temporary failure')
	`, profileID, fmt.Sprintf("test:%s:%d", status, profileID), status)
	if err != nil {
		t.Fatalf("insert job returned error: %v", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("LastInsertId returned error: %v", err)
	}
	return id
}
