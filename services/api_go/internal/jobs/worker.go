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

	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/asr"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/auth"
	ownedfiles "github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/files"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/media"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/resolver"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/results"
)

type WorkerConfig struct {
	Owner          string
	Concurrency    int
	Lease          time.Duration
	Heartbeat      time.Duration
	Poll           time.Duration
	MaxVideoBytes  int64
	VideoRetention time.Duration
}

type Worker struct {
	repo        Repository
	resolver    *resolver.Service
	downloader  media.Downloader
	fileRepo    *ownedfiles.Repository
	storage     *ownedfiles.Storage
	transcriber *Transcriber
	config      WorkerConfig
	now         func() time.Time
	stopMu      sync.Mutex
	stops       map[string]context.CancelFunc
}

func NewWorker(repo Repository, resolverService *resolver.Service, downloader media.Downloader, fileRepo *ownedfiles.Repository, storage *ownedfiles.Storage, transcriber *Transcriber, config WorkerConfig) *Worker {
	if config.Concurrency < 1 {
		config.Concurrency = 1
	}
	if config.Concurrency > 2 {
		config.Concurrency = 2
	}
	if config.Lease <= 0 {
		config.Lease = 60 * time.Second
	}
	if config.Heartbeat <= 0 {
		config.Heartbeat = 15 * time.Second
	}
	if config.Poll <= 0 {
		config.Poll = time.Second
	}
	return &Worker{repo: repo, resolver: resolverService, downloader: downloader, fileRepo: fileRepo, storage: storage, transcriber: transcriber, config: config, now: time.Now, stops: make(map[string]context.CancelFunc)}
}

func (w *Worker) Run(ctx context.Context) error {
	if _, err := w.repo.Recover(ctx, w.now().UTC()); err != nil {
		return err
	}
	if _, err := w.repo.FailExhaustedQueued(ctx, w.now().UTC()); err != nil {
		return err
	}
	var group sync.WaitGroup
	group.Add(w.config.Concurrency)
	for i := 0; i < w.config.Concurrency; i++ {
		go func(index int) { defer group.Done(); w.loop(ctx, fmt.Sprintf("%s-%d", w.config.Owner, index+1)) }(i)
	}
	<-ctx.Done()
	group.Wait()
	return nil
}

func (w *Worker) loop(ctx context.Context, owner string) {
	for {
		job, err := w.repo.ClaimNext(ctx, owner, w.now().UTC(), w.config.Lease)
		if err == nil {
			if processErr := w.process(ctx, owner, job); processErr != nil {
				slog.ErrorContext(ctx, "job processing failed", "job_id", job.ID, "event", "job_failed", "error_code", classify(processErr), "error", processErr)
			}
			continue
		}
		if !errors.Is(err, ErrNotFound) {
			slog.ErrorContext(ctx, "job claim failed", "event", "job_claim_failed", "error", err)
		}
		timer := time.NewTimer(w.config.Poll)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return
		case <-timer.C:
		}
	}
}

