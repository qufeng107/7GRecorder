package worker

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/7grecorder/7grecorder/backend/internal/account"
	"github.com/7grecorder/7grecorder/backend/internal/config"
	"github.com/7grecorder/7grecorder/backend/internal/db"
	"github.com/7grecorder/7grecorder/backend/internal/profile"
	"github.com/7grecorder/7grecorder/backend/internal/recorder"
)

type fakeRecorder struct {
	desired recorder.DesiredProfile
	status  recorder.RuntimeStatus
	err     error
}

func (f *fakeRecorder) SyncProfile(_ context.Context, desired recorder.DesiredProfile) (recorder.RuntimeStatus, error) {
	f.desired = desired
	return f.status, f.err
}

func TestRunOnceSyncsPendingRecorderProfile(t *testing.T) {
	ctx := context.Background()
	database := openTestDB(t, ctx)
	actor := bootstrapTestAdmin(t, ctx, database)
	created, err := profile.NewStore(database).Create(ctx, actor, profile.CreateRequest{
		Name:         "7G Live",
		RoomID:       "1741048619",
		StreamerName: "7G",
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	recorderClient := &fakeRecorder{
		status: recorder.RuntimeStatus{StreamStatus: "LIVE", RecorderStatus: "RECORDING"},
	}
	if err := New(database, recorderClient).RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce returned error: %v", err)
	}
	if recorderClient.desired.ProfileID != created.ID || recorderClient.desired.RoomID != "1741048619" {
		t.Fatalf("unexpected desired profile: %#v", recorderClient.desired)
	}

	var jobStatus string
	var syncStatus string
	var streamStatus string
	var recorderStatus string
	err = database.QueryRowContext(ctx, `
		SELECT j.status, r.sync_status, r.stream_status, r.recorder_status
		FROM jobs j
		JOIN recording_profile_runtime r ON r.recording_profile_id = j.recording_profile_id
		WHERE j.recording_profile_id = ?
	`, created.ID).Scan(&jobStatus, &syncStatus, &streamStatus, &recorderStatus)
	if err != nil {
		t.Fatalf("query runtime returned error: %v", err)
	}
	if jobStatus != "SUCCEEDED" || syncStatus != "SYNCED" || streamStatus != "LIVE" || recorderStatus != "RECORDING" {
		t.Fatalf("unexpected statuses job=%s sync=%s stream=%s recorder=%s", jobStatus, syncStatus, streamStatus, recorderStatus)
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
