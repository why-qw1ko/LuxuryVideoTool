package jobs

import (
	"context"
	"errors"
	"time"

	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/auth"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/resolver"
)

type Service struct {
	repo Repository
	resolver *resolver.Service
	now func() time.Time
	forceCancel func(string) bool
}

func NewService(repo Repository, resolverService *resolver.Service) *Service { return &Service{repo: repo, resolver: resolverService, now: time.Now} }
func (s *Service) SetForceCancel(cancel func(string) bool) { s.forceCancel = cancel }

func (s *Service) CreateInfo(ctx context.Context, input CreateInput) (Job, bool, error) {
	input.Action = "info"
	if existing, err := s.repo.FindByIdempotencyKey(ctx, input.UserID, input.IdempotencyKey); err == nil {
		if existing.InputText != input.ShareText || existing.Action != "info" || existing.ForceRefresh != input.Force { return Job{}, false, ErrIdempotencyConflict }
		return existing, true, nil
	} else if !errors.Is(err, ErrNotFound) { return Job{}, false, err }
	parsed, err := resolver.ExtractInput(input.ShareText)
	if err != nil { return Job{}, false, err }
	now := s.now().UTC()
	id, err := auth.NewID(now)
	if err != nil { return Job{}, false, err }
	job := Job{ID: id, UserID: input.UserID, InputText: input.ShareText, InputURL: parsed.URL.String(), Action: "info", Status: "resolving", Progress: 10, StatusMessage: "正在读取作品信息", IdempotencyKey: input.IdempotencyKey, ForceRefresh: input.Force, CreatedAt: now, UpdatedAt: now}
	if err := s.repo.CreateInfo(ctx, job); err != nil {
		if existing, findErr := s.repo.FindByIdempotencyKey(ctx, input.UserID, input.IdempotencyKey); findErr == nil {
			if existing.InputText != input.ShareText || existing.Action != "info" || existing.ForceRefresh != input.Force { return Job{}, false, ErrIdempotencyConflict }
			return existing, true, nil
		}
		return Job{}, false, err
	}
	work, _, err := s.resolver.Resolve(ctx, input.ShareText, input.Force)
	if err != nil {
		code, message := resolverError(err)
		if failErr := s.repo.Fail(context.WithoutCancel(ctx), job.ID, code, message, s.now().UTC()); failErr != nil { return Job{}, false, errors.Join(err, failErr) }
		failed, findErr := s.repo.FindByID(context.WithoutCancel(ctx), input.UserID, job.ID)
		if findErr != nil { return Job{}, false, errors.Join(err, findErr) }
		return failed, false, nil
	}
	completed := s.now().UTC()
	if err := s.repo.CompleteInfo(ctx, job.ID, work, completed); err != nil { return Job{}, false, err }
	created, err := s.repo.FindByID(ctx, input.UserID, job.ID)
	return created, false, err
}

func (s *Service) CreateDownload(ctx context.Context, input CreateInput) (Job, bool, error) {
	input.Action = "download"
	if existing, err := s.repo.FindByIdempotencyKey(ctx, input.UserID, input.IdempotencyKey); err == nil {
		if existing.InputText != input.ShareText || existing.Action != input.Action || existing.ForceRefresh != input.Force { return Job{}, false, ErrIdempotencyConflict }; return existing, true, nil
	} else if !errors.Is(err, ErrNotFound) { return Job{}, false, err }
	parsed, err := resolver.ExtractInput(input.ShareText); if err != nil { return Job{}, false, err }
	now := s.now().UTC(); id, err := auth.NewID(now); if err != nil { return Job{}, false, err }
	job := Job{ID: id, UserID: input.UserID, InputText: input.ShareText, InputURL: parsed.URL.String(), Action: "download", Status: "queued", Progress: 0, StatusMessage: "等待处理", IdempotencyKey: input.IdempotencyKey, ForceRefresh: input.Force, MaxAttempts: 3, CreatedAt: now, UpdatedAt: now}
	if err := s.repo.CreateQueued(ctx, job); err != nil {
		if existing, findErr := s.repo.FindByIdempotencyKey(ctx, input.UserID, input.IdempotencyKey); findErr == nil { return existing, true, nil }; return Job{}, false, err
	}
	created, err := s.repo.FindByID(ctx, input.UserID, job.ID)
	return created, false, err
}

func (s *Service) Get(ctx context.Context, userID, jobID string) (Job, error) { return s.repo.FindByID(ctx, userID, jobID) }
func (s *Service) Cancel(ctx context.Context, userID, jobID string) (Job, error) {
	_, err := s.repo.Cancel(ctx, userID, jobID, s.now().UTC())
	if errors.Is(err, ErrNotCancellable) && s.forceCancel != nil && s.forceCancel(jobID) {
		deadline := time.NewTimer(2*time.Second); defer deadline.Stop(); ticker := time.NewTicker(20*time.Millisecond); defer ticker.Stop()
		for { select { case <-ctx.Done(): return Job{}, ctx.Err(); case <-deadline.C: return Job{}, ErrNotCancellable; case <-ticker.C: current, findErr := s.repo.FindByID(ctx, userID, jobID); if findErr == nil && current.Status == "cancelled" { return current, nil } } }
	}
	if err != nil { return Job{}, err }; return s.repo.FindByID(ctx, userID, jobID)
}
func (s *Service) Retry(ctx context.Context, userID, jobID string) (Job, error) { if err := s.repo.Retry(ctx, userID, jobID, s.now().UTC()); err != nil { return Job{}, err }; return s.repo.FindByID(ctx, userID, jobID) }

func resolverError(err error) (string, string) {
	switch {
	case errors.Is(err, resolver.ErrInvalidShareLink): return "INVALID_SHARE_LINK", "未找到有效的抖音作品链接"
	case errors.Is(err, resolver.ErrURLNotAllowed): return "URL_NOT_ALLOWED", "链接目标不允许访问"
	case errors.Is(err, resolver.ErrWorkUnavailable): return "DOUYIN_WORK_UNAVAILABLE", "作品不存在或不可访问"
	default: return "DOUYIN_RESOLVE_FAILED", "无法解析该作品，请确认链接仍然有效"
	}
}
