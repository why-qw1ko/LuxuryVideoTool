package asr

import (
	"context"
	"errors"
	"time"
)

var (
	ErrAuth = errors.New("ASR authentication failed")
	ErrRateLimited = errors.New("ASR rate limited")
	ErrBudgetExceeded = errors.New("ASR budget exceeded")
	ErrFailed = errors.New("ASR failed")
	ErrInputRejected = errors.New("ASR input rejected")
)

type Segment struct { Index int `json:"index"`; StartMS int64 `json:"startMs"`; EndMS int64 `json:"endMs"`; Text string `json:"text"`; SpeakerID *int `json:"speakerId,omitempty"` }
type TranscribeRequest struct { AudioPath string; AudioURL string; LanguageHints []string; Hotwords []string; JobID string; Timeout time.Duration; CallbackURL string; TaskReporter func(taskID, requestID string) error; ProgressReporter func(done bool) error }
type TranscribeResult struct { RawText string; NormalizedText string; Segments []Segment; Language string; DurationSeconds float64; BilledSeconds float64; RequestID string; ProviderTaskID string; Provider string; Model string; Summary map[string]any }

type Provider interface {
	Name() string
	Model() string
	Validate(ctx context.Context) error
	Transcribe(ctx context.Context, req TranscribeRequest) (TranscribeResult, error)
}
type RecoverableProvider interface { Provider; Resume(ctx context.Context, req TranscribeRequest, taskID string) (TranscribeResult, error) }

type Fake struct { ProviderName string; ModelName string; Result TranscribeResult; Err error; Calls int }
func (f *Fake) Name() string { if f.ProviderName == "" { return "fake" }; return f.ProviderName }
func (f *Fake) Model() string { if f.ModelName == "" { return "fake-model" }; return f.ModelName }
func (f *Fake) Validate(context.Context) error { return f.Err }
func (f *Fake) Transcribe(context.Context, TranscribeRequest) (TranscribeResult, error) { f.Calls++; return f.Result, f.Err }

type Call struct { ID, JobID, Provider, Model, ProviderRequestID, Status, ErrorCode string; SegmentIndex int; AudioSeconds, BilledSeconds, EstimatedCostCNY float64; Summary map[string]any; StartedAt time.Time; CompletedAt *time.Time }

type rateLimitError struct{ after time.Duration }
func (e rateLimitError) Error() string { return ErrRateLimited.Error() }
func (e rateLimitError) Unwrap() error { return ErrRateLimited }
func RetryAfter(err error, attempt int) time.Duration { var limited rateLimitError; if errors.As(err, &limited) && limited.after > 0 { return limited.after }; if errors.Is(err, ErrRateLimited) { return time.Duration(30*(1<<min(attempt, 2)))*time.Second }; return 0 }
