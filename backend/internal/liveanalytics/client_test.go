package liveanalytics

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestAPIClientStartSignsAndMapsResponse(t *testing.T) {
	fixedTime := time.Unix(1624594467, 0)
	httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/v2/app/start" || r.Method != http.MethodPost {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		if string(body) != `{"app_id":1793018783146,"code":"room-code"}` && string(body) != `{"code":"room-code","app_id":1793018783146}` {
			t.Fatalf("unexpected request body %s", body)
		}
		canonical := strings.Join([]string{
			"x-bili-accesskeyid:test-key", "x-bili-content-md5:" + r.Header.Get("x-bili-content-md5"),
			"x-bili-signature-method:HMAC-SHA256", "x-bili-signature-nonce:fixed-nonce",
			"x-bili-signature-version:1.0", "x-bili-timestamp:1624594467",
		}, "\n")
		mac := hmac.New(sha256.New, []byte("test-secret"))
		_, _ = mac.Write([]byte(canonical))
		if got, want := r.Header.Get("Authorization"), hex.EncodeToString(mac.Sum(nil)); got != want {
			t.Fatalf("signature=%q want=%q", got, want)
		}
		return jsonResponse(`{"code":0,"message":"ok","data":{"game_info":{"game_id":"g1"},"websocket_info":{"auth_body":"auth","wss_link":["wss://example/sub"]},"anchor_info":{"room_id":1741048619,"open_id":"anchor","uname":"Streamer"}}}`), nil
	})}
	client := APIClient{BaseURL: "https://openlive.invalid", HTTPClient: httpClient, AccessKey: "test-key", SecretKey: "test-secret", Now: func() time.Time { return fixedTime }, Nonce: func() string { return "fixed-nonce" }}
	result, err := client.Start(t.Context(), 1793018783146, "room-code")
	if err != nil {
		t.Fatal(err)
	}
	if result.GameID != "g1" || result.RoomID != 1741048619 || result.OpenID != "anchor" || len(result.WSSLinks) != 1 {
		t.Fatalf("unexpected result %#v", result)
	}
}

func TestAPIClientTreatsBusinessCodeAsFailure(t *testing.T) {
	payload, _ := json.Marshal(map[string]any{"code": 8002, "message": "denied", "request_id": "safe-id", "data": map[string]any{}})
	client := APIClient{BaseURL: "https://openlive.invalid", HTTPClient: &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return jsonResponse(string(payload)), nil
	})}, AccessKey: "key", SecretKey: "secret"}
	if _, err := client.Start(t.Context(), 1, "code"); err == nil || !strings.Contains(err.Error(), "8002") {
		t.Fatalf("expected business error, got %v", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return fn(request) }
func jsonResponse(body string) *http.Response {
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}
}
