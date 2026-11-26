package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/asr"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/audit"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/auth"
	ownedfiles "github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/files"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/jobs"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/media"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/resolver"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/settings"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/version"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/web"
)

const maxAuthBodyBytes = 16 * 1024
const refreshCookieName = "dc_refresh"
const webAppVersionPrefix = "web-"

type Readiness func() error

type Dependencies struct {
	Build           version.Info
	Auth            *auth.Service
	Jobs            *jobs.Service
	Files           *ownedfiles.Repository
	Storage         *ownedfiles.Storage
	Thumbnails      *media.Thumbnailer
	Covers          coverFetcher
	ASRSigner       *ownedfiles.Signer
	Audit           *audit.Recorder
	Ready           Readiness
	LoginRateLimit  int
	Settings        *settings.Service
	AliyunAvailable bool
	ASRProviderFactory func(settings.ProviderConfig) asr.Provider
	// AllowInsecureProviderSettings 允许在纯 HTTP（无 TLS）下配置 API Key。
	// 仅用于自托管的内网/私有部署；公网必须保持默认的 HTTPS 校验。
	AllowInsecureProviderSettings bool
}

type healthResponse struct {
	Status  string       `json:"status"`
	Version version.Info `json:"version"`
}

type userResponse struct {
	ID          string    `json:"id"`
	DisplayName string    `json:"displayName"`
	Role        auth.Role `json:"role"`
	IsCDK       bool      `json:"isCdk,omitempty"`
}

type tokenResponse struct {
	AccessToken           string       `json:"accessToken"`
	AccessTokenExpiresAt  time.Time    `json:"accessTokenExpiresAt"`
	RefreshToken          string       `json:"refreshToken,omitempty"`
	RefreshTokenExpiresAt time.Time    `json:"refreshTokenExpiresAt,omitempty"`
	User                  userResponse `json:"user"`
	RequestID             string       `json:"requestId"`
}

// isWebClient 识别网页版登录（网页端固定 platform=windows、appVersion 以 web- 开头）。
// 网页端的 refresh token 不返回给 JS，而是写入 httpOnly Cookie；原生客户端保持 JSON 返回。
func isWebClient(platform, appVersion string) bool {
	return platform == "windows" && strings.HasPrefix(strings.ToLower(strings.TrimSpace(appVersion)), webAppVersionPrefix)
}

func requestScheme(r *http.Request) string {
	if r.TLS != nil {
		return "https"
	}
	if strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https") {
		return "https"
	}
	return "http"
}

func cookieSecure(r *http.Request) bool {
	if requestScheme(r) == "https" {
		return true
	}
	address := net.ParseIP(remoteIP(r.RemoteAddr))
	return address != nil && address.IsLoopback()
}

// sameOrigin 校验 Origin 头与请求目标同源，作为 Cookie 认证下的 CSRF 纵深防御。
// 旧浏览器可能不带 Origin，此时依赖 SameSite=Strict 阻止跨站携带 Cookie。
func sameOrigin(r *http.Request) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return true
	}
	parsed, err := url.Parse(origin)
	if err != nil {
		return false
	}
	return parsed.Scheme == requestScheme(r) && parsed.Host == r.Host
}

func setRefreshCookie(w http.ResponseWriter, r *http.Request, token string, ttl time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name:     refreshCookieName,
		Value:    token,
		Path:     "/api/v1/auth",
		MaxAge:   int(ttl.Seconds()),
		HttpOnly: true,
		Secure:   cookieSecure(r),
		SameSite: http.SameSiteStrictMode,
	})
}

func clearRefreshCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     refreshCookieName,
		Value:    "",
		Path:     "/api/v1/auth",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   cookieSecure(r),
		SameSite: http.SameSiteStrictMode,
	})
}

