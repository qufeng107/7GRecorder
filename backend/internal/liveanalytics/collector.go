package liveanalytics

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/7grecorder/7grecorder/backend/internal/config"
	"github.com/gorilla/websocket"
)

const (
	reconcileInterval = 5 * time.Second
	quotaInterval     = time.Minute
	heartbeatInterval = 20 * time.Second
	reconnectDelay    = 5 * time.Second
)

type CollectorManager struct {
	store Store
	cfg   config.Config
	mu    sync.Mutex
	runs  map[int64]managedCollector
	wg    sync.WaitGroup
}

type managedCollector struct {
	key    string
	cancel context.CancelFunc
	done   <-chan struct{}
}

func NewCollectorManager(database *sql.DB, cfg config.Config) *CollectorManager {
	return &CollectorManager{store: NewStore(database, cfg), cfg: cfg, runs: map[int64]managedCollector{}}
}

func (m *CollectorManager) Run(ctx context.Context) {
	if err := m.store.InterruptActive(ctx); err != nil {
		return
	}
	m.reconcile(ctx)
	if _, err := m.store.EnforceRawQuota(ctx); err != nil {
		log.Printf("live analytics quota maintenance failed: %v", err)
	}
	ticker := time.NewTicker(reconcileInterval)
	defer ticker.Stop()
	quotaTicker := time.NewTicker(quotaInterval)
	defer quotaTicker.Stop()
	for {
		select {
		case <-ctx.Done():
			m.stopAll()
			return
		case <-ticker.C:
			m.reconcile(ctx)
		case <-quotaTicker.C:
			if _, err := m.store.EnforceRawQuota(ctx); err != nil {
				log.Printf("live analytics quota maintenance failed: %v", err)
			}
		}
	}
}

