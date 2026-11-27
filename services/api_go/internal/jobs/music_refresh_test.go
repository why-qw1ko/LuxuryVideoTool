package jobs

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/auth"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/database"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/resolver"
)

type musicRefreshAdapter func(context.Context, string) (resolver.Work, error)

func (f musicRefreshAdapter) Resolve(ctx context.Context, input string) (resolver.Work, error) {
	return f(ctx, input)
}

func newMusicRefreshFixture(t *testing.T, adapter resolver.Adapter, count int) (*Service, *SQLiteRepository, []Job) {
	t.Helper()
	ctx := context.Background()
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "music-refresh.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	now := time.Now().UTC().Truncate(time.Millisecond)
	for _, userID := range []string{"owner", "other"} {
		if err := auth.NewSQLiteRepository(db).CreateUser(ctx, auth.User{
			ID: userID, UsernameNormalized: userID, DisplayName: userID, PasswordHash: "hash",
			Role: auth.RoleUser, IsActive: true, CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			t.Fatal(err)
		}
	}
	cache := resolver.NewSQLiteCache(db)
	repo := NewSQLiteRepository(db)
	var result []Job
	for index := 0; index < count; index++ {
		id := fmt.Sprint(100 + index)
		work, err := cache.Save(ctx, resolver.Work{
			DouyinWorkID: id, Type: "note", CanonicalURL: "https://www.douyin.com/note/" + id,
			Title: "Saved work", MusicURL: "https://example.test/old.mp3", ResolvedAt: now,
		})
		if err != nil {
			t.Fatal(err)
		}
		job := Job{
			ID: "job-" + id, UserID: "owner", InputURL: work.CanonicalURL, InputText: work.CanonicalURL,
			Action: "info", Status: "resolving", IdempotencyKey: id, CreatedAt: now, UpdatedAt: now,
		}
		if err := repo.CreateInfo(ctx, job); err != nil {
			t.Fatal(err)
		}
		if err := repo.CompleteInfo(ctx, job.ID, "", work, now); err != nil {
			t.Fatal(err)
		}
		job, err = repo.FindByID(ctx, "owner", job.ID)
		if err != nil {
			t.Fatal(err)
		}
		result = append(result, job)
	}
	service := NewService(repo, resolver.NewService(adapter, cache, time.Hour, resolver.DouyinResolverVersion))
	return service, repo, result
}

func TestMusicRefreshForcesSourceAndPreservesJobQuota(t *testing.T) {
	var calls atomic.Int32
	adapter := musicRefreshAdapter(func(ctx context.Context, input string) (resolver.Work, error) {
		calls.Add(1)
		if input != "https://www.douyin.com/note/100" {
			t.Errorf("refresh input = %q", input)
		}
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > musicRefreshTimeout || time.Until(deadline) < 20*time.Second {
			t.Errorf("refresh deadline = %v present=%v", deadline, ok)
		}
		return resolver.Work{DouyinWorkID: "100", Type: "note", CanonicalURL: input, Title: "Current work",
			MusicURL: "https://example.test/new.mp3", MusicTitle: "Current music", MusicArtist: "Artist"}, nil
	})
	service, repo, jobs := newMusicRefreshFixture(t, adapter, 1)
	ctx := context.Background()
	beforeUsage, _, err := service.DailyUsage(ctx, "owner")
	if err != nil {
		t.Fatal(err)
	}
	refreshed, err := service.RefreshMusic(ctx, "owner", jobs[0].ID)
	if err != nil || refreshed.ID != jobs[0].ID || refreshed.Work.MusicURL != "https://example.test/new.mp3" ||
		refreshed.Work.MusicTitle != "Current music" || refreshed.Status != "completed" || calls.Load() != 1 {
		t.Fatalf("refresh=%+v error=%v calls=%d", refreshed, err, calls.Load())
	}
	persisted, err := repo.FindByID(ctx, "owner", jobs[0].ID)
	if err != nil || persisted.Work.MusicURL != refreshed.Work.MusicURL {
		t.Fatalf("persisted music=%+v error=%v", persisted.Work, err)
	}
	afterUsage, _, err := service.DailyUsage(ctx, "owner")
	page, listErr := repo.List(ctx, ListInput{UserID: "owner", Limit: 10})
	if err != nil || listErr != nil || beforeUsage != afterUsage || page.Total != 1 {
		t.Fatalf("refresh changed quota/tasks: before=%d after=%d total=%d error=%v/%v", beforeUsage, afterUsage, page.Total, err, listErr)
	}
	var limited *MusicRefreshLimitError
	if _, err := service.RefreshMusic(ctx, "owner", jobs[0].ID); !errors.As(err, &limited) || limited.RetryAfter <= 0 || calls.Load() != 1 {
		t.Fatalf("successful refresh cooldown: error=%v calls=%d", err, calls.Load())
	}
	if _, err := service.RefreshMusic(ctx, "other", jobs[0].ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("other owner refresh = %v", err)
	}
}

func TestMusicRefreshErrorsKeepFallbackAndConsumeCooldown(t *testing.T) {
	upstreamErr := errors.New("upstream unavailable")
	var calls atomic.Int32
	service, repo, jobs := newMusicRefreshFixture(t, musicRefreshAdapter(func(context.Context, string) (resolver.Work, error) {
		calls.Add(1)
		return resolver.Work{}, upstreamErr
	}), 1)
	first, err := service.RefreshMusic(context.Background(), "owner", jobs[0].ID)
	if !errors.Is(err, upstreamErr) || first.Work.MusicURL != "https://example.test/old.mp3" {
		t.Fatalf("failed refresh lost fallback: job=%+v error=%v", first, err)
	}
	var limited *MusicRefreshLimitError
	if _, err := service.RefreshMusic(context.Background(), "owner", jobs[0].ID); !errors.As(err, &limited) || calls.Load() != 1 {
		t.Fatalf("failed refresh cooldown: error=%v calls=%d", err, calls.Load())
	}
	service.now = func() time.Time { return time.Now().Add(musicRefreshCooldown + time.Second) }
	if _, err := service.RefreshAnyMusic(context.Background(), jobs[0].ID); !errors.Is(err, upstreamErr) || calls.Load() != 2 {
		t.Fatalf("cooldown did not reset: error=%v calls=%d", err, calls.Load())
	}
	current, err := repo.FindByID(context.Background(), "owner", jobs[0].ID)
	if err != nil || current.Work.MusicURL != "https://example.test/old.mp3" {
		t.Fatalf("failed refresh changed stored fallback: job=%+v error=%v", current, err)
	}
	service.resolver = nil
	if _, err := service.RefreshMusic(context.Background(), "owner", jobs[0].ID); !errors.Is(err, ErrMusicRefreshUnavailable) {
		t.Fatalf("missing resolver = %v", err)
	}
	if _, err := repo.db.ExecContext(context.Background(), `UPDATE jobs SET status='queued' WHERE id=?`, jobs[0].ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.RefreshAnyMusic(context.Background(), jobs[0].ID); !errors.Is(err, ErrMusicRefreshNotReady) {
		t.Fatalf("unfinished refresh = %v", err)
	}
}

func TestMusicRefreshMergesWaitersAndBoundsConcurrency(t *testing.T) {
	started := make(chan struct{}, 2)
	release := make(chan struct{})
	var calls atomic.Int32
	adapter := musicRefreshAdapter(func(ctx context.Context, input string) (resolver.Work, error) {
		calls.Add(1)
		started <- struct{}{}
		select {
		case <-release:
			id := input[strings.LastIndex(input, "/")+1:]
			return resolver.Work{DouyinWorkID: id, Type: "note", CanonicalURL: input, Title: "Refreshed", MusicURL: "https://example.test/new.mp3"}, nil
		case <-ctx.Done():
			return resolver.Work{}, ctx.Err()
		}
	})
	service, _, jobs := newMusicRefreshFixture(t, adapter, 3)
	firstCtx, cancelFirst := context.WithCancel(context.Background())
	defer cancelFirst()
	first := make(chan error, 1)
	second := make(chan error, 1)
	third := make(chan error, 1)
	go func() { _, err := service.RefreshMusic(firstCtx, "owner", jobs[0].ID); first <- err }()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("first refresh did not start")
	}
	go func() { _, err := service.RefreshAnyMusic(context.Background(), jobs[0].ID); second <- err }()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	for {
		service.musicRefresh.mu.Lock()
		waiters := service.musicRefresh.entries[jobs[0].Work.DouyinWorkID].waiters
		service.musicRefresh.mu.Unlock()
		if waiters == 2 {
			break
		}
		select {
		case <-deadline.C:
			t.Fatal("second request did not join shared refresh")
		case <-time.After(time.Millisecond):
		}
	}
	go func() { _, err := service.RefreshMusic(context.Background(), "owner", jobs[1].ID); third <- err }()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("second work refresh did not start")
	}
	var limited *MusicRefreshLimitError
	if _, err := service.RefreshMusic(context.Background(), "owner", jobs[2].ID); !errors.As(err, &limited) || calls.Load() != 2 {
		t.Fatalf("concurrency limit failed: error=%v calls=%d", err, calls.Load())
	}
	cancelFirst()
	if err := <-first; !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled waiter = %v", err)
	}
	close(release)
	if err := <-second; err != nil {
		t.Fatalf("shared refresh was canceled by another waiter: %v", err)
	}
	if err := <-third; err != nil {
		t.Fatalf("second work refresh = %v", err)
	}
}

