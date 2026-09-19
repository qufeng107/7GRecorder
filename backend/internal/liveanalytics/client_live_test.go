package liveanalytics

import (
	"context"
	"encoding/json"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// This opt-in smoke test uses only credentials explicitly provided in the environment.
// CI skips it and no secret, auth body, identity code, or event payload is logged.
func TestAuthorizedOpenLiveSession(t *testing.T) {
	accessKey := os.Getenv("BILIBILI_ACCESS_KEY_ID")
	secretKey := os.Getenv("BILIBILI_ACCESS_KEY_SECRET")
	identityCode := os.Getenv("BILIBILI_TEST_ROOM_CODE")
	appIDText := os.Getenv("BILIBILI_OPEN_LIVE_APP_ID")
	if accessKey == "" || secretKey == "" || identityCode == "" || appIDText == "" {
		t.Skip("authorized OpenLive environment is not configured")
	}
	appID, err := strconv.ParseInt(appIDText, 10, 64)
	if err != nil || appID <= 0 {
		t.Fatal("BILIBILI_OPEN_LIVE_APP_ID is invalid")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 25*time.Second)
	defer cancel()
	client := APIClient{AccessKey: accessKey, SecretKey: secretKey}
	start, err := client.Start(ctx, appID, identityCode)
	if err != nil {
		t.Fatalf("OpenLive start failed: %v", err)
	}
	defer func() {
		endCtx, endCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer endCancel()
		if err := client.End(endCtx, appID, start.GameID); err != nil {
			t.Errorf("OpenLive end failed: %v", err)
		}
	}()
	if start.RoomID <= 0 {
		t.Fatal("OpenLive start returned no room")
	}
	conn, err := dialOpenLive(ctx, start.WSSLinks)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := conn.WriteMessage(websocket.BinaryMessage, EncodePacket(OpAuth, []byte(start.AuthBody))); err != nil {
		t.Fatal(err)
	}
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetReadDeadline(deadline)
	}
	for {
		_, frame, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("OpenLive auth read failed: %v", err)
		}
		packets, err := DecodePackets(frame)
		if err != nil {
			t.Fatal(err)
		}
		for _, packet := range packets {
			if packet.Operation != OpAuthReply {
				continue
			}
			var reply struct {
				Code int `json:"code"`
			}
			if json.Unmarshal(packet.Body, &reply) != nil || reply.Code != 0 {
				t.Fatal("OpenLive WebSocket authentication was rejected")
			}
			return
		}
	}
}
