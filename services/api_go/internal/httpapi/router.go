package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/audit"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/auth"
	ownedfiles "github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/files"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/jobs"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/resolver"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/version"
)

const maxAuthBodyBytes = 16 * 1024

type Readiness func() error

type Dependencies struct {
	Build          version.Info
	Auth           *auth.Service
	Jobs           *jobs.Service
	Files          *ownedfiles.Repository
	Storage        *ownedfiles.Storage
	Audit          *audit.Recorder
	Ready          Readiness
	LoginRateLimit int
}

type healthResponse struct {
	Status  string       `json:"status"`
	Version version.Info `json:"version"`
}

type userResponse struct {
	ID          string    `json:"id"`
	DisplayName string    `json:"displayName"`
	Role        auth.Role `json:"role"`
}

type tokenResponse struct {
	AccessToken          string       `json:"accessToken"`
	AccessTokenExpiresAt time.Time    `json:"accessTokenExpiresAt"`
	RefreshToken         string       `json:"refreshToken"`
	RefreshTokenExpiresAt time.Time   `json:"refreshTokenExpiresAt"`
	User                 userResponse `json:"user"`
	RequestID            string       `json:"requestId"`
}

func New(deps Dependencies) http.Handler {
	if deps.LoginRateLimit <= 0 {
		deps.LoginRateLimit = 5
	}
	mux := http.NewServeMux()
	limiter := newLoginLimiter(deps.LoginRateLimit)
	mux.HandleFunc("GET /health/live", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, healthResponse{Status: "ok", Version: deps.Build})
	})
	mux.HandleFunc("GET /health/ready", func(w http.ResponseWriter, r *http.Request) {
		if deps.Ready == nil || deps.Ready() != nil {
			writeError(w, r, http.StatusServiceUnavailable, "SERVICE_NOT_READY", "服务尚未就绪", true)
			return
		}
		writeJSON(w, http.StatusOK, healthResponse{Status: "ready", Version: deps.Build})
	})
	mux.HandleFunc("POST /api/v1/auth/login", func(w http.ResponseWriter, r *http.Request) {
		if deps.Auth == nil { writeError(w, r, http.StatusServiceUnavailable, "SERVICE_NOT_READY", "服务尚未就绪", true); return }
		handleLogin(deps, limiter, w, r)
	})
	mux.HandleFunc("POST /api/v1/auth/refresh", func(w http.ResponseWriter, r *http.Request) {
		if deps.Auth == nil { writeError(w, r, http.StatusServiceUnavailable, "SERVICE_NOT_READY", "服务尚未就绪", true); return }
		var input struct { RefreshToken string `json:"refreshToken"` }
		if err := decodeJSON(w, r, &input); err != nil { return }
		if input.RefreshToken == "" || len(input.RefreshToken) > 512 { writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "请求格式不正确", false); return }
		pair, err := deps.Auth.Refresh(r.Context(), input.RefreshToken)
		if err != nil { writeError(w, r, http.StatusUnauthorized, "AUTH_SESSION_REVOKED", "会话已失效，请重新登录", false); return }
		recordAudit(deps.Audit, r, pair.User.ID, "auth.refresh", pair.SessionID, nil)
		writeJSON(w, http.StatusOK, tokenPayload(pair, RequestID(r.Context())))
	})

	protected := http.NewServeMux()
	protected.HandleFunc("POST /api/v1/auth/logout", func(w http.ResponseWriter, r *http.Request) {
		principal, _ := Principal(r.Context())
		if err := deps.Auth.Logout(r.Context(), principal.UserID, principal.SessionID); err != nil { writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "退出登录失败", true); return }
		recordAudit(deps.Audit, r, principal.UserID, "auth.logout", principal.SessionID, nil)
		writeJSON(w, http.StatusOK, map[string]any{"requestId": RequestID(r.Context())})
	})
	protected.HandleFunc("GET /api/v1/auth/sessions", func(w http.ResponseWriter, r *http.Request) {
		principal, _ := Principal(r.Context())
		sessions, err := deps.Auth.ListSessions(r.Context(), principal.UserID)
		if err != nil { writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "无法读取设备会话", true); return }
		type item struct {
			ID string `json:"id"`; Device auth.Device `json:"device"`; Current bool `json:"current"`
			CreatedAt time.Time `json:"createdAt"`; LastUsedAt time.Time `json:"lastUsedAt"`; ExpiresAt time.Time `json:"expiresAt"`
		}
		items := make([]item, 0, len(sessions))
		for _, session := range sessions { items = append(items, item{ID: session.ID, Device: session.Device, Current: session.ID == principal.SessionID, CreatedAt: session.CreatedAt, LastUsedAt: session.LastUsedAt, ExpiresAt: session.ExpiresAt}) }
		writeJSON(w, http.StatusOK, map[string]any{"sessions": items, "requestId": RequestID(r.Context())})
	})
	protected.HandleFunc("DELETE /api/v1/auth/sessions/{id}", func(w http.ResponseWriter, r *http.Request) {
		principal, _ := Principal(r.Context())
		if err := deps.Auth.RevokeSession(r.Context(), principal, r.PathValue("id")); err != nil { writeError(w, r, http.StatusNotFound, "SESSION_NOT_FOUND", "设备会话不存在", false); return }
		recordAudit(deps.Audit, r, principal.UserID, "auth.session_revoke", r.PathValue("id"), nil)
		writeJSON(w, http.StatusOK, map[string]any{"requestId": RequestID(r.Context())})
	})
	protected.HandleFunc("DELETE /api/v1/admin/sessions/{id}", func(w http.ResponseWriter, r *http.Request) {
		principal, _ := Principal(r.Context())
		if err := deps.Auth.RevokeAnySession(r.Context(), principal, r.PathValue("id")); errors.Is(err, auth.ErrForbidden) { writeError(w, r, http.StatusForbidden, "FORBIDDEN", "无权限执行此操作", false); return } else if err != nil { writeError(w, r, http.StatusNotFound, "SESSION_NOT_FOUND", "设备会话不存在", false); return }
		recordAudit(deps.Audit, r, principal.UserID, "admin.session_revoke", r.PathValue("id"), nil)
		writeJSON(w, http.StatusOK, map[string]any{"requestId": RequestID(r.Context())})
	})
	protected.HandleFunc("POST /api/v1/jobs", func(w http.ResponseWriter, r *http.Request) {
		if deps.Jobs == nil { writeError(w, r, http.StatusServiceUnavailable, "SERVICE_NOT_READY", "服务尚未就绪", true); return }
		principal, _ := Principal(r.Context())
		key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
		if key == "" || len(key) > 128 { writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "缺少有效的 Idempotency-Key", false); return }
		var input struct {
			ShareText string `json:"shareText"`
			Action string `json:"action"`
			Options struct { Force bool `json:"force"` } `json:"options"`
		}
		if err := decodeJSON(w, r, &input); err != nil { return }
		if (input.Action != "info" && input.Action != "download") || strings.TrimSpace(input.ShareText) == "" || len(input.ShareText) > 4096 { writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "仅支持有效的 info 或 download 任务", false); return }
		var job jobs.Job; var reused bool; var err error
		if input.Action == "info" { job, reused, err = deps.Jobs.CreateInfo(r.Context(), jobs.CreateInput{UserID: principal.UserID, ShareText: input.ShareText, IdempotencyKey: key, Force: input.Options.Force})
		} else { job, reused, err = deps.Jobs.CreateDownload(r.Context(), jobs.CreateInput{UserID: principal.UserID, ShareText: input.ShareText, IdempotencyKey: key, Force: input.Options.Force}) }
		if errors.Is(err, jobs.ErrIdempotencyConflict) { writeError(w, r, http.StatusConflict, "IDEMPOTENCY_CONFLICT", "Idempotency-Key 已用于其他请求", false); return }
		if err != nil { writeResolverError(w, r, err); return }
		recordJobAudit(deps.Audit, r, principal.UserID, job.ID, reused, job.Status)
		writeJSON(w, http.StatusAccepted, map[string]any{"job": job, "reused": reused, "requestId": RequestID(r.Context())})
	})
	protected.HandleFunc("POST /api/v1/jobs/{id}/cancel", func(w http.ResponseWriter, r *http.Request) {
		principal, _ := Principal(r.Context()); job, err := deps.Jobs.Cancel(r.Context(), principal.UserID, r.PathValue("id"))
		if errors.Is(err, jobs.ErrNotFound) { writeError(w, r, http.StatusNotFound, "JOB_NOT_FOUND", "任务不存在", false); return }
		if errors.Is(err, jobs.ErrNotCancellable) { writeError(w, r, http.StatusConflict, "JOB_NOT_CANCELLABLE", "当前阶段不能即时取消", false); return }
		if err != nil { writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "取消任务失败", true); return }; writeJSON(w, http.StatusOK, map[string]any{"job": job, "requestId": RequestID(r.Context())})
	})
	protected.HandleFunc("POST /api/v1/jobs/{id}/retry", func(w http.ResponseWriter, r *http.Request) {
		principal, _ := Principal(r.Context()); job, err := deps.Jobs.Retry(r.Context(), principal.UserID, r.PathValue("id"))
		if errors.Is(err, jobs.ErrNotFound) { writeError(w, r, http.StatusNotFound, "JOB_NOT_FOUND", "任务不存在", false); return }
		if errors.Is(err, jobs.ErrNotRetryable) { writeError(w, r, http.StatusConflict, "JOB_NOT_RETRYABLE", "当前任务不能重试", false); return }
		if err != nil { writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "重试任务失败", true); return }; writeJSON(w, http.StatusAccepted, map[string]any{"job": job, "requestId": RequestID(r.Context())})
	})
	protected.HandleFunc("GET /api/v1/files/{id}", func(w http.ResponseWriter, r *http.Request) {
		if deps.Files == nil || deps.Storage == nil { writeError(w, r, http.StatusServiceUnavailable, "SERVICE_NOT_READY", "服务尚未就绪", true); return }
		principal, _ := Principal(r.Context()); file, err := deps.Files.FindOwned(r.Context(), principal.UserID, r.PathValue("id"))
		if errors.Is(err, ownedfiles.ErrNotFound) { writeError(w, r, http.StatusNotFound, "FILE_NOT_FOUND", "文件不存在", false); return }
		if err != nil { writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "无法读取文件", true); return }
		handleFileDownload(deps.Storage, file, w, r)
	})
	protected.HandleFunc("GET /api/v1/jobs/{id}", func(w http.ResponseWriter, r *http.Request) {
		if deps.Jobs == nil { writeError(w, r, http.StatusServiceUnavailable, "SERVICE_NOT_READY", "服务尚未就绪", true); return }
		principal, _ := Principal(r.Context())
		job, err := deps.Jobs.Get(r.Context(), principal.UserID, r.PathValue("id"))
		if errors.Is(err, jobs.ErrNotFound) { writeError(w, r, http.StatusNotFound, "JOB_NOT_FOUND", "任务不存在", false); return }
		if err != nil { writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "无法读取任务", true); return }
		writeJSON(w, http.StatusOK, map[string]any{"job": job, "requestId": RequestID(r.Context())})
	})
	if deps.Auth != nil {
		mux.Handle("POST /api/v1/auth/logout", requireAuth(deps.Auth, protected))
		mux.Handle("GET /api/v1/auth/sessions", requireAuth(deps.Auth, protected))
		mux.Handle("DELETE /api/v1/auth/sessions/{id}", requireAuth(deps.Auth, protected))
		mux.Handle("DELETE /api/v1/admin/sessions/{id}", requireAuth(deps.Auth, protected))
		mux.Handle("POST /api/v1/jobs", requireAuth(deps.Auth, protected))
		mux.Handle("GET /api/v1/jobs/{id}", requireAuth(deps.Auth, protected))
		mux.Handle("POST /api/v1/jobs/{id}/cancel", requireAuth(deps.Auth, protected))
		mux.Handle("POST /api/v1/jobs/{id}/retry", requireAuth(deps.Auth, protected))
		mux.Handle("GET /api/v1/files/{id}", requireAuth(deps.Auth, protected))
	}
	return requestMiddleware(deps.Build.Version, mux)
}

