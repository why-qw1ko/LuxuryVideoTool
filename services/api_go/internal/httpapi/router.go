package httpapi

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/audit"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/auth"
	ownedfiles "github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/files"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/jobs"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/resolver"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/settings"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/version"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/web"
)

const maxAuthBodyBytes = 16 * 1024

type Readiness func() error

type Dependencies struct {
	Build           version.Info
	Auth            *auth.Service
	Jobs            *jobs.Service
	Files           *ownedfiles.Repository
	Storage         *ownedfiles.Storage
	ASRSigner       *ownedfiles.Signer
	Audit           *audit.Recorder
	Ready           Readiness
	LoginRateLimit  int
	Settings        *settings.Service
	AliyunAvailable bool
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
}

type tokenResponse struct {
	AccessToken           string       `json:"accessToken"`
	AccessTokenExpiresAt  time.Time    `json:"accessTokenExpiresAt"`
	RefreshToken          string       `json:"refreshToken"`
	RefreshTokenExpiresAt time.Time    `json:"refreshTokenExpiresAt"`
	User                  userResponse `json:"user"`
	RequestID             string       `json:"requestId"`
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
		pair, err := deps.Auth.Refresh(r.Context(), input.RefreshToken)
		if err != nil {
			writeError(w, r, http.StatusUnauthorized, "AUTH_SESSION_REVOKED", "会话已失效，请重新登录", false)
			return
		}
		recordAudit(deps.Audit, r, pair.User.ID, "auth.refresh", pair.SessionID, nil)
		writeJSON(w, http.StatusOK, tokenPayload(pair, RequestID(r.Context())))
	})

	protected := http.NewServeMux()
	protected.HandleFunc("POST /api/v1/auth/logout", func(w http.ResponseWriter, r *http.Request) {
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
	protected.HandleFunc("GET /api/v1/admin/settings/providers", func(w http.ResponseWriter, r *http.Request) {
		principal, _ := Principal(r.Context())
		if principal.Role != auth.RoleAdmin {
			writeError(w, r, http.StatusForbidden, "FORBIDDEN", "仅管理员可查看 API 配置", false)
			return
		}
		if deps.Settings == nil {
			writeError(w, r, http.StatusServiceUnavailable, "SERVICE_NOT_READY", "配置服务尚未就绪", true)
			return
		}
		status, err := deps.Settings.Status(r.Context())
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "无法读取 API 配置", true)
			return
		}
		status.AliyunAvailable = deps.AliyunAvailable
		writeJSON(w, http.StatusOK, map[string]any{"providers": status, "requestId": RequestID(r.Context())})
	})
	protected.HandleFunc("PUT /api/v1/admin/settings/providers", func(w http.ResponseWriter, r *http.Request) {
		principal, _ := Principal(r.Context())
		if principal.Role != auth.RoleAdmin {
			writeError(w, r, http.StatusForbidden, "FORBIDDEN", "仅管理员可修改 API 配置", false)
			return
		}
		if !secureSettingsRequest(r) && !deps.AllowInsecureProviderSettings {
			writeError(w, r, http.StatusForbidden, "HTTPS_REQUIRED", "远程配置 API Key 必须使用 HTTPS（自托管内网可设置环境变量 ALLOW_INSECURE_PROVIDER_SETTINGS=1 放行）", false)
			return
		}
		if deps.Settings == nil {
			writeError(w, r, http.StatusServiceUnavailable, "SERVICE_NOT_READY", "配置服务尚未就绪", true)
			return
		}
		var input struct {
			AliyunAPIKey      *string `json:"aliyunApiKey"`
			SiliconFlowAPIKey *string `json:"siliconFlowApiKey"`
			ASRModel          *string `json:"asrModel"`
		}
		if err := decodeJSON(w, r, &input); err != nil {
			return
		}
		if input.AliyunAPIKey == nil && input.SiliconFlowAPIKey == nil && input.ASRModel == nil {
			writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "至少提交一个 API Key 或模型配置", false)
			return
		}
		if input.AliyunAPIKey != nil {
			if err := deps.Settings.Set(r.Context(), settings.AliyunKey, *input.AliyunAPIKey, principal.UserID); err != nil {
				writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "阿里 API Key 无效", false)
				return
			}
		}
		if input.SiliconFlowAPIKey != nil {
			if err := deps.Settings.Set(r.Context(), settings.SiliconFlowKey, *input.SiliconFlowAPIKey, principal.UserID); err != nil {
				writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "硅基流动 API Key 无效", false)
				return
			}
		}
		if input.ASRModel != nil {
			if err := deps.Settings.Set(r.Context(), settings.ASRModel, *input.ASRModel, principal.UserID); err != nil {
				writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "模型配置无效", false)
				return
			}
		}
		recordAudit(deps.Audit, r, principal.UserID, "admin.provider_settings.update", "", map[string]any{"aliyunChanged": input.AliyunAPIKey != nil, "siliconFlowChanged": input.SiliconFlowAPIKey != nil, "modelChanged": input.ASRModel != nil})
		status, _ := deps.Settings.Status(r.Context())
		status.AliyunAvailable = deps.AliyunAvailable
		writeJSON(w, http.StatusOK, map[string]any{"providers": status, "requestId": RequestID(r.Context())})
	})
	webFiles, err := fs.Sub(web.Files, "static")
	if err != nil {
		panic("invalid embedded web assets: " + err.Error())
	}
	webHandler := http.FileServer(http.FS(webFiles))
	mux.Handle("GET /", webHandler)
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
			} `json:"options"`
		}
		if err := decodeJSON(w, r, &input); err != nil {
			return
		}
		if (input.Action != "info" && input.Action != "download" && input.Action != "transcribe" && input.Action != "full") || strings.TrimSpace(input.ShareText) == "" || len(input.ShareText) > 4096 {
			writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "不支持的任务类型", false)
			return
		}
		var job jobs.Job
		var reused bool
		var err error
		if input.Action == "info" {
			job, reused, err = deps.Jobs.CreateInfo(r.Context(), jobs.CreateInput{UserID: principal.UserID, ShareText: input.ShareText, IdempotencyKey: key, Force: input.Options.Force})
		} else {
			job, reused, err = deps.Jobs.CreateDownload(r.Context(), jobs.CreateInput{UserID: principal.UserID, ShareText: input.ShareText, IdempotencyKey: key, Action: input.Action, Force: input.Options.Force, KeepVideo: input.Options.KeepVideo, LanguageHints: input.Options.LanguageHints, Hotwords: input.Options.Hotwords})
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
		withMediaPreviews([]jobs.Job{job}, deps.ASRSigner, time.Now())
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
		page, err := deps.Jobs.List(r.Context(), jobs.ListInput{UserID: principal.UserID, Query: r.URL.Query().Get("q"), Status: status, Action: action, Limit: limit, Offset: offset})
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "无法读取历史记录", true)
			return
		}
		withMediaPreviews(page.Items, deps.ASRSigner, time.Now())
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
		if deps.Files == nil || deps.Storage == nil || deps.ASRSigner == nil || !deps.ASRSigner.Validate(r.PathValue("id"), r.URL.Query().Get("expires"), r.URL.Query().Get("signature"), time.Now()) {
			http.NotFound(w, r)
			return
		}
		file, err := deps.Files.FindByID(r.Context(), r.PathValue("id"))
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
		if err != nil || (file.Kind != "image" && file.Kind != "animated" && file.Kind != "video") {
			http.NotFound(w, r)
			return
		}
		handleInlinePreview(deps.Storage, file, w, r)
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
		withMediaPreviews([]jobs.Job{job}, deps.ASRSigner, time.Now())
		writeJSON(w, http.StatusOK, map[string]any{"job": job, "requestId": RequestID(r.Context())})
	})
	protected.HandleFunc("GET /api/v1/jobs/{id}/images/archive", func(w http.ResponseWriter, r *http.Request) {
		if deps.Jobs == nil || deps.Files == nil || deps.Storage == nil {
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
		if job.Status != "completed" {
			writeError(w, r, http.StatusConflict, "JOB_NOT_READY", "任务尚未完成，暂不能打包配图", false)
			return
		}
		owned, err := deps.Files.ListByJob(r.Context(), principal.UserID, r.PathValue("id"))
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "无法读取媒体文件", true)
			return
		}
		var media []ownedfiles.File
		for _, file := range owned {
			if file.Kind == "image" || file.Kind == "animated" {
				media = append(media, file)
			}
		}
		if len(media) == 0 {
			writeError(w, r, http.StatusNotFound, "FILES_NOT_FOUND", "该任务没有可打包的配图", false)
			return
		}
		base := jobWorkID(job)
		filename := base + "_images.zip"
		// 先写入临时文件，全部成功后再流式返回；中途任何错误都能正常返回 5xx，
		// 避免已返回 200 后才发现 zip 写入失败，客户端拿到截断的坏包。
		tmp, err := os.CreateTemp("", "douyin-images-*.zip")
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "无法创建打包临时文件", true)
			return
		}
		tmpPath := tmp.Name()
		defer os.Remove(tmpPath)
		defer tmp.Close()
		zw := zip.NewWriter(tmp)
		writeZipEntry := func(entryName string, file ownedfiles.File) error {
			handle, err := deps.Storage.Open(file)
			if err != nil {
				return err
			}
			defer handle.Close()
			entry, err := zw.Create(entryName)
			if err != nil {
				return err
			}
			_, err = io.Copy(entry, handle)
			return err
		}
		for index, file := range media {
			name := fmt.Sprintf("%02d_%s", index+1, url.PathEscape(file.OriginalName))
			if err := writeZipEntry(name, file); err != nil {
				_ = zw.Close()
				writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "打包配图失败", true)
				return
			}
		}
		if err := zw.Close(); err != nil {
			writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "打包配图失败", true)
			return
		}
		if _, err := tmp.Seek(0, io.SeekStart); err != nil {
			writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "打包配图失败", true)
			return
		}
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Disposition", "attachment; filename*=UTF-8''"+url.PathEscape(filename))
		http.ServeContent(w, r, filename, time.Now(), tmp)
	})
	if deps.Auth != nil {
		mux.Handle("POST /api/v1/auth/logout", requireAuth(deps.Auth, protected))
		mux.Handle("GET /api/v1/auth/sessions", requireAuth(deps.Auth, protected))
		mux.Handle("DELETE /api/v1/auth/sessions/{id}", requireAuth(deps.Auth, protected))
		mux.Handle("DELETE /api/v1/admin/sessions/{id}", requireAuth(deps.Auth, protected))
		mux.Handle("GET /api/v1/admin/settings/providers", requireAuth(deps.Auth, protected))
		mux.Handle("PUT /api/v1/admin/settings/providers", requireAuth(deps.Auth, protected))
		mux.Handle("POST /api/v1/jobs", requireAuth(deps.Auth, protected))
		mux.Handle("GET /api/v1/jobs", requireAuth(deps.Auth, protected))
		mux.Handle("GET /api/v1/jobs/{id}", requireAuth(deps.Auth, protected))
		mux.Handle("GET /api/v1/jobs/{id}/images/archive", requireAuth(deps.Auth, protected))
		mux.Handle("DELETE /api/v1/jobs/{id}", requireAuth(deps.Auth, protected))
		mux.Handle("POST /api/v1/jobs/{id}/cancel", requireAuth(deps.Auth, protected))
		mux.Handle("POST /api/v1/jobs/{id}/retry", requireAuth(deps.Auth, protected))
		mux.Handle("GET /api/v1/files/{id}", requireAuth(deps.Auth, protected))
	}
	return requestMiddleware(deps.Build.Version, mux)
}

