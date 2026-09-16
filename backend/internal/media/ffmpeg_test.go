package media

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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
	if runtime.GOOS == "windows" {
		t.Skip("fake ffmpeg script uses POSIX shell")
	}
	ctx := context.Background()
	root := t.TempDir()
	ffmpegPath := writeFakeSegmentFFmpeg(t, root, 2)
	ffprobePath := writeFakeMediaFFprobe(t, root, false)
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

	merger := NewFFmpegMerger(root, filepath.Join(root, "temp"), ffmpegPath)
	merger.FFprobePath = ffprobePath
	result, err := merger.PackageSegments(ctx, SegmentPackageRequest{
		UploadSourceID:        1,
		Segments:              segments,
		OutputDirRelativePath: "upload-sources/1/1/parts",
		MaxPartBytes:          100,
		MaxPartDurationSecs:   60,
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

func TestPackageSegmentsNormalizesDifferentVideoDimensions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake ffmpeg scripts use POSIX shell")
	}
	ctx := context.Background()
	root := t.TempDir()
	segments := []Segment{
		{RelativePath: "recordings/normal.flv", SizeBytes: 5, DurationMs: 60_000, TimelineStartMs: 0, TimelineEndMs: 60_000},
		{RelativePath: "recordings/pk.flv", SizeBytes: 5, DurationMs: 60_000, TimelineStartMs: 60_000, TimelineEndMs: 120_000},
	}
	for _, segment := range segments {
		path := filepath.Join(root, filepath.FromSlash(segment.RelativePath))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("video"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	merger := NewFFmpegMerger(root, filepath.Join(root, "temp"), writeFakeSegmentFFmpeg(t, root, 1))
	merger.FFprobePath = writeFakeMediaFFprobe(t, root, true)
	result, err := merger.PackageSegments(ctx, SegmentPackageRequest{
		UploadSourceID: 1, Segments: segments, OutputDirRelativePath: "upload-sources/1/parts",
		MaxPartBytes: 100, MaxPartDurationSecs: 120, OutputBaseName: "pk-safe",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Outputs) != 1 {
		t.Fatalf("expected dimensions-only transition to remain one duration part, got %#v", result.Outputs)
	}
	if result.Outputs[0].RelativePath != "upload-sources/1/parts/pk-safe-p01.flv" {
		t.Fatalf("unexpected output ordering: %#v", result.Outputs)
	}
	if result.Outputs[0].TimelineStartMs != 0 || result.Outputs[0].TimelineEndMs != 120_000 {
		t.Fatalf("unexpected output timeline: %#v", result.Outputs)
	}
}

func TestPackageSegmentsFallsBackWhenDimensionNormalizationFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake ffmpeg scripts use POSIX shell")
	}
	ctx := context.Background()
	root := t.TempDir()
	segments := []Segment{
		{RelativePath: "recordings/normal.flv", SizeBytes: 5, DurationMs: 60_000, TimelineStartMs: 0, TimelineEndMs: 60_000},
		{RelativePath: "recordings/pk.flv", SizeBytes: 5, DurationMs: 60_000, TimelineStartMs: 60_000, TimelineEndMs: 120_000},
	}
	for _, segment := range segments {
		path := filepath.Join(root, filepath.FromSlash(segment.RelativePath))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("video"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	merger := NewFFmpegMerger(root, filepath.Join(root, "temp"), writeFakeNormalizationFailFFmpeg(t, root))
	merger.FFprobePath = writeFakeMediaFFprobe(t, root, true)
	result, err := merger.PackageSegments(ctx, SegmentPackageRequest{
		UploadSourceID: 1, Segments: segments, OutputDirRelativePath: "upload-sources/1/parts",
		MaxPartBytes: 100, MaxPartDurationSecs: 120, OutputBaseName: "pk-fallback",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Outputs) != 2 {
		t.Fatalf("expected safe compatibility-group fallback, got %#v", result.Outputs)
	}
	if result.Outputs[0].TimelineEndMs != result.Outputs[1].TimelineStartMs || result.Outputs[1].TimelineEndMs != 120_000 {
		t.Fatalf("unexpected fallback timeline: %#v", result.Outputs)
	}
}

func TestResolutionNormalizationCanvasUsesLongestDurationAndLargerTie(t *testing.T) {
	base := mediaStreamSignature{VideoCodec: "h264", VideoProfile: "High", VideoLevel: 40, VideoWidth: 1920, VideoHeight: 1080, VideoPixelFmt: "yuv420p", VideoFrameRate: "60/1", HasAudio: true, AudioCodec: "aac", AudioSampleRate: "48000", AudioChannels: 2, AudioLayout: "stereo"}
	pk := base
	pk.VideoLevel = 51
	pk.VideoWidth = 1440
	pk.VideoHeight = 1920
	segments := []Segment{{DurationMs: 60_000}, {DurationMs: 120_000}, {DurationMs: 120_000}}
	canvas, ok := resolutionNormalizationCanvas(segments, []mediaStreamSignature{base, pk, base})
	if !ok || canvas != (videoCanvas{Width: 1920, Height: 1080}) {
		t.Fatalf("unexpected duration-selected canvas: %#v, ok=%t", canvas, ok)
	}
	canvas, ok = resolutionNormalizationCanvas([]Segment{{DurationMs: 60_000}, {DurationMs: 60_000}}, []mediaStreamSignature{base, pk})
	if !ok || canvas != (videoCanvas{Width: 1440, Height: 1920}) {
		t.Fatalf("unexpected larger tie canvas: %#v, ok=%t", canvas, ok)
	}
}

func TestResolutionNormalizationRejectsOtherStreamChanges(t *testing.T) {
	base := mediaStreamSignature{VideoCodec: "h264", VideoProfile: "High", VideoLevel: 40, VideoWidth: 1920, VideoHeight: 1080, VideoPixelFmt: "yuv420p", VideoFrameRate: "60/1", HasAudio: true, AudioCodec: "aac", AudioSampleRate: "48000", AudioChannels: 2, AudioLayout: "stereo"}
	changed := base
	changed.VideoWidth = 1080
	changed.VideoHeight = 1920
	changed.VideoFrameRate = "30/1"
	segments := []Segment{{RelativePath: "normal.flv", DurationMs: 60_000}, {RelativePath: "pk.flv", DurationMs: 60_000}}
	if canvas, ok := resolutionNormalizationCanvas(segments, []mediaStreamSignature{base, changed}); ok {
		t.Fatalf("unexpected normalization canvas for frame-rate change: %#v", canvas)
	}
	groups := compatibleSegmentGroups(segments, []mediaStreamSignature{base, changed})
	if len(groups) != 2 {
		t.Fatalf("expected incompatible signatures to remain separate, got %#v", groups)
	}
}

func TestNormalizedSegmentOutputArgsPreserveAspectRatioWithoutUpscale(t *testing.T) {
	args := normalizedSegmentOutputArgs(2, true, videoCanvas{Width: 1920, Height: 1080}, 7200)
	joined := strings.Join(args, " ")
	for _, expected := range []string{
		"scale=w='min(iw,1920)':h='min(ih,1080)'",
		"force_original_aspect_ratio=decrease",
		"pad=1920:1080:(ow-iw)/2:(oh-ih)/2:color=black",
		"concat=n=2:v=1:a=1[vout][aout]",
		"expr:gte(t,n_forced*7200)",
	} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("expected normalized ffmpeg args to contain %q, got %q", expected, joined)
		}
	}
}

func TestPackageSegmentSecondsPrefersTwoHoursAndFallsBackToOneHour(t *testing.T) {
	if seconds := packageSegmentSeconds(3_000_000_000, 2*60*60*1000, 3_800_000_000, 7200); seconds != 7200 {
		t.Fatalf("expected two hour segment, got %d", seconds)
	}
	if seconds := packageSegmentSeconds(5_000_000_000, 2*60*60*1000, 3_800_000_000, 7200); seconds != 3600 {
		t.Fatalf("expected one hour fallback, got %d", seconds)
	}
}

func writeFakeSegmentFFmpeg(t *testing.T, root string, outputCount int) string {
	t.Helper()
	path := filepath.Join(root, "fake-ffmpeg.sh")
	script := "#!/bin/sh\nset -eu\npattern=\"\"\nfor arg in \"$@\"; do pattern=\"$arg\"; done\n"
	for i := 0; i < outputCount; i++ {
		script += fmt.Sprintf("file=$(printf \"$pattern\" %d)\n", i)
		script += "mkdir -p \"$(dirname \"$file\")\"\n"
		script += "printf video > \"$file\"\n"
	}
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake ffmpeg returned error: %v", err)
	}
	return path
}

func writeFakeNormalizationFailFFmpeg(t *testing.T, root string) string {
	t.Helper()
	path := filepath.Join(root, "fake-normalization-fail-ffmpeg.sh")
	script := `#!/bin/sh
set -eu
pattern=""
for arg in "$@"; do
  if [ "$arg" = "-filter_complex" ]; then exit 1; fi
  pattern="$arg"
done
file=$(printf "$pattern" 0)
mkdir -p "$(dirname "$file")"
printf video > "$file"
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake normalization failure ffmpeg returned error: %v", err)
	}
	return path
}

func writeFakeMediaFFprobe(t *testing.T, root string, varyDimensions bool) string {
	t.Helper()
	path := filepath.Join(root, fmt.Sprintf("fake-ffprobe-%t.sh", varyDimensions))
	heightCase := "height=1080"
	if varyDimensions {
		heightCase = "case \"$input\" in *pk.flv) height=2160 ;; *) height=1080 ;; esac"
	}
	script := "#!/bin/sh\nset -eu\ninput=\"\"\nfor arg in \"$@\"; do input=\"$arg\"; done\n" + heightCase + "\n" +
		"printf '{\"streams\":[{\"codec_type\":\"video\",\"codec_name\":\"h264\",\"profile\":\"High\",\"level\":40,\"width\":1920,\"height\":%s,\"pix_fmt\":\"yuv420p\",\"r_frame_rate\":\"60/1\"},{\"codec_type\":\"audio\",\"codec_name\":\"aac\",\"sample_rate\":\"48000\",\"channels\":2,\"channel_layout\":\"stereo\"}]}' \"$height\"\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake ffprobe returned error: %v", err)
	}
	return path
}