func handleFileDownload(storage *ownedfiles.Storage, file ownedfiles.File, w http.ResponseWriter, r *http.Request) {
	handle, err := storage.Open(file); if err != nil { writeError(w, r, http.StatusNotFound, "FILE_NOT_FOUND", "文件不存在", false); return }; defer handle.Close()
	stat, err := handle.Stat(); if err != nil { writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "无法读取文件", true); return }
	w.Header().Set("Content-Type", file.MIMEType); w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Disposition", "attachment; filename*=UTF-8''"+url.PathEscape(file.OriginalName))
	http.ServeContent(w, r, file.OriginalName, stat.ModTime(), handle)
}

func recordJobAudit(recorder *audit.Recorder, r *http.Request, userID, jobID string, reused bool, status string) {
	if recorder == nil { return }
	if err := recorder.Record(r.Context(), audit.Event{ActorUserID: userID, Action: "job.create", TargetType: "job", TargetID: jobID, RequestID: RequestID(r.Context()), IP: remoteIP(r.RemoteAddr), Metadata: map[string]any{"reused": reused, "status": status}}); err != nil {
		slog.ErrorContext(r.Context(), "audit record failed", "service", "api", "request_id", RequestID(r.Context()), "user_id", userID, "event", "audit_failure", "error_code", "AUDIT_WRITE_FAILED")
	}
}

