package media

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

var ErrUnsafeRelativePath = errors.New("unsafe relative path")

type Segment struct {
	RelativePath string
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
}

type PackageOutput struct {
	RelativePath     string
	SizeBytes        int64
	DurationMs       int64
	TimelineStartMs  int64
	TimelineEndMs    int64
}

type PackageResult struct {
	Outputs []PackageOutput
}

type Merger interface {
	Merge(ctx context.Context, req MergeRequest) (MergeResult, error)
}

type Packager interface {
	Package(ctx context.Context, req PackageRequest) (PackageResult, error)
}

type FFmpegMerger struct {
	DataRoot   string
	TempRoot   string
	FFmpegPath string
}

func NewFFmpegMerger(dataRoot string, tempRoot string, ffmpegPath string) FFmpegMerger {
	if ffmpegPath == "" {
		ffmpegPath = "ffmpeg"
	}
	return FFmpegMerger{DataRoot: dataRoot, TempRoot: tempRoot, FFmpegPath: ffmpegPath}
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
		maxBytes = 4 * 1024 * 1024 * 1024
	}
	maxSeconds := req.MaxPartDurationSecs
	if maxSeconds <= 0 {
		maxSeconds = 7200
	}
	durationMs := req.DurationMs
	if durationMs <= 0 {
		durationMs = maxSeconds * 1000
	}
	if sizeBytes <= maxBytes && durationMs <= maxSeconds*1000 {
		return PackageResult{Outputs: []PackageOutput{{
			RelativePath:    req.InputRelativePath,
			SizeBytes:       sizeBytes,
			DurationMs:      durationMs,
			TimelineStartMs: 0,
			TimelineEndMs:   durationMs,
		}}}, nil
	}
	outputDir := req.OutputDirRelativePath
	if outputDir == "" {
		outputDir = filepath.ToSlash(filepath.Join("upload-sources", fmt.Sprintf("%d", req.UploadSourceID), "parts"))
	}
	outputDirPath, err := resolveWithinRoot(m.DataRoot, outputDir)
	if err != nil {
		return PackageResult{}, fmt.Errorf("resolve package output dir: %w", err)
	}
	if err := os.RemoveAll(outputDirPath); err != nil {
		return PackageResult{}, fmt.Errorf("clear package output dir: %w", err)
	}
	if err := os.MkdirAll(outputDirPath, 0o755); err != nil {
		return PackageResult{}, fmt.Errorf("create package output dir: %w", err)
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
	pattern := filepath.Join(outputDirPath, fmt.Sprintf("upload-source-%d-part-%%03d.flv", req.UploadSourceID))
	cmd := exec.CommandContext(ctx, m.FFmpegPath, "-hide_banner", "-loglevel", "error", "-i", inputPath, "-c", "copy", "-map", "0", "-f", "segment", "-segment_time", fmt.Sprintf("%d", segmentSeconds), "-reset_timestamps", "1", "-segment_format", "flv", "-y", pattern)
	output, err := cmd.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = err.Error()
		}
		return PackageResult{}, fmt.Errorf("ffmpeg package failed: %s", message)
	}
	matches, err := filepath.Glob(filepath.Join(outputDirPath, fmt.Sprintf("upload-source-%d-part-*.flv", req.UploadSourceID)))
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
		info, err := os.Stat(path)
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
		rel, err := filepath.Rel(rootAbs, path)
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

func escapeConcatPath(path string) string {
	return strings.ReplaceAll(path, "'", "'\\''")
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