func (w *Worker) process(ctx context.Context, owner string, job Job) (resultErr error) {
	jobCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	defer func() {
		if resultErr != nil && errors.Is(resultErr, context.Canceled) && ctx.Err() == nil {
			_ = w.repo.CancelOwned(context.Background(), job.ID, owner, "任务已取消", w.now().UTC())
		}
	}()
	w.stopMu.Lock()
	w.stops[job.ID] = cancel
	w.stopMu.Unlock()
	defer func() { w.stopMu.Lock(); delete(w.stops, job.ID); w.stopMu.Unlock() }()
	heartbeatDone := make(chan struct{})
	go w.heartbeat(jobCtx, cancel, heartbeatDone, job.ID, owner)
	defer func() { cancel(); <-heartbeatDone }()
	resolveStepID, err := auth.NewID(w.now())
	if err != nil {
		return err
	}
	if err := w.repo.BeginStep(jobCtx, Step{ID: resolveStepID, JobID: job.ID, Name: "resolve", Attempt: job.AttemptCount, StartedAt: w.now().UTC(), Details: map[string]any{}}); err != nil {
		return err
	}
	work, _, err := w.resolver.Resolve(jobCtx, job.InputText, job.ForceRefresh)
	if err != nil {
		code, message := mediaError(err)
		_ = w.repo.FinishStep(context.Background(), resolveStepID, "failed", code, message, map[string]any{}, w.now().UTC())
		if errors.Is(err, context.Canceled) {
			_ = w.repo.CancelOwned(context.Background(), job.ID, owner, "任务已取消", w.now().UTC())
			return err
		}
		return w.failed(job, owner, err)
	}
	if err := w.repo.FinishStep(jobCtx, resolveStepID, "completed", "", "", map[string]any{"workId": work.ID}, w.now().UTC()); err != nil {
		return err
	}
	if err := w.repo.SetResolved(jobCtx, job.ID, owner, work, w.now().UTC()); err != nil {
		return err
	}
	if work.Type != "video" || work.VideoURL == "" {
		if work.Type == "note" {
			return w.processNote(jobCtx, owner, job, work)
		}
		return w.failed(job, owner, resolver.ErrVideoRequired)
	}
	stepID, err := auth.NewID(w.now())
	if err != nil {
		return err
	}
	step := Step{ID: stepID, JobID: job.ID, Name: "download", Attempt: job.AttemptCount, StartedAt: w.now().UTC(), Details: map[string]any{}}
	if err := w.repo.BeginStep(jobCtx, step); err != nil {
		return err
	}
	relative, temporary, final, err := w.storage.NewTarget(job.UserID, job.ID, ".mp4")
	if err != nil {
		return w.finishFailed(job, owner, stepID, err)
	}
	result, err := w.downloader.Download(jobCtx, work.VideoURL, temporary, final, w.config.MaxVideoBytes)
	if err != nil {
		return w.finishFailed(job, owner, stepID, err)
	}
	if jobCtx.Err() != nil {
		_ = w.storage.Remove(ownedfiles.File{RelativePath: relative})
		return w.finishFailed(job, owner, stepID, jobCtx.Err())
	}
	now := w.now().UTC()
	expires := now.Add(w.config.VideoRetention)
	file, err := ownedfiles.NewFile(now, job.UserID, job.ID, "video", relative, safeMediaName(work.DouyinWorkID)+".mp4", result.MIMEType, result.SHA256, result.SizeBytes, &expires)
	if err != nil {
		return w.finishFailed(job, owner, stepID, err)
	}
	if err := w.fileRepo.Create(jobCtx, file); err != nil {
		return w.finishFailed(job, owner, stepID, err)
	}
	if err := w.repo.FinishStep(jobCtx, stepID, "completed", "", "", map[string]any{"fileId": file.ID, "sizeBytes": file.SizeBytes, "sha256": file.SHA256}, now); err != nil {
		w.rollbackFile(file)
		return err
	}
	if job.Action == "download" {
		if err := w.repo.CompleteDownload(jobCtx, job.ID, owner, file.ID, now); err != nil {
			w.rollbackFile(file)
			return err
		}
		return nil
	}
	if w.transcriber == nil {
		return w.finishFailed(job, owner, stepID, asr.ErrAuth)
	}
	job.Work = &work
	if err := w.transcriber.Run(jobCtx, w.repo, job, owner, file); err != nil {
		return w.failed(job, owner, err)
	}
	if job.Action == "transcribe" && !job.KeepVideo {
		_ = w.storage.Remove(file)
		_ = w.fileRepo.MarkDeleted(context.Background(), file.ID, w.now().UTC())
	}
	return nil
}