func TestMusicRefreshCapacityCannotEvictActiveCooldowns(t *testing.T) {
	var calls atomic.Int32
	service, _, jobs := newMusicRefreshFixture(t, musicRefreshAdapter(func(ctx context.Context, input string) (resolver.Work, error) {
		calls.Add(1)
		return resolver.Work{DouyinWorkID: "100", Type: "note", CanonicalURL: input, Title: "Refreshed", MusicURL: "https://example.test/new.mp3"}, nil
	}), 1)
	now := time.Now().UTC()
	service.now = func() time.Time { return now }
	service.musicRefresh.entries = make(map[string]*musicRefreshEntry)
	for index := 0; index < musicRefreshCapacity; index++ {
		service.musicRefresh.entries[fmt.Sprint(1000+index)] = &musicRefreshEntry{startedAt: now}
	}
	var limited *MusicRefreshLimitError
	if _, err := service.RefreshMusic(context.Background(), "owner", jobs[0].ID); !errors.As(err, &limited) || len(service.musicRefresh.entries) != musicRefreshCapacity || calls.Load() != 0 {
		t.Fatalf("capacity limit failed: size=%d calls=%d error=%v", len(service.musicRefresh.entries), calls.Load(), err)
	}
	service.now = func() time.Time { return now.Add(musicRefreshCooldown) }
	if _, err := service.RefreshMusic(context.Background(), "owner", jobs[0].ID); err != nil || calls.Load() != 1 || len(service.musicRefresh.entries) != 1 {
		t.Fatalf("expired entries were not reclaimed: size=%d calls=%d error=%v", len(service.musicRefresh.entries), calls.Load(), err)
	}
}

