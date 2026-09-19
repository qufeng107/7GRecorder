package liveanalytics

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const rawChunkBytes int64 = 32 << 20

type RawWriter struct {
	mu                   sync.Mutex
	bytes                int64
	dataRoot             string
	profileID, sessionID int64
	part                 int
	file                 *os.File
	buffered             *bufio.Writer
	relative             string
}

type rawRecord struct {
	ReceivedAt string          `json:"received_at"`
	CMD        string          `json:"cmd"`
	Payload    json.RawMessage `json:"payload"`
}

func NewRawWriter(dataRoot string, profileID, sessionID int64, now time.Time) (*RawWriter, error) {
	return newRawPart(dataRoot, profileID, sessionID, now, 0)
}

func newRawPart(dataRoot string, profileID, sessionID int64, now time.Time, part int) (*RawWriter, error) {
	relative := filepath.ToSlash(filepath.Join("live-analytics", fmt.Sprintf("profile-%d", profileID), now.UTC().Format("2006-01-02"), fmt.Sprintf("session-%d-part-%06d.jsonl", sessionID, part)))
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
	if err := rejectRawSymlinks(dataRoot, relative); err != nil {
		return nil, err
	}
	safeRoot, err := os.OpenRoot(dataRoot)
	if err != nil {
		return nil, err
	}
	defer safeRoot.Close()
	directory := ""
	for _, part := range strings.Split(filepath.Dir(filepath.FromSlash(relative)), string(filepath.Separator)) {
		directory = filepath.Join(directory, part)
		if err := safeRoot.Mkdir(directory, 0o750); err != nil && !errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("create live analytics directory: %w", err)
		}
	}
	file, err := safeRoot.OpenFile(filepath.FromSlash(relative), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("create live analytics raw file: %w", err)
	}
	return &RawWriter{file: file, buffered: bufio.NewWriterSize(file, 64*1024), relative: relative, dataRoot: dataRoot, profileID: profileID, sessionID: sessionID, part: part}, nil
}

func (w *RawWriter) RelativePath() string { w.mu.Lock(); defer w.mu.Unlock(); return w.relative }
func (w *RawWriter) NeedsRotation() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.bytes >= rawChunkBytes
}

func (w *RawWriter) Rotate(now time.Time) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.close(); err != nil {
		return err
	}
	next, err := newRawPart(w.dataRoot, w.profileID, w.sessionID, now, w.part+1)
	if err != nil {
		return err
	}
	w.file, w.buffered, w.relative, w.part, w.bytes = next.file, next.buffered, next.relative, next.part, 0
	return nil
}

func (w *RawWriter) Write(receivedAt time.Time, cmd string, payload json.RawMessage) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	line, err := json.Marshal(rawRecord{ReceivedAt: receivedAt.UTC().Format(time.RFC3339Nano), CMD: cmd, Payload: payload})
	if err != nil {
		return err
	}
	if _, err := w.buffered.Write(append(line, '\n')); err != nil {
		return err
	}
	w.bytes += int64(len(line) + 1)
	return w.buffered.Flush()
}

func (w *RawWriter) Sync() error { w.mu.Lock(); defer w.mu.Unlock(); return w.file.Sync() }

func (w *RawWriter) Close() error { w.mu.Lock(); defer w.mu.Unlock(); return w.close() }

func (w *RawWriter) close() error {
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
