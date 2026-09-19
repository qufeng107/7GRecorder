package db

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/7grecorder/7grecorder/backend/internal/config"
	"github.com/7grecorder/7grecorder/backend/migrations"
	_ "github.com/mattn/go-sqlite3"
	"github.com/pressly/goose/v3"
)

func TestMigrateCleanDatabase(t *testing.T) {
	root := t.TempDir()
	cfg := config.Config{
		DataRoot:   root,
		SQLitePath: filepath.Join(root, "7grecorder.db"),
		TempRoot:   filepath.Join(root, "temp"),
	}

	if err := Migrate(t.Context(), cfg); err != nil {
		t.Fatal(err)
	}
	database, err := Open(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	for _, table := range []string{"song_settings", "song_analysis_runs", "song_analysis_chunks", "song_recognition_matches", "song_artifacts", "media_cache_entries", "storage_reservations", "live_analytics_configs", "live_capture_sessions", "live_capture_minutes", "live_capture_raw_files"} {
		var count int
		if err := database.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("expected migrated table %s", table)
		}
	}
}

func TestSongsMigrationRefusesToDiscardLegacyRows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	database, err := sql.Open("sqlite3", path+"?_foreign_keys=on")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if _, err := database.ExecContext(t.Context(), "PRAGMA journal_mode=WAL;"); err != nil {
		t.Fatal(err)
	}
	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("sqlite3"); err != nil {
		t.Fatal(err)
	}
	if err := goose.UpToContext(t.Context(), database, ".", 10); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(t.Context(), `
		INSERT INTO users (id, username, password_hash, role) VALUES (1, 'admin', 'hash', 'SUPER_ADMIN');
		INSERT INTO recording_profiles (id, owner_user_id, name, room_id, streamer_name) VALUES (1, 1, '7G', '1', 'Singer');
		INSERT INTO recordings (id, recording_profile_id, started_at, recording_status, source_room_id, streamer_name_snapshot)
			VALUES (1, 1, CURRENT_TIMESTAMP, 'COMPLETED', '1', 'Singer');
		INSERT INTO songs (id, recording_profile_id, recording_id, title, start_ms, end_ms, status)
			VALUES (1, 1, 1, 'legacy song', 0, 1000, 'DRAFT');
	`); err != nil {
		t.Fatal(err)
	}
	if err := goose.UpToContext(t.Context(), database, ".", 11); err == nil {
		t.Fatal("expected migration 11 to reject legacy Songs data")
	}
	var title string
	if err := database.QueryRowContext(t.Context(), `SELECT title FROM songs WHERE id = 1`).Scan(&title); err != nil {
		t.Fatal(err)
	}
	if title != "legacy song" {
		t.Fatalf("legacy row changed: %q", title)
	}
}

func TestRawFileMigrationImportsExistingEvidence(t *testing.T) {
	database, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "existing.db")+"?_foreign_keys=on")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if _, err := database.ExecContext(t.Context(), "PRAGMA journal_mode=WAL;"); err != nil {
		t.Fatal(err)
	}
	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("sqlite3"); err != nil {
		t.Fatal(err)
	}
	if err := goose.UpToContext(t.Context(), database, ".", 13); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(t.Context(), `
        INSERT INTO users(id,username,password_hash,role) VALUES(1,'admin','hash','SUPER_ADMIN');
        INSERT INTO recording_profiles(id,owner_user_id,name,room_id,streamer_name) VALUES(1,1,'test','1','test');
        INSERT INTO live_capture_sessions(id,recording_profile_id,status,raw_relative_path,raw_status,raw_size_bytes)
        VALUES(1,1,'ENDED','live-analytics/old.jsonl','AVAILABLE',123),(2,1,'CONNECTED','live-analytics/current.jsonl','WRITING',45);
    `); err != nil {
		t.Fatal(err)
	}
	if err := goose.UpToContext(t.Context(), database, ".", 14); err != nil {
		t.Fatal(err)
	}
	var count, total int64
	if err := database.QueryRowContext(t.Context(), `SELECT COUNT(*),SUM(size_bytes) FROM live_capture_raw_files`).Scan(&count, &total); err != nil {
		t.Fatal(err)
	}
	if count != 2 || total != 168 {
		t.Fatalf("evidence metadata lost: count=%d total=%d", count, total)
	}
	var status string
	if err := database.QueryRowContext(t.Context(), `SELECT status FROM live_capture_raw_files WHERE session_id = 2`).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "WRITING" {
		t.Fatalf("active part lost protection: %s", status)
	}
}
