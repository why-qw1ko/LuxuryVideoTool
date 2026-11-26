package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

const cdkColumns = "id, prefix, task_limit, task_count, created_at, code_redeemed_at, revoked_at"

func (r *SQLiteRepository) CreateCDKs(ctx context.Context, items []CDK) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin CDK generation: %w", err)
	}
	defer tx.Rollback()
	for _, item := range items {
		if _, err := tx.ExecContext(ctx, `INSERT INTO login_cdks (id, code_hash, prefix, task_limit, created_at)
			VALUES (?, ?, ?, ?, ?)`, item.ID, item.codeHash, item.Prefix, item.TaskLimit, millis(item.CreatedAt)); err != nil {
			return fmt.Errorf("store CDK: %w", err)
		}
	}
	return tx.Commit()
}

func (r *SQLiteRepository) ListCDKs(ctx context.Context, limit, offset int) ([]CDK, int, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, 0, err
	}
	defer tx.Rollback()
	var total int
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM login_cdks").Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := tx.QueryContext(ctx, "SELECT "+cdkColumns+" FROM login_cdks ORDER BY created_at DESC, id DESC LIMIT ? OFFSET ?", limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := make([]CDK, 0, min(limit, total))
	for rows.Next() {
		item, err := scanCDK(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}

func (r *SQLiteRepository) SetCDKTaskLimit(ctx context.Context, id string, limit int) (CDK, error) {
	return scanMutableCDK(r.db.QueryRowContext(ctx, `UPDATE login_cdks
		SET task_limit = CASE WHEN revoked_at IS NULL THEN ? ELSE task_limit END
		WHERE id = ? RETURNING `+cdkColumns, limit, id))
}

func (r *SQLiteRepository) ResetCDKTaskCount(ctx context.Context, id string) (CDK, error) {
	return scanMutableCDK(r.db.QueryRowContext(ctx, `UPDATE login_cdks
		SET task_count = CASE WHEN revoked_at IS NULL THEN 0 ELSE task_count END
		WHERE id = ? RETURNING `+cdkColumns, id))
}

func (r *SQLiteRepository) RotateCDK(ctx context.Context, id, prefix, hash string, now time.Time) (CDK, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return CDK{}, err
	}
	defer tx.Rollback()
	item, err := scanMutableCDK(tx.QueryRowContext(ctx, `UPDATE login_cdks SET
		code_hash = CASE WHEN revoked_at IS NULL THEN ? ELSE code_hash END,
		prefix = CASE WHEN revoked_at IS NULL THEN ? ELSE prefix END,
		code_redeemed_at = CASE WHEN revoked_at IS NULL THEN NULL ELSE code_redeemed_at END
		WHERE id = ? RETURNING `+cdkColumns, hash, prefix, id))
	if err != nil {
		return CDK{}, err
	}
	// Retain session records while removing lookup by any refresh token from the previous key.
	at := millis(now)
	if _, err := tx.ExecContext(ctx, `UPDATE refresh_tokens SET
		token_hash = 'cdk-rotated:' || id,
		last_used_at = CASE WHEN revoked_at IS NULL THEN ? ELSE last_used_at END,
		revoked_at = COALESCE(revoked_at, ?)
		WHERE user_id = (SELECT user_id FROM login_cdks WHERE id = ?) AND token_hash NOT LIKE 'cdk-rotated:%'`, at, at, id); err != nil {
		return CDK{}, fmt.Errorf("invalidate rotated CDK sessions: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return CDK{}, err
	}
	return item, nil
}

func (r *SQLiteRepository) RevokeCDK(ctx context.Context, id string, now time.Time) (CDK, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return CDK{}, err
	}
	defer tx.Rollback()
	at := millis(now)
	item, err := scanCDK(tx.QueryRowContext(ctx, "UPDATE login_cdks SET revoked_at = COALESCE(revoked_at, ?) WHERE id = ? RETURNING "+cdkColumns, at, id))
	if err != nil {
		return CDK{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE users SET is_active = 0, updated_at = ?
		WHERE id = (SELECT user_id FROM login_cdks WHERE id = ?) AND is_active = 1`, at, id); err != nil {
		return CDK{}, fmt.Errorf("disable CDK user: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE refresh_tokens SET revoked_at = ?, last_used_at = ?
		WHERE user_id = (SELECT user_id FROM login_cdks WHERE id = ?) AND revoked_at IS NULL`, at, at, id); err != nil {
		return CDK{}, fmt.Errorf("revoke CDK sessions: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return CDK{}, err
	}
	return item, nil
}

func (r *SQLiteRepository) DeleteRevokedCDK(ctx context.Context, id string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var deletedID string
	err = tx.QueryRowContext(ctx, "DELETE FROM login_cdks WHERE id = ? AND revoked_at IS NOT NULL RETURNING id", id).Scan(&deletedID)
	if errors.Is(err, sql.ErrNoRows) {
		var exists bool
		if err := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM login_cdks WHERE id = ?)", id).Scan(&exists); err != nil {
			return err
		}
		if exists {
			return ErrCDKNotRevoked
		}
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (r *SQLiteRepository) CreateCDKSession(ctx context.Context, hash string, user User, newSession func(User) (Session, error)) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// Check quota in the write transaction that binds the first account or reuses an existing one.
	// The deferred foreign key permits first-time account creation in this transaction.
	at := millis(user.CreatedAt)
	var userID string
	err = tx.QueryRowContext(ctx, `UPDATE login_cdks SET code_redeemed_at = COALESCE(code_redeemed_at, ?),
		redeemed_at = COALESCE(redeemed_at, ?), user_id = COALESCE(user_id, ?)
		WHERE code_hash = ? AND revoked_at IS NULL AND task_count < task_limit
		RETURNING user_id`, at, at, user.ID, hash).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		var exhausted bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(
			SELECT 1 FROM login_cdks WHERE code_hash = ? AND revoked_at IS NULL AND task_count >= task_limit
		)`, hash).Scan(&exhausted); err != nil {
			return fmt.Errorf("check CDK task quota: %w", err)
		}
		if exhausted {
			return ErrCDKExhausted
		}
		return ErrInvalidCDK
	}
	if err != nil {
		return fmt.Errorf("prepare CDK session: %w", err)
	}
	if userID == user.ID {
		if err := insertUser(ctx, tx, user); err != nil {
			return err
		}
	} else {
		user, err = scanUser(tx.QueryRowContext(ctx, `UPDATE users SET display_name = ?, updated_at = ?, last_login_at = ?
			WHERE id = ? AND is_cdk = 1 AND is_active = 1
			RETURNING id, username_normalized, display_name, password_hash, role, is_active, is_cdk, created_at, updated_at, last_login_at`,
			user.DisplayName, at, at, userID))
		if errors.Is(err, ErrNotFound) {
			return ErrInvalidCDK
		}
		if err != nil {
			return err
		}
	}
	session, err := newSession(user)
	if err != nil {
		return err
	}
	if err := insertSession(ctx, tx, session); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *SQLiteRepository) CDKUsage(ctx context.Context, userID string) (CDKUsage, error) {
	var usage CDKUsage
	err := r.db.QueryRowContext(ctx, "SELECT task_count, task_limit FROM login_cdks WHERE user_id = ?", userID).Scan(&usage.Used, &usage.Limit)
	return usage, err
}

func scanMutableCDK(row scanner) (CDK, error) {
	item, err := scanCDK(row)
	if err == nil && item.RevokedAt != nil {
		return CDK{}, ErrCDKRevoked
	}
	return item, err
}

func scanCDK(row scanner) (CDK, error) {
	var item CDK
	var created int64
	var redeemed, revoked sql.NullInt64
	if err := row.Scan(&item.ID, &item.Prefix, &item.TaskLimit, &item.TaskCount, &created, &redeemed, &revoked); errors.Is(err, sql.ErrNoRows) {
		return CDK{}, ErrNotFound
	} else if err != nil {
		return CDK{}, err
	}
	item.CreatedAt = fromMillis(created)
	item.Status = "unused"
	if redeemed.Valid {
		at := fromMillis(redeemed.Int64)
		item.RedeemedAt = &at
		item.Status = "redeemed"
	}
	if revoked.Valid {
		at := fromMillis(revoked.Int64)
		item.RevokedAt = &at
		item.Status = "revoked"
	}
	return item, nil
}
