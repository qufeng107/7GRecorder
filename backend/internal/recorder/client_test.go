package recorder

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/7grecorder/7grecorder/backend/internal/config"
)

func TestHTTPClientSyncProfileAddsRoomAndConfiguresIt(t *testing.T) {
	ctx := context.Background()
	var calls []string
	var configPayload map[string]map[string]interface{}
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
		case "PUT /api/room/1741048619/config":
			if err := json.NewDecoder(r.Body).Decode(&configPayload); err != nil {
				t.Fatalf("decode config payload: %v", err)
			}
			w.WriteHeader(http.StatusNoContent)
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
		"PUT /api/room/1741048619/config",
	}
	if len(calls) != len(wantCalls) {
		t.Fatalf("expected calls %v, got %v", wantCalls, calls)
	}
	for i := range wantCalls {
		if calls[i] != wantCalls[i] {
			t.Fatalf("expected calls %v, got %v", wantCalls, calls)
		}
	}
	if configPayload["AutoRecord"]["Value"] != true {
		t.Fatalf("expected AutoRecord true, got %#v", configPayload["AutoRecord"]["Value"])
	}
	if configPayload["CuttingNumber"]["Value"] != float64(30) {
		t.Fatalf("expected 30 minute segments, got %#v", configPayload["CuttingNumber"]["Value"])
	}
}

func TestHTTPClientSyncProfileFallsBackWhenPutConfigIsNotAllowed(t *testing.T) {
	ctx := context.Background()
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		switch r.Method + " " + r.URL.Path {
		case "GET /api/room/1741048619":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"roomId":1741048619,"streaming":false,"recording":false}`))
		case "PUT /api/room/1741048619/config":
			w.WriteHeader(http.StatusMethodNotAllowed)
		case "PATCH /api/room/1741048619/config":
			w.WriteHeader(http.StatusNoContent)
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
	if err != nil {
		t.Fatalf("SyncProfile returned error: %v", err)
	}
	wantCalls := []string{
		"GET /api/room/1741048619",
		"PUT /api/room/1741048619/config",
		"PATCH /api/room/1741048619/config",
	}
	if len(calls) != len(wantCalls) {
		t.Fatalf("expected calls %v, got %v", wantCalls, calls)
	}
	for i := range wantCalls {
		if calls[i] != wantCalls[i] {
			t.Fatalf("expected calls %v, got %v", wantCalls, calls)
		}
	}
}
