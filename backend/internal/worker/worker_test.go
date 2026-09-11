package worker

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/7grecorder/7grecorder/backend/internal/account"
	"github.com/7grecorder/7grecorder/backend/internal/config"
	"github.com/7grecorder/7grecorder/backend/internal/db"
	"github.com/7grecorder/7grecorder/backend/internal/media"
	"github.com/7grecorder/7grecorder/backend/internal/profile"
	"github.com/7grecorder/7grecorder/backend/internal/recorder"
	"github.com/7grecorder/7grecorder/backend/internal/upload"
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

type fakeCOSUploader struct {
	request upload.COSUploadRequest
	result  upload.COSUploadResult
	err     error
}

type fakeBilibiliUploader struct {
	request upload.BilibiliUploadRequest
	result  upload.BilibiliUploadResult
	err     error
}

type fakePackager struct {
	request        media.PackageRequest
	segmentRequest media.SegmentPackageRequest
	result         media.PackageResult
	err            error
}

type fakeCompressor struct {
	request media.CompressionRequest
	result  media.CompressionResult
	err     error
}

type fakeEditor struct {
	request media.EditRequest
	result  media.PackageResult
	err     error
}

func (f *fakePackager) Package(_ context.Context, request media.PackageRequest) (media.PackageResult, error) {
	f.request = request
	return f.result, f.err
}

func (f *fakePackager) PackageSegments(_ context.Context, request media.SegmentPackageRequest) (media.PackageResult, error) {
	f.segmentRequest = request
	return f.result, f.err
}

func (f *fakeCompressor) Compress(_ context.Context, request media.CompressionRequest) (media.CompressionResult, error) {
	f.request = request
	return f.result, f.err
}

func (f *fakeEditor) ApplyCuts(_ context.Context, request media.EditRequest) (media.PackageResult, error) {
	f.request = request
	return f.result, f.err
}

func (f *fakeCOSUploader) Upload(ctx context.Context, request upload.COSUploadRequest, progress upload.ProgressReporter) (upload.COSUploadResult, error) {
	f.request = request
	if progress != nil {
		progress(ctx, upload.UploadProgress{CurrentBytes: request.SourceSizeBytes, TotalBytes: request.SourceSizeBytes, Message: "test cos upload"})
	}
	return f.result, f.err
}

func (f *fakeBilibiliUploader) Upload(ctx context.Context, request upload.BilibiliUploadRequest, progress upload.ProgressReporter) (upload.BilibiliUploadResult, error) {
	f.request = request
	if progress != nil {
		var total int64
		for _, part := range request.Parts {
			total += part.SizeBytes
		}
		progress(ctx, upload.UploadProgress{CurrentBytes: total, TotalBytes: total, Message: "test bilibili upload"})
	}
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

	packager := &fakePackager{result: media.PackageResult{Outputs: []media.PackageOutput{
		{RelativePath: "upload-sources/1/1/parts/7G Live-20260905-\u7b2c01\u573a\u76f4\u64ad-p01.flv", SizeBytes: 20, DurationMs: 180000, TimelineStartMs: 0, TimelineEndMs: 180000},
		{RelativePath: "upload-sources/1/1/parts/7G Live-20260905-\u7b2c01\u573a\u76f4\u64ad-p02.flv", SizeBytes: 30, DurationMs: 180000, TimelineStartMs: 180000, TimelineEndMs: 360000},
	}}}
	if err := NewWithPackager(database, &fakeRecorder{}, cfg, packager).RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce returned error: %v", err)
	}
	if len(packager.segmentRequest.Segments) != 2 || packager.segmentRequest.OutputDirRelativePath != "upload-sources/1/1/parts" {
		t.Fatalf("unexpected segment package request: %#v", packager.segmentRequest)
	}
	if packager.segmentRequest.OutputBaseName != "7G Live-20260905-\u7b2c01\u573a\u76f4\u64ad" {
		t.Fatalf("unexpected segment package output base name: %q", packager.segmentRequest.OutputBaseName)
	}

	var sourceStatus string
	var outputPath string
	var totalBytes int64
	var outputCount int
	if err := database.QueryRowContext(ctx, `
		SELECT us.status, COALESCE(us.output_relative_path, ''), us.total_bytes, COUNT(uso.id)
		FROM upload_sources us
		LEFT JOIN upload_source_outputs uso ON uso.upload_source_id = us.id
		WHERE us.id = 1
		GROUP BY us.id
	`).Scan(&sourceStatus, &outputPath, &totalBytes, &outputCount); err != nil {
		t.Fatalf("query upload source returned error: %v", err)
	}
	if sourceStatus != "READY_TO_UPLOAD" || outputPath != "" || totalBytes != 50 || outputCount != 2 {
		t.Fatalf("unexpected upload source result: status=%s output=%s bytes=%d outputs=%d", sourceStatus, outputPath, totalBytes, outputCount)
	}

	var jobStatus string
	if err := database.QueryRowContext(ctx, `SELECT status FROM jobs WHERE business_key = 'upload-source:1:merge'`).Scan(&jobStatus); err != nil {
		t.Fatalf("query job returned error: %v", err)
	}
	if jobStatus != "SUCCEEDED" {
		t.Fatalf("unexpected job status: %s", jobStatus)
	}
}

