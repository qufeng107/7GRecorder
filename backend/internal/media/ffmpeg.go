package media

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

var ErrUnsafeRelativePath = errors.New("unsafe relative path")

type Segment struct {
	RelativePath    string
	SizeBytes       int64
	DurationMs      int64
	TimelineStartMs int64
	TimelineEndMs   int64
}

type MergeRequest struct {
	UploadSourceID     int64
	Segments           []Segment
	OutputRelativePath string
}

type MergeResult struct {
	RelativePath string
	SizeBytes    int64
}

type PackageRequest struct {
	UploadSourceID        int64
	InputRelativePath     string
	OutputDirRelativePath string
	DurationMs            int64
	SizeBytes             int64
	MaxPartBytes          int64
	MaxPartDurationSecs   int64
	OutputBaseName        string
}

type SegmentPackageRequest struct {
	UploadSourceID        int64
	Segments              []Segment
	OutputDirRelativePath string
	MaxPartBytes          int64
	MaxPartDurationSecs   int64
	OutputBaseName        string
}

type CompressionRequest struct {
	UploadSourceID     int64
	InputRelativePath  string
	OutputRelativePath string
	Preset             string
}

type CompressionResult struct {
	RelativePath string
	SizeBytes    int64
	Preset       string
}

type PackageOutput struct {
	RelativePath    string
	SizeBytes       int64
	DurationMs      int64
	TimelineStartMs int64
	TimelineEndMs   int64
}

type PackageResult struct {
	Outputs []PackageOutput
}

type Merger interface {
	Merge(ctx context.Context, req MergeRequest) (MergeResult, error)
}

type Packager interface {
	Package(ctx context.Context, req PackageRequest) (PackageResult, error)
	PackageSegments(ctx context.Context, req SegmentPackageRequest) (PackageResult, error)
}

type Compressor interface {
	Compress(ctx context.Context, req CompressionRequest) (CompressionResult, error)
}

type FFmpegMerger struct {
	DataRoot    string
	TempRoot    string
	FFmpegPath  string
	FFprobePath string
}

func NewFFmpegMerger(dataRoot string, tempRoot string, ffmpegPath string) FFmpegMerger {
	if ffmpegPath == "" {
		ffmpegPath = "ffmpeg"
	}
	return FFmpegMerger{DataRoot: dataRoot, TempRoot: tempRoot, FFmpegPath: ffmpegPath, FFprobePath: "ffprobe"}
}

func (m FFmpegMerger) Merge(ctx context.Context, req MergeRequest) (MergeResult, error) {
	if req.UploadSourceID <= 0 || len(req.Segments) == 0 || req.OutputRelativePath == "" {
		return MergeResult{}, errors.New("invalid merge request")
	}
	outputPath, err := resolveWithinRoot(m.DataRoot, req.OutputRelativePath)
	if err != nil {
		return MergeResult{}, fmt.Errorf("resolve merge output: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return MergeResult{}, fmt.Errorf("create merge output dir: %w", err)
	}

	tempRoot := m.TempRoot
	if tempRoot == "" {
		tempRoot = filepath.Join(m.DataRoot, "temp")
	}
	workDir := filepath.Join(tempRoot, "upload-sources", fmt.Sprintf("%d", req.UploadSourceID))
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return MergeResult{}, fmt.Errorf("create merge temp dir: %w", err)
	}
	defer os.RemoveAll(workDir)

	listPath := filepath.Join(workDir, "concat.txt")
	listFile, err := os.Create(listPath)
	if err != nil {
		return MergeResult{}, fmt.Errorf("create concat list: %w", err)
	}
	writer := bufio.NewWriter(listFile)
	for _, segment := range req.Segments {
		absolutePath, err := resolveWithinRoot(m.DataRoot, segment.RelativePath)
		if err != nil {
			_ = listFile.Close()
			return MergeResult{}, fmt.Errorf("resolve merge segment: %w", err)
		}
		info, err := os.Lstat(absolutePath)
		if err != nil {
			_ = listFile.Close()
			return MergeResult{}, fmt.Errorf("stat merge segment: %w", err)
		}
		if info.IsDir() {
			_ = listFile.Close()
			return MergeResult{}, errors.New("merge segment is a directory")
		}
		if _, err := fmt.Fprintf(writer, "file '%s'\n", escapeConcatPath(absolutePath)); err != nil {
			_ = listFile.Close()
			return MergeResult{}, fmt.Errorf("write concat list: %w", err)
		}
	}
	if err := writer.Flush(); err != nil {
		_ = listFile.Close()
		return MergeResult{}, fmt.Errorf("flush concat list: %w", err)
	}
	if err := listFile.Close(); err != nil {
		return MergeResult{}, fmt.Errorf("close concat list: %w", err)
	}

	tempOutput := filepath.Join(workDir, "merged.flv")
	cmd := exec.CommandContext(ctx, m.FFmpegPath, "-hide_banner", "-loglevel", "error", "-f", "concat", "-safe", "0", "-i", listPath, "-c", "copy", "-y", tempOutput)
	output, err := cmd.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = err.Error()
		}
		return MergeResult{}, fmt.Errorf("ffmpeg merge failed: %s", message)
	}
	if err := os.Rename(tempOutput, outputPath); err != nil {
		return MergeResult{}, fmt.Errorf("move merge output: %w", err)
	}
	info, err := os.Stat(outputPath)
	if err != nil {
		return MergeResult{}, fmt.Errorf("stat merge output: %w", err)
	}
	return MergeResult{RelativePath: req.OutputRelativePath, SizeBytes: info.Size()}, nil
}

