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
	job, err := r.find(ctx, `j.user_id = ? AND j.id = ?`, userID, jobID); if err != nil { return Job{}, err }
	if job.Status == "completed" && job.Action == "download" { items, filesErr := r.FindFiles(ctx, userID, jobID); if filesErr != nil { return Job{}, filesErr }; job.Result = map[string]any{"files": items} }
	return job, nil
}

func (r *SQLiteRepository) CreateInfo(ctx context.Context, job Job) error {
	return r.create(ctx, job, true)
}

func (r *SQLiteRepository) CreateQueued(ctx context.Context, job Job) error { return r.create(ctx, job, false) }

func (r *SQLiteRepository) create(ctx context.Context, job Job, started bool) error {
	var startedAt any
	if started { startedAt = job.CreatedAt.UnixMilli() }
	_, err := r.db.ExecContext(ctx, `INSERT INTO jobs(id, user_id, input_text, input_url, action, status,
		progress, status_message, idempotency_key, force_refresh, created_at, updated_at, started_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, job.ID, job.UserID, job.InputText, job.InputURL,
		job.Action, job.Status, job.Progress, job.StatusMessage, job.IdempotencyKey, boolInt(job.ForceRefresh),
		job.CreatedAt.UnixMilli(), job.UpdatedAt.UnixMilli(), startedAt)
	if err != nil { return fmt.Errorf("create job: %w", err) }
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
		j.created_at, j.updated_at, j.completed_at, j.attempt_count, j.max_attempts, j.lease_owner,
		w.id, w.douyin_work_id, w.content_type, w.canonical_url, w.author_id, w.author_name,
		w.title, w.description, w.cover_url, w.published_at, w.metadata_json, w.resolver_name, w.resolver_version, w.resolved_at
		FROM jobs j LEFT JOIN works w ON w.id = j.work_id WHERE ` + where
	row := r.db.QueryRowContext(ctx, query, args...)
	var job Job
	var force int
	var errorCode, errorMessage sql.NullString
	var created, updated int64
	var completed sql.NullInt64
	var leaseOwner sql.NullString
	var workID, douyinID, kind, canonical, authorID, authorName, title, description, cover, metadata, resolverName, resolverVersion sql.NullString
	var published, resolved sql.NullInt64
	err := row.Scan(&job.ID, &job.UserID, &job.InputText, &job.InputURL, &job.Action, &job.Status, &job.Progress,
		&job.StatusMessage, &job.IdempotencyKey, &force, &errorCode, &errorMessage, &created, &updated, &completed,
		&job.AttemptCount, &job.MaxAttempts, &leaseOwner,
		&workID, &douyinID, &kind, &canonical, &authorID, &authorName, &title, &description, &cover, &published, &metadata,
		&resolverName, &resolverVersion, &resolved)
	if errors.Is(err, sql.ErrNoRows) { return Job{}, ErrNotFound }
	if err != nil { return Job{}, fmt.Errorf("find job: %w", err) }
	job.ForceRefresh, job.ErrorCode, job.ErrorMessage = force == 1, errorCode.String, errorMessage.String
	job.LeaseOwner = leaseOwner.String
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

func (r *SQLiteRepository) ClaimNext(ctx context.Context, owner string, now time.Time, lease time.Duration) (Job, error) {
	var id string
	err := r.db.QueryRowContext(ctx, `UPDATE jobs SET status = 'resolving', progress = 5, status_message = '正在读取作品信息',
		attempt_count = attempt_count + 1, lease_owner = ?, lease_expires_at = ?, heartbeat_at = ?,
		started_at = COALESCE(started_at, ?), retry_at = NULL, updated_at = ?
		WHERE id = (SELECT id FROM jobs WHERE (status = 'queued' OR (status = 'retry_wait' AND retry_at <= ?))
		AND attempt_count < max_attempts ORDER BY created_at LIMIT 1) AND (status = 'queued' OR status = 'retry_wait') RETURNING id`,
		owner, now.Add(lease).UnixMilli(), now.UnixMilli(), now.UnixMilli(), now.UnixMilli(), now.UnixMilli()).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) { return Job{}, ErrNotFound }
	if err != nil { return Job{}, fmt.Errorf("claim queued job: %w", err) }
	return r.find(ctx, `j.id = ?`, id)
}

func (r *SQLiteRepository) Heartbeat(ctx context.Context, jobID, owner string, now time.Time, lease time.Duration) error {
	result, err := r.db.ExecContext(ctx, `UPDATE jobs SET heartbeat_at = ?, lease_expires_at = ?, updated_at = ?
		WHERE id = ? AND lease_owner = ? AND status IN ('resolving','downloading','extracting','transcribing','postprocessing')`,
		now.UnixMilli(), now.Add(lease).UnixMilli(), now.UnixMilli(), jobID, owner)
	if err != nil { return fmt.Errorf("heartbeat job: %w", err) }; if changed(result) != nil { return ErrLeaseLost }; return nil
}

func (r *SQLiteRepository) SetResolved(ctx context.Context, jobID, owner string, work resolver.Work, at time.Time) error {
	result, err := r.db.ExecContext(ctx, `UPDATE jobs SET work_id = ?, status = 'downloading', progress = 30,
		status_message = '正在下载媒体', updated_at = ? WHERE id = ? AND lease_owner = ? AND status = 'resolving'`, work.ID, at.UnixMilli(), jobID, owner)
	if err != nil { return fmt.Errorf("set job resolved: %w", err) }; if changed(result) != nil { return ErrLeaseLost }; return nil
}

func (r *SQLiteRepository) SetStage(ctx context.Context, jobID, owner, status string, progress int, message string, at time.Time) error {
	result, err := r.db.ExecContext(ctx, `UPDATE jobs SET status = ?, progress = ?, status_message = ?, updated_at = ?
		WHERE id = ? AND lease_owner = ?`, status, progress, message, at.UnixMilli(), jobID, owner)
	if err != nil { return fmt.Errorf("set job stage: %w", err) }; if changed(result) != nil { return ErrLeaseLost }; return nil
}

func (r *SQLiteRepository) CompleteDownload(ctx context.Context, jobID, owner, fileID string, at time.Time) error {
	result, err := r.db.ExecContext(ctx, `UPDATE jobs SET status = 'completed', progress = 100, status_message = '媒体下载完成',
		completed_at = ?, updated_at = ?, lease_owner = NULL, lease_expires_at = NULL, heartbeat_at = NULL,
		error_code = NULL, error_message = NULL WHERE id = ? AND lease_owner = ? AND status = 'downloading'`, at.UnixMilli(), at.UnixMilli(), jobID, owner)
	if err != nil { return fmt.Errorf("complete download job %s: %w", fileID, err) }; if changed(result) != nil { return ErrLeaseLost }; return nil
}

func (r *SQLiteRepository) RetryLater(ctx context.Context, jobID, owner, code, message string, retryAt, at time.Time) error {
	result, err := r.db.ExecContext(ctx, `UPDATE jobs SET status = CASE WHEN attempt_count >= max_attempts THEN 'failed' ELSE 'retry_wait' END,
		progress = CASE WHEN attempt_count >= max_attempts THEN 100 ELSE progress END, status_message = ?, error_code = ?, error_message = ?,
		retry_at = CASE WHEN attempt_count >= max_attempts THEN NULL ELSE ? END, completed_at = CASE WHEN attempt_count >= max_attempts THEN ? ELSE NULL END,
		lease_owner = NULL, lease_expires_at = NULL, heartbeat_at = NULL, updated_at = ? WHERE id = ? AND lease_owner = ?`,
		message, code, message, retryAt.UnixMilli(), at.UnixMilli(), at.UnixMilli(), jobID, owner)
	if err != nil { return fmt.Errorf("schedule job retry: %w", err) }; if changed(result) != nil { return ErrLeaseLost }; return nil
}

func (r *SQLiteRepository) CancelOwned(ctx context.Context, jobID, owner, message string, at time.Time) error {
	result, err := r.db.ExecContext(ctx, `UPDATE jobs SET status = 'cancelled', progress = 100, status_message = ?,
		cancel_requested_at = ?, completed_at = ?, lease_owner = NULL, lease_expires_at = NULL, heartbeat_at = NULL, updated_at = ?
		WHERE id = ? AND lease_owner = ?`, message, at.UnixMilli(), at.UnixMilli(), at.UnixMilli(), jobID, owner)
	if err != nil { return fmt.Errorf("cancel owned job: %w", err) }; if changed(result) != nil { return ErrLeaseLost }; return nil
}

func (r *SQLiteRepository) Cancel(ctx context.Context, userID, jobID string, at time.Time) (bool, error) {
	result, err := r.db.ExecContext(ctx, `UPDATE jobs SET status = 'cancelled', progress = 100, status_message = '任务已取消',
		cancel_requested_at = ?, completed_at = ?, updated_at = ?, lease_owner = NULL, lease_expires_at = NULL
		WHERE id = ? AND user_id = ? AND status IN ('queued','retry_wait')`, at.UnixMilli(), at.UnixMilli(), at.UnixMilli(), jobID, userID)
	if err != nil { return false, fmt.Errorf("cancel job: %w", err) }
	count, err := result.RowsAffected(); if err != nil { return false, err }; if count > 0 { return true, nil }
	var exists int; var status string
	err = r.db.QueryRowContext(ctx, `SELECT 1, status FROM jobs WHERE id = ? AND user_id = ?`, jobID, userID).Scan(&exists, &status)
	if errors.Is(err, sql.ErrNoRows) { return false, ErrNotFound }; if err != nil { return false, err }
	return false, ErrNotCancellable
}

func (r *SQLiteRepository) Retry(ctx context.Context, userID, jobID string, at time.Time) error {
	result, err := r.db.ExecContext(ctx, `UPDATE jobs SET status = 'queued', progress = 0, status_message = '等待处理',
		attempt_count = 0, error_code = NULL, error_message = NULL, completed_at = NULL, retry_at = NULL, cancel_requested_at = NULL, updated_at = ?
		WHERE id = ? AND user_id = ? AND status IN ('failed','cancelled')`, at.UnixMilli(), jobID, userID)
	if err != nil { return fmt.Errorf("retry job: %w", err) }
	if changed(result) != nil { if _, findErr := r.FindByID(ctx, userID, jobID); errors.Is(findErr, ErrNotFound) { return ErrNotFound }; return ErrNotRetryable }; return nil
}

func (r *SQLiteRepository) Recover(ctx context.Context, now time.Time) (int64, error) {
	result, err := r.db.ExecContext(ctx, `UPDATE jobs SET status = CASE WHEN attempt_count >= max_attempts THEN 'failed' ELSE 'queued' END,
		progress = CASE WHEN attempt_count >= max_attempts THEN 100 ELSE 0 END,
		status_message = CASE WHEN attempt_count >= max_attempts THEN '任务恢复次数已耗尽' ELSE '服务恢复后重新排队' END,
		error_code = CASE WHEN attempt_count >= max_attempts THEN 'JOB_RECOVERY_EXHAUSTED' ELSE error_code END,
		error_message = CASE WHEN attempt_count >= max_attempts THEN '任务恢复次数已耗尽' ELSE error_message END,
		completed_at = CASE WHEN attempt_count >= max_attempts THEN ? ELSE NULL END,
		lease_owner = NULL, lease_expires_at = NULL, heartbeat_at = NULL, updated_at = ?
		WHERE status IN ('resolving','downloading','extracting','postprocessing') AND lease_expires_at < ?`, now.UnixMilli(), now.UnixMilli(), now.UnixMilli())
	if err != nil { return 0, fmt.Errorf("recover expired jobs: %w", err) }; return result.RowsAffected()
}

func (r *SQLiteRepository) FailExhaustedQueued(ctx context.Context, at time.Time) (int64, error) {
	result, err := r.db.ExecContext(ctx, `UPDATE jobs SET status = 'failed', progress = 100, status_message = '任务重试次数已耗尽',
		error_code = 'JOB_RETRY_EXHAUSTED', error_message = '任务重试次数已耗尽', completed_at = ?, updated_at = ?
		WHERE status IN ('queued','retry_wait') AND attempt_count >= max_attempts`, at.UnixMilli(), at.UnixMilli())
	if err != nil { return 0, fmt.Errorf("fail exhausted queued jobs: %w", err) }; return result.RowsAffected()
}

func (r *SQLiteRepository) BeginStep(ctx context.Context, step Step) error {
	details, err := json.Marshal(step.Details); if err != nil { return err }
	_, err = r.db.ExecContext(ctx, `INSERT INTO job_steps(id, job_id, step_name, attempt, status, started_at, details_json)
		VALUES (?, ?, ?, ?, 'running', ?, ?)`, step.ID, step.JobID, step.Name, step.Attempt, step.StartedAt.UnixMilli(), string(details))
	if err != nil { return fmt.Errorf("begin job step: %w", err) }; return nil
}

func (r *SQLiteRepository) FinishStep(ctx context.Context, stepID, status, code, message string, details map[string]any, at time.Time) error {
	encoded, err := json.Marshal(details); if err != nil { return err }
	result, err := r.db.ExecContext(ctx, `UPDATE job_steps SET status = ?, completed_at = ?, details_json = ?, error_code = ?, error_message = ? WHERE id = ? AND status = 'running'`,
		status, at.UnixMilli(), string(encoded), nullText(code), nullText(message), stepID)
	if err != nil { return fmt.Errorf("finish job step: %w", err) }; return changed(result)
}

func (r *SQLiteRepository) FindFiles(ctx context.Context, userID, jobID string) ([]JobFile, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, kind, original_name, mime_type, size_bytes FROM files WHERE user_id = ? AND job_id = ? AND deleted_at IS NULL ORDER BY created_at`, userID, jobID)
	if err != nil { return nil, fmt.Errorf("find job files: %w", err) }; defer rows.Close(); var items []JobFile
	for rows.Next() { var item JobFile; if err := rows.Scan(&item.ID, &item.Kind, &item.Name, &item.MIMEType, &item.SizeBytes); err != nil { return nil, err }; items = append(items, item) }; return items, rows.Err()
}

func (r *SQLiteRepository) DeleteFileRecord(ctx context.Context, fileID string) error { _, err := r.db.ExecContext(ctx, `DELETE FROM files WHERE id = ?`, fileID); return err }

func changed(result sql.Result) error { count, err := result.RowsAffected(); if err != nil { return err }; if count == 0 { return ErrNotFound }; return nil }
func boolInt(value bool) int { if value { return 1 }; return 0 }
func nullText(value string) any { if value == "" { return nil }; return value }