func New(deps Dependencies) http.Handler {
	if deps.LoginRateLimit <= 0 {
		deps.LoginRateLimit = 5
	}
	mux := http.NewServeMux()
	limiter := newLoginLimiter(deps.LoginRateLimit)
	registerCDKRoutes(mux, deps, limiter)
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
		if deps.Auth == nil {
			writeError(w, r, http.StatusServiceUnavailable, "SERVICE_NOT_READY", "服务尚未就绪", true)
			return
		}
		handleLogin(deps, limiter, w, r)
	})
	mux.HandleFunc("POST /api/v1/auth/refresh", func(w http.ResponseWriter, r *http.Request) {
		if deps.Auth == nil {
			writeError(w, r, http.StatusServiceUnavailable, "SERVICE_NOT_READY", "服务尚未就绪", true)
			return
		}
		// 网页端：refresh token 只存在于 httpOnly Cookie，登录态恢复与续期都走 Cookie；
		// 原生客户端仍通过请求体携带 refresh token。
		cookie, cookieErr := r.Cookie(refreshCookieName)
		web := cookieErr == nil && cookie.Value != ""
		var refreshToken string
		if web {
			if !sameOrigin(r) {
				writeError(w, r, http.StatusForbidden, "CSRF_REJECTED", "拒绝跨站会话刷新", false)
				return
			}
			refreshToken = cookie.Value
		} else {
			var input struct {
				RefreshToken string `json:"refreshToken"`
			}
			if err := decodeJSON(w, r, &input); err != nil {
				return
			}
			if input.RefreshToken == "" || len(input.RefreshToken) > 512 {
				writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "请求格式不正确", false)
				return
			}
			refreshToken = input.RefreshToken
		}
		pair, err := deps.Auth.Refresh(r.Context(), refreshToken)
		if err != nil {
			writeError(w, r, http.StatusUnauthorized, "AUTH_SESSION_REVOKED", "会话已失效，请重新登录", false)
			return
		}
		if web {
			setRefreshCookie(w, r, pair.RefreshToken, time.Until(pair.RefreshTokenExpires))
		}
		recordAudit(deps.Audit, r, pair.User.ID, "auth.refresh", pair.SessionID, nil)
		writeJSON(w, http.StatusOK, tokenPayload(pair, RequestID(r.Context()), web))
	})

	protected := http.NewServeMux()
	protected.HandleFunc("POST /api/v1/auth/logout", func(w http.ResponseWriter, r *http.Request) {
		clearRefreshCookie(w, r)
		principal, _ := Principal(r.Context())
		if err := deps.Auth.Logout(r.Context(), principal.UserID, principal.SessionID); err != nil {
			writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "退出登录失败", true)
			return
		}
		recordAudit(deps.Audit, r, principal.UserID, "auth.logout", principal.SessionID, nil)
		writeJSON(w, http.StatusOK, map[string]any{"requestId": RequestID(r.Context())})
	})
	protected.HandleFunc("GET /api/v1/auth/sessions", func(w http.ResponseWriter, r *http.Request) {
		principal, _ := Principal(r.Context())
		sessions, err := deps.Auth.ListSessions(r.Context(), principal.UserID)
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "无法读取设备会话", true)
			return
		}
		type item struct {
			ID         string      `json:"id"`
			Device     auth.Device `json:"device"`
			Current    bool        `json:"current"`
			CreatedAt  time.Time   `json:"createdAt"`
			LastUsedAt time.Time   `json:"lastUsedAt"`
			ExpiresAt  time.Time   `json:"expiresAt"`
		}
		items := make([]item, 0, len(sessions))
		for _, session := range sessions {
			items = append(items, item{ID: session.ID, Device: session.Device, Current: session.ID == principal.SessionID, CreatedAt: session.CreatedAt, LastUsedAt: session.LastUsedAt, ExpiresAt: session.ExpiresAt})
		}
		writeJSON(w, http.StatusOK, map[string]any{"sessions": items, "requestId": RequestID(r.Context())})
	})
	protected.HandleFunc("DELETE /api/v1/auth/sessions/{id}", func(w http.ResponseWriter, r *http.Request) {
		principal, _ := Principal(r.Context())
		if err := deps.Auth.RevokeSession(r.Context(), principal, r.PathValue("id")); err != nil {
			writeError(w, r, http.StatusNotFound, "SESSION_NOT_FOUND", "设备会话不存在", false)
			return
		}
		recordAudit(deps.Audit, r, principal.UserID, "auth.session_revoke", r.PathValue("id"), nil)
		writeJSON(w, http.StatusOK, map[string]any{"requestId": RequestID(r.Context())})
	})
	protected.HandleFunc("POST /api/v1/auth/password", func(w http.ResponseWriter, r *http.Request) {
		principal, _ := Principal(r.Context())
		var input struct {
			CurrentPassword string `json:"currentPassword"`
			NewPassword     string `json:"newPassword"`
		}
		if err := decodeJSON(w, r, &input); err != nil {
			return
		}
		if input.CurrentPassword == "" || len(input.CurrentPassword) > 1024 {
			writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "当前密码不正确", false)
			return
		}
		if !auth.ValidNewPassword(input.NewPassword) {
			writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "新密码须为 6 至 12 位", false)
			return
		}
		if err := deps.Auth.ChangePassword(r.Context(), principal, input.CurrentPassword, input.NewPassword); errors.Is(err, auth.ErrInvalidCredentials) {
			writeError(w, r, http.StatusUnauthorized, "AUTH_INVALID_CREDENTIALS", "当前密码不正确", false)
			return
		} else if errors.Is(err, auth.ErrForbidden) {
			writeError(w, r, http.StatusForbidden, "FORBIDDEN", "CDK 会话不支持修改密码", false)
			return
		} else if errors.Is(err, auth.ErrNotFound) {
			writeError(w, r, http.StatusUnauthorized, "AUTH_TOKEN_EXPIRED", "登录状态已失效，请重新登录", true)
			return
		} else if err != nil {
			writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "修改密码失败", true)
			return
		}
		recordAudit(deps.Audit, r, principal.UserID, "auth.password_change", principal.SessionID, nil)
		writeJSON(w, http.StatusOK, map[string]any{"requestId": RequestID(r.Context())})
	})
	protected.HandleFunc("DELETE /api/v1/admin/sessions/{id}", func(w http.ResponseWriter, r *http.Request) {
		principal, _ := Principal(r.Context())
		if err := deps.Auth.RevokeAnySession(r.Context(), principal, r.PathValue("id")); errors.Is(err, auth.ErrForbidden) {
			writeError(w, r, http.StatusForbidden, "FORBIDDEN", "无权限执行此操作", false)
			return
		} else if err != nil {
			writeError(w, r, http.StatusNotFound, "SESSION_NOT_FOUND", "设备会话不存在", false)
			return
		}
		recordAudit(deps.Audit, r, principal.UserID, "admin.session_revoke", r.PathValue("id"), nil)
		writeJSON(w, http.StatusOK, map[string]any{"requestId": RequestID(r.Context())})
	})
	protected.HandleFunc("GET /api/v1/admin/stats", func(w http.ResponseWriter, r *http.Request) {
		principal, _ := Principal(r.Context())
		if !adminAllowed(principal, w, r) {
			return
		}
		users, activeUsers := 0, 0
		if deps.Auth != nil {
			userList, err := deps.Auth.ListUsers(r.Context())
			if err != nil {
				writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "无法读取统计信息", true)
				return
			}
			users = len(userList)
			for _, user := range userList {
				if user.IsActive {
					activeUsers++
				}
			}
		}
		stats := jobs.Stats{ByStatus: map[string]int{}, ByDay: []jobs.DayCount{}}
		if deps.Jobs != nil {
			var err error
			stats, err = deps.Jobs.Stats(r.Context())
			if err != nil {
				writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "无法读取统计信息", true)
				return
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"stats": map[string]any{
				"users": users, "activeUsers": activeUsers,
				"totalJobs": stats.Total, "todayJobs": stats.Today,
				"byStatus": stats.ByStatus, "byDay": stats.ByDay,
			},
			"requestId": RequestID(r.Context()),
		})
	})
	protected.HandleFunc("GET /api/v1/admin/users", func(w http.ResponseWriter, r *http.Request) {
		principal, _ := Principal(r.Context())
		if !adminAllowed(principal, w, r) {
			return
		}
		if deps.Auth == nil {
			writeError(w, r, http.StatusServiceUnavailable, "SERVICE_NOT_READY", "服务尚未就绪", true)
			return
		}
		users, err := deps.Auth.ListUsers(r.Context())
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "无法读取用户列表", true)
			return
		}
		var overrides map[string]int
		if deps.Settings != nil {
			overrides, err = deps.Settings.ListUserDailyTaskLimits(r.Context())
			if err != nil {
				writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "无法读取用户额度", true)
				return
			}
		}
		type item struct {
			ID          string     `json:"id"`
			Username    string     `json:"username"`
			DisplayName string     `json:"displayName"`
			Role        auth.Role  `json:"role"`
			IsActive    bool       `json:"isActive"`
			CreatedAt   time.Time  `json:"createdAt"`
			UpdatedAt   time.Time  `json:"updatedAt"`
			LastLoginAt *time.Time `json:"lastLoginAt,omitempty"`
			DailyTaskLimitOverride *int `json:"dailyTaskLimitOverride"`
		}
		items := make([]item, 0, len(users))
		for _, user := range users {
			entry := item{ID: user.ID, Username: user.UsernameNormalized, DisplayName: user.DisplayName, Role: user.Role, IsActive: user.IsActive, CreatedAt: user.CreatedAt, UpdatedAt: user.UpdatedAt, LastLoginAt: user.LastLoginAt}
			if limit, ok := overrides[user.ID]; ok {
				entry.DailyTaskLimitOverride = &limit
			}
			items = append(items, entry)
		}
		writeJSON(w, http.StatusOK, map[string]any{"users": items, "requestId": RequestID(r.Context())})
	})
	protected.HandleFunc("POST /api/v1/admin/users", func(w http.ResponseWriter, r *http.Request) {
		principal, _ := Principal(r.Context())
		if !adminAllowed(principal, w, r) {
			return
		}
		var input struct {
			Username    string    `json:"username"`
			DisplayName string    `json:"displayName"`
			Password    string    `json:"password"`
			Role        auth.Role `json:"role"`
		}
		if err := decodeJSON(w, r, &input); err != nil {
			return
		}
		if input.Role != auth.RoleAdmin && input.Role != auth.RoleUser {
			writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "角色必须是 admin 或 user", false)
			return
		}
		if !auth.ValidNewPassword(input.Password) {
			writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "密码须为 6 至 12 位", false)
			return
		}
		user, err := deps.Auth.CreateUser(r.Context(), input.Username, input.DisplayName, input.Password, input.Role)
		if errors.Is(err, auth.ErrUsernameTaken) {
			writeError(w, r, http.StatusConflict, "USERNAME_TAKEN", "用户名已存在", false)
			return
		}
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "创建用户失败，请检查用户名、显示名是否合法", false)
			return
		}
		recordAudit(deps.Audit, r, principal.UserID, "admin.user.create", user.ID, map[string]any{"role": string(user.Role)})
		writeJSON(w, http.StatusCreated, map[string]any{"user": userResponse{ID: user.ID, DisplayName: user.DisplayName, Role: user.Role}, "requestId": RequestID(r.Context())})
	})
	protected.HandleFunc("POST /api/v1/admin/users/{id}/password", func(w http.ResponseWriter, r *http.Request) {
		principal, _ := Principal(r.Context())
		if !adminAllowed(principal, w, r) {
			return
		}
		var input struct {
			Password string `json:"password"`
		}
		if err := decodeJSON(w, r, &input); err != nil {
			return
		}
		if !auth.ValidNewPassword(input.Password) {
			writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "密码须为 6 至 12 位", false)
			return
		}
		targetID := r.PathValue("id")
		if err := deps.Auth.ResetPassword(r.Context(), targetID, input.Password); errors.Is(err, auth.ErrNotFound) {
			writeError(w, r, http.StatusNotFound, "USER_NOT_FOUND", "用户不存在", false)
			return
		} else if err != nil {
			writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "重置密码失败", true)
			return
		}
		recordAudit(deps.Audit, r, principal.UserID, "admin.user.reset_password", targetID, nil)
		writeJSON(w, http.StatusOK, map[string]any{"requestId": RequestID(r.Context())})
	})
	protected.HandleFunc("PATCH /api/v1/admin/users/{id}/active", func(w http.ResponseWriter, r *http.Request) {
		principal, _ := Principal(r.Context())
		if !adminAllowed(principal, w, r) {
			return
		}
		var input struct {
			Active bool `json:"active"`
		}
		if err := decodeJSON(w, r, &input); err != nil {
			return
		}
		targetID := r.PathValue("id")
		if !input.Active && targetID == principal.UserID {
			writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "不能禁用当前登录的管理员账号", false)
			return
		}
		if err := deps.Auth.SetUserActive(r.Context(), targetID, input.Active); errors.Is(err, auth.ErrNotFound) {
			writeError(w, r, http.StatusNotFound, "USER_NOT_FOUND", "用户不存在", false)
			return
		} else if err != nil {
			writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "更新用户状态失败", true)
			return
		}
		recordAudit(deps.Audit, r, principal.UserID, "admin.user.set_active", targetID, map[string]any{"active": input.Active})
		writeJSON(w, http.StatusOK, map[string]any{"requestId": RequestID(r.Context())})
	})
	protected.HandleFunc("GET /api/v1/admin/users/{id}/sessions", func(w http.ResponseWriter, r *http.Request) {
		principal, _ := Principal(r.Context())
		if !adminAllowed(principal, w, r) {
			return
		}
		sessions, err := deps.Auth.ListSessions(r.Context(), r.PathValue("id"))
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "无法读取设备会话", true)
			return
		}
		type item struct {
			ID         string      `json:"id"`
			Device     auth.Device `json:"device"`
			Current    bool        `json:"current"`
			CreatedAt  time.Time   `json:"createdAt"`
			LastUsedAt time.Time   `json:"lastUsedAt"`
			ExpiresAt  time.Time   `json:"expiresAt"`
		}
		items := make([]item, 0, len(sessions))
		for _, session := range sessions {
			items = append(items, item{ID: session.ID, Device: session.Device, Current: session.UserID == principal.UserID && session.ID == principal.SessionID, CreatedAt: session.CreatedAt, LastUsedAt: session.LastUsedAt, ExpiresAt: session.ExpiresAt})
		}
		writeJSON(w, http.StatusOK, map[string]any{"sessions": items, "requestId": RequestID(r.Context())})
	})
	protected.HandleFunc("GET /api/v1/admin/jobs", func(w http.ResponseWriter, r *http.Request) {
		principal, _ := Principal(r.Context())
		if !adminAllowed(principal, w, r) {
			return
		}
		if deps.Jobs == nil {
			writeError(w, r, http.StatusServiceUnavailable, "SERVICE_NOT_READY", "服务尚未就绪", true)
			return
		}
		limit, limitErr := queryInt(r, "limit", 50)
		offset, offsetErr := queryInt(r, "offset", 0)
		status, action := strings.TrimSpace(r.URL.Query().Get("status")), strings.TrimSpace(r.URL.Query().Get("action"))
		userID := strings.TrimSpace(r.URL.Query().Get("userId"))
		if limitErr != nil || offsetErr != nil || limit < 1 || limit > 100 || offset < 0 || !validJobFilter(status, action) {
			writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "历史筛选参数不正确", false)
			return
		}
		input := jobs.ListInput{UserID: userID, Query: r.URL.Query().Get("q"), Status: status, Action: action, Limit: limit, Offset: offset}
		if r.URL.Query().Get("view") == "summary" {
			writeJobSummaries(w, r, deps.Jobs, input, true)
			return
		}
		page, err := deps.Jobs.ListAll(r.Context(), input)
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "无法读取任务列表", true)
			return
		}
		withMediaPreviews(page.Items, deps, time.Now())
		writeJSON(w, http.StatusOK, map[string]any{"jobs": page.Items, "total": page.Total, "limit": page.Limit, "offset": page.Offset, "requestId": RequestID(r.Context())})
	})
	protected.HandleFunc("GET /api/v1/admin/jobs/{id}", func(w http.ResponseWriter, r *http.Request) {
		principal, _ := Principal(r.Context())
		if !adminAllowed(principal, w, r) {
			return
		}
		if deps.Jobs == nil {
			writeError(w, r, http.StatusServiceUnavailable, "SERVICE_NOT_READY", "服务尚未就绪", true)
			return
		}
		job, err := deps.Jobs.GetAny(r.Context(), r.PathValue("id"))
		if errors.Is(err, jobs.ErrNotFound) {
			writeError(w, r, http.StatusNotFound, "JOB_NOT_FOUND", "任务不存在", false)
			return
		}
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "无法读取任务", true)
			return
		}
		withMediaPreviews([]jobs.Job{job}, deps, time.Now())
		writeJSON(w, http.StatusOK, map[string]any{"job": job, "requestId": RequestID(r.Context())})
	})
	protected.HandleFunc("POST /api/v1/admin/jobs/{id}/cancel", func(w http.ResponseWriter, r *http.Request) {
		principal, _ := Principal(r.Context())
		if !adminAllowed(principal, w, r) {
			return
		}
		jobID := r.PathValue("id")
		job, err := deps.Jobs.GetAny(r.Context(), jobID)
		if errors.Is(err, jobs.ErrNotFound) {
			writeError(w, r, http.StatusNotFound, "JOB_NOT_FOUND", "任务不存在", false)
			return
		}
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "无法读取任务", true)
			return
		}
		job, err = deps.Jobs.Cancel(r.Context(), job.UserID, jobID)
		if errors.Is(err, jobs.ErrNotFound) {
			writeError(w, r, http.StatusNotFound, "JOB_NOT_FOUND", "任务不存在", false)
			return
		}
		if errors.Is(err, jobs.ErrNotCancellable) {
			writeError(w, r, http.StatusConflict, "JOB_NOT_CANCELLABLE", "当前阶段不能即时取消", false)
			return
		}
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "取消任务失败", true)
			return
		}
		recordAudit(deps.Audit, r, principal.UserID, "admin.job.cancel", jobID, nil)
		writeJSON(w, http.StatusOK, map[string]any{"job": job, "requestId": RequestID(r.Context())})
	})
	protected.HandleFunc("POST /api/v1/admin/jobs/{id}/retry", func(w http.ResponseWriter, r *http.Request) {
		principal, _ := Principal(r.Context())
		if !adminAllowed(principal, w, r) {
			return
		}
		jobID := r.PathValue("id")
		job, err := deps.Jobs.GetAny(r.Context(), jobID)
		if errors.Is(err, jobs.ErrNotFound) {
			writeError(w, r, http.StatusNotFound, "JOB_NOT_FOUND", "任务不存在", false)
			return
		}
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "无法读取任务", true)
			return
		}
		job, err = deps.Jobs.Retry(r.Context(), job.UserID, jobID)
		if errors.Is(err, jobs.ErrNotFound) {
			writeError(w, r, http.StatusNotFound, "JOB_NOT_FOUND", "任务不存在", false)
			return
		}
		if errors.Is(err, jobs.ErrNotRetryable) {
			writeError(w, r, http.StatusConflict, "JOB_NOT_RETRYABLE", "当前任务不能重试", false)
			return
		}
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "重试任务失败", true)
			return
		}
		recordAudit(deps.Audit, r, principal.UserID, "admin.job.retry", jobID, nil)
		writeJSON(w, http.StatusAccepted, map[string]any{"job": job, "requestId": RequestID(r.Context())})
	})
	protected.HandleFunc("DELETE /api/v1/admin/jobs/{id}", func(w http.ResponseWriter, r *http.Request) {
		principal, _ := Principal(r.Context())
		if !adminAllowed(principal, w, r) {
			return
		}
		jobID := r.PathValue("id")
		job, findErr := deps.Jobs.GetAny(r.Context(), jobID)
		if errors.Is(findErr, jobs.ErrNotFound) {
			writeError(w, r, http.StatusNotFound, "JOB_NOT_FOUND", "任务不存在", false)
			return
		}
		if findErr != nil {
			writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "删除任务失败", true)
			return
		}
		if job.Status != "completed" && job.Status != "failed" && job.Status != "cancelled" {
			writeError(w, r, http.StatusConflict, "JOB_NOT_DELETABLE", "进行中的任务不能删除", false)
			return
		}
		var owned []ownedfiles.File
		if deps.Files != nil {
			owned, _ = deps.Files.ListByJob(r.Context(), job.UserID, jobID)
		}
		if deps.Storage != nil {
			for _, file := range owned {
				if removeErr := deps.Storage.Remove(file); removeErr != nil {
					writeError(w, r, http.StatusInternalServerError, "FILE_DELETE_FAILED", "删除任务文件失败", true)
					return
				}
			}
		}
		err := deps.Jobs.Delete(r.Context(), job.UserID, jobID)
		if errors.Is(err, jobs.ErrNotFound) {
			writeError(w, r, http.StatusNotFound, "JOB_NOT_FOUND", "任务不存在", false)
			return
		}
		if errors.Is(err, jobs.ErrNotDeletable) {
			writeError(w, r, http.StatusConflict, "JOB_NOT_DELETABLE", "进行中的任务不能删除", false)
			return
		}
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "删除任务失败", true)
			return
		}
		recordAudit(deps.Audit, r, principal.UserID, "admin.job.delete", jobID, nil)
		writeJSON(w, http.StatusOK, map[string]any{"requestId": RequestID(r.Context())})
	})
	protected.HandleFunc("GET /api/v1/admin/files/{id}", func(w http.ResponseWriter, r *http.Request) {
		principal, _ := Principal(r.Context())
		if !adminAllowed(principal, w, r) {
			return
		}
		if deps.Files == nil || deps.Storage == nil {
			writeError(w, r, http.StatusServiceUnavailable, "SERVICE_NOT_READY", "服务尚未就绪", true)
			return
		}
		file, err := deps.Files.FindByID(r.Context(), r.PathValue("id"))
		if errors.Is(err, ownedfiles.ErrNotFound) {
			writeError(w, r, http.StatusNotFound, "FILE_NOT_FOUND", "文件不存在", false)
			return
		}
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "无法读取文件", true)
			return
		}
		recordAudit(deps.Audit, r, principal.UserID, "admin.file.download", file.ID, nil)
		handleFileDownload(deps.Storage, file, w, r)
	})
	webFiles, err := fs.Sub(web.Files, "static")
	if err != nil {
		panic("invalid embedded web assets: " + err.Error())
	}
	webHandler := gzipStatic(http.FileServer(http.FS(webFiles)))
	webETag := web.StaticETag()
	mux.Handle("GET /", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", webETag)
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Vary", "Accept-Encoding")
		if r.Header.Get("If-None-Match") == webETag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		webHandler.ServeHTTP(w, r)
	}))
	protected.HandleFunc("POST /api/v1/jobs", func(w http.ResponseWriter, r *http.Request) {
		if deps.Jobs == nil {
			writeError(w, r, http.StatusServiceUnavailable, "SERVICE_NOT_READY", "服务尚未就绪", true)
			return
		}
		principal, _ := Principal(r.Context())
		key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
		if key == "" || len(key) > 128 {
			writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "缺少有效的 Idempotency-Key", false)
			return
		}
		var input struct {
			ShareText string `json:"shareText"`
			Action    string `json:"action"`
			Options   struct {
				Force         bool     `json:"force"`
				KeepVideo     bool     `json:"keepVideo"`
				LanguageHints []string `json:"languageHints"`
				Hotwords      []string `json:"hotwords"`
				ASRProvider   string   `json:"asrProvider"`
			} `json:"options"`
		}
		if err := decodeJSON(w, r, &input); err != nil {
			return
		}
		if (input.Action != "info" && input.Action != "download" && input.Action != "transcribe" && input.Action != "full") || strings.TrimSpace(input.ShareText) == "" || len(input.ShareText) > 4096 {
			writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "不支持的任务类型", false)
			return
		}
		transcription := input.Action == "transcribe" || input.Action == "full"
		requestedProvider := input.Options.ASRProvider
		if requestedProvider != "" && (!transcription || !settings.ValidProvider(requestedProvider)) {
			writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "转写服务选项不合法", false)
			return
		}
		policy, limit, err := userUsagePolicy(r.Context(), deps.Settings, principal.UserID)
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "无法读取任务使用设置", true)
			return
		}
		providerUnavailable := requestedProvider != ""
		if transcription && policy.AllowTranscribe && deps.Settings != nil {
			providers, err := deps.Settings.Providers(r.Context(), deps.AliyunAvailable)
			if err != nil {
				writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "无法读取转写服务配置", true)
				return
			}
			providerUnavailable = !providers.Allows(requestedProvider)
		}
		creation := jobs.CreateInput{
			UserID: principal.UserID, ShareText: input.ShareText, IdempotencyKey: key, Force: input.Options.Force,
			Action: input.Action, KeepVideo: input.Options.KeepVideo, LanguageHints: input.Options.LanguageHints, Hotwords: input.Options.Hotwords,
			DailyTaskLimit: limit, ActionDisabled: !policy.Allows(input.Action),
			ASRProvider: requestedProvider, ASRProviderUnavailable: providerUnavailable,
		}
		var job jobs.Job
		var reused bool
		if input.Action == "info" {
			job, reused, err = deps.Jobs.CreateInfo(r.Context(), creation)
		} else {
			job, reused, err = deps.Jobs.CreateDownload(r.Context(), creation)
		}
		if errors.Is(err, jobs.ErrCDKTaskLimit) {
			writeError(w, r, http.StatusTooManyRequests, "CDK_LIMIT_REACHED", "CDK 任务次数已用完", false)
			return
		}
		if errors.Is(err, auth.ErrRevokedSession) {
			writeError(w, r, http.StatusUnauthorized, "AUTH_SESSION_REVOKED", "登录状态已失效，请重新登录", false)
			return
		}
		if errors.Is(err, jobs.ErrDailyTaskLimit) {
			writeDailyTaskLimitError(deps, w, r, principal.UserID, limit)
			return
		}
		if errors.Is(err, jobs.ErrTaskActionDisabled) {
			writeError(w, r, http.StatusForbidden, "TASK_ACTION_DISABLED", "管理员已暂停新建此类任务", false)
			return
		}
		if errors.Is(err, jobs.ErrASRProviderUnavailable) {
			writeError(w, r, http.StatusForbidden, "ASR_PROVIDER_UNAVAILABLE", "转写服务不可用或已限制选择，请刷新后重试", false)
			return
		}
		if errors.Is(err, jobs.ErrIdempotencyConflict) {
			writeError(w, r, http.StatusConflict, "IDEMPOTENCY_CONFLICT", "Idempotency-Key 已用于其他请求", false)
			return
		}
		if errors.Is(err, jobs.ErrInvalidOptions) {
			writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "任务选项不合法", false)
			return
		}
		if err != nil {
			writeResolverError(w, r, err)
			return
		}
		recordJobAudit(deps.Audit, r, principal.UserID, job.ID, reused, job.Status)
		// 幂等键复用且任务已完成时，files 是 result_json 解码的 []any（无 expiresAt/预览地址），
		// 重新走一次 FindByID 注入类型化文件，保证保留期提示与预览地址与 GET 一致。
		// 重查失败直接报错，而不是静默返回缺 expiresAt/类型化形状的不一致响应。
		if reused && job.Status == "completed" {
			fresh, getErr := deps.Jobs.Get(r.Context(), principal.UserID, job.ID)
			if getErr != nil {
				writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "无法读取任务", true)
				return
			}
			job = fresh
		}
			withMediaPreviews([]jobs.Job{job}, deps, time.Now())
		writeJSON(w, http.StatusAccepted, map[string]any{"job": job, "reused": reused, "requestId": RequestID(r.Context())})
	})
	protected.HandleFunc("GET /api/v1/jobs", func(w http.ResponseWriter, r *http.Request) {
		if deps.Jobs == nil {
			writeError(w, r, http.StatusServiceUnavailable, "SERVICE_NOT_READY", "服务尚未就绪", true)
			return
		}
		principal, _ := Principal(r.Context())
		limit, limitErr := queryInt(r, "limit", 20)
		offset, offsetErr := queryInt(r, "offset", 0)
		status, action := strings.TrimSpace(r.URL.Query().Get("status")), strings.TrimSpace(r.URL.Query().Get("action"))
		if limitErr != nil || offsetErr != nil || limit < 1 || limit > 100 || offset < 0 || !validJobFilter(status, action) {
			writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "历史筛选参数不正确", false)
			return
		}
		input := jobs.ListInput{UserID: principal.UserID, Query: r.URL.Query().Get("q"), Status: status, Action: action, Limit: limit, Offset: offset}
		if r.URL.Query().Get("view") == "summary" {
			writeJobSummaries(w, r, deps.Jobs, input, false)
			return
		}
		page, err := deps.Jobs.List(r.Context(), input)
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "无法读取历史记录", true)
			return
		}
		withMediaPreviews(page.Items, deps, time.Now())
		writeJSON(w, http.StatusOK, map[string]any{"jobs": page.Items, "total": page.Total, "limit": page.Limit, "offset": page.Offset, "requestId": RequestID(r.Context())})
	})
	protected.HandleFunc("POST /api/v1/jobs/{id}/cancel", func(w http.ResponseWriter, r *http.Request) {
		principal, _ := Principal(r.Context())
		job, err := deps.Jobs.Cancel(r.Context(), principal.UserID, r.PathValue("id"))
		if errors.Is(err, jobs.ErrNotFound) {
			writeError(w, r, http.StatusNotFound, "JOB_NOT_FOUND", "任务不存在", false)
			return
		}
		if errors.Is(err, jobs.ErrNotCancellable) {
			writeError(w, r, http.StatusConflict, "JOB_NOT_CANCELLABLE", "当前阶段不能即时取消", false)
			return
		}
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "取消任务失败", true)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"job": job, "requestId": RequestID(r.Context())})
	})
	protected.HandleFunc("POST /api/v1/jobs/{id}/retry", func(w http.ResponseWriter, r *http.Request) {
		principal, _ := Principal(r.Context())
		job, err := deps.Jobs.Retry(r.Context(), principal.UserID, r.PathValue("id"))
		if errors.Is(err, jobs.ErrNotFound) {
			writeError(w, r, http.StatusNotFound, "JOB_NOT_FOUND", "任务不存在", false)
			return
		}
		if errors.Is(err, jobs.ErrNotRetryable) {
			writeError(w, r, http.StatusConflict, "JOB_NOT_RETRYABLE", "当前任务不能重试", false)
			return
		}
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "重试任务失败", true)
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]any{"job": job, "requestId": RequestID(r.Context())})
	})
	protected.HandleFunc("DELETE /api/v1/jobs/{id}", func(w http.ResponseWriter, r *http.Request) {
		principal, _ := Principal(r.Context())
		job, findErr := deps.Jobs.Get(r.Context(), principal.UserID, r.PathValue("id"))
		if errors.Is(findErr, jobs.ErrNotFound) {
			writeError(w, r, http.StatusNotFound, "JOB_NOT_FOUND", "任务不存在", false)
			return
		}
		if findErr != nil {
			writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "删除任务失败", true)
			return
		}
		if job.Status != "completed" && job.Status != "failed" && job.Status != "cancelled" {
			writeError(w, r, http.StatusConflict, "JOB_NOT_DELETABLE", "进行中的任务不能删除", false)
			return
		}
		var owned []ownedfiles.File
		if deps.Files != nil {
			owned, _ = deps.Files.ListByJob(r.Context(), principal.UserID, r.PathValue("id"))
		}
		if deps.Storage != nil {
			for _, file := range owned {
				if removeErr := deps.Storage.Remove(file); removeErr != nil {
					writeError(w, r, http.StatusInternalServerError, "FILE_DELETE_FAILED", "删除任务文件失败", true)
					return
				}
			}
		}
		err := deps.Jobs.Delete(r.Context(), principal.UserID, r.PathValue("id"))
		if errors.Is(err, jobs.ErrNotFound) {
			writeError(w, r, http.StatusNotFound, "JOB_NOT_FOUND", "任务不存在", false)
			return
		}
		if errors.Is(err, jobs.ErrNotDeletable) {
			writeError(w, r, http.StatusConflict, "JOB_NOT_DELETABLE", "进行中的任务不能删除", false)
			return
		}
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "删除任务失败", true)
			return
		}
		recordAudit(deps.Audit, r, principal.UserID, "job.delete", r.PathValue("id"), nil)
		writeJSON(w, http.StatusOK, map[string]any{"requestId": RequestID(r.Context())})
	})
	protected.HandleFunc("GET /api/v1/files/{id}", func(w http.ResponseWriter, r *http.Request) {
		if deps.Files == nil || deps.Storage == nil {
			writeError(w, r, http.StatusServiceUnavailable, "SERVICE_NOT_READY", "服务尚未就绪", true)
			return
		}
		principal, _ := Principal(r.Context())
		file, err := deps.Files.FindOwned(r.Context(), principal.UserID, r.PathValue("id"))
		if errors.Is(err, ownedfiles.ErrNotFound) {
			writeError(w, r, http.StatusNotFound, "FILE_NOT_FOUND", "文件不存在", false)
			return
		}
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "无法读取文件", true)
			return
		}
		handleFileDownload(deps.Storage, file, w, r)
	})
	mux.HandleFunc("GET /api/v1/asr-source/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if deps.ASRSigner == nil || !deps.ASRSigner.Validate(id, r.URL.Query().Get("expires"), r.URL.Query().Get("signature"), time.Now()) {
			http.NotFound(w, r)
			return
		}
		if id == asr.ConnectionSampleID {
			w.Header().Set("Content-Type", "audio/wav")
			w.Header().Set("Cache-Control", "private, no-store")
			http.ServeContent(w, r, "asr-probe.wav", time.Time{}, bytes.NewReader(asr.ConnectionSampleWAV()))
			return
		}
		if deps.Files == nil || deps.Storage == nil {
			http.NotFound(w, r)
			return
		}
		file, err := deps.Files.FindByID(r.Context(), id)
		if err != nil || file.Kind != "asr_source" {
			http.NotFound(w, r)
			return
		}
		handleFileDownload(deps.Storage, file, w, r)
	})
	// 媒体预览：同源签名地址，供 <img>/<video> 直接加载（无需 Authorization 头），
	// 避免外部 CDN 防盗链/区域受限导致预览图加载失败。
	mux.HandleFunc("GET /api/v1/media-preview/{id}", func(w http.ResponseWriter, r *http.Request) {
		if deps.Files == nil || deps.Storage == nil || deps.ASRSigner == nil || !deps.ASRSigner.Validate(r.PathValue("id"), r.URL.Query().Get("expires"), r.URL.Query().Get("signature"), time.Now()) {
			http.NotFound(w, r)
			return
		}
		file, err := deps.Files.FindByID(r.Context(), r.PathValue("id"))
		if err != nil || (file.Kind != "image" && file.Kind != "animated" && file.Kind != "video" && file.Kind != "music") {
			http.NotFound(w, r)
			return
		}
		handleInlinePreview(deps.Storage, file, w, r)
	})
	mux.HandleFunc("GET /api/v1/media-preview/{id}/thumbnail", func(w http.ResponseWriter, r *http.Request) {
		if deps.Files == nil || deps.Storage == nil || deps.Thumbnails == nil || deps.ASRSigner == nil {
			http.NotFound(w, r)
			return
		}
		id, query, now := r.PathValue("id"), r.URL.Query(), time.Now()
		if !deps.ASRSigner.ValidateThumbnail(id, query.Get("expires"), query.Get("signature"), now) {
			http.NotFound(w, r)
			return
		}
		file, err := deps.Files.FindByID(r.Context(), id)
		if err != nil || (file.ExpiresAt != nil && !file.ExpiresAt.After(now)) {
			http.NotFound(w, r)
			return
		}
		thumbnail, err := deps.Thumbnails.Ensure(r.Context(), deps.Storage, file)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		handleInlinePreview(deps.Storage, thumbnail, w, r)
	})
	mux.HandleFunc("GET /api/v1/job-cover/{id}", func(w http.ResponseWriter, r *http.Request) {
		handleCoverPreview(deps, w, r)
	})
	protected.HandleFunc("GET /api/v1/jobs/{id}", func(w http.ResponseWriter, r *http.Request) {
		if deps.Jobs == nil {
			writeError(w, r, http.StatusServiceUnavailable, "SERVICE_NOT_READY", "服务尚未就绪", true)
			return
		}
		principal, _ := Principal(r.Context())
		job, err := deps.Jobs.Get(r.Context(), principal.UserID, r.PathValue("id"))
		if errors.Is(err, jobs.ErrNotFound) {
			writeError(w, r, http.StatusNotFound, "JOB_NOT_FOUND", "任务不存在", false)
			return
		}
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "无法读取任务", true)
			return
		}
		withMediaPreviews([]jobs.Job{job}, deps, time.Now())
		writeJSON(w, http.StatusOK, map[string]any{"job": job, "requestId": RequestID(r.Context())})
	})
	protected.HandleFunc("POST /api/v1/jobs/{id}/images/archive-download", func(w http.ResponseWriter, r *http.Request) {
		handleArchiveDownloadLink(deps, w, r, false)
	})
	protected.HandleFunc("POST /api/v1/admin/jobs/{id}/images/archive-download", func(w http.ResponseWriter, r *http.Request) {
		handleArchiveDownloadLink(deps, w, r, true)
	})
	mux.HandleFunc("GET /api/v1/archive-download/{id}", func(w http.ResponseWriter, r *http.Request) {
		handleSignedArchiveDownload(deps, w, r)
	})
	protected.HandleFunc("GET /api/v1/jobs/{id}/images/archive", func(w http.ResponseWriter, r *http.Request) {
		principal, _ := Principal(r.Context())
		handleImageArchive(deps, principal.UserID, w, r)
	})
	registerUsageRoutes(mux, protected, deps)
	registerProviderRoutes(mux, protected, deps)
	if deps.Auth != nil {
		mux.Handle("POST /api/v1/auth/logout", requireAuth(deps.Auth, protected))
		mux.Handle("GET /api/v1/auth/sessions", requireAuth(deps.Auth, protected))
		mux.Handle("DELETE /api/v1/auth/sessions/{id}", requireAuth(deps.Auth, protected))
		mux.Handle("POST /api/v1/auth/password", requireAuth(deps.Auth, protected))
		mux.Handle("DELETE /api/v1/admin/sessions/{id}", requireAuth(deps.Auth, protected))
		mux.Handle("GET /api/v1/admin/stats", requireAuth(deps.Auth, protected))
		mux.Handle("POST /api/v1/jobs", requireAuth(deps.Auth, protected))
		mux.Handle("GET /api/v1/jobs", requireAuth(deps.Auth, protected))
		mux.Handle("GET /api/v1/jobs/{id}", requireAuth(deps.Auth, protected))
		mux.Handle("GET /api/v1/jobs/{id}/images/archive", requireAuth(deps.Auth, protected))
		mux.Handle("POST /api/v1/jobs/{id}/images/archive-download", requireAuth(deps.Auth, protected))
		mux.Handle("DELETE /api/v1/jobs/{id}", requireAuth(deps.Auth, protected))
		mux.Handle("POST /api/v1/jobs/{id}/cancel", requireAuth(deps.Auth, protected))
		mux.Handle("POST /api/v1/jobs/{id}/retry", requireAuth(deps.Auth, protected))
		mux.Handle("GET /api/v1/files/{id}", requireAuth(deps.Auth, protected))
		mux.Handle("GET /api/v1/admin/users", requireAuth(deps.Auth, protected))
		mux.Handle("POST /api/v1/admin/users", requireAuth(deps.Auth, protected))
		mux.Handle("POST /api/v1/admin/users/{id}/password", requireAuth(deps.Auth, protected))
		mux.Handle("PATCH /api/v1/admin/users/{id}/active", requireAuth(deps.Auth, protected))
		mux.Handle("GET /api/v1/admin/users/{id}/sessions", requireAuth(deps.Auth, protected))
		mux.Handle("GET /api/v1/admin/jobs", requireAuth(deps.Auth, protected))
		mux.Handle("GET /api/v1/admin/jobs/{id}", requireAuth(deps.Auth, protected))
		mux.Handle("POST /api/v1/admin/jobs/{id}/images/archive-download", requireAuth(deps.Auth, protected))
		mux.Handle("POST /api/v1/admin/jobs/{id}/cancel", requireAuth(deps.Auth, protected))
		mux.Handle("POST /api/v1/admin/jobs/{id}/retry", requireAuth(deps.Auth, protected))
		mux.Handle("DELETE /api/v1/admin/jobs/{id}", requireAuth(deps.Auth, protected))
		mux.Handle("GET /api/v1/admin/files/{id}", requireAuth(deps.Auth, protected))
	}
	return requestMiddleware(deps.Build.Version, mux)
}