func TestRunOncePackagesUploadSource(t *testing.T) {
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
		INSERT INTO upload_sources
			(id, recording_profile_id, source_key, source_room_id, streamer_name_snapshot,
				started_at, completed_at, duration_ms, status, output_relative_path,
				total_bytes, recording_count, file_count, max_gap_seconds, merge_gap_threshold_seconds)
		VALUES (1, ?, 'profile:1:1:1', '1741048619', '7G',
			'2026-09-05T10:00:00Z', '2026-09-05T15:30:00Z', 19800000, 'PACKAGE_PENDING',
			'upload-sources/1/1/upload-source-1.flv', 8000000000, 4, 4, 60, 600)
	`, created.ID); err != nil {
		t.Fatalf("insert upload source returned error: %v", err)
	}
	if _, err := database.ExecContext(ctx, `
		INSERT INTO jobs
			(recording_profile_id, upload_source_id, type, resource_class, business_key, payload_json, status, priority, max_attempts)
		VALUES
			(?, 1, 'PACKAGE_UPLOAD_SOURCE', 'MEDIA', 'upload-source:1:package', '{"upload_source_id":1}', 'PENDING', 65, 3)
	`, created.ID); err != nil {
		t.Fatalf("insert package job returned error: %v", err)
	}
	packager := &fakePackager{result: media.PackageResult{Outputs: []media.PackageOutput{
		{RelativePath: "upload-sources/1/1/parts/upload-source-1-part-000.flv", SizeBytes: 3900000000, DurationMs: 7200000, TimelineStartMs: 0, TimelineEndMs: 7200000},
		{RelativePath: "upload-sources/1/1/parts/upload-source-1-part-001.flv", SizeBytes: 4100000000, DurationMs: 12600000, TimelineStartMs: 7200000, TimelineEndMs: 19800000},
	}}}
	if err := NewWithPackager(database, &fakeRecorder{}, cfg, packager).RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce returned error: %v", err)
	}
	if packager.request.UploadSourceID != 1 || packager.request.MaxPartBytes != cfg.UploadMaxPartBytes {
		t.Fatalf("unexpected package request: %#v", packager.request)
	}
	if packager.request.OutputBaseName != "7G Live-20260905-\u7b2c01\u573a\u76f4\u64ad" {
		t.Fatalf("unexpected package output base name: %q", packager.request.OutputBaseName)
	}
	var status string
	var outputCount int
	if err := database.QueryRowContext(ctx, `
		SELECT us.status, COUNT(uso.id)
		FROM upload_sources us
		LEFT JOIN upload_source_outputs uso ON uso.upload_source_id = us.id
		WHERE us.id = 1
		GROUP BY us.id
	`).Scan(&status, &outputCount); err != nil {
		t.Fatalf("query package result returned error: %v", err)
	}
	if status != "READY_TO_UPLOAD" || outputCount != 2 {
		t.Fatalf("unexpected package result status=%s outputs=%d", status, outputCount)
	}
}

func TestRunOnceAppliesUploadSourceEdit(t *testing.T) {
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
		INSERT INTO upload_sources
			(id, recording_profile_id, source_key, source_room_id, streamer_name_snapshot,
				started_at, completed_at, duration_ms, status,
				total_bytes, recording_count, file_count, max_gap_seconds, merge_gap_threshold_seconds,
				ready_at, review_status, edit_decision_json)
		VALUES (1, ?, 'profile:1:1:1', '1741048619', '7G',
			'2026-09-05T10:00:00Z', '2026-09-05T10:30:00Z', 1800000, 'READY_TO_UPLOAD',
			50, 1, 1, 0, 600, CURRENT_TIMESTAMP, 'REQUIRED',
			'{"cuts":[{"start_ms":60000,"end_ms":120000}]}');
		INSERT INTO upload_source_outputs
			(id, upload_source_id, sort_order, relative_path, size_bytes, duration_ms, timeline_start_ms, timeline_end_ms, status)
		VALUES
			(1, 1, 0, 'upload-sources/1/1/parts/source-p01.flv', 50, 1800000, 0, 1800000, 'READY_TO_UPLOAD');
		INSERT INTO jobs
			(recording_profile_id, upload_source_id, type, resource_class, business_key, payload_json, status, priority, max_attempts)
		VALUES
			(?, 1, 'APPLY_UPLOAD_SOURCE_EDIT', 'MEDIA', 'upload-source:1:edit', '{"upload_source_id":1}', 'PENDING', 70, 3)
	`, created.ID, created.ID); err != nil {
		t.Fatalf("seed edit source returned error: %v", err)
	}

	editor := &fakeEditor{result: media.PackageResult{Outputs: []media.PackageOutput{{
		RelativePath:    "upload-sources/1/1/edited/7G Live-20260905-\u7b2c01\u573a\u76f4\u64ad-edited-p01.flv",
		SizeBytes:       45,
		DurationMs:      1740000,
		TimelineStartMs: 0,
		TimelineEndMs:   1740000,
	}}}}
	if err := NewWithEditor(database, &fakeRecorder{}, cfg, editor).RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce returned error: %v", err)
	}
	if len(editor.request.Cuts) != 1 || editor.request.Cuts[0].StartMs != 60000 || editor.request.Cuts[0].EndMs != 120000 {
		t.Fatalf("unexpected edit cuts: %#v", editor.request.Cuts)
	}
	if len(editor.request.Outputs) != 1 || editor.request.Outputs[0].RelativePath != "upload-sources/1/1/parts/source-p01.flv" {
		t.Fatalf("unexpected edit outputs: %#v", editor.request.Outputs)
	}
	var sourceStatus string
	var editJSON string
	var outputPath string
	var jobStatus string
	if err := database.QueryRowContext(ctx, `
		SELECT us.review_status, COALESCE(us.edit_decision_json, ''), uso.relative_path, j.status
		FROM upload_sources us
		JOIN upload_source_outputs uso ON uso.upload_source_id = us.id
		JOIN jobs j ON j.upload_source_id = us.id AND j.type = 'APPLY_UPLOAD_SOURCE_EDIT'
		WHERE us.id = 1
	`).Scan(&sourceStatus, &editJSON, &outputPath, &jobStatus); err != nil {
		t.Fatalf("query edit result returned error: %v", err)
	}
	if sourceStatus != "REQUIRED" || editJSON != "" || outputPath != "upload-sources/1/1/edited/7G Live-20260905-\u7b2c01\u573a\u76f4\u64ad-edited-p01.flv" || jobStatus != "SUCCEEDED" {
		t.Fatalf("unexpected edit result review=%s edit=%q output=%q job=%s", sourceStatus, editJSON, outputPath, jobStatus)
	}
}