func writeResolverError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, resolver.ErrInvalidShareLink): writeError(w, r, http.StatusBadRequest, "INVALID_SHARE_LINK", "未找到有效的抖音作品链接", false)
	case errors.Is(err, resolver.ErrURLNotAllowed): writeError(w, r, http.StatusBadRequest, "URL_NOT_ALLOWED", "链接目标不允许访问", false)
	case errors.Is(err, resolver.ErrWorkUnavailable): writeError(w, r, http.StatusNotFound, "DOUYIN_WORK_UNAVAILABLE", "作品不存在或不可访问", false)
	default: writeError(w, r, http.StatusUnprocessableEntity, "DOUYIN_RESOLVE_FAILED", "无法解析该作品，请确认链接仍然有效", false)
	}
}

func handleLogin(deps Dependencies, limiter *loginLimiter, w http.ResponseWriter, r *http.Request) {
	clientIP := remoteIP(r.RemoteAddr)
	var input struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Device struct { ID string `json:"id"`; Name string `json:"name"`; Platform string `json:"platform"`; AppVersion string `json:"appVersion"` } `json:"device"`
	}
	if err := decodeJSON(w, r, &input); err != nil { return }
	now, username := time.Now(), auth.NormalizeUsername(input.Username)
	if !limiter.Allow("ip|"+clientIP, now) || !limiter.Allow("account|"+username, now) { writeError(w, r, http.StatusTooManyRequests, "RATE_LIMITED", "登录尝试过于频繁，请稍后重试", true); return }
	if username == "" || len(username) > 64 || input.Password == "" || len(input.Password) > 1024 || input.Device.ID == "" || len(input.Device.ID) > 128 || input.Device.Name == "" || len(input.Device.Name) > 128 || input.Device.AppVersion == "" || len(input.Device.AppVersion) > 64 || (input.Device.Platform != "android" && input.Device.Platform != "windows") {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "登录信息不完整", false); return
	}
	pair, err := deps.Auth.Login(r.Context(), auth.LoginInput{Username: username, Password: input.Password, Device: auth.Device{ID: input.Device.ID, Name: input.Device.Name, Platform: input.Device.Platform, AppVersion: input.Device.AppVersion}})
	if errors.Is(err, auth.ErrInvalidCredentials) || errors.Is(err, auth.ErrInactiveUser) { writeError(w, r, http.StatusUnauthorized, "AUTH_INVALID_CREDENTIALS", "用户名或密码错误", false); return }
	if err != nil { writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "登录暂时不可用", true); return }
	recordAudit(deps.Audit, r, pair.User.ID, "auth.login", pair.SessionID, map[string]any{"platform": input.Device.Platform})
	writeJSON(w, http.StatusOK, tokenPayload(pair, RequestID(r.Context())))
}

