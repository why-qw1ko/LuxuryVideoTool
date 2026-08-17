package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

type SQLiteRepository struct{ db *sql.DB }

func NewSQLiteRepository(db *sql.DB) *SQLiteRepository { return &SQLiteRepository{db: db} }

func (r *SQLiteRepository) CreateUser(ctx context.Context, user User) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO users(
		id, username_normalized, display_name, password_hash, role, is_active, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, user.ID, user.UsernameNormalized, user.DisplayName,
		user.PasswordHash, user.Role, boolInt(user.IsActive), millis(user.CreatedAt), millis(user.UpdatedAt))
	if err != nil {
		return fmt.Errorf("create user: %w", err)
	}
	return nil
}

func (r *SQLiteRepository) UpdatePassword(ctx context.Context, userID, passwordHash string, now time.Time) error {
	result, err := r.db.ExecContext(ctx, "UPDATE users SET password_hash = ?, updated_at = ? WHERE id = ?", passwordHash, millis(now), userID)
	if err != nil {
		return fmt.Errorf("update password: %w", err)
	}
	return requireChanged(result)
}

func (r *SQLiteRepository) SetUserActive(ctx context.Context, userID string, active bool, now time.Time) error {
	result, err := r.db.ExecContext(ctx, "UPDATE users SET is_active = ?, updated_at = ? WHERE id = ?", boolInt(active), millis(now), userID)
	if err != nil {
		return fmt.Errorf("set user active: %w", err)
	}
	return requireChanged(result)
}

func (r *SQLiteRepository) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, username_normalized, display_name, password_hash,
		role, is_active, created_at, updated_at, last_login_at FROM users ORDER BY created_at ASC, username_normalized ASC`)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()
	var users []User
	for rows.Next() {
		user, scanErr := scanUser(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan user: %w", scanErr)
		}
		users = append(users, user)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	return users, nil
}

func (r *SQLiteRepository) FindUserByUsername(ctx context.Context, username string) (User, error) {
	return scanUser(r.db.QueryRowContext(ctx, `SELECT id, username_normalized, display_name, password_hash,
		role, is_active, created_at, updated_at, last_login_at FROM users WHERE username_normalized = ?`, username))
}

func (r *SQLiteRepository) FindUserByID(ctx context.Context, userID string) (User, error) {
	return scanUser(r.db.QueryRowContext(ctx, `SELECT id, username_normalized, display_name, password_hash,
		role, is_active, created_at, updated_at, last_login_at FROM users WHERE id = ?`, userID))
}

func (r *SQLiteRepository) FindSessionByID(ctx context.Context, userID, sessionID string) (Session, error) {
	query := `SELECT id, user_id, token_hash, device_id, device_name, platform,
		app_version, expires_at, revoked_at, replaced_by, created_at, last_used_at
		FROM refresh_tokens WHERE id = ?`
	args := []any{sessionID}
	if userID != "" { query += " AND user_id = ?"; args = append(args, userID) }
	row := r.db.QueryRowContext(ctx, query, args...)
	var session Session
	var expires, created, used int64
	var revoked sql.NullInt64
	var replaced sql.NullString
	if err := row.Scan(&session.ID, &session.UserID, &session.TokenHash, &session.Device.ID,
		&session.Device.Name, &session.Device.Platform, &session.Device.AppVersion, &expires,
		&revoked, &replaced, &created, &used); errors.Is(err, sql.ErrNoRows) {
		return Session{}, ErrNotFound
	} else if err != nil {
		return Session{}, fmt.Errorf("find session by id: %w", err)
	}
	session.ExpiresAt, session.CreatedAt, session.LastUsedAt = fromMillis(expires), fromMillis(created), fromMillis(used)
	if revoked.Valid { value := fromMillis(revoked.Int64); session.RevokedAt = &value }
	if replaced.Valid { session.ReplacedBy = &replaced.String }
	return session, nil
}

func (r *SQLiteRepository) RecordLogin(ctx context.Context, userID string, now time.Time) error {
	result, err := r.db.ExecContext(ctx, "UPDATE users SET last_login_at = ?, updated_at = ? WHERE id = ?", millis(now), millis(now), userID)
	if err != nil {
		return fmt.Errorf("record login: %w", err)
	}
	return requireChanged(result)
}

func (r *SQLiteRepository) CreateSession(ctx context.Context, session Session) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO refresh_tokens(
		id, user_id, token_hash, device_id, device_name, platform, app_version,
		expires_at, created_at, last_used_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, session.ID, session.UserID, session.TokenHash,
		session.Device.ID, session.Device.Name, session.Device.Platform, session.Device.AppVersion,
		millis(session.ExpiresAt), millis(session.CreatedAt), millis(session.LastUsedAt))
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	return nil
}

func (r *SQLiteRepository) FindSessionByTokenHash(ctx context.Context, tokenHash string) (Session, User, error) {
	row := r.db.QueryRowContext(ctx, `SELECT
		t.id, t.user_id, t.token_hash, t.device_id, t.device_name, t.platform, t.app_version,
		t.expires_at, t.revoked_at, t.replaced_by, t.created_at, t.last_used_at,
		u.id, u.username_normalized, u.display_name, u.password_hash, u.role, u.is_active,
		u.created_at, u.updated_at, u.last_login_at
	FROM refresh_tokens t JOIN users u ON u.id = t.user_id WHERE t.token_hash = ?`, tokenHash)
	var session Session
	var user User
	var sessionExpires, sessionCreated, sessionUsed int64
	var revokedAt sql.NullInt64
	var replacedBy sql.NullString
	var active int
	var userCreated, userUpdated int64
	var lastLogin sql.NullInt64
	err := row.Scan(&session.ID, &session.UserID, &session.TokenHash, &session.Device.ID,
		&session.Device.Name, &session.Device.Platform, &session.Device.AppVersion, &sessionExpires,
		&revokedAt, &replacedBy, &sessionCreated, &sessionUsed, &user.ID, &user.UsernameNormalized,
		&user.DisplayName, &user.PasswordHash, &user.Role, &active, &userCreated, &userUpdated, &lastLogin)
	if errors.Is(err, sql.ErrNoRows) {
		return Session{}, User{}, ErrInvalidToken
	}
	if err != nil {
		return Session{}, User{}, fmt.Errorf("find session: %w", err)
	}
	session.ExpiresAt, session.CreatedAt, session.LastUsedAt = fromMillis(sessionExpires), fromMillis(sessionCreated), fromMillis(sessionUsed)
	if revokedAt.Valid {
		value := fromMillis(revokedAt.Int64)
		session.RevokedAt = &value
	}
	if replacedBy.Valid {
		session.ReplacedBy = &replacedBy.String
	}
	user.IsActive, user.CreatedAt, user.UpdatedAt = active == 1, fromMillis(userCreated), fromMillis(userUpdated)
	if lastLogin.Valid {
		value := fromMillis(lastLogin.Int64)
		user.LastLoginAt = &value
	}
	return session, user, nil
}

func (r *SQLiteRepository) RotateSession(ctx context.Context, oldID string, replacement Session, now time.Time) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin session rotation: %w", err)
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO refresh_tokens(
		id, user_id, token_hash, device_id, device_name, platform, app_version, expires_at, created_at, last_used_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, replacement.ID, replacement.UserID, replacement.TokenHash,
		replacement.Device.ID, replacement.Device.Name, replacement.Device.Platform, replacement.Device.AppVersion,
		millis(replacement.ExpiresAt), millis(replacement.CreatedAt), millis(replacement.LastUsedAt))
	if err != nil {
		return fmt.Errorf("create rotated session: %w", err)
	}
	result, err := tx.ExecContext(ctx, `UPDATE refresh_tokens SET revoked_at = ?, replaced_by = ?, last_used_at = ?
		WHERE id = ? AND revoked_at IS NULL AND expires_at > ?`, millis(now), replacement.ID, millis(now), oldID, millis(now))
	if err != nil {
		return fmt.Errorf("revoke rotated session: %w", err)
	}
	if err := requireChanged(result); err != nil {
		return ErrRevokedSession
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit session rotation: %w", err)
	}
	return nil
}

func (r *SQLiteRepository) ListSessions(ctx context.Context, userID string, now time.Time) ([]Session, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, user_id, token_hash, device_id, device_name, platform,
		app_version, expires_at, revoked_at, replaced_by, created_at, last_used_at
		FROM refresh_tokens WHERE user_id = ? AND revoked_at IS NULL AND expires_at > ? ORDER BY last_used_at DESC`, userID, millis(now))
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	defer rows.Close()
	var sessions []Session
	for rows.Next() {
		var session Session
		var expires, created, used int64
		var revoked sql.NullInt64
		var replaced sql.NullString
		if err := rows.Scan(&session.ID, &session.UserID, &session.TokenHash, &session.Device.ID,
			&session.Device.Name, &session.Device.Platform, &session.Device.AppVersion, &expires,
			&revoked, &replaced, &created, &used); err != nil {
			return nil, fmt.Errorf("scan session: %w", err)
		}
		session.TokenHash = ""
		session.ExpiresAt, session.CreatedAt, session.LastUsedAt = fromMillis(expires), fromMillis(created), fromMillis(used)
		sessions = append(sessions, session)
	}
	return sessions, rows.Err()
}

