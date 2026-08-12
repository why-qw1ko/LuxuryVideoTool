package jobs

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/resolver"
)

type SQLiteRepository struct{ db *sql.DB }
func NewSQLiteRepository(db *sql.DB) *SQLiteRepository { return &SQLiteRepository{db: db} }

func (r *SQLiteRepository) FindByIdempotencyKey(ctx context.Context, userID, key string) (Job, error) {
	return r.find(ctx, `j.user_id = ? AND j.idempotency_key = ?`, userID, key)
}
func (r *SQLiteRepository) FindByID(ctx context.Context, userID, jobID string) (Job, error) {
	return r.find(ctx, `j.user_id = ? AND j.id = ?`, userID, jobID)
}

func (r *SQLiteRepository) CreateInfo(ctx context.Context, job Job) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO jobs(id, user_id, input_text, input_url, action, status,
		progress, status_message, idempotency_key, force_refresh, created_at, updated_at, started_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, job.ID, job.UserID, job.InputText, job.InputURL,
		job.Action, job.Status, job.Progress, job.StatusMessage, job.IdempotencyKey, boolInt(job.ForceRefresh),
		job.CreatedAt.UnixMilli(), job.UpdatedAt.UnixMilli(), job.CreatedAt.UnixMilli())
	if err != nil { return fmt.Errorf("create info job: %w", err) }
	return nil
}

func (r *SQLiteRepository) CompleteInfo(ctx context.Context, jobID string, work resolver.Work, at time.Time) error {
	result, err := r.db.ExecContext(ctx, `UPDATE jobs SET work_id = ?, status = 'completed', progress = 100,
		status_message = '作品信息解析完成', completed_at = ?, updated_at = ? WHERE id = ? AND status = 'resolving'`,
		work.ID, at.UnixMilli(), at.UnixMilli(), jobID)
	if err != nil { return fmt.Errorf("complete info job: %w", err) }
	return changed(result)
}

func (r *SQLiteRepository) Fail(ctx context.Context, jobID, code, message string, at time.Time) error {
	result, err := r.db.ExecContext(ctx, `UPDATE jobs SET status = 'failed', progress = 100, status_message = ?,
		error_code = ?, error_message = ?, completed_at = ?, updated_at = ? WHERE id = ?`, message, code, message,
		at.UnixMilli(), at.UnixMilli(), jobID)
	if err != nil { return fmt.Errorf("fail info job: %w", err) }
	return changed(result)
}

func (r *SQLiteRepository) find(ctx context.Context, where string, args ...any) (Job, error) {
	query := `SELECT j.id, j.user_id, j.input_text, j.input_url, j.action, j.status, j.progress,
		j.status_message, j.idempotency_key, j.force_refresh, j.error_code, j.error_message,
		j.created_at, j.updated_at, j.completed_at,
		w.id, w.douyin_work_id, w.content_type, w.canonical_url, w.author_id, w.author_name,
		w.title, w.description, w.cover_url, w.published_at, w.metadata_json, w.resolver_name, w.resolver_version, w.resolved_at
		FROM jobs j LEFT JOIN works w ON w.id = j.work_id WHERE ` + where
	row := r.db.QueryRowContext(ctx, query, args...)
	var job Job
	var force int
	var errorCode, errorMessage sql.NullString
	var created, updated int64
	var completed sql.NullInt64
	var workID, douyinID, kind, canonical, authorID, authorName, title, description, cover, metadata, resolverName, resolverVersion sql.NullString
	var published, resolved sql.NullInt64
	err := row.Scan(&job.ID, &job.UserID, &job.InputText, &job.InputURL, &job.Action, &job.Status, &job.Progress,
		&job.StatusMessage, &job.IdempotencyKey, &force, &errorCode, &errorMessage, &created, &updated, &completed,
		&workID, &douyinID, &kind, &canonical, &authorID, &authorName, &title, &description, &cover, &published, &metadata,
		&resolverName, &resolverVersion, &resolved)
	if errors.Is(err, sql.ErrNoRows) { return Job{}, ErrNotFound }
	if err != nil { return Job{}, fmt.Errorf("find job: %w", err) }
	job.ForceRefresh, job.ErrorCode, job.ErrorMessage = force == 1, errorCode.String, errorMessage.String
	if errorCode.Valid { job.Error = &JobError{Code: errorCode.String, Message: errorMessage.String} }
	job.CreatedAt, job.UpdatedAt = time.UnixMilli(created).UTC(), time.UnixMilli(updated).UTC()
	if completed.Valid { value := time.UnixMilli(completed.Int64).UTC(); job.CompletedAt = &value }
	if workID.Valid {
		work := resolver.Work{ID: workID.String, DouyinWorkID: douyinID.String, Type: kind.String, CanonicalURL: canonical.String,
			AuthorID: authorID.String, AuthorName: authorName.String, Title: title.String, Description: description.String,
			CoverURL: cover.String, ResolverName: resolverName.String, ResolverVersion: resolverVersion.String,
			ResolvedAt: time.UnixMilli(resolved.Int64).UTC()}
		if published.Valid { value := time.UnixMilli(published.Int64).UTC(); work.PublishedAt = &value }
		var extra struct {
			VideoURL string `json:"videoUrl"`
			DurationMS int64 `json:"durationMs"`
			Width int `json:"width"`
			Height int `json:"height"`
			Images []resolver.Image `json:"images"`
			Hashtags []string `json:"hashtags"`
		}
		if metadata.Valid && json.Unmarshal([]byte(metadata.String), &extra) == nil {
			work.VideoURL, work.DurationMS, work.Width, work.Height = extra.VideoURL, extra.DurationMS, extra.Width, extra.Height
			work.Images, work.Hashtags = extra.Images, extra.Hashtags
		}
		job.Work = &work
		if job.Status == "completed" { job.Result = map[string]any{"workId": work.ID} }
	}
	return job, nil
}

func changed(result sql.Result) error { count, err := result.RowsAffected(); if err != nil { return err }; if count == 0 { return ErrNotFound }; return nil }
func boolInt(value bool) int { if value { return 1 }; return 0 }