func (m *CollectorManager) reconcile(ctx context.Context) {
	requests, err := m.store.EnabledRequests(ctx)
	if err != nil {
		return
	}
	wanted := make(map[int64]CaptureRequest, len(requests))
	for _, req := range requests {
		wanted[req.RecordingProfileID] = req
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for profileID, running := range m.runs {
		select {
		case <-running.done:
			delete(m.runs, profileID)
			continue
		default:
		}
		req, ok := wanted[profileID]
		if !ok || collectorKey(req) != running.key {
			running.cancel()
		}
	}
	for profileID, req := range wanted {
		if _, ok := m.runs[profileID]; ok {
			continue
		}
		runCtx, cancel := context.WithCancel(ctx)
		done := make(chan struct{})
		m.runs[profileID] = managedCollector{key: collectorKey(req), cancel: cancel, done: done}
		m.wg.Add(1)
		go func() {
			defer m.wg.Done()
			defer close(done)
			m.runProfile(runCtx, req)
		}()
	}
}

func (m *CollectorManager) stopAll() {
	m.mu.Lock()
	for id, running := range m.runs {
		running.cancel()
		delete(m.runs, id)
	}
	m.mu.Unlock()
	m.wg.Wait()
}

func collectorKey(req CaptureRequest) string {
	return fmt.Sprintf("%d:%d:%s", req.CredentialID, req.AppID, req.ExpectedRoomID)
}

func (m *CollectorManager) runProfile(ctx context.Context, req CaptureRequest) {
	for {
		err := m.captureOnce(ctx, req)
		if ctx.Err() != nil {
			return
		}
		m.store.RecordConfigError(context.WithoutCancel(ctx), req.RecordingProfileID, safeCollectorError(err))
		timer := time.NewTimer(reconnectDelay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func (m *CollectorManager) captureOnce(ctx context.Context, req CaptureRequest) (captureErr error) {
	client := APIClient{AccessKey: req.AccessKeyID, SecretKey: req.AccessKeySecret}
	start, err := client.Start(ctx, req.AppID, req.IdentityCode)
	if err != nil {
		return err
	}
	ended := false
	endProject := func() {
		if ended {
			return
		}
		ended = true
		endCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = client.End(endCtx, req.AppID, start.GameID)
	}
	defer endProject()
	if strconv.FormatInt(start.RoomID, 10) != req.ExpectedRoomID {
		return fmt.Errorf("OpenLive identity room does not match recording profile")
	}
	sessionID, err := m.store.CreateSession(ctx, req, start)
	if err != nil {
		return err
	}
	counts := map[string]int{}
	var total, unknown, gaps int64
	var lastEvent time.Time
	var lastStatsWrite time.Time
	minutes := map[string]map[string]int64{}
	flush := func(writeCtx context.Context) error {
		if err := m.store.flushStats(writeCtx, sessionID, counts, total, unknown, gaps, lastEvent, minutes); err != nil {
			return err
		}
		clear(minutes)
		return nil
	}
	finalized := false
	defer func() {
		if finalized {
			return
		}
		gaps++
		finalCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = flush(finalCtx)
		_ = m.store.FinishSession(finalCtx, sessionID, "FAILED", safeCollectorError(captureErr))
	}()
	raw, err := NewRawWriter(m.cfg.DataRoot, req.RecordingProfileID, sessionID, time.Now())
	if err != nil {
		return err
	}
	defer raw.Close()
	if err := m.store.SetRawPath(ctx, sessionID, raw.RelativePath()); err != nil {
		return err
	}
	conn, err := dialOpenLive(ctx, start.WSSLinks)
	if err != nil {
		return err
	}
	defer conn.Close()
	conn.SetReadLimit(16 << 20)
	if err := conn.SetReadDeadline(time.Now().Add(3 * heartbeatInterval)); err != nil {
		return err
	}
	_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	if err := conn.WriteMessage(websocket.BinaryMessage, EncodePacket(OpAuth, []byte(start.AuthBody))); err != nil {
		_ = m.store.FinishSession(context.WithoutCancel(ctx), sessionID, "FAILED", "OpenLive WebSocket authentication write failed")
		return err
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	heartbeatErrors := make(chan error, 1)
	go m.heartbeatLoop(runCtx, client, conn, start.GameID, sessionID, raw, heartbeatErrors)
	go func() { <-runCtx.Done(); _ = conn.Close() }()

	authenticated := false
	for {
		select {
		case heartbeatErr := <-heartbeatErrors:
			gaps++
			_ = flush(context.WithoutCancel(ctx))
			_ = m.store.FinishSession(context.WithoutCancel(ctx), sessionID, "FAILED", safeCollectorError(heartbeatErr))
			finalized = true
			return heartbeatErr
		default:
		}
		_, frame, readErr := conn.ReadMessage()
		if readErr != nil {
			status, message := "FAILED", "OpenLive WebSocket connection ended unexpectedly"
			if ctx.Err() != nil {
				status, message = "ENDED", ""
			} else {
				gaps++
			}
			_ = flush(context.WithoutCancel(ctx))
			_ = m.store.FinishSession(context.WithoutCancel(ctx), sessionID, status, message)
			finalized = true
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return readErr
		}
		if err := conn.SetReadDeadline(time.Now().Add(3 * heartbeatInterval)); err != nil {
			return err
		}
		packets, decodeErr := DecodePackets(frame)
		if decodeErr != nil {
			gaps++
			_ = flush(context.WithoutCancel(ctx))
			continue
		}
		for _, packet := range packets {
			switch packet.Operation {
			case OpAuthReply:
				var reply struct {
					Code int `json:"code"`
				}
				if json.Unmarshal(packet.Body, &reply) != nil || reply.Code != 0 {
					return fmt.Errorf("OpenLive WebSocket authentication rejected")
				}
				if !authenticated {
					authenticated = true
					_ = m.store.MarkConnected(ctx, sessionID)
				}
			case OpMessage:
				var envelope struct {
					CMD string `json:"cmd"`
				}
				if json.Unmarshal(packet.Body, &envelope) != nil || envelope.CMD == "" {
					gaps++
					continue
				}
				now := time.Now().UTC()
				if err := raw.Write(now, envelope.CMD, packet.Body); err != nil {
					return err
				}
				minute := now.Truncate(time.Minute).Format(time.RFC3339)
				if minutes[minute] == nil {
					minutes[minute] = map[string]int64{}
				}
				minutes[minute][envelope.CMD]++
				counts[envelope.CMD]++
				total++
				if !isKnownCommand(envelope.CMD) {
					unknown++
				}
				lastEvent = now
				if lastStatsWrite.IsZero() || total%25 == 0 || time.Since(lastStatsWrite) >= 2*time.Second {
					if err := flush(ctx); err != nil {
						return err
					}
					lastStatsWrite = now
				}
				if envelope.CMD == "LIVE_OPEN_PLATFORM_INTERACTION_END" {
					return fmt.Errorf("OpenLive interaction session ended by the platform")
				}
			}
		}
	}
}

func (m *CollectorManager) heartbeatLoop(ctx context.Context, client APIClient, conn *websocket.Conn, gameID string, sessionID int64, raw *RawWriter, errorsOut chan<- error) {
	defer conn.Close()
	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := conn.WriteMessage(websocket.BinaryMessage, EncodePacket(OpHeartbeat, nil)); err != nil {
				select {
				case errorsOut <- err:
				default:
				}
				return
			}
			heartbeatCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			err := client.Heartbeat(heartbeatCtx, gameID)
			cancel()
			if err != nil {
				_ = conn.Close()
				select {
				case errorsOut <- err:
				default:
				}
				return
			}
			_ = raw.Sync()
			_ = m.store.MarkHeartbeat(ctx, sessionID)
		}
	}
}

func dialOpenLive(ctx context.Context, links []string) (*websocket.Conn, error) {
	var lastErr error
	for _, link := range links {
		conn, response, err := websocket.DefaultDialer.DialContext(ctx, link, http.Header{})
		if response != nil && response.Body != nil {
			_ = response.Body.Close()
		}
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}
	return nil, fmt.Errorf("connect OpenLive WebSocket: %w", lastErr)
}

func safeCollectorError(err error) string {
	if err == nil {
		return "live analytics collector failed"
	}
	if errors.Is(err, context.Canceled) {
		return "collector stopped"
	}
	message := err.Error()
	if len(message) > 500 {
		message = message[:500]
	}
	return message
}
