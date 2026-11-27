package httpapi

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/jobs"
)

func registerMusicRoutes(mux, protected *http.ServeMux, deps Dependencies) {
	if deps.Auth == nil {
		return
	}
	register := func(pattern string, admin bool) {
		protected.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "private, no-store")
			principal, _ := Principal(r.Context())
			if admin && !adminAllowed(principal, w, r) {
				return
			}
			if deps.Jobs == nil {
				writeError(w, r, http.StatusServiceUnavailable, "SERVICE_NOT_READY", "音乐源刷新服务尚未就绪", true)
				return
			}
			if r.Body != nil {
				body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1))
				if err != nil || len(body) != 0 {
					writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "刷新仅使用任务中已保存的作品，不接受请求参数", false)
					return
				}
			}
			var job jobs.Job
			var err error
			if admin {
				job, err = deps.Jobs.RefreshAnyMusic(r.Context(), r.PathValue("id"))
			} else {
				job, err = deps.Jobs.RefreshMusic(r.Context(), principal.UserID, r.PathValue("id"))
			}
			if err != nil {
				writeMusicRefreshError(w, r, err)
				return
			}
			job = withMediaPreviews([]jobs.Job{job}, deps, time.Now())[0]
			if admin {
				recordAdminAudit(deps.Audit, r, principal.UserID, "admin.job.music_refresh", "job", job.ID, nil)
			}
			writeJSON(w, http.StatusOK, map[string]any{"job": job, "requestId": RequestID(r.Context())})
		})
		mux.Handle(pattern, requireAuth(deps.Auth, protected))
	}
	register("POST /api/v1/jobs/{id}/music/refresh", false)
	register("POST /api/v1/admin/jobs/{id}/music/refresh", true)
}

func writeMusicRefreshError(w http.ResponseWriter, r *http.Request, err error) {
	var limited *jobs.MusicRefreshLimitError
	switch {
	case r.Context().Err() != nil:
		return
	case errors.Is(err, jobs.ErrNotFound):
		writeError(w, r, http.StatusNotFound, "JOB_NOT_FOUND", "任务不存在", false)
	case errors.Is(err, jobs.ErrMusicRefreshUnavailable):
		writeError(w, r, http.StatusServiceUnavailable, "SERVICE_NOT_READY", "音乐源刷新服务尚未就绪", true)
	case errors.Is(err, jobs.ErrMusicRefreshNotReady):
		writeError(w, r, http.StatusConflict, "JOB_NOT_READY", "任务完成且存在作品信息后才能刷新音乐源", false)
	case errors.As(err, &limited):
		seconds := max(1, int((limited.RetryAfter+time.Second-1)/time.Second))
		w.Header().Set("Retry-After", strconv.Itoa(seconds))
		writeError(w, r, http.StatusTooManyRequests, "MUSIC_REFRESH_RATE_LIMITED", "音乐源刷新过于频繁，请稍后重试", true)
	case errors.Is(err, context.DeadlineExceeded):
		writeError(w, r, http.StatusGatewayTimeout, "MUSIC_REFRESH_TIMEOUT", "刷新音乐源超时，可继续尝试原有地址", true)
	default:
		writeError(w, r, http.StatusBadGateway, "MUSIC_REFRESH_FAILED", "无法刷新音乐源，可继续尝试原有地址", true)
	}
}