func TestRunOnceUploadsCOSObject(t *testing.T) {
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
	sourceRelativePath := "upload-sources/1/1/parts/7G-20260905-\u7b2c01\u573a\u76f4\u64ad-p01.flv"
	sourcePath := filepath.Join(cfg.DataRoot, sourceRelativePath)
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0o755); err != nil {
		t.Fatalf("create upload source dir returned error: %v", err)
	}
	if err := os.WriteFile(sourcePath, []byte("video"), 0o644); err != nil {
		t.Fatalf("write upload source returned error: %v", err)
	}
	store := upload.NewStore(database, cfg)
	credential, err := store.CreateCredential(ctx, actor, upload.CredentialCreate{
		Scope:        "USER",
		Platform:     "tencent_cos",
		Purpose:      "STORAGE",
		AccountLabel: "cos account",
		Secret:       []byte(`{"secret_id":"id","secret_key":"key"}`),
	})
	if err != nil {
		t.Fatalf("CreateCredential returned error: %v", err)
	}
	if _, err := store.UpsertCOSConfig(ctx, actor, created.ID, upload.COSConfigUpsert{
		CredentialID:    credential.ID,
		Enabled:         true,
		Region:          "ap-shanghai",
		Bucket:          "bucket-1250000000",
		Prefix:          "7grecorder/test/",
		MaxManagedBytes: 1000000000,
	}); err != nil {
		t.Fatalf("UpsertCOSConfig returned error: %v", err)
	}
	if _, err := database.ExecContext(ctx, `
		INSERT INTO upload_sources
			(id, recording_profile_id, source_key, source_room_id, streamer_name_snapshot,
				started_at, completed_at, duration_ms, status, output_relative_path,
				total_bytes, recording_count, file_count, max_gap_seconds, merge_gap_threshold_seconds, ready_at)
		VALUES (1, ?, 'profile:1:1:1', '1741048619', '7G',
			'2026-09-05T10:00:00Z', '2026-09-05T10:30:00Z', 1800000, 'READY_TO_UPLOAD',
			?, 5, 1, 1, 0, 600, CURRENT_TIMESTAMP)
	`, created.ID, sourceRelativePath); err != nil {
		t.Fatalf("insert upload source returned error: %v", err)
	}
	if _, err := database.ExecContext(ctx, `
		INSERT INTO upload_source_outputs
			(id, upload_source_id, sort_order, relative_path, size_bytes, duration_ms, timeline_start_ms, timeline_end_ms, status)
		VALUES
			(1, 1, 0, ?, 5, 1800000, 0, 1800000, 'READY_TO_UPLOAD')
	`, sourceRelativePath); err != nil {
		t.Fatalf("insert upload source output returned error: %v", err)
	}
	result, err := store.Reconcile(ctx, actor)
	if err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}
	if result.COSObjectsCreated != 1 || result.COSJobsCreated != 1 {
		t.Fatalf("unexpected reconcile result: %#v", result)
	}

	cosUploader := &fakeCOSUploader{result: upload.COSUploadResult{ETag: "etag"}}
	if err := NewWithCOSUploader(database, &fakeRecorder{}, cfg, cosUploader).RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce returned error: %v", err)
	}
	if cosUploader.request.ObjectID != 1 || cosUploader.request.ObjectKey != "7grecorder/test/upload-sources/1/1/parts/7G-20260905-\u7b2c01\u573a\u76f4\u64ad-p01.flv" {
		t.Fatalf("unexpected cos upload request: %#v", cosUploader.request)
	}
	if cosUploader.request.Secret.SecretID != "id" || cosUploader.request.Secret.SecretKey != "key" {
		t.Fatalf("unexpected cos secret: %#v", cosUploader.request.Secret)
	}

	var objectStatus string
	var etag string
	if err := database.QueryRowContext(ctx, `
		SELECT status, COALESCE(etag, '')
		FROM upload_source_cos_objects
		WHERE id = 1
	`).Scan(&objectStatus, &etag); err != nil {
		t.Fatalf("query cos object returned error: %v", err)
	}
	if objectStatus != "AVAILABLE" || etag != "etag" {
		t.Fatalf("unexpected cos object status=%s etag=%s", objectStatus, etag)
	}

	var jobStatus string
	if err := database.QueryRowContext(ctx, `SELECT status FROM jobs WHERE business_key = 'upload-source:1:output:1:cos:1'`).Scan(&jobStatus); err != nil {
		t.Fatalf("query job returned error: %v", err)
	}
	if jobStatus != "SUCCEEDED" {
		t.Fatalf("unexpected job status: %s", jobStatus)
	}
}

