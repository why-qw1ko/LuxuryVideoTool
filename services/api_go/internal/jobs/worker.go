package jobs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/auth"
	ownedfiles "github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/files"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/media"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/resolver"
)

type WorkerConfig struct {
	Owner string
	Concurrency int
	Lease time.Duration
	Heartbeat time.Duration
	Poll time.Duration
	MaxVideoBytes int64
	VideoRetention time.Duration
}

type Worker struct {
	repo Repository
	resolver *resolver.Service
	downloader media.Downloader
	fileRepo *ownedfiles.Repository
	storage *ownedfiles.Storage
	config WorkerConfig
	now func() time.Time
	stopMu sync.Mutex
	stops map[string]context.CancelFunc
}

func NewWorker(repo Repository, resolverService *resolver.Service, downloader media.Downloader, fileRepo *ownedfiles.Repository, storage *ownedfiles.Storage, config WorkerConfig) *Worker {
	if config.Concurrency < 1 { config.Concurrency = 1 }; if config.Concurrency > 2 { config.Concurrency = 2 }
	if config.Lease <= 0 { config.Lease = 60*time.Second }; if config.Heartbeat <= 0 { config.Heartbeat = 15*time.Second }; if config.Poll <= 0 { config.Poll = time.Second }
	return &Worker{repo: repo, resolver: resolverService, downloader: downloader, fileRepo: fileRepo, storage: storage, config: config, now: time.Now, stops: make(map[string]context.CancelFunc)}
}

func (w *Worker) Run(ctx context.Context) error {
	if _, err := w.repo.Recover(ctx, w.now().UTC()); err != nil { return err }
	if _, err := w.repo.FailExhaustedQueued(ctx, w.now().UTC()); err != nil { return err }
	var group sync.WaitGroup; group.Add(w.config.Concurrency)
	for i := 0; i < w.config.Concurrency; i++ { go func(index int) { defer group.Done(); w.loop(ctx, fmt.Sprintf("%s-%d", w.config.Owner, index+1)) }(i) }
	<-ctx.Done(); group.Wait(); return nil
}

func (w *Worker) loop(ctx context.Context, owner string) {
	for {
		job, err := w.repo.ClaimNext(ctx, owner, w.now().UTC(), w.config.Lease)
		if err == nil { if processErr := w.process(ctx, owner, job); processErr != nil { slog.ErrorContext(ctx, "job processing failed", "job_id", job.ID, "event", "job_failed", "error_code", classify(processErr)) }; continue }
		if !errors.Is(err, ErrNotFound) { slog.ErrorContext(ctx, "job claim failed", "event", "job_claim_failed", "error", err) }
		timer := time.NewTimer(w.config.Poll); select { case <-ctx.Done(): if !timer.Stop() { <-timer.C }; return; case <-timer.C: }
	}
}

func (w *Worker) process(ctx context.Context, owner string, job Job) (resultErr error) {
	jobCtx, cancel := context.WithCancel(ctx); defer cancel()
	defer func() { if resultErr != nil && errors.Is(resultErr, context.Canceled) && ctx.Err() == nil { _ = w.repo.CancelOwned(context.Background(), job.ID, owner, "任务已取消", w.now().UTC()) } }()
	w.stopMu.Lock(); w.stops[job.ID] = cancel; w.stopMu.Unlock(); defer func() { w.stopMu.Lock(); delete(w.stops, job.ID); w.stopMu.Unlock() }()
	heartbeatDone := make(chan struct{}); go w.heartbeat(jobCtx, cancel, heartbeatDone, job.ID, owner); defer func() { cancel(); <-heartbeatDone }()
	resolveStepID, err := auth.NewID(w.now()); if err != nil { return err }
	if err := w.repo.BeginStep(jobCtx, Step{ID: resolveStepID, JobID: job.ID, Name: "resolve", Attempt: job.AttemptCount, StartedAt: w.now().UTC(), Details: map[string]any{}}); err != nil { return err }
	work, _, err := w.resolver.Resolve(jobCtx, job.InputText, job.ForceRefresh)
	if err != nil { code, message := mediaError(err); _ = w.repo.FinishStep(context.Background(), resolveStepID, "failed", code, message, map[string]any{}, w.now().UTC()); if errors.Is(err, context.Canceled) { _ = w.repo.CancelOwned(context.Background(), job.ID, owner, "任务已取消", w.now().UTC()); return err }; return w.failed(job, owner, err) }
	if err := w.repo.FinishStep(jobCtx, resolveStepID, "completed", "", "", map[string]any{"workId": work.ID}, w.now().UTC()); err != nil { return err }
	if err := w.repo.SetResolved(jobCtx, job.ID, owner, work, w.now().UTC()); err != nil { return err }
	if work.Type != "video" || work.VideoURL == "" { return w.failed(job, owner, resolver.ErrResolveFailed) }
	stepID, err := auth.NewID(w.now()); if err != nil { return err }
	step := Step{ID: stepID, JobID: job.ID, Name: "download", Attempt: job.AttemptCount, StartedAt: w.now().UTC(), Details: map[string]any{}}
	if err := w.repo.BeginStep(jobCtx, step); err != nil { return err }
	relative, temporary, final, err := w.storage.NewTarget(job.UserID, job.ID, ".mp4"); if err != nil { return w.finishFailed(job, owner, stepID, err) }
	result, err := w.downloader.Download(jobCtx, work.VideoURL, temporary, final, w.config.MaxVideoBytes)
	if err != nil { return w.finishFailed(job, owner, stepID, err) }
	if jobCtx.Err() != nil { _ = w.storage.Remove(ownedfiles.File{RelativePath: relative}); return w.finishFailed(job, owner, stepID, jobCtx.Err()) }
	now := w.now().UTC(); expires := now.Add(w.config.VideoRetention)
	file, err := ownedfiles.NewFile(now, job.UserID, job.ID, "video", relative, safeMediaName(work.DouyinWorkID)+".mp4", result.MIMEType, result.SHA256, result.SizeBytes, &expires)
	if err != nil { return w.finishFailed(job, owner, stepID, err) }
	if err := w.fileRepo.Create(jobCtx, file); err != nil { return w.finishFailed(job, owner, stepID, err) }
	if err := w.repo.FinishStep(jobCtx, stepID, "completed", "", "", map[string]any{"fileId": file.ID, "sizeBytes": file.SizeBytes, "sha256": file.SHA256}, now); err != nil { w.rollbackFile(file); return err }
	if err := w.repo.CompleteDownload(jobCtx, job.ID, owner, file.ID, now); err != nil { w.rollbackFile(file); return err }; return nil
}