func (r *SQLiteRepository) RevokeSession(ctx context.Context, userID, sessionID string, now time.Time) error {
	result, err := r.db.ExecContext(ctx, `UPDATE refresh_tokens SET revoked_at = ?, last_used_at = ?
		WHERE id = ? AND user_id = ? AND revoked_at IS NULL`, millis(now), millis(now), sessionID, userID)
	if err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	return requireChanged(result)
}

func (r *SQLiteRepository) RevokeAllUserSessions(ctx context.Context, userID string, now time.Time) error {
	_, err := r.db.ExecContext(ctx, `UPDATE refresh_tokens SET revoked_at = ?, last_used_at = ?
		WHERE user_id = ? AND revoked_at IS NULL`, millis(now), millis(now), userID)
	if err != nil {
		return fmt.Errorf("revoke user sessions: %w", err)
	}
	return nil
}

type scanner interface{ Scan(dest ...any) error }

func scanUser(row scanner) (User, error) {
	var user User
	var active int
	var created, updated int64
	var lastLogin sql.NullInt64
	err := row.Scan(&user.ID, &user.UsernameNormalized, &user.DisplayName, &user.PasswordHash,
		&user.Role, &active, &created, &updated, &lastLogin)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("scan user: %w", err)
	}
	user.IsActive, user.CreatedAt, user.UpdatedAt = active == 1, fromMillis(created), fromMillis(updated)
	if lastLogin.Valid {
		value := fromMillis(lastLogin.Int64)
		user.LastLoginAt = &value
	}
	return user, nil
}

func requireChanged(result sql.Result) error {
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrNotFound
	}
	return nil
}

func millis(value time.Time) int64 { return value.UTC().UnixMilli() }
func fromMillis(value int64) time.Time { return time.UnixMilli(value).UTC() }
func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
