package upload

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	defaultBilibiliTitleTemplate       = "{{profile_name}} {{date_compact}} 第{{live_ordinal}}场直播"
	defaultBilibiliDescriptionTemplate = "主播：{{streamer_name}}\n直播间：{{room_id}}\n录制时间：{{started_at_china}} - {{completed_at_china}}\n分片：{{part_count}} 个\n\n由 7GRecorder 自动归档。"
)

type BilibiliPublishingSettings struct {
	TitleTemplate       string   `json:"title_template"`
	DescriptionTemplate string   `json:"description_template"`
	Tags                []string `json:"tags,omitempty"`
	Copyright           int      `json:"copyright,omitempty"`
	Source              string   `json:"source,omitempty"`
}

type BilibiliJobPayload struct {
	PublicationID  int64 `json:"publication_id"`
	UploadSourceID int64 `json:"upload_source_id"`
}

type BilibiliUploadPart struct {
	OutputID        int64  `json:"output_id"`
	Title          string `json:"title"`
	RelativePath   string `json:"relative_path"`
	SourcePath     string `json:"-"`
	SizeBytes      int64  `json:"size_bytes"`
	DurationMs     int64  `json:"duration_ms"`
	TimelineStartMs int64  `json:"timeline_start_ms"`
	TimelineEndMs   int64  `json:"timeline_end_ms"`
}

type BilibiliUploadRequest struct {
	PublicationID      int64                       `json:"publication_id"`
	UploadSourceID     int64                       `json:"upload_source_id"`
	RecordingProfileID int64                       `json:"recording_profile_id"`
	CredentialID       int64                       `json:"credential_id"`
	Title              string                      `json:"title"`
	Description        string                      `json:"description"`
	Tags               []string                    `json:"tags,omitempty"`
	Copyright          int                         `json:"copyright,omitempty"`
	Source             string                      `json:"source,omitempty"`
	Parts              []BilibiliUploadPart        `json:"parts"`
	Settings           BilibiliPublishingSettings `json:"settings"`
	Secret             json.RawMessage             `json:"-"`
}

type BilibiliUploadResult struct {
	ExternalID  string
	ExternalURL string
}

type BilibiliUploader interface {
	Upload(ctx context.Context, request BilibiliUploadRequest) (BilibiliUploadResult, error)
}

type NoopBilibiliUploader struct{}

func NewNoopBilibiliUploader() NoopBilibiliUploader {
	return NoopBilibiliUploader{}
}

func (NoopBilibiliUploader) Upload(_ context.Context, _ BilibiliUploadRequest) (BilibiliUploadResult, error) {
	return BilibiliUploadResult{}, NewClassifiedError("PERMANENT", "bilibili uploader adapter is not configured yet")
}

func NormalizeBilibiliSettings(raw json.RawMessage) (json.RawMessage, error) {
	settings, err := parseBilibiliSettings(raw)
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(settings)
	if err != nil {
		return nil, err
	}
	return encoded, nil
}

