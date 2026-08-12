package jobs

import (
	"context"
	"errors"
	"time"

	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/resolver"
)

var (
	ErrNotFound = errors.New("job not found")
	ErrIdempotencyConflict = errors.New("idempotency key reused with different request")
	ErrNotCancellable = errors.New("job not cancellable")
	ErrNotRetryable = errors.New("job not retryable")
	ErrNotDeletable = errors.New("job not deletable")
	ErrInvalidOptions = errors.New("invalid job options")
	ErrLeaseLost = errors.New("job lease lost")
)

type Job struct {
	ID             string         `json:"id"`
	UserID         string         `json:"-"`
	Work           *resolver.Work `json:"work,omitempty"`
	InputText      string         `json:"-"`
	InputURL       string         `json:"-"`
	Action         string         `json:"action"`
	Status         string         `json:"status"`
	Progress       int            `json:"progress"`
	StatusMessage  string         `json:"statusMessage"`
	IdempotencyKey string         `json:"-"`
	ForceRefresh   bool           `json:"-"`
	ErrorCode      string         `json:"-"`
	ErrorMessage   string         `json:"-"`
	Result         any            `json:"result"`
	Error          *JobError      `json:"error"`
	CreatedAt      time.Time      `json:"createdAt"`
	UpdatedAt      time.Time      `json:"updatedAt"`
	CompletedAt    *time.Time     `json:"completedAt,omitempty"`
	AttemptCount   int            `json:"attemptCount"`
	MaxAttempts    int            `json:"maxAttempts"`
	LeaseOwner     string         `json:"-"`
	KeepVideo      bool           `json:"-"`
	LanguageHints  []string       `json:"-"`
	Hotwords       []string       `json:"-"`
}

type JobError struct {
	Code string `json:"code"`
	Message string `json:"message"`
}

type CreateInput struct {
	UserID, ShareText, IdempotencyKey string
	Action string
	Force bool
	KeepVideo bool
	LanguageHints []string
	Hotwords []string
}

type ListInput struct {
	UserID string
	Query string
	Status string
	Action string
	Limit int
	Offset int
}

type JobPage struct {
	Items []Job `json:"items"`
	Total int `json:"total"`
	Limit int `json:"limit"`
	Offset int `json:"offset"`
}

type Step struct {
	ID string
	JobID string
	Name string
	Attempt int
	Status string
	StartedAt time.Time
	CompletedAt *time.Time
	Details map[string]any
	ErrorCode string
	ErrorMessage string
}

type Repository interface {
	FindByIdempotencyKey(ctx context.Context, userID, key string) (Job, error)
	CreateInfo(ctx context.Context, job Job) error
	CompleteInfo(ctx context.Context, jobID string, work resolver.Work, at time.Time) error
	Fail(ctx context.Context, jobID, code, message string, at time.Time) error
	FindByID(ctx context.Context, userID, jobID string) (Job, error)
	List(ctx context.Context, input ListInput) (JobPage, error)
	Delete(ctx context.Context, userID, jobID string) error
	CreateQueued(ctx context.Context, job Job) error
	ClaimNext(ctx context.Context, owner string, now time.Time, lease time.Duration) (Job, error)
	Heartbeat(ctx context.Context, jobID, owner string, now time.Time, lease time.Duration) error
	SetResolved(ctx context.Context, jobID, owner string, work resolver.Work, at time.Time) error
	SetStage(ctx context.Context, jobID, owner, status string, progress int, message string, at time.Time) error
	CompleteDownload(ctx context.Context, jobID, owner string, fileID string, at time.Time) error
	RetryLater(ctx context.Context, jobID, owner, code, message string, retryAt, at time.Time) error
	CancelOwned(ctx context.Context, jobID, owner, message string, at time.Time) error
	Cancel(ctx context.Context, userID, jobID string, at time.Time) (bool, error)
	Retry(ctx context.Context, userID, jobID string, at time.Time) error
	Recover(ctx context.Context, now time.Time) (int64, error)
	BeginStep(ctx context.Context, step Step) error
	FinishStep(ctx context.Context, stepID, status, code, message string, details map[string]any, at time.Time) error
	FindFiles(ctx context.Context, userID, jobID string) ([]JobFile, error)
	DeleteFileRecord(ctx context.Context, fileID string) error
	FailExhaustedQueued(ctx context.Context, at time.Time) (int64, error)
	CompleteTranscription(ctx context.Context, jobID, owner string, result map[string]any, at time.Time) error
	DeleteFilesByKind(ctx context.Context, jobID, kind string) error
}

type JobFile struct { ID string `json:"id"`; Kind string `json:"kind"`; Name string `json:"name"`; MIMEType string `json:"mimeType"`; SizeBytes int64 `json:"sizeBytes"` }
