package recording

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/7grecorder/7grecorder/backend/internal/account"
	"github.com/7grecorder/7grecorder/backend/internal/config"
	"github.com/7grecorder/7grecorder/backend/internal/db"
	"github.com/7grecorder/7grecorder/backend/internal/media"
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
	oldTime := closedTestTime()
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
	if items[0].DurationMs <= 0 || items[0].Files[0].DurationMs <= 0 {
		t.Fatalf("expected duration metadata, got recording=%d file=%d", items[0].DurationMs, items[0].Files[0].DurationMs)
	}
}

func TestReconcileLocalImportsDanmakuWithMatchingVideo(t *testing.T) {
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
	baseName := "record-1741048619-20260905-224258-164-title"
	videoPath := filepath.Join(recordingDir, baseName+".flv")
	danmakuPath := filepath.Join(recordingDir, baseName+".xml")
	if err := os.WriteFile(videoPath, []byte("video"), 0o644); err != nil {
		t.Fatalf("WriteFile video returned error: %v", err)
	}
	if err := os.WriteFile(danmakuPath, []byte("<i></i>"), 0o644); err != nil {
		t.Fatalf("WriteFile danmaku returned error: %v", err)
	}
	oldTime := closedTestTime()
	if err := os.Chtimes(videoPath, oldTime, oldTime); err != nil {
		t.Fatalf("Chtimes video returned error: %v", err)
	}
	if err := os.Chtimes(danmakuPath, oldTime, oldTime); err != nil {
		t.Fatalf("Chtimes danmaku returned error: %v", err)
	}

	result, err := NewStore(database, cfg).ReconcileLocal(ctx, actor)
	if err != nil {
		t.Fatalf("ReconcileLocal returned error: %v", err)
	}
	if result.Imported != 2 {
		t.Fatalf("expected two imported files, got %#v", result)
	}

	items, err := NewStore(database, cfg).List(ctx, actor)
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one recording, got %d", len(items))
	}
	if len(items[0].Files) != 2 {
		t.Fatalf("expected video and danmaku files, got %#v", items[0].Files)
	}
	kinds := map[string]bool{}
	for _, file := range items[0].Files {
		kinds[file.Kind] = true
	}
	if !kinds["video"] || !kinds["danmaku"] {
		t.Fatalf("expected video and danmaku kinds, got %#v", items[0].Files)
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
	oldTime := closedTestTime()
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
	oldTime := closedTestTime()
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

func TestUploadSourceOutputForReviewDownloadRequiresReviewGate(t *testing.T) {
	ctx := context.Background()
	cfg, database := openTestDB(t, ctx)
	actor := bootstrapTestAdmin(t, ctx, database)
	if _, err := profile.NewStore(database).Create(ctx, actor, profile.CreateRequest{
		Name:         "7G",
		RoomID:       "1741048619",
		StreamerName: "Streamer",
	}); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	relativePath := "upload-sources/1/1/parts/review-p01.flv"
	absolutePath := filepath.Join(cfg.DataRoot, relativePath)
	if err := os.MkdirAll(filepath.Dir(absolutePath), 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	if err := os.WriteFile(absolutePath, []byte("video"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	if _, err := database.ExecContext(ctx, `
		INSERT INTO upload_sources
			(id, recording_profile_id, source_key, title, source_room_id, streamer_name_snapshot,
				started_at, completed_at, duration_ms, status, total_bytes, recording_count,
				file_count, max_gap_seconds, merge_gap_threshold_seconds, ready_at)
		VALUES (1, 1, 'profile:1:1:1', 'review me', '1741048619', 'Streamer',
			'2026-09-05T10:00:00Z', '2026-09-05T10:30:00Z', 1800000,
			'READY_TO_UPLOAD', 5, 1, 1, 0, 600, CURRENT_TIMESTAMP);
		INSERT INTO upload_source_outputs
			(id, upload_source_id, sort_order, relative_path, size_bytes, duration_ms,
				timeline_start_ms, timeline_end_ms, status)
		VALUES (1, 1, 0, 'upload-sources/1/1/parts/review-p01.flv', 5, 1800000,
			0, 1800000, 'READY_TO_UPLOAD');
	`); err != nil {
		t.Fatalf("seed upload source returned error: %v", err)
	}

	store := NewStore(database, cfg)
	if _, err := store.UploadSourceOutputForReviewDownload(ctx, actor, 1, 1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound before review is required, got %v", err)
	}
	if _, err := database.ExecContext(ctx, `
		UPDATE upload_sources
		SET review_status = 'REQUIRED', review_requested_at = CURRENT_TIMESTAMP
		WHERE id = 1
	`); err != nil {
		t.Fatalf("mark review required returned error: %v", err)
	}
	file, err := store.UploadSourceOutputForReviewDownload(ctx, actor, 1, 1)
	if err != nil {
		t.Fatalf("UploadSourceOutputForReviewDownload returned error: %v", err)
	}
	if file.AbsolutePath != absolutePath || file.OriginalName != "review-p01.flv" || file.ContentType != "video/x-flv" {
		t.Fatalf("unexpected review download file: %#v", file)
	}
}

func TestApproveUploadSourceReviewResetsFrozenRemoteUploads(t *testing.T) {
	ctx := context.Background()
	cfg, database := openTestDB(t, ctx)
	actor := bootstrapTestAdmin(t, ctx, database)
	created, err := profile.NewStore(database).Create(ctx, actor, profile.CreateRequest{
		Name:         "7G",
		RoomID:       "1741048619",
		StreamerName: "Streamer",
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if _, err := database.ExecContext(ctx, `
		INSERT INTO credentials (id, owner_user_id, scope, platform, purpose, account_label, encrypted_secret, status)
		VALUES
			(1, ?, 'USER', 'bilibili', 'PUBLISHER', 'bili account', X'00', 'UNVERIFIED'),
			(2, ?, 'USER', 'tencent_cos', 'STORAGE', 'cos account', X'00', 'UNVERIFIED');
		INSERT INTO publishing_profiles
			(id, recording_profile_id, platform, credential_id, enabled, settings_json)
		VALUES (1, ?, 'bilibili', 1, 1, '{}');
		INSERT INTO cos_storage_profiles
			(id, recording_profile_id, credential_id, enabled, region, bucket, prefix, max_managed_bytes)
		VALUES (1, ?, 2, 1, 'ap-shanghai', 'bucket-1250000000', '7grecorder/test/', 1000000000);
		INSERT INTO upload_sources
			(id, recording_profile_id, source_key, title, source_room_id, streamer_name_snapshot,
				started_at, completed_at, duration_ms, status, total_bytes, recording_count,
				file_count, max_gap_seconds, merge_gap_threshold_seconds, ready_at, review_status)
		VALUES (1, ?, 'profile:1:1:1', 'review me', '1741048619', 'Streamer',
			'2026-09-05T10:00:00Z', '2026-09-05T10:30:00Z', 1800000,
			'READY_TO_UPLOAD', 5, 1, 1, 0, 600, CURRENT_TIMESTAMP, 'REQUIRED');
		INSERT INTO upload_source_outputs
			(id, upload_source_id, sort_order, relative_path, size_bytes, duration_ms,
				timeline_start_ms, timeline_end_ms, status)
		VALUES (1, 1, 0, 'upload-sources/1/1/parts/review-p01.flv', 5, 1800000,
			0, 1800000, 'READY_TO_UPLOAD');
		INSERT INTO publications
			(id, recording_profile_id, upload_source_id, platform, credential_id, status, last_error)
		VALUES (1, ?, 1, 'bilibili', 1, 'FAILED', 'cancelled manually: freeze for review');
		INSERT INTO upload_source_cos_objects
			(id, cos_storage_profile_id, recording_profile_id, upload_source_id, upload_source_output_id,
				object_key, size_bytes, source_size_bytes, status, last_error)
		VALUES (1, 1, ?, 1, 1, '7grecorder/test/upload-sources/1/1/parts/review-p01.flv',
			5, 5, 'FAILED', 'cancelled manually: freeze for review');
		INSERT INTO jobs
			(recording_profile_id, upload_source_id, publication_id, type, resource_class,
				business_key, payload_json, status, priority, max_attempts, attempts, last_error)
		VALUES
			(?, 1, 1, 'UPLOAD_BILIBILI', 'NETWORK', 'upload-source:1:bilibili:upload',
				'{"publication_id":1,"upload_source_id":1}', 'CANCELLED', 80, 3, 1,
				'cancelled manually: freeze for review'),
			(?, 1, NULL, 'UPLOAD_COS_OBJECT', 'NETWORK', 'upload-source:1:output:1:cos:1',
				'{"cos_object_id":1,"upload_source_id":1,"output_id":1}', 'CANCELLED', 90, 5, 1,
				'cancelled manually: freeze for review');
	`, actor.ID, actor.ID, created.ID, created.ID, created.ID, created.ID, created.ID, created.ID, created.ID); err != nil {
		t.Fatalf("seed frozen upload source returned error: %v", err)
	}

	item, err := NewStore(database, cfg).ApproveUploadSourceReview(ctx, actor, 1, UploadReviewRequest{})
	if err != nil {
		t.Fatalf("ApproveUploadSourceReview returned error: %v", err)
	}
	if item.ReviewStatus != "APPROVED" {
		t.Fatalf("expected review approved, got %#v", item)
	}
	var publicationStatus, cosStatus, bilibiliJobStatus, cosJobStatus string
	if err := database.QueryRowContext(ctx, `SELECT status FROM publications WHERE id = 1`).Scan(&publicationStatus); err != nil {
		t.Fatalf("query publication returned error: %v", err)
	}
	if err := database.QueryRowContext(ctx, `SELECT status FROM upload_source_cos_objects WHERE id = 1`).Scan(&cosStatus); err != nil {
		t.Fatalf("query cos object returned error: %v", err)
	}
	if err := database.QueryRowContext(ctx, `SELECT status FROM jobs WHERE business_key = 'upload-source:1:bilibili:upload'`).Scan(&bilibiliJobStatus); err != nil {
		t.Fatalf("query bilibili job returned error: %v", err)
	}
	if err := database.QueryRowContext(ctx, `SELECT status FROM jobs WHERE business_key = 'upload-source:1:output:1:cos:1'`).Scan(&cosJobStatus); err != nil {
		t.Fatalf("query cos job returned error: %v", err)
	}
	if publicationStatus != "PENDING" || cosStatus != "PENDING" || bilibiliJobStatus != "PENDING" || cosJobStatus != "PENDING" {
		t.Fatalf("expected frozen remote uploads reset, publication=%s cos=%s bili_job=%s cos_job=%s", publicationStatus, cosStatus, bilibiliJobStatus, cosJobStatus)
	}
}

func TestRequireUploadSourceReviewFreezesRunningRemoteUploads(t *testing.T) {
	ctx := context.Background()
	cfg, database := openTestDB(t, ctx)
	actor := bootstrapTestAdmin(t, ctx, database)
	created, err := profile.NewStore(database).Create(ctx, actor, profile.CreateRequest{
		Name:         "7G",
		RoomID:       "1741048619",
		StreamerName: "Streamer",
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if _, err := database.ExecContext(ctx, `
		INSERT INTO credentials (id, owner_user_id, scope, platform, purpose, account_label, encrypted_secret, status)
		VALUES
			(1, ?, 'USER', 'bilibili', 'PUBLISHER', 'bili account', X'00', 'UNVERIFIED'),
			(2, ?, 'USER', 'tencent_cos', 'STORAGE', 'cos account', X'00', 'UNVERIFIED');
		INSERT INTO cos_storage_profiles
			(id, recording_profile_id, credential_id, enabled, region, bucket, prefix, max_managed_bytes)
		VALUES (1, ?, 2, 1, 'ap-shanghai', 'bucket-1250000000', '7grecorder/test/', 1000000000);
		INSERT INTO upload_sources
			(id, recording_profile_id, source_key, title, source_room_id, streamer_name_snapshot,
				started_at, completed_at, duration_ms, status, total_bytes, recording_count,
				file_count, max_gap_seconds, merge_gap_threshold_seconds, ready_at)
		VALUES (1, ?, 'profile:1:1:1', 'review me', '1741048619', 'Streamer',
			'2026-09-05T10:00:00Z', '2026-09-05T10:30:00Z', 1800000,
			'READY_TO_UPLOAD', 5, 1, 1, 0, 600, CURRENT_TIMESTAMP);
		INSERT INTO upload_source_outputs
			(id, upload_source_id, sort_order, relative_path, size_bytes, duration_ms,
				timeline_start_ms, timeline_end_ms, status)
		VALUES (1, 1, 0, 'upload-sources/1/1/parts/review-p01.flv', 5, 1800000,
			0, 1800000, 'READY_TO_UPLOAD');
		INSERT INTO publications
			(id, recording_profile_id, upload_source_id, platform, credential_id, status)
		VALUES (1, ?, 1, 'bilibili', 1, 'UPLOADING');
		INSERT INTO upload_source_cos_objects
			(id, cos_storage_profile_id, recording_profile_id, upload_source_id, upload_source_output_id,
				object_key, size_bytes, source_size_bytes, status)
		VALUES (1, 1, ?, 1, 1, '7grecorder/test/upload-sources/1/1/parts/review-p01.flv',
			5, 5, 'UPLOADING');
		INSERT INTO jobs
			(recording_profile_id, upload_source_id, publication_id, type, resource_class,
				business_key, payload_json, status, priority, max_attempts, attempts, locked_by)
		VALUES
			(?, 1, 1, 'UPLOAD_BILIBILI', 'NETWORK', 'upload-source:1:bilibili:upload',
				'{"publication_id":1,"upload_source_id":1}', 'RUNNING', 80, 3, 1, 'worker:1'),
			(?, 1, NULL, 'UPLOAD_COS_OBJECT', 'NETWORK', 'upload-source:1:output:1:cos:1',
				'{"cos_object_id":1,"upload_source_id":1,"output_id":1}', 'RUNNING', 90, 5, 1, 'worker:2');
	`, actor.ID, actor.ID, created.ID, created.ID, created.ID, created.ID, created.ID, created.ID); err != nil {
		t.Fatalf("seed running upload source returned error: %v", err)
	}

	item, err := NewStore(database, cfg).RequireUploadSourceReview(ctx, actor, 1, UploadReviewRequest{})
	if err != nil {
		t.Fatalf("RequireUploadSourceReview returned error: %v", err)
	}
	if item.ReviewStatus != "REQUIRED" {
		t.Fatalf("expected review required, got %#v", item)
	}
	var publicationStatus, cosStatus, bilibiliJobStatus, cosJobStatus string
	if err := database.QueryRowContext(ctx, `SELECT status FROM publications WHERE id = 1`).Scan(&publicationStatus); err != nil {
		t.Fatalf("query publication returned error: %v", err)
	}
	if err := database.QueryRowContext(ctx, `SELECT status FROM upload_source_cos_objects WHERE id = 1`).Scan(&cosStatus); err != nil {
		t.Fatalf("query cos object returned error: %v", err)
	}
	if err := database.QueryRowContext(ctx, `SELECT status FROM jobs WHERE business_key = 'upload-source:1:bilibili:upload'`).Scan(&bilibiliJobStatus); err != nil {
		t.Fatalf("query bilibili job returned error: %v", err)
	}
	if err := database.QueryRowContext(ctx, `SELECT status FROM jobs WHERE business_key = 'upload-source:1:output:1:cos:1'`).Scan(&cosJobStatus); err != nil {
		t.Fatalf("query cos job returned error: %v", err)
	}
	if publicationStatus != "PENDING" || cosStatus != "PENDING" || bilibiliJobStatus != "CANCELLED" || cosJobStatus != "CANCELLED" {
		t.Fatalf("expected running uploads frozen, publication=%s cos=%s bili_job=%s cos_job=%s", publicationStatus, cosStatus, bilibiliJobStatus, cosJobStatus)
	}
	if _, err := database.ExecContext(ctx, `UPDATE publications SET status = 'VERIFIED' WHERE id = 1`); err == nil {
		t.Fatal("expected review gate to reject late bilibili completion")
	}
	if _, err := database.ExecContext(ctx, `UPDATE upload_source_cos_objects SET status = 'AVAILABLE' WHERE id = 1`); err == nil {
		t.Fatal("expected review gate to reject late cos completion")
	}
}

func TestApproveUploadSourceReviewRejectsPendingEditDecision(t *testing.T) {
	ctx := context.Background()
	cfg, database := openTestDB(t, ctx)
	actor := bootstrapTestAdmin(t, ctx, database)
	created, err := profile.NewStore(database).Create(ctx, actor, profile.CreateRequest{
		Name:         "7G",
		RoomID:       "1741048619",
		StreamerName: "Streamer",
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if _, err := database.ExecContext(ctx, `
		INSERT INTO upload_sources
			(id, recording_profile_id, source_key, title, source_room_id, streamer_name_snapshot,
				started_at, completed_at, duration_ms, status, total_bytes, recording_count,
				file_count, max_gap_seconds, merge_gap_threshold_seconds, ready_at, review_status,
				edit_decision_json)
		VALUES (1, ?, 'profile:1:1:1', 'review me', '1741048619', 'Streamer',
			'2026-09-05T10:00:00Z', '2026-09-05T10:30:00Z', 1800000,
			'READY_TO_UPLOAD', 5, 1, 1, 0, 600, CURRENT_TIMESTAMP, 'REQUIRED',
			'{"cuts":[{"start_ms":60000,"end_ms":120000}]}');
		INSERT INTO upload_source_outputs
			(id, upload_source_id, sort_order, relative_path, size_bytes, duration_ms,
				timeline_start_ms, timeline_end_ms, status)
		VALUES (1, 1, 0, 'upload-sources/1/1/parts/review-p01.flv', 5, 1800000,
			0, 1800000, 'READY_TO_UPLOAD');
	`, created.ID); err != nil {
		t.Fatalf("seed upload source returned error: %v", err)
	}

	_, err = NewStore(database, cfg).ApproveUploadSourceReview(ctx, actor, 1, UploadReviewRequest{})
	if !errors.Is(err, ErrNotReady) {
		t.Fatalf("expected ErrNotReady, got %v", err)
	}
}

func TestLocalStorageStatusSummarizesIndexedVideos(t *testing.T) {
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
	oldTime := closedTestTime()
	if err := os.Chtimes(filePath, oldTime, oldTime); err != nil {
		t.Fatalf("Chtimes returned error: %v", err)
	}

	store := NewStore(database, cfg)
	if _, err := store.ReconcileLocal(ctx, actor); err != nil {
		t.Fatalf("ReconcileLocal returned error: %v", err)
	}
	status, err := store.LocalStorageStatus(ctx, actor)
	if err != nil {
		t.Fatalf("LocalStorageStatus returned error: %v", err)
	}
	if status.IndexedVideoFiles != 1 || status.IndexedVideoBytes != 5 || status.CompletedRecordings != 1 {
		t.Fatalf("unexpected storage status: %#v", status)
	}
	if status.DiskTotalBytes <= 0 || status.DiskAvailableBytes <= 0 {
		t.Fatalf("expected disk stats, got %#v", status)
	}
	if status.Settings.MaxRecordingBytes <= 0 || status.Health == "" {
		t.Fatalf("expected storage policy preview, got %#v", status)
	}
}

func TestSetLocalProtectedTogglesRecording(t *testing.T) {
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
	oldTime := closedTestTime()
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
	protected, err := store.SetLocalProtected(ctx, actor, items[0].ID, true)
	if err != nil {
		t.Fatalf("SetLocalProtected true returned error: %v", err)
	}
	if !protected.LocalProtected {
		t.Fatalf("expected recording to be protected")
	}
	unprotected, err := store.SetLocalProtected(ctx, actor, items[0].ID, false)
	if err != nil {
		t.Fatalf("SetLocalProtected false returned error: %v", err)
	}
	if unprotected.LocalProtected {
		t.Fatalf("expected recording to be unprotected")
	}
}

func TestCleanupCandidatesExcludeProtectedRecordings(t *testing.T) {
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
	oldTime := closedTestTime()
	if err := os.Chtimes(filePath, oldTime, oldTime); err != nil {
		t.Fatalf("Chtimes returned error: %v", err)
	}

	store := NewStore(database, cfg)
	if _, err := store.ReconcileLocal(ctx, actor); err != nil {
		t.Fatalf("ReconcileLocal returned error: %v", err)
	}
	seedDeliveredCleanupSource(t, ctx, database, actor.ID, 1, 1)
	candidates, err := store.CleanupCandidates(ctx, actor, 10)
	if err != nil {
		t.Fatalf("CleanupCandidates returned error: %v", err)
	}
	if len(candidates.Items) != 1 || candidates.Items[0].ReclaimableBytes != 5 || candidates.PreviewReclaimableBytes != 5 {
		t.Fatalf("unexpected cleanup candidates: %#v", candidates)
	}

	if _, err := store.SetLocalProtected(ctx, actor, candidates.Items[0].RecordingID, true); err != nil {
		t.Fatalf("SetLocalProtected returned error: %v", err)
	}
	candidates, err = store.CleanupCandidates(ctx, actor, 10)
	if err != nil {
		t.Fatalf("second CleanupCandidates returned error: %v", err)
	}
	if len(candidates.Items) != 0 || candidates.PreviewReclaimableBytes != 0 {
		t.Fatalf("expected protected recording to be excluded, got %#v", candidates)
	}
}

func TestRunLocalCleanupDeletesOldestUnprotectedCompletedRecording(t *testing.T) {
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
	oldTime := closedTestTime()
	if err := os.Chtimes(filePath, oldTime, oldTime); err != nil {
		t.Fatalf("Chtimes returned error: %v", err)
	}

	store := NewStore(database, cfg)
	if _, err := store.ReconcileLocal(ctx, actor); err != nil {
		t.Fatalf("ReconcileLocal returned error: %v", err)
	}
	seedDeliveredCleanupSource(t, ctx, database, actor.ID, 1, 1)
	derivedPath := filepath.Join(cfg.DataRoot, "upload-sources", "1", "1", "parts", "part-01.flv")
	if err := os.MkdirAll(filepath.Dir(derivedPath), 0o755); err != nil {
		t.Fatalf("MkdirAll derived output returned error: %v", err)
	}
	if err := os.WriteFile(derivedPath, []byte("derived"), 0o644); err != nil {
		t.Fatalf("WriteFile derived output returned error: %v", err)
	}
	if _, err := store.UpsertLocalStorageSettings(ctx, actor, LocalStorageSettingsUpsert{
		MaxRecordingBytes:          1,
		MinSystemFreeBytes:         1,
		CleanupTargetRatio:         0.5,
		AbsoluteEmergencyFreeBytes: 1,
	}); err != nil {
		t.Fatalf("UpsertLocalStorageSettings returned error: %v", err)
	}

	result, err := store.RunLocalCleanup(ctx, actor, CleanupRunRequest{MaxRecordings: 5})
	if err != nil {
		t.Fatalf("RunLocalCleanup returned error: %v", err)
	}
	if result.DeletedRecordings != 1 || result.DeletedFiles != 1 || result.ReclaimedBytes != 12 {
		t.Fatalf("unexpected cleanup result: %#v", result)
	}
	if _, err := os.Stat(filePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected file to be deleted, stat err=%v", err)
	}
	if _, err := os.Stat(derivedPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected derived output to be deleted, stat err=%v", err)
	}
	var cleanupStatus string
	if err := database.QueryRowContext(ctx, `SELECT local_cleanup_status FROM upload_sources WHERE id = 1`).Scan(&cleanupStatus); err != nil {
		t.Fatalf("read upload source cleanup status returned error: %v", err)
	}
	if cleanupStatus != "DELETED" {
		t.Fatalf("expected delivered source to be marked deleted, got %q", cleanupStatus)
	}

	items, err := store.List(ctx, actor)
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(items) != 1 || items[0].LocalStorageStatus != "DELETED" || items[0].Files[0].FileStatus != "DELETED" {
		t.Fatalf("expected deleted metadata, got %#v", items)
	}
}

func TestAutomaticCleanupRequiresAllEnabledDestinationsAndRetainsNewestSource(t *testing.T) {
	ctx := context.Background()
	cfg, database := openTestDB(t, ctx)
	actor := bootstrapTestAdmin(t, ctx, database)
	if _, err := profile.NewStore(database).Create(ctx, actor, profile.CreateRequest{
		Name: "7G", RoomID: "1741048619", StreamerName: "Streamer",
	}); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	insertRecordingMetadata(t, ctx, database, insertRecordingRequest{
		Title: "old", StartedAt: "2026-09-05T10:00:00Z", CompletedAt: "2026-09-05T10:30:00Z", DurationMs: 1800000, SizeBytes: 5,
	})
	seedDeliveredCleanupSource(t, ctx, database, actor.ID, 1, 1)
	if _, err := database.ExecContext(ctx, `UPDATE publications SET status = 'FAILED' WHERE id = 1`); err != nil {
		t.Fatalf("mark publication failed returned error: %v", err)
	}

	store := NewStore(database, cfg)
	if _, err := store.UpsertLocalStorageSettings(ctx, actor, LocalStorageSettingsUpsert{
		MaxRecordingBytes: 1, MinSystemFreeBytes: 1, CleanupTargetRatio: 0.5, AbsoluteEmergencyFreeBytes: 1,
	}); err != nil {
		t.Fatalf("UpsertLocalStorageSettings returned error: %v", err)
	}
	result, err := store.RunAutomaticUploadSourceCleanup(ctx, 10)
	if err != nil {
		t.Fatalf("RunAutomaticUploadSourceCleanup returned error: %v", err)
	}
	if result.DeletedRecordings != 0 {
		t.Fatalf("expected failed Bilibili delivery to block cleanup, got %#v", result)
	}
	if _, err := database.ExecContext(ctx, `UPDATE publications SET status = 'VERIFIED' WHERE id = 1`); err != nil {
		t.Fatalf("mark publication verified returned error: %v", err)
	}
	result, err = store.RunAutomaticUploadSourceCleanup(ctx, 10)
	if err != nil {
		t.Fatalf("second RunAutomaticUploadSourceCleanup returned error: %v", err)
	}
	if result.DeletedRecordings != 1 {
		t.Fatalf("expected delivered old source cleanup, got %#v", result)
	}
	var newestStatus string
	if err := database.QueryRowContext(ctx, `SELECT local_cleanup_status FROM upload_sources WHERE id = 2`).Scan(&newestStatus); err != nil {
		t.Fatalf("read newest cleanup status returned error: %v", err)
	}
	if newestStatus != "AVAILABLE" {
		t.Fatalf("expected newest source to remain available, got %q", newestStatus)
	}
}

func TestRunLocalCleanupSkipsProtectedRecording(t *testing.T) {
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
	oldTime := closedTestTime()
	if err := os.Chtimes(filePath, oldTime, oldTime); err != nil {
		t.Fatalf("Chtimes returned error: %v", err)
	}

	store := NewStore(database, cfg)
	if _, err := store.ReconcileLocal(ctx, actor); err != nil {
		t.Fatalf("ReconcileLocal returned error: %v", err)
	}
	seedDeliveredCleanupSource(t, ctx, database, actor.ID, 1, 1)
	items, err := store.List(ctx, actor)
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if _, err := store.SetLocalProtected(ctx, actor, items[0].ID, true); err != nil {
		t.Fatalf("SetLocalProtected returned error: %v", err)
	}
	if _, err := store.UpsertLocalStorageSettings(ctx, actor, LocalStorageSettingsUpsert{
		MaxRecordingBytes:          1,
		MinSystemFreeBytes:         1,
		CleanupTargetRatio:         0.5,
		AbsoluteEmergencyFreeBytes: 1,
	}); err != nil {
		t.Fatalf("UpsertLocalStorageSettings returned error: %v", err)
	}

	result, err := store.RunLocalCleanup(ctx, actor, CleanupRunRequest{MaxRecordings: 5})
	if err != nil {
		t.Fatalf("RunLocalCleanup returned error: %v", err)
	}
	if result.DeletedRecordings != 0 || result.DeletedFiles != 0 {
		t.Fatalf("expected protected recording to be skipped, got %#v", result)
	}
	if _, err := os.Stat(filePath); err != nil {
		t.Fatalf("expected protected file to remain, stat err=%v", err)
	}
}

func TestListGroupsCombinesContinuousRecordingsAndSplitsRealGaps(t *testing.T) {
	ctx := context.Background()
	cfg, database := openTestDB(t, ctx)
	actor := bootstrapTestAdmin(t, ctx, database)
	if _, err := profile.NewStore(database).Create(ctx, actor, profile.CreateRequest{
		Name:         "7G",
		RoomID:       "1741048619",
		StreamerName: "Streamer",
	}); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	insertRecordingMetadata(t, ctx, database, insertRecordingRequest{
		Title:       "part 1",
		StartedAt:   "2026-09-06T10:00:00Z",
		CompletedAt: "2026-09-06T10:02:00Z",
		DurationMs:  120000,
		SizeBytes:   20,
	})
	insertRecordingMetadata(t, ctx, database, insertRecordingRequest{
		Title:       "part 2",
		StartedAt:   "2026-09-06T10:03:30Z",
		CompletedAt: "2026-09-06T10:08:00Z",
		DurationMs:  270000,
		SizeBytes:   30,
	})
	insertRecordingMetadata(t, ctx, database, insertRecordingRequest{
		Title:       "part 3",
		StartedAt:   "2026-09-06T10:12:30Z",
		CompletedAt: "2026-09-06T10:18:00Z",
		DurationMs:  330000,
		SizeBytes:   40,
	})

	groups, err := NewStore(database, cfg).ListGroups(ctx, actor, RecordingGroupListRequest{
		MaxGapSeconds:         120,
		ShortThresholdSeconds: 180,
	})
	if err != nil {
		t.Fatalf("ListGroups returned error: %v", err)
	}
	if groups.Total != 2 || len(groups.Items) != 2 {
		t.Fatalf("expected two groups, got %#v", groups)
	}
	newest := groups.Items[0]
	oldest := groups.Items[1]
	if newest.RecordingCount != 1 || newest.TotalBytes != 40 || newest.ReadyForMerge {
		t.Fatalf("unexpected newest group: %#v", newest)
	}
	if oldest.RecordingCount != 2 || oldest.TotalBytes != 50 || oldest.MaxGapSeconds != 90 || !oldest.ReadyForMerge {
		t.Fatalf("unexpected oldest group: %#v", oldest)
	}
	if !oldest.HasShortSegment {
		t.Fatalf("expected oldest group to include short segment: %#v", oldest)
	}
}

func TestListGroupsUsesDefaultThresholds(t *testing.T) {
	ctx := context.Background()
	cfg, database := openTestDB(t, ctx)
	actor := bootstrapTestAdmin(t, ctx, database)
	if _, err := profile.NewStore(database).Create(ctx, actor, profile.CreateRequest{
		Name:         "7G",
		RoomID:       "1741048619",
		StreamerName: "Streamer",
	}); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	insertRecordingMetadata(t, ctx, database, insertRecordingRequest{
		Title:       "part 1",
		StartedAt:   "2026-09-06T10:00:00Z",
		CompletedAt: "2026-09-06T10:01:00Z",
		DurationMs:  60000,
		SizeBytes:   20,
	})
	insertRecordingMetadata(t, ctx, database, insertRecordingRequest{
		Title:       "part 2",
		StartedAt:   "2026-09-06T10:02:59Z",
		CompletedAt: "2026-09-06T10:05:00Z",
		DurationMs:  121000,
		SizeBytes:   30,
	})

	groups, err := NewStore(database, cfg).ListGroups(ctx, actor, RecordingGroupListRequest{})
	if err != nil {
		t.Fatalf("ListGroups returned error: %v", err)
	}
	if groups.MaxGapSeconds != 600 || groups.ShortThresholdSeconds != 180 {
		t.Fatalf("unexpected thresholds: %#v", groups)
	}
	if len(groups.Items) != 1 || groups.Items[0].RecordingCount != 2 {
		t.Fatalf("expected default gap threshold to combine recordings, got %#v", groups)
	}
}

func TestDiscoverUploadSourcesPersistsContinuousSegments(t *testing.T) {
	ctx := context.Background()
	cfg, database := openTestDB(t, ctx)
	actor := bootstrapTestAdmin(t, ctx, database)
	if _, err := profile.NewStore(database).Create(ctx, actor, profile.CreateRequest{
		Name:         "7G",
		RoomID:       "1741048619",
		StreamerName: "Streamer",
	}); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if _, err := database.ExecContext(ctx, `
		UPDATE recording_profile_runtime
		SET stream_status = 'OFFLINE', recorder_status = 'IDLE'
		WHERE recording_profile_id = 1
	`); err != nil {
		t.Fatalf("update runtime returned error: %v", err)
	}

	insertRecordingMetadata(t, ctx, database, insertRecordingRequest{
		Title:       "part 1",
		StartedAt:   "2026-09-05T10:00:00Z",
		CompletedAt: "2026-09-05T10:03:00Z",
		DurationMs:  180000,
		SizeBytes:   20,
	})
	insertRecordingMetadata(t, ctx, database, insertRecordingRequest{
		Title:       "part 2",
		StartedAt:   "2026-09-05T10:12:00Z",
		CompletedAt: "2026-09-05T10:15:00Z",
		DurationMs:  180000,
		SizeBytes:   30,
	})

	result, err := NewStore(database, cfg).DiscoverUploadSources(ctx, 600)
	if err != nil {
		t.Fatalf("DiscoverUploadSources returned error: %v", err)
	}
	if result.Created != 1 {
		t.Fatalf("expected one upload source, got %#v", result)
	}
	sources, err := NewStore(database, cfg).ListUploadSources(ctx, actor, 600)
	if err != nil {
		t.Fatalf("ListUploadSources returned error: %v", err)
	}
	if len(sources.Items) != 1 {
		t.Fatalf("expected one upload source item, got %#v", sources)
	}
	source := sources.Items[0]
	if source.Status != "MERGE_PENDING" || source.RecordingCount != 2 || source.TotalBytes != 50 || source.MaxGapSeconds != 540 {
		t.Fatalf("unexpected upload source: %#v", source)
	}
	if len(source.Segments) != 2 || source.Segments[1].TimelineStartMs != 180000 || source.Segments[1].TimelineEndMs != 360000 {
		t.Fatalf("unexpected upload source segments: %#v", source.Segments)
	}
	var jobType string
	if err := database.QueryRowContext(ctx, `
		SELECT type
		FROM jobs
		WHERE business_key = 'upload-source:1:merge'
	`).Scan(&jobType); err != nil {
		t.Fatalf("query merge job returned error: %v", err)
	}
	if jobType != "MERGE_UPLOAD_SOURCE" {
		t.Fatalf("unexpected merge job type: %s", jobType)
	}
}

func TestDiscoverUploadSourcesWaitsForAdjacentUnfinishedRecording(t *testing.T) {
	ctx := context.Background()
	cfg, database := openTestDB(t, ctx)
	actor := bootstrapTestAdmin(t, ctx, database)
	if _, err := profile.NewStore(database).Create(ctx, actor, profile.CreateRequest{
		Name:         "7G",
		RoomID:       "1741048619",
		StreamerName: "Streamer",
	}); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if _, err := database.ExecContext(ctx, `
		UPDATE recording_profile_runtime
		SET stream_status = 'OFFLINE', recorder_status = 'IDLE'
		WHERE recording_profile_id = 1
	`); err != nil {
		t.Fatalf("update runtime returned error: %v", err)
	}

	insertRecordingMetadata(t, ctx, database, insertRecordingRequest{
		Title:       "part 1",
		StartedAt:   "2026-09-05T10:00:00Z",
		CompletedAt: "2026-09-05T10:03:00Z",
		DurationMs:  180000,
		SizeBytes:   20,
	})
	result, err := database.ExecContext(ctx, `
		INSERT INTO recordings
			(recording_profile_id, title, started_at, recording_status, local_storage_status,
				source_room_id, streamer_name_snapshot)
		VALUES (1, 'part 2', '2026-09-05T10:03:07Z', 'ACTIVE', 'AVAILABLE',
			'1741048619', 'Streamer')
	`)
	if err != nil {
		t.Fatalf("insert active recording returned error: %v", err)
	}
	recordingID, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("LastInsertId returned error: %v", err)
	}
	if _, err := database.ExecContext(ctx, `
		INSERT INTO recording_files
			(recording_id, relative_path, original_name, kind, file_status, size_bytes, opened_at)
		VALUES (?, 'recordings/1741048619-Streamer/part 2.flv', 'part 2.flv',
			'video', 'WRITING', 30, '2026-09-05T10:03:07Z')
	`, recordingID); err != nil {
		t.Fatalf("insert writing file returned error: %v", err)
	}

	discover, err := NewStore(database, cfg).DiscoverUploadSources(ctx, 600)
	if err != nil {
		t.Fatalf("DiscoverUploadSources returned error: %v", err)
	}
	if discover.Created != 0 || discover.Delayed != 1 {
		t.Fatalf("expected upload source creation to wait, got %#v", discover)
	}
	sources, err := NewStore(database, cfg).ListUploadSources(ctx, actor, 600)
	if err != nil {
		t.Fatalf("ListUploadSources returned error: %v", err)
	}
	if len(sources.Items) != 0 {
		t.Fatalf("expected no upload source while adjacent recording is unfinished, got %#v", sources)
	}
}

func TestDiscoverUploadSourcesWaitsForAdjacentActiveFile(t *testing.T) {
	ctx := context.Background()
	cfg, database := openTestDB(t, ctx)
	actor := bootstrapTestAdmin(t, ctx, database)
	if _, err := profile.NewStore(database).Create(ctx, actor, profile.CreateRequest{
		Name:         "7G",
		RoomID:       "1741048619",
		StreamerName: "Streamer",
	}); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if _, err := database.ExecContext(ctx, `
		UPDATE recording_profile_runtime
		SET stream_status = 'OFFLINE', recorder_status = 'IDLE'
		WHERE recording_profile_id = 1
	`); err != nil {
		t.Fatalf("update runtime returned error: %v", err)
	}

	insertRecordingMetadata(t, ctx, database, insertRecordingRequest{
		Title:       "part 1",
		StartedAt:   "2026-09-05T10:00:00Z",
		CompletedAt: "2026-09-05T10:03:00Z",
		DurationMs:  180000,
		SizeBytes:   20,
	})
	recordingDir := filepath.Join(cfg.DataRoot, "recordings", "1741048619-Streamer")
	if err := os.MkdirAll(recordingDir, 0o755); err != nil {
		t.Fatalf("MkdirAll recording dir returned error: %v", err)
	}
	activePath := filepath.Join(recordingDir, "record-1741048619-20260905-180307-001-part 2.flv")
	if err := os.WriteFile(activePath, []byte("active video"), 0o644); err != nil {
		t.Fatalf("WriteFile active returned error: %v", err)
	}

	discover, err := NewStore(database, cfg).DiscoverUploadSources(ctx, 600)
	if err != nil {
		t.Fatalf("DiscoverUploadSources returned error: %v", err)
	}
	if discover.Created != 0 || discover.Delayed != 1 {
		t.Fatalf("expected active filesystem recording to delay upload source, got %#v", discover)
	}
}

func TestDiscoverUploadSourcesIgnoresDerivedActiveFiles(t *testing.T) {
	ctx := context.Background()
	cfg, database := openTestDB(t, ctx)
	actor := bootstrapTestAdmin(t, ctx, database)
	if _, err := profile.NewStore(database).Create(ctx, actor, profile.CreateRequest{
		Name:         "7G",
		RoomID:       "1741048619",
		StreamerName: "Streamer",
	}); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if _, err := database.ExecContext(ctx, `
		UPDATE recording_profile_runtime
		SET stream_status = 'OFFLINE', recorder_status = 'IDLE'
		WHERE recording_profile_id = 1
	`); err != nil {
		t.Fatalf("update runtime returned error: %v", err)
	}

	insertRecordingMetadata(t, ctx, database, insertRecordingRequest{
		Title:       "part 1",
		StartedAt:   "2026-09-05T10:00:00Z",
		CompletedAt: "2026-09-05T10:03:00Z",
		DurationMs:  180000,
		SizeBytes:   20,
	})
	derivedDir := filepath.Join(cfg.DataRoot, "upload-sources", "1", "1", "parts")
	if err := os.MkdirAll(derivedDir, 0o755); err != nil {
		t.Fatalf("MkdirAll derived dir returned error: %v", err)
	}
	derivedPath := filepath.Join(derivedDir, "record-1741048619-20260905-180307-001-derived.flv")
	if err := os.WriteFile(derivedPath, []byte("derived video"), 0o644); err != nil {
		t.Fatalf("WriteFile derived returned error: %v", err)
	}

	discover, err := NewStore(database, cfg).DiscoverUploadSources(ctx, 600)
	if err != nil {
		t.Fatalf("DiscoverUploadSources returned error: %v", err)
	}
	if discover.Created != 1 || discover.Delayed != 0 {
		t.Fatalf("expected derived files outside recordings root to be ignored, got %#v", discover)
	}
}

func TestDiscoverUploadSourcesMarksSingleSegmentPendingPackage(t *testing.T) {
	ctx := context.Background()
	cfg, database := openTestDB(t, ctx)
	actor := bootstrapTestAdmin(t, ctx, database)
	if _, err := profile.NewStore(database).Create(ctx, actor, profile.CreateRequest{
		Name:         "7G",
		RoomID:       "1741048619",
		StreamerName: "Streamer",
	}); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if _, err := database.ExecContext(ctx, `
		UPDATE recording_profile_runtime
		SET stream_status = 'OFFLINE', recorder_status = 'IDLE'
		WHERE recording_profile_id = 1
	`); err != nil {
		t.Fatalf("update runtime returned error: %v", err)
	}
	insertRecordingMetadata(t, ctx, database, insertRecordingRequest{
		Title:       "single",
		StartedAt:   "2026-09-05T10:00:00Z",
		CompletedAt: "2026-09-05T10:03:00Z",
		DurationMs:  180000,
		SizeBytes:   20,
	})

	result, err := NewStore(database, cfg).DiscoverUploadSources(ctx, 600)
	if err != nil {
		t.Fatalf("DiscoverUploadSources returned error: %v", err)
	}
	if result.Created != 1 {
		t.Fatalf("expected one upload source, got %#v", result)
	}
	sources, err := NewStore(database, cfg).ListUploadSources(ctx, actor, 600)
	if err != nil {
		t.Fatalf("ListUploadSources returned error: %v", err)
	}
	if len(sources.Items) != 1 || sources.Items[0].Status != "PACKAGE_PENDING" || sources.Items[0].OutputRecordingFileID == 0 {
		t.Fatalf("expected single segment to be pending package, got %#v", sources)
	}
}

func TestUploadSourcePackageBaseNameUsesChinaDateOrdinal(t *testing.T) {
	ctx := context.Background()
	cfg, database := openTestDB(t, ctx)
	actor := bootstrapTestAdmin(t, ctx, database)
	if _, err := profile.NewStore(database).Create(ctx, actor, profile.CreateRequest{
		Name:         "7G",
		RoomID:       "1741048619",
		StreamerName: "Streamer",
	}); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	store := NewStore(database, cfg)
	for _, row := range []struct {
		ID        int64
		StartedAt string
	}{
		{ID: 1, StartedAt: "2026-09-05T15:00:00Z"},
		{ID: 2, StartedAt: "2026-09-05T16:00:00Z"},
		{ID: 3, StartedAt: "2026-09-06T02:00:00Z"},
	} {
		if _, err := database.ExecContext(ctx, `
			INSERT INTO upload_sources
				(id, recording_profile_id, source_key, source_room_id, streamer_name_snapshot,
					started_at, completed_at, duration_ms, status, output_relative_path,
					total_bytes, recording_count, file_count, max_gap_seconds, merge_gap_threshold_seconds)
			VALUES (?, 1, ?, '1741048619', 'Streamer',
				?, '2026-09-06T03:00:00Z', 1800000, 'PACKAGE_PENDING',
				'upload-sources/1/1/upload-source-1.flv', 50, 1, 1, 0, 600)
		`, row.ID, fmt.Sprintf("profile:1:%d:%d", row.ID, row.ID), row.StartedAt); err != nil {
			t.Fatalf("insert upload source returned error: %v", err)
		}
	}

	baseName, err := store.UploadSourcePackageBaseName(ctx, UploadSource{
		ID:                 2,
		RecordingProfileID: 1,
		ProfileName:        "7G",
		StartedAt:          "2026-09-05T16:00:00Z",
	})
	if err != nil {
		t.Fatalf("UploadSourcePackageBaseName returned error: %v", err)
	}
	if baseName != "7G-20260906-\u7b2c01\u573a\u76f4\u64ad" {
		t.Fatalf("unexpected base name: %q", baseName)
	}
}

func TestUploadSourceCOSStatusUsesCurrentOutputObjects(t *testing.T) {
	ctx := context.Background()
	cfg, database := openTestDB(t, ctx)
	actor := bootstrapTestAdmin(t, ctx, database)
	if _, err := profile.NewStore(database).Create(ctx, actor, profile.CreateRequest{
		Name:         "7G",
		RoomID:       "1741048619",
		StreamerName: "Streamer",
	}); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if _, err := database.ExecContext(ctx, `
		INSERT INTO credentials (id, owner_user_id, scope, platform, purpose, account_label, encrypted_secret, status)
		VALUES (1, 1, 'USER', 'tencent_cos', 'STORAGE', 'cos account', X'00', 'UNVERIFIED');
		INSERT INTO cos_storage_profiles
			(id, recording_profile_id, credential_id, enabled, region, bucket, prefix, max_managed_bytes)
		VALUES (1, 1, 1, 1, 'ap-shanghai', 'bucket-1250000000', '7grecorder/test/', 1000000000);
		INSERT INTO upload_sources
			(id, recording_profile_id, source_key, title, source_room_id, streamer_name_snapshot,
				started_at, completed_at, duration_ms, status, output_relative_path,
				total_bytes, recording_count, file_count, max_gap_seconds, merge_gap_threshold_seconds, ready_at)
		VALUES (1, 1, 'profile:1:1:1', 'ready upload', '1741048619', 'Streamer',
			'2026-09-05T10:00:00Z', '2026-09-05T10:30:00Z', 1800000, 'READY_TO_UPLOAD',
			'upload-sources/1/1/parts/7G-20260905-p01.flv', 50, 1, 1, 0, 600, CURRENT_TIMESTAMP);
		INSERT INTO upload_source_outputs
			(id, upload_source_id, sort_order, relative_path, size_bytes, duration_ms, timeline_start_ms, timeline_end_ms, status)
		VALUES
			(1, 1, 0, 'upload-sources/1/1/parts/7G-20260905-p01.flv', 50, 1800000, 0, 1800000, 'READY_TO_UPLOAD');
		INSERT INTO upload_source_cos_objects
			(cos_storage_profile_id, recording_profile_id, upload_source_id, upload_source_output_id, object_key, size_bytes, status)
		VALUES
			(1, 1, 1, NULL, '7grecorder/test/old-upload-source.flv', 50, 'FAILED'),
			(1, 1, 1, 1, '7grecorder/test/upload-sources/1/1/parts/7G-20260905-p01.flv', 50, 'AVAILABLE');
	`); err != nil {
		t.Fatalf("seed upload source returned error: %v", err)
	}

	sources, err := NewStore(database, cfg).ListUploadSources(ctx, actor, 600)
	if err != nil {
		t.Fatalf("ListUploadSources returned error: %v", err)
	}
	if len(sources.Items) != 1 {
		t.Fatalf("expected one upload source, got %#v", sources)
	}
	if sources.Items[0].COSStatus != "AVAILABLE" {
		t.Fatalf("expected current output COS status to win, got %q", sources.Items[0].COSStatus)
	}
	if len(sources.Items[0].Outputs) != 1 || sources.Items[0].Outputs[0].COSStatus != "AVAILABLE" {
		t.Fatalf("unexpected output statuses: %#v", sources.Items[0].Outputs)
	}
}

func TestMarkUploadSourcePackageSucceededUpdatesOutputsReferencedByCOS(t *testing.T) {
	ctx := context.Background()
	cfg, database := openTestDB(t, ctx)
	actor := bootstrapTestAdmin(t, ctx, database)
	if _, err := profile.NewStore(database).Create(ctx, actor, profile.CreateRequest{
		Name:         "7G",
		RoomID:       "1741048619",
		StreamerName: "Streamer",
	}); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if _, err := database.ExecContext(ctx, `
		INSERT INTO credentials (id, owner_user_id, scope, platform, purpose, account_label, encrypted_secret, status)
		VALUES (1, 1, 'USER', 'tencent_cos', 'STORAGE', 'cos account', X'00', 'UNVERIFIED');
		INSERT INTO cos_storage_profiles
			(id, recording_profile_id, credential_id, enabled, region, bucket, prefix, max_managed_bytes)
		VALUES (1, 1, 1, 1, 'ap-shanghai', 'bucket-1250000000', '7grecorder/test/', 1000000000);
		INSERT INTO upload_sources
			(id, recording_profile_id, source_key, title, source_room_id, streamer_name_snapshot,
				started_at, completed_at, duration_ms, status, output_relative_path,
				total_bytes, recording_count, file_count, max_gap_seconds, merge_gap_threshold_seconds)
		VALUES (1, 1, 'profile:1:1:1', 'ready upload', '1741048619', 'Streamer',
			'2026-09-05T10:00:00Z', '2026-09-05T10:30:00Z', 1800000, 'PACKAGE_PENDING',
			'upload-sources/1/1/upload-source-1.flv', 50, 1, 1, 0, 600);
		INSERT INTO upload_source_outputs
			(id, upload_source_id, sort_order, relative_path, size_bytes, duration_ms, timeline_start_ms, timeline_end_ms, status)
		VALUES
			(1, 1, 0, 'upload-sources/1/1/parts/old.flv', 50, 1800000, 0, 1800000, 'READY_TO_UPLOAD');
		INSERT INTO upload_source_cos_objects
			(cos_storage_profile_id, recording_profile_id, upload_source_id, upload_source_output_id, object_key, size_bytes, status)
		VALUES
			(1, 1, 1, 1, '7grecorder/test/upload-sources/1/1/parts/old.flv', 50, 'AVAILABLE');
	`); err != nil {
		t.Fatalf("seed upload source returned error: %v", err)
	}

	if err := NewStore(database, cfg).MarkUploadSourcePackageSucceeded(ctx, 1, []media.PackageOutput{{
		RelativePath:    "upload-sources/1/1/parts/new.flv",
		SizeBytes:       75,
		DurationMs:      1900000,
		TimelineStartMs: 0,
		TimelineEndMs:   1900000,
	}}); err != nil {
		t.Fatalf("MarkUploadSourcePackageSucceeded returned error: %v", err)
	}
	var outputID int64
	var outputPath string
	var outputSize int64
	if err := database.QueryRowContext(ctx, `
		SELECT id, relative_path, size_bytes
		FROM upload_source_outputs
		WHERE upload_source_id = 1 AND sort_order = 0
	`).Scan(&outputID, &outputPath, &outputSize); err != nil {
		t.Fatalf("query output returned error: %v", err)
	}
	if outputID != 1 || outputPath != "upload-sources/1/1/parts/new.flv" || outputSize != 75 {
		t.Fatalf("unexpected updated output id=%d path=%q size=%d", outputID, outputPath, outputSize)
	}
	var referencedOutputID int64
	if err := database.QueryRowContext(ctx, `
		SELECT upload_source_output_id
		FROM upload_source_cos_objects
		WHERE id = 1
	`).Scan(&referencedOutputID); err != nil {
		t.Fatalf("query cos object returned error: %v", err)
	}
	if referencedOutputID != outputID {
		t.Fatalf("expected COS object to keep output id %d, got %d", outputID, referencedOutputID)
	}
}

func TestRegroupUploadSourcesReplacesFragmentedSources(t *testing.T) {
	ctx := context.Background()
	cfg, database := openTestDB(t, ctx)
	actor := bootstrapTestAdmin(t, ctx, database)
	if _, err := profile.NewStore(database).Create(ctx, actor, profile.CreateRequest{
		Name:         "7G",
		RoomID:       "1741048619",
		StreamerName: "Streamer",
	}); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if _, err := database.ExecContext(ctx, `
		UPDATE recording_profile_runtime
		SET stream_status = 'OFFLINE', recorder_status = 'IDLE'
		WHERE recording_profile_id = 1
	`); err != nil {
		t.Fatalf("update runtime returned error: %v", err)
	}
	insertRecordingMetadata(t, ctx, database, insertRecordingRequest{
		Title:       "part 1",
		StartedAt:   "2026-09-08T10:00:00Z",
		CompletedAt: "2026-09-08T10:03:00Z",
		DurationMs:  180000,
		SizeBytes:   20,
	})
	insertRecordingMetadata(t, ctx, database, insertRecordingRequest{
		Title:       "part 2",
		StartedAt:   "2026-09-08T10:04:00Z",
		CompletedAt: "2026-09-08T10:07:00Z",
		DurationMs:  180000,
		SizeBytes:   30,
	})
	store := NewStore(database, cfg)
	if _, err := store.DiscoverUploadSources(ctx, 1); err != nil {
		t.Fatalf("DiscoverUploadSources returned error: %v", err)
	}
	before, err := store.ListUploadSources(ctx, actor, 600)
	if err != nil {
		t.Fatalf("ListUploadSources returned error: %v", err)
	}
	if len(before.Items) != 2 {
		t.Fatalf("expected fragmented upload sources before regroup, got %#v", before.Items)
	}

	result, err := store.RegroupUploadSources(ctx, actor, UploadSourceRegroupRequest{
		RecordingProfileID: 1,
		ChinaDate:          "2026-09-08",
		MergeGapSeconds:    600,
	})
	if err != nil {
		t.Fatalf("RegroupUploadSources returned error: %v", err)
	}
	if result.ReplacedSources != 2 || result.CreatedSources != 1 || result.CancelledJobs != 2 || len(result.Blocked) != 0 {
		t.Fatalf("unexpected regroup result: %#v", result)
	}
	after, err := store.ListUploadSources(ctx, actor, 600)
	if err != nil {
		t.Fatalf("ListUploadSources returned error: %v", err)
	}
	if len(after.Items) != 1 || after.Items[0].RecordingCount != 2 || after.Items[0].Status != "MERGE_PENDING" {
		t.Fatalf("expected one regrouped source, got %#v", after.Items)
	}
	if len(after.Items[0].Segments) != 2 || after.Items[0].Segments[1].TimelineStartMs != 180000 {
		t.Fatalf("unexpected regrouped segments: %#v", after.Items[0].Segments)
	}
	var replaced int
	if err := database.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM upload_sources WHERE status = 'REPLACED'
	`).Scan(&replaced); err != nil {
		t.Fatalf("query replaced sources returned error: %v", err)
	}
	if replaced != 2 {
		t.Fatalf("expected two replaced sources, got %d", replaced)
	}
}

func TestRegroupUploadSourcesBlocksBilibiliPublication(t *testing.T) {
	ctx := context.Background()
	cfg, database := openTestDB(t, ctx)
	actor := bootstrapTestAdmin(t, ctx, database)
	if _, err := profile.NewStore(database).Create(ctx, actor, profile.CreateRequest{
		Name:         "7G",
		RoomID:       "1741048619",
		StreamerName: "Streamer",
	}); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if _, err := database.ExecContext(ctx, `
		UPDATE recording_profile_runtime
		SET stream_status = 'OFFLINE', recorder_status = 'IDLE'
		WHERE recording_profile_id = 1
	`); err != nil {
		t.Fatalf("update runtime returned error: %v", err)
	}
	insertRecordingMetadata(t, ctx, database, insertRecordingRequest{
		Title:       "part 1",
		StartedAt:   "2026-09-08T10:00:00Z",
		CompletedAt: "2026-09-08T10:03:00Z",
		DurationMs:  180000,
		SizeBytes:   20,
	})
	insertRecordingMetadata(t, ctx, database, insertRecordingRequest{
		Title:       "part 2",
		StartedAt:   "2026-09-08T10:04:00Z",
		CompletedAt: "2026-09-08T10:07:00Z",
		DurationMs:  180000,
		SizeBytes:   30,
	})
	store := NewStore(database, cfg)
	if _, err := store.DiscoverUploadSources(ctx, 1); err != nil {
		t.Fatalf("DiscoverUploadSources returned error: %v", err)
	}
	if _, err := database.ExecContext(ctx, `
		INSERT INTO publications
			(recording_profile_id, upload_source_id, platform, status, attempts)
		VALUES (1, 1, 'bilibili', 'PENDING', 0)
	`); err != nil {
		t.Fatalf("insert publication returned error: %v", err)
	}

	result, err := store.RegroupUploadSources(ctx, actor, UploadSourceRegroupRequest{
		RecordingProfileID: 1,
		ChinaDate:          "2026-09-08",
		MergeGapSeconds:    600,
	})
	if err != nil {
		t.Fatalf("RegroupUploadSources returned error: %v", err)
	}
	if result.CreatedSources != 0 || len(result.Blocked) != 1 || result.Blocked[0].Reason != "BILIBILI_PUBLICATION_EXISTS" {
		t.Fatalf("expected bilibili publication block, got %#v", result)
	}
	after, err := store.ListUploadSources(ctx, actor, 600)
	if err != nil {
		t.Fatalf("ListUploadSources returned error: %v", err)
	}
	if len(after.Items) != 2 {
		t.Fatalf("expected fragmented sources to remain visible, got %#v", after.Items)
	}
}

func TestRepairUploadSourcesResetsMissingDerivedFilesToMerge(t *testing.T) {
	ctx := context.Background()
	cfg, database := openTestDB(t, ctx)
	actor := bootstrapTestAdmin(t, ctx, database)
	if _, err := profile.NewStore(database).Create(ctx, actor, profile.CreateRequest{
		Name:         "7G",
		RoomID:       "1741048619",
		StreamerName: "Streamer",
	}); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if _, err := database.ExecContext(ctx, `
		UPDATE recording_profile_runtime
		SET stream_status = 'OFFLINE', recorder_status = 'IDLE'
		WHERE recording_profile_id = 1
	`); err != nil {
		t.Fatalf("update runtime returned error: %v", err)
	}
	insertRecordingMetadata(t, ctx, database, insertRecordingRequest{
		Title:       "part 1",
		StartedAt:   "2026-09-08T10:00:00Z",
		CompletedAt: "2026-09-08T10:03:00Z",
		DurationMs:  180000,
		SizeBytes:   20,
	})
	insertRecordingMetadata(t, ctx, database, insertRecordingRequest{
		Title:       "part 2",
		StartedAt:   "2026-09-08T10:04:00Z",
		CompletedAt: "2026-09-08T10:07:00Z",
		DurationMs:  180000,
		SizeBytes:   30,
	})
	for _, relativePath := range []string{
		"recordings/1741048619-Streamer/part 1.flv",
		"recordings/1741048619-Streamer/part 2.flv",
	} {
		path := filepath.Join(cfg.DataRoot, relativePath)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("create recording file dir returned error: %v", err)
		}
		if err := os.WriteFile(path, []byte("video"), 0o644); err != nil {
			t.Fatalf("write recording file returned error: %v", err)
		}
	}
	store := NewStore(database, cfg)
	if _, err := store.DiscoverUploadSources(ctx, 600); err != nil {
		t.Fatalf("DiscoverUploadSources returned error: %v", err)
	}
	if _, err := database.ExecContext(ctx, `
		UPDATE upload_sources
		SET status = 'PACKAGE_FAILED',
			output_relative_path = 'upload-sources/1/1/upload-source-1.flv',
			last_error = 'package input missing'
		WHERE id = 1;
		UPDATE jobs
		SET status = 'SUCCEEDED'
		WHERE business_key = 'upload-source:1:merge';
		INSERT INTO jobs
			(recording_profile_id, upload_source_id, type, resource_class, business_key, payload_json, status, priority, max_attempts, last_error)
		VALUES
			(1, 1, 'PACKAGE_UPLOAD_SOURCE', 'MEDIA', 'upload-source:1:package', '{"upload_source_id":1}', 'FAILED', 65, 3, 'waiting for merge rerun'),
			(1, 1, 'UPLOAD_BILIBILI', 'NETWORK', 'upload-source:1:bilibili:upload', '{"publication_id":1,"upload_source_id":1}', 'FAILED', 80, 3, 'source file missing');
		INSERT INTO upload_source_outputs
			(upload_source_id, sort_order, relative_path, size_bytes, duration_ms, timeline_start_ms, timeline_end_ms, status)
		VALUES
			(1, 0, 'upload-sources/1/1/parts/upload-source-1-part-000.flv', 20, 180000, 0, 180000, 'READY_TO_UPLOAD');
		INSERT INTO publications
			(id, recording_profile_id, upload_source_id, platform, status, last_error)
		VALUES (1, 1, 1, 'bilibili', 'SOURCE_MISSING', 'source file missing');
	`); err != nil {
		t.Fatalf("seed broken upload source returned error: %v", err)
	}

	result, err := store.RepairUploadSources(ctx, actor)
	if err != nil {
		t.Fatalf("RepairUploadSources returned error: %v", err)
	}
	if result.ResetToMerge != 1 || result.OutputsMarkedMissing != 1 || result.PackageJobsReset != 1 || result.MergeJobsReset != 1 || result.BilibiliPublicationsReset != 1 {
		t.Fatalf("unexpected repair result: %#v", result)
	}
	var sourceStatus string
	var outputPath string
	if err := database.QueryRowContext(ctx, `
		SELECT status, COALESCE(output_relative_path, '')
		FROM upload_sources
		WHERE id = 1
	`).Scan(&sourceStatus, &outputPath); err != nil {
		t.Fatalf("query repaired source returned error: %v", err)
	}
	if sourceStatus != "MERGE_PENDING" || outputPath != "" {
		t.Fatalf("unexpected repaired source status=%s output=%q", sourceStatus, outputPath)
	}
	var mergeJobStatus string
	if err := database.QueryRowContext(ctx, `SELECT status FROM jobs WHERE business_key = 'upload-source:1:merge'`).Scan(&mergeJobStatus); err != nil {
		t.Fatalf("query repaired merge job returned error: %v", err)
	}
	if mergeJobStatus != "PENDING" {
		t.Fatalf("expected merge job PENDING, got %s", mergeJobStatus)
	}
	var packageJobStatus string
	var packageJobError string
	if err := database.QueryRowContext(ctx, `
		SELECT status, COALESCE(last_error, '')
		FROM jobs
		WHERE business_key = 'upload-source:1:package'
	`).Scan(&packageJobStatus, &packageJobError); err != nil {
		t.Fatalf("query repaired package job returned error: %v", err)
	}
	if packageJobStatus != "CANCELLED" || packageJobError != "waiting for merge rerun" {
		t.Fatalf("expected package job waiting for merge, got status=%s error=%q", packageJobStatus, packageJobError)
	}
	var publicationStatus string
	if err := database.QueryRowContext(ctx, `SELECT status FROM publications WHERE id = 1`).Scan(&publicationStatus); err != nil {
		t.Fatalf("query repaired publication returned error: %v", err)
	}
	if publicationStatus != "PENDING" {
		t.Fatalf("expected publication PENDING, got %s", publicationStatus)
	}
	if err := store.MarkUploadSourceMergeSucceeded(ctx, 1, "upload-sources/1/1/upload-source-1.flv", 50); err != nil {
		t.Fatalf("MarkUploadSourceMergeSucceeded returned error: %v", err)
	}
	if err := database.QueryRowContext(ctx, `
		SELECT status, COALESCE(last_error, '')
		FROM jobs
		WHERE business_key = 'upload-source:1:package'
	`).Scan(&packageJobStatus, &packageJobError); err != nil {
		t.Fatalf("query package job after merge returned error: %v", err)
	}
	if packageJobStatus != "PENDING" || packageJobError != "" {
		t.Fatalf("expected package job to resume after merge, got status=%s error=%q", packageJobStatus, packageJobError)
	}
}

func TestRepairUploadSourcesSkipsIntentionalLocalCleanup(t *testing.T) {
	ctx := context.Background()
	cfg, database := openTestDB(t, ctx)
	actor := bootstrapTestAdmin(t, ctx, database)
	if _, err := profile.NewStore(database).Create(ctx, actor, profile.CreateRequest{
		Name: "7G", RoomID: "1741048619", StreamerName: "Streamer",
	}); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if _, err := database.ExecContext(ctx, `
		INSERT INTO upload_sources
			(id, recording_profile_id, source_key, source_room_id, streamer_name_snapshot,
				started_at, completed_at, status, recording_count, local_cleanup_status, local_deleted_at)
		VALUES (1, 1, 'cleaned', '1741048619', 'Streamer',
			'2026-09-05T10:00:00Z', '2026-09-05T11:00:00Z', 'READY_TO_UPLOAD', 1, 'DELETED', CURRENT_TIMESTAMP)
	`); err != nil {
		t.Fatalf("seed cleaned upload source returned error: %v", err)
	}

	result, err := NewStore(database, cfg).RepairUploadSources(ctx, actor)
	if err != nil {
		t.Fatalf("RepairUploadSources returned error: %v", err)
	}
	if result.Checked != 0 || result.ResetToMerge != 0 || result.ResetToPackage != 0 {
		t.Fatalf("expected intentional cleanup to be ignored by repair, got %#v", result)
	}
}

func TestDiscoverUploadSourcesBackfillsMissingMergeJobs(t *testing.T) {
	ctx := context.Background()
	cfg, database := openTestDB(t, ctx)
	actor := bootstrapTestAdmin(t, ctx, database)
	if _, err := profile.NewStore(database).Create(ctx, actor, profile.CreateRequest{
		Name:         "7G",
		RoomID:       "1741048619",
		StreamerName: "Streamer",
	}); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if _, err := database.ExecContext(ctx, `
		UPDATE recording_profile_runtime
		SET stream_status = 'OFFLINE', recorder_status = 'IDLE'
		WHERE recording_profile_id = 1
	`); err != nil {
		t.Fatalf("update runtime returned error: %v", err)
	}
	insertRecordingMetadata(t, ctx, database, insertRecordingRequest{
		Title:       "part 1",
		StartedAt:   "2026-09-05T10:00:00Z",
		CompletedAt: "2026-09-05T10:03:00Z",
		DurationMs:  180000,
		SizeBytes:   20,
	})
	insertRecordingMetadata(t, ctx, database, insertRecordingRequest{
		Title:       "part 2",
		StartedAt:   "2026-09-05T10:04:00Z",
		CompletedAt: "2026-09-05T10:07:00Z",
		DurationMs:  180000,
		SizeBytes:   30,
	})
	if _, err := database.ExecContext(ctx, `
		INSERT INTO upload_sources
			(id, recording_profile_id, source_key, title, source_room_id, streamer_name_snapshot,
				started_at, completed_at, duration_ms, status, total_bytes, recording_count, file_count,
				max_gap_seconds, merge_gap_threshold_seconds)
		VALUES (1, 1, 'profile:1:1:2', 'parts', '1741048619', 'Streamer',
			'2026-09-05T10:00:00Z', '2026-09-05T10:07:00Z', 360000, 'MERGE_PENDING',
			50, 2, 2, 60, 600)
	`); err != nil {
		t.Fatalf("insert upload source returned error: %v", err)
	}
	if _, err := database.ExecContext(ctx, `
		INSERT INTO upload_source_segments
			(upload_source_id, recording_id, recording_file_id, sort_order, source_started_at,
				source_completed_at, timeline_start_ms, timeline_end_ms, relative_path, size_bytes, duration_ms)
		VALUES
			(1, 1, 1, 0, '2026-09-05T10:00:00Z', '2026-09-05T10:03:00Z', 0, 180000,
				'recordings/1741048619-Streamer/part 1.flv', 20, 180000),
			(1, 2, 2, 1, '2026-09-05T10:04:00Z', '2026-09-05T10:07:00Z', 180000, 360000,
				'recordings/1741048619-Streamer/part 2.flv', 30, 180000)
	`); err != nil {
		t.Fatalf("insert upload source segments returned error: %v", err)
	}

	result, err := NewStore(database, cfg).DiscoverUploadSources(ctx, 600)
	if err != nil {
		t.Fatalf("DiscoverUploadSources returned error: %v", err)
	}
	if result.Created != 0 || result.MergeJobsEnqueued != 1 {
		t.Fatalf("expected one backfilled merge job, got %#v", result)
	}
	var jobType string
	if err := database.QueryRowContext(ctx, `
		SELECT type
		FROM jobs
		WHERE business_key = 'upload-source:1:merge'
	`).Scan(&jobType); err != nil {
		t.Fatalf("query merge job returned error: %v", err)
	}
	if jobType != "MERGE_UPLOAD_SOURCE" {
		t.Fatalf("unexpected merge job type: %s", jobType)
	}
}

func TestUpsertLocalStorageSettingsUpdatesPolicyPreview(t *testing.T) {
	ctx := context.Background()
	cfg, database := openTestDB(t, ctx)
	actor := bootstrapTestAdmin(t, ctx, database)
	store := NewStore(database, cfg)

	settings, err := store.UpsertLocalStorageSettings(ctx, actor, LocalStorageSettingsUpsert{
		MaxRecordingBytes:          1,
		MinSystemFreeBytes:         1,
		CleanupTargetRatio:         0.5,
		AbsoluteEmergencyFreeBytes: 1,
	})
	if err != nil {
		t.Fatalf("UpsertLocalStorageSettings returned error: %v", err)
	}
	if settings.MaxRecordingBytes != 1 || settings.CleanupTargetRatio != 0.5 {
		t.Fatalf("unexpected settings: %#v", settings)
	}

	status, err := store.LocalStorageStatus(ctx, actor)
	if err != nil {
		t.Fatalf("LocalStorageStatus returned error: %v", err)
	}
	if !status.SettingsConfigured || status.Settings.MaxRecordingBytes != 1 {
		t.Fatalf("expected configured settings, got %#v", status)
	}
}

type insertRecordingRequest struct {
	Title       string
	StartedAt   string
	CompletedAt string
	DurationMs  int64
	SizeBytes   int64
}

func insertRecordingMetadata(t *testing.T, ctx context.Context, database *sql.DB, req insertRecordingRequest) {
	t.Helper()
	result, err := database.ExecContext(ctx, `
		INSERT INTO recordings
			(recording_profile_id, title, started_at, completed_at, duration_ms, recording_status,
				local_storage_status, source_room_id, streamer_name_snapshot)
		VALUES (1, ?, ?, ?, ?, 'COMPLETED', 'AVAILABLE', '1741048619', 'Streamer')
	`, req.Title, req.StartedAt, req.CompletedAt, req.DurationMs)
	if err != nil {
		t.Fatalf("insert recording returned error: %v", err)
	}
	recordingID, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("LastInsertId returned error: %v", err)
	}
	if _, err := database.ExecContext(ctx, `
		INSERT INTO recording_files
			(recording_id, relative_path, original_name, kind, file_status, size_bytes, duration_ms, closed_at)
		VALUES (?, ?, ?, 'video', 'CLOSED', ?, ?, ?)
	`, recordingID, "recordings/1741048619-Streamer/"+req.Title+".flv", req.Title+".flv", req.SizeBytes, req.DurationMs, req.CompletedAt); err != nil {
		t.Fatalf("insert recording file returned error: %v", err)
	}
}

func seedDeliveredCleanupSource(t *testing.T, ctx context.Context, database *sql.DB, ownerUserID, recordingID, recordingFileID int64) {
	t.Helper()
	if _, err := database.ExecContext(ctx, `
		INSERT INTO credentials
			(id, owner_user_id, scope, platform, purpose, account_label, encrypted_secret, status)
		VALUES (1, ?, 'USER', 'bilibili', 'PUBLISHER', 'cleanup test', X'00', 'UNVERIFIED');
		INSERT INTO publishing_profiles
			(id, recording_profile_id, platform, credential_id, enabled, settings_json)
		VALUES (1, 1, 'bilibili', 1, 1, '{}');
		INSERT INTO upload_sources
			(id, recording_profile_id, source_key, title, source_room_id, streamer_name_snapshot,
				started_at, completed_at, duration_ms, status, total_bytes, recording_count,
				file_count, max_gap_seconds, merge_gap_threshold_seconds, ready_at)
		VALUES
			(1, 1, 'cleanup-old', 'old delivered source', '1741048619', 'Streamer',
				'2026-09-05T10:00:00Z', '2026-09-05T10:30:00Z', 1800000,
				'READY_TO_UPLOAD', 5, 1, 1, 0, 600, CURRENT_TIMESTAMP),
			(2, 1, 'cleanup-newest', 'newest retained source', '1741048619', 'Streamer',
				'2026-09-06T10:00:00Z', '2026-09-06T10:30:00Z', 1800000,
				'MERGE_PENDING', 0, 0, 0, 0, 600, NULL);
		INSERT INTO upload_source_segments
			(upload_source_id, recording_id, recording_file_id, sort_order, source_started_at,
				source_completed_at, timeline_start_ms, timeline_end_ms, relative_path, size_bytes, duration_ms)
		SELECT 1, ?, ?, 0, rec.started_at, rec.completed_at, 0, rec.duration_ms,
			rf.relative_path, rf.size_bytes, rf.duration_ms
		FROM recordings rec
		JOIN recording_files rf ON rf.id = ?
		WHERE rec.id = ?;
		INSERT INTO publications
			(id, recording_profile_id, upload_source_id, platform, credential_id, external_id,
				external_url, status, published_at, verified_at)
		VALUES (1, 1, 1, 'bilibili', 1, 'BV1cleanup', 'https://www.bilibili.com/video/BV1cleanup',
			'VERIFIED', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);
	`, ownerUserID, recordingID, recordingFileID, recordingFileID, recordingID); err != nil {
		t.Fatalf("seed delivered cleanup source returned error: %v", err)
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

func closedTestTime() time.Time {
	return time.Date(2026, 9, 5, 15, 12, 58, 0, time.UTC)
}
