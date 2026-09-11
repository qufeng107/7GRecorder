package upload

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/7grecorder/7grecorder/backend/internal/config"
)

const defaultBiliupPath = "biliup"

var (
	bilibiliBVIDPattern     = regexp.MustCompile(`BV[0-9A-Za-z]+`)
	biliupFileProgressRegex = regexp.MustCompile(`([0-9]+(?:\.[0-9]+)?)\s+([KMGT]?i?B)/([0-9]+(?:\.[0-9]+)?)\s+([KMGT]?i?B)`)
	biliupCompletedRegex    = regexp.MustCompile(`Upload completed:\s+(.+?)\s+=>`)
)

type BiliupCLIUploader struct {
	Path     string
	TempRoot string
}

func NewBiliupCLIUploader(cfg config.Config) BiliupCLIUploader {
	path := strings.TrimSpace(cfg.BiliupPath)
	if path == "" {
		path = defaultBiliupPath
	}
	tempRoot := strings.TrimSpace(cfg.TempRoot)
	if tempRoot == "" {
		tempRoot = filepath.Join(cfg.DataRoot, "temp")
	}
	return BiliupCLIUploader{Path: path, TempRoot: tempRoot}
}

func (u BiliupCLIUploader) Upload(ctx context.Context, request BilibiliUploadRequest, progress ProgressReporter) (BilibiliUploadResult, error) {
	if strings.TrimSpace(u.Path) == "" {
		return BilibiliUploadResult{}, NewClassifiedError("PERMANENT", "biliup path is not configured")
	}
	if request.PublicationID <= 0 || request.Title == "" || len(request.Parts) == 0 {
		return BilibiliUploadResult{}, ErrValidation
	}
	if err := os.MkdirAll(u.TempRoot, 0o755); err != nil {
		return BilibiliUploadResult{}, fmt.Errorf("create biliup temp root: %w", err)
	}
	workDir, err := os.MkdirTemp(u.TempRoot, fmt.Sprintf("biliup-%d-", request.PublicationID))
	if err != nil {
		return BilibiliUploadResult{}, fmt.Errorf("create biliup temp dir: %w", err)
	}
	defer os.RemoveAll(workDir)

	cookieFile := filepath.Join(workDir, "cookies.json")
	if err := writeBiliupCookieFile(cookieFile, request.Secret); err != nil {
		return BilibiliUploadResult{}, err
	}

	primaryTID := bilibiliTID(request.Settings)
	fallbackTID := bilibiliFallbackTID(request.Settings)
	for _, part := range request.Parts {
		if strings.TrimSpace(part.SourcePath) == "" {
			return BilibiliUploadResult{}, NewClassifiedError("SOURCE_MISSING", "bilibili upload part has no source path")
		}
	}

	cmd := exec.CommandContext(ctx, u.Path, buildBiliupArgs(cookieFile, request, primaryTID)...)
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(), "HOME="+workDir)
	output, err := runBiliupCommand(ctx, cmd, request, progress)
	if err != nil && fallbackTID > 0 && fallbackTID != primaryTID && isBiliupPartitionFailure(output) {
		if progress != nil {
			progress(ctx, UploadProgress{CurrentBytes: 0, TotalBytes: totalBilibiliPartBytes(request), Message: fmt.Sprintf("retrying with fallback tid %d", fallbackTID)})
		}
		cmd = exec.CommandContext(ctx, u.Path, buildBiliupArgs(cookieFile, request, fallbackTID)...)
		cmd.Dir = workDir
		cmd.Env = append(os.Environ(), "HOME="+workDir)
		output, err = runBiliupCommand(ctx, cmd, request, progress)
	}
	if err != nil {
		return BilibiliUploadResult{}, classifyBiliupFailure(err, output)
	}
	externalID := parseBiliupExternalID(output)
	externalURL := ""
	if externalID != "" {
		externalURL = "https://www.bilibili.com/video/" + externalID
	}
	return BilibiliUploadResult{ExternalID: externalID, ExternalURL: externalURL}, nil
}

func buildBiliupArgs(cookieFile string, request BilibiliUploadRequest, tid int) []string {
	args := []string{
		"-u", cookieFile,
		"upload",
		"--submit", bilibiliSubmitMode(request.Settings),
		"--limit", strconv.Itoa(bilibiliUploadLimit(request.Settings)),
		"--copyright", strconv.Itoa(bilibiliCopyright(request)),
		"--tid", strconv.Itoa(tid),
		"--title", request.Title,
		"--desc", request.Description,
	}
	if tags := strings.Join(request.Tags, ","); tags != "" {
		args = append(args, "--tag", tags)
	}
	if strings.TrimSpace(request.Source) != "" {
		args = append(args, "--source", request.Source)
	}
	if line := bilibiliUploadLine(request.Settings); line != "" {
		args = append(args, "--line", line)
	}
	for _, part := range request.Parts {
		args = append(args, part.SourcePath)
	}
	return args
}