func (w *Worker) Cancel(jobID string) bool { w.stopMu.Lock(); cancel := w.stops[jobID]; w.stopMu.Unlock(); if cancel == nil { return false }; cancel(); return true }

func (w *Worker) heartbeat(ctx context.Context, cancel context.CancelFunc, done chan<- struct{}, jobID, owner string) {
	defer close(done); ticker := time.NewTicker(w.config.Heartbeat); defer ticker.Stop()
	for { select { case <-ctx.Done(): return; case <-ticker.C: if err := w.repo.Heartbeat(context.WithoutCancel(ctx), jobID, owner, w.now().UTC(), w.config.Lease); err != nil { cancel(); return } } }
}

func (w *Worker) finishFailed(job Job, owner, stepID string, err error) error {
	now := w.now().UTC(); code, message := mediaError(err); _ = w.repo.FinishStep(context.Background(), stepID, "failed", code, message, map[string]any{}, now); return w.failedWith(job, owner, code, message, err)
}
func (w *Worker) failed(job Job, owner string, err error) error { code, message := mediaError(err); return w.failedWith(job, owner, code, message, err) }
func (w *Worker) failedWith(job Job, owner, code, message string, cause error) error {
	if errors.Is(cause, context.Canceled) {
		if persistErr := w.repo.CancelOwned(context.Background(), job.ID, owner, "任务已取消", w.now().UTC()); persistErr != nil { return errors.Join(cause, persistErr) }; return cause
	}
	delay := time.Duration(1<<min(job.AttemptCount, 6))*time.Second + time.Duration(rand.IntN(1000))*time.Millisecond
	if persistErr := w.repo.RetryLater(context.Background(), job.ID, owner, code, message, w.now().UTC().Add(delay), w.now().UTC()); persistErr != nil { return errors.Join(cause, persistErr) }; return cause
}

func (w *Worker) rollbackFile(file ownedfiles.File) { _ = w.storage.Remove(file); _ = w.repo.DeleteFileRecord(context.Background(), file.ID) }

func mediaError(err error) (string, string) {
	switch { case errors.Is(err, media.ErrTooLarge): return "MEDIA_TOO_LARGE", "视频超过允许的大小"; case errors.Is(err, resolver.ErrURLNotAllowed): return "URL_NOT_ALLOWED", "媒体地址不允许访问"; case errors.Is(err, resolver.ErrWorkUnavailable): return "DOUYIN_WORK_UNAVAILABLE", "作品不存在或不可访问"; case errors.Is(err, resolver.ErrResolveFailed), errors.Is(err, resolver.ErrInvalidShareLink): return "DOUYIN_RESOLVE_FAILED", "无法解析该作品"; default: return "MEDIA_DOWNLOAD_FAILED", "媒体下载失败" }
}
func classify(err error) string { code, _ := mediaError(err); return code }
func safeMediaName(value string) string { value = filepath.Base(strings.TrimSpace(value)); if value == "." || value == "" { return "douyin-video" }; return value }
