package liveanalytics

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/7grecorder/7grecorder/backend/internal/account"
)

var ErrEvidenceUnavailable = errors.New("raw evidence unavailable")

const maxEventLine = 1 << 20

type EventPage struct {
	Items      []rawRecord `json:"items"`
	NextOffset int64       `json:"next_offset"`
	HasMore    bool        `json:"has_more"`
}

// Events reads one bounded page. Cursors are byte positions, never filesystem paths.
func (s Store) Events(ctx context.Context, actor account.User, id, offset int64, limit int) (EventPage, error) {
	session, err := s.GetSession(ctx, actor, id)
	if err != nil {
		return EventPage{}, err
	}
	if offset < 0 || limit < 1 || limit > 100 {
		return EventPage{}, ErrValidation
	}
	if session.RawStatus != "WRITING" && session.RawStatus != "AVAILABLE" {
		return EventPage{}, ErrEvidenceUnavailable
	}
	var relative string
	if err := s.db.QueryRowContext(ctx, "SELECT raw_relative_path FROM live_capture_sessions WHERE id = ?", id).Scan(&relative); err != nil {
		return EventPage{}, err
	}
	if _, err := s.resolveRawPath(relative); err != nil {
		return EventPage{}, err
	}
	root, err := os.OpenRoot(s.cfg.DataRoot)
	if err != nil {
		return EventPage{}, err
	}
	defer root.Close()
	file, err := root.Open(filepath.FromSlash(relative))
	if errors.Is(err, os.ErrNotExist) {
		return EventPage{}, ErrEvidenceUnavailable
	}
	if err != nil {
		return EventPage{}, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return EventPage{}, err
	}
	if !info.Mode().IsRegular() || offset > info.Size() {
		return EventPage{}, ErrValidation
	}
	if offset > 0 {
		var previous [1]byte
		if _, err := file.ReadAt(previous[:], offset-1); err != nil || previous[0] != '\n' {
			return EventPage{}, ErrValidation
		}
	}
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return EventPage{}, err
	}
	page := EventPage{Items: []rawRecord{}, NextOffset: offset}
	reader := bufio.NewReaderSize(io.LimitReader(file, info.Size()-offset), maxEventLine)
	for len(page.Items) < limit {
		if err := ctx.Err(); err != nil {
			return EventPage{}, err
		}
		line, err := reader.ReadSlice('\n')
		if errors.Is(err, io.EOF) {
			if len(line) > 0 && session.RawStatus != "WRITING" {
				return EventPage{}, fmt.Errorf("incomplete closed evidence record")
			}
			break
		} // A live writer may not have completed this last line yet.
		if err != nil {
			return EventPage{}, fmt.Errorf("read bounded evidence record: %w", err)
		}
		var record rawRecord
		if json.Unmarshal(line, &record) != nil || record.CMD == "" || !json.Valid(record.Payload) {
			return EventPage{}, fmt.Errorf("invalid evidence record")
		}
		page.Items = append(page.Items, record)
		page.NextOffset += int64(len(line))
		if page.NextOffset-offset >= 4<<20 {
			break
		}
	}
	page.HasMore = page.NextOffset < info.Size()
	return page, nil
}
