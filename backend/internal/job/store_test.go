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

	updated, err := NewStore(database).Retry(ctx, admin, id, RetryRequest{})
	if err != nil {
		t.Fatalf("Retry returned error: %v", err)
	}
	if updated.Status != "PENDING" || updated.Attempts != 0 || updated.LastError != "" {
		t.Fatalf("unexpected retried job: %#v", updated)
	}
}

func TestRetryAmbiguousBilibiliRequiresConfirmation(t *testing.T) {
	ctx := context.Background()
	database := openTestDB(t, ctx)
	admin := bootstrapTestAdmin(t, ctx, database)
	created := createTestProfile(t, ctx, database, admin, "7G", "1741048619")
	if _, err := database.ExecContext(ctx, `
		INSERT INTO upload_sources
			(id, recording_profile_id, source_key, source_room_id, streamer_name_snapshot,
			 started_at, completed_at, status, total_bytes, recording_count, file_count)
		VALUES (1, ?, 'source:1', '1741048619', '7G',
			 '2026-09-12T10:00:00Z', '2026-09-12T11:00:00Z', 'READY_TO_UPLOAD', 100, 1, 1);
		INSERT INTO publications
			(id, recording_profile_id, upload_source_id, platform, status, last_error)
		VALUES (1, ?, 1, 'bilibili', 'AMBIGUOUS', 'verify before retry');
		INSERT INTO jobs
			(id, recording_profile_id, upload_source_id, publication_id, type, resource_class,
			 business_key, status, attempts, max_attempts, last_error_class, last_error)
		VALUES (10, ?, 1, 1, 'UPLOAD_BILIBILI', 'NETWORK', 'source:1:bili',
			 'FAILED', 1, 3, 'AMBIGUOUS', 'verify before retry');
	`, created.ID, created.ID, created.ID); err != nil {
		t.Fatalf("seed ambiguous Bilibili job returned error: %v", err)
	}

	store := NewStore(database)
	if _, err := store.Retry(ctx, admin, 10, RetryRequest{}); !errors.Is(err, ErrValidation) {
		t.Fatalf("expected confirmation validation error, got %v", err)
	}
	var publicationStatus, jobStatus string
	if err := database.QueryRowContext(ctx, `
		SELECT p.status, j.status FROM publications p JOIN jobs j ON j.publication_id = p.id WHERE j.id = 10
	`).Scan(&publicationStatus, &jobStatus); err != nil {
		t.Fatalf("query frozen states returned error: %v", err)
	}
	if publicationStatus != "AMBIGUOUS" || jobStatus != "FAILED" {
		t.Fatalf("unconfirmed retry mutated states: publication=%s job=%s", publicationStatus, jobStatus)
	}

	updated, err := store.Retry(ctx, admin, 10, RetryRequest{ConfirmAmbiguousBilibili: true})
	if err != nil {
		t.Fatalf("confirmed Retry returned error: %v", err)
	}
	if updated.Status != "PENDING" || updated.Attempts != 0 {
		t.Fatalf("unexpected confirmed retry job: %#v", updated)
	}
	if err := database.QueryRowContext(ctx, `SELECT status FROM publications WHERE id = 1`).Scan(&publicationStatus); err != nil {
		t.Fatalf("query retried publication returned error: %v", err)
	}
	if publicationStatus != "PENDING" {
		t.Fatalf("expected pending publication, got %s", publicationStatus)
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
