package liveanalytics

import (
	"context"
	"github.com/7grecorder/7grecorder/backend/internal/account"
)

type MinuteCount struct {
	Minute     string `json:"minute"`
	CMD        string `json:"cmd"`
	EventCount int64  `json:"event_count"`
}
type Timeline struct {
	Items     []MinuteCount `json:"items"`
	Truncated bool          `json:"truncated"`
}

func (s Store) Timeline(ctx context.Context, actor account.User, id int64) (Timeline, error) {
	if _, err := s.GetSession(ctx, actor, id); err != nil {
		return Timeline{}, err
	}
	result := Timeline{Items: []MinuteCount{}}
	var count int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(DISTINCT minute) FROM live_capture_minutes WHERE session_id = ?", id).Scan(&count); err != nil {
		return result, err
	}
	result.Truncated = count > 1440
	rows, err := s.db.QueryContext(ctx, `SELECT minute, cmd, event_count FROM live_capture_minutes WHERE session_id = ?
 AND minute IN (SELECT DISTINCT minute FROM live_capture_minutes WHERE session_id = ? ORDER BY minute DESC LIMIT 1440)
 ORDER BY minute, cmd`, id, id)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		var item MinuteCount
		if err := rows.Scan(&item.Minute, &item.CMD, &item.EventCount); err != nil {
			return result, err
		}
		result.Items = append(result.Items, item)
	}
	return result, rows.Err()
}
