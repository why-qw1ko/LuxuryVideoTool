package asr

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/auth"
	"golang.org/x/text/unicode/norm"
)

type Budget struct { DailyCNY, MonthlyCNY, PricePerMinuteCNY float64 }
type Service struct { primary, fallback Provider; repo *Repository; budget Budget; now func() time.Time; budgetMu sync.Mutex }
func NewService(primary, fallback Provider, repo *Repository, budget Budget) *Service { return &Service{primary: primary, fallback: fallback, repo: repo, budget: budget, now: time.Now} }

func (s *Service) Transcribe(ctx context.Context, jobID, audioPath, audioURL string, duration time.Duration, languages, hotwords []string) (TranscribeResult, error) {
	return s.TranscribeSegment(ctx, jobID, 0, audioPath, audioURL, duration, languages, hotwords, nil)
}
func (s *Service) TranscribeSegment(ctx context.Context, jobID string, index int, audioPath, audioURL string, duration time.Duration, languages, hotwords []string, progress func(done bool) error) (TranscribeResult, error) {
	seconds := duration.Seconds(); estimate := seconds/60*s.budget.PricePerMinuteCNY
	s.budgetMu.Lock(); defer s.budgetMu.Unlock(); if err := s.checkBudget(ctx, estimate); err != nil { return TranscribeResult{}, err }
	result, err := s.call(ctx, s.primary, jobID, index, audioPath, audioURL, seconds, estimate, languages, hotwords, progress)
	if err != nil && fallbackAllowed(err) && s.fallback != nil { if budgetErr := s.checkBudget(ctx, estimate); budgetErr != nil { return TranscribeResult{}, budgetErr }; return s.call(ctx, s.fallback, jobID, index, audioPath, "", seconds, estimate, languages, hotwords, progress) }
	return result, err
}
func (s *Service) call(ctx context.Context, provider Provider, jobID string, index int, audioPath, audioURL string, seconds, estimate float64, languages, hotwords []string, progress func(done bool) error) (TranscribeResult, error) { now := s.now().UTC(); id, err := auth.NewID(now); if err != nil { return TranscribeResult{}, err }; call := Call{ID: id, JobID: jobID, Provider: provider.Name(), Model: provider.Model(), SegmentIndex: index, AudioSeconds: seconds, EstimatedCostCNY: estimate, Status: "submitted", StartedAt: now, Summary: map[string]any{}}; if err := s.repo.Create(ctx, call); err != nil { return TranscribeResult{}, err }; timeout := 30*time.Minute; callCtx, cancel := context.WithTimeout(ctx, timeout); defer cancel(); result, err := provider.Transcribe(callCtx, TranscribeRequest{AudioPath: audioPath, AudioURL: audioURL, LanguageHints: languages, Hotwords: hotwords, JobID: jobID, Timeout: timeout, TaskReporter: func(taskID, requestID string) error { return s.repo.SetProviderTask(context.WithoutCancel(ctx), id, taskID, requestID) }, ProgressReporter: progress}); if err != nil { code := errorCode(err); _ = s.repo.Fail(context.WithoutCancel(ctx), id, code, map[string]any{"retryable": fallbackAllowed(err)}, s.now().UTC()); return TranscribeResult{}, err }; result.Provider, result.Model = provider.Name(), provider.Model(); actualCost := result.BilledSeconds/60*s.budget.PricePerMinuteCNY; if result.BilledSeconds == 0 { actualCost = estimate }; if err := s.repo.Complete(ctx, id, result, actualCost, s.now().UTC()); err != nil { return TranscribeResult{}, err }; result.NormalizedText = Normalize(result.RawText); return result, nil }
func (s *Service) checkBudget(ctx context.Context, estimate float64) error { if s.budget.MonthlyCNY > 0 { total, err := s.repo.MonthlyEstimatedCost(ctx, s.now()); if err != nil { return err }; if total+estimate > s.budget.MonthlyCNY { return ErrBudgetExceeded } }; if s.budget.DailyCNY > 0 { total, err := s.repo.DailyEstimatedCost(ctx, s.now()); if err != nil { return err }; if total+estimate > s.budget.DailyCNY { return ErrBudgetExceeded } }; return nil }
func fallbackAllowed(err error) bool { return errors.Is(err, ErrFailed) || errors.Is(err, ErrRateLimited) || errors.Is(err, context.DeadlineExceeded) }
func errorCode(err error) string { switch { case errors.Is(err, ErrAuth): return "ASR_AUTH_FAILED"; case errors.Is(err, ErrRateLimited): return "ASR_RATE_LIMITED"; case errors.Is(err, ErrBudgetExceeded): return "ASR_BUDGET_EXCEEDED"; default: return "ASR_FAILED" } }
func Normalize(value string) string { value = strings.ReplaceAll(value, "\r\n", "\n"); value = strings.ReplaceAll(value, "\r", "\n"); value = norm.NFC.String(value); var b strings.Builder; for _, r := range value { if r == '\n' || r == '\t' || !unicode.IsControl(r) { b.WriteRune(r) } }; return strings.TrimSpace(b.String()) }
func Merge(parts []string) string { var merged string; for _, part := range parts { part = Normalize(part); if part == "" { continue }; if merged == "" { merged = part; continue }; left, right := []rune(merged), []rune(part); max := min(40, min(len(left), len(right))); overlap := 0; for size := max; size >= 4; size-- { if string(left[len(left)-size:]) == string(right[:size]) { overlap = size; break } }; if overlap > 0 { merged += string(right[overlap:]) } else { merged += "\n"+part } }; return merged }
