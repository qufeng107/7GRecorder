package upload

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/7grecorder/7grecorder/backend/internal/account"
	"github.com/7grecorder/7grecorder/backend/internal/config"
	"github.com/7grecorder/7grecorder/backend/internal/db"
	"github.com/7grecorder/7grecorder/backend/internal/profile"
)

func TestCreateCredentialEncryptsSecret(t *testing.T) {
	ctx := context.Background()
	cfg, database := openTestDB(t, ctx)
	actor := bootstrapTestAdmin(t, ctx, database)

	created, err := NewStore(database, cfg).CreateCredential(ctx, actor, CredentialCreate{
		Scope:        "USER",
		Platform:     "bilibili",
		Purpose:      "PUBLISHER",
		AccountLabel: "bili account",
		Secret:       []byte(`{"cookie":"secret-cookie"}`),
	})
	if err != nil {
		t.Fatalf("CreateCredential returned error: %v", err)
	}
	if created.ID == 0 || created.AccountLabel != "bili account" {
		t.Fatalf("unexpected credential: %#v", created)
	}

	var encrypted []byte
	if err := database.QueryRowContext(ctx, `
		SELECT encrypted_secret FROM credentials WHERE id = ?
	`, created.ID).Scan(&encrypted); err != nil {
		t.Fatalf("query encrypted secret returned error: %v", err)
	}
	if string(encrypted) == `{"cookie":"secret-cookie"}` {
		t.Fatal("secret was stored as plaintext")
	}
}

func TestReconcileCreatesUploadModuleJobsForReadySources(t *testing.T) {
	ctx := context.Background()
	cfg, database := openTestDB(t, ctx)
	actor := bootstrapTestAdmin(t, ctx, database)
	if _, err := profile.NewStore(database).Create(ctx, actor, profile.CreateRequest{
		Name:         "7G",
		RoomID:       "1741048619",
		StreamerName: "Streamer",
	}); err != nil {
		t.Fatalf("Create profile returned error: %v", err)
	}
	store := NewStore(database, cfg)
	biliCredential, err := store.CreateCredential(ctx, actor, CredentialCreate{
		Scope:        "USER",
		Platform:     "bilibili",
		Purpose:      "PUBLISHER",
		AccountLabel: "bili account",
		Secret:       []byte(`{"cookie":"cookie"}`),
	})
	if err != nil {
		t.Fatalf("CreateCredential bilibili returned error: %v", err)
	}
	cosCredential, err := store.CreateCredential(ctx, actor, CredentialCreate{
		Scope:        "USER",
		Platform:     "tencent_cos",
		Purpose:      "STORAGE",
		AccountLabel: "cos account",
		Secret:       []byte(`{"secret_id":"id","secret_key":"key"}`),
	})
	if err != nil {
		t.Fatalf("CreateCredential cos returned error: %v", err)
	}
	if _, err := store.UpsertBilibiliConfig(ctx, actor, 1, PublishingConfigUpsert{
		CredentialID: biliCredential.ID,
		Enabled:      true,
		Settings:     []byte(`{"copyright":2}`),
	}); err != nil {
		t.Fatalf("UpsertBilibiliConfig returned error: %v", err)
	}
	if _, err := store.UpsertCOSConfig(ctx, actor, 1, COSConfigUpsert{
		CredentialID:    cosCredential.ID,
		Enabled:         true,
		Region:          "ap-shanghai",
		Bucket:          "bucket-1250000000",
		Prefix:          "7grecorder/test/",
		MaxManagedBytes: 1000000000,
	}); err != nil {
		t.Fatalf("UpsertCOSConfig returned error: %v", err)
	}
	insertReadyUploadSource(t, ctx, database)

	result, err := store.Reconcile(ctx, actor)
	if err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}
	if result.PublicationsCreated != 1 || result.BilibiliJobsCreated != 1 || result.COSObjectsCreated != 1 || result.COSJobsCreated != 1 {
		t.Fatalf("unexpected reconcile result: %#v", result)
	}

	second, err := store.Reconcile(ctx, actor)
	if err != nil {
		t.Fatalf("second Reconcile returned error: %v", err)
	}
	if second.PublicationsCreated != 0 || second.BilibiliJobsCreated != 0 || second.COSObjectsCreated != 0 || second.COSJobsCreated != 0 {
		t.Fatalf("expected idempotent reconcile, got %#v", second)
	}
	assertJobExists(t, ctx, database, "upload-source:1:bilibili:upload")
	assertJobExists(t, ctx, database, "upload-source:1:output:1:cos:1")
	var objectKey string
	if err := database.QueryRowContext(ctx, `
		SELECT object_key FROM upload_source_cos_objects WHERE upload_source_output_id = 1
	`).Scan(&objectKey); err != nil {
		t.Fatalf("query cos object key returned error: %v", err)
	}
	if objectKey != "7grecorder/test/upload-sources/1/1/parts/7G-20260905-\u7b2c01\u573a\u76f4\u64ad-p01.flv" {
		t.Fatalf("unexpected cos object key: %q", objectKey)
	}
}