func (m FFmpegMerger) Package(ctx context.Context, req PackageRequest) (PackageResult, error) {
	if req.UploadSourceID <= 0 || req.InputRelativePath == "" {
		return PackageResult{}, errors.New("invalid package request")
	}
	inputPath, err := resolveWithinRoot(m.DataRoot, req.InputRelativePath)
	if err != nil {
		return PackageResult{}, fmt.Errorf("resolve package input: %w", err)
	}
	info, err := os.Stat(inputPath)
	if err != nil {
		return PackageResult{}, fmt.Errorf("stat package input: %w", err)
	}
	if info.IsDir() {
		return PackageResult{}, errors.New("package input is a directory")
	}
	sizeBytes := req.SizeBytes
	if sizeBytes <= 0 {
		sizeBytes = info.Size()
	}
	maxBytes := req.MaxPartBytes
	if maxBytes <= 0 {
		maxBytes = 3800000000
	}
	maxSeconds := req.MaxPartDurationSecs
	if maxSeconds <= 0 {
		maxSeconds = 7200
	}
	durationMs := req.DurationMs
	if durationMs <= 0 {
		durationMs = maxSeconds * 1000
	}
	outputDir := req.OutputDirRelativePath
	if outputDir == "" {
		outputDir = filepath.ToSlash(filepath.Join("upload-sources", fmt.Sprintf("%d", req.UploadSourceID), "parts"))
	}
	outputBaseName := sanitizePackageBaseName(req.OutputBaseName)
	if outputBaseName == "" {
		outputBaseName = fmt.Sprintf("upload-source-%d", req.UploadSourceID)
	}
	outputDirPath, err := resolveWithinRoot(m.DataRoot, outputDir)
	if err != nil {
		return PackageResult{}, fmt.Errorf("resolve package output dir: %w", err)
	}
	if err := os.MkdirAll(outputDirPath, 0o755); err != nil {
		return PackageResult{}, fmt.Errorf("create package output dir: %w", err)
	}
	if err := clearPackageOutputs(outputDirPath, outputBaseName); err != nil {
		return PackageResult{}, fmt.Errorf("clear package output dir: %w", err)
	}
	if sizeBytes <= maxBytes && durationMs <= maxSeconds*1000 {
		outputRelativePath := filepath.ToSlash(filepath.Join(outputDir, fmt.Sprintf("%s-p01.flv", outputBaseName)))
		outputPath, err := resolveWithinRoot(m.DataRoot, outputRelativePath)
		if err != nil {
			return PackageResult{}, fmt.Errorf("resolve package single output: %w", err)
		}
		if inputPath != outputPath {
			if err := os.Remove(outputPath); err != nil && !errors.Is(err, os.ErrNotExist) {
				return PackageResult{}, fmt.Errorf("clear package single output: %w", err)
			}
			if err := os.Link(inputPath, outputPath); err != nil {
				if err := copyFile(inputPath, outputPath); err != nil {
					return PackageResult{}, fmt.Errorf("copy package single output: %w", err)
				}
			}
		}
		outputInfo, err := os.Stat(outputPath)
		if err != nil {
			return PackageResult{}, fmt.Errorf("stat package single output: %w", err)
		}
		return PackageResult{Outputs: []PackageOutput{{
			RelativePath:    outputRelativePath,
			SizeBytes:       outputInfo.Size(),
			DurationMs:      durationMs,
			TimelineStartMs: 0,
			TimelineEndMs:   durationMs,
		}}}, nil
	}
	segmentSeconds := maxSeconds
	if sizeBytes > 0 && durationMs > 0 {
		estimated := int64(float64(durationMs) * float64(maxBytes) / float64(sizeBytes) / 1000)
		if estimated > 0 && estimated < segmentSeconds {
			segmentSeconds = estimated
		}
	}
	if segmentSeconds < 60 {
		segmentSeconds = 60
	}
	tempPattern := filepath.Join(outputDirPath, "part-%03d.tmp.flv")
	cmd := exec.CommandContext(ctx, m.FFmpegPath, "-hide_banner", "-loglevel", "error", "-i", inputPath, "-c", "copy", "-map", "0", "-f", "segment", "-segment_time", fmt.Sprintf("%d", segmentSeconds), "-reset_timestamps", "1", "-segment_format", "flv", "-y", tempPattern)
	output, err := cmd.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = err.Error()
		}
		return PackageResult{}, fmt.Errorf("ffmpeg package failed: %s", message)
	}
	matches, err := filepath.Glob(filepath.Join(outputDirPath, "part-*.tmp.flv"))
	if err != nil {
		return PackageResult{}, fmt.Errorf("list package outputs: %w", err)
	}
	sort.Strings(matches)
	results := make([]PackageOutput, 0, len(matches))
	var timeline int64
	rootAbs, err := filepath.Abs(m.DataRoot)
	if err != nil {
		return PackageResult{}, fmt.Errorf("resolve data root: %w", err)
	}
	for index, path := range matches {
		target := filepath.Join(outputDirPath, fmt.Sprintf("%s-p%02d.flv", outputBaseName, index+1))
		if err := os.Rename(path, target); err != nil {
			return PackageResult{}, fmt.Errorf("rename package output: %w", err)
		}
		info, err := os.Stat(target)
		if err != nil {
			return PackageResult{}, fmt.Errorf("stat package output: %w", err)
		}
		partDuration := segmentSeconds * 1000
		if index == len(matches)-1 && durationMs > timeline {
			partDuration = durationMs - timeline
		}
		if partDuration <= 0 {
			partDuration = segmentSeconds * 1000
		}
		rel, err := filepath.Rel(rootAbs, target)
		if err != nil {
			return PackageResult{}, fmt.Errorf("rel package output: %w", err)
		}
		results = append(results, PackageOutput{
			RelativePath:    filepath.ToSlash(rel),
			SizeBytes:       info.Size(),
			DurationMs:      partDuration,
			TimelineStartMs: timeline,
			TimelineEndMs:   timeline + partDuration,
		})
		timeline += partDuration
	}
	if len(results) == 0 {
		return PackageResult{}, errors.New("ffmpeg package produced no outputs")
	}
	return PackageResult{Outputs: results}, nil
}

