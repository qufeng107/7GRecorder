package upload

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/7grecorder/7grecorder/backend/internal/account"
	"github.com/7grecorder/7grecorder/backend/internal/profile"
	"github.com/7grecorder/7grecorder/backend/internal/recording"
)

type quotaDeleter struct {
	keys   []string
	fail   bool
	before func()
}

func (d *quotaDeleter) Delete(_ context.Context, r COSUploadRequest) error {
	d.keys = append(d.keys, r.ObjectKey)
	if d.before != nil {
		d.before()
	}
	if d.fail {
		return errors.New("delete unavailable")
	}
	return nil
}
func quotaFixture(t *testing.T, limit int64) (Store, COSUploadRequest) {
	t.Helper()
	ctx := context.Background()
	cfg, database := openTestDB(t, ctx)
	actor := bootstrapTestAdmin(t, ctx, database)
	if _, err := profile.NewStore(database).Create(ctx, actor, profile.CreateRequest{Name: "quota", RoomID: "1", StreamerName: "test"}); err != nil {
		t.Fatal(err)
	}
	s := NewStore(database, cfg)
	cred, err := s.CreateCredential(ctx, actor, CredentialCreate{AccountLabel: "quota-test", Scope: "USER", Platform: "tencent_cos", Purpose: "STORAGE", Secret: []byte(`{"secret_id":"fake","secret_key":"fake"}`)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.UpsertCOSConfig(ctx, actor, 1, COSConfigUpsert{Enabled: true, CredentialID: cred.ID, Region: "ap-shanghai", Bucket: "test-123", Prefix: "archive/", MaxManagedBytes: limit}); err != nil {
		t.Fatal(err)
	}
	insertReadyUploadSource(t, ctx, database)
	if _, err = s.Reconcile(ctx, actor); err != nil {
		t.Fatal(err)
	}
	var relative string
	if err = database.QueryRow(`SELECT relative_path FROM upload_source_outputs WHERE id=1`).Scan(&relative); err != nil {
		t.Fatal(err)
	}
	filename := filepath.Join(cfg.DataRoot, relative)
	if err = os.MkdirAll(filepath.Dir(filename), 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filename, make([]byte, 30), 0600); err != nil {
		t.Fatal(err)
	}
	req, err := s.COSUploadRequest(ctx, COSJobPayload{COSObjectID: 1, UploadSourceID: 1, OutputID: 1})
	if err != nil {
		t.Fatal(err)
	}
	return s, req
}
func quotaExec(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.Exec(query, args...); err != nil {
		t.Fatal(err)
	}
}
func quotaRaw(t *testing.T, s Store, id, size int64, key, status string) {
	t.Helper()
	quotaExec(t, s.db, `INSERT INTO recordings(id,recording_profile_id,started_at,completed_at,recording_status,source_room_id,streamer_name_snapshot)
 VALUES(?,1,?, '2026-01-02','COMPLETED','1','test')`, id, fmt.Sprintf("2026-01-%02d", id))
	quotaExec(t, s.db, `INSERT INTO recording_files(id,recording_id,relative_path,original_name,kind,file_status,size_bytes) VALUES(?,?,?, 'test.xml','danmaku','CLOSED',?)`, id, id, fmt.Sprintf("%d.xml", id), size)
	quotaExec(t, s.db, `INSERT INTO cos_objects(id,cos_storage_profile_id,recording_profile_id,recording_id,recording_file_id,object_key,size_bytes,status) VALUES(?,1,1,?,?,?,?,?)`, id, id, id, key, size, status)
}
func quotaStatus(t *testing.T, s Store, table string, id int64) string {
	t.Helper()
	var status string
	if err := s.db.QueryRow(`SELECT status FROM `+table+` WHERE id=?`, id).Scan(&status); err != nil {
		t.Fatal(err)
	}
	return status
}

func TestCOSQuotaAdmission(t *testing.T) {
	for _, tc := range []struct {
		name    string
		limit   int64
		wantErr bool
		deletes int
	}{
		{"exact fit actual bytes", 100, false, 0}, {"reclaims oldest", 99, false, 1}, {"oversized does not delete", 29, true, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, req := quotaFixture(t, tc.limit)
			quotaRaw(t, s, 1, 70, "archive/raw/old.xml", "AVAILABLE")
			d := &quotaDeleter{}
			err := s.AdmitCOSUpload(context.Background(), req, false, d)
			if (err != nil) != tc.wantErr || len(d.keys) != tc.deletes {
				t.Fatalf("err=%v deleted=%v", err, d.keys)
			}
			if !tc.wantErr {
				var size int64
				if err = s.db.QueryRow(`SELECT quota_reserved_bytes FROM upload_source_cos_objects WHERE id=1 AND status='UPLOADING'`).Scan(&size); err != nil || size != 30 {
					t.Fatalf("reservation size=%d err=%v", size, err)
				}
			}
		})
	}
}
func TestCOSQuotaDeletionRecoveryAndDailyCheck(t *testing.T) {
	s, req := quotaFixture(t, 60)
	quotaRaw(t, s, 1, 70, "archive/raw/old.xml", "AVAILABLE")
	d := &quotaDeleter{fail: true, before: func() {
		if quotaStatus(t, s, "cos_objects", 1) != "DELETING" {
			t.Fatal("delete intent not committed before network")
		}
	}}
	if err := s.AdmitCOSUpload(context.Background(), req, false, d); err == nil {
		t.Fatal("delete failure allowed upload")
	}
	if quotaStatus(t, s, "upload_source_cos_objects", 1) != "PENDING" {
		t.Fatal("failed cleanup reserved upload")
	}
	d.fail = false
	if err := s.ReconcileCOSQuota(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	if quotaStatus(t, s, "cos_objects", 1) != "DELETED" || len(d.keys) != 2 {
		t.Fatal("daily check did not resume deletion")
	}
	if err := s.AdmitCOSUpload(context.Background(), req, false, d); err != nil {
		t.Fatal(err)
	}
	if len(d.keys) != 2 {
		t.Fatal("deleted tombstone was reused")
	}
}
func TestCOSQuotaSafety(t *testing.T) {
	for _, tc := range []struct{ name, mutation string }{
		{"protected", `UPDATE recordings SET local_protected=1`},
		{"active", `UPDATE recordings SET recording_status='ACTIVE'`},
		{"unknown prefix", `UPDATE cos_objects SET object_key='archive-other/raw.xml'`},
		{"in flight", `UPDATE cos_objects SET status='UPLOADING',quota_reserved_bytes=size_bytes`},
		{"review", `UPDATE recordings SET upload_review_status='REQUIRED'`},
		{"writing", `UPDATE recording_files SET file_status='WRITING'`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, req := quotaFixture(t, 90)
			quotaRaw(t, s, 1, 70, "archive/raw/old.xml", "AVAILABLE")
			quotaExec(t, s.db, tc.mutation)
			d := &quotaDeleter{}
			if err := s.AdmitCOSUpload(context.Background(), req, false, d); err == nil {
				t.Fatal("unsafe quota admission")
			}
			if len(d.keys) != 0 {
				t.Fatalf("unsafe deletes: %v", d.keys)
			}
		})
	}
}
func TestCOSQuotaVideoCleanupAndTombstone(t *testing.T) {
	s, req := quotaFixture(t, 20)
	quotaRaw(t, s, 1, 0, "archive/raw/old.xml", "DELETED")
	quotaExec(t, s.db, `INSERT INTO upload_source_segments(upload_source_id,recording_id,recording_file_id,sort_order,source_started_at,source_completed_at,timeline_start_ms,timeline_end_ms,relative_path) VALUES(1,1,1,0,'2026-01-01','2026-01-02',0,1,'1.xml')`)
	quotaExec(t, s.db, `UPDATE upload_source_cos_objects SET status='AVAILABLE',size_bytes=30`)
	d := &quotaDeleter{}
	if err := s.ReconcileCOSQuota(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	if len(d.keys) != 1 || d.keys[0] != req.ObjectKey || quotaStatus(t, s, "upload_source_cos_objects", 1) != "DELETED" {
		t.Fatal("video not reclaimed")
	}
	actor := account.User{ID: 1, Role: account.RoleSuperAdmin}
	if _, err := s.Reconcile(context.Background(), actor); err != nil {
		t.Fatal(err)
	}
	if quotaStatus(t, s, "upload_source_cos_objects", 1) != "DELETED" {
		t.Fatal("deleted video requeued")
	}
	sources, err := recording.NewStore(s.db, s.cfg).ListUploadSources(context.Background(), actor, 600)
	if err != nil || len(sources.Items) != 1 || sources.Items[0].COSStatus != "DELETED" || sources.Items[0].Outputs[0].COSStatus != "DELETED" {
		t.Fatalf("deleted status lost in API: %#v error=%v", sources, err)
	}
	if _, err := os.Stat(req.SourcePath); err != nil {
		t.Fatal("local video removed")
	}
}

func TestCOSQuotaFailedUploadRetainsReservation(t *testing.T) {
	s, req := quotaFixture(t, 100)
	quotaRaw(t, s, 1, 70, "archive/raw/old.xml", "AVAILABLE")
	if err := s.AdmitCOSUpload(context.Background(), req, false, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkCOSObjectUploadFailed(context.Background(), 1, "TRANSIENT", "HEAD failed after PUT"); err != nil {
		t.Fatal(err)
	}
	// The retry owns its existing 30 bytes, so exact-fit retry needs no deletion.
	if err := s.AdmitCOSUpload(context.Background(), req, false, nil); err != nil {
		t.Fatal(err)
	}
	var reserved int64
	if err := s.db.QueryRow(`SELECT quota_reserved_bytes FROM upload_source_cos_objects WHERE id=1`).Scan(&reserved); err != nil || reserved != 30 {
		t.Fatalf("reservation=%d err=%v", reserved, err)
	}
	quotaExec(t, s.db, `UPDATE upload_source_cos_objects SET status='DELETED',quota_reserved_bytes=0`)
	if err := s.MarkCOSObjectUploadFailed(context.Background(), 1, "PERMANENT", "stale job"); err != nil {
		t.Fatal(err)
	}
	if quotaStatus(t, s, "upload_source_cos_objects", 1) != "DELETED" {
		t.Fatal("stale failure resurrected tombstone")
	}
}

func TestCOSQuotaConcurrentAdmission(t *testing.T) {
	s, req := quotaFixture(t, 50)
	quotaRaw(t, s, 1, 30, "archive/raw/new.xml", "PENDING")
	raw := req
	raw.ObjectID = 1
	raw.UploadSourceID = 0
	raw.ObjectKey = "archive/raw/new.xml"
	results := make(chan error, 2)
	go func() { results <- s.AdmitCOSUpload(context.Background(), req, false, nil) }()
	go func() { results <- s.AdmitCOSUpload(context.Background(), raw, true, nil) }()
	successes := 0
	for i := 0; i < 2; i++ {
		if <-results == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("expected one reservation, got %d", successes)
	}
}

func TestCOSQuotaDestinationCannotMoveRegisteredObjects(t *testing.T) {
	s, _ := quotaFixture(t, 100)
	actor := account.User{ID: 1, Role: account.RoleSuperAdmin}
	for _, tc := range []struct {
		bucket, prefix string
		bad            bool
	}{{"new-123", "archive/", true}, {"test-123", "other/", true}, {"test-123", "archive/", false}} {
		_, err := s.UpsertCOSConfig(context.Background(), actor, 1, COSConfigUpsert{Enabled: true, CredentialID: 1, Region: "ap-shanghai", Bucket: tc.bucket, Prefix: tc.prefix, MaxManagedBytes: 60})
		if errors.Is(err, ErrValidation) != tc.bad {
			t.Fatalf("bucket=%s prefix=%s error=%v", tc.bucket, tc.prefix, err)
		}
	}
}

func TestCOSQuotaDailyDisabledAndOldest(t *testing.T) {
	s, _ := quotaFixture(t, 60)
	quotaRaw(t, s, 1, 40, "archive/raw/old.xml", "AVAILABLE")
	quotaRaw(t, s, 2, 40, "archive/raw/new.xml", "AVAILABLE")
	d := &quotaDeleter{}
	quotaExec(t, s.db, `UPDATE cos_storage_profiles SET enabled=0`)
	if err := s.ReconcileCOSQuota(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	if len(d.keys) != 0 {
		t.Fatal("disabled profile cleaned")
	}
	quotaExec(t, s.db, `UPDATE cos_storage_profiles SET enabled=1`)
	if err := s.ReconcileCOSQuota(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	if len(d.keys) != 1 || d.keys[0] != "archive/raw/old.xml" {
		t.Fatalf("deletes=%v", d.keys)
	}
}

func quotaSongRun(t *testing.T, s Store) {
	t.Helper()
	quotaExec(t, s.db, `INSERT INTO song_analysis_runs(id,source_cos_object_id,upload_source_id,upload_source_output_id,
 recording_profile_id,source_cos_storage_profile_id,source_object_key,source_size_bytes,source_timeline_start_ms,source_timeline_end_ms,
 provider_region,provider_container_id,provider_credential_id,destination_cos_storage_profile_id,songs_prefix,boundary_padding_ms,algorithm_version,status)
 VALUES(1,1,1,1,1,1,'archive/video.flv',30,0,1000,'test','test',1,1,'songs',0,'test','DOWNLOADING')`)
}
func TestCOSQuotaVideoProtections(t *testing.T) {
	for _, mutation := range []string{
		`UPDATE recordings SET local_protected=1`,
		`UPDATE recordings SET recording_status='ACTIVE'`,
		`UPDATE upload_sources SET review_status='REQUIRED'`,
		`UPDATE song_analysis_runs SET status='DOWNLOADING'`,
	} {
		t.Run(mutation, func(t *testing.T) {
			s, _ := quotaFixture(t, 20)
			quotaRaw(t, s, 1, 0, "archive/raw/old.xml", "DELETED")
			quotaExec(t, s.db, `INSERT INTO upload_source_segments(upload_source_id,recording_id,recording_file_id,sort_order,source_started_at,source_completed_at,timeline_start_ms,timeline_end_ms,relative_path) VALUES(1,1,1,0,'2026-01-01','2026-01-02',0,1,'1.xml')`)
			quotaExec(t, s.db, `UPDATE upload_source_cos_objects SET status='AVAILABLE',size_bytes=30`)
			quotaSongRun(t, s)
			quotaExec(t, s.db, `UPDATE song_analysis_runs SET status='COMPLETED'`)
			quotaExec(t, s.db, mutation)
			d := &quotaDeleter{}
			if err := s.ReconcileCOSQuota(context.Background(), d); err == nil {
				t.Fatal("protected video considered reclaimable")
			}
			if len(d.keys) != 0 {
				t.Fatalf("deleted protected video: %v", d.keys)
			}
		})
	}
}
func TestCOSQuotaSongsShareBudgetAndRetryReservation(t *testing.T) {
	s, req := quotaFixture(t, 60)
	quotaExec(t, s.db, `UPDATE upload_source_cos_objects SET status='AVAILABLE',size_bytes=30`)
	quotaSongRun(t, s)
	quotaExec(t, s.db, `INSERT INTO songs(id,recording_profile_id,upload_source_id,analysis_run_id,detected_start_ms,detected_end_ms,start_ms,end_ms) VALUES(1,1,1,1,0,1000,0,1000)`)
	quotaExec(t, s.db, `INSERT INTO song_artifacts(id,song_id,clip_revision,cos_storage_profile_id,object_key,status) VALUES(1,1,1,1,'songs/audio.m4a','GENERATING')`)
	req.ObjectKey = "songs/audio.m4a"
	for i := 0; i < 2; i++ {
		if err := s.AdmitCOSArtifact(context.Background(), req, 1, nil); err != nil {
			t.Fatal(err)
		}
	}
	quotaExec(t, s.db, `UPDATE cos_storage_profiles SET max_managed_bytes=59`)
	if err := s.AdmitCOSArtifact(context.Background(), req, 1, nil); err == nil {
		t.Fatal("songs bypassed profile budget")
	}
}
