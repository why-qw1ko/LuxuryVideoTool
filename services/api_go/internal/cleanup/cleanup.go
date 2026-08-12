package cleanup

import (
	"context"
	"log/slog"
	"time"

	ownedfiles "github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/files"
)

type Service struct { repo *ownedfiles.Repository; storage *ownedfiles.Storage; interval time.Duration; now func() time.Time }
func New(repo *ownedfiles.Repository, storage *ownedfiles.Storage, interval time.Duration) *Service { if interval <= 0 { interval = time.Hour }; return &Service{repo: repo, storage: storage, interval: interval, now: time.Now} }
func (s *Service) Run(ctx context.Context) {
	ticker := time.NewTicker(s.interval); defer ticker.Stop()
	for { s.Once(ctx); select { case <-ctx.Done(): return; case <-ticker.C: } }
}
func (s *Service) Once(ctx context.Context) {
	for {
		items, err := s.repo.Expired(ctx, s.now().UTC(), 100); if err != nil { slog.ErrorContext(ctx, "file cleanup query failed", "event", "cleanup_failed", "error", err); return }
		if len(items) == 0 { return }
		for _, file := range items {
			if err := s.storage.Remove(file); err != nil { slog.ErrorContext(ctx, "file cleanup remove failed", "event", "cleanup_failed", "file_id", file.ID, "error", err); continue }
			if err := s.repo.MarkDeleted(ctx, file.ID, s.now().UTC()); err != nil { slog.ErrorContext(ctx, "file cleanup record failed", "event", "cleanup_failed", "file_id", file.ID, "error", err) }
		}
		if len(items) < 100 { return }
	}
}
