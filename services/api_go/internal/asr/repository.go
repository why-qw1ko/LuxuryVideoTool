package asr

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

type Repository struct{ db *sql.DB }
func NewRepository(db *sql.DB) *Repository { return &Repository{db: db} }
func (r *Repository) MonthlyEstimatedCost(ctx context.Context, month time.Time) (float64, error) {
	start := time.Date(month.UTC().Year(), month.UTC().Month(), 1, 0, 0, 0, 0, time.UTC); end := start.AddDate(0, 1, 0); var total float64
	err := r.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(estimated_cost_cny), 0) FROM asr_calls WHERE started_at >= ? AND started_at < ?`, start.UnixMilli(), end.UnixMilli()).Scan(&total); return total, err
}
func (r *Repository) DailyEstimatedCost(ctx context.Context, day time.Time) (float64, error) {
	start := time.Date(day.UTC().Year(), day.UTC().Month(), day.UTC().Day(), 0, 0, 0, 0, time.UTC); end := start.AddDate(0, 0, 1); var total float64
	err := r.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(estimated_cost_cny), 0) FROM asr_calls WHERE started_at >= ? AND started_at < ?`, start.UnixMilli(), end.UnixMilli()).Scan(&total); return total, err
}
func (r *Repository) Create(ctx context.Context, call Call) error { summary, _ := json.Marshal(call.Summary); _, err := r.db.ExecContext(ctx, `INSERT INTO asr_calls(id, job_id, provider, model, provider_request_id, provider_task_id, segment_index, audio_seconds, billed_seconds, estimated_cost_cny, status, response_summary_json, started_at, completed_at, error_code) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, call.ID, call.JobID, call.Provider, call.Model, nullString(call.ProviderRequestID), nil, call.SegmentIndex, call.AudioSeconds, nullFloat(call.BilledSeconds), call.EstimatedCostCNY, call.Status, string(summary), call.StartedAt.UTC().UnixMilli(), nil, nullString(call.ErrorCode)); if err != nil { return fmt.Errorf("create ASR call: %w", err) }; return nil }
func (r *Repository) SetProviderTask(ctx context.Context, id, taskID, requestID string) error { summary, _ := json.Marshal(map[string]any{"submitRequestId": requestID}); _, err := r.db.ExecContext(ctx, `UPDATE asr_calls SET provider_request_id=?, provider_task_id=?, status='running', response_summary_json=? WHERE id=?`, nullString(requestID), taskID, string(summary), id); return err }
func (r *Repository) Complete(ctx context.Context, id string, result TranscribeResult, cost float64, at time.Time) error { summary, _ := json.Marshal(result.Summary); _, err := r.db.ExecContext(ctx, `UPDATE asr_calls SET status='completed', provider_request_id=?, provider_task_id=?, billed_seconds=?, estimated_cost_cny=?, response_summary_json=?, completed_at=?, error_code=NULL WHERE id=?`, nullString(result.RequestID), nullString(result.ProviderTaskID), result.BilledSeconds, cost, string(summary), at.UTC().UnixMilli(), id); return err }
func (r *Repository) Fail(ctx context.Context, id, code string, summary map[string]any, at time.Time) error { encoded, _ := json.Marshal(summary); _, err := r.db.ExecContext(ctx, `UPDATE asr_calls SET status='failed', response_summary_json=?, completed_at=?, error_code=? WHERE id=?`, string(encoded), at.UTC().UnixMilli(), code, id); return err }
func nullString(value string) any { if value == "" { return nil }; return value }
func nullFloat(value float64) any { if value == 0 { return nil }; return value }