func TestRunOnceUploadsCOSRecordingFile(t *testing.T) {
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
	sourceRelativePath := "recordings/1741048619-7G/record-1741048619-20260905-224258-164-title.xml"
	sourcePath := filepath.Join(cfg.DataRoot, sourceRelativePath)
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0o755); err != nil {
		t.Fatalf("create recording dir returned error: %v", err)
	}
	if err := os.WriteFile(sourcePath, []byte("<i></i>"), 0o644); err != nil {
		t.Fatalf("write danmaku source returned error: %v", err)
	}
	store := upload.NewStore(database, cfg)
	credential, err := store.CreateCredential(ctx, actor, upload.CredentialCreate{
		Scope:        "USER",
		Platform:     "tencent_cos",
		Purpose:      "STORAGE",
		AccountLabel: "cos account",
		Secret:       []byte(`{"secret_id":"id","secret_key":"key"}`),
	})
	if err != nil {
		t.Fatalf("CreateCredential returned error: %v", err)
	}
	if _, err := store.UpsertCOSConfig(ctx, actor, created.ID, upload.COSConfigUpsert{
		CredentialID:    credential.ID,
		Enabled:         true,
		Region:          "ap-shanghai",
		Bucket:          "bucket-1250000000",
		Prefix:          "7grecorder/test/",
		MaxManagedBytes: 1000000000,
	}); err != nil {
		t.Fatalf("UpsertCOSConfig returned error: %v", err)
	}
	if _, err := database.ExecContext(ctx, `
		INSERT INTO recordings
			(id, recording_profile_id, title, started_at, completed_at, duration_ms, recording_status,
				local_storage_status, source_room_id, streamer_name_snapshot)
		VALUES (1, ?, 'ready upload', '2026-09-05T10:00:00Z', '2026-09-05T10:30:00Z',
			1800000, 'COMPLETED', 'AVAILABLE', '1741048619', '7G')
	`, created.ID); err != nil {
		t.Fatalf("insert recording returned error: %v", err)
	}
	if _, err := database.ExecContext(ctx, `
		INSERT INTO recording_files
			(id, recording_id, relative_path, original_name, kind, file_status, size_bytes, closed_at)
		VALUES (1, 1, ?, 'record-1741048619-20260905-224258-164-title.xml', 'danmaku', 'CLOSED', 7, '2026-09-05T10:30:00Z')
	`, sourceRelativePath); err != nil {
		t.Fatalf("insert danmaku file returned error: %v", err)
	}
	result, err := store.Reconcile(ctx, actor)
	if err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}
	if result.COSFileObjectsCreated != 1 || result.COSFileJobsCreated != 1 {
		t.Fatalf("unexpected reconcile result: %#v", result)
	}

	cosUploader := &fakeCOSUploader{result: upload.COSUploadResult{ETag: "etag"}}
	if err := NewWithCOSUploader(database, &fakeRecorder{}, cfg, cosUploader).RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce returned error: %v", err)
	}
	if cosUploader.request.ObjectKey != "7grecorder/test/raw/recordings/1741048619-7G/record-1741048619-20260905-224258-164-title.xml" {
		t.Fatalf("unexpected raw cos request: %#v", cosUploader.request)
	}

	var objectStatus string
	if err := database.QueryRowContext(ctx, `SELECT status FROM cos_objects WHERE id = 1`).Scan(&objectStatus); err != nil {
		t.Fatalf("query raw cos object returned error: %v", err)
	}
	if objectStatus != "AVAILABLE" {
		t.Fatalf("unexpected raw cos object status: %s", objectStatus)
	}
}

