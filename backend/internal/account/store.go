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
	ErrForbidden          = errors.New("forbidden")
	ErrValidation         = errors.New("validation failed")
	ErrUsernameInUse      = errors.New("username is already in use")
	ErrCannotDisableSelf  = errors.New("cannot disable current user")
)

type User struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	Role      string `json:"role"`
	Enabled   bool   `json:"enabled"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type ManagerPolicy struct {
	CanEditRecordingProfile bool   `json:"can_edit_recording_profile"`
	CanEditBilibiliModule   bool   `json:"can_edit_bilibili_module"`
	CanEditCosModule        bool   `json:"can_edit_cos_module"`
	CanEditNeteaseModule    bool   `json:"can_edit_netease_module"`
	CanManageLocalFiles     bool   `json:"can_manage_local_files"`
	UpdatedAt               string `json:"updated_at"`
}

type Account struct {
	User
	ProfileCount int64          `json:"profile_count"`
	Policy       *ManagerPolicy `json:"policy,omitempty"`
}

type CreateRequest struct {
	Username string         `json:"username"`
	Password string         `json:"password"`
	Role     string         `json:"role"`
	Enabled  *bool          `json:"enabled"`
	Policy   *PolicyUpsert  `json:"policy"`
}

type UpdateRequest struct {
	Username *string `json:"username"`
	Password *string `json:"password"`
	Enabled  *bool   `json:"enabled"`
}

type PolicyUpsert struct {
	CanEditRecordingProfile *bool `json:"can_edit_recording_profile"`
	CanEditBilibiliModule   *bool `json:"can_edit_bilibili_module"`
	CanEditCosModule        *bool `json:"can_edit_cos_module"`
	CanEditNeteaseModule    *bool `json:"can_edit_netease_module"`
	CanManageLocalFiles     *bool `json:"can_manage_local_files"`
}

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) Store {
	return Store{db: db}
}

func (s Store) List(ctx context.Context, actor User) ([]Account, error) {
	if actor.Role != RoleSuperAdmin {
		return nil, ErrForbidden
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT u.id, u.username, u.role, u.enabled, u.created_at, u.updated_at,
			COUNT(rp.id)
		FROM users u
		LEFT JOIN recording_profiles rp ON rp.owner_user_id = u.id
		GROUP BY u.id, u.username, u.role, u.enabled, u.created_at, u.updated_at
		ORDER BY u.role ASC, u.username ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("list accounts: %w", err)
	}
	defer rows.Close()

	items := make([]Account, 0)
	for rows.Next() {
		item, err := scanAccountRow(rows)
		if err != nil {
			return nil, err
		}
		if item.Role == RoleManager {
			policy, err := s.Policy(ctx, actor, item.ID)
			if err != nil {
				return nil, err
			}
			item.Policy = &policy
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate accounts: %w", err)
	}
	return items, nil
}

func (s Store) Create(ctx context.Context, actor User, req CreateRequest) (Account, error) {
	if actor.Role != RoleSuperAdmin {
		return Account{}, ErrForbidden
	}
	username := strings.TrimSpace(req.Username)
	role := strings.TrimSpace(req.Role)
	if role == "" {
		role = RoleManager
	}
	if username == "" || req.Password == "" || role != RoleManager {
		return Account{}, ErrValidation
	}
	passwordHash, err := auth.HashPassword(req.Password)
	if err != nil {
		return Account{}, err
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Account{}, fmt.Errorf("begin account create: %w", err)
	}
	defer tx.Rollback()

	result, err := tx.ExecContext(ctx, `
		INSERT INTO users (username, password_hash, role, enabled)
		VALUES (?, ?, ?, ?)
	`, username, passwordHash, role, boolInt(enabled))
	if isUniqueConstraint(err) {
		return Account{}, ErrUsernameInUse
	}
	if err != nil {
		return Account{}, fmt.Errorf("create account: %w", err)
	}
	userID, err := result.LastInsertId()
	if err != nil {
		return Account{}, fmt.Errorf("read account id: %w", err)
	}
	policy := mergePolicy(defaultManagerPolicy(), req.Policy)
	if err := upsertPolicy(ctx, tx, userID, policy); err != nil {
		return Account{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO audit_logs (actor_user_id, action, resource_type, resource_id, summary)
		VALUES (?, 'ACCOUNT_CREATE', 'user', ?, ?)
	`, actor.ID, userID, "Created MANAGER account"); err != nil {
		return Account{}, fmt.Errorf("write audit log: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Account{}, fmt.Errorf("commit account create: %w", err)
	}
	return s.Get(ctx, actor, userID)
}

