package liveanalytics

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/7grecorder/7grecorder/backend/internal/account"
	"github.com/7grecorder/7grecorder/backend/internal/config"
	databasepkg "github.com/7grecorder/7grecorder/backend/internal/db"
	"github.com/7grecorder/7grecorder/backend/internal/profile"
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
	if err := store.MarkConnected(t.Context(), sessionID); err != nil {
		t.Fatal(err)
	}
	lastEvent := time.Date(2026, 9, 19, 2, 3, 4, 0, time.UTC)
	if err := store.UpdateStats(t.Context(), sessionID, map[string]int{"LIVE_OPEN_PLATFORM_DM": 3}, 3, 0, 2, lastEvent); err != nil {
		t.Fatal(err)
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
}
