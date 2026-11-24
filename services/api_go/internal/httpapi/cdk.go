package httpapi

import (
	"errors"
	"net/http"
	"time"

	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/auth"
)

func registerCDKRoutes(mux *http.ServeMux, deps Dependencies, limiter *loginLimiter) {
	mux.HandleFunc("POST /api/v1/auth/cdk/login", func(w http.ResponseWriter, r *http.Request) {
		if deps.Auth == nil {
			writeError(w, r, http.StatusServiceUnavailable, "SERVICE_NOT_READY", "服务尚未就绪", true)
			return
		}
		if !sameOrigin(r) {
			writeError(w, r, http.StatusForbidden, "CSRF_REJECTED", "拒绝跨站 CDK 登录", false)
			return
		}
		if !limiter.Allow("ip|"+remoteIP(r.RemoteAddr), time.Now()) {
			writeError(w, r, http.StatusTooManyRequests, "RATE_LIMITED", "登录尝试过于频繁，请稍后重试", true)
			return
		}
		var input struct {
			Code   string      `json:"code"`
			Device auth.Device `json:"device"`
		}
		if err := decodeJSON(w, r, &input); err != nil {
			return
		}
		if input.Code == "" || len(input.Code) > 128 || !validLoginDevice(input.Device) {
			writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "请填写 CDK 密钥和设备信息", false)
			return
		}
		pair, err := deps.Auth.LoginCDK(r.Context(), input.Code, input.Device)
		if errors.Is(err, auth.ErrInvalidCDK) {
			writeError(w, r, http.StatusUnauthorized, "AUTH_INVALID_CDK", "CDK 密钥无效、已使用或已作废", false)
			return
		}
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "登录暂时不可用", true)
			return
		}
		w.Header().Set("Cache-Control", "private, no-store")
		web := isWebClient(input.Device.Platform, input.Device.AppVersion)
		if web {
			setRefreshCookie(w, r, pair.RefreshToken, time.Until(pair.RefreshTokenExpires))
		}
		recordAudit(deps.Audit, r, pair.User.ID, "auth.cdk_login", pair.SessionID, map[string]any{"platform": input.Device.Platform})
		writeJSON(w, http.StatusOK, tokenPayload(pair, RequestID(r.Context()), web))
	})
	if deps.Auth == nil {
		return
	}

	register := func(pattern string, handler func(http.ResponseWriter, *http.Request, auth.Principal)) {
		mux.Handle(pattern, requireAuth(deps.Auth, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			principal, _ := Principal(r.Context())
			if adminAllowed(principal, w, r) {
				w.Header().Set("Cache-Control", "private, no-store")
				handler(w, r, principal)
			}
		})))
	}
	register("GET /api/v1/admin/cdks", func(w http.ResponseWriter, r *http.Request, _ auth.Principal) {
		limit, limitErr := queryInt(r, "limit", 50)
		offset, offsetErr := queryInt(r, "offset", 0)
		if limitErr != nil || offsetErr != nil || limit < 1 || limit > 100 || offset < 0 {
			writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "分页参数不正确", false)
			return
		}
		items, total, err := deps.Auth.ListCDKs(r.Context(), limit, offset)
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "无法读取 CDK 密钥列表", true)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": total, "requestId": RequestID(r.Context())})
	})
	register("POST /api/v1/admin/cdks", func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		var input struct {
			Count     int `json:"count"`
			TaskLimit int `json:"taskLimit"`
		}
		if err := decodeJSON(w, r, &input); err != nil {
			return
		}
		items, err := deps.Auth.CreateCDKs(r.Context(), input.Count, input.TaskLimit)
		if err != nil {
			writeCDKError(w, r, err)
			return
		}
		recordAdminAudit(deps.Audit, r, principal.UserID, "admin.cdk.generate", "cdk", "", map[string]any{"count": input.Count, "taskLimit": input.TaskLimit})
		writeJSON(w, http.StatusCreated, map[string]any{"items": items, "requestId": RequestID(r.Context())})
	})
	register("PATCH /api/v1/admin/cdks/{id}", func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		var input struct {
			TaskLimit int `json:"taskLimit"`
		}
		if err := decodeJSON(w, r, &input); err != nil {
			return
		}
		item, err := deps.Auth.SetCDKTaskLimit(r.Context(), r.PathValue("id"), input.TaskLimit)
		if err != nil {
			writeCDKError(w, r, err)
			return
		}
		recordAdminAudit(deps.Audit, r, principal.UserID, "admin.cdk.set_task_limit", "cdk", item.ID, map[string]any{"taskLimit": item.TaskLimit})
		writeJSON(w, http.StatusOK, map[string]any{"item": item, "requestId": RequestID(r.Context())})
	})
	register("DELETE /api/v1/admin/cdks/{id}", func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		item, err := deps.Auth.RevokeCDK(r.Context(), r.PathValue("id"))
		if err != nil {
			writeCDKError(w, r, err)
			return
		}
		recordAdminAudit(deps.Audit, r, principal.UserID, "admin.cdk.revoke", "cdk", item.ID, nil)
		writeJSON(w, http.StatusOK, map[string]any{"item": item, "requestId": RequestID(r.Context())})
	})
}

func writeCDKError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, auth.ErrCDKInput):
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "请检查生成数量或任务次数", false)
	case errors.Is(err, auth.ErrNotFound):
		writeError(w, r, http.StatusNotFound, "CDK_NOT_FOUND", "CDK 密钥不存在", false)
	case errors.Is(err, auth.ErrCDKRevoked):
		writeError(w, r, http.StatusConflict, "CDK_REVOKED", "CDK 密钥已作废", false)
	default:
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "暂时无法管理 CDK 密钥", true)
	}
}
