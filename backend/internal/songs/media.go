package songs

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

type AudioCutter interface {
	ExtractAnalysisAudio(context.Context, string, string) error
	CutM4A(context.Context, string, string, int64, int64) (int64, error)
}

type FFmpegAudioCutter struct {
	Path string
}

func (f FFmpegAudioCutter) ExtractAnalysisAudio(ctx context.Context, source, destination string) error {
	return f.runAtomic(ctx, destination, "-i", source, "-vn", "-ac", "1", "-ar", "32000", "-b:a", "64k", "-f", "mp3")
}

func (f FFmpegAudioCutter) CutM4A(ctx context.Context, source, destination string, startMs, endMs int64) (int64, error) {
	if startMs < 0 || endMs <= startMs {
		return 0, ErrValidation
	}
	err := f.runAtomic(ctx, destination,
		"-ss", formatFFmpegSeconds(startMs), "-i", source, "-t", formatFFmpegSeconds(endMs-startMs),
		"-vn", "-c:a", "aac", "-b:a", "192k", "-movflags", "+faststart", "-f", "mp4")
	if err != nil {
		return 0, err
	}
	info, err := os.Stat(destination)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

func (f FFmpegAudioCutter) runAtomic(ctx context.Context, destination string, args ...string) error {
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	temp := destination + ".part"
	_ = os.Remove(temp)
	defer os.Remove(temp)
	path := strings.TrimSpace(f.Path)
	if path == "" {
		path = "ffmpeg"
	}
	commandArgs := []string{"-hide_banner", "-loglevel", "error"}
	commandArgs = append(commandArgs, args...)
	commandArgs = append(commandArgs, "-y", temp)
	output, err := exec.CommandContext(ctx, path, commandArgs...).CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if len(message) > 1000 {
			message = message[len(message)-1000:]
		}
		if errors.Is(ctx.Err(), context.Canceled) {
			return ctx.Err()
		}
		return fmt.Errorf("ffmpeg audio processing failed: %s", message)
	}
	info, err := os.Stat(temp)
	if err != nil || info.Size() == 0 {
		return errors.New("ffmpeg produced no audio output")
	}
	return os.Rename(temp, destination)
}

func formatFFmpegSeconds(milliseconds int64) string {
	return strconv.FormatFloat(float64(milliseconds)/1000, 'f', 3, 64)
}