// processNote 处理图文/动图作品。统一行为：凡是进入 worker 的操作（download/transcribe/full）
// 都下载配图（动图下载动态版 MP4）并以配文（desc）生成结果，无需 ASR；
// 纯解析（info）走同步路径，不经过这里。
func (w *Worker) processNote(ctx context.Context, owner string, job Job, work resolver.Work) error {
	job.Work = &work
	now := w.now().UTC()
	// 上一尝试（失败/取消/失租）可能残留配图与结果文件；先清空，避免重试后画廊、ZIP、下载链接混入过期副本。
	// 用 Background context，确保即使本次已取消也能完成清理。
	if stale, err := w.fileRepo.ListByJob(context.Background(), job.UserID, job.ID); err == nil {
		for _, file := range stale {
			_ = w.storage.Remove(file)
			_ = w.repo.DeleteFileRecord(context.Background(), file.ID)
		}
	}
	if len(work.Images) > 0 {
		stepID, err := auth.NewID(now)
		if err != nil {
			return err
		}
		if err := w.repo.BeginStep(ctx, Step{ID: stepID, JobID: job.ID, Name: "download", Attempt: job.AttemptCount, StartedAt: now, Details: map[string]any{}}); err != nil {
			return err
		}
		expires := now.Add(w.config.VideoRetention)
		downloaded := 0
		for index, img := range work.Images {
			if ctx.Err() != nil {
				_ = w.repo.FinishStep(context.Background(), stepID, "cancelled", "", "任务已取消", map[string]any{}, now)
				return ctx.Err()
			}
			url, ext, kind, name := img.URL, ".jpg", "image", fmt.Sprintf("image-%02d.jpg", index+1)
			if img.AnimatedURL != "" {
				url, ext, kind, name = img.AnimatedURL, ".mp4", "animated", fmt.Sprintf("image-%02d.mp4", index+1)
			}
			if url == "" {
				continue
			}
			if err := w.repo.SetStage(ctx, job.ID, owner, "downloading", 30+index*45/len(work.Images), fmt.Sprintf("正在下载配图 %d/%d", index+1, len(work.Images)), now); err != nil {
				if ctx.Err() != nil {
					// 取消竞态（循环顶部检查之后才取消）：步骤标 cancelled 而非 failed，与任务状态一致。
					_ = w.repo.FinishStep(context.Background(), stepID, "cancelled", "", "任务已取消", map[string]any{}, now)
					return ctx.Err()
				}
				// 中途失租/其他错误时收尾 download 步骤，避免留下永久 running 的行。
				return w.finishFailed(job, owner, stepID, err)
			}
			relative, temporary, final, err := w.storage.NewScopedTarget("images", job.UserID, job.ID, ext)
			if err != nil {
				continue
			}
			// 单张配图较短超时，避免外部 CDN 卡死导致任务一直挂着；失败跳过，全部失败才报错。
			fileCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
			result, err := w.downloader.Download(fileCtx, url, temporary, final, w.config.MaxVideoBytes)
			cancel()
			if err != nil {
				_ = w.storage.Remove(ownedfiles.File{RelativePath: relative})
				if ctx.Err() != nil {
					_ = w.repo.FinishStep(context.Background(), stepID, "cancelled", "", "任务已取消", map[string]any{}, now)
					return ctx.Err()
				}
				slog.WarnContext(ctx, "note image download failed, skipping", "job_id", job.ID, "index", index+1, "error", err)
				continue
			}
			// 每张图用递增时间戳创建文件记录，确保按图片顺序返回（created_at + ULID 均可排序）。
			file, err := ownedfiles.NewFile(now.Add(time.Duration(index)*time.Millisecond), job.UserID, job.ID, kind, relative, name, result.MIMEType, result.SHA256, result.SizeBytes, &expires)
			if err != nil {
				_ = w.storage.Remove(ownedfiles.File{RelativePath: relative})
				continue
			}
			if err := w.fileRepo.Create(ctx, file); err != nil {
				_ = w.storage.Remove(ownedfiles.File{RelativePath: relative})
				continue
			}
			downloaded++
		}
		if downloaded == 0 {
			return w.finishFailed(job, owner, stepID, media.ErrDownload)
		}
		if err := w.repo.FinishStep(ctx, stepID, "completed", "", "", map[string]any{"files": downloaded}, now); err != nil {
			return err
		}
	}
	// note 路径无需 ASR：仅需结果文件落盘（Storage/FileRepo），不依赖 transcriber。
	if err := w.repo.SetStage(ctx, job.ID, owner, "postprocessing", 90, "正在生成结果文件", now); err != nil {
		return err
	}
	text := asr.Normalize(work.Description)
	if text == "" {
		text = asr.Normalize(work.Title)
	}
	bundle, err := results.BuildNote(work, now)
	if err != nil {
		return err
	}
	files := []struct{ name string; body []byte; mime, kind string }{
		{"result.md", []byte(bundle.Markdown), "text/markdown; charset=utf-8", "result_markdown"},
		{"result.txt", []byte(bundle.Text), "text/plain; charset=utf-8", "result_text"},
		{"meta.json", bundle.Meta, "application/json", "result_meta"},
	}
	resultFiles := make([]JobFile, 0, len(files))
	for _, content := range files {
		file, err := writeResult(w.storage, w.fileRepo, ctx, job, content.name, content.kind, content.mime, content.body)
		if err != nil {
			return err
		}
		resultFiles = append(resultFiles, JobFile{ID: file.ID, Kind: file.Kind, Name: file.OriginalName, MIMEType: file.MIMEType, SizeBytes: file.SizeBytes})
	}
	result := map[string]any{"rawText": text, "normalizedText": text, "files": resultFiles}
	return w.repo.CompleteTranscription(ctx, job.ID, owner, result, "配图下载与文案生成完成", now)
}

func (w *Worker) Cancel(jobID string) bool {
	w.stopMu.Lock()
	cancel := w.stops[jobID]
	w.stopMu.Unlock()
	if cancel == nil {
		return false
	}
	cancel()
	return true
}

func (w *Worker) heartbeat(ctx context.Context, cancel context.CancelFunc, done chan<- struct{}, jobID, owner string) {
	defer close(done)
	ticker := time.NewTicker(w.config.Heartbeat)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := w.repo.Heartbeat(context.WithoutCancel(ctx), jobID, owner, w.now().UTC(), w.config.Lease); err != nil {
				cancel()
				return
			}
		}
	}
}