func (s Store) Get(ctx context.Context, actor User, id int64) (Account, error) {
	if actor.Role != RoleSuperAdmin {
		return Account{}, ErrForbidden
	}
	row := s.db.QueryRowContext(ctx, `
		SELECT u.id, u.username, u.role, u.enabled, u.created_at, u.updated_at,
			COUNT(rp.id)
		FROM users u
		LEFT JOIN recording_profiles rp ON rp.owner_user_id = u.id
		WHERE u.id = ?
		GROUP BY u.id, u.username, u.role, u.enabled, u.created_at, u.updated_at
	`, id)
	item, err := scanAccountRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Account{}, ErrNotFound
	}
	if err != nil {
		return Account{}, fmt.Errorf("load account: %w", err)
	}
	if item.Role == RoleManager {
		policy, err := s.Policy(ctx, actor, item.ID)
		if err != nil {
			return Account{}, err
		}
		item.Policy = &policy
	}
	return item, nil
}

func (s Store) Update(ctx context.Context, actor User, id int64, req UpdateRequest) (Account, error) {
	if actor.Role != RoleSuperAdmin {
		return Account{}, ErrForbidden
	}
	current, err := s.Get(ctx, actor, id)
	if err != nil {
		return Account{}, err
	}
	username := current.Username
	enabled := current.Enabled
	if req.Username != nil {
		username = strings.TrimSpace(*req.Username)
	}
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	if username == "" {
		return Account{}, ErrValidation
	}
	if id == actor.ID && !enabled {
		return Account{}, ErrCannotDisableSelf
	}
	if req.Password != nil && *req.Password == "" {
		return Account{}, ErrValidation
	}

	if req.Password != nil {
		passwordHash, err := auth.HashPassword(*req.Password)
		if err != nil {
			return Account{}, err
		}
		_, err = s.db.ExecContext(ctx, `
			UPDATE users
			SET username = ?, password_hash = ?, enabled = ?, updated_at = CURRENT_TIMESTAMP
			WHERE id = ?
		`, username, passwordHash, boolInt(enabled), id)
		if isUniqueConstraint(err) {
			return Account{}, ErrUsernameInUse
		}
		if err != nil {
			return Account{}, fmt.Errorf("update account: %w", err)
		}
	} else {
		_, err = s.db.ExecContext(ctx, `
			UPDATE users
			SET username = ?, enabled = ?, updated_at = CURRENT_TIMESTAMP
			WHERE id = ?
		`, username, boolInt(enabled), id)
		if isUniqueConstraint(err) {
			return Account{}, ErrUsernameInUse
		}
		if err != nil {
			return Account{}, fmt.Errorf("update account: %w", err)
		}
	}
	return s.Get(ctx, actor, id)
}

func (s Store) Policy(ctx context.Context, actor User, userID int64) (ManagerPolicy, error) {
	if actor.Role != RoleSuperAdmin && actor.ID != userID {
		return ManagerPolicy{}, ErrForbidden
	}
	policy, err := s.policy(ctx, userID)
	if errors.Is(err, sql.ErrNoRows) {
		return defaultManagerPolicy(), nil
	}
	if err != nil {
		return ManagerPolicy{}, err
	}
	return policy, nil
}