func TestRunOnceCompressesCOSObjectBeforeUpload(t *testing.T) {
	ctx := context.Background()
	cfg, database := openTestDBWithConfig(t, ctx)
	cfg.COSCompressionEnabled = true
	cfg.COSCompressionPreset = upload.COSCompressionPresetH264CRF23MediumMP4
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
	sourceRelativePath := "upload-sources/1/1/parts/7G-20260905-\u7b2c01\u573a\u76f4\u64ad-p01.flv"
	sourcePath := filepath.Join(cfg.DataRoot, sourceRelativePath)
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0o755); err != nil {
		t.Fatalf("create upload source dir returned error: %v", err)
	}
	if err := os.WriteFile(sourcePath, []byte("video"), 0o644); err != nil {
		t.Fatalf("write upload source returned error: %v", err)
	}
	store := upload.NewStore(database, cfg)
	credential, err := store.CreateCredential(ctx, actor, upload.CredentialCreate{
		Scope:        "USER",
		Platform:     "tencent_cos",
		Purpose:      "STORAGE",
		AccountLabel: "cos account",
		Secret:       []byte(`{"secret_id":"id","secret_key":"key"}`),
	})
	if err != nil {
		t.Fatalf("CreateCredential returned error: %v", err)
	}
	if _, err := store.UpsertCOSConfig(ctx, actor, created.ID, upload.COSConfigUpsert{
		CredentialID:    credential.ID,
		Enabled:         true,
		Region:          "ap-shanghai",
		Bucket:          "bucket-1250000000",
		Prefix:          "7grecorder/test/",
		MaxManagedBytes: 1000000000,
	}); err != nil {
		t.Fatalf("UpsertCOSConfig returned error: %v", err)
	}
	if _, err := database.ExecContext(ctx, `
		INSERT INTO upload_sources
			(id, recording_profile_id, source_key, source_room_id, streamer_name_snapshot,
				started_at, completed_at, duration_ms, status, output_relative_path,
				total_bytes, recording_count, file_count, max_gap_seconds, merge_gap_threshold_seconds, ready_at)
		VALUES (1, ?, 'profile:1:1:1', '1741048619', '7G',
			'2026-09-05T10:00:00Z', '2026-09-05T10:30:00Z', 1800000, 'READY_TO_UPLOAD',
			?, 5, 1, 1, 0, 600, CURRENT_TIMESTAMP)
	`, created.ID, sourceRelativePath); err != nil {
		t.Fatalf("insert upload source returned error: %v", err)
	}
	if _, err := database.ExecContext(ctx, `
		INSERT INTO upload_source_outputs
			(id, upload_source_id, sort_order, relative_path, size_bytes, duration_ms, timeline_start_ms, timeline_end_ms, status)
		VALUES
			(1, 1, 0, ?, 5, 1800000, 0, 1800000, 'READY_TO_UPLOAD')
	`, sourceRelativePath); err != nil {
		t.Fatalf("insert upload source output returned error: %v", err)
	}
	result, err := store.Reconcile(ctx, actor)
	if err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}
	if result.COSObjectsCreated != 1 || result.COSJobsCreated != 1 {
		t.Fatalf("unexpected reconcile result: %#v", result)
	}

	compressedRelativePath := "upload-sources/1/1/cos/7G-20260905-\u7b2c01\u573a\u76f4\u64ad-p01.mp4"
	compressedPath := filepath.Join(cfg.DataRoot, compressedRelativePath)
	if err := os.MkdirAll(filepath.Dir(compressedPath), 0o755); err != nil {
		t.Fatalf("create compressed dir returned error: %v", err)
	}
	if err := os.WriteFile(compressedPath, []byte("mp4"), 0o644); err != nil {
		t.Fatalf("write compressed output returned error: %v", err)
	}
	compressor := &fakeCompressor{result: media.CompressionResult{
		RelativePath: compressedRelativePath,
		SizeBytes:    3,
		Preset:       upload.COSCompressionPresetH264CRF23MediumMP4,
	}}
	cosUploader := &fakeCOSUploader{result: upload.COSUploadResult{ETag: "etag", SizeBytes: 3}}
	if err := NewWithCOSUploaderAndCompressor(database, &fakeRecorder{}, cfg, cosUploader, compressor).RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce returned error: %v", err)
	}
	if compressor.request.InputRelativePath != sourceRelativePath || compressor.request.OutputRelativePath != compressedRelativePath {
		t.Fatalf("unexpected compression request: %#v", compressor.request)
	}
	if cosUploader.request.SourcePath != compressedPath {
		t.Fatalf("expected compressed upload path %q, got %q", compressedPath, cosUploader.request.SourcePath)
	}
	if cosUploader.request.ObjectKey != "7grecorder/test/upload-sources/1/1/cos/7G-20260905-\u7b2c01\u573a\u76f4\u64ad-p01.mp4" {
		t.Fatalf("unexpected compressed object key: %q", cosUploader.request.ObjectKey)
	}

	var objectStatus string
	var compressionStatus string
	var compressedFrom string
	var sizeBytes int64
	var sourceSizeBytes int64
	if err := database.QueryRowContext(ctx, `
		SELECT status, compression_status, COALESCE(compressed_from_relative_path, ''), size_bytes, source_size_bytes
		FROM upload_source_cos_objects
		WHERE id = 1
	`).Scan(&objectStatus, &compressionStatus, &compressedFrom, &sizeBytes, &sourceSizeBytes); err != nil {
		t.Fatalf("query cos object returned error: %v", err)
	}
	if objectStatus != "AVAILABLE" || compressionStatus != "COMPRESSED" || compressedFrom != compressedRelativePath || sizeBytes != 3 || sourceSizeBytes != 5 {
		t.Fatalf("unexpected cos object metadata status=%s compression=%s from=%s size=%d source=%d", objectStatus, compressionStatus, compressedFrom, sizeBytes, sourceSizeBytes)
	}
	if _, err := os.Stat(compressedPath); !os.IsNotExist(err) {
		t.Fatalf("expected compressed local file to be removed after upload, got %v", err)
	}
}