func jobWorkID(job jobs.Job) string {
	if job.Work != nil && job.Work.DouyinWorkID != "" {
		return job.Work.DouyinWorkID
	}
	return job.ID
}

func writeJobSummaries(w http.ResponseWriter, r *http.Request, service *jobs.Service, input jobs.ListInput, withOwner bool) {
	page, err := service.ListSummaries(r.Context(), input, withOwner)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "无法读取任务列表", true)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusOK, map[string]any{"jobs": page.Items, "total": page.Total, "limit": page.Limit, "offset": page.Offset, "requestId": RequestID(r.Context())})
}

func queryInt(r *http.Request, name string, fallback int) (int, error) {
	value := strings.TrimSpace(r.URL.Query().Get(name))
	if value == "" {
		return fallback, nil
	}
	return strconv.Atoi(value)
}
func validJobFilter(status, action string) bool {
	// "processing" 是前端合并的筛选组（命中全部瞬态处理阶段）；单个真实状态仍允许，兼容旧 API 调用。
	statuses := map[string]bool{"": true, "processing": true, "queued": true, "resolving": true, "downloading": true, "extracting": true, "transcribing": true, "postprocessing": true, "retry_wait": true, "completed": true, "failed": true, "cancelled": true}
	actions := map[string]bool{"": true, "info": true, "download": true, "transcribe": true, "full": true}
	return statuses[status] && actions[action]
}

