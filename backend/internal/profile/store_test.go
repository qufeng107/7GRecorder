package profile

import (
	"context"
	"database/sql"
	"encoding/json"
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

func TestCreateProfileEnqueuesRecorderSyncForBilibiliRoom(t *testing.T) {
	ctx := context.Background()
	database := openTestDB(t, ctx)
	store := NewStore(database)
	actor := bootstrapTestAdmin(t, ctx, database)

	created, err := store.Create(ctx, actor, CreateRequest{
		Name:         "7G Live",
		RoomID:       "1741048619",
		StreamerName: "7G",
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	var jobType string
	var businessKey string
	var payloadJSON string
	err = database.QueryRowContext(ctx, `
		SELECT type, business_key, payload_json
		FROM jobs
		WHERE recording_profile_id = ?
	`, created.ID).Scan(&jobType, &businessKey, &payloadJSON)
	if err != nil {
		t.Fatalf("query sync job returned error: %v", err)
	}
	if jobType != "SYNC_RECORDER_PROFILE" {
		t.Fatalf("expected SYNC_RECORDER_PROFILE job, got %q", jobType)
	}
	if businessKey != "profile:1:recorder:sync" {
		t.Fatalf("expected stable recorder sync business key, got %q", businessKey)
	}

	var payload RecorderSyncPayload
	if err := json.Unmarshal([]byte(payloadJSON), &payload); err != nil {
		t.Fatalf("unmarshal recorder sync payload returned error: %v", err)
	}
	if payload.RoomID != "1741048619" {
		t.Fatalf("expected room 1741048619, got %q", payload.RoomID)
	}
	if payload.LiveURL != "https://live.bilibili.com/1741048619" {
		t.Fatalf("expected Bilibili live URL, got %q", payload.LiveURL)
	}
	if !payload.Enabled || !payload.AutoRecord {
		t.Fatal("expected enabled auto-record payload")
	}
	if payload.OutputRelativeDir != "recordings/1" {
		t.Fatalf("expected stable output directory, got %q", payload.OutputRelativeDir)
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

	var payloadJSON string
	err = database.QueryRowContext(ctx, `
		SELECT payload_json
		FROM jobs
		WHERE business_key = ?
	`, "profile:1:recorder:sync").Scan(&payloadJSON)
	if err != nil {
		t.Fatalf("query sync job returned error: %v", err)
	}
	var payload RecorderSyncPayload
	if err := json.Unmarshal([]byte(payloadJSON), &payload); err != nil {
		t.Fatalf("unmarshal recorder sync payload returned error: %v", err)
	}
	if payload.Quality != "high" || payload.SegmentDurationSec != 600 {
		t.Fatalf("expected updated sync payload, got quality=%q segment=%d", payload.Quality, payload.SegmentDurationSec)
	}
}

func TestUpdateSettingsRejectsCoreChangesDuringActiveRecording(t *testing.T) {
	ctx := context.Background()
	database := openTestDB(t, ctx)
	store := NewStore(database)
	actor := bootstrapTestAdmin(t, ctx, database)

	created, err := store.Create(ctx, actor, CreateRequest{Name: "Main room", RoomID: "123456", StreamerName: "Streamer"})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	_, err = database.ExecContext(ctx, `
		INSERT INTO recordings
			(recording_profile_id, started_at, recording_status, source_room_id, streamer_name_snapshot)
		VALUES (?, CURRENT_TIMESTAMP, 'ACTIVE', ?, ?)
	`, created.ID, created.RoomID, created.StreamerName)
	if err != nil {
		t.Fatalf("insert active recording returned error: %v", err)
	}

	quality := "high"
	_, err = store.UpsertSettings(ctx, actor, created.ID, SettingsUpsert{Quality: &quality})
	if !errors.Is(err, ErrRecordingActive) {
		t.Fatalf("expected ErrRecordingActive, got %v", err)
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
