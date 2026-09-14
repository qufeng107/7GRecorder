package songs_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/7grecorder/7grecorder/backend/internal/account"
	"github.com/7grecorder/7grecorder/backend/internal/config"
	"github.com/7grecorder/7grecorder/backend/internal/db"
	"github.com/7grecorder/7grecorder/backend/internal/job"
	"github.com/7grecorder/7grecorder/backend/internal/profile"
	"github.com/7grecorder/7grecorder/backend/internal/songs"
	"github.com/7grecorder/7grecorder/backend/internal/upload"
)

func TestCreateRunSnapshotsAvailableCOSVideoAndSchedulesDownload(t *testing.T) {
	ctx := context.Background()
	cfg, database, actor := openSongsTestDB(t, ctx)
	uploadStore := upload.NewStore(database, cfg)
	cosCredential, err := uploadStore.CreateCredential(ctx, actor, upload.CredentialCreate{
		Scope: "SYSTEM", Platform: "tencent_cos", Purpose: "STORAGE", AccountLabel: "cos",
		Secret: []byte(`{"secret_id":"id","secret_key":"key"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	acrCredential, err := uploadStore.CreateCredential(ctx, actor, upload.CredentialCreate{
		Scope: "SYSTEM", Platform: "acrcloud", Purpose: "SONG_RECOGNITION", AccountLabel: "acr",
		Secret: []byte(`{"access_token":"token"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := uploadStore.UpsertCOSConfig(ctx, actor, 1, upload.COSConfigUpsert{
		CredentialID: cosCredential.ID, Enabled: true, Region: "ap-shanghai", Bucket: "bucket-1250000000",
		Prefix: "archive/", MaxManagedBytes: 1 << 30,
	}); err != nil {
		t.Fatal(err)
	}
	insertSongSource(t, ctx, database, "archive/videos/source.flv", "AVAILABLE")

	store := songs.NewStore(database, cfg)
	if _, err := store.UpsertSettings(ctx, actor, songs.SettingsUpsert{
		Enabled: true, CredentialID: acrCredential.ID, Region: "eu-west-1", ContainerID: "container",
		DestinationCOSStorageProfileID: 1, SongsPrefix: "/songs/", BoundaryPaddingMs: 500, AlgorithmVersion: "v1",
	}); err != nil {
		t.Fatal(err)
	}
	sources, err := store.ListSources(ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 1 || sources[0].TimelineEndMs != 1800000 {
		t.Fatalf("unexpected sources: %#v", sources)
	}

	run, err := store.CreateRun(ctx, actor, songs.CreateRunRequest{COSObjectID: 1})
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != "PENDING" || run.SourceObjectKey != "archive/videos/source.flv" || run.SourceSizeBytes != 50 {
		t.Fatalf("unexpected run: %#v", run)
	}
	var jobID int64
	var jobType, resourceClass, businessKey string
	if err := database.QueryRowContext(ctx, `SELECT id, type, resource_class, business_key FROM jobs WHERE payload_json = ?`,
		`{"analysis_run_id":1}`).Scan(&jobID, &jobType, &resourceClass, &businessKey); err != nil {
		t.Fatal(err)
	}
	if jobType != "DOWNLOAD_SONG_SOURCE" || resourceClass != "NETWORK" || businessKey != "song-analysis:1:download-source" {
		t.Fatalf("unexpected job: %s %s %s", jobType, resourceClass, businessKey)
	}
	if _, err := database.ExecContext(ctx, `INSERT INTO local_storage_settings
		(id, max_recording_bytes, min_system_free_bytes, cleanup_target_ratio, absolute_emergency_free_bytes, updated_by_user_id)
		VALUES (1, 49, 1, 0.85, 1, ?)`, actor.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.ReserveSourceDownload(ctx, jobID, run.ID); err != songs.ErrWaitingForSpace {
		t.Fatalf("expected managed-space wait, got %v", err)
	}
	if _, err := database.ExecContext(ctx, `UPDATE local_storage_settings SET max_recording_bytes = 1000 WHERE id = 1`); err != nil {
		t.Fatal(err)
	}
	if err := store.ReserveSourceDownload(ctx, jobID, run.ID); err != nil {
		t.Fatalf("reserve source: %v", err)
	}
	var reserved int64
	if err := database.QueryRowContext(ctx, `SELECT reserved_bytes FROM storage_reservations WHERE job_id = ?`, jobID).Scan(&reserved); err != nil {
		t.Fatal(err)
	}
	if reserved != 50 {
		t.Fatalf("expected 50 reserved bytes, got %d", reserved)
	}
	if _, err := database.ExecContext(ctx, `UPDATE jobs SET status = 'FAILED' WHERE id = ?`, jobID); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `UPDATE song_analysis_runs SET status = 'FAILED' WHERE id = ?`, run.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := job.NewStore(database).Retry(ctx, actor, jobID, job.RetryRequest{}); err != nil {
		t.Fatalf("retry song job: %v", err)
	}
	var runStatus string
	if err := database.QueryRowContext(ctx, `SELECT status FROM song_analysis_runs WHERE id = ?`, run.ID).Scan(&runStatus); err != nil {
		t.Fatal(err)
	}
	if runStatus != "PENDING" {
		t.Fatalf("expected retried run pending, got %s", runStatus)
	}
	if _, err := job.NewStore(database).Cancel(ctx, actor, jobID); err != nil {
		t.Fatalf("cancel song job: %v", err)
	}
	if err := database.QueryRowContext(ctx, `SELECT status FROM song_analysis_runs WHERE id = ?`, run.ID).Scan(&runStatus); err != nil {
		t.Fatal(err)
	}
	if runStatus != "CANCELLED" {
		t.Fatalf("expected cancelled run, got %s", runStatus)
	}
}

func TestSourcesExcludeNonVideoAndUnavailableObjects(t *testing.T) {
	ctx := context.Background()
	cfg, database, actor := openSongsTestDB(t, ctx)
	uploadStore := upload.NewStore(database, cfg)
	cosCredential, err := uploadStore.CreateCredential(ctx, actor, upload.CredentialCreate{
		Scope: "SYSTEM", Platform: "tencent_cos", Purpose: "STORAGE", AccountLabel: "cos",
		Secret: []byte(`{"secret_id":"id","secret_key":"key"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := uploadStore.UpsertCOSConfig(ctx, actor, 1, upload.COSConfigUpsert{
		CredentialID: cosCredential.ID, Enabled: true, Region: "ap-shanghai", Bucket: "bucket-1250000000",
		Prefix: "archive/", MaxManagedBytes: 1 << 30,
	}); err != nil {
		t.Fatal(err)
	}
	insertSongSource(t, ctx, database, "archive/source.xml", "AVAILABLE")
	insertSecondSongSource(t, ctx, database, "archive/source.mp4", "FAILED")
	items, err := songs.NewStore(database, cfg).ListSources(ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("expected no selectable sources, got %#v", items)
	}
	if _, err := songs.NewStore(database, cfg).ListSources(ctx, account.User{Role: account.RoleManager}); err != songs.ErrForbidden {
		t.Fatalf("expected forbidden, got %v", err)
	}
}

func openSongsTestDB(t *testing.T, ctx context.Context) (config.Config, *sql.DB, account.User) {
	t.Helper()
	root := t.TempDir()
	keyPath := filepath.Join(root, "master.key")
	if err := os.WriteFile(keyPath, []byte("songs-test-master-key"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{DataRoot: root, TempRoot: filepath.Join(root, "temp"), SQLitePath: filepath.Join(root, "db.sqlite"), MasterKeyPath: keyPath}
	if err := db.Migrate(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	database, err := db.Open(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	actor, err := account.NewStore(database).BootstrapSuperAdmin(ctx, "admin", "correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := profile.NewStore(database).Create(ctx, actor, profile.CreateRequest{Name: "7G", RoomID: "1", StreamerName: "Singer"}); err != nil {
		t.Fatal(err)
	}
	return cfg, database, actor
}

func insertSongSource(t *testing.T, ctx context.Context, database *sql.DB, objectKey, status string) {
	t.Helper()
	if _, err := database.ExecContext(ctx, `INSERT INTO upload_sources
		(id, recording_profile_id, source_key, source_room_id, streamer_name_snapshot, started_at, completed_at,
		duration_ms, status, total_bytes, recording_count, file_count, max_gap_seconds, merge_gap_threshold_seconds)
		VALUES (1, 1, 'source:1', '1', 'Singer', '2026-09-13T10:00:00Z', '2026-09-13T10:30:00Z',
		1800000, 'READY_TO_UPLOAD', 50, 1, 1, 0, 600)`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `INSERT INTO upload_source_outputs
		(id, upload_source_id, sort_order, relative_path, size_bytes, duration_ms, timeline_start_ms, timeline_end_ms, status)
		VALUES (1, 1, 0, 'upload-sources/1/source.flv', 50, 1800000, 0, 1800000, 'READY_TO_UPLOAD')`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `INSERT INTO upload_source_cos_objects
		(id, cos_storage_profile_id, recording_profile_id, upload_source_id, upload_source_output_id, object_key,
		size_bytes, source_size_bytes, compression_status, etag, status)
		VALUES (1, 1, 1, 1, 1, ?, 50, 50, 'DISABLED', 'etag-1', ?)`, objectKey, status); err != nil {
		t.Fatal(err)
	}
}

func insertSecondSongSource(t *testing.T, ctx context.Context, database *sql.DB, objectKey, status string) {
	t.Helper()
	if _, err := database.ExecContext(ctx, `INSERT INTO upload_sources
		(id, recording_profile_id, source_key, source_room_id, streamer_name_snapshot, started_at, completed_at,
		duration_ms, status, total_bytes, recording_count, file_count, max_gap_seconds, merge_gap_threshold_seconds)
		VALUES (2, 1, 'source:2', '1', 'Singer', '2026-09-13T11:00:00Z', '2026-09-13T11:30:00Z',
		1800000, 'READY_TO_UPLOAD', 50, 1, 1, 0, 600)`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `INSERT INTO upload_source_outputs
		(id, upload_source_id, sort_order, relative_path, size_bytes, duration_ms, timeline_start_ms, timeline_end_ms, status)
		VALUES (2, 2, 0, 'upload-sources/1/source.mp4', 50, 1800000, 0, 1800000, 'READY_TO_UPLOAD')`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `INSERT INTO upload_source_cos_objects
		(id, cos_storage_profile_id, recording_profile_id, upload_source_id, upload_source_output_id, object_key,
		size_bytes, source_size_bytes, compression_status, etag, status)
		VALUES (2, 1, 1, 2, 2, ?, 50, 50, 'DISABLED', 'etag-2', ?)`, objectKey, status); err != nil {
		t.Fatal(err)
	}
}
