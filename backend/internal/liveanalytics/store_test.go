package liveanalytics

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/7grecorder/7grecorder/backend/internal/account"
	"github.com/7grecorder/7grecorder/backend/internal/config"
	databasepkg "github.com/7grecorder/7grecorder/backend/internal/db"
	"github.com/7grecorder/7grecorder/backend/internal/profile"
	"github.com/7grecorder/7grecorder/backend/internal/storagepolicy"
	"github.com/7grecorder/7grecorder/backend/internal/upload"
)

func TestConfigUsesEncryptedOpenLiveCredentialAndOwnership(t *testing.T) {
	root := t.TempDir()
	masterKey := filepath.Join(root, "master.key")
	if err := os.WriteFile(masterKey, []byte("test-master-key"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{DataRoot: root, SQLitePath: filepath.Join(root, "db.sqlite"), TempRoot: filepath.Join(root, "temp"), MasterKeyPath: masterKey}
	if err := databasepkg.Migrate(t.Context(), cfg); err != nil {
		t.Fatal(err)
	}
	database, err := databasepkg.Open(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	admin, err := account.NewStore(database).BootstrapSuperAdmin(t.Context(), "admin", "long-test-password")
	if err != nil {
		t.Fatal(err)
	}
	createdProfile, err := profile.NewStore(database).Create(t.Context(), admin, profile.CreateRequest{Name: "room", RoomID: "1741048619", StreamerName: "Streamer"})
	if err != nil {
		t.Fatal(err)
	}
	credential, err := upload.NewStore(database, cfg).CreateCredential(t.Context(), admin, upload.CredentialCreate{
		Scope: "SYSTEM", Platform: "bilibili_open_live", Purpose: "LIVE_ANALYTICS", AccountLabel: "test project",
		Secret: []byte(`{"access_key_id":"key","access_key_secret":"secret","identity_code":"identity"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	store := NewStore(database, cfg)
	saved, err := store.UpsertConfig(t.Context(), admin, createdProfile.ID, ConfigUpsert{CredentialID: credential.ID, AppID: 1793018783146, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if !saved.Enabled || saved.CredentialID != credential.ID {
		t.Fatalf("unexpected config %#v", saved)
	}
	requests, err := store.EnabledRequests(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(requests) != 1 || requests[0].AccessKeyID != "key" || requests[0].AccessKeySecret != "secret" || requests[0].IdentityCode != "identity" || requests[0].ExpectedRoomID != "1741048619" {
		t.Fatalf("unexpected capture request %#v", requests)
	}
	manager := account.User{ID: 99, Role: account.RoleManager}
	if _, err := store.UpsertConfig(t.Context(), manager, createdProfile.ID, ConfigUpsert{}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected forbidden, got %v", err)
	}

	sessionID, err := store.CreateSession(t.Context(), requests[0], StartResult{
		GameID: "game-1", RoomID: 1741048619, OpenID: "anchor-open-id", Name: "Streamer",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateSession(t.Context(), requests[0], StartResult{GameID: "game-2", RoomID: 1741048619}); err == nil {
		t.Fatal("expected the active-session uniqueness guard to reject a second session")
	}

	writer, err := NewRawWriter(root, createdProfile.ID, sessionID, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	if err := store.SetRawPath(t.Context(), sessionID, writer.RelativePath()); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if err := writer.Write(time.Now(), "LIVE_OPEN_PLATFORM_DM", []byte(`{"cmd":"LIVE_OPEN_PLATFORM_DM","data":{"msg":"sample"}}`)); err != nil {
			t.Fatal(err)
		}
	}
	first, err := store.Events(t.Context(), admin, sessionID, 0, 2)
	if err != nil || len(first.Items) != 2 || !first.HasMore {
		t.Fatalf("first page: %#v %v", first, err)
	}
	second, err := store.Events(t.Context(), admin, sessionID, first.NextOffset, 2)
	if err != nil || len(second.Items) != 1 || second.HasMore {
		t.Fatalf("second page: %#v %v", second, err)
	}
	if _, err := store.Events(t.Context(), admin, sessionID, 1, 2); !errors.Is(err, ErrValidation) {
		t.Fatalf("invalid cursor: %v", err)
	}
	if _, err := store.Events(t.Context(), manager, sessionID, 0, 2); !errors.Is(err, ErrForbidden) {
		t.Fatalf("ownership: %v", err)
	}
	if err := store.MarkConnected(t.Context(), sessionID); err != nil {
		t.Fatal(err)
	}
	lastEvent := time.Date(2026, 9, 19, 2, 3, 4, 0, time.UTC)
	if err := store.UpdateStats(t.Context(), sessionID, map[string]int{"LIVE_OPEN_PLATFORM_DM": 3}, 3, 0, 2, lastEvent); err != nil {
		t.Fatal(err)
	}
	if err := store.flushStats(t.Context(), sessionID, map[string]int{"LIVE_OPEN_PLATFORM_DM": 3}, 3, 0, 2, lastEvent,
		map[string]map[string]int64{"2026-09-19T02:03:00Z": {"LIVE_OPEN_PLATFORM_DM": 2}}); err != nil {
		t.Fatal(err)
	}
	if err := store.flushStats(t.Context(), sessionID, map[string]int{"LIVE_OPEN_PLATFORM_DM": 3}, 3, 0, 2, lastEvent,
		map[string]map[string]int64{"2026-09-19T02:03:00Z": {"LIVE_OPEN_PLATFORM_DM": 1}}); err != nil {
		t.Fatal(err)
	}
	timeline, err := store.Timeline(t.Context(), admin, sessionID)
	if err != nil || len(timeline.Items) != 1 || timeline.Items[0].EventCount != 3 {
		t.Fatalf("timeline %#v %v", timeline, err)
	}
	if _, err := store.Timeline(t.Context(), manager, sessionID); !errors.Is(err, ErrForbidden) {
		t.Fatalf("timeline permission %v", err)
	}
	if err := store.InterruptActive(t.Context()); err != nil {
		t.Fatal(err)
	}
	session, err := store.GetSession(t.Context(), admin, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if session.Status != "INTERRUPTED" || session.EventCount != 3 || session.EventCounts["LIVE_OPEN_PLATFORM_DM"] != 3 || session.GapCount != 3 || session.LastEventAt == "" {
		t.Fatalf("unexpected interrupted capture session %#v", session)
	}
	t.Run("historical source is matched before recent limit", func(t *testing.T) {
		if _, err := database.ExecContext(t.Context(), `
            UPDATE live_capture_sessions SET started_at='2026-09-01 00:00:00',ended_at='2026-09-01 01:00:00' WHERE id = ?;
            INSERT INTO upload_sources(id,recording_profile_id,source_key,source_room_id,streamer_name_snapshot,started_at,completed_at,status)
            VALUES(1,?,'old-source','1741048619','test','2026-09-01T00:00:00Z','2026-09-01T01:00:00Z','READY_TO_UPLOAD');
            WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x < 101)
            INSERT INTO live_capture_sessions(recording_profile_id,status,started_at,ended_at)
            SELECT ?,'ENDED','2026-09-18 00:00:00','2026-09-18 01:00:00' FROM n;
        `, sessionID, createdProfile.ID, createdProfile.ID); err != nil {
			t.Fatal(err)
		}
		items, err := store.ListSessions(t.Context(), admin, createdProfile.ID, 1)
		if err != nil || len(items) != 1 || items[0].ID != sessionID {
			t.Fatalf("historical match lost: %#v %v", items, err)
		}
		if _, err := store.ListSessions(t.Context(), manager, createdProfile.ID, 1); !errors.Is(err, ErrForbidden) {
			t.Fatalf("ownership error: %v", err)
		}
		if _, err := store.ListSessions(t.Context(), admin, createdProfile.ID, 999); !errors.Is(err, ErrNotFound) {
			t.Fatalf("unknown source error: %v", err)
		}
	})

}

func TestRawQuotaDeletesOldestEndedSessionAndProtectsActiveSession(t *testing.T) {
	root := t.TempDir()
	masterKey := filepath.Join(root, "master.key")
	if err := os.WriteFile(masterKey, []byte("test-master-key"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{DataRoot: root, SQLitePath: filepath.Join(root, "db.sqlite"), TempRoot: filepath.Join(root, "temp"), MasterKeyPath: masterKey}
	if err := databasepkg.Migrate(t.Context(), cfg); err != nil {
		t.Fatal(err)
	}
	database, err := databasepkg.Open(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	admin, err := account.NewStore(database).BootstrapSuperAdmin(t.Context(), "admin", "long-test-password")
	if err != nil {
		t.Fatal(err)
	}
	createdProfile, err := profile.NewStore(database).Create(t.Context(), admin, profile.CreateRequest{Name: "room", RoomID: "1741048619", StreamerName: "Streamer"})
	if err != nil {
		t.Fatal(err)
	}
	store := NewStore(database, cfg)
	request := CaptureRequest{RecordingProfileID: createdProfile.ID, ExpectedRoomID: createdProfile.RoomID}
	oldID, err := store.CreateSession(t.Context(), request, StartResult{GameID: "old", RoomID: 1741048619})
	if err != nil {
		t.Fatal(err)
	}
	oldWriter, err := NewRawWriter(root, createdProfile.ID, oldID, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	oldPath := oldWriter.RelativePath()
	if err := oldWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := store.SetRawPath(t.Context(), oldID, oldPath); err != nil {
		t.Fatal(err)
	}
	if err := store.FinishSession(t.Context(), oldID, "ENDED", ""); err != nil {
		t.Fatal(err)
	}

	activeID, err := store.CreateSession(t.Context(), request, StartResult{GameID: "active", RoomID: 1741048619})
	if err != nil {
		t.Fatal(err)
	}
	activeWriter, err := NewRawWriter(root, createdProfile.ID, activeID, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	activePath := activeWriter.RelativePath()
	if err := activeWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := store.SetRawPath(t.Context(), activeID, activePath); err != nil {
		t.Fatal(err)
	}

	fileSize := storagepolicy.LiveAnalyticsRawBytes*3/5 + 1
	if err := os.Truncate(filepath.Join(root, filepath.FromSlash(oldPath)), fileSize); err != nil {
		t.Fatal(err)
	}
	if err := os.Truncate(filepath.Join(root, filepath.FromSlash(activePath)), fileSize); err != nil {
		t.Fatal(err)
	}
	result, err := store.EnforceRawQuota(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if result.DeletedFiles != 1 || result.DeletedBytes != fileSize || result.AfterBytes != fileSize {
		t.Fatalf("unexpected raw cleanup result %#v", result)
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(oldPath))); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected oldest ended raw file deleted, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(activePath))); err != nil {
		t.Fatalf("active raw file was not protected: %v", err)
	}
	oldSession, err := store.GetSession(t.Context(), admin, oldID)
	if err != nil {
		t.Fatal(err)
	}
	if oldSession.RawStatus != "DELETED" || oldSession.RawSizeBytes != fileSize || oldSession.RawDeletedAt == "" {
		t.Fatalf("unexpected cleaned session metadata %#v", oldSession)
	}
	if _, err := database.ExecContext(t.Context(), `UPDATE live_capture_raw_files SET status = 'DELETING', deleted_at = NULL WHERE session_id = ?`, oldID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.EnforceRawQuota(t.Context()); err != nil {
		t.Fatal(err)
	}
	recovered, err := store.GetSession(t.Context(), admin, oldID)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.RawStatus != "DELETED" || recovered.RawDeletedAt == "" {
		t.Fatalf("interrupted cleanup claim was not recovered: %#v", recovered)
	}
}

func TestActiveSessionClosedChunkCanBeReclaimed(t *testing.T) {
	root := t.TempDir()
	cfg := config.Config{DataRoot: root, SQLitePath: filepath.Join(root, "test.db"), TempRoot: filepath.Join(root, "temp")}
	if err := databasepkg.Migrate(t.Context(), cfg); err != nil {
		t.Fatal(err)
	}
	database, err := databasepkg.Open(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	admin, err := account.NewStore(database).BootstrapSuperAdmin(t.Context(), "admin", "correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	p, err := profile.NewStore(database).Create(t.Context(), admin, profile.CreateRequest{Name: "test", RoomID: "123", StreamerName: "test"})
	if err != nil {
		t.Fatal(err)
	}
	store := NewStore(database, cfg)
	id, err := store.CreateSession(t.Context(), CaptureRequest{RecordingProfileID: p.ID}, StartResult{RoomID: 123})
	if err != nil {
		t.Fatal(err)
	}
	writer, err := NewRawWriter(root, p.ID, id, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	first := writer.RelativePath()
	if err := store.SetRawPath(t.Context(), id, first); err != nil {
		t.Fatal(err)
	}
	if err := writer.Rotate(time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := store.SetRawPath(t.Context(), id, writer.RelativePath()); err != nil {
		t.Fatal(err)
	}
	// Sparse logical size exercises the quota without allocating gigabytes.
	if err := os.Truncate(filepath.Join(root, first), storagepolicy.LiveAnalyticsRawBytes+1); err != nil {
		t.Fatal(err)
	}
	result, err := store.EnforceRawQuota(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if result.DeletedFiles != 1 {
		t.Fatalf("closed part not reclaimed: %#v", result)
	}
	files, err := store.RawFiles(t.Context(), admin, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(files.Items) != 2 || files.Items[0].Status != "WRITING" || files.Items[1].Status != "DELETED" {
		t.Fatalf("unexpected files: %#v", files)
	}
	if _, err := store.Events(t.Context(), admin, id, 0, 10, files.Items[1].ID); !errors.Is(err, ErrEvidenceUnavailable) {
		t.Fatalf("deleted part error: %v", err)
	}
	if _, err := store.Events(t.Context(), admin, id, 0, 10, files.Items[0].ID+100); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign part error: %v", err)
	}
	if err := writer.Write(time.Now(), "CONTINUE", json.RawMessage(`{"cmd":"CONTINUE"}`)); err != nil {
		t.Fatal(err)
	}
	page, err := store.Events(t.Context(), admin, id, 0, 10, files.Items[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].CMD != "CONTINUE" {
		t.Fatalf("active writer did not continue: %#v", page)
	}
	session, err := store.GetSession(t.Context(), admin, id)
	if err != nil {
		t.Fatal(err)
	}
	if session.Status != "STARTING" || session.RawStatus != "WRITING" {
		t.Fatalf("rotation changed live session: %#v", session)
	}
}