func (m FFmpegMerger) PackageSegments(ctx context.Context, req SegmentPackageRequest) (PackageResult, error) {
	if req.UploadSourceID <= 0 || len(req.Segments) == 0 {
		return PackageResult{}, errors.New("invalid segment package request")
	}
	maxBytes := req.MaxPartBytes
	if maxBytes <= 0 {
		maxBytes = 3800000000
	}
	maxSeconds := req.MaxPartDurationSecs
	if maxSeconds <= 0 {
		maxSeconds = 7200
	}
	outputDir := req.OutputDirRelativePath
	if outputDir == "" {
		outputDir = filepath.ToSlash(filepath.Join("upload-sources", fmt.Sprintf("%d", req.UploadSourceID), "parts"))
	}
	outputBaseName := sanitizePackageBaseName(req.OutputBaseName)
	if outputBaseName == "" {
		outputBaseName = fmt.Sprintf("upload-source-%d", req.UploadSourceID)
	}
	outputDirPath, err := resolveWithinRoot(m.DataRoot, outputDir)
	if err != nil {
		return PackageResult{}, fmt.Errorf("resolve segment package output dir: %w", err)
	}
	if err := os.MkdirAll(outputDirPath, 0o755); err != nil {
		return PackageResult{}, fmt.Errorf("create segment package output dir: %w", err)
	}
	if err := clearPackageOutputs(outputDirPath, outputBaseName); err != nil {
		return PackageResult{}, fmt.Errorf("clear segment package output dir: %w", err)
	}

	segments := make([]Segment, 0, len(req.Segments))
	for _, segment := range req.Segments {
		absolutePath, err := resolveWithinRoot(m.DataRoot, segment.RelativePath)
		if err != nil {
			return PackageResult{}, fmt.Errorf("resolve segment package input: %w", err)
		}
		info, err := os.Lstat(absolutePath)
		if err != nil {
			return PackageResult{}, fmt.Errorf("stat segment package input: %w", err)
		}
		if info.IsDir() {
			return PackageResult{}, errors.New("segment package input is a directory")
		}
		if segment.SizeBytes <= 0 {
			segment.SizeBytes = info.Size()
		}
		if segment.DurationMs <= 0 && segment.TimelineEndMs > segment.TimelineStartMs {
			segment.DurationMs = segment.TimelineEndMs - segment.TimelineStartMs
		}
		segments = append(segments, segment)
	}

	groups := packageSegmentGroups(segments, maxBytes, maxSeconds*1000)
	if len(groups) == 0 {
		return PackageResult{}, errors.New("segment package produced no groups")
	}

	tempRoot := m.TempRoot
	if tempRoot == "" {
		tempRoot = filepath.Join(m.DataRoot, "temp")
	}
	workDir := filepath.Join(tempRoot, "upload-sources", fmt.Sprintf("%d", req.UploadSourceID), "parts-work")
	if err := os.RemoveAll(workDir); err != nil {
		return PackageResult{}, fmt.Errorf("clear segment package temp dir: %w", err)
	}
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return PackageResult{}, fmt.Errorf("create segment package temp dir: %w", err)
	}
	defer os.RemoveAll(workDir)

	results := make([]PackageOutput, 0, len(groups))
	for index, group := range groups {
		outputRelativePath := filepath.ToSlash(filepath.Join(outputDir, fmt.Sprintf("%s-p%02d.flv", outputBaseName, index+1)))
		outputPath, err := resolveWithinRoot(m.DataRoot, outputRelativePath)
		if err != nil {
			return PackageResult{}, fmt.Errorf("resolve segment package output: %w", err)
		}
		if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
			return PackageResult{}, fmt.Errorf("create segment package part dir: %w", err)
		}
		if len(group.Segments) == 1 {
			inputPath, err := resolveWithinRoot(m.DataRoot, group.Segments[0].RelativePath)
			if err != nil {
				return PackageResult{}, fmt.Errorf("resolve segment package single input: %w", err)
			}
			if inputPath != outputPath {
				if err := os.Remove(outputPath); err != nil && !errors.Is(err, os.ErrNotExist) {
					return PackageResult{}, fmt.Errorf("clear segment package single output: %w", err)
				}
				if err := os.Link(inputPath, outputPath); err != nil {
					if err := copyFile(inputPath, outputPath); err != nil {
						return PackageResult{}, fmt.Errorf("copy segment package single output: %w", err)
					}
				}
			}
		} else if err := m.concatSegmentGroup(ctx, workDir, group.Segments, outputPath, index); err != nil {
			return PackageResult{}, err
		}
		info, err := os.Stat(outputPath)
		if err != nil {
			return PackageResult{}, fmt.Errorf("stat segment package output: %w", err)
		}
		results = append(results, PackageOutput{
			RelativePath:    outputRelativePath,
			SizeBytes:       info.Size(),
			DurationMs:      group.DurationMs,
			TimelineStartMs: group.TimelineStartMs,
			TimelineEndMs:   group.TimelineEndMs,
		})
	}
	return PackageResult{Outputs: results}, nil
}

