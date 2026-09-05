package account

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/7grecorder/7grecorder/backend/internal/auth"
)

const (
	RoleSuperAdmin = "SUPER_ADMIN"
	RoleManager    = "MANAGER"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrDisabledUser       = errors.New("user disabled")
	ErrSuperAdminExists   = errors.New("super admin already exists")
	ErrNotFound           = errors.New("not found")
)

type User struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	Role      string `json:"role"`
	Enabled   bool   `json:"enabled"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) Store {
	return Store{db: db}
}

func (s Store) BootstrapSuperAdmin(ctx context.Context, username string, password string) (User, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return User{}, errors.New("username is required")
	}

	var count int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users WHERE role = ?", RoleSuperAdmin).Scan(&count); err != nil {
		return User{}, fmt.Errorf("count super admins: %w", err)
	}
	if count > 0 {
		return User{}, ErrSuperAdminExists
	}

	passwordHash, err := auth.HashPassword(password)
	if err != nil {
		return User{}, err
	}

	result, err := s.db.ExecContext(ctx, `
		INSERT INTO users (username, password_hash, role, enabled)
		VALUES (?, ?, ?, 1)
	`, username, passwordHash, RoleSuperAdmin)
	if err != nil {
		return User{}, fmt.Errorf("create super admin: %w", err)
	}
	userID, err := result.LastInsertId()
	if err != nil {
		return User{}, fmt.Errorf("read user id: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO audit_logs (actor_user_id, action, resource_type, resource_id, summary)
		VALUES (?, 'ADMIN_BOOTSTRAP', 'user', ?, 'Created first SUPER_ADMIN account')
	`, userID, userID); err != nil {
		return User{}, fmt.Errorf("write audit log: %w", err)
	}
	return s.UserByID(ctx, userID)
}

func (s Store) Login(ctx context.Context, username string, password string, ttl time.Duration) (User, string, time.Time, error) {
	var user User
	var passwordHash string
	var enabled int
	err := s.db.QueryRowContext(ctx, `
		SELECT id, username, password_hash, role, enabled, created_at, updated_at
		FROM users
		WHERE username = ?
	`, strings.TrimSpace(username)).Scan(
		&user.ID,
		&user.Username,
		&passwordHash,
		&user.Role,
		&enabled,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, "", time.Time{}, ErrInvalidCredentials
	}
	if err != nil {
		return User{}, "", time.Time{}, fmt.Errorf("load user: %w", err)
	}
	user.Enabled = enabled == 1
	if !user.Enabled {
		return User{}, "", time.Time{}, ErrDisabledUser
	}
	if !auth.VerifyPassword(password, passwordHash) {
		return User{}, "", time.Time{}, ErrInvalidCredentials
	}

	token, digest, err := newSessionToken()
	if err != nil {
		return User{}, "", time.Time{}, err
	}
	expiresAt := time.Now().UTC().Add(ttl)
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO sessions (user_id, token_digest, expires_at, last_seen_at)
		VALUES (?, ?, ?, ?)
	`, user.ID, digest, expiresAt.Format(time.RFC3339), time.Now().UTC().Format(time.RFC3339)); err != nil {
		return User{}, "", time.Time{}, fmt.Errorf("create session: %w", err)
	}
	return user, token, expiresAt, nil
}

func (s Store) UserBySessionToken(ctx context.Context, token string) (User, error) {
	if token == "" {
		return User{}, ErrNotFound
	}
	digest := sessionTokenDigest(token)
	var user User
	var enabled int
	err := s.db.QueryRowContext(ctx, `
		SELECT users.id, users.username, users.role, users.enabled, users.created_at, users.updated_at
		FROM sessions
		INNER JOIN users ON users.id = sessions.user_id
		WHERE sessions.token_digest = ? AND sessions.expires_at > ?
	`, digest, time.Now().UTC().Format(time.RFC3339)).Scan(
		&user.ID,
		&user.Username,
		&user.Role,
		&enabled,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("load session user: %w", err)
	}
	user.Enabled = enabled == 1
	if !user.Enabled {
		return User{}, ErrDisabledUser
	}
	_, _ = s.db.ExecContext(ctx, "UPDATE sessions SET last_seen_at = ? WHERE token_digest = ?", time.Now().UTC().Format(time.RFC3339), digest)
	return user, nil
}

func (s Store) Logout(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	if _, err := s.db.ExecContext(ctx, "DELETE FROM sessions WHERE token_digest = ?", sessionTokenDigest(token)); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

func (s Store) UserByID(ctx context.Context, id int64) (User, error) {
	var user User
	var enabled int
	err := s.db.QueryRowContext(ctx, `
		SELECT id, username, role, enabled, created_at, updated_at
		FROM users
		WHERE id = ?
	`, id).Scan(&user.ID, &user.Username, &user.Role, &enabled, &user.CreatedAt, &user.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("load user: %w", err)
	}
	user.Enabled = enabled == 1
	return user, nil
}

func newSessionToken() (string, string, error) {
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", "", fmt.Errorf("generate session token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(tokenBytes)
	return token, sessionTokenDigest(token), nil
}

func sessionTokenDigest(token string) string {
	sum := sha256.Sum256([]byte(token))
	return base64.RawStdEncoding.EncodeToString(sum[:])
}