func (w *Worker) finishFailed(job Job, owner, stepID string, err error) error {
	now := w.now().UTC()
	code, message := mediaError(err)
	_ = w.repo.FinishStep(context.Background(), stepID, "failed", code, message, map[string]any{}, now)
	return w.failedWith(job, owner, code, message, err)
}
func (w *Worker) failed(job Job, owner string, err error) error {
	code, message := mediaError(err)
	return w.failedWith(job, owner, code, message, err)
}
func (w *Worker) failedWith(job Job, owner, code, message string, cause error) error {
	if errors.Is(cause, context.Canceled) {
		if persistErr := w.repo.CancelOwned(context.Background(), job.ID, owner, "任务已取消", w.now().UTC()); persistErr != nil {
			return errors.Join(cause, persistErr)
		}
		return cause
	}
	if errors.Is(cause, asr.ErrAuth) || errors.Is(cause, asr.ErrBudgetExceeded) || errors.Is(cause, asr.ErrInputRejected) || errors.Is(cause, resolver.ErrInvalidShareLink) || errors.Is(cause, resolver.ErrWorkUnavailable) || errors.Is(cause, resolver.ErrVideoRequired) || errors.Is(cause, media.ErrTooLarge) {
		if persistErr := w.repo.Fail(context.Background(), job.ID, code, message, w.now().UTC()); persistErr != nil {
			return errors.Join(cause, persistErr)
		}
		return cause
	}
	if errors.Is(cause, media.ErrFFmpeg) && job.AttemptCount >= 2 {
		if persistErr := w.repo.Fail(context.Background(), job.ID, code, message, w.now().UTC()); persistErr != nil {
			return errors.Join(cause, persistErr)
		}
		return cause
	}
	delay := asr.RetryAfter(cause, job.AttemptCount-1)
	if delay <= 0 {
		delay = time.Duration(1<<min(job.AttemptCount, 6))*time.Second + time.Duration(rand.IntN(1000))*time.Millisecond
	}
	if persistErr := w.repo.RetryLater(context.Background(), job.ID, owner, code, message, w.now().UTC().Add(delay), w.now().UTC()); persistErr != nil {
		return errors.Join(cause, persistErr)
	}
	return cause
}

func (w *Worker) rollbackFile(file ownedfiles.File) {
	_ = w.storage.Remove(file)
	_ = w.repo.DeleteFileRecord(context.Background(), file.ID)
}

func mediaError(err error) (string, string) {
	switch {
	case errors.Is(err, asr.ErrAuth):
		return "ASR_AUTH_FAILED", "语音识别服务认证失败"
	case errors.Is(err, asr.ErrRateLimited):
		return "ASR_RATE_LIMITED", "语音识别服务限流"
	case errors.Is(err, asr.ErrBudgetExceeded):
		return "ASR_BUDGET_EXCEEDED", "语音识别预算已达上限"
	case errors.Is(err, asr.ErrFailed):
		return "ASR_FAILED", "语音识别失败"
	case errors.Is(err, media.ErrFFmpeg):
		return "FFMPEG_FAILED", "音频处理失败"
	case errors.Is(err, media.ErrTooLarge):
		return "MEDIA_TOO_LARGE", "视频超过允许的大小"
	case errors.Is(err, resolver.ErrURLNotAllowed):
		return "URL_NOT_ALLOWED", "媒体地址不允许访问"
	case errors.Is(err, resolver.ErrWorkUnavailable):
		return "DOUYIN_WORK_UNAVAILABLE", "作品不存在或不可访问"
	case errors.Is(err, resolver.ErrVideoRequired):
		return "VIDEO_REQUIRED", "该作品是图文或不含视频，不能下载视频或提取口播"
	case errors.Is(err, resolver.ErrResolveFailed), errors.Is(err, resolver.ErrInvalidShareLink):
		return "DOUYIN_RESOLVE_FAILED", diagnosticMessage("无法解析该作品", err, resolver.ErrResolveFailed)
	case errors.Is(err, media.ErrDownload):
		return "MEDIA_DOWNLOAD_FAILED", diagnosticMessage("媒体下载失败", err, media.ErrDownload)
	default:
		return "MEDIA_PROCESSING_FAILED", "媒体处理失败：" + truncateError(err.Error())
	}
}
func diagnosticMessage(prefix string, err, sentinel error) string {
	detail := strings.TrimSpace(strings.TrimPrefix(err.Error(), sentinel.Error()+":"))
	if detail == "" || detail == err.Error() {
		return prefix
	}
	return prefix + "：" + truncateError(detail)
}
func truncateError(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 300 {
		return value[:300] + "…"
	}
	return value
}
func classify(err error) string { code, _ := mediaError(err); return code }
func safeMediaName(value string) string {
	value = filepath.Base(strings.TrimSpace(value))
	if value == "." || value == "" {
		return "douyin-video"
	}
	return value
}