type segmentPackageGroup struct {
	Segments        []Segment
	SizeBytes       int64
	DurationMs      int64
	TimelineStartMs int64
	TimelineEndMs   int64
}

func packageSegmentGroups(segments []Segment, maxBytes int64, maxDurationMs int64) []segmentPackageGroup {
	var groups []segmentPackageGroup
	var current segmentPackageGroup
	flush := func() {
		if len(current.Segments) == 0 {
			return
		}
		if current.TimelineEndMs <= current.TimelineStartMs {
			current.TimelineEndMs = current.TimelineStartMs + current.DurationMs
		}
		groups = append(groups, current)
		current = segmentPackageGroup{}
	}
	for _, segment := range segments {
		segmentDuration := segment.DurationMs
		if segmentDuration <= 0 && segment.TimelineEndMs > segment.TimelineStartMs {
			segmentDuration = segment.TimelineEndMs - segment.TimelineStartMs
		}
		wouldExceedBytes := maxBytes > 0 && current.SizeBytes > 0 && current.SizeBytes+segment.SizeBytes > maxBytes
		wouldExceedDuration := maxDurationMs > 0 && current.DurationMs > 0 && current.DurationMs+segmentDuration > maxDurationMs
		if len(current.Segments) > 0 && (wouldExceedBytes || wouldExceedDuration) {
			flush()
		}
		if len(current.Segments) == 0 {
			current.TimelineStartMs = segment.TimelineStartMs
		}
		current.Segments = append(current.Segments, segment)
		current.SizeBytes += segment.SizeBytes
		current.DurationMs += segmentDuration
		if segment.TimelineEndMs > 0 {
			current.TimelineEndMs = segment.TimelineEndMs
		} else {
			current.TimelineEndMs = current.TimelineStartMs + current.DurationMs
		}
	}
	flush()
	return groups
}

