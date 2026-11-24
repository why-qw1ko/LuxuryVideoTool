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
	var used, limit int
	var revoked sql.NullInt64
	err := tx.QueryRowContext(ctx, "SELECT task_count, task_limit, revoked_at FROM login_cdks WHERE user_id = ?", userID).
		Scan(&used, &limit, &revoked)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read CDK task usage: %w", err)
	}
	if revoked.Valid {
		return auth.ErrRevokedSession
	}
	if used >= limit {
		return ErrCDKTaskLimit
	}
	if _, err := tx.ExecContext(ctx, "UPDATE login_cdks SET task_count = task_count + 1 WHERE user_id = ?", userID); err != nil {
		return fmt.Errorf("record CDK task usage: %w", err)
	}
	return nil
}