func TestMusicRefreshLastCanceledWaiterStopsSharedRequest(t *testing.T) {
	started := make(chan struct{})
	stopped := make(chan struct{})
	service, _, jobs := newMusicRefreshFixture(t, musicRefreshAdapter(func(ctx context.Context, _ string) (resolver.Work, error) {
		close(started)
		<-ctx.Done()
		close(stopped)
		return resolver.Work{}, ctx.Err()
	}), 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() { _, err := service.RefreshMusic(ctx, "owner", jobs[0].ID); result <- err }()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("refresh did not start")
	}
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled caller = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("canceled caller did not return")
	}
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("last canceled waiter left resolver running")
	}
}

func TestMusicRefreshRechecksDeletedTaskBeforeReturning(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	service, repo, jobs := newMusicRefreshFixture(t, musicRefreshAdapter(func(ctx context.Context, input string) (resolver.Work, error) {
		close(started)
		select {
		case <-release:
			return resolver.Work{DouyinWorkID: "100", Type: "note", CanonicalURL: input, Title: "Refreshed", MusicURL: "https://example.test/new.mp3"}, nil
		case <-ctx.Done():
			return resolver.Work{}, ctx.Err()
		}
	}), 1)
	result := make(chan error, 1)
	go func() { _, err := service.RefreshMusic(context.Background(), "owner", jobs[0].ID); result <- err }()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("refresh did not start")
	}
	if err := repo.Delete(context.Background(), "owner", jobs[0].ID); err != nil {
		t.Fatal(err)
	}
	close(release)
	select {
	case err := <-result:
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("deleted task returned refreshed data: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("refresh did not complete")
	}
}