func handleFileDownload(storage *ownedfiles.Storage, file ownedfiles.File, w http.ResponseWriter, r *http.Request) {
	handle, err := storage.Open(file)
	if err != nil {
		writeError(w, r, http.StatusNotFound, "FILE_NOT_FOUND", "文件不存在", false)
		return
	}
	defer handle.Close()
	stat, err := handle.Stat()
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "无法读取文件", true)
		return
	}
	w.Header().Set("Content-Type", file.MIMEType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Disposition", "attachment; filename*=UTF-8''"+url.PathEscape(file.OriginalName))
	http.ServeContent(w, r, file.OriginalName, stat.ModTime(), handle)
}

func handleInlinePreview(storage *ownedfiles.Storage, file ownedfiles.File, w http.ResponseWriter, r *http.Request) {
	handle, err := storage.Open(file)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer handle.Close()
	stat, err := handle.Stat()
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", file.MIMEType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Disposition", "inline; filename*=UTF-8''"+url.PathEscape(file.OriginalName))
	w.Header().Set("Cache-Control", "private, max-age=3600")
	http.ServeContent(w, r, file.OriginalName, stat.ModTime(), handle)
}

// withMediaPreviews 为作品封面和本地媒体文件注入同源预览地址。
// files 只可能是 []jobs.JobFile：GET/List 通过 FindByID/FindFiles 注入类型化文件，复用幂等键的
// POST 也先重查一次，因此 result_json 解码出的 []any 分支在这里不可达。
func withMediaPreviews(items []jobs.Job, deps Dependencies, now time.Time) {
	signer := deps.ASRSigner
	if signer == nil || len(items) == 0 {
		return
	}
	// Keep signed URLs stable across job polling and page reloads within the hour.
	expires := now.UTC().Truncate(time.Hour).Add(24 * time.Hour)
	for i := range items {
		if items[i].Status == "completed" {
			addCoverPreview(deps, items[i].Work, items[i].ID, expires)
		}
		result, ok := items[i].Result.(map[string]any)
		if !ok {
			continue
		}
		files, ok := result["files"].([]jobs.JobFile)
		if !ok {
			continue
		}
		for j := range files {
			file := &files[j]
			hasThumbnail := ownedfiles.HasThumbnail(file.Kind)
			if hasThumbnail || file.Kind == "music" {
				file.PreviewURL = signer.PreviewURL(file.ID, expires)
			}
			if hasThumbnail && deps.Thumbnails != nil && (file.ExpiresAt == nil || file.ExpiresAt.After(now)) {
				file.ThumbnailURL = signer.ThumbnailURL(file.ID, expires)
			}
		}
		result["files"] = files
	}
}

