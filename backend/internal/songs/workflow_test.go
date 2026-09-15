package songs_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/7grecorder/7grecorder/backend/internal/songs"
	"github.com/7grecorder/7grecorder/backend/internal/upload"
)

func TestRecognitionPersistsSongArtifactAndPlayableCache(t *testing.T) {
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
		Enabled: true, CredentialID: acrCredential.ID, Region: "eu-west-1", ContainerID: "100",
		DestinationCOSStorageProfileID: 1, SongsPrefix: "songs", BoundaryPaddingMs: 500, AlgorithmVersion: "mvp1",
	}); err != nil {
		t.Fatal(err)
	}
	run, err := store.CreateRun(ctx, actor, songs.CreateRunRequest{COSObjectID: 1})
	if err != nil {
		t.Fatal(err)
	}
	var downloadJobID int64
	if err := database.QueryRowContext(ctx, `SELECT id FROM jobs WHERE type = 'DOWNLOAD_SONG_SOURCE'`).Scan(&downloadJobID); err != nil {
		t.Fatal(err)
	}
	download, err := store.DownloadRequest(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepathDir(download.Destination), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(download.Destination, make([]byte, 50), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := store.MarkDownloading(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.MarkDownloaded(ctx, downloadJobID, run.ID); err != nil {
		t.Fatal(err)
	}
	var processJobs int
	if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM jobs WHERE type = 'PROCESS_SONG_ANALYSIS' AND resource_class = 'AI'`).Scan(&processJobs); err != nil || processJobs != 1 {
		t.Fatalf("process job count=%d err=%v", processJobs, err)
	}
	process, err := store.ProcessRequest(ctx, run.ID)
	if err != nil || process.AccessToken != "token" {
		t.Fatalf("process request: %#v err=%v", process, err)
	}
	if err := store.MarkRecognizing(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.MarkProviderSubmitted(ctx, run.ID, process.ProviderName, "file-1", 1); err != nil {
		t.Fatal(err)
	}
	songIDs, err := store.PersistRecognition(ctx, run.ID, songs.RecognitionResult{ProviderFileID: "file-1", RawJSON: `{"data":{}}`, Matches: []songs.RecognitionMatch{{
		Engine: "ACRCLOUD_FINGERPRINT", ACRID: "acr-1", Title: "Test Song", Artist: "Singer", StartMs: 10_000, EndMs: 40_000, Score: 97, EvidenceJSON: `{}`,
	}}})
	if err != nil || len(songIDs) != 1 {
		t.Fatalf("persist recognition: %#v err=%v", songIDs, err)
	}
	artifact, err := store.AudioArtifactRequest(ctx, songIDs[0])
	if err != nil {
		t.Fatal(err)
	}
	if artifact.StartOffsetMs != 9500 || artifact.EndOffsetMs != 40500 || artifact.COSRequest.ObjectKey != "songs/1/1/audio-r1.m4a" {
		t.Fatalf("unexpected artifact: %#v", artifact)
	}
	if err := store.MarkAudioGenerating(ctx, artifact); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepathDir(artifact.DestinationPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(artifact.DestinationPath, []byte("m4a"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := store.MarkAudioAvailable(ctx, artifact, upload.COSUploadResult{ETag: "audio-etag", SizeBytes: 3}); err != nil {
		t.Fatal(err)
	}
	if err := store.MarkRunCompleted(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	items, err := store.ListSongs(ctx, actor)
	if err != nil || len(items) != 1 || items[0].AudioURL == "" || items[0].AudioArtifactStatus != "AVAILABLE" {
		t.Fatalf("songs: %#v err=%v", items, err)
	}
	playback, err := store.AudioForPlayback(ctx, actor, songIDs[0])
	if err != nil || playback.AbsolutePath != artifact.DestinationPath {
		t.Fatalf("playback: %#v err=%v", playback, err)
	}
}

func TestAudioCacheCapacityEvictsOnlyExpiredUnleasedEntries(t *testing.T) {
	ctx := context.Background()
	cfg, database, actor := openSongsTestDB(t, ctx)
	if _, err := database.ExecContext(ctx, `INSERT INTO local_storage_settings
		(id, max_recording_bytes, min_system_free_bytes, cleanup_target_ratio, absolute_emergency_free_bytes, updated_by_user_id)
		VALUES (1, 100, 1, 0.85, 1, ?)`, actor.ID); err != nil {
		t.Fatal(err)
	}
	oldRelative := "songs/cache/audio/old.m4a"
	oldPath := filepath.Join(cfg.DataRoot, filepath.FromSlash(oldRelative))
	if err := os.MkdirAll(filepath.Dir(oldPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(oldPath, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `INSERT INTO media_cache_entries
		(kind, cache_key, relative_path, size_bytes, status, last_accessed_at, grace_until)
		VALUES ('SONG_AUDIO', 'old', ?, 3, 'AVAILABLE', datetime('now', '-2 hours'), datetime('now', '-1 hour'))`, oldRelative); err != nil {
		t.Fatal(err)
	}
	store := songs.NewStore(database, cfg)
	if err := store.EnsureAudioCacheCapacity(ctx, 3); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("expected expired cache to be removed, stat err=%v", err)
	}
	protectedRelative := "songs/cache/audio/protected.m4a"
	protectedPath := filepath.Join(cfg.DataRoot, filepath.FromSlash(protectedRelative))
	if err := os.WriteFile(protectedPath, []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `INSERT INTO media_cache_entries
		(kind, cache_key, relative_path, size_bytes, status, grace_until)
		VALUES ('SONG_AUDIO', 'protected', ?, 3, 'AVAILABLE', datetime('now', '+10 minutes'))`, protectedRelative); err != nil {
		t.Fatal(err)
	}
	if err := store.EnsureAudioCacheCapacity(ctx, 3); !errors.Is(err, songs.ErrWaitingForSpace) {
		t.Fatalf("expected protected cache to defer, got %v", err)
	}
	if _, err := os.Stat(protectedPath); err != nil {
		t.Fatalf("protected cache was removed: %v", err)
	}
}

func TestCancelledRunRejectsLateRecognitionAndFailureUpdates(t *testing.T) {
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
		Enabled: true, CredentialID: acrCredential.ID, Region: "eu-west-1", ContainerID: "100",
		DestinationCOSStorageProfileID: 1, SongsPrefix: "songs", AlgorithmVersion: "mvp1",
	}); err != nil {
		t.Fatal(err)
	}
	run, err := store.CreateRun(ctx, actor, songs.CreateRunRequest{COSObjectID: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `UPDATE song_analysis_runs SET status = 'CANCELLED' WHERE id = ?`, run.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.PersistRecognition(ctx, run.ID, songs.RecognitionResult{}); err == nil {
		t.Fatal("expected late recognition persistence to be rejected")
	}
	if err := store.MarkProcessFailed(ctx, run.ID, "TRANSIENT", "late failure"); err != nil {
		t.Fatal(err)
	}
	if err := store.MarkRunCompleted(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	var status string
	if err := database.QueryRowContext(ctx, `SELECT status FROM song_analysis_runs WHERE id = ?`, run.ID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "CANCELLED" {
		t.Fatalf("late processing changed cancelled run to %q", status)
	}
}

func filepathDir(path string) string { return filepath.Dir(path) }
