package media

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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

type Merger interface {
	Merge(ctx context.Context, req MergeRequest) (MergeResult, error)
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