func recordAudit(recorder *audit.Recorder, r *http.Request, userID, action, sessionID string, metadata map[string]any) {
	if recorder == nil { return }
	if err := recorder.Record(r.Context(), audit.Event{ActorUserID: userID, Action: action, TargetType: "session", TargetID: sessionID, RequestID: RequestID(r.Context()), IP: remoteIP(r.RemoteAddr), Metadata: metadata}); err != nil {
		slog.ErrorContext(r.Context(), "audit record failed", "service", "api", "request_id", RequestID(r.Context()), "user_id", userID, "event", "audit_failure", "error_code", "AUDIT_WRITE_FAILED")
	}
}

func tokenPayload(pair auth.TokenPair, requestID string) tokenResponse {
	return tokenResponse{AccessToken: pair.AccessToken, AccessTokenExpiresAt: pair.AccessTokenExpires, RefreshToken: pair.RefreshToken, RefreshTokenExpiresAt: pair.RefreshTokenExpires, User: userResponse{ID: pair.User.ID, DisplayName: pair.User.DisplayName, Role: pair.User.Role}, RequestID: requestID}
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxAuthBodyBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil { writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "请求格式不正确", false); return err }
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) { writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "请求只能包含一个 JSON 对象", false); return errors.New("invalid trailing JSON data") }
	return nil
}

func remoteIP(value string) string {
	host, _, err := net.SplitHostPort(value)
	if err == nil { return host }
	return strings.TrimSpace(value)
}