func TestReconcileSkipsUploadSourceWaitingForReview(t *testing.T) {
	ctx := context.Background()
	cfg, database := openTestDB(t, ctx)
	actor := bootstrapTestAdmin(t, ctx, database)
	if _, err := profile.NewStore(database).Create(ctx, actor, profile.CreateRequest{
		Name:         "7G",
		RoomID:       "1741048619",
		StreamerName: "Streamer",
	}); err != nil {
		t.Fatalf("Create profile returned error: %v", err)
	}
	store := NewStore(database, cfg)
	biliCredential, err := store.CreateCredential(ctx, actor, CredentialCreate{
		Scope:        "USER",
		Platform:     "bilibili",
		Purpose:      "PUBLISHER",
		AccountLabel: "bili account",
		Secret:       []byte(`{"cookie":"cookie"}`),
	})
	if err != nil {
		t.Fatalf("CreateCredential bilibili returned error: %v", err)
	}
	cosCredential, err := store.CreateCredential(ctx, actor, CredentialCreate{
		Scope:        "USER",
		Platform:     "tencent_cos",
		Purpose:      "STORAGE",
		AccountLabel: "cos account",
		Secret:       []byte(`{"secret_id":"id","secret_key":"key"}`),
	})
	if err != nil {
		t.Fatalf("CreateCredential cos returned error: %v", err)
	}
	if _, err := store.UpsertBilibiliConfig(ctx, actor, 1, PublishingConfigUpsert{
		CredentialID: biliCredential.ID,
		Enabled:      true,
		Settings:     []byte(`{"copyright":2}`),
	}); err != nil {
		t.Fatalf("UpsertBilibiliConfig returned error: %v", err)
	}
	if _, err := store.UpsertCOSConfig(ctx, actor, 1, COSConfigUpsert{
		CredentialID:    cosCredential.ID,
		Enabled:         true,
		Region:          "ap-shanghai",
		Bucket:          "bucket-1250000000",
		Prefix:          "7grecorder/test/",
		MaxManagedBytes: 1000000000,
	}); err != nil {
		t.Fatalf("UpsertCOSConfig returned error: %v", err)
	}
	insertReadyUploadSource(t, ctx, database)
	if _, err := database.ExecContext(ctx, `
		UPDATE upload_sources
		SET review_status = 'REQUIRED',
			review_requested_at = CURRENT_TIMESTAMP
		WHERE id = 1
	`); err != nil {
		t.Fatalf("mark upload source waiting for review returned error: %v", err)
	}

	result, err := store.Reconcile(ctx, actor)
	if err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}
	if result.PublicationsCreated != 0 || result.BilibiliJobsCreated != 0 || result.COSObjectsCreated != 0 || result.COSJobsCreated != 0 {
		t.Fatalf("expected review gate to block upload jobs, got %#v", result)
	}
}

