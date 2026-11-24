package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

const cdkColumns = "id, prefix, task_limit, task_count, created_at, redeemed_at, revoked_at"

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
	item, err := scanCDK(r.db.QueryRowContext(ctx, `UPDATE login_cdks
		SET task_limit = CASE WHEN revoked_at IS NULL THEN ? ELSE task_limit END
		WHERE id = ? RETURNING `+cdkColumns, limit, id))
	if err == nil && item.RevokedAt != nil {
		return CDK{}, ErrCDKRevoked
	}
	return item, err
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

func (r *SQLiteRepository) RedeemCDK(ctx context.Context, hash string, user User, session Session) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// Claim the code first. The deferred user foreign key permits the new
	// account and session to be inserted in this same transaction.
	result, err := tx.ExecContext(ctx, `UPDATE login_cdks SET redeemed_at = ?, user_id = ?
		WHERE code_hash = ? AND redeemed_at IS NULL AND revoked_at IS NULL`, millis(user.CreatedAt), user.ID, hash)
	if err != nil {
		return fmt.Errorf("redeem CDK: %w", err)
	}
	if err := requireChanged(result); errors.Is(err, ErrNotFound) {
		return ErrInvalidCDK
	} else if err != nil {
		return err
	}
	if err := insertUser(ctx, tx, user); err != nil {
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