func recordJobAudit(recorder *audit.Recorder, r *http.Request, userID, jobID string, reused bool, status string) {
	if recorder == nil {
		return
	}
	if err := recorder.Record(r.Context(), audit.Event{ActorUserID: userID, Action: "job.create", TargetType: "job", TargetID: jobID, RequestID: RequestID(r.Context()), IP: remoteIP(r.RemoteAddr), Metadata: map[string]any{"reused": reused, "status": status}}); err != nil {
		slog.ErrorContext(r.Context(), "audit record failed", "service", "api", "request_id", RequestID(r.Context()), "user_id", userID, "event", "audit_failure", "error_code", "AUDIT_WRITE_FAILED")
	}
}

func writeResolverError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, resolver.ErrInvalidShareLink):
		writeError(w, r, http.StatusBadRequest, "INVALID_SHARE_LINK", "未找到有效的抖音作品链接", false)
	case errors.Is(err, resolver.ErrURLNotAllowed):
		writeError(w, r, http.StatusBadRequest, "URL_NOT_ALLOWED", "链接目标不允许访问", false)
	case errors.Is(err, resolver.ErrWorkUnavailable):
		writeError(w, r, http.StatusNotFound, "DOUYIN_WORK_UNAVAILABLE", "作品不存在或不可访问", false)
	default:
		writeError(w, r, http.StatusUnprocessableEntity, "DOUYIN_RESOLVE_FAILED", "无法解析该作品，请确认链接仍然有效", false)
	}
}