func (m FFmpegMerger) concatSegmentGroup(ctx context.Context, workDir string, segments []Segment, outputPath string, index int) error {
	listPath := filepath.Join(workDir, fmt.Sprintf("concat-%03d.txt", index+1))
	listFile, err := os.Create(listPath)
	if err != nil {
		return fmt.Errorf("create segment concat list: %w", err)
	}
	writer := bufio.NewWriter(listFile)
	for _, segment := range segments {
		absolutePath, err := resolveWithinRoot(m.DataRoot, segment.RelativePath)
		if err != nil {
			_ = listFile.Close()
			return fmt.Errorf("resolve segment concat input: %w", err)
		}
		if _, err := fmt.Fprintf(writer, "file '%s'\n", escapeConcatPath(absolutePath)); err != nil {
			_ = listFile.Close()
			return fmt.Errorf("write segment concat list: %w", err)
		}
	}
	if err := writer.Flush(); err != nil {
		_ = listFile.Close()
		return fmt.Errorf("flush segment concat list: %w", err)
	}
	if err := listFile.Close(); err != nil {
		return fmt.Errorf("close segment concat list: %w", err)
	}

	tempOutput := filepath.Join(workDir, fmt.Sprintf("part-%03d.tmp.flv", index+1))
	cmd := exec.CommandContext(ctx, m.FFmpegPath, "-hide_banner", "-loglevel", "error", "-f", "concat", "-safe", "0", "-i", listPath, "-c", "copy", "-y", tempOutput)
	output, err := cmd.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = err.Error()
		}
		return fmt.Errorf("ffmpeg segment package failed: %s", message)
	}
	if err := os.Rename(tempOutput, outputPath); err != nil {
		return fmt.Errorf("move segment package output: %w", err)
	}
	return nil
}