func TestBilibiliUploadRequestDefaultsRepostSourceToLiveRoom(t *testing.T) {
	ctx := context.Background()
	cfg, database := openTestDB(t, ctx)
	actor := bootstrapTestAdmin(t, ctx, database)
	if _, err := profile.NewStore(database).Create(ctx, actor, profile.CreateRequest{
		Name:         "7G",
		RoomID:       "1741048619",
		StreamerName: "Streamer",
	}); err != nil {
		t.Fatalf("Create profile returned error: %v", err)
	}
	store := NewStore(database, cfg)
	credential, err := store.CreateCredential(ctx, actor, CredentialCreate{
		Scope:        "USER",
		Platform:     "bilibili",
		Purpose:      "PUBLISHER",
		AccountLabel: "bili account",
		Secret:       []byte(`{"cookie_info":{},"token_info":{}}`),
	})
	if err != nil {
		t.Fatalf("CreateCredential returned error: %v", err)
	}
	if _, err := store.UpsertBilibiliConfig(ctx, actor, 1, PublishingConfigUpsert{
		CredentialID: credential.ID,
		Enabled:      true,
		Settings:     []byte(`{"copyright":2}`),
	}); err != nil {
		t.Fatalf("UpsertBilibiliConfig returned error: %v", err)
	}
	insertReadyUploadSource(t, ctx, database)
	outputPath := filepath.Join(cfg.DataRoot, "upload-sources", "1", "1", "parts", "7G-20260905-\u7b2c01\u573a\u76f4\u64ad-p01.flv")
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		t.Fatalf("create output dir returned error: %v", err)
	}
	if err := os.WriteFile(outputPath, []byte("video"), 0o644); err != nil {
		t.Fatalf("write output file returned error: %v", err)
	}
	if _, err := store.Reconcile(ctx, actor); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	request, err := store.BilibiliUploadRequest(ctx, BilibiliJobPayload{PublicationID: 1, UploadSourceID: 1})
	if err != nil {
		t.Fatalf("BilibiliUploadRequest returned error: %v", err)
	}
	if request.Copyright != 2 {
		t.Fatalf("expected repost copyright, got %d", request.Copyright)
	}
	if request.Source != "https://live.bilibili.com/1741048619" {
		t.Fatalf("unexpected repost source: %q", request.Source)
	}
}

func TestReconcileSkipsUploadSourceWithAdjacentRecordingOutsideSource(t *testing.T) {
	ctx := context.Background()
	cfg, database := openTestDB(t, ctx)
	actor := bootstrapTestAdmin(t, ctx, database)
	if _, err := profile.NewStore(database).Create(ctx, actor, profile.CreateRequest{
		Name:         "7G",
		RoomID:       "1741048619",
		StreamerName: "Streamer",
	}); err != nil {
		t.Fatalf("Create profile returned error: %v", err)
	}
	store := NewStore(database, cfg)
	biliCredential, err := store.CreateCredential(ctx, actor, CredentialCreate{
		Scope:        "USER",
		Platform:     "bilibili",
		Purpose:      "PUBLISHER",
		AccountLabel: "bili account",
		Secret:       []byte(`{"cookie":"cookie"}`),
	})
	if err != nil {
		t.Fatalf("CreateCredential bilibili returned error: %v", err)
	}
	cosCredential, err := store.CreateCredential(ctx, actor, CredentialCreate{
		Scope:        "USER",
		Platform:     "tencent_cos",
		Purpose:      "STORAGE",
		AccountLabel: "cos account",
		Secret:       []byte(`{"secret_id":"id","secret_key":"key"}`),
	})
	if err != nil {
		t.Fatalf("CreateCredential cos returned error: %v", err)
	}
	if _, err := store.UpsertBilibiliConfig(ctx, actor, 1, PublishingConfigUpsert{
		CredentialID: biliCredential.ID,
		Enabled:      true,
		Settings:     []byte(`{"copyright":2}`),
	}); err != nil {
		t.Fatalf("UpsertBilibiliConfig returned error: %v", err)
	}
	if _, err := store.UpsertCOSConfig(ctx, actor, 1, COSConfigUpsert{
		CredentialID:    cosCredential.ID,
		Enabled:         true,
		Region:          "ap-shanghai",
		Bucket:          "bucket-1250000000",
		Prefix:          "7grecorder/test/",
		MaxManagedBytes: 1000000000,
	}); err != nil {
		t.Fatalf("UpsertCOSConfig returned error: %v", err)
	}
	insertReadyUploadSource(t, ctx, database)
	if _, err := database.ExecContext(ctx, `
		INSERT INTO recordings
			(id, recording_profile_id, title, started_at, completed_at, duration_ms, recording_status,
				local_storage_status, source_room_id, streamer_name_snapshot)
		VALUES (2, 1, 'adjacent part', '2026-09-05T10:30:07Z', '2026-09-05T10:45:00Z',
			899000, 'COMPLETED', 'AVAILABLE', '1741048619', 'Streamer');
		INSERT INTO recording_files
			(id, recording_id, relative_path, original_name, kind, file_status, size_bytes, duration_ms, closed_at)
		VALUES (2, 2, 'recordings/1741048619-Streamer/adjacent.flv', 'adjacent.flv',
			'video', 'CLOSED', 30, 899000, '2026-09-05T10:45:00Z');
	`); err != nil {
		t.Fatalf("insert adjacent recording returned error: %v", err)
	}

	result, err := store.Reconcile(ctx, actor)
	if err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}
	if result.PublicationsCreated != 0 || result.BilibiliJobsCreated != 0 || result.COSObjectsCreated != 0 || result.COSJobsCreated != 0 {
		t.Fatalf("expected adjacent source to block upload jobs, got %#v", result)
	}
}

