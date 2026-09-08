package upload

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/7grecorder/7grecorder/backend/internal/config"
)

const defaultBiliupPath = "biliup"

var bilibiliBVIDPattern = regexp.MustCompile(`BV[0-9A-Za-z]+`)

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

func (u BiliupCLIUploader) Upload(ctx context.Context, request BilibiliUploadRequest) (BilibiliUploadResult, error) {
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

	args := []string{
		"-u", cookieFile,
		"upload",
		"--submit", bilibiliSubmitMode(request.Settings),
		"--limit", strconv.Itoa(bilibiliUploadLimit(request.Settings)),
		"--copyright", strconv.Itoa(bilibiliCopyright(request)),
		"--tid", strconv.Itoa(bilibiliTID(request.Settings)),
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
		if strings.TrimSpace(part.SourcePath) == "" {
			return BilibiliUploadResult{}, NewClassifiedError("SOURCE_MISSING", "bilibili upload part has no source path")
		}
		args = append(args, part.SourcePath)
	}

	cmd := exec.CommandContext(ctx, u.Path, args...)
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(), "HOME="+workDir)
	output, err := cmd.CombinedOutput()
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

func bilibiliTID(settings BilibiliPublishingSettings) int {
	if settings.TID > 0 {
		return settings.TID
	}
	return 171
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
	return 3
}

func bilibiliUploadLine(settings BilibiliPublishingSettings) string {
	value := strings.TrimSpace(settings.Line)
	if value == "" || strings.EqualFold(value, "AUTO") {
		return ""
	}
	return value
}
