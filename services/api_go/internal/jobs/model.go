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
}

type JobError struct {
	Code string `json:"code"`
	Message string `json:"message"`
}

type CreateInput struct {
	UserID, ShareText, IdempotencyKey string
	Force bool
}

type Repository interface {
	FindByIdempotencyKey(ctx context.Context, userID, key string) (Job, error)
	CreateInfo(ctx context.Context, job Job) error
	CompleteInfo(ctx context.Context, jobID string, work resolver.Work, at time.Time) error
	Fail(ctx context.Context, jobID, code, message string, at time.Time) error
	FindByID(ctx context.Context, userID, jobID string) (Job, error)
}
