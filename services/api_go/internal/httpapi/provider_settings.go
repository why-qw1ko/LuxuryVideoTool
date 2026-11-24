package httpapi

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/asr"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/settings"
)

const providerProbeTimeout = 40 * time.Second

func registerProviderRoutes(mux, protected *http.ServeMux, deps Dependencies) {
	if deps.Auth == nil {
		return
	}
	register := func(pattern string, route func(http.ResponseWriter, *http.Request, string), requireSecure bool) {
		protected.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			principal, _ := Principal(r.Context())
			if !adminAllowed(principal, w, r) || !usageSettingsReady(deps, w, r) {
				return
			}
			if requireSecure && !secureSettingsRequest(r) && !deps.AllowInsecureProviderSettings {
				writeError(w, r, http.StatusForbidden, "HTTPS_REQUIRED", "远程配置转写服务必须使用 HTTPS", false)
				return
			}
			w.Header().Set("Cache-Control", "private, no-store")
			route(w, r, principal.UserID)
		})
		mux.Handle(pattern, requireAuth(deps.Auth, protected))
	}

	register("GET /api/v1/admin/settings/providers", func(w http.ResponseWriter, r *http.Request, _ string) {
		current, err := deps.Settings.Providers(r.Context(), deps.AliyunAvailable)
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "无法读取转写服务配置", true)
			return
		}
		writeProviderStatus(w, r, current.Status)
	}, false)
	register("PUT /api/v1/admin/settings/providers", func(w http.ResponseWriter, r *http.Request, actorID string) {
		var update settings.ProviderUpdate
		if err := decodeJSON(w, r, &update); err != nil {
			return
		}
		status, err := deps.Settings.UpdateProviders(r.Context(), update, actorID, deps.AliyunAvailable)
		if err != nil {
			writeProviderSettingsError(w, r, err)
			return
		}
		recordAdminAudit(deps.Audit, r, actorID, "admin.provider_settings.update", "runtime_settings", "asr_providers", map[string]any{
			"aliyunChanged": update.AliyunAPIKey != nil, "siliconFlowChanged": update.SiliconFlowAPIKey != nil,
			"deepSeekChanged": update.DeepSeekAPIKey != nil, "modelChanged": update.ASRModel != nil,
			"policyChanged": update.DefaultProvider != nil || update.EnabledProviders != nil || update.AllowUserSelection != nil,
		})
		writeProviderStatus(w, r, status)
	}, true)
	register("DELETE /api/v1/admin/settings/providers/{id}/key", func(w http.ResponseWriter, r *http.Request, actorID string) {
		id := r.PathValue("id")
		empty := ""
		var update settings.ProviderUpdate
		switch id {
		case settings.ProviderAliyun:
			update.AliyunAPIKey = &empty
		case settings.ProviderSiliconFlow:
			update.SiliconFlowAPIKey = &empty
		case settings.ProviderDeepSeek:
			update.DeepSeekAPIKey = &empty
		default:
			writeError(w, r, http.StatusNotFound, "ASR_PROVIDER_NOT_FOUND", "转写服务不存在", false)
			return
		}
		status, err := deps.Settings.UpdateProviders(r.Context(), update, actorID, deps.AliyunAvailable)
		if err != nil {
			writeProviderSettingsError(w, r, err)
			return
		}
		recordAdminAudit(deps.Audit, r, actorID, "admin.provider_settings.clear_key", "asr_provider", id, nil)
		writeProviderStatus(w, r, status)
	}, true)

	inFlight := make(chan struct{}, 1)
	register("POST /api/v1/admin/settings/providers/{id}/test", func(w http.ResponseWriter, r *http.Request, actorID string) {
		id := r.PathValue("id")
		if !settings.ValidProvider(id) {
			writeError(w, r, http.StatusNotFound, "ASR_PROVIDER_NOT_FOUND", "转写服务不存在", false)
			return
		}
		current, err := deps.Settings.Providers(r.Context(), deps.AliyunAvailable)
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "无法读取转写服务配置", true)
			return
		}
		config := current.Config(id)
		if !config.Configured {
			writeError(w, r, http.StatusConflict, "ASR_TEST_NOT_CONFIGURED", "请先保存该服务的 API Key", false)
			return
		}
		if !config.Available || (id == settings.ProviderAliyun && !deps.ASRSigner.Enabled()) {
			writeError(w, r, http.StatusConflict, "ASR_TEST_NOT_AVAILABLE", "阿里云需要可公开访问的 HTTPS 站点地址", false)
			return
		}
		if deps.ASRProviderFactory == nil {
			writeError(w, r, http.StatusServiceUnavailable, "SERVICE_NOT_READY", "转写服务尚未就绪", true)
			return
		}
		provider := deps.ASRProviderFactory(config)
		if provider == nil {
			writeError(w, r, http.StatusServiceUnavailable, "SERVICE_NOT_READY", "转写服务尚未就绪", true)
			return
		}
		select {
		case inFlight <- struct{}{}:
			defer func() { <-inFlight }()
		default:
			writeError(w, r, http.StatusConflict, "ASR_TEST_BUSY", "另一项连接测试正在进行", true)
			return
		}
		start := time.Now()
		ctx, cancel := context.WithTimeout(r.Context(), providerProbeTimeout)
		defer cancel()
		var sampleURL string
		if id == settings.ProviderAliyun {
			sampleURL = deps.ASRSigner.URL(asr.ConnectionSampleID, start.Add(2*time.Minute))
		}
		err = asr.ProbeProvider(ctx, provider, sampleURL)
		recordAdminAudit(deps.Audit, r, actorID, "admin.provider_settings.test", "asr_provider", id, map[string]any{"success": err == nil})
		if err != nil {
			writeProviderProbeError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"provider": id, "durationMs": time.Since(start).Milliseconds(), "message": "测试音频已成功处理",
			"requestId": RequestID(r.Context()),
		})
	}, true)
}

func writeProviderStatus(w http.ResponseWriter, r *http.Request, status settings.ProviderStatusList) {
	writeJSON(w, http.StatusOK, map[string]any{"providers": status, "requestId": RequestID(r.Context())})
}

func writeProviderSettingsError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, settings.ErrInvalidProviders) {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "转写服务配置无效，默认服务须为已启用的服务", false)
		return
	}
	writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "无法保存转写服务配置", true)
}

func writeProviderProbeError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		writeError(w, r, http.StatusGatewayTimeout, "ASR_TEST_TIMEOUT", "连接测试超时，请稍后重试", true)
	case errors.Is(err, asr.ErrAuth):
		writeError(w, r, http.StatusBadGateway, "ASR_TEST_AUTH", "服务商鉴权失败，请检查 API Key 或权限", false)
	case errors.Is(err, asr.ErrRateLimited):
		writeError(w, r, http.StatusBadGateway, "ASR_TEST_RATE_LIMITED", "服务商请求过于频繁，请稍后重试", true)
	case errors.Is(err, asr.ErrBudgetExceeded):
		writeError(w, r, http.StatusBadGateway, "ASR_TEST_QUOTA", "服务商额度不足，请检查账户余额和用量", false)
	default:
		writeError(w, r, http.StatusBadGateway, "ASR_TEST_FAILED", "服务商未能处理测试音频，请检查模型配置、服务状态和服务器网络", true)
	}
}
