package jobs

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/auth"
)

var ErrCDKTaskLimit = errors.New("CDK task limit reached")

func countCDKTask(ctx context.Context, tx *sql.Tx, userID string) error {
	var used, limit, revoked, deleted sql.NullInt64
	err := tx.QueryRowContext(ctx, `SELECT c.task_count, c.task_limit, c.revoked_at, u.deleted_at
		FROM users u LEFT JOIN login_cdks c ON c.user_id = u.id
		WHERE u.id = ? AND (u.is_cdk = 1 OR u.deleted_at IS NOT NULL)`, userID).Scan(&used, &limit, &revoked, &deleted)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read CDK task usage: %w", err)
	}
	// Reject requests authenticated before account deletion or CDK revocation.
	if deleted.Valid || !used.Valid || revoked.Valid {
		return auth.ErrRevokedSession
	}
	if used.Int64 >= limit.Int64 {
		return ErrCDKTaskLimit
	}
	if _, err := tx.ExecContext(ctx, "UPDATE login_cdks SET task_count = task_count + 1 WHERE user_id = ?", userID); err != nil {
		return fmt.Errorf("record CDK task usage: %w", err)
	}
	return nil
}