func TestCOSDownloadURLRequestRequiresAvailableOutputObject(t *testing.T) {
	ctx := context.Background()
	cfg, database := openTestDB(t, ctx)
	actor := bootstrapTestAdmin(t, ctx, database)
	if _, err := profile.NewStore(database).Create(ctx, actor, profile.CreateRequest{
		Name:         "7G",
		RoomID:       "1741048619",
		StreamerName: "Streamer",
	}); err != nil {
		t.Fatalf("Create profile returned error: %v", err)
	}
	store := NewStore(database, cfg)
	credential, err := store.CreateCredential(ctx, actor, CredentialCreate{
		Scope:        "USER",
		Platform:     "tencent_cos",
		Purpose:      "STORAGE",
		AccountLabel: "cos account",
		Secret:       []byte(`{"secret_id":"id","secret_key":"key"}`),
	})
	if err != nil {
		t.Fatalf("CreateCredential returned error: %v", err)
	}
	if _, err := store.UpsertCOSConfig(ctx, actor, 1, COSConfigUpsert{
		CredentialID:    credential.ID,
		Enabled:         true,
		Region:          "ap-shanghai",
		Bucket:          "bucket-1250000000",
		Prefix:          "7grecorder/test/",
		MaxManagedBytes: 1000000000,
	}); err != nil {
		t.Fatalf("UpsertCOSConfig returned error: %v", err)
	}
	insertReadyUploadSource(t, ctx, database)
	if _, err := store.Reconcile(ctx, actor); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}
	if _, err := store.COSDownloadURLRequest(ctx, actor, 1, 1); !errors.Is(err, ErrNotReady) {
		t.Fatalf("expected ErrNotReady before upload, got %v", err)
	}
	if err := store.MarkCOSObjectUploaded(ctx, 1, COSUploadResult{ETag: "etag"}); err != nil {
		t.Fatalf("MarkCOSObjectUploaded returned error: %v", err)
	}

	request, err := store.COSDownloadURLRequest(ctx, actor, 1, 1)
	if err != nil {
		t.Fatalf("COSDownloadURLRequest returned error: %v", err)
	}
	if request.ObjectKey != "7grecorder/test/upload-sources/1/1/parts/7G-20260905-\u7b2c01\u573a\u76f4\u64ad-p01.flv" {
		t.Fatalf("unexpected object key: %q", request.ObjectKey)
	}
	if request.Secret.SecretID != "id" || request.Secret.SecretKey != "key" {
		t.Fatalf("unexpected decrypted secret: %#v", request.Secret)
	}
}