func handleLogin(deps Dependencies, limiter *loginLimiter, w http.ResponseWriter, r *http.Request) {
	clientIP := remoteIP(r.RemoteAddr)
	var input struct {
		Username string      `json:"username"`
		Password string      `json:"password"`
		Device   auth.Device `json:"device"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		return
	}
	now, username := time.Now(), auth.NormalizeUsername(input.Username)
	if !limiter.Allow("ip|"+clientIP, now) || !limiter.Allow("account|"+username, now) {
		writeError(w, r, http.StatusTooManyRequests, "RATE_LIMITED", "登录尝试过于频繁，请稍后重试", true)
		return
	}
	if username == "" || len(username) > 64 || input.Password == "" || len(input.Password) > 1024 || !validLoginDevice(input.Device) {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "登录信息不完整", false)
		return
	}
	pair, err := deps.Auth.Login(r.Context(), auth.LoginInput{Username: username, Password: input.Password, Device: input.Device})
	if errors.Is(err, auth.ErrInvalidCredentials) || errors.Is(err, auth.ErrInactiveUser) {
		writeError(w, r, http.StatusUnauthorized, "AUTH_INVALID_CREDENTIALS", "用户名或密码错误", false)
		return
	}
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "登录暂时不可用", true)
		return
	}
	web := isWebClient(input.Device.Platform, input.Device.AppVersion)
	if web {
		setRefreshCookie(w, r, pair.RefreshToken, time.Until(pair.RefreshTokenExpires))
	}
	recordAudit(deps.Audit, r, pair.User.ID, "auth.login", pair.SessionID, map[string]any{"platform": input.Device.Platform})
	writeJSON(w, http.StatusOK, tokenPayload(pair, RequestID(r.Context()), web))
}

func validLoginDevice(device auth.Device) bool {
	return device.ID != "" && len(device.ID) <= 128 && device.Name != "" && len(device.Name) <= 128 &&
		device.AppVersion != "" && len(device.AppVersion) <= 64 && (device.Platform == "android" || device.Platform == "windows")
}

func recordAudit(recorder *audit.Recorder, r *http.Request, userID, action, sessionID string, metadata map[string]any) {
	if recorder == nil {
		return
	}
	if err := recorder.Record(r.Context(), audit.Event{ActorUserID: userID, Action: action, TargetType: "session", TargetID: sessionID, RequestID: RequestID(r.Context()), IP: remoteIP(r.RemoteAddr), Metadata: metadata}); err != nil {
		slog.ErrorContext(r.Context(), "audit record failed", "service", "api", "request_id", RequestID(r.Context()), "user_id", userID, "event", "audit_failure", "error_code", "AUDIT_WRITE_FAILED")
	}
}

func tokenPayload(pair auth.TokenPair, requestID string, web bool) tokenResponse {
	response := tokenResponse{AccessToken: pair.AccessToken, AccessTokenExpiresAt: pair.AccessTokenExpires, User: userResponse{ID: pair.User.ID, DisplayName: pair.User.DisplayName, Role: pair.User.Role, IsCDK: pair.User.IsCDK}, RequestID: requestID}
	if !web {
		response.RefreshToken = pair.RefreshToken
		response.RefreshTokenExpiresAt = pair.RefreshTokenExpires
	}
	return response
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxAuthBodyBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "请求格式不正确", false)
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "请求只能包含一个 JSON 对象", false)
		return errors.New("invalid trailing JSON data")
	}
	return nil
}

func remoteIP(value string) string {
	host, _, err := net.SplitHostPort(value)
	if err == nil {
		return host
	}
	return strings.TrimSpace(value)
}

func adminAllowed(principal auth.Principal, w http.ResponseWriter, r *http.Request) bool {
	if principal.Role != auth.RoleAdmin {
		writeError(w, r, http.StatusForbidden, "FORBIDDEN", "仅管理员可执行此操作", false)
		return false
	}
	return true
}

func secureSettingsRequest(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	address := net.ParseIP(remoteIP(r.RemoteAddr))
	return address != nil && address.IsLoopback()
}
