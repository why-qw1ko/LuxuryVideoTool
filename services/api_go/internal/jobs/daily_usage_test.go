package jobs

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/auth"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/database"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/resolver"
)

func newQuotaService(t *testing.T, now *time.Time) *Service {
	t.Helper()
	ctx := context.Background()
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	users := auth.NewSQLiteRepository(db)
	for _, id := range []string{"first", "second"} {
		if err := users.CreateUser(ctx, auth.User{
			ID: id, UsernameNormalized: id, DisplayName: id, PasswordHash: "hash", Role: auth.RoleUser,
			IsActive: true, CreatedAt: *now, UpdatedAt: *now,
		}); err != nil {
			t.Fatal(err)
		}
	}
	parser := &resolver.Fake{Err: resolver.ErrWorkUnavailable}
	service := NewService(NewSQLiteRepository(db), resolver.NewService(parser, resolver.NewSQLiteCache(db), time.Hour, resolver.DouyinResolverVersion))
	service.now = func() time.Time { return *now }
	return service
}

func TestDailyTaskLimitPreservesUsageAcrossTaskChanges(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, time.November, 24, 15, 59, 59, 0, time.UTC)
	service := newQuotaService(t, &now)
	checkUsage := func(userID string, want int, resetAt time.Time) {
		t.Helper()
		used, reset, err := service.DailyUsage(ctx, userID)
		if err != nil || used != want || !reset.Equal(resetAt) {
			t.Fatalf("daily usage for %s = %d, reset=%v, err=%v; want %d, %v", userID, used, reset, err, want, resetAt)
		}
	}
	resetAt := now.Add(time.Second)
	checkUsage("first", 0, resetAt)
	input := CreateInput{UserID: "first", ShareText: "https://www.douyin.com/video/123", DailyTaskLimit: 4}
	input.IdempotencyKey = "info"
	info, reused, err := service.CreateInfo(ctx, input)
	if err != nil || reused || info.Status != "failed" {
		t.Fatalf("info=%#v reused=%v err=%v", info, reused, err)
	}
	input.ActionDisabled = true
	if replayed, reused, err := service.CreateInfo(ctx, input); err != nil || !reused || replayed.ID != info.ID {
		t.Fatalf("info replay id=%q reused=%v err=%v", replayed.ID, reused, err)
	}
	input.IdempotencyKey = "disabled"
	if _, _, err := service.CreateInfo(ctx, input); !errors.Is(err, ErrTaskActionDisabled) {
		t.Fatalf("disabled new info job err=%v", err)
	}
	input.ActionDisabled = false
	invalid := input
	invalid.ShareText = "invalid share"
	if _, _, err := service.CreateDownload(ctx, invalid); !errors.Is(err, resolver.ErrInvalidShareLink) {
		t.Fatalf("invalid share err=%v", err)
	}

	var download Job
	for _, action := range []string{"download", "transcribe", "full"} {
		input.IdempotencyKey, input.Action = action, action
		job, reused, err := service.CreateDownload(ctx, input)
		if err != nil || reused || job.Action != action {
			t.Fatalf("job=%#v reused=%v err=%v", job, reused, err)
		}
		if action == "download" {
			download = job
		}
	}
	checkUsage("first", 4, resetAt)
	input.ActionDisabled = true
	if _, reused, err := service.CreateDownload(ctx, input); err != nil || !reused {
		t.Fatalf("existing full task reused=%v err=%v", reused, err)
	}
	latest := input
	latest.IdempotencyKey = "over-quota"
	if _, _, err := service.CreateDownload(ctx, latest); !errors.Is(err, ErrTaskActionDisabled) {
		t.Fatalf("disabled new full task err=%v", err)
	}
	latest.ActionDisabled = false
	if _, _, err := service.CreateDownload(ctx, latest); !errors.Is(err, ErrDailyTaskLimit) {
		t.Fatalf("over quota err=%v", err)
	}
	page, err := service.List(ctx, ListInput{UserID: "first"})
	if err != nil || page.Total != 4 {
		t.Fatalf("jobs after rejection=%d err=%v", page.Total, err)
	}
	if _, err := service.Cancel(ctx, "first", download.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Retry(ctx, "first", download.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Cancel(ctx, "first", download.ID); err != nil {
		t.Fatal(err)
	}
	if err := service.Delete(ctx, "first", download.ID); err != nil {
		t.Fatal(err)
	}
	checkUsage("first", 4, resetAt)
	other := latest
	other.UserID, other.DailyTaskLimit = "second", 1
	if _, _, err := service.CreateDownload(ctx, other); err != nil {
		t.Fatal(err)
	}
	checkUsage("second", 1, resetAt)

	now = resetAt
	checkUsage("first", 0, resetAt.Add(24*time.Hour))
	if _, reused, err := service.CreateDownload(ctx, input); err != nil || !reused {
		t.Fatalf("replay after midnight reused=%v err=%v", reused, err)
	}
	latest.DailyTaskLimit = 1
	if _, _, err := service.CreateDownload(ctx, latest); err != nil {
		t.Fatal(err)
	}
	latest.IdempotencyKey, latest.DailyTaskLimit = "unlimited", 0
	if _, _, err := service.CreateDownload(ctx, latest); err != nil {
		t.Fatal(err)
	}
	checkUsage("first", 2, resetAt.Add(24*time.Hour))
}

func TestStatsUsesBeijingMidnight(t *testing.T) {
	ctx := context.Background()
	midnight := time.Date(2026, time.November, 24, 16, 0, 0, 0, time.UTC)
	now := midnight.Add(-time.Millisecond)
	service := newQuotaService(t, &now)
	for _, creation := range []struct {
		key string
		at  time.Time
	}{
		{"before-midnight", midnight.Add(-time.Millisecond)},
		{"at-midnight", midnight},
		{"after-midnight", midnight.Add(time.Millisecond)},
	} {
		now = creation.at
		_, _, err := service.CreateDownload(ctx, CreateInput{
			UserID: "first", ShareText: "https://www.douyin.com/video/123", IdempotencyKey: creation.key,
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	now = midnight.Add(-time.Millisecond)
	stats, err := service.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Today != 1 || len(stats.ByDay) != 14 || stats.ByDay[13] != (DayCount{Day: "2026-11-24", Count: 1}) {
		t.Fatalf("stats before Beijing midnight = %+v", stats)
	}
	now = midnight
	stats, err = service.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Today != 2 || len(stats.ByDay) != 14 || stats.ByDay[12] != (DayCount{Day: "2026-11-24", Count: 1}) ||
		stats.ByDay[13] != (DayCount{Day: "2026-11-25", Count: 2}) {
		t.Fatalf("stats after Beijing midnight = %+v", stats)
	}
}

func TestDailyTaskLimitConcurrentCreations(t *testing.T) {
	now := time.Date(2026, time.November, 24, 12, 0, 0, 0, time.UTC)
	service := newQuotaService(t, &now)
	const requests, limit = 8, 3
	start := make(chan struct{})
	results := make(chan error, requests)
	for i := range requests {
		go func() {
			<-start
			_, _, err := service.CreateDownload(context.Background(), CreateInput{
				UserID: "first", ShareText: "https://www.douyin.com/video/123", IdempotencyKey: fmt.Sprintf("key-%d", i), DailyTaskLimit: limit,
			})
			results <- err
		}()
	}
	close(start)
	var created, rejected int
	for range requests {
		switch err := <-results; {
		case err == nil:
			created++
		case errors.Is(err, ErrDailyTaskLimit):
			rejected++
		default:
			t.Error(err)
		}
	}
	if created != limit || rejected != requests-limit {
		t.Fatalf("created=%d rejected=%d, want %d created and %d rejected", created, rejected, limit, requests-limit)
	}
}

type concurrentLookupMisses struct {
	Repository
	lookups atomic.Int32
}

func (r *concurrentLookupMisses) FindByIdempotencyKey(ctx context.Context, userID, key string) (Job, error) {
	if r.lookups.Add(1) <= 2 {
		return Job{}, ErrNotFound
	}
	return r.Repository.FindByIdempotencyKey(ctx, userID, key)
}

func TestDailyTaskLimitConcurrentIdempotencyConflict(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, time.November, 24, 12, 0, 0, 0, time.UTC)
	service := newQuotaService(t, &now)
	// Both requests see the key as missing before SQLite arbitrates the insert.
	service.repo = &concurrentLookupMisses{Repository: service.repo}
	results := make(chan error, 2)
	for _, keepVideo := range []bool{false, true} {
		go func() {
			_, _, err := service.CreateDownload(ctx, CreateInput{
				UserID: "first", ShareText: "https://www.douyin.com/video/123", IdempotencyKey: "same-key", KeepVideo: keepVideo, DailyTaskLimit: 1,
			})
			results <- err
		}()
	}
	first, second := <-results, <-results
	if !(first == nil && errors.Is(second, ErrIdempotencyConflict)) && !(second == nil && errors.Is(first, ErrIdempotencyConflict)) {
		t.Fatalf("concurrent create errors=%v, %v; want one conflict and one success", first, second)
	}
	if used, _, err := service.DailyUsage(ctx, "first"); err != nil || used != 1 {
		t.Fatalf("daily usage=%d err=%v, want 1", used, err)
	}
}

func TestDailyTaskLimitBackfillsExistingJobs(t *testing.T) {
	ctx := context.Background()
	midnight := time.Date(2026, time.November, 24, 16, 0, 0, 0, time.UTC)
	now := midnight.Add(-time.Millisecond)
	service := newQuotaService(t, &now)
	repo := service.repo.(*SQLiteRepository)
	for _, status := range []string{"cancelled", "failed"} {
		if err := repo.CreateQueued(ctx, Job{
			ID: status, UserID: "first", InputText: "previous request", InputURL: "https://www.douyin.com/video/123",
			Action: "download", Status: status, IdempotencyKey: status, CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			t.Fatal(err)
		}
		now = midnight
	}

	// Simulate the prior schema in this test's temporary database.
	if _, err := repo.db.ExecContext(ctx, `DROP TABLE job_daily_usage;
		DELETE FROM schema_migrations WHERE version = '0006_job_daily_usage.sql'`); err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(ctx, repo.db); err != nil {
		t.Fatal(err)
	}
	for _, date := range []string{"2026-11-24", "2026-11-25"} {
		if used, err := repo.DailyTaskUsage(ctx, "first", date); err != nil || used != 1 {
			t.Fatalf("usage after migration on %s=%d err=%v, want 1", date, used, err)
		}
	}
}