func (s Store) UpsertPolicy(ctx context.Context, actor User, userID int64, req PolicyUpsert) (ManagerPolicy, error) {
	if actor.Role != RoleSuperAdmin {
		return ManagerPolicy{}, ErrForbidden
	}
	user, err := s.UserByID(ctx, userID)
	if err != nil {
		return ManagerPolicy{}, err
	}
	if user.Role != RoleManager {
		return ManagerPolicy{}, ErrValidation
	}
	current, err := s.Policy(ctx, actor, userID)
	if err != nil {
		return ManagerPolicy{}, err
	}
	policy := mergePolicy(current, &req)
	if err := upsertPolicy(ctx, s.db, userID, policy); err != nil {
		return ManagerPolicy{}, err
	}
	return s.Policy(ctx, actor, userID)
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

type accountScanner interface {
	Scan(dest ...interface{}) error
}

type policyExecutor interface {
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
}

func scanAccountRow(row accountScanner) (Account, error) {
	var item Account
	var enabled int
	if err := row.Scan(
		&item.ID,
		&item.Username,
		&item.Role,
		&enabled,
		&item.CreatedAt,
		&item.UpdatedAt,
		&item.ProfileCount,
	); err != nil {
		return Account{}, err
	}
	item.Enabled = enabled == 1
	return item, nil
}

func (s Store) policy(ctx context.Context, userID int64) (ManagerPolicy, error) {
	var policy ManagerPolicy
	var canEditRecordingProfile int
	var canEditBilibiliModule int
	var canEditCosModule int
	var canEditNeteaseModule int
	var canManageLocalFiles int
	err := s.db.QueryRowContext(ctx, `
		SELECT can_edit_recording_profile, can_edit_bilibili_module, can_edit_cos_module,
			can_edit_netease_module, can_manage_local_files, updated_at
		FROM manager_policies
		WHERE user_id = ?
	`, userID).Scan(
		&canEditRecordingProfile,
		&canEditBilibiliModule,
		&canEditCosModule,
		&canEditNeteaseModule,
		&canManageLocalFiles,
		&policy.UpdatedAt,
	)
	if err != nil {
		return ManagerPolicy{}, err
	}
	policy.CanEditRecordingProfile = canEditRecordingProfile == 1
	policy.CanEditBilibiliModule = canEditBilibiliModule == 1
	policy.CanEditCosModule = canEditCosModule == 1
	policy.CanEditNeteaseModule = canEditNeteaseModule == 1
	policy.CanManageLocalFiles = canManageLocalFiles == 1
	return policy, nil
}

func defaultManagerPolicy() ManagerPolicy {
	return ManagerPolicy{
		CanEditRecordingProfile: true,
		CanEditBilibiliModule:   true,
		CanEditCosModule:        true,
		CanEditNeteaseModule:    true,
		CanManageLocalFiles:     true,
	}
}

func mergePolicy(policy ManagerPolicy, req *PolicyUpsert) ManagerPolicy {
	if req == nil {
		return policy
	}
	if req.CanEditRecordingProfile != nil {
		policy.CanEditRecordingProfile = *req.CanEditRecordingProfile
	}
	if req.CanEditBilibiliModule != nil {
		policy.CanEditBilibiliModule = *req.CanEditBilibiliModule
	}
	if req.CanEditCosModule != nil {
		policy.CanEditCosModule = *req.CanEditCosModule
	}
	if req.CanEditNeteaseModule != nil {
		policy.CanEditNeteaseModule = *req.CanEditNeteaseModule
	}
	if req.CanManageLocalFiles != nil {
		policy.CanManageLocalFiles = *req.CanManageLocalFiles
	}
	return policy
}

func upsertPolicy(ctx context.Context, exec policyExecutor, userID int64, policy ManagerPolicy) error {
	if _, err := exec.ExecContext(ctx, `
		INSERT INTO manager_policies
			(user_id, can_edit_recording_profile, can_edit_bilibili_module, can_edit_cos_module,
				can_edit_netease_module, can_manage_local_files)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(user_id) DO UPDATE SET
			can_edit_recording_profile = excluded.can_edit_recording_profile,
			can_edit_bilibili_module = excluded.can_edit_bilibili_module,
			can_edit_cos_module = excluded.can_edit_cos_module,
			can_edit_netease_module = excluded.can_edit_netease_module,
			can_manage_local_files = excluded.can_manage_local_files,
			updated_at = CURRENT_TIMESTAMP
	`, userID,
		boolInt(policy.CanEditRecordingProfile),
		boolInt(policy.CanEditBilibiliModule),
		boolInt(policy.CanEditCosModule),
		boolInt(policy.CanEditNeteaseModule),
		boolInt(policy.CanManageLocalFiles),
	); err != nil {
		return fmt.Errorf("upsert manager policy: %w", err)
	}
	return nil
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func isUniqueConstraint(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "unique constraint")
}
