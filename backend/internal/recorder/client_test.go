package recorder

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/7grecorder/7grecorder/backend/internal/config"
)

func TestHTTPClientSyncProfileAddsRoomAndConfiguresIt(t *testing.T) {
	ctx := context.Background()
	var calls []string
	var configPayload map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		switch r.Method + " " + r.URL.Path {
		case "GET /api/room/1741048619":
			if len(calls) == 1 {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"roomId":1741048619,"streaming":true,"recording":false}`))
		case "POST /api/room":
			var payload map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode add room payload: %v", err)
			}
			if payload["roomId"] != float64(1741048619) {
				t.Fatalf("expected roomId 1741048619, got %#v", payload["roomId"])
			}
			w.WriteHeader(http.StatusCreated)
		case "POST /api/room/1741048619/config":
			if err := json.NewDecoder(r.Body).Decode(&configPayload); err != nil {
				t.Fatalf("decode config payload: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"AutoRecord":true,
				"OptionalRecordMode":{"HasValue":true,"Value":0},
				"OptionalRecordDanmaku":{"HasValue":true,"Value":true},
				"OptionalCuttingMode":{"HasValue":true,"Value":1},
				"OptionalCuttingNumber":{"HasValue":true,"Value":30},
				"OptionalRecordingQuality":{"HasValue":true,"Value":"avc20000,hevc20000,avc10000,hevc10000"}
			}`))
		default:
			t.Fatalf("unexpected recorder request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	client := NewHTTPClient(config.Config{RecorderBaseURL: server.URL})
	status, err := client.SyncProfile(ctx, DesiredProfile{
		RoomID:             "1741048619",
		Enabled:            true,
		AutoRecord:         true,
		Quality:            "4k",
		RecordDanmaku:      true,
		SegmentDurationSec: 1800,
	})
	if err != nil {
		t.Fatalf("SyncProfile returned error: %v", err)
	}
	if status.StreamStatus != "LIVE" || status.RecorderStatus != "IDLE" {
		t.Fatalf("unexpected runtime status: %#v", status)
	}
	wantCalls := []string{
		"GET /api/room/1741048619",
		"POST /api/room",
		"GET /api/room/1741048619",
		"POST /api/room/1741048619/config",
	}
	if len(calls) != len(wantCalls) {
		t.Fatalf("expected calls %v, got %v", wantCalls, calls)
	}
	for i := range wantCalls {
		if calls[i] != wantCalls[i] {
			t.Fatalf("expected calls %v, got %v", wantCalls, calls)
		}
	}
	if configPayload["AutoRecord"] != true {
		t.Fatalf("expected AutoRecord true, got %#v", configPayload["AutoRecord"])
	}
	cuttingNumber := configPayload["OptionalCuttingNumber"].(map[string]interface{})
	if cuttingNumber["HasValue"] != true || cuttingNumber["Value"] != float64(30) {
		t.Fatalf("expected explicit 30 minute segments, got %#v", cuttingNumber)
	}
	danmaku := configPayload["OptionalRecordDanmaku"].(map[string]interface{})
	if danmaku["HasValue"] != true || danmaku["Value"] != true {
		t.Fatalf("expected explicit danmaku recording, got %#v", danmaku)
	}
	quality := configPayload["OptionalRecordingQuality"].(map[string]interface{})
	if quality["HasValue"] != true || quality["Value"] != "avc20000,hevc20000,avc10000,hevc10000" {
		t.Fatalf("unexpected recording quality: %#v", quality)
	}
}

func TestHTTPClientSyncProfileRejectsSuccessfulConfigResponseWithDrift(t *testing.T) {
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /api/room/1741048619":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"roomId":1741048619,"streaming":false,"recording":false}`))
		case "POST /api/room/1741048619/config":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"AutoRecord":true,
				"OptionalRecordMode":{"HasValue":true,"Value":0},
				"OptionalRecordDanmaku":{"HasValue":false,"Value":false},
				"OptionalCuttingMode":{"HasValue":true,"Value":1},
				"OptionalCuttingNumber":{"HasValue":true,"Value":30},
				"OptionalRecordingQuality":{"HasValue":true,"Value":"avc10000,hevc10000"}
			}`))
		default:
			t.Fatalf("unexpected recorder request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	client := NewHTTPClient(config.Config{RecorderBaseURL: server.URL})
	_, err := client.SyncProfile(ctx, DesiredProfile{
		RoomID:             "1741048619",
		Enabled:            true,
		AutoRecord:         true,
		RecordDanmaku:      true,
		SegmentDurationSec: 1800,
	})
	if err == nil || !strings.Contains(err.Error(), "OptionalRecordDanmaku did not match desired value") {
		t.Fatalf("expected verified config drift error, got %v", err)
	}
}

func TestHTTPClientSyncProfileIncludesConfigErrorDetails(t *testing.T) {
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /api/room/1741048619":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"roomId":1741048619,"streaming":false,"recording":false}`))
		case "POST /api/room/1741048619/config":
			http.Error(w, "bad config field", http.StatusBadRequest)
		default:
			t.Fatalf("unexpected recorder request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	client := NewHTTPClient(config.Config{RecorderBaseURL: server.URL})
	_, err := client.SyncProfile(ctx, DesiredProfile{
		RoomID:             "1741048619",
		Enabled:            true,
		AutoRecord:         true,
		RecordDanmaku:      true,
		SegmentDurationSec: 1800,
	})
	if err == nil {
		t.Fatal("expected SyncProfile error")
	}
	message := err.Error()
	if !strings.Contains(message, "returned status 400") || !strings.Contains(message, "bad config field") {
		t.Fatalf("expected method and body in error, got %q", message)
	}
}
