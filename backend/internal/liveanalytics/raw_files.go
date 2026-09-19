package liveanalytics

import (
	"context"
	"github.com/7grecorder/7grecorder/backend/internal/account"
)

type RawFile struct {
	ID        int64  `json:"id"`
	Status    string `json:"status"`
	SizeBytes int64  `json:"size_bytes"`
	CreatedAt string `json:"created_at"`
	ClosedAt  string `json:"closed_at,omitempty"`
	DeletedAt string `json:"deleted_at,omitempty"`
}
type RawFiles struct {
	Items     []RawFile `json:"items"`
	Truncated bool      `json:"truncated"`
}

func (s Store) RawFiles(ctx context.Context, actor account.User, sessionID int64) (RawFiles, error) {
	if _, err := s.GetSession(ctx, actor, sessionID); err != nil {
		return RawFiles{}, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id,status,size_bytes,created_at,COALESCE(closed_at,''),COALESCE(deleted_at,'')
        FROM live_capture_raw_files WHERE session_id = ? ORDER BY id DESC LIMIT 501`, sessionID)
	if err != nil {
		return RawFiles{}, err
	}
	defer rows.Close()
	result := RawFiles{Items: []RawFile{}}
	for rows.Next() {
		var item RawFile
		if err := rows.Scan(&item.ID, &item.Status, &item.SizeBytes, &item.CreatedAt, &item.ClosedAt, &item.DeletedAt); err != nil {
			return RawFiles{}, err
		}
		if len(result.Items) == 500 {
			result.Truncated = true
			break
		}
		result.Items = append(result.Items, item)
	}
	return result, rows.Err()
}
