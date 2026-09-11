package upload

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/7grecorder/7grecorder/backend/internal/config"
)

func TestBiliupCLIUploaderRunsUploadCommandWithTempCookieFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake shell executable test is Linux-only")
	}

	tempRoot := t.TempDir()
	fakeBiliup := filepath.Join(tempRoot, "fake-biliup")
	argsOut := filepath.Join(tempRoot, "args.txt")
	cookieOut := filepath.Join(tempRoot, "cookie.json")
	script := `#!/bin/sh
set -eu
printf '%s\n' "$@" > "$BILIUP_ARGS_OUT"
cp "$2" "$BILIUP_COOKIE_OUT"
cat "$BILIUP_SUCCESS_FIXTURE"
`
	if err := os.WriteFile(fakeBiliup, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake biliup: %v", err)
	}
	workingDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working dir: %v", err)
	}
	t.Setenv("BILIUP_ARGS_OUT", argsOut)
	t.Setenv("BILIUP_COOKIE_OUT", cookieOut)
	t.Setenv("BILIUP_SUCCESS_FIXTURE", filepath.Join(workingDir, "testdata", "biliup", "upload_success.txt"))

	cookie := json.RawMessage(`{"cookie_info":{"cookies":[{"name":"SESSDATA","value":"redacted"}]},"token_info":{"access_token":"redacted"}}`)
	uploader := NewBiliupCLIUploader(config.Config{DataRoot: tempRoot, TempRoot: filepath.Join(tempRoot, "tmp"), BiliupPath: fakeBiliup})
	result, err := uploader.Upload(context.Background(), BilibiliUploadRequest{
		PublicationID:  42,
		UploadSourceID: 7,
		Title:          "title",
		Description:    "description",
		Tags:           []string{"tag1", "tag2"},
		Copyright:      1,
		Parts: []BilibiliUploadPart{
			{SourcePath: filepath.Join(tempRoot, "part1.flv")},
			{SourcePath: filepath.Join(tempRoot, "part2.flv")},
		},
		Settings: BilibiliPublishingSettings{TID: 65, Submit: "web", UploadLimit: 2, Line: "bda2"},
		Secret:   cookie,
	}, nil)
	if err != nil {
		t.Fatalf("Upload returned error: %v", err)
	}
	if result.ExternalID != "BV1abcDEF234" {
		t.Fatalf("ExternalID = %q", result.ExternalID)
	}
	if result.ExternalURL != "https://www.bilibili.com/video/BV1abcDEF234" {
		t.Fatalf("ExternalURL = %q", result.ExternalURL)
	}

	argsRaw, err := os.ReadFile(argsOut)
	if err != nil {
		t.Fatalf("read args: %v", err)
	}
	args := string(argsRaw)
	for _, want := range []string{
		"upload",
		"--submit",
		"web",
		"--limit",
		"2",
		"--copyright",
		"1",
		"--tid",
		"65",
		"--title",
		"title",
		"--desc",
		"description",
		"--tag",
		"tag1,tag2",
		"--line",
		"bda2",
		"part1.flv",
		"part2.flv",
	} {
		if !strings.Contains(args, want) {
			t.Fatalf("args missing %q in:\n%s", want, args)
		}
	}

	cookieRaw, err := os.ReadFile(cookieOut)
	if err != nil {
		t.Fatalf("read cookie: %v", err)
	}
	if strings.TrimSpace(string(cookieRaw)) != strings.TrimSpace(string(cookie)) {
		t.Fatalf("cookie file mismatch:\n%s", cookieRaw)
	}
}

func TestNormalizeBiliupCookieRejectsBrowserCookieString(t *testing.T) {
	_, err := normalizeBiliupCookie(json.RawMessage(`{"cookie":"SESSDATA=redacted"}`))
	if err == nil {
		t.Fatal("expected error")
	}
	classified, ok := err.(ClassifiedError)
	if !ok {
		t.Fatalf("error type = %T", err)
	}
	if classified.ErrorClass() != "AUTH" {
		t.Fatalf("Class = %q", classified.ErrorClass())
	}
}

func TestNormalizeBiliupCookieAllowsUnknownJSONFileShape(t *testing.T) {
	raw := json.RawMessage(`{"SESSDATA":"redacted","bili_jct":"redacted"}`)
	normalized, err := normalizeBiliupCookie(raw)
	if err != nil {
		t.Fatalf("normalize returned error: %v", err)
	}
	if strings.TrimSpace(string(normalized)) != strings.TrimSpace(string(raw)) {
		t.Fatalf("normalized cookie mismatch:\n%s", normalized)
	}
}