func TestRunOnceUploadsBilibiliPublication(t *testing.T) {
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
	sourceRelativePath := "upload-sources/1/1/parts/7G Live-20260905-\u7b2c01\u573a\u76f4\u64ad-p01.flv"
	sourcePath := filepath.Join(cfg.DataRoot, sourceRelativePath)
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0o755); err != nil {
		t.Fatalf("create upload source dir returned error: %v", err)
	}
	if err := os.WriteFile(sourcePath, []byte("video"), 0o644); err != nil {
		t.Fatalf("write upload source returned error: %v", err)
	}
	store := upload.NewStore(database, cfg)
	credential, err := store.CreateCredential(ctx, actor, upload.CredentialCreate{
		Scope:        "USER",
		Platform:     "bilibili",
		Purpose:      "PUBLISHER",
		AccountLabel: "bili account",
		Secret:       []byte(`{"cookie":"cookie"}`),
	})
	if err != nil {
		t.Fatalf("CreateCredential returned error: %v", err)
	}
	if _, err := store.UpsertBilibiliConfig(ctx, actor, created.ID, upload.PublishingConfigUpsert{
		CredentialID: credential.ID,
		Enabled:      true,
		Settings: []byte(`{
			"title_template":"{{profile_name}} {{date_compact}} 第{{live_ordinal}}场直播",
			"description_template":"直播间 {{room_id}} {{started_at_china}}",
			"tags":["录播","七宫筱野"],
			"copyright":2
		}`),
	}); err != nil {
		t.Fatalf("UpsertBilibiliConfig returned error: %v", err)
	}
	if _, err := database.ExecContext(ctx, `
		INSERT INTO upload_sources
			(id, recording_profile_id, source_key, source_room_id, streamer_name_snapshot,
				started_at, completed_at, duration_ms, status, output_relative_path,
				total_bytes, recording_count, file_count, max_gap_seconds, merge_gap_threshold_seconds, ready_at)
		VALUES (1, ?, 'profile:1:1:1', '1741048619', '7G',
			'2026-09-05T10:00:00Z', '2026-09-05T10:30:00Z', 1800000, 'READY_TO_UPLOAD',
			?, 5, 1, 1, 0, 600, CURRENT_TIMESTAMP)
	`, created.ID, sourceRelativePath); err != nil {
		t.Fatalf("insert upload source returned error: %v", err)
	}
	if _, err := database.ExecContext(ctx, `
		INSERT INTO upload_source_outputs
			(id, upload_source_id, sort_order, relative_path, size_bytes, duration_ms, timeline_start_ms, timeline_end_ms, status)
		VALUES
			(1, 1, 0, ?, 5, 1800000, 0, 1800000, 'READY_TO_UPLOAD')
	`, sourceRelativePath); err != nil {
		t.Fatalf("insert upload source output returned error: %v", err)
	}
	result, err := store.Reconcile(ctx, actor)
	if err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}
	if result.PublicationsCreated != 1 || result.BilibiliJobsCreated != 1 {
		t.Fatalf("unexpected reconcile result: %#v", result)
	}

	uploader := &fakeBilibiliUploader{result: upload.BilibiliUploadResult{ExternalID: "BV1test", ExternalURL: "https://www.bilibili.com/video/BV1test"}}
	if err := NewWithBilibiliUploader(database, &fakeRecorder{}, cfg, uploader).RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce returned error: %v", err)
	}
	if uploader.request.Title != "7G Live 20260905 \u7b2c01\u573a\u76f4\u64ad" {
		t.Fatalf("unexpected bilibili title: %q", uploader.request.Title)
	}
	if len(uploader.request.Parts) != 1 || uploader.request.Parts[0].SourcePath == "" {
		t.Fatalf("unexpected bilibili parts: %#v", uploader.request.Parts)
	}
	if len(uploader.request.Tags) != 2 || uploader.request.Copyright != 2 {
		t.Fatalf("unexpected bilibili settings: %#v", uploader.request)
	}

	var publicationStatus string
	var externalURL string
	if err := database.QueryRowContext(ctx, `
		SELECT status, COALESCE(external_url, '')
		FROM publications
		WHERE id = 1
	`).Scan(&publicationStatus, &externalURL); err != nil {
		t.Fatalf("query publication returned error: %v", err)
	}
	if publicationStatus != "VERIFIED" || externalURL != "https://www.bilibili.com/video/BV1test" {
		t.Fatalf("unexpected publication status=%s url=%s", publicationStatus, externalURL)
	}

	var jobStatus string
	if err := database.QueryRowContext(ctx, `SELECT status FROM jobs WHERE business_key = 'upload-source:1:bilibili:upload'`).Scan(&jobStatus); err != nil {
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
	masterKeyPath := filepath.Join(root, "master.key")
	if err := os.WriteFile(masterKeyPath, []byte("test-master-key"), 0o600); err != nil {
		t.Fatalf("write master key returned error: %v", err)
	}
	cfg := config.Config{
		DataRoot:      root,
		SQLitePath:    filepath.Join(root, "7grecorder.db"),
		TempRoot:      filepath.Join(root, "temp"),
		MasterKeyPath: masterKeyPath,
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
