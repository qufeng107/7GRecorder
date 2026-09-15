package songs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type RecognitionRequest struct {
	Region           string
	ContainerID      string
	AccessToken      string
	AudioPath        string
	ProviderFilename string
}

type RecognitionMatch struct {
	Engine       string
	ACRID        string
	ISRC         string
	Title        string
	Artist       string
	StartMs      int64
	EndMs        int64
	Score        float64
	EvidenceJSON string
}

type RecognitionResult struct {
	ProviderFileID string
	RawJSON        string
	Matches        []RecognitionMatch
}

type RecognitionProgress struct {
	Message        string
	ProviderFileID string
	PollCount      int
}

type RecognitionReporter func(context.Context, RecognitionProgress) error

type Recognizer interface {
	Recognize(context.Context, RecognitionRequest, RecognitionReporter) (RecognitionResult, error)
}

type ProviderError struct {
	Class   string
	Message string
}

func (e *ProviderError) Error() string { return e.Message }

type ACRCloudRecognizer struct {
	Client       *http.Client
	PollInterval time.Duration
	MaxPollTime  time.Duration
	BaseURL      string
}

func NewACRCloudRecognizer() ACRCloudRecognizer {
	return ACRCloudRecognizer{
		Client:       &http.Client{Timeout: 10 * time.Minute},
		PollInterval: 30 * time.Second,
		MaxPollTime:  2 * time.Hour,
	}
}

func (a ACRCloudRecognizer) Recognize(ctx context.Context, req RecognitionRequest, report RecognitionReporter) (RecognitionResult, error) {
	baseURL := strings.TrimRight(a.BaseURL, "/")
	if baseURL == "" {
		var err error
		baseURL, err = acrCloudBaseURL(req.Region, req.ContainerID)
		if err != nil {
			return RecognitionResult{}, err
		}
	}
	if strings.TrimSpace(req.AccessToken) == "" || strings.TrimSpace(req.ProviderFilename) == "" {
		return RecognitionResult{}, &ProviderError{Class: "PERMANENT", Message: "ACRCloud request is incomplete"}
	}
	client := a.Client
	if client == nil {
		client = http.DefaultClient
	}
	fileID, err := a.findExisting(ctx, client, baseURL, req)
	if err != nil {
		return RecognitionResult{}, err
	}
	if fileID == "" {
		if report != nil {
			if err := report(ctx, RecognitionProgress{Message: "Uploading analysis audio to ACRCloud"}); err != nil {
				return RecognitionResult{}, err
			}
		}
		fileID, err = a.submit(ctx, client, baseURL, req)
		if err != nil {
			return RecognitionResult{}, err
		}
	}
	if report != nil {
		if err := report(ctx, RecognitionProgress{Message: "Waiting for ACRCloud recognition", ProviderFileID: fileID}); err != nil {
			return RecognitionResult{}, err
		}
	}
	interval := a.PollInterval
	if interval <= 0 {
		interval = 30 * time.Second
	}
	maxPoll := a.MaxPollTime
	if maxPoll <= 0 {
		maxPoll = 2 * time.Hour
	}
	deadline := time.NewTimer(maxPoll)
	defer deadline.Stop()
	pollCount := 0
	for {
		pollCount++
		if report != nil {
			if err := report(ctx, RecognitionProgress{Message: "Polling ACRCloud recognition", ProviderFileID: fileID, PollCount: pollCount}); err != nil {
				return RecognitionResult{}, err
			}
		}
		result, ready, err := a.fetch(ctx, client, baseURL, req.AccessToken, fileID)
		if err != nil {
			return RecognitionResult{}, err
		}
		if ready {
			return result, nil
		}
		delay := recognitionPollDelay(interval, pollCount)
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return RecognitionResult{}, ctx.Err()
		case <-deadline.C:
			timer.Stop()
			return RecognitionResult{}, &ProviderError{Class: "TRANSIENT", Message: "ACRCloud recognition timed out"}
		case <-timer.C:
		}
	}
}

func recognitionPollDelay(base time.Duration, pollCount int) time.Duration {
	switch {
	case pollCount >= 4:
		return base * 10
	case pollCount == 3:
		return base * 4
	case pollCount == 2:
		return base * 2
	default:
		return base
	}
}

func acrCloudBaseURL(region, containerID string) (string, error) {
	region = strings.TrimSpace(region)
	switch region {
	case "eu-west-1", "us-west-2", "ap-southeast-1":
	default:
		return "", &ProviderError{Class: "PERMANENT", Message: "unsupported ACRCloud region"}
	}
	containerID = strings.TrimSpace(containerID)
	if containerID == "" || strings.ContainsAny(containerID, "/?#") {
		return "", &ProviderError{Class: "PERMANENT", Message: "invalid ACRCloud container ID"}
	}
	return "https://api-" + region + ".acrcloud.com/api/fs-containers/" + url.PathEscape(containerID) + "/files", nil
}

