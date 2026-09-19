package liveanalytics

import (
	"encoding/json"
	"github.com/7grecorder/7grecorder/backend/internal/config"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRawWriterPreservesKnownAndUnknownCommands(t *testing.T) {
	root := t.TempDir()
	writer, err := NewRawWriter(root, 7, 9, time.Date(2026, 9, 19, 1, 2, 3, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	known := json.RawMessage(`{"cmd":"LIVE_OPEN_PLATFORM_DM","data":{"msg":"hello"}}`)
	unknown := json.RawMessage(`{"cmd":"NEW_PLATFORM_EVENT","data":{"future":true}}`)
	if err := writer.Write(time.Unix(10, 0), "LIVE_OPEN_PLATFORM_DM", known); err != nil {
		t.Fatal(err)
	}
	if err := writer.Write(time.Unix(11, 0), "NEW_PLATFORM_EVENT", unknown); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(writer.RelativePath(), "live-analytics/profile-7/2026-09-19/") {
		t.Fatalf("unexpected path %q", writer.RelativePath())
	}
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(writer.RelativePath())))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, `"msg":"hello"`) || !strings.Contains(text, `"future":true`) {
		t.Fatalf("raw events were not preserved: %s", text)
	}
}

func TestRawPathsRejectSymlinkedParent(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "live-analytics")); err != nil {
		t.Fatal(err)
	}
	if _, err := NewRawWriter(root, 1, 1, time.Now()); err == nil {
		t.Fatal("writer accepted symlink")
	}
	store := Store{cfg: config.Config{DataRoot: root}}
	if _, err := store.resolveRawPath("live-analytics/session.jsonl"); err == nil {
		t.Fatal("cleanup accepted symlink")
	}
	entries, err := os.ReadDir(outside)
	if err != nil || len(entries) != 0 {
		t.Fatal("outside directory modified")
	}
}