func (m FFmpegMerger) Compress(ctx context.Context, req CompressionRequest) (CompressionResult, error) {
	if req.UploadSourceID <= 0 || req.InputRelativePath == "" || req.OutputRelativePath == "" {
		return CompressionResult{}, errors.New("invalid compression request")
	}
	preset := strings.TrimSpace(req.Preset)
	if preset == "" {
		preset = "h264_crf23_medium_mp4"
	}
	if preset != "h264_crf23_medium_mp4" {
		return CompressionResult{}, fmt.Errorf("unsupported compression preset: %s", preset)
	}
	inputPath, err := resolveWithinRoot(m.DataRoot, req.InputRelativePath)
	if err != nil {
		return CompressionResult{}, fmt.Errorf("resolve compression input: %w", err)
	}
	info, err := os.Stat(inputPath)
	if err != nil {
		return CompressionResult{}, fmt.Errorf("stat compression input: %w", err)
	}
	if info.IsDir() {
		return CompressionResult{}, errors.New("compression input is a directory")
	}
	outputPath, err := resolveWithinRoot(m.DataRoot, req.OutputRelativePath)
	if err != nil {
		return CompressionResult{}, fmt.Errorf("resolve compression output: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return CompressionResult{}, fmt.Errorf("create compression output dir: %w", err)
	}

	tempRoot := m.TempRoot
	if tempRoot == "" {
		tempRoot = filepath.Join(m.DataRoot, "temp")
	}
	workDir := filepath.Join(tempRoot, "cos-compression", fmt.Sprintf("%d", req.UploadSourceID))
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return CompressionResult{}, fmt.Errorf("create compression temp dir: %w", err)
	}
	defer os.RemoveAll(workDir)

	tempOutput := compressionTempOutputPath(workDir, outputPath)
	cmd := exec.CommandContext(ctx, m.FFmpegPath,
		"-hide_banner", "-loglevel", "error",
		"-i", inputPath,
		"-map", "0:v:0", "-map", "0:a?",
		"-c:v", "libx264",
		"-preset", "medium",
		"-crf", "23",
		"-pix_fmt", "yuv420p",
		"-c:a", "aac",
		"-b:a", "128k",
		"-movflags", "+faststart",
		"-y", tempOutput,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = err.Error()
		}
		return CompressionResult{}, fmt.Errorf("ffmpeg compression failed: %s", message)
	}
	if err := os.Rename(tempOutput, outputPath); err != nil {
		return CompressionResult{}, fmt.Errorf("move compression output: %w", err)
	}
	outputInfo, err := os.Stat(outputPath)
	if err != nil {
		return CompressionResult{}, fmt.Errorf("stat compression output: %w", err)
	}
	if outputInfo.IsDir() || outputInfo.Size() <= 0 {
		return CompressionResult{}, errors.New("compression output is invalid")
	}
	if err := m.probe(ctx, outputPath); err != nil {
		return CompressionResult{}, err
	}
	return CompressionResult{RelativePath: req.OutputRelativePath, SizeBytes: outputInfo.Size(), Preset: preset}, nil
}

func compressionTempOutputPath(workDir string, outputPath string) string {
	base := filepath.Base(outputPath)
	ext := filepath.Ext(base)
	if ext == "" {
		return filepath.Join(workDir, base+".tmp")
	}
	stem := strings.TrimSuffix(base, ext)
	return filepath.Join(workDir, stem+".tmp"+ext)
}

func (m FFmpegMerger) probe(ctx context.Context, path string) error {
	ffprobePath := m.FFprobePath
	if ffprobePath == "" {
		ffprobePath = "ffprobe"
	}
	cmd := exec.CommandContext(ctx, ffprobePath, "-v", "error", "-show_entries", "format=duration", "-of", "default=noprint_wrappers=1:nokey=1", path)
	output, err := cmd.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = err.Error()
		}
		return fmt.Errorf("ffprobe validation failed: %s", message)
	}
	if strings.TrimSpace(string(output)) == "" {
		return errors.New("ffprobe validation returned empty duration")
	}
	return nil
}

func escapeConcatPath(path string) string {
	return strings.ReplaceAll(path, "'", "'\\''")
}

func sanitizePackageBaseName(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	var builder strings.Builder
	previousDash := false
	for _, ch := range value {
		if ch == '/' || ch == '\\' || ch == ':' || ch == '*' || ch == '?' || ch == '"' || ch == '<' || ch == '>' || ch == '|' || ch < 32 {
			if !previousDash {
				builder.WriteByte('-')
				previousDash = true
			}
			continue
		}
		builder.WriteRune(ch)
		previousDash = false
	}
	return strings.Trim(builder.String(), " .-")
}

func copyFile(source string, target string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.Create(target)
	if err != nil {
		return err
	}
	if _, err := io.Copy(output, input); err != nil {
		_ = output.Close()
		return err
	}
	return output.Close()
}

func clearPackageOutputs(outputDir string, outputBaseName string) error {
	for _, pattern := range []string{"part-*.tmp.flv", outputBaseName + "-p*.flv"} {
		matches, err := filepath.Glob(filepath.Join(outputDir, pattern))
		if err != nil {
			return err
		}
		for _, match := range matches {
			if err := os.Remove(match); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
	}
	return nil
}

func resolveWithinRoot(root string, relativePath string) (string, error) {
	if root == "" || relativePath == "" || filepath.IsAbs(relativePath) {
		return "", ErrUnsafeRelativePath
	}
	cleaned := filepath.Clean(relativePath)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(os.PathSeparator)) {
		return "", ErrUnsafeRelativePath
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	candidateAbs, err := filepath.Abs(filepath.Join(rootAbs, cleaned))
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(rootAbs, candidateAbs)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", ErrUnsafeRelativePath
	}
	return candidateAbs, nil
}
