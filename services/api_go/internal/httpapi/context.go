package httpapi

import (
	"context"

	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/auth"
)

type contextKey string

const (
	requestIDKey contextKey = "request_id"
	principalKey contextKey = "principal"
	requestMetaKey contextKey = "request_meta"
)

type requestMeta struct { userID string }

func RequestID(ctx context.Context) string {
	value, _ := ctx.Value(requestIDKey).(string)
	return value
}

func Principal(ctx context.Context) (auth.Principal, bool) {
	value, ok := ctx.Value(principalKey).(auth.Principal)
	return value, ok
}
