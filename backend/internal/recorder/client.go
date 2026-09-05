package recorder

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/7grecorder/7grecorder/backend/internal/config"
)

var ErrNotConfigured = errors.New("recorder base url is not configured")

type SyncClient interface {
	SyncProfile(ctx context.Context, desired DesiredProfile) (RuntimeStatus, error)
}

type HTTPClient struct {
	baseURL  string
	username string
	password string
	client   *http.Client
}

func NewHTTPClient(cfg config.Config) HTTPClient {
	return HTTPClient{
		baseURL:  strings.TrimRight(cfg.RecorderBaseURL, "/"),
		username: cfg.RecorderUser,
		password: cfg.RecorderPassword,
		client:   &http.Client{Timeout: 15 * time.Second},
	}
}

func (c HTTPClient) SyncProfile(ctx context.Context, desired DesiredProfile) (RuntimeStatus, error) {
	if c.baseURL == "" {
		return RuntimeStatus{}, ErrNotConfigured
	}
	roomID, err := strconv.ParseInt(strings.TrimSpace(desired.RoomID), 10, 64)
	if err != nil || roomID <= 0 {
		return RuntimeStatus{}, fmt.Errorf("invalid recorder room id %q", desired.RoomID)
	}

	if !desired.Enabled {
		status, _, err := c.do(ctx, http.MethodDelete, fmt.Sprintf("/api/room/%d", roomID), nil)
		if err != nil {
			return RuntimeStatus{}, err
		}
		if status != http.StatusOK && status != http.StatusNoContent && status != http.StatusNotFound {
			return RuntimeStatus{}, fmt.Errorf("disable recorder room %d returned status %d", roomID, status)
		}
		return RuntimeStatus{StreamStatus: "UNKNOWN", RecorderStatus: "IDLE"}, nil
	}

	room, exists, err := c.room(ctx, roomID)
	if err != nil {
		return RuntimeStatus{}, err
	}
	if !exists {
		if err := c.addRoom(ctx, roomID, desired.AutoRecord); err != nil {
			return RuntimeStatus{}, err
		}
		room, exists, err = c.room(ctx, roomID)
		if err != nil {
			return RuntimeStatus{}, err
		}
		if !exists {
			return RuntimeStatus{}, fmt.Errorf("recorder room %d was not available after add", roomID)
		}
	}
	if err := c.setRoomConfig(ctx, roomID, desired); err != nil {
		return RuntimeStatus{}, err
	}
	return runtimeStatus(room), nil
}

func (c HTTPClient) room(ctx context.Context, roomID int64) (roomResponse, bool, error) {
	status, body, err := c.do(ctx, http.MethodGet, fmt.Sprintf("/api/room/%d", roomID), nil)
	if err != nil {
		return roomResponse{}, false, err
	}
	if status == http.StatusNotFound {
		return roomResponse{}, false, nil
	}
	if status != http.StatusOK {
		return roomResponse{}, false, fmt.Errorf("load recorder room %d returned status %d", roomID, status)
	}
	var room roomResponse
	if len(body) > 0 {
		if err := json.Unmarshal(body, &room); err != nil {
			return roomResponse{}, false, fmt.Errorf("decode recorder room %d: %w", roomID, err)
		}
	}
	return room, true, nil
}

func (c HTTPClient) addRoom(ctx context.Context, roomID int64, autoRecord bool) error {
	payload := map[string]interface{}{
		"roomId":     roomID,
		"autoRecord": autoRecord,
	}
	status, _, err := c.do(ctx, http.MethodPost, "/api/room", payload)
	if err != nil {
		return err
	}
	if status != http.StatusOK && status != http.StatusCreated && status != http.StatusNoContent {
		return fmt.Errorf("add recorder room %d returned status %d", roomID, status)
	}
	return nil
}

func (c HTTPClient) setRoomConfig(ctx context.Context, roomID int64, desired DesiredProfile) error {
	payload := map[string]interface{}{
		"RoomId":           roomID,
		"AutoRecord":       desired.AutoRecord,
		"RecordMode":       0,
		"RecordDanmaku":    desired.RecordDanmaku,
		"CuttingMode":      1,
		"CuttingNumber":    segmentMinutes(desired.SegmentDurationSec),
		"RecordingQuality": qualityQN(desired.Quality),
	}
	path := fmt.Sprintf("/api/room/%d/config", roomID)
	var status int
	var responseBody []byte
	var err error
	var attempted []string
	for _, method := range []string{http.MethodPut, http.MethodPatch, http.MethodPost} {
		attempted = append(attempted, method)
		status, responseBody, err = c.do(ctx, method, path, payload)
		if err != nil {
			return err
		}
		if status == http.StatusOK || status == http.StatusNoContent {
			return nil
		}
		if status != http.StatusMethodNotAllowed {
			return fmt.Errorf("set recorder room %d config via %s returned status %d: %s", roomID, method, status, trimBody(responseBody))
		}
	}
	return fmt.Errorf("set recorder room %d config returned status %d after methods %s: %s", roomID, status, strings.Join(attempted, ","), trimBody(responseBody))
}

func (c HTTPClient) do(ctx context.Context, method string, path string, payload interface{}) (int, []byte, error) {
	target, err := url.JoinPath(c.baseURL, strings.TrimPrefix(path, "/"))
	if err != nil {
		return 0, nil, fmt.Errorf("build recorder request url: %w", err)
	}
	var body io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return 0, nil, fmt.Errorf("marshal recorder request: %w", err)
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return 0, nil, fmt.Errorf("build recorder request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.username != "" || c.password != "" {
		req.SetBasicAuth(c.username, c.password)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("call recorder %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return 0, nil, fmt.Errorf("read recorder response: %w", err)
	}
	return resp.StatusCode, data, nil
}

type roomResponse struct {
	Streaming bool `json:"streaming"`
	Recording bool `json:"recording"`
}

func runtimeStatus(room roomResponse) RuntimeStatus {
	status := RuntimeStatus{
		StreamStatus:   "OFFLINE",
		RecorderStatus: "IDLE",
	}
	if room.Streaming {
		status.StreamStatus = "LIVE"
	}
	if room.Recording {
		status.RecorderStatus = "RECORDING"
	}
	return status
}

func segmentMinutes(seconds int64) int64 {
	if seconds < 60 {
		return 1
	}
	return (seconds + 59) / 60
}

func qualityQN(quality string) string {
	switch strings.ToLower(strings.TrimSpace(quality)) {
	case "4k":
		return "20000"
	case "blue_ray_dolby", "dolby":
		return "401"
	case "blue_ray", "high":
		return "400"
	case "super":
		return "250"
	case "hd":
		return "150"
	case "smooth":
		return "80"
	default:
		return "10000"
	}
}

func trimBody(body []byte) string {
	message := strings.TrimSpace(string(body))
	if message == "" {
		return "<empty body>"
	}
	if len(message) > 500 {
		return message[:500]
	}
	return message
}