func runBiliupCommand(ctx context.Context, cmd *exec.Cmd, request BilibiliUploadRequest, progress ProgressReporter) ([]byte, error) {
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("open biliup stdout: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("open biliup stderr: %w", err)
	}
	state := newBiliupProgressState(request)
	var output bytes.Buffer
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	done := make(chan error, 2)
	var outputMu sync.Mutex
	go func() { done <- collectBiliupOutput(ctx, stdout, &output, &outputMu, state, progress) }()
	go func() { done <- collectBiliupOutput(ctx, stderr, &output, &outputMu, state, progress) }()
	var collectErr error
	for i := 0; i < 2; i++ {
		if err := <-done; err != nil && collectErr == nil {
			collectErr = err
		}
	}
	waitErr := cmd.Wait()
	if progress != nil && waitErr == nil {
		progress(ctx, UploadProgress{CurrentBytes: state.totalBytes, TotalBytes: state.totalBytes, Message: "upload submitted"})
	}
	if waitErr != nil {
		return output.Bytes(), waitErr
	}
	if collectErr != nil {
		return output.Bytes(), collectErr
	}
	return output.Bytes(), nil
}

func collectBiliupOutput(ctx context.Context, reader io.Reader, output *bytes.Buffer, outputMu *sync.Mutex, state *biliupProgressState, progress ProgressReporter) error {
	chunk := make([]byte, 4096)
	line := strings.Builder{}
	for {
		n, err := reader.Read(chunk)
		if n > 0 {
			text := string(chunk[:n])
			outputMu.Lock()
			output.WriteString(text)
			outputMu.Unlock()
			for _, r := range text {
				if r == '\n' || r == '\r' {
					state.observe(ctx, line.String(), progress)
					line.Reset()
					continue
				}
				line.WriteRune(r)
			}
		}
		if errors.Is(err, io.EOF) {
			if line.Len() > 0 {
				state.observe(ctx, line.String(), progress)
			}
			return nil
		}
		if err != nil {
			return err
		}
	}
}

type biliupProgressState struct {
	mu             sync.Mutex
	totalBytes     int64
	completedBytes int64
	partSizes      map[string]int64
	lastCurrent    int64
}

func newBiliupProgressState(request BilibiliUploadRequest) *biliupProgressState {
	state := &biliupProgressState{partSizes: map[string]int64{}}
	for _, part := range request.Parts {
		state.totalBytes += part.SizeBytes
		state.partSizes[filepath.Base(part.SourcePath)] = part.SizeBytes
	}
	return state
}

func (s *biliupProgressState) observe(ctx context.Context, line string, progress ProgressReporter) {
	if progress == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}
	if match := biliupCompletedRegex.FindStringSubmatch(line); len(match) == 2 {
		if size := s.partSizes[strings.TrimSpace(match[1])]; size > 0 {
			s.completedBytes += size
			if s.completedBytes > s.totalBytes {
				s.completedBytes = s.totalBytes
			}
			s.lastCurrent = s.completedBytes
			progress(ctx, UploadProgress{CurrentBytes: s.completedBytes, TotalBytes: s.totalBytes, Message: "uploaded " + filepath.Base(match[1])})
		}
		return
	}
	if match := biliupFileProgressRegex.FindStringSubmatch(line); len(match) == 5 {
		current := parseBiliupBytes(match[1], match[2])
		absolute := s.completedBytes + current
		if absolute < s.lastCurrent {
			absolute = s.lastCurrent
		}
		if s.totalBytes > 0 && absolute > s.totalBytes {
			absolute = s.totalBytes
		}
		s.lastCurrent = absolute
		progress(ctx, UploadProgress{CurrentBytes: absolute, TotalBytes: s.totalBytes, Message: "uploading to bilibili"})
		return
	}
	if strings.Contains(line, "pre_upload") || strings.Contains(line, "Retry attempt") || strings.Contains(line, "APP") {
		progress(ctx, UploadProgress{CurrentBytes: s.lastCurrent, TotalBytes: s.totalBytes, Message: truncateBiliupProgressMessage(line)})
	}
}

func parseBiliupBytes(number string, unit string) int64 {
	value, err := strconv.ParseFloat(number, 64)
	if err != nil {
		return 0
	}
	switch strings.ToLower(unit) {
	case "gib", "gb":
		value *= 1024 * 1024 * 1024
	case "mib", "mb":
		value *= 1024 * 1024
	case "kib", "kb":
		value *= 1024
	}
	return int64(value)
}

