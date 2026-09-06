package worker

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/7grecorder/7grecorder/backend/internal/account"
	"github.com/7grecorder/7grecorder/backend/internal/config"
	"github.com/7grecorder/7grecorder/backend/internal/db"
	"github.com/7grecorder/7grecorder/backend/internal/media"
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

type fakeMerger struct {
	request media.MergeRequest
	result  media.MergeResult
	err     error
}

func (f *fakeMerger) Merge(_ context.Context, request media.MergeRequest) (media.MergeResult, error) {
	f.request = request
	return f.result, f.err
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

func TestRunOnceMergesPendingUploadSource(t *testing.T) {
	ctx := context.Background()
	cfg, database := openTestDBWithConfig(t, ctx)
	actor := bootstrapTestAdmin(t, ctx, database)
	created, err := profile.NewStore(database).Create(ctx, actor, profile.CreateRequest{
		Name:         "7G Live",
		RoomID:       "1741048619",
		StreamerName: "7G",
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if _, err := database.ExecContext(ctx, `UPDATE jobs SET status = 'SUCCEEDED' WHERE type = 'SYNC_RECORDER_PROFILE'`); err != nil {
		t.Fatalf("complete initial sync job returned error: %v", err)
	}
	if _, err := database.ExecContext(ctx, `
		INSERT INTO recordings
			(id, recording_profile_id, title, started_at, completed_at, duration_ms, recording_status, local_storage_status, source_room_id, streamer_name_snapshot)
		VALUES
			(1, ?, 'part 1', '2026-09-05T10:00:00Z', '2026-09-05T10:03:00Z', 180000, 'COMPLETED', 'AVAILABLE', '1741048619', '7G'),
			(2, ?, 'part 2', '2026-09-05T10:04:00Z', '2026-09-05T10:07:00Z', 180000, 'COMPLETED', 'AVAILABLE', '1741048619', '7G')
	`, created.ID, created.ID); err != nil {
		t.Fatalf("insert recordings returned error: %v", err)
	}
	if _, err := database.ExecContext(ctx, `
		INSERT INTO recording_files
			(id, recording_id, relative_path, original_name, kind, file_status, size_bytes, duration_ms, closed_at)
		VALUES
			(1, 1, 'recordings/1741048619-7G/part1.flv', 'part1.flv', 'video', 'CLOSED', 20, 180000, '2026-09-05T10:03:00Z'),
			(2, 2, 'recordings/1741048619-7G/part2.flv', 'part2.flv', 'video', 'CLOSED', 30, 180000, '2026-09-05T10:07:00Z')
	`); err != nil {
		t.Fatalf("insert recording files returned error: %v", err)
	}
	if _, err := database.ExecContext(ctx, `
		INSERT INTO upload_sources
			(id, recording_profile_id, source_key, source_room_id, streamer_name_snapshot, started_at, completed_at, duration_ms, status, total_bytes, recording_count, file_count, max_gap_seconds, merge_gap_threshold_seconds)
		VALUES
			(1, ?, 'profile:1:1:2', '1741048619', '7G', '2026-09-05T10:00:00Z', '2026-09-05T10:07:00Z', 360000, 'MERGE_PENDING', 50, 2, 2, 60, 600)
	`, created.ID); err != nil {
		t.Fatalf("insert upload source returned error: %v", err)
	}
	if _, err := database.ExecContext(ctx, `
		INSERT INTO upload_source_segments
			(upload_source_id, recording_id, recording_file_id, sort_order, source_started_at, source_completed_at, timeline_start_ms, timeline_end_ms, relative_path, size_bytes, duration_ms)
		VALUES
			(1, 1, 1, 0, '2026-09-05T10:00:00Z', '2026-09-05T10:03:00Z', 0, 180000, 'recordings/1741048619-7G/part1.flv', 20, 180000),
			(1, 2, 2, 1, '2026-09-05T10:04:00Z', '2026-09-05T10:07:00Z', 180000, 360000, 'recordings/1741048619-7G/part2.flv', 30, 180000)
	`); err != nil {
		t.Fatalf("insert upload source segments returned error: %v", err)
	}
	if _, err := database.ExecContext(ctx, `
		INSERT INTO jobs
			(recording_profile_id, type, resource_class, business_key, payload_json, status, priority, max_attempts)
		VALUES
			(?, 'MERGE_UPLOAD_SOURCE', 'MEDIA', 'upload-source:1:merge', '{"upload_source_id":1}', 'PENDING', 60, 3)
	`, created.ID); err != nil {
		t.Fatalf("insert merge job returned error: %v", err)
	}

	merger := &fakeMerger{result: media.MergeResult{RelativePath: "upload-sources/1/1/upload-source-1.flv", SizeBytes: 45}}
	if err := NewWithMerger(database, &fakeRecorder{}, cfg, merger).RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce returned error: %v", err)
	}
	if len(merger.request.Segments) != 2 || merger.request.OutputRelativePath != "upload-sources/1/1/upload-source-1.flv" {
		t.Fatalf("unexpected merge request: %#v", merger.request)
	}

	var sourceStatus string
	var outputPath string
	var totalBytes int64
	if err := database.QueryRowContext(ctx, `
		SELECT status, COALESCE(output_relative_path, ''), total_bytes
		FROM upload_sources
		WHERE id = 1
	`).Scan(&sourceStatus, &outputPath, &totalBytes); err != nil {
		t.Fatalf("query upload source returned error: %v", err)
	}
	if sourceStatus != "READY_TO_UPLOAD" || outputPath != "upload-sources/1/1/upload-source-1.flv" || totalBytes != 45 {
		t.Fatalf("unexpected upload source result: status=%s output=%s bytes=%d", sourceStatus, outputPath, totalBytes)
	}

	var jobStatus string
	if err := database.QueryRowContext(ctx, `SELECT status FROM jobs WHERE business_key = 'upload-source:1:merge'`).Scan(&jobStatus); err != nil {
		t.Fatalf("query job returned error: %v", err)
	}
	if jobStatus != "SUCCEEDED" {
		t.Fatalf("unexpected job status: %s", jobStatus)
	}
}

func openTestDB(t *testing.T, ctx context.Context) *sql.DB {
	t.Helper()
	_, database := openTestDBWithConfig(t, ctx)
	return database
}

func openTestDBWithConfig(t *testing.T, ctx context.Context) (config.Config, *sql.DB) {
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
