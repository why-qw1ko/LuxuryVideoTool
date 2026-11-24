package settings

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

const usageSetting = "usage_policy"
const MaxDailyTaskLimit = 100_000

var ErrInvalidUsage = errors.New("invalid usage settings")
var ErrUserNotFound = errors.New("usage settings user not found")

type Usage struct {
	DailyTaskLimit  int  `json:"dailyTaskLimit"`
	AllowInfo       bool `json:"allowInfo"`
	AllowDownload   bool `json:"allowDownload"`
	AllowTranscribe bool `json:"allowTranscribe"`
}

func DefaultUsage() Usage {
	return Usage{AllowInfo: true, AllowDownload: true, AllowTranscribe: true}
}

func (p Usage) Allows(action string) bool {
	switch action {
	case "info":
		return p.AllowInfo
	case "download":
		return p.AllowDownload
	case "full", "transcribe":
		return p.AllowTranscribe
	default:
		return false
	}
}

func (s *Service) Usage(ctx context.Context) (Usage, error) {
	value := DefaultUsage()
	stored, err := s.Get(ctx, usageSetting)
	if errors.Is(err, sql.ErrNoRows) {
		return value, nil
	}
	if err != nil {
		return Usage{}, err
	}
	if err := json.Unmarshal([]byte(stored), &value); err != nil {
		return Usage{}, err
	}
	return value, nil
}

func (s *Service) SetUsage(ctx context.Context, value Usage, actorID string) error {
	if value.DailyTaskLimit < 0 || value.DailyTaskLimit > MaxDailyTaskLimit {
		return ErrInvalidUsage
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return s.store(ctx, usageSetting, string(encoded), actorID)
}

func (s *Service) EffectiveDailyTaskLimit(ctx context.Context, userID string, defaultLimit int) (int, error) {
	var limit int
	err := s.db.QueryRowContext(ctx, `SELECT daily_task_limit FROM user_daily_task_limits WHERE user_id = ? AND daily_task_limit IS NOT NULL`, userID).Scan(&limit)
	if errors.Is(err, sql.ErrNoRows) {
		return defaultLimit, nil
	}
	return limit, err
}

func (s *Service) ListUserDailyTaskLimits(ctx context.Context) (map[string]int, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT user_id, daily_task_limit FROM user_daily_task_limits WHERE daily_task_limit IS NOT NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[string]int)
	for rows.Next() {
		var userID string
		var limit int
		if err := rows.Scan(&userID, &limit); err != nil {
			return nil, err
		}
		result[userID] = limit
	}
	return result, rows.Err()
}

func (s *Service) SetUserDailyTaskLimit(ctx context.Context, userID string, limit *int, actorID string) error {
	if limit != nil && (*limit < 0 || *limit > MaxDailyTaskLimit) {
		return ErrInvalidUsage
	}
	result, err := s.db.ExecContext(ctx, `INSERT INTO user_daily_task_limits(user_id, daily_task_limit, updated_at, updated_by)
		SELECT id, ?, ?, ? FROM users WHERE id = ? AND is_cdk = 0
		ON CONFLICT(user_id) DO UPDATE SET daily_task_limit=excluded.daily_task_limit,
		updated_at=excluded.updated_at, updated_by=excluded.updated_by`, limit, time.Now().UTC().UnixMilli(), actorID, userID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err == nil && affected == 0 {
		return ErrUserNotFound
	}
	return err
}
