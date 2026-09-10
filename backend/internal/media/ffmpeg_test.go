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

func TestPackageSegmentsCreatesPartsWithoutWholeMerge(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	segments := []Segment{
		{RelativePath: "recordings/part1.flv", SizeBytes: 5, DurationMs: 60_000, TimelineStartMs: 0, TimelineEndMs: 60_000},
		{RelativePath: "recordings/part2.flv", SizeBytes: 5, DurationMs: 60_000, TimelineStartMs: 60_000, TimelineEndMs: 120_000},
	}
	for _, segment := range segments {
		path := filepath.Join(root, filepath.FromSlash(segment.RelativePath))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("create segment dir returned error: %v", err)
		}
		if err := os.WriteFile(path, []byte("video"), 0o644); err != nil {
			t.Fatalf("write segment returned error: %v", err)
		}
	}

	result, err := NewFFmpegMerger(root, filepath.Join(root, "temp"), "ffmpeg").PackageSegments(ctx, SegmentPackageRequest{
		UploadSourceID:        1,
		Segments:              segments,
		OutputDirRelativePath: "upload-sources/1/1/parts",
		MaxPartBytes:          6,
		MaxPartDurationSecs:   7200,
		OutputBaseName:        "7G-20260905-live-01",
	})
	if err != nil {
		t.Fatalf("PackageSegments returned error: %v", err)
	}
	if len(result.Outputs) != 2 {
		t.Fatalf("expected two outputs, got %d", len(result.Outputs))
	}
	if result.Outputs[0].TimelineStartMs != 0 || result.Outputs[1].TimelineStartMs != 60_000 {
		t.Fatalf("unexpected timelines: %#v", result.Outputs)
	}
	for _, output := range result.Outputs {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(output.RelativePath))); err != nil {
			t.Fatalf("stat segment package output returned error: %v", err)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "upload-sources", "1", "upload-source-1.flv")); !os.IsNotExist(err) {
		t.Fatalf("expected no whole-recording merge output, got %v", err)
	}
}

func TestPackageSegmentGroupsBatchesAdjacentSegments(t *testing.T) {
	groups := packageSegmentGroups([]Segment{
		{RelativePath: "a.flv", SizeBytes: 4, DurationMs: 1000, TimelineStartMs: 0, TimelineEndMs: 1000},
		{RelativePath: "b.flv", SizeBytes: 4, DurationMs: 1000, TimelineStartMs: 1000, TimelineEndMs: 2000},
		{RelativePath: "c.flv", SizeBytes: 4, DurationMs: 1000, TimelineStartMs: 2000, TimelineEndMs: 3000},
	}, 8, 10_000)
	if len(groups) != 2 {
		t.Fatalf("expected two groups, got %d", len(groups))
	}
	if len(groups[0].Segments) != 2 || groups[0].SizeBytes != 8 || groups[0].TimelineEndMs != 2000 {
		t.Fatalf("unexpected first group: %#v", groups[0])
	}
	if len(groups[1].Segments) != 1 || groups[1].TimelineStartMs != 2000 {
		t.Fatalf("unexpected second group: %#v", groups[1])
	}
}

func TestCompressionTempOutputKeepsFinalExtension(t *testing.T) {
	path := compressionTempOutputPath("/tmp/work", "/data/7grecorder/upload-sources/9/7G-20260908-p01.mp4")
	if filepath.Base(path) != "7G-20260908-p01.tmp.mp4" {
		t.Fatalf("unexpected temp output path: %q", path)
	}
}
