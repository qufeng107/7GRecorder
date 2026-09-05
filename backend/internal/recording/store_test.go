package recording

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/7grecorder/7grecorder/backend/internal/account"
	"github.com/7grecorder/7grecorder/backend/internal/config"
	"github.com/7grecorder/7grecorder/backend/internal/db"
	"github.com/7grecorder/7grecorder/backend/internal/profile"
)

func TestReconcileLocalImportsRecordingFiles(t *testing.T) {
	ctx := context.Background()
	cfg, database := openTestDB(t, ctx)
	actor := bootstrapTestAdmin(t, ctx, database)
	_, err := profile.NewStore(database).Create(ctx, actor, profile.CreateRequest{
		Name:         "7G",
		RoomID:       "1741048619",
		StreamerName: "Streamer",
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	recordingDir := filepath.Join(cfg.DataRoot, "recordings", "1741048619-Streamer")
	if err := os.MkdirAll(recordingDir, 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	filePath := filepath.Join(recordingDir, "record-1741048619-20260905-224258-164-title.flv")
	if err := os.WriteFile(filePath, []byte("video"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	oldTime := time.Now().Add(-10 * time.Minute)
	if err := os.Chtimes(filePath, oldTime, oldTime); err != nil {
		t.Fatalf("Chtimes returned error: %v", err)
	}

	result, err := NewStore(database, cfg).ReconcileLocal(ctx, actor)
	if err != nil {
		t.Fatalf("ReconcileLocal returned error: %v", err)
	}
	if result.Imported != 1 || result.Updated != 0 {
		t.Fatalf("expected one import, got %#v", result)
	}

	items, err := NewStore(database, cfg).List(ctx, actor)
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one recording, got %d", len(items))
	}
	if items[0].RoomID != "1741048619" || items[0].Title != "title" {
		t.Fatalf("unexpected recording: %#v", items[0])
	}
	if len(items[0].Files) != 1 || items[0].Files[0].RelativePath != "recordings/1741048619-Streamer/record-1741048619-20260905-224258-164-title.flv" {
		t.Fatalf("unexpected recording files: %#v", items[0].Files)
	}
}

func TestReconcileLocalUpdatesExistingRecordingFiles(t *testing.T) {
	ctx := context.Background()
	cfg, database := openTestDB(t, ctx)
	actor := bootstrapTestAdmin(t, ctx, database)
	_, err := profile.NewStore(database).Create(ctx, actor, profile.CreateRequest{
		Name:         "7G",
		RoomID:       "1741048619",
		StreamerName: "Streamer",
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	recordingDir := filepath.Join(cfg.DataRoot, "recordings", "1741048619-Streamer")
	if err := os.MkdirAll(recordingDir, 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	filePath := filepath.Join(recordingDir, "record-1741048619-20260905-224258-164-title.flv")
	if err := os.WriteFile(filePath, []byte("video"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	oldTime := time.Now().Add(-10 * time.Minute)
	if err := os.Chtimes(filePath, oldTime, oldTime); err != nil {
		t.Fatalf("Chtimes returned error: %v", err)
	}

	store := NewStore(database, cfg)
	if _, err := store.ReconcileLocal(ctx, actor); err != nil {
		t.Fatalf("first ReconcileLocal returned error: %v", err)
	}
	if err := os.WriteFile(filePath, []byte("longer video"), 0o644); err != nil {
		t.Fatalf("second WriteFile returned error: %v", err)
	}
	if err := os.Chtimes(filePath, oldTime, oldTime); err != nil {
		t.Fatalf("second Chtimes returned error: %v", err)
	}
	result, err := store.ReconcileLocal(ctx, actor)
	if err != nil {
		t.Fatalf("second ReconcileLocal returned error: %v", err)
	}
	if result.Imported != 0 || result.Updated != 1 {
		t.Fatalf("expected one update, got %#v", result)
	}
}

func TestFileForDownloadReturnsClosedVideoFile(t *testing.T) {
	ctx := context.Background()
	cfg, database := openTestDB(t, ctx)
	actor := bootstrapTestAdmin(t, ctx, database)
	_, err := profile.NewStore(database).Create(ctx, actor, profile.CreateRequest{
		Name:         "7G",
		RoomID:       "1741048619",
		StreamerName: "Streamer",
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	recordingDir := filepath.Join(cfg.DataRoot, "recordings", "1741048619-Streamer")
	if err := os.MkdirAll(recordingDir, 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	filePath := filepath.Join(recordingDir, "record-1741048619-20260905-224258-164-title.flv")
	if err := os.WriteFile(filePath, []byte("video"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	oldTime := time.Now().Add(-10 * time.Minute)
	if err := os.Chtimes(filePath, oldTime, oldTime); err != nil {
		t.Fatalf("Chtimes returned error: %v", err)
	}

	store := NewStore(database, cfg)
	if _, err := store.ReconcileLocal(ctx, actor); err != nil {
		t.Fatalf("ReconcileLocal returned error: %v", err)
	}
	items, err := store.List(ctx, actor)
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}

	file, err := store.FileForDownload(ctx, actor, items[0].Files[0].ID)
	if err != nil {
		t.Fatalf("FileForDownload returned error: %v", err)
	}
	if file.OriginalName != "record-1741048619-20260905-224258-164-title.flv" {
		t.Fatalf("unexpected download file: %#v", file)
	}
}

func TestFileForDownloadRejectsWritingFile(t *testing.T) {
	ctx := context.Background()
	_, database := openTestDB(t, ctx)
	actor := bootstrapTestAdmin(t, ctx, database)
	if _, err := database.ExecContext(ctx, `
		INSERT INTO recording_profiles
			(owner_user_id, name, platform, room_id, streamer_name, timezone)
		VALUES (?, '7G', 'bilibili', '1741048619', 'Streamer', 'Asia/Shanghai')
	`, actor.ID); err != nil {
		t.Fatalf("insert profile returned error: %v", err)
	}
	if _, err := database.ExecContext(ctx, `
		INSERT INTO recordings
			(recording_profile_id, title, started_at, recording_status, source_room_id, streamer_name_snapshot)
		VALUES (1, 'title', CURRENT_TIMESTAMP, 'ACTIVE', '1741048619', 'Streamer')
	`); err != nil {
		t.Fatalf("insert recording returned error: %v", err)
	}
	if _, err := database.ExecContext(ctx, `
		INSERT INTO recording_files
			(recording_id, relative_path, original_name, kind, file_status)
		VALUES (1, 'recordings/1741048619-Streamer/file.flv', 'file.flv', 'video', 'WRITING')
	`); err != nil {
		t.Fatalf("insert file returned error: %v", err)
	}

	_, err := NewStore(database, config.Config{}).FileForDownload(ctx, actor, 1)
	if !errors.Is(err, ErrNotReady) {
		t.Fatalf("expected ErrNotReady, got %v", err)
	}
}

func openTestDB(t *testing.T, ctx context.Context) (config.Config, *sql.DB) {
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
	return cfg, database
}

func bootstrapTestAdmin(t *testing.T, ctx context.Context, database *sql.DB) account.User {
	t.Helper()
	user, err := account.NewStore(database).BootstrapSuperAdmin(ctx, "admin", "correct horse battery staple")
	if err != nil {
		t.Fatalf("BootstrapSuperAdmin returned error: %v", err)
	}
	return user
}
