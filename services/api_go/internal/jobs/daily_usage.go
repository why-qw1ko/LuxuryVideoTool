package jobs

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

var taskQuotaLocation = time.FixedZone("UTC+8", 8*60*60)

// DailyUsage returns this account's count of accepted tasks and the next Beijing midnight.
func (s *Service) DailyUsage(ctx context.Context, userID string) (int, time.Time, error) {
	now := s.now().In(taskQuotaLocation)
	year, month, day := now.Date()
	resetAt := time.Date(year, month, day+1, 0, 0, 0, 0, taskQuotaLocation)
	used, err := s.repo.DailyTaskUsage(ctx, userID, now.Format(time.DateOnly))
	return used, resetAt, err
}

func (r *SQLiteRepository) DailyTaskUsage(ctx context.Context, userID, day string) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, `SELECT created_count FROM job_daily_usage WHERE user_id = ? AND usage_date = ?`, userID, day).Scan(&count)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read daily job usage: %w", err)
	}
	return count, nil
}

func countNewTask(ctx context.Context, tx *sql.Tx, userID string, at time.Time, limit int) error {
	result, err := tx.ExecContext(ctx, `INSERT INTO job_daily_usage (user_id, usage_date, created_count)
		VALUES (?, ?, 1) ON CONFLICT (user_id, usage_date) DO UPDATE SET created_count = created_count + 1
		WHERE ? = 0 OR created_count < ?`, userID, at.In(taskQuotaLocation).Format(time.DateOnly), limit, limit)
	if err != nil {
		return fmt.Errorf("record daily job usage: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrDailyTaskLimit
	}
	return nil
}