func TestReconcileCreatesCOSJobsForClosedDanmakuFiles(t *testing.T) {
	ctx := context.Background()
	cfg, database := openTestDB(t, ctx)
	actor := bootstrapTestAdmin(t, ctx, database)
	if _, err := profile.NewStore(database).Create(ctx, actor, profile.CreateRequest{
		Name:         "7G",
		RoomID:       "1741048619",
		StreamerName: "Streamer",
	}); err != nil {
		t.Fatalf("Create profile returned error: %v", err)
	}
	store := NewStore(database, cfg)
	credential, err := store.CreateCredential(ctx, actor, CredentialCreate{
		Scope:        "USER",
		Platform:     "tencent_cos",
		Purpose:      "STORAGE",
		AccountLabel: "cos account",
		Secret:       []byte(`{"secret_id":"id","secret_key":"key"}`),
	})
	if err != nil {
		t.Fatalf("CreateCredential returned error: %v", err)
	}
	if _, err := store.UpsertCOSConfig(ctx, actor, 1, COSConfigUpsert{
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
		VALUES (1, 1, 'ready upload', '2026-09-05T10:00:00Z', '2026-09-05T10:30:00Z',
			1800000, 'COMPLETED', 'AVAILABLE', '1741048619', 'Streamer')
	`); err != nil {
		t.Fatalf("insert recording returned error: %v", err)
	}
	if _, err := database.ExecContext(ctx, `
		INSERT INTO recording_files
			(id, recording_id, relative_path, original_name, kind, file_status, size_bytes, closed_at)
		VALUES (1, 1, 'recordings/1741048619-Streamer/danmaku.xml', 'danmaku.xml', 'danmaku', 'CLOSED', 12, '2026-09-05T10:30:00Z')
	`); err != nil {
		t.Fatalf("insert danmaku file returned error: %v", err)
	}

	result, err := store.Reconcile(ctx, actor)
	if err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}
	if result.COSFileObjectsCreated != 1 || result.COSFileJobsCreated != 1 {
		t.Fatalf("unexpected reconcile result: %#v", result)
	}
	assertJobExists(t, ctx, database, "recording-file:1:cos:1")
	var objectKey string
	if err := database.QueryRowContext(ctx, `SELECT object_key FROM cos_objects WHERE recording_file_id = 1`).Scan(&objectKey); err != nil {
		t.Fatalf("query cos file object returned error: %v", err)
	}
	if objectKey != "7grecorder/test/raw/recordings/1741048619-Streamer/danmaku.xml" {
		t.Fatalf("unexpected raw cos object key: %q", objectKey)
	}

	if _, err := store.COSRecordingFileDownloadURLRequest(ctx, actor, 1); !errors.Is(err, ErrNotReady) {
		t.Fatalf("expected ErrNotReady before raw file upload, got %v", err)
	}
	if err := store.MarkCOSRecordingFileUploaded(ctx, 1, COSUploadResult{ETag: "etag"}); err != nil {
		t.Fatalf("MarkCOSRecordingFileUploaded returned error: %v", err)
	}
	request, err := store.COSRecordingFileDownloadURLRequest(ctx, actor, 1)
	if err != nil {
		t.Fatalf("COSRecordingFileDownloadURLRequest returned error: %v", err)
	}
	if request.ObjectKey != "7grecorder/test/raw/recordings/1741048619-Streamer/danmaku.xml" {
		t.Fatalf("unexpected raw download object key: %q", request.ObjectKey)
	}
}

func openTestDB(t *testing.T, ctx context.Context) (config.Config, *sql.DB) {
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

func insertReadyUploadSource(t *testing.T, ctx context.Context, database *sql.DB) {
	t.Helper()
	outputRelativePath := "upload-sources/1/1/parts/7G-20260905-\u7b2c01\u573a\u76f4\u64ad-p01.flv"
	if _, err := database.ExecContext(ctx, `
		INSERT INTO upload_sources
			(id, recording_profile_id, source_key, title, source_room_id, streamer_name_snapshot,
				started_at, completed_at, duration_ms, status, output_relative_path,
				total_bytes, recording_count, file_count, max_gap_seconds, merge_gap_threshold_seconds, ready_at)
		VALUES (1, 1, 'profile:1:1:1', 'ready upload', '1741048619', 'Streamer',
			'2026-09-05T10:00:00Z', '2026-09-05T10:30:00Z', 1800000, 'READY_TO_UPLOAD',
			?, 50, 1, 1, 0, 600, CURRENT_TIMESTAMP)
	`, outputRelativePath); err != nil {
		t.Fatalf("insert upload source returned error: %v", err)
	}
	if _, err := database.ExecContext(ctx, `
		INSERT INTO upload_source_outputs
			(id, upload_source_id, sort_order, relative_path, size_bytes, duration_ms, timeline_start_ms, timeline_end_ms, status)
		VALUES
			(1, 1, 0, ?, 50, 1800000, 0, 1800000, 'READY_TO_UPLOAD')
	`, outputRelativePath); err != nil {
		t.Fatalf("insert upload source output returned error: %v", err)
	}
}

func assertJobExists(t *testing.T, ctx context.Context, database *sql.DB, businessKey string) {
	t.Helper()
	var count int
	if err := database.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM jobs WHERE business_key = ?
	`, businessKey).Scan(&count); err != nil {
		t.Fatalf("count job returned error: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected one job %s, got %d", businessKey, count)
	}
}
