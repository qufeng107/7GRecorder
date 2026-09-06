package job

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/7grecorder/7grecorder/backend/internal/account"
)

var (
	ErrForbidden  = errors.New("job forbidden")
	ErrNotFound   = errors.New("job not found")
	ErrValidation = errors.New("job validation failed")
)

type Job struct {
	ID                 int64  `json:"id"`
	RecordingProfileID int64  `json:"recording_profile_id,omitempty"`
	RecordingID        int64  `json:"recording_id,omitempty"`
	RecordingFileID    int64  `json:"recording_file_id,omitempty"`
	Type               string `json:"type"`
	ResourceClass      string `json:"resource_class"`
	BusinessKey        string `json:"business_key,omitempty"`
	Status             string `json:"status"`
	Priority           int    `json:"priority"`
	Attempts           int    `json:"attempts"`
	MaxAttempts        int    `json:"max_attempts"`
	RunAfter           string `json:"run_after"`
	LockedAt           string `json:"locked_at,omitempty"`
	HeartbeatAt        string `json:"heartbeat_at,omitempty"`
	LockedBy           string `json:"locked_by,omitempty"`
	LastErrorClass     string `json:"last_error_class,omitempty"`
	LastError          string `json:"last_error,omitempty"`
	CreatedAt          string `json:"created_at"`
	UpdatedAt          string `json:"updated_at"`
	ProfileName        string `json:"profile_name,omitempty"`
	OwnerUsername      string `json:"owner_username,omitempty"`
}

type Store struct {
	db *sql.DB
}

func NewStore(database *sql.DB) Store {
	return Store{db: database}
}

func (s Store) List(ctx context.Context, actor account.User, limit int) ([]Job, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}

	query := `
		SELECT j.id,
			COALESCE(j.recording_profile_id, 0),
			COALESCE(j.recording_id, 0),
			COALESCE(j.recording_file_id, 0),
			j.type,
			j.resource_class,
			COALESCE(j.business_key, ''),
			j.status,
			j.priority,
			j.attempts,
			j.max_attempts,
			j.run_after,
			COALESCE(j.locked_at, ''),
			COALESCE(j.heartbeat_at, ''),
			COALESCE(j.locked_by, ''),
			COALESCE(j.last_error_class, ''),
			COALESCE(j.last_error, ''),
			j.created_at,
			j.updated_at,
			COALESCE(p.name, ''),
			COALESCE(u.username, '')
		FROM jobs j
		LEFT JOIN recording_profiles p ON p.id = j.recording_profile_id
		LEFT JOIN users u ON u.id = p.owner_user_id
	`
	args := []interface{}{}
	if actor.Role != account.RoleSuperAdmin {
		query += `
			WHERE j.recording_profile_id IN (
				SELECT id FROM recording_profiles WHERE owner_user_id = ?
			)
		`
		args = append(args, actor.ID)
	}
	query += " ORDER BY j.updated_at DESC, j.id DESC LIMIT ?"
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list jobs: %w", err)
	}
	defer rows.Close()

	items := make([]Job, 0)
	for rows.Next() {
		var item Job
		if err := rows.Scan(
			&item.ID,
			&item.RecordingProfileID,
			&item.RecordingID,
			&item.RecordingFileID,
			&item.Type,
			&item.ResourceClass,
			&item.BusinessKey,
			&item.Status,
			&item.Priority,
			&item.Attempts,
			&item.MaxAttempts,
			&item.RunAfter,
			&item.LockedAt,
			&item.HeartbeatAt,
			&item.LockedBy,
			&item.LastErrorClass,
			&item.LastError,
			&item.CreatedAt,
			&item.UpdatedAt,
			&item.ProfileName,
			&item.OwnerUsername,
		); err != nil {
			return nil, fmt.Errorf("scan job: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate jobs: %w", err)
	}
	return items, nil
}

