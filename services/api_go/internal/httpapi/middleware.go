package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/auth"
)

type statusRecorder struct {
	http.ResponseWriter
	status int
	wroteHeader bool
}

func (r *statusRecorder) WriteHeader(code int) {
	if r.wroteHeader { return }
	r.wroteHeader = true
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(body []byte) (int, error) {
	if !r.wroteHeader { r.WriteHeader(http.StatusOK) }
	return r.ResponseWriter.Write(body)
}

func requestMiddleware(buildVersion string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		id, err := auth.NewID(started)
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "服务暂时不可用", true)
			return
		}
		ctx := context.WithValue(r.Context(), requestIDKey, id)
		meta := &requestMeta{}
		ctx = context.WithValue(ctx, requestMetaKey, meta)
		w.Header().Set("X-Request-ID", id)
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(recorder, r.WithContext(ctx))
		slog.InfoContext(ctx, "http request", "service", "api", "version", buildVersion,
			"request_id", id, "event", "http_request", "method", r.Method, "path", r.URL.Path,
			"user_id", meta.userID, "status", recorder.status, "duration_ms", time.Since(started).Milliseconds())
	})
}

func requireAuth(service *auth.Service, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := strings.TrimSpace(r.Header.Get("Authorization"))
		scheme, token, found := strings.Cut(header, " ")
		if !found || !strings.EqualFold(scheme, "Bearer") || strings.TrimSpace(token) == "" {
			writeError(w, r, http.StatusUnauthorized, "AUTH_TOKEN_EXPIRED", "登录状态已失效，请重新登录", true)
			return
		}
		principal, err := service.ValidateAccess(r.Context(), strings.TrimSpace(token))
		if err != nil {
			code, retryable := "AUTH_TOKEN_EXPIRED", true
			if errors.Is(err, auth.ErrRevokedSession) {
				code, retryable = "AUTH_SESSION_REVOKED", false
			}
			writeError(w, r, http.StatusUnauthorized, code, "登录状态已失效，请重新登录", retryable)
			return
		}
		ctx := context.WithValue(r.Context(), principalKey, principal)
		if meta, ok := ctx.Value(requestMetaKey).(*requestMeta); ok { meta.userID = principal.UserID }
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

type loginLimiter struct {
	mu          sync.Mutex
	limit       int
	windows     map[string]*loginWindow
	lastCleanup time.Time
}

type loginWindow struct {
	started time.Time
	count   int
}

func newLoginLimiter(limit int) *loginLimiter {
	return &loginLimiter{limit: limit, windows: make(map[string]*loginWindow)}
}

func (l *loginLimiter) Allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.limit <= 0 {
		return false
	}
	if l.lastCleanup.IsZero() || now.Sub(l.lastCleanup) >= time.Minute {
		for candidate, window := range l.windows {
			if now.Sub(window.started) >= time.Minute {
				delete(l.windows, candidate)
			}
		}
		l.lastCleanup = now
	}
	if len(l.windows) >= 10_000 {
		return false
	}
	window := l.windows[key]
	if window == nil || now.Sub(window.started) >= time.Minute {
		l.windows[key] = &loginWindow{started: now, count: 1}
		return true
	}
	if window.count >= l.limit {
		return false
	}
	window.count++
	return true
}