func (s Store) BilibiliUploadRequest(ctx context.Context, payload BilibiliJobPayload) (BilibiliUploadRequest, error) {
	if payload.PublicationID <= 0 || payload.UploadSourceID <= 0 {
		return BilibiliUploadRequest{}, ErrValidation
	}

	var request BilibiliUploadRequest
	var encryptedSecret []byte
	var settingsJSON string
	var source struct {
		ProfileName  string
		RoomID       string
		StreamerName string
		Title        string
		StartedAt    string
		CompletedAt  string
		DurationMs   int64
		TotalBytes   int64
	}
	err := s.db.QueryRowContext(ctx, `
		SELECT p.id,
			p.upload_source_id,
			p.recording_profile_id,
			p.credential_id,
			pp.settings_json,
			c.encrypted_secret,
			rp.name,
			us.source_room_id,
			us.streamer_name_snapshot,
			COALESCE(us.title, ''),
			us.started_at,
			us.completed_at,
			us.duration_ms,
			us.total_bytes
		FROM publications p
		JOIN upload_sources us ON us.id = p.upload_source_id
		JOIN recording_profiles rp ON rp.id = p.recording_profile_id
		JOIN publishing_profiles pp ON pp.recording_profile_id = p.recording_profile_id
			AND pp.platform = 'bilibili'
			AND pp.enabled = 1
			AND pp.credential_id = p.credential_id
		JOIN credentials c ON c.id = p.credential_id
		WHERE p.id = ?
			AND p.upload_source_id = ?
			AND p.platform = 'bilibili'
			AND p.status IN ('PENDING', 'FAILED')
			AND us.status = 'READY_TO_UPLOAD'
	`, payload.PublicationID, payload.UploadSourceID).Scan(
		&request.PublicationID,
		&request.UploadSourceID,
		&request.RecordingProfileID,
		&request.CredentialID,
		&settingsJSON,
		&encryptedSecret,
		&source.ProfileName,
		&source.RoomID,
		&source.StreamerName,
		&source.Title,
		&source.StartedAt,
		&source.CompletedAt,
		&source.DurationMs,
		&source.TotalBytes,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return BilibiliUploadRequest{}, ErrNotFound
	}
	if err != nil {
		return BilibiliUploadRequest{}, fmt.Errorf("load bilibili upload request: %w", err)
	}

	secret, err := s.decryptSecret(encryptedSecret)
	if err != nil {
		return BilibiliUploadRequest{}, err
	}
	settings, err := parseBilibiliSettings(json.RawMessage(settingsJSON))
	if err != nil {
		return BilibiliUploadRequest{}, err
	}
	parts, err := s.bilibiliUploadParts(ctx, request.UploadSourceID)
	if err != nil {
		return BilibiliUploadRequest{}, err
	}
	if len(parts) == 0 {
		return BilibiliUploadRequest{}, ErrNotReady
	}
	for index := range parts {
		sourcePath, err := resolveWithinRoot(s.cfg.DataRoot, parts[index].RelativePath)
		if err != nil {
			return BilibiliUploadRequest{}, fmt.Errorf("resolve bilibili upload part: %w", err)
		}
		info, err := os.Stat(sourcePath)
		if errors.Is(err, os.ErrNotExist) {
			return BilibiliUploadRequest{}, NewClassifiedError("SOURCE_MISSING", "bilibili upload source file is missing")
		}
		if err != nil {
			return BilibiliUploadRequest{}, fmt.Errorf("stat bilibili upload part: %w", err)
		}
		if info.IsDir() {
			return BilibiliUploadRequest{}, NewClassifiedError("SOURCE_MISSING", "bilibili upload source path is a directory")
		}
		parts[index].SourcePath = sourcePath
		parts[index].Title = partTitle(parts[index].RelativePath, index+1)
	}

	vars := bilibiliTemplateVars{
		ProfileName:  source.ProfileName,
		StreamerName: source.StreamerName,
		RoomID:       source.RoomID,
		SourceTitle:  source.Title,
		StartedAt:    source.StartedAt,
		CompletedAt:  source.CompletedAt,
		LiveOrdinal:  s.uploadSourceChinaOrdinal(ctx, request.RecordingProfileID, request.UploadSourceID, source.StartedAt),
		PartCount:    len(parts),
	}
	request.Title = renderBilibiliTemplate(settings.TitleTemplate, vars)
	request.Description = renderBilibiliTemplate(settings.DescriptionTemplate, vars)
	request.Tags = settings.Tags
	request.Copyright = settings.Copyright
	request.Source = settings.Source
	request.Parts = parts
	request.Settings = settings
	request.Secret = secret
	return request, nil
}

func (s Store) MarkBilibiliUploading(ctx context.Context, publicationID int64, request BilibiliUploadRequest) error {
	if publicationID <= 0 {
		return ErrValidation
	}
	snapshot, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("encode bilibili request snapshot: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `
		UPDATE publications
		SET status = 'UPLOADING',
			request_snapshot_json = ?,
			last_error = NULL,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, string(snapshot), publicationID)
	if err != nil {
		return fmt.Errorf("mark bilibili uploading: %w", err)
	}
	return nil
}

func (s Store) MarkBilibiliUploaded(ctx context.Context, publicationID int64, result BilibiliUploadResult) error {
	if publicationID <= 0 {
		return ErrValidation
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE publications
		SET status = 'VERIFIED',
			external_id = NULLIF(?, ''),
			external_url = NULLIF(?, ''),
			last_error = NULL,
			published_at = CURRENT_TIMESTAMP,
			verified_at = CURRENT_TIMESTAMP,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, result.ExternalID, result.ExternalURL, publicationID)
	if err != nil {
		return fmt.Errorf("mark bilibili uploaded: %w", err)
	}
	return nil
}

func (s Store) MarkBilibiliUploadFailed(ctx context.Context, publicationID int64, errorClass string, message string) error {
	if publicationID <= 0 {
		return ErrValidation
	}
	status := "FAILED"
	if errorClass == "SOURCE_MISSING" {
		status = "SOURCE_MISSING"
	}
	if errorClass == "AMBIGUOUS" {
		status = "AMBIGUOUS"
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE publications
		SET status = ?,
			last_error = ?,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, status, message, publicationID)
	if err != nil {
		return fmt.Errorf("mark bilibili failed: %w", err)
	}
	return nil
}