func jobWorkID(job jobs.Job) string {
	if job.Work != nil && job.Work.DouyinWorkID != "" {
		return job.Work.DouyinWorkID
	}
	return job.ID
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

// withMediaPreviews 为图文/动图媒体文件注入同源预览地址（外部 CDN 不可用时的兜底显示来源）。
// files 只可能是 []jobs.JobFile：GET/List 通过 FindByID/FindFiles 注入类型化文件，复用幂等键的
// POST 也先重查一次，因此 result_json 解码出的 []any 分支在这里不可达。
func withMediaPreviews(items []jobs.Job, signer *ownedfiles.Signer, now time.Time) {
	if signer == nil || len(items) == 0 {
		return
	}
	expires := now.Add(24 * time.Hour)
	for i := range items {
		result, ok := items[i].Result.(map[string]any)
		if !ok {
			continue
		}
		files, ok := result["files"].([]jobs.JobFile)
		if !ok {
			continue
		}
		for j := range files {
			// video 也注入：视频预览/下载走同源签名地址直接流式传输，避免前端 fetch 整文件缓冲导致"半天才弹出"。
			if files[j].Kind == "image" || files[j].Kind == "animated" || files[j].Kind == "video" {
				files[j].PreviewURL = signer.PreviewURL(files[j].ID, expires)
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
		Username string `json:"username"`
		Password string `json:"password"`
		Device   struct {
			ID         string `json:"id"`
			Name       string `json:"name"`
			Platform   string `json:"platform"`
			AppVersion string `json:"appVersion"`
		} `json:"device"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		return
	}
	now, username := time.Now(), auth.NormalizeUsername(input.Username)
	if !limiter.Allow("ip|"+clientIP, now) || !limiter.Allow("account|"+username, now) {
		writeError(w, r, http.StatusTooManyRequests, "RATE_LIMITED", "登录尝试过于频繁，请稍后重试", true)
		return
	}
	if username == "" || len(username) > 64 || input.Password == "" || len(input.Password) > 1024 || input.Device.ID == "" || len(input.Device.ID) > 128 || input.Device.Name == "" || len(input.Device.Name) > 128 || input.Device.AppVersion == "" || len(input.Device.AppVersion) > 64 || (input.Device.Platform != "android" && input.Device.Platform != "windows") {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "登录信息不完整", false)
		return
	}
	pair, err := deps.Auth.Login(r.Context(), auth.LoginInput{Username: username, Password: input.Password, Device: auth.Device{ID: input.Device.ID, Name: input.Device.Name, Platform: input.Device.Platform, AppVersion: input.Device.AppVersion}})
	if errors.Is(err, auth.ErrInvalidCredentials) || errors.Is(err, auth.ErrInactiveUser) {
		writeError(w, r, http.StatusUnauthorized, "AUTH_INVALID_CREDENTIALS", "用户名或密码错误", false)
		return
	}
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "登录暂时不可用", true)
		return
	}
	recordAudit(deps.Audit, r, pair.User.ID, "auth.login", pair.SessionID, map[string]any{"platform": input.Device.Platform})
	writeJSON(w, http.StatusOK, tokenPayload(pair, RequestID(r.Context())))
}

func recordAudit(recorder *audit.Recorder, r *http.Request, userID, action, sessionID string, metadata map[string]any) {
	if recorder == nil {
		return
	}
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

func secureSettingsRequest(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	address := net.ParseIP(remoteIP(r.RemoteAddr))
	return address != nil && address.IsLoopback()
}