func (s Store) Retry(ctx context.Context, actor account.User, id int64) (Job, error) {
	if id <= 0 {
		return Job{}, ErrValidation
	}
	current, err := s.Get(ctx, actor, id)
	if err != nil {
		return Job{}, err
	}
	if current.Status != "FAILED" && current.Status != "CANCELLED" {
		return Job{}, ErrValidation
	}
	if _, err := s.db.ExecContext(ctx, `
		UPDATE jobs
		SET status = 'PENDING',
			attempts = 0,
			run_after = CURRENT_TIMESTAMP,
			locked_at = NULL,
			heartbeat_at = NULL,
			locked_by = NULL,
			last_error_class = NULL,
			last_error = NULL,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, id); err != nil {
		return Job{}, fmt.Errorf("retry job: %w", err)
	}
	return s.Get(ctx, actor, id)
}

func (s Store) Cancel(ctx context.Context, actor account.User, id int64) (Job, error) {
	if id <= 0 {
		return Job{}, ErrValidation
	}
	current, err := s.Get(ctx, actor, id)
	if err != nil {
		return Job{}, err
	}
	if current.Status == "SUCCEEDED" || current.Status == "CANCELLED" || current.Status == "RUNNING" {
		return Job{}, ErrValidation
	}
	if _, err := s.db.ExecContext(ctx, `
		UPDATE jobs
		SET status = 'CANCELLED',
			locked_at = NULL,
			heartbeat_at = NULL,
			locked_by = NULL,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, id); err != nil {
		return Job{}, fmt.Errorf("cancel job: %w", err)
	}
	return s.Get(ctx, actor, id)
}

func (s Store) Get(ctx context.Context, actor account.User, id int64) (Job, error) {
	items, err := s.listByID(ctx, actor, id)
	if err != nil {
		return Job{}, err
	}
	if len(items) == 0 {
		return Job{}, ErrNotFound
	}
	return items[0], nil
}

func (s Store) listByID(ctx context.Context, actor account.User, id int64) ([]Job, error) {
	if id <= 0 {
		return nil, ErrValidation
	}
	query := `
		SELECT j.id,
			COALESCE(j.recording_profile_id, 0),
			COALESCE(j.recording_id, 0),
			COALESCE(j.recording_file_id, 0),
			j.type,
			j.resource_class,
			COALESCE(j.business_key, ''),
			j.status,
			j.priority,
			j.attempts,
			j.max_attempts,
			j.run_after,
			COALESCE(j.locked_at, ''),
			COALESCE(j.heartbeat_at, ''),
			COALESCE(j.locked_by, ''),
			COALESCE(j.last_error_class, ''),
			COALESCE(j.last_error, ''),
			j.created_at,
			j.updated_at,
			COALESCE(p.name, ''),
			COALESCE(u.username, '')
		FROM jobs j
		LEFT JOIN recording_profiles p ON p.id = j.recording_profile_id
		LEFT JOIN users u ON u.id = p.owner_user_id
		WHERE j.id = ?
	`
	args := []interface{}{id}
	if actor.Role != account.RoleSuperAdmin {
		query += `
			AND j.recording_profile_id IN (
				SELECT id FROM recording_profiles WHERE owner_user_id = ?
			)
		`
		args = append(args, actor.ID)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("get job: %w", err)
	}
	defer rows.Close()

	items := make([]Job, 0, 1)
	for rows.Next() {
		var item Job
		if err := rows.Scan(
			&item.ID,
			&item.RecordingProfileID,
			&item.RecordingID,
			&item.RecordingFileID,
			&item.Type,
			&item.ResourceClass,
			&item.BusinessKey,
			&item.Status,
			&item.Priority,
			&item.Attempts,
			&item.MaxAttempts,
			&item.RunAfter,
			&item.LockedAt,
			&item.HeartbeatAt,
			&item.LockedBy,
			&item.LastErrorClass,
			&item.LastError,
			&item.CreatedAt,
			&item.UpdatedAt,
			&item.ProfileName,
			&item.OwnerUsername,
		); err != nil {
			return nil, fmt.Errorf("scan job: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate job: %w", err)
	}
	return items, nil
}