func (s Store) bilibiliUploadParts(ctx context.Context, uploadSourceID int64) ([]BilibiliUploadPart, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, relative_path, size_bytes, duration_ms, timeline_start_ms, timeline_end_ms
		FROM upload_source_outputs
		WHERE upload_source_id = ? AND status = 'READY_TO_UPLOAD'
		ORDER BY sort_order ASC, id ASC
	`, uploadSourceID)
	if err != nil {
		return nil, fmt.Errorf("list bilibili upload parts: %w", err)
	}
	defer rows.Close()

	items := make([]BilibiliUploadPart, 0)
	for rows.Next() {
		var item BilibiliUploadPart
		if err := rows.Scan(&item.OutputID, &item.RelativePath, &item.SizeBytes, &item.DurationMs, &item.TimelineStartMs, &item.TimelineEndMs); err != nil {
			return nil, fmt.Errorf("scan bilibili upload part: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate bilibili upload parts: %w", err)
	}
	return items, nil
}

func parseBilibiliSettings(raw json.RawMessage) (BilibiliPublishingSettings, error) {
	settings := BilibiliPublishingSettings{}
	value := strings.TrimSpace(string(raw))
	if value != "" && value != "null" {
		if err := json.Unmarshal(raw, &settings); err != nil {
			return BilibiliPublishingSettings{}, ErrValidation
		}
	}
	settings.TitleTemplate = strings.TrimSpace(settings.TitleTemplate)
	if settings.TitleTemplate == "" {
		settings.TitleTemplate = defaultBilibiliTitleTemplate
	}
	settings.DescriptionTemplate = strings.TrimSpace(settings.DescriptionTemplate)
	if settings.DescriptionTemplate == "" {
		settings.DescriptionTemplate = defaultBilibiliDescriptionTemplate
	}
	if len([]rune(settings.TitleTemplate)) > 80 {
		return BilibiliPublishingSettings{}, ErrValidation
	}
	settings.Tags = cleanTags(settings.Tags)
	return settings, nil
}

func cleanTags(tags []string) []string {
	seen := map[string]bool{}
	cleaned := make([]string, 0, len(tags))
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" || seen[tag] {
			continue
		}
		seen[tag] = true
		cleaned = append(cleaned, tag)
	}
	return cleaned
}

type bilibiliTemplateVars struct {
	ProfileName  string
	StreamerName string
	RoomID       string
	SourceTitle  string
	StartedAt    string
	CompletedAt  string
	LiveOrdinal  int
	PartCount    int
}

func renderBilibiliTemplate(template string, vars bilibiliTemplateVars) string {
	started := parseTime(vars.StartedAt)
	completed := parseTime(vars.CompletedAt)
	chinaStarted := started.In(chinaLocation())
	chinaCompleted := completed.In(chinaLocation())
	values := map[string]string{
		"profile_name":       vars.ProfileName,
		"streamer_name":      vars.StreamerName,
		"room_id":            vars.RoomID,
		"source_title":       vars.SourceTitle,
		"date":               chinaStarted.Format("2006/01/02"),
		"date_compact":       chinaStarted.Format("20060102"),
		"start_time":         chinaStarted.Format("15:04:05"),
		"end_time":           chinaCompleted.Format("15:04:05"),
		"started_at_china":   chinaStarted.Format("2006/01/02 15:04:05"),
		"completed_at_china": chinaCompleted.Format("2006/01/02 15:04:05"),
		"live_ordinal":       fmt.Sprintf("%02d", vars.LiveOrdinal),
		"part_count":         strconv.Itoa(vars.PartCount),
	}
	result := template
	for key, value := range values {
		result = strings.ReplaceAll(result, "{{"+key+"}}", value)
	}
	return strings.TrimSpace(result)
}

func (s Store) uploadSourceChinaOrdinal(ctx context.Context, profileID int64, uploadSourceID int64, startedAt string) int {
	started := parseTime(startedAt)
	if started.IsZero() {
		return 1
	}
	chinaDate := started.In(chinaLocation()).Format("20060102")
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, started_at
		FROM upload_sources
		WHERE recording_profile_id = ?
		ORDER BY started_at ASC, id ASC
	`, profileID)
	if err != nil {
		return 1
	}
	defer rows.Close()

	ordinal := 0
	for rows.Next() {
		var id int64
		var startedAt string
		if err := rows.Scan(&id, &startedAt); err != nil {
			return 1
		}
		parsed := parseTime(startedAt)
		if parsed.IsZero() || parsed.In(chinaLocation()).Format("20060102") != chinaDate {
			continue
		}
		ordinal++
		if id == uploadSourceID {
			return ordinal
		}
	}
	return 1
}

func partTitle(relativePath string, fallback int) string {
	name := strings.TrimSuffix(filepath.Base(relativePath), filepath.Ext(relativePath))
	if strings.TrimSpace(name) == "" {
		return fmt.Sprintf("P%02d", fallback)
	}
	return name
}

func parseTime(value string) time.Time {
	parsed, err := time.Parse(time.RFC3339, value)
	if err == nil {
		return parsed
	}
	layouts := []string{"2006-01-02 15:04:05", "2006-01-02T15:04:05Z07:00"}
	for _, layout := range layouts {
		parsed, err = time.Parse(layout, value)
		if err == nil {
			return parsed
		}
	}
	return time.Time{}
}

func chinaLocation() *time.Location {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return time.FixedZone("CST", 8*60*60)
	}
	return location
}
