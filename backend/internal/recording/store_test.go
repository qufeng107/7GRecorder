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
	if result.DeletedRecordings != 1 || result.DeletedFiles != 1 || result.ReclaimedBytes != 5 {
		t.Fatalf("unexpected cleanup result: %#v", result)
	}
	if _, err := os.Stat(filePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected file to be deleted, stat err=%v", err)
	}

	items, err := store.List(ctx, actor)
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(items) != 1 || items[0].LocalStorageStatus != "DELETED" || items[0].Files[0].FileStatus != "DELETED" {
		t.Fatalf("expected deleted metadata, got %#v", items)
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
}

func TestDiscoverUploadSourcesMarksSingleSegmentReady(t *testing.T) {
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
	if len(sources.Items) != 1 || sources.Items[0].Status != "READY_TO_UPLOAD" || sources.Items[0].OutputRecordingFileID == 0 {
		t.Fatalf("expected single segment to be ready, got %#v", sources)
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
