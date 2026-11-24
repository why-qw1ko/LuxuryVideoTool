package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/audit"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/settings"
)

func registerUsageRoutes(mux, protected *http.ServeMux, deps Dependencies) {
	if deps.Auth == nil {
		return
	}
	register := func(pattern string, handler http.HandlerFunc) {
		protected.HandleFunc(pattern, handler)
		mux.Handle(pattern, requireAuth(deps.Auth, protected))
	}
	register("GET /api/v1/admin/settings/usage", func(w http.ResponseWriter, r *http.Request) {
		principal, _ := Principal(r.Context())
		if !adminAllowed(principal, w, r) || !usageSettingsReady(deps, w, r) {
			return
		}
		value, err := deps.Settings.Usage(r.Context())
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "无法读取使用设置", true)
			return
		}
		w.Header().Set("Cache-Control", "private, no-store")
		writeJSON(w, http.StatusOK, map[string]any{"settings": value, "requestId": RequestID(r.Context())})
	})
	register("PUT /api/v1/admin/settings/usage", func(w http.ResponseWriter, r *http.Request) {
		principal, _ := Principal(r.Context())
		if !adminAllowed(principal, w, r) || !usageSettingsReady(deps, w, r) {
			return
		}
		var input struct {
			DailyTaskLimit       *int     `json:"dailyTaskLimit"`
			AllowInfo            *bool    `json:"allowInfo"`
			AllowDownload        *bool    `json:"allowDownload"`
			AllowTranscribe      *bool    `json:"allowTranscribe"`
			DailyASRBudgetCNY    *float64 `json:"dailyAsrBudgetCny"`
			MonthlyASRBudgetCNY  *float64 `json:"monthlyAsrBudgetCny"`
			ASRPricePerMinuteCNY *float64 `json:"asrPricePerMinuteCny"`
		}
		if err := decodeJSON(w, r, &input); err != nil {
			return
		}
		if input.DailyTaskLimit == nil || input.AllowInfo == nil || input.AllowDownload == nil || input.AllowTranscribe == nil ||
			input.DailyASRBudgetCNY == nil || input.MonthlyASRBudgetCNY == nil || input.ASRPricePerMinuteCNY == nil {
			writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "请提交完整的使用设置", false)
			return
		}
		value := settings.Usage{
			DailyTaskLimit: *input.DailyTaskLimit, AllowInfo: *input.AllowInfo, AllowDownload: *input.AllowDownload,
			AllowTranscribe: *input.AllowTranscribe, DailyASRBudgetCNY: *input.DailyASRBudgetCNY,
			MonthlyASRBudgetCNY: *input.MonthlyASRBudgetCNY, ASRPricePerMinuteCNY: *input.ASRPricePerMinuteCNY,
		}
		if err := deps.Settings.SetUsage(r.Context(), value, principal.UserID); err != nil {
			writeUsageSettingsError(w, r, err)
			return
		}
		recordAdminAudit(deps.Audit, r, principal.UserID, "admin.usage_settings.update", "runtime_settings", "usage_policy", nil)
		writeJSON(w, http.StatusOK, map[string]any{"settings": value, "requestId": RequestID(r.Context())})
	})
	register("PUT /api/v1/admin/users/{id}/daily-task-limit", func(w http.ResponseWriter, r *http.Request) {
		principal, _ := Principal(r.Context())
		if !adminAllowed(principal, w, r) || !usageSettingsReady(deps, w, r) {
			return
		}
		var input struct {
			DailyTaskLimit json.RawMessage `json:"dailyTaskLimit"`
		}
		if err := decodeJSON(w, r, &input); err != nil {
			return
		}
		var limit *int
		if len(input.DailyTaskLimit) == 0 || json.Unmarshal(input.DailyTaskLimit, &limit) != nil {
			writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "请输入有效的每日次数或选择跟随默认设置", false)
			return
		}
		userID := r.PathValue("id")
		if err := deps.Settings.SetUserDailyTaskLimit(r.Context(), userID, limit, principal.UserID); err != nil {
			writeUsageSettingsError(w, r, err)
			return
		}
		recordAdminAudit(deps.Audit, r, principal.UserID, "admin.user.daily_task_limit.update", "user", userID, map[string]any{"dailyTaskLimit": limit})
		writeJSON(w, http.StatusOK, map[string]any{"dailyTaskLimitOverride": limit, "requestId": RequestID(r.Context())})
	})
	register("GET /api/v1/jobs/usage", func(w http.ResponseWriter, r *http.Request) {
		if deps.Jobs == nil {
			writeError(w, r, http.StatusServiceUnavailable, "SERVICE_NOT_READY", "任务服务尚未就绪", true)
			return
		}
		principal, _ := Principal(r.Context())
		_, limit, err := userUsagePolicy(r.Context(), deps.Settings, principal.UserID)
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "无法读取每日额度", true)
			return
		}
		used, resetAt, err := deps.Jobs.DailyUsage(r.Context(), principal.UserID)
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "无法读取每日额度", true)
			return
		}
		usage := dailyUsageResult(used, limit, resetAt)
		if principal.IsCDK {
			cdkUsage, err := deps.Auth.CDKUsage(r.Context(), principal.UserID)
			if err != nil {
				writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "无法读取 CDK 额度", true)
				return
			}
			usage["cdk"] = cdkUsage
		}
		w.Header().Set("Cache-Control", "private, no-store")
		writeJSON(w, http.StatusOK, map[string]any{"usage": usage, "requestId": RequestID(r.Context())})
	})
}

