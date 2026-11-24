package jobs

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/auth"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/database"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/resolver"
)

func TestWorkerResolveFailureStopsWithoutAutomaticRetry(t *testing.T) {
	ctx := context.Background()
	repo, service, worker, adapter := setupResolveWorker(t)
	for _, tc := range []struct {
		cause error
		code  string
	}{
		{resolver.ErrWorkUnavailable, "DOUYIN_WORK_UNAVAILABLE"},
		{resolver.ErrResolveFailed, "DOUYIN_RESOLVE_FAILED"},
		{resolver.ErrResolveTimeout, "DOUYIN_RESOLVE_TIMEOUT"},
	} {
		t.Run(tc.code, func(t *testing.T) {
			adapter.Err = fmt.Errorf("%w: upstream diagnostic", tc.cause)
			input := CreateInput{UserID: "owner", ShareText: "https://www.douyin.com/video/123", IdempotencyKey: tc.code}
			job, _, err := service.CreateDownload(ctx, input)
			if err != nil {
				t.Fatal(err)
			}
			claimed, err := repo.ClaimNext(ctx, "worker", time.Now(), time.Minute)
			if err != nil || claimed.ID != job.ID {
				t.Fatalf("claim = %q, %v; want %q", claimed.ID, err, job.ID)
			}
			if err := worker.process(ctx, "worker", claimed); !errors.Is(err, tc.cause) {
				t.Fatalf("process error = %v, want %v", err, tc.cause)
			}
			failed, err := service.Get(ctx, input.UserID, job.ID)
			if err != nil || failed.Status != "failed" || failed.AttemptCount != 1 || failed.Error == nil || failed.Error.Code != tc.code {
				t.Fatalf("failed job = %+v, %v", failed, err)
			}
			if strings.Contains(failed.Error.Message, "upstream diagnostic") {
				t.Fatalf("upstream diagnostic exposed to user: %q", failed.Error.Message)
			}
			if _, err := repo.ClaimNext(ctx, "worker", time.Now().Add(time.Hour), time.Minute); !errors.Is(err, ErrNotFound) {
				t.Fatalf("terminal failure automatically retried: %v", err)
			}
		})
	}
}

func TestInfoJobManualRetryOnlyResolves(t *testing.T) {
	ctx := context.Background()
	repo, service, worker, adapter := setupResolveWorker(t)
	adapter.Err = resolver.ErrResolveTimeout
	input := CreateInput{UserID: "owner", ShareText: "https://www.douyin.com/video/123", IdempotencyKey: "info"}
	job, _, err := service.CreateInfo(ctx, input)
	if err != nil || job.Status != "failed" || job.Error == nil || job.Error.Code != "DOUYIN_RESOLVE_TIMEOUT" {
		t.Fatalf("initial job = %+v, %v", job, err)
	}
	if _, err := service.Retry(ctx, job.UserID, job.ID); err != nil {
		t.Fatal(err)
	}
	adapter.Err = nil
	adapter.Work = resolver.Work{DouyinWorkID: "123", Type: "video", Title: "Recovered", VideoURL: "https://sample.douyinvod.com/video.mp4", ResolverVersion: resolver.DouyinResolverVersion, ResolvedAt: time.Now()}
	claimed, err := repo.ClaimNext(ctx, "worker", time.Now(), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := worker.process(ctx, "worker", claimed); err != nil {
		t.Fatal(err)
	}
	finished, err := service.Get(ctx, job.UserID, job.ID)
	if err != nil || finished.Status != "completed" || finished.LeaseOwner != "" || finished.Work == nil || finished.Work.Title != "Recovered" {
		t.Fatalf("retried job = %+v, %v", finished, err)
	}
	files, err := repo.FindFiles(ctx, job.UserID, job.ID)
	if err != nil || len(files) != 0 {
		t.Fatalf("info job files = %+v, %v", files, err)
	}
}

func TestWorkerShutdownRequeuesInterruptedResolution(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	repo, service, worker, _ := setupResolveWorker(t)
	worker.resolver = resolver.NewService(resolveAdapterFunc(func(ctx context.Context, _ string) (resolver.Work, error) {
		cancel()
		return resolver.Work{}, ctx.Err()
	}), nil, time.Hour, resolver.DouyinResolverVersion)
	input := CreateInput{UserID: "owner", ShareText: "https://www.douyin.com/video/123", IdempotencyKey: "shutdown"}
	job, _, err := service.CreateDownload(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := repo.ClaimNext(ctx, "worker", time.Now(), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := worker.process(ctx, "worker", claimed); !errors.Is(err, context.Canceled) {
		t.Fatalf("process after shutdown = %v", err)
	}
	resumed, err := repo.ClaimNext(context.Background(), "replacement-worker", time.Now(), time.Minute)
	if err != nil || resumed.ID != job.ID || resumed.AttemptCount != 1 {
		t.Fatalf("claim after shutdown = %+v, %v", resumed, err)
	}
}

type resolveAdapterFunc func(context.Context, string) (resolver.Work, error)

func (f resolveAdapterFunc) Resolve(ctx context.Context, shareText string) (resolver.Work, error) {
	return f(ctx, shareText)
}

func setupResolveWorker(t *testing.T) (*SQLiteRepository, *Service, *Worker, *resolver.Fake) {
	t.Helper()
	ctx := context.Background()
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "resolve.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	now := time.Now().UTC()
	user := auth.User{ID: "owner", UsernameNormalized: "owner", DisplayName: "Owner", PasswordHash: "hash", Role: auth.RoleUser, IsActive: true, CreatedAt: now, UpdatedAt: now}
	if err := auth.NewSQLiteRepository(db).CreateUser(ctx, user); err != nil {
		t.Fatal(err)
	}
	repo := NewSQLiteRepository(db)
	adapter := &resolver.Fake{}
	resolverService := resolver.NewService(adapter, resolver.NewSQLiteCache(db), time.Hour, resolver.DouyinResolverVersion)
	return repo, NewService(repo, resolverService), NewWorker(repo, resolverService, nil, nil, nil, nil, WorkerConfig{}), adapter
}