func TestBiliupCLIUploaderReportsProgress(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake shell executable test is Linux-only")
	}

	tempRoot := t.TempDir()
	fakeBiliup := filepath.Join(tempRoot, "fake-biliup")
	script := `#!/bin/sh
set -eu
printf ' 12.00 MiB/24.00 MiB (1.00 MiB/s, 12s)\r'
printf 'Upload completed: part1.flv => cost 1.0s, 1.0 MB/s.\n'
printf 'ResponseData { code: 0, data: Some(Object {"bvid": String("BV1abcDEF234")}), message: "OK" }\n'
`
	if err := os.WriteFile(fakeBiliup, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake biliup: %v", err)
	}

	var progresses []UploadProgress
	reporter := func(_ context.Context, progress UploadProgress) {
		progresses = append(progresses, progress)
	}
	cookie := json.RawMessage(`{"cookie_info":{"cookies":[{"name":"SESSDATA","value":"redacted"}]},"token_info":{"access_token":"redacted"}}`)
	uploader := NewBiliupCLIUploader(config.Config{DataRoot: tempRoot, TempRoot: filepath.Join(tempRoot, "tmp"), BiliupPath: fakeBiliup})
	_, err := uploader.Upload(context.Background(), BilibiliUploadRequest{
		PublicationID:  42,
		UploadSourceID: 7,
		Title:          "title",
		Description:    "description",
		Copyright:      1,
		Parts: []BilibiliUploadPart{
			{SourcePath: filepath.Join(tempRoot, "part1.flv"), SizeBytes: 24 * 1024 * 1024},
		},
		Secret: cookie,
	}, reporter)
	if err != nil {
		t.Fatalf("Upload returned error: %v", err)
	}
	if len(progresses) == 0 {
		t.Fatal("expected progress updates")
	}
	last := progresses[len(progresses)-1]
	if last.CurrentBytes != last.TotalBytes || last.TotalBytes != 24*1024*1024 {
		t.Fatalf("last progress = %#v", last)
	}
}

func TestBiliupCLIUploaderFallsBackWhenPartitionIsRejected(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake shell executable test is Linux-only")
	}

	tempRoot := t.TempDir()
	fakeBiliup := filepath.Join(tempRoot, "fake-biliup")
	argsOut := filepath.Join(tempRoot, "args.txt")
	script := `#!/bin/sh
set -eu
printf '%s\n' "$@" >> "$BILIUP_ARGS_OUT"
for arg in "$@"; do
  if [ "$arg" = "2047" ]; then
    printf '分区不可用 tid=2047\n' >&2
    exit 1
  fi
done
printf 'ResponseData { code: 0, data: Some(Object {"bvid": String("BV1fallback")}), message: "OK" }\n'
`
	if err := os.WriteFile(fakeBiliup, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake biliup: %v", err)
	}
	t.Setenv("BILIUP_ARGS_OUT", argsOut)

	cookie := json.RawMessage(`{"cookie_info":{"cookies":[{"name":"SESSDATA","value":"redacted"}]},"token_info":{"access_token":"redacted"}}`)
	uploader := NewBiliupCLIUploader(config.Config{DataRoot: tempRoot, TempRoot: filepath.Join(tempRoot, "tmp"), BiliupPath: fakeBiliup})
	result, err := uploader.Upload(context.Background(), BilibiliUploadRequest{
		PublicationID:  42,
		UploadSourceID: 7,
		Title:          "title",
		Description:    "description",
		Copyright:      1,
		Parts: []BilibiliUploadPart{
			{SourcePath: filepath.Join(tempRoot, "part1.flv"), SizeBytes: 24 * 1024 * 1024},
		},
		Secret: cookie,
	}, nil)
	if err != nil {
		t.Fatalf("Upload returned error: %v", err)
	}
	if result.ExternalID != "BV1fallback" {
		t.Fatalf("ExternalID = %q", result.ExternalID)
	}
	argsRaw, err := os.ReadFile(argsOut)
	if err != nil {
		t.Fatalf("read args: %v", err)
	}
	args := string(argsRaw)
	if !strings.Contains(args, "2047") || !strings.Contains(args, "27") {
		t.Fatalf("expected primary and fallback tids in args:\n%s", args)
	}
}

func TestParseBiliupExternalID(t *testing.T) {
	output, err := os.ReadFile(filepath.Join("testdata", "biliup", "upload_success.txt"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	id := parseBiliupExternalID(output)
	if id != "BV1abcDEF234" {
		t.Fatalf("id = %q", id)
	}
}
