package liveanalytics

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const DefaultAPIBaseURL = "https://live-open.biliapi.com"

type APIClient struct {
	BaseURL    string
	HTTPClient *http.Client
	AccessKey  string
	SecretKey  string
	Now        func() time.Time
	Nonce      func() string
}

type StartResult struct {
	GameID    string
	AuthBody  string
	WSSLinks  []string
	RoomID    int64
	AnchorUID int64
	OpenID    string
	UnionID   string
	Name      string
	FaceURL   string
}

type apiEnvelope struct {
	Code      int             `json:"code"`
	Message   string          `json:"message"`
	RequestID string          `json:"request_id"`
	Data      json.RawMessage `json:"data"`
}

func (c APIClient) Start(ctx context.Context, appID int64, identityCode string) (StartResult, error) {
	var response struct {
		GameInfo struct {
			GameID string `json:"game_id"`
		} `json:"game_info"`
		WebSocketInfo struct {
			AuthBody string   `json:"auth_body"`
			WSSLink  []string `json:"wss_link"`
		} `json:"websocket_info"`
		AnchorInfo struct {
			RoomID  int64  `json:"room_id"`
			UID     int64  `json:"uid"`
			OpenID  string `json:"open_id"`
			UnionID string `json:"union_id"`
			Name    string `json:"uname"`
			FaceURL string `json:"uface"`
		} `json:"anchor_info"`
	}
	if err := c.post(ctx, "/v2/app/start", map[string]any{"code": identityCode, "app_id": appID}, &response); err != nil {
		return StartResult{}, err
	}
	if response.GameInfo.GameID == "" || response.WebSocketInfo.AuthBody == "" || len(response.WebSocketInfo.WSSLink) == 0 {
		return StartResult{}, fmt.Errorf("OpenLive start response is incomplete")
	}
	return StartResult{
		GameID: response.GameInfo.GameID, AuthBody: response.WebSocketInfo.AuthBody,
		WSSLinks: response.WebSocketInfo.WSSLink, RoomID: response.AnchorInfo.RoomID,
		AnchorUID: response.AnchorInfo.UID, OpenID: response.AnchorInfo.OpenID,
		UnionID: response.AnchorInfo.UnionID, Name: response.AnchorInfo.Name, FaceURL: response.AnchorInfo.FaceURL,
	}, nil
}

func (c APIClient) Heartbeat(ctx context.Context, gameID string) error {
	return c.post(ctx, "/v2/app/heartbeat", map[string]any{"game_id": gameID}, nil)
}

func (c APIClient) End(ctx context.Context, appID int64, gameID string) error {
	return c.post(ctx, "/v2/app/end", map[string]any{"app_id": appID, "game_id": gameID}, nil)
}

func (c APIClient) post(ctx context.Context, path string, payload any, output any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode OpenLive request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.baseURL(), "/")+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create OpenLive request: %w", err)
	}
	for key, value := range c.signedHeaders(body) {
		req.Header.Set(key, value)
	}
	response, err := c.httpClient().Do(req)
	if err != nil {
		return fmt.Errorf("call OpenLive %s: %w", path, err)
	}
	defer response.Body.Close()
	limited := io.LimitReader(response.Body, 2<<20)
	responseBody, err := io.ReadAll(limited)
	if err != nil {
		return fmt.Errorf("read OpenLive %s response: %w", path, err)
	}
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("OpenLive %s returned HTTP %d", path, response.StatusCode)
	}
	var envelope apiEnvelope
	if err := json.Unmarshal(responseBody, &envelope); err != nil {
		return fmt.Errorf("decode OpenLive %s response: %w", path, err)
	}
	if envelope.Code != 0 {
		return fmt.Errorf("OpenLive %s rejected request: code=%d message=%s request_id=%s", path, envelope.Code, envelope.Message, envelope.RequestID)
	}
	if output != nil && len(envelope.Data) > 0 {
		if err := json.Unmarshal(envelope.Data, output); err != nil {
			return fmt.Errorf("decode OpenLive %s data: %w", path, err)
		}
	}
	return nil
}

func (c APIClient) signedHeaders(body []byte) map[string]string {
	digest := md5.Sum(body)
	now := time.Now
	if c.Now != nil {
		now = c.Now
	}
	nonce := randomNonce
	if c.Nonce != nil {
		nonce = c.Nonce
	}
	headers := map[string]string{
		"Accept":                   "application/json",
		"Content-Type":             "application/json",
		"x-bili-accesskeyid":       c.AccessKey,
		"x-bili-content-md5":       hex.EncodeToString(digest[:]),
		"x-bili-signature-method":  "HMAC-SHA256",
		"x-bili-signature-nonce":   nonce(),
		"x-bili-signature-version": "1.0",
		"x-bili-timestamp":         strconv.FormatInt(now().Unix(), 10),
	}
	canonical := strings.Join([]string{
		"x-bili-accesskeyid:" + headers["x-bili-accesskeyid"],
		"x-bili-content-md5:" + headers["x-bili-content-md5"],
		"x-bili-signature-method:" + headers["x-bili-signature-method"],
		"x-bili-signature-nonce:" + headers["x-bili-signature-nonce"],
		"x-bili-signature-version:" + headers["x-bili-signature-version"],
		"x-bili-timestamp:" + headers["x-bili-timestamp"],
	}, "\n")
	mac := hmac.New(sha256.New, []byte(c.SecretKey))
	_, _ = mac.Write([]byte(canonical))
	headers["Authorization"] = hex.EncodeToString(mac.Sum(nil))
	return headers
}

func (c APIClient) baseURL() string {
	if strings.TrimSpace(c.BaseURL) == "" {
		return DefaultAPIBaseURL
	}
	return c.BaseURL
}

func (c APIClient) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return &http.Client{Timeout: 15 * time.Second}
}

func randomNonce() string {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 10)
	}
	return hex.EncodeToString(value)
}
