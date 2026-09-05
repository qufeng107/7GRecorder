package profile

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/7grecorder/7grecorder/backend/internal/account"
	"github.com/7grecorder/7grecorder/backend/internal/config"
	"github.com/7grecorder/7grecorder/backend/internal/db"
)

func TestCreateProfileCreatesDefaults(t *testing.T) {
	ctx := context.Background()
	database := openTestDB(t, ctx)
	store := NewStore(database)
	actor := bootstrapTestAdmin(t, ctx, database)

	created, err := store.Create(ctx, actor, CreateRequest{
		Name:         "Main room",
		RoomID:       "123456",
		StreamerName: "Streamer",
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	if created.ID == 0 {
		t.Fatal("expected profile id")
	}
	if created.Platform != "bilibili" {
		t.Fatalf("expected bilibili platform, got %q", created.Platform)
	}
	if !created.Settings.AutoRecord || !created.Settings.RecordDanmaku {
		t.Fatal("expected default recording settings to be enabled")
	}
	if created.Settings.SegmentDurationSec != 1800 {
		t.Fatalf("expected default segment duration, got %d", created.Settings.SegmentDurationSec)
	}
	if created.Runtime.SyncStatus != "PENDING" {
		t.Fatalf("expected pending sync status, got %q", created.Runtime.SyncStatus)
	}
}

func TestCreateProfileRejectsDuplicateActiveRoom(t *testing.T) {
	ctx := context.Background()
	database := openTestDB(t, ctx)
	store := NewStore(database)
	actor := bootstrapTestAdmin(t, ctx, database)

	_, err := store.Create(ctx, actor, CreateRequest{Name: "A", RoomID: "123456", StreamerName: "A"})
	if err != nil {
		t.Fatalf("first Create returned error: %v", err)
	}
	_, err = store.Create(ctx, actor, CreateRequest{Name: "B", RoomID: "123456", StreamerName: "B"})
	if !errors.Is(err, ErrRoomInUse) {
		t.Fatalf("expected ErrRoomInUse, got %v", err)
	}
}

func TestManagerPolicyBlocksProfileCreate(t *testing.T) {
	ctx := context.Background()
	database := openTestDB(t, ctx)
	store := NewStore(database)
	manager := insertTestManager(t, ctx, database)

	_, err := database.ExecContext(ctx, `
		INSERT INTO manager_policies (user_id, can_edit_recording_profile)
		VALUES (?, 0)
	`, manager.ID)
	if err != nil {
		t.Fatalf("insert manager policy returned error: %v", err)
	}

	_, err = store.Create(ctx, manager, CreateRequest{Name: "A", RoomID: "123456", StreamerName: "A"})
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestUpdateSettings(t *testing.T) {
	ctx := context.Background()
	database := openTestDB(t, ctx)
	store := NewStore(database)
	actor := bootstrapTestAdmin(t, ctx, database)

	created, err := store.Create(ctx, actor, CreateRequest{Name: "Main room", RoomID: "123456", StreamerName: "Streamer"})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	quality := "high"
	autoRecord := false
	segment := int64(600)
	settings, err := store.UpsertSettings(ctx, actor, created.ID, SettingsUpsert{
		AutoRecord:         &autoRecord,
		Quality:            &quality,
		SegmentDurationSec: &segment,
	})
	if err != nil {
		t.Fatalf("UpsertSettings returned error: %v", err)
	}
	if settings.AutoRecord {
		t.Fatal("expected auto_record to be false")
	}
	if settings.Quality != "high" {
		t.Fatalf("expected quality high, got %q", settings.Quality)
	}
	if settings.SegmentDurationSec != 600 {
		t.Fatalf("expected segment duration 600, got %d", settings.SegmentDurationSec)
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

func insertTestManager(t *testing.T, ctx context.Context, database *sql.DB) account.User {
	t.Helper()
	result, err := database.ExecContext(ctx, `
		INSERT INTO users (username, password_hash, role, enabled)
		VALUES ('manager', 'not-used', ?, 1)
	`, account.RoleManager)
	if err != nil {
		t.Fatalf("insert manager returned error: %v", err)
	}
	userID, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("LastInsertId returned error: %v", err)
	}
	user, err := account.NewStore(database).UserByID(ctx, userID)
	if err != nil {
		t.Fatalf("UserByID returned error: %v", err)
	}
	return user
}