func usageSettingsReady(deps Dependencies, w http.ResponseWriter, r *http.Request) bool {
	if deps.Settings == nil {
		writeError(w, r, http.StatusServiceUnavailable, "SERVICE_NOT_READY", "配置服务尚未就绪", true)
		return false
	}
	return true
}

func userUsagePolicy(ctx context.Context, service *settings.Service, userID string) (settings.Usage, int, error) {
	if service == nil {
		return settings.DefaultUsage(), 0, nil
	}
	policy, err := service.Usage(ctx)
	if err != nil {
		return settings.Usage{}, 0, err
	}
	limit, err := service.EffectiveDailyTaskLimit(ctx, userID, policy.DailyTaskLimit)
	return policy, limit, err
}

func dailyUsageResult(used, limit int, resetAt time.Time) map[string]any {
	return map[string]any{"used": used, "limit": limit, "resetAt": resetAt}
}

func writeDailyTaskLimitError(deps Dependencies, w http.ResponseWriter, r *http.Request, userID string, limit int) {
	used, resetAt, err := deps.Jobs.DailyUsage(r.Context(), userID)
	message := "今日任务次数已用完，北京时间零点重置"
	if err != nil {
		writeError(w, r, http.StatusTooManyRequests, "DAILY_TASK_LIMIT", message, false)
		return
	}
	w.Header().Set("Retry-After", strconv.FormatInt(max(1, int64(time.Until(resetAt).Seconds())+1), 10))
	writeJSON(w, http.StatusTooManyRequests, errorEnvelope{Error: APIError{
		Code: "DAILY_TASK_LIMIT", Message: message, RequestID: RequestID(r.Context()), Details: dailyUsageResult(used, limit, resetAt),
	}})
}

func writeUsageSettingsError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, settings.ErrInvalidUsage):
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "次数、预算或估算单价超出允许范围", false)
	case errors.Is(err, settings.ErrUserNotFound):
		writeError(w, r, http.StatusNotFound, "USER_NOT_FOUND", "用户不存在", false)
	default:
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "无法保存使用设置", true)
	}
}

func recordAdminAudit(recorder *audit.Recorder, r *http.Request, actorID, action, targetType, targetID string, metadata map[string]any) {
	if recorder == nil {
		return
	}
	if err := recorder.Record(r.Context(), audit.Event{
		ActorUserID: actorID, Action: action, TargetType: targetType, TargetID: targetID,
		RequestID: RequestID(r.Context()), IP: remoteIP(r.RemoteAddr), Metadata: metadata,
	}); err != nil {
		slog.ErrorContext(r.Context(), "admin audit failed", "request_id", RequestID(r.Context()), "action", action)
	}
}
