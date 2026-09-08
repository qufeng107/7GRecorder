package media

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestPackageCreatesNamedSinglePart(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	inputRelativePath := "recordings/source.flv"
	inputPath := filepath.Join(root, inputRelativePath)
	if err := os.MkdirAll(filepath.Dir(inputPath), 0o755); err != nil {
		t.Fatalf("create input dir returned error: %v", err)
	}
	if err := os.WriteFile(inputPath, []byte("video"), 0o644); err != nil {
		t.Fatalf("write input returned error: %v", err)
	}

	result, err := NewFFmpegMerger(root, filepath.Join(root, "temp"), "ffmpeg").Package(ctx, PackageRequest{
		UploadSourceID:        1,
		InputRelativePath:     inputRelativePath,
		OutputDirRelativePath: "upload-sources/1/1/parts",
		DurationMs:            60_000,
		SizeBytes:             5,
		MaxPartBytes:          4 * 1024 * 1024 * 1024,
		MaxPartDurationSecs:   7200,
		OutputBaseName:        "7G-20260905-live-01",
	})
	if err != nil {
		t.Fatalf("Package returned error: %v", err)
	}
	if len(result.Outputs) != 1 {
		t.Fatalf("expected one output, got %d", len(result.Outputs))
	}
	expected := "upload-sources/1/1/parts/7G-20260905-live-01-p01.flv"
	if result.Outputs[0].RelativePath != expected {
		t.Fatalf("unexpected output path: %q", result.Outputs[0].RelativePath)
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(expected))); err != nil {
		t.Fatalf("stat named output returned error: %v", err)
	}
}

func TestCompressionTempOutputKeepsFinalExtension(t *testing.T) {
	path := compressionTempOutputPath("/tmp/work", "/data/7grecorder/upload-sources/9/7G-20260908-p01.mp4")
	if filepath.Base(path) != "7G-20260908-p01.tmp.mp4" {
		t.Fatalf("unexpected temp output path: %q", path)
	}
}