func (a ACRCloudRecognizer) findExisting(ctx context.Context, client *http.Client, baseURL string, req RecognitionRequest) (string, error) {
	query := url.Values{"page": {"1"}, "per_page": {"20"}, "search": {req.ProviderFilename}, "with_result": {"1"}}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"?"+query.Encode(), nil)
	if err != nil {
		return "", err
	}
	setACRHeaders(httpReq, req.AccessToken)
	body, err := doACRRequest(client, httpReq, http.StatusOK)
	if err != nil {
		return "", err
	}
	var envelope struct {
		Data []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return "", &ProviderError{Class: "PERMANENT", Message: "malformed ACRCloud file-list response"}
	}
	for _, item := range envelope.Data {
		if item.Name == req.ProviderFilename && item.ID != "" {
			return item.ID, nil
		}
	}
	return "", nil
}

func (a ACRCloudRecognizer) submit(ctx context.Context, client *http.Client, baseURL string, req RecognitionRequest) (string, error) {
	file, err := os.Open(req.AudioPath)
	if err != nil {
		return "", &ProviderError{Class: "SOURCE_MISSING", Message: fmt.Sprintf("open analysis audio: %v", err)}
	}
	reader, pipeWriter := io.Pipe()
	writer := multipart.NewWriter(pipeWriter)
	contentType := writer.FormDataContentType()
	go func() {
		defer file.Close()
		fail := func(err error) { _ = pipeWriter.CloseWithError(err) }
		part, err := writer.CreateFormFile("file", filepath.Base(req.ProviderFilename))
		if err != nil {
			fail(err)
			return
		}
		if _, err := io.Copy(part, file); err != nil {
			fail(err)
			return
		}
		if err := writer.WriteField("data_type", "audio"); err != nil {
			fail(err)
			return
		}
		if err := writer.WriteField("name", req.ProviderFilename); err != nil {
			fail(err)
			return
		}
		if err := writer.Close(); err != nil {
			fail(err)
			return
		}
		_ = pipeWriter.Close()
	}()
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL, reader)
	if err != nil {
		return "", err
	}
	setACRHeaders(httpReq, req.AccessToken)
	httpReq.Header.Set("Content-Type", contentType)
	response, err := doACRRequest(client, httpReq, http.StatusOK, http.StatusCreated)
	if err != nil {
		return "", err
	}
	var envelope struct {
		Data struct {
			ID     string `json:"id"`
			FileID string `json:"file_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response, &envelope); err != nil {
		return "", &ProviderError{Class: "PERMANENT", Message: "malformed ACRCloud submission response"}
	}
	if envelope.Data.ID != "" {
		return envelope.Data.ID, nil
	}
	if envelope.Data.FileID != "" {
		return envelope.Data.FileID, nil
	}
	return "", &ProviderError{Class: "PERMANENT", Message: "ACRCloud submission returned no file ID"}
}

func (a ACRCloudRecognizer) fetch(ctx context.Context, client *http.Client, baseURL, token, fileID string) (RecognitionResult, bool, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/"+url.PathEscape(fileID), nil)
	if err != nil {
		return RecognitionResult{}, false, err
	}
	setACRHeaders(httpReq, token)
	body, err := doACRRequest(client, httpReq, http.StatusOK)
	if err != nil {
		return RecognitionResult{}, false, err
	}
	item, err := decodeACRFile(body)
	if err != nil {
		return RecognitionResult{}, false, err
	}
	switch item.State {
	case 0:
		return RecognitionResult{}, false, nil
	case 1:
		matches, err := parseACRMatches(item.Results)
		if err != nil {
			return RecognitionResult{}, false, err
		}
		return RecognitionResult{ProviderFileID: fileID, RawJSON: string(body), Matches: matches}, true, nil
	case -1:
		return RecognitionResult{ProviderFileID: fileID, RawJSON: string(body), Matches: []RecognitionMatch{}}, true, nil
	default:
		return RecognitionResult{}, false, &ProviderError{Class: "PERMANENT", Message: fmt.Sprintf("ACRCloud file entered error state %d", item.State)}
	}
}

type acrFile struct {
	State   int             `json:"state"`
	Results json.RawMessage `json:"results"`
}

func decodeACRFile(body []byte) (acrFile, error) {
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil || len(envelope.Data) == 0 {
		return acrFile{}, &ProviderError{Class: "PERMANENT", Message: "malformed ACRCloud result response"}
	}
	var item acrFile
	if envelope.Data[0] == '[' {
		var items []acrFile
		if err := json.Unmarshal(envelope.Data, &items); err != nil || len(items) == 0 {
			return acrFile{}, &ProviderError{Class: "PERMANENT", Message: "malformed ACRCloud result list"}
		}
		item = items[0]
	} else if err := json.Unmarshal(envelope.Data, &item); err != nil {
		return acrFile{}, &ProviderError{Class: "PERMANENT", Message: "malformed ACRCloud result item"}
	}
	return item, nil
}

func parseACRMatches(raw json.RawMessage) ([]RecognitionMatch, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return []RecognitionMatch{}, nil
	}
	var groups struct {
		Music      []json.RawMessage `json:"music"`
		CoverSongs []json.RawMessage `json:"cover_songs"`
	}
	if err := json.Unmarshal(raw, &groups); err != nil {
		return nil, &ProviderError{Class: "PERMANENT", Message: "malformed ACRCloud results payload"}
	}
	matches := make([]RecognitionMatch, 0, len(groups.Music)+len(groups.CoverSongs))
	for _, group := range []struct {
		engine string
		items  []json.RawMessage
	}{{"ACRCLOUD_FINGERPRINT", groups.Music}, {"ACRCLOUD_COVER", groups.CoverSongs}} {
		for _, rawItem := range group.items {
			var item struct {
				Offset         float64 `json:"offset"`
				PlayedDuration float64 `json:"played_duration"`
				Result         struct {
					ACRID                   string  `json:"acrid"`
					Title                   string  `json:"title"`
					Score                   float64 `json:"score"`
					SampleBeginTimeOffsetMs int64   `json:"sample_begin_time_offset_ms"`
					SampleEndTimeOffsetMs   int64   `json:"sample_end_time_offset_ms"`
					Artists                 []struct {
						Name string `json:"name"`
					} `json:"artists"`
					ExternalIDs struct {
						ISRC string `json:"isrc"`
					} `json:"external_ids"`
				} `json:"result"`
			}
			if err := json.Unmarshal(rawItem, &item); err != nil {
				return nil, &ProviderError{Class: "PERMANENT", Message: "malformed ACRCloud match"}
			}
			start := int64(item.Offset * 1000)
			end := start + int64(item.PlayedDuration*1000)
			if item.PlayedDuration <= 0 && item.Result.SampleEndTimeOffsetMs > item.Result.SampleBeginTimeOffsetMs {
				start = item.Result.SampleBeginTimeOffsetMs
				end = item.Result.SampleEndTimeOffsetMs
			}
			if strings.TrimSpace(item.Result.Title) == "" || end <= start {
				continue
			}
			artists := make([]string, 0, len(item.Result.Artists))
			for _, artist := range item.Result.Artists {
				if name := strings.TrimSpace(artist.Name); name != "" {
					artists = append(artists, name)
				}
			}
			matches = append(matches, RecognitionMatch{
				Engine: group.engine, ACRID: item.Result.ACRID, ISRC: item.Result.ExternalIDs.ISRC,
				Title: strings.TrimSpace(item.Result.Title), Artist: strings.Join(artists, ", "), StartMs: start,
				EndMs: end, Score: item.Result.Score, EvidenceJSON: string(rawItem),
			})
		}
	}
	sort.SliceStable(matches, func(i, j int) bool { return matches[i].StartMs < matches[j].StartMs })
	return coalesceMatches(matches), nil
}

func coalesceMatches(items []RecognitionMatch) []RecognitionMatch {
	result := make([]RecognitionMatch, 0, len(items))
	for _, item := range items {
		if len(result) == 0 {
			result = append(result, item)
			continue
		}
		last := &result[len(result)-1]
		sameID := item.ACRID != "" && item.ACRID == last.ACRID
		if !sameID {
			sameID = item.ISRC != "" && item.ISRC == last.ISRC
		}
		if !sameID {
			sameID = strings.EqualFold(item.Title, last.Title) && strings.EqualFold(item.Artist, last.Artist)
		}
		if sameID && item.StartMs <= last.EndMs+15000 {
			if item.EndMs > last.EndMs {
				last.EndMs = item.EndMs
			}
			if item.Score > last.Score {
				last.Score = item.Score
			}
			continue
		}
		result = append(result, item)
	}
	return result
}

func setACRHeaders(req *http.Request, token string) {
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(token))
}

func doACRRequest(client *http.Client, req *http.Request, expected ...int) ([]byte, error) {
	resp, err := client.Do(req)
	if err != nil {
		return nil, &ProviderError{Class: "TRANSIENT", Message: fmt.Sprintf("ACRCloud request failed: %v", err)}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, &ProviderError{Class: "TRANSIENT", Message: "read ACRCloud response failed"}
	}
	for _, status := range expected {
		if resp.StatusCode == status {
			return body, nil
		}
	}
	class := "TRANSIENT"
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		class = "AUTH"
	} else if resp.StatusCode >= 400 && resp.StatusCode < 500 && resp.StatusCode != http.StatusTooManyRequests {
		class = "PERMANENT"
	}
	message := strings.TrimSpace(string(body))
	if len(message) > 500 {
		message = message[:500]
	}
	return nil, &ProviderError{Class: class, Message: "ACRCloud HTTP " + strconv.Itoa(resp.StatusCode) + ": " + message}
}

func ProviderErrorClass(err error) string {
	var providerErr *ProviderError
	if errors.As(err, &providerErr) && providerErr.Class != "" {
		return providerErr.Class
	}
	return "TRANSIENT"
}
