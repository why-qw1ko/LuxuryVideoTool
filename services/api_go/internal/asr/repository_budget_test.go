package asr_test

import (
	"context"
	"database/sql"
	"math"
	"path/filepath"
	"testing"
	"time"

	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/asr"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/auth"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/database"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/jobs"
)

func newBudgetRepositoryFixture(t *testing.T, now time.Time) (*sql.DB, *jobs.Service, *asr.Repository) {
	t.Helper()
	ctx := context.Background()
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "budget.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := auth.NewSQLiteRepository(db).CreateUser(ctx, auth.User{
		ID: "user", UsernameNormalized: "user", DisplayName: "User", PasswordHash: "hash", Role: auth.RoleUser,
		IsActive: true, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	jobRepo := jobs.NewSQLiteRepository(db)
	if err := jobRepo.CreateQueued(ctx, jobs.Job{
		ID: "job", UserID: "user", InputText: "https://www.douyin.com/video/123", InputURL: "https://www.douyin.com/video/123",
		Action: "transcribe", Status: "failed", IdempotencyKey: "create-job", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	return db, jobs.NewService(jobRepo, nil), asr.NewRepository(db)
}

func assertBudgetCost(t *testing.T, cost func(context.Context, time.Time) (float64, error), date time.Time, want float64) {
	t.Helper()
	got, err := cost(context.Background(), date)
	if err != nil || math.Abs(got-want) > 1e-9 {
		t.Fatalf("budget usage for %s = %v, err=%v; want %v", date, got, err, want)
	}
}

func TestASRBudgetUsageSurvivesRetryAndDeletion(t *testing.T) {
	ctx := context.Background()
	newMonth := time.Date(2026, time.December, 1, 0, 0, 0, 0, time.UTC)
	started := newMonth.Add(-time.Millisecond)
	db, jobService, repo := newBudgetRepositoryFixture(t, started)
	for _, call := range []asr.Call{
		{ID: "adjusted", JobID: "job", Provider: "test", Model: "test", Status: "submitted", StartedAt: started, EstimatedCostCNY: 0.7},
		{ID: "failed", JobID: "job", Provider: "test", Model: "test", Status: "submitted", StartedAt: started, EstimatedCostCNY: 0.3},
		{ID: "last", JobID: "job", Provider: "test", Model: "test", Status: "submitted", StartedAt: newMonth, EstimatedCostCNY: 0.5},
	} {
		if err := repo.Create(ctx, call); err != nil {
			t.Fatal(err)
		}
	}
	assertBudgetCost(t, repo.DailyEstimatedCost, started, 1.0)
	if err := repo.Fail(ctx, "failed", "ASR_FAILURE", nil, newMonth); err != nil {
		t.Fatal(err)
	}
	result := asr.TranscribeResult{BilledSeconds: 30}
	for range 2 {
		if err := repo.Complete(ctx, "adjusted", result, 0.2, newMonth); err != nil {
			t.Fatal(err)
		}
	}
	assertBudgetCost(t, repo.DailyEstimatedCost, started, 0.5)
	assertBudgetCost(t, repo.DailyEstimatedCost, newMonth, 0.5)
	if _, err := jobService.Retry(ctx, "user", "job"); err != nil {
		t.Fatal(err)
	}
	assertBudgetCost(t, repo.DailyEstimatedCost, started, 0.5)
	if _, err := jobService.Cancel(ctx, "user", "job"); err != nil {
		t.Fatal(err)
	}
	if err := jobService.Delete(ctx, "user", "job"); err != nil {
		t.Fatal(err)
	}
	var remaining int
	if err := db.QueryRow(`SELECT COUNT(*) FROM asr_calls`).Scan(&remaining); err != nil || remaining != 0 {
		t.Fatalf("remaining call details=%d error=%v", remaining, err)
	}
	assertBudgetCost(t, repo.DailyEstimatedCost, started, 0.5)
	assertBudgetCost(t, repo.MonthlyEstimatedCost, started, 0.5)
	assertBudgetCost(t, repo.DailyEstimatedCost, newMonth, 0.5)
	assertBudgetCost(t, repo.MonthlyEstimatedCost, newMonth, 0.5)
}

func TestASRBudgetUsageBackfillsExistingCalls(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, time.November, 24, 12, 0, 0, 0, time.UTC)
	db, _, repo := newBudgetRepositoryFixture(t, now)
	// Simulate the prior schema only in this test's temporary database.
	if _, err := db.Exec(`DROP TRIGGER asr_budget_record_call;
		DROP TRIGGER asr_budget_adjust_call;
		DROP TABLE asr_budget_usage;
		DELETE FROM schema_migrations WHERE version = '0008_asr_budget_usage.sql'`); err != nil {
		t.Fatal(err)
	}
	for _, call := range []asr.Call{
		{ID: "completed", JobID: "job", Provider: "test", Model: "test", Status: "completed", StartedAt: now, EstimatedCostCNY: 0.2},
		{ID: "failed", JobID: "job", Provider: "test", Model: "test", Status: "failed", StartedAt: now, EstimatedCostCNY: 0.1},
	} {
		if err := repo.Create(ctx, call); err != nil {
			t.Fatal(err)
		}
	}
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	assertBudgetCost(t, repo.DailyEstimatedCost, now, 0.3)
	assertBudgetCost(t, repo.MonthlyEstimatedCost, now, 0.3)
}