func TestMusicRefreshDoesNotJoinAbandonedRequest(t *testing.T) {
	started := make(chan struct{})
	canceled := make(chan struct{})
	allowExit := make(chan struct{})
	var calls atomic.Int32
	service, _, jobs := newMusicRefreshFixture(t, musicRefreshAdapter(func(ctx context.Context, _ string) (resolver.Work, error) {
		calls.Add(1)
		close(started)
		<-ctx.Done()
		close(canceled)
		<-allowExit
		return resolver.Work{}, ctx.Err()
	}), 1)
	defer close(allowExit)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() { _, err := service.RefreshMusic(ctx, "owner", jobs[0].ID); result <- err }()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("refresh did not start")
	}
	cancel()
	select {
	case <-canceled:
	case <-time.After(5 * time.Second):
		t.Fatal("abandoned refresh was not canceled")
	}
	var limited *MusicRefreshLimitError
	if _, err := service.RefreshMusic(context.Background(), "owner", jobs[0].ID); !errors.As(err, &limited) || limited.RetryAfter <= 0 || calls.Load() != 1 {
		t.Fatalf("request joined abandoned refresh: calls=%d error=%v", calls.Load(), err)
	}
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("original waiter = %v", err)
	}
}

func TestMusicRefreshEmptyMusicDoesNotOverwriteSavedFallback(t *testing.T) {
	var calls atomic.Int32
	service, repo, jobs := newMusicRefreshFixture(t, musicRefreshAdapter(func(_ context.Context, input string) (resolver.Work, error) {
		calls.Add(1)
		return resolver.Work{DouyinWorkID: "100", Type: "note", CanonicalURL: input, Title: "Valid work without music"}, nil
	}), 1)
	job, err := service.RefreshMusic(context.Background(), "owner", jobs[0].ID)
	if !errors.Is(err, resolver.ErrMusicUnavailable) || job.Work == nil || job.Work.MusicURL != "https://example.test/old.mp3" {
		t.Fatalf("empty refreshed music lost fallback: job=%+v error=%v", job, err)
	}
	persisted, err := repo.FindByID(context.Background(), "owner", jobs[0].ID)
	if err != nil || persisted.Work == nil || persisted.Work.MusicURL != "https://example.test/old.mp3" || persisted.Work.Title != "Saved work" {
		t.Fatalf("empty refreshed music overwrote cache: work=%+v error=%v", persisted.Work, err)
	}
	var limited *MusicRefreshLimitError
	if _, err := service.RefreshAnyMusic(context.Background(), jobs[0].ID); !errors.As(err, &limited) || calls.Load() != 1 {
		t.Fatalf("empty music error did not retain cooldown: calls=%d error=%v", calls.Load(), err)
	}
	// Normal forced parsing still accepts a nonempty work without music.
	if _, _, err := service.resolver.Resolve(context.Background(), jobs[0].InputURL, true); err != nil || calls.Load() != 2 {
		t.Fatalf("ordinary resolve changed behavior: calls=%d error=%v", calls.Load(), err)
	}
	parsed, err := repo.FindByID(context.Background(), "owner", jobs[0].ID)
	if err != nil || parsed.Work.MusicURL != "" || parsed.Work.Title != "Valid work without music" {
		t.Fatalf("ordinary resolve did not retain existing behavior: work=%+v error=%v", parsed.Work, err)
	}
}
