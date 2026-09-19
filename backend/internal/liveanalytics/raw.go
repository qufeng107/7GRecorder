package liveanalytics

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type RawWriter struct {
	file     *os.File
	buffered *bufio.Writer
	relative string
}

type rawRecord struct {
	ReceivedAt string          `json:"received_at"`
	CMD        string          `json:"cmd"`
	Payload    json.RawMessage `json:"payload"`
}

func NewRawWriter(dataRoot string, profileID, sessionID int64, now time.Time) (*RawWriter, error) {
	relative := filepath.ToSlash(filepath.Join("live-analytics", fmt.Sprintf("profile-%d", profileID), now.UTC().Format("2006-01-02"), fmt.Sprintf("session-%d.jsonl", sessionID)))
	absolute := filepath.Join(dataRoot, filepath.FromSlash(relative))
	root, err := filepath.Abs(filepath.Join(dataRoot, "live-analytics"))
	if err != nil {
		return nil, err
	}
	resolved, err := filepath.Abs(absolute)
	if err != nil {
		return nil, err
	}
	relToRoot, err := filepath.Rel(root, resolved)
	if err != nil || relToRoot == ".." || (len(relToRoot) > 3 && relToRoot[:3] == ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("live analytics raw path escapes data root")
	}
	if err := os.MkdirAll(filepath.Dir(resolved), 0o750); err != nil {
		return nil, fmt.Errorf("create live analytics directory: %w", err)
	}
	file, err := os.OpenFile(resolved, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("create live analytics raw file: %w", err)
	}
	return &RawWriter{file: file, buffered: bufio.NewWriterSize(file, 64*1024), relative: relative}, nil
}

func (w *RawWriter) RelativePath() string { return w.relative }

func (w *RawWriter) Write(receivedAt time.Time, cmd string, payload json.RawMessage) error {
	line, err := json.Marshal(rawRecord{ReceivedAt: receivedAt.UTC().Format(time.RFC3339Nano), CMD: cmd, Payload: payload})
	if err != nil {
		return err
	}
	if _, err := w.buffered.Write(append(line, '\n')); err != nil {
		return err
	}
	return w.buffered.Flush()
}

func (w *RawWriter) Sync() error { return w.file.Sync() }

func (w *RawWriter) Close() error {
	flushErr := w.buffered.Flush()
	syncErr := w.file.Sync()
	closeErr := w.file.Close()
	if flushErr != nil {
		return flushErr
	}
	if syncErr != nil {
		return syncErr
	}
	return closeErr
}

var knownCommands = map[string]struct{}{
	"LIVE_OPEN_PLATFORM_DM": {}, "LIVE_OPEN_PLATFORM_DM_MIRROR": {},
	"LIVE_OPEN_PLATFORM_SEND_GIFT": {}, "LIVE_OPEN_PLATFORM_SUPER_CHAT": {},
	"LIVE_OPEN_PLATFORM_SUPER_CHAT_DEL": {}, "LIVE_OPEN_PLATFORM_GUARD": {},
	"LIVE_OPEN_PLATFORM_LIKE": {}, "LIVE_OPEN_PLATFORM_LIVE_ROOM_ENTER": {},
	"LIVE_OPEN_PLATFORM_LIVE_START": {}, "LIVE_OPEN_PLATFORM_LIVE_END": {},
	"LIVE_OPEN_PLATFORM_INTERACTION_END": {},
}

func isKnownCommand(cmd string) bool { _, ok := knownCommands[cmd]; return ok }