func truncateBiliupProgressMessage(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 160 {
		return value[:160]
	}
	return value
}

func writeBiliupCookieFile(path string, raw json.RawMessage) error {
	cookie, err := normalizeBiliupCookie(raw)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create biliup credential dir: %w", err)
	}
	return os.WriteFile(path, cookie, 0o600)
}

func normalizeBiliupCookie(raw json.RawMessage) ([]byte, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, NewClassifiedError("AUTH", "bilibili credential is empty")
	}
	var wrapper map[string]json.RawMessage
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		return nil, NewClassifiedError("AUTH", "bilibili credential must be JSON")
	}
	for _, key := range []string{"cookie_file", "cookie_json", "biliup_cookie"} {
		if value, ok := wrapper[key]; ok && len(bytes.TrimSpace(value)) > 0 {
			if bytes.HasPrefix(bytes.TrimSpace(value), []byte(`"`)) {
				var encoded string
				if err := json.Unmarshal(value, &encoded); err != nil {
					return nil, NewClassifiedError("AUTH", "bilibili cookie_file_json is invalid")
				}
				value = []byte(encoded)
			}
			if !json.Valid(value) {
				return nil, NewClassifiedError("AUTH", "bilibili cookie file content must be valid JSON")
			}
			return value, nil
		}
	}
	if _, ok := wrapper["cookie"]; ok {
		return nil, NewClassifiedError("AUTH", "bilibili credential must use a biliup-generated cookies.json file, not a browser cookie string")
	}
	if _, ok := wrapper["cookie_info"]; ok {
		return raw, nil
	}
	if _, ok := wrapper["token_info"]; ok {
		return raw, nil
	}
	if _, ok := wrapper["cookies"]; ok {
		return raw, nil
	}
	return raw, nil
}

func classifyBiliupFailure(err error, output []byte) error {
	message := strings.TrimSpace(string(output))
	if message == "" {
		message = err.Error()
	}
	lower := strings.ToLower(message)
	switch {
	case errors.Is(err, exec.ErrNotFound), strings.Contains(lower, "executable file not found"):
		return NewClassifiedError("PERMANENT", "biliup executable was not found")
	case strings.Contains(lower, "cookie"), strings.Contains(lower, "csrf"), strings.Contains(lower, "sessdata"), strings.Contains(lower, "login"):
		return NewClassifiedError("AUTH", message)
	case strings.Contains(lower, "timeout"), strings.Contains(lower, "connection"), strings.Contains(lower, "network"):
		return NewClassifiedError("TRANSIENT", message)
	default:
		return NewClassifiedError("AMBIGUOUS", message)
	}
}

func parseBiliupExternalID(output []byte) string {
	match := bilibiliBVIDPattern.FindString(string(output))
	return strings.TrimSpace(match)
}

func isBiliupPartitionFailure(output []byte) bool {
	message := strings.ToLower(string(output))
	return strings.Contains(message, "tid") ||
		strings.Contains(message, "partition") ||
		strings.Contains(message, "category") ||
		strings.Contains(string(output), "分区")
}

func totalBilibiliPartBytes(request BilibiliUploadRequest) int64 {
	var total int64
	for _, part := range request.Parts {
		total += part.SizeBytes
	}
	return total
}

func bilibiliTID(settings BilibiliPublishingSettings) int {
	if settings.TID > 0 {
		return settings.TID
	}
	return defaultBilibiliTID
}

func bilibiliFallbackTID(settings BilibiliPublishingSettings) int {
	if settings.FallbackTID > 0 {
		return settings.FallbackTID
	}
	return defaultBilibiliFallbackTID
}

func bilibiliCopyright(request BilibiliUploadRequest) int {
	switch request.Copyright {
	case 1, 2:
		return request.Copyright
	default:
		return 1
	}
}

func bilibiliSubmitMode(settings BilibiliPublishingSettings) string {
	value := strings.ToLower(strings.TrimSpace(settings.Submit))
	switch value {
	case "app", "web", "client":
		return value
	default:
		return "app"
	}
}

func bilibiliUploadLimit(settings BilibiliPublishingSettings) int {
	if settings.UploadLimit > 0 && settings.UploadLimit <= 8 {
		return settings.UploadLimit
	}
	return 1
}

func bilibiliUploadLine(settings BilibiliPublishingSettings) string {
	value := strings.TrimSpace(settings.Line)
	if value == "" || strings.EqualFold(value, "AUTO") {
		return ""
	}
	return value
}
