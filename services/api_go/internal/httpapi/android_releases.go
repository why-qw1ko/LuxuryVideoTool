package httpapi

import (
	"errors"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/auth"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/releases"
)

func registerAndroidReleaseRoutes(mux *http.ServeMux, deps Dependencies) {
	mux.HandleFunc("GET /api/v1/app/android/version", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if !androidReleasesReady(deps, w, r) {
			return
		}
		state, err := deps.AndroidReleases.State(r.Context())
		if err != nil {
			writeAndroidReleaseError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"release": state.Release, "requestId": RequestID(r.Context())})
	})
	mux.HandleFunc("GET /api/v1/app/android/releases/{id}/apk", func(w http.ResponseWriter, r *http.Request) {
		if !androidReleasesReady(deps, w, r) {
			return
		}
		file, release, err := deps.AndroidReleases.OpenPublished(r.Context(), r.PathValue("id"))
		if err != nil {
			writeAndroidReleaseError(w, r, err)
			return
		}
		defer file.Close()
		info, err := file.Stat()
		if err != nil {
			writeAndroidReleaseError(w, r, err)
			return
		}
		w.Header().Set("Content-Type", "application/vnd.android.package-archive")
		w.Header().Set("Content-Disposition", `attachment; filename="luxuryvideo-`+release.ID+`.apk"`)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		w.Header().Set("ETag", `"`+release.SHA256+`"`)
		http.ServeContent(w, r, "app.apk", info.ModTime(), file)
	})
	if deps.Auth == nil {
		return
	}
	register := func(pattern string, handler func(http.ResponseWriter, *http.Request, auth.Principal)) {
		mux.Handle(pattern, requireAuth(deps.Auth, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			principal, _ := Principal(r.Context())
			w.Header().Set("Cache-Control", "private, no-store")
			if adminAllowed(principal, w, r) && androidReleasesReady(deps, w, r) {
				handler(w, r, principal)
			}
		})))
	}
	respond := func(w http.ResponseWriter, r *http.Request, state releases.State, err error) {
		if err != nil {
			writeAndroidReleaseError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"draft": state.Draft, "release": state.Release, "requestId": RequestID(r.Context())})
	}
	register("GET /api/v1/admin/app/android/release", func(w http.ResponseWriter, r *http.Request, _ auth.Principal) {
		state, err := deps.AndroidReleases.State(r.Context())
		respond(w, r, state, err)
	})
	register("POST /api/v1/admin/app/android/apk", func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		const maxBody = releases.MaxAPKSize + (1 << 20)
		if r.ContentLength > maxBody {
			writeAndroidReleaseError(w, r, releases.ErrTooLarge)
			return
		}
		// Large APK uploads need their own deadline; other routes retain the
		// server's short read timeout.
		controller := http.NewResponseController(w)
		if err := controller.SetReadDeadline(time.Now().Add(15 * time.Minute)); err != nil && !errors.Is(err, http.ErrNotSupported) {
			writeAndroidReleaseError(w, r, err)
			return
		}
		defer controller.SetReadDeadline(time.Time{})
		r.Body = http.MaxBytesReader(w, r.Body, maxBody)
		reader, err := r.MultipartReader()
		if err != nil {
			writeAndroidReleaseError(w, r, releases.ErrInput)
			return
		}
		part, err := reader.NextPart()
		if err != nil || part.FormName() != "apk" || !strings.EqualFold(filepath.Ext(part.FileName()), ".apk") {
			writeAndroidReleaseError(w, r, releases.ErrInput)
			return
		}
		state, err := deps.AndroidReleases.Upload(r.Context(), part)
		if err == nil {
			recordAdminAudit(deps.Audit, r, principal.UserID, "admin.android.upload", "android_release", state.Draft.ID, map[string]any{"versionCode": state.Draft.VersionCode, "sha256": state.Draft.SHA256})
		}
		respond(w, r, state, err)
	})
	register("PUT /api/v1/admin/app/android/release", func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		var input releases.DraftInput
		if err := decodeJSONLimit(w, r, &input, 64<<10); err != nil {
			return
		}
		state, err := deps.AndroidReleases.SaveDraft(r.Context(), input)
		if err == nil {
			recordAdminAudit(deps.Audit, r, principal.UserID, "admin.android.edit", "android_release", state.Draft.ID, nil)
		}
		respond(w, r, state, err)
	})
	register("POST /api/v1/admin/app/android/publish", func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		var input struct {
			DraftID string `json:"draftId"`
		}
		if err := decodeJSON(w, r, &input); err != nil {
			return
		}
		state, err := deps.AndroidReleases.Publish(r.Context(), input.DraftID)
		if err == nil {
			recordAdminAudit(deps.Audit, r, principal.UserID, "admin.android.publish", "android_release", state.Release.ID, map[string]any{"versionCode": state.Release.VersionCode, "forceUpdate": state.Release.ForceUpdate})
		}
		respond(w, r, state, err)
	})
	register("PUT /api/v1/admin/app/android/policy", func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		var input struct {
			ReleaseID   string `json:"releaseId"`
			ForceUpdate *bool  `json:"forceUpdate"`
		}
		if err := decodeJSON(w, r, &input); err != nil {
			return
		}
		if input.ForceUpdate == nil {
			writeAndroidReleaseError(w, r, releases.ErrInput)
			return
		}
		state, err := deps.AndroidReleases.SetPolicy(r.Context(), input.ReleaseID, *input.ForceUpdate)
		if err == nil {
			recordAdminAudit(deps.Audit, r, principal.UserID, "admin.android.policy", "android_release", state.Release.ID, map[string]any{"forceUpdate": state.Release.ForceUpdate})
		}
		respond(w, r, state, err)
	})
}

func androidReleasesReady(deps Dependencies, w http.ResponseWriter, r *http.Request) bool {
	if deps.AndroidReleases == nil {
		writeError(w, r, http.StatusServiceUnavailable, "SERVICE_NOT_READY", "版本发布服务尚未就绪", true)
		return false
	}
	return true
}

func writeAndroidReleaseError(w http.ResponseWriter, r *http.Request, err error) {
	var limit *http.MaxBytesError
	switch {
	case errors.Is(err, releases.ErrTooLarge), errors.As(err, &limit):
		writeError(w, r, http.StatusRequestEntityTooLarge, "APK_TOO_LARGE", "APK 大小不能超过 256 MiB", false)
	case errors.Is(err, releases.ErrInvalidAPK):
		writeError(w, r, http.StatusBadRequest, "INVALID_APK", "请选择本应用的完整 APK，包名或版本信息不正确", false)
	case errors.Is(err, releases.ErrInput):
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "请上传 APK 并填写有效的更新说明，版本信息必须与 APK 一致", false)
	case errors.Is(err, releases.ErrConflict):
		writeError(w, r, http.StatusConflict, "RELEASE_CHANGED", "版本已被其他操作修改，请重新加载后操作", false)
	case errors.Is(err, releases.ErrDowngrade):
		writeError(w, r, http.StatusConflict, "VERSION_NOT_INCREASED", "新 APK 的版本编号必须递增；同一安装包可以重新编辑发布", false)
	case errors.Is(err, releases.ErrNotFound):
		writeError(w, r, http.StatusNotFound, "RELEASE_NOT_FOUND", "安装包尚未发布或文件不存在", false)
	case errors.Is(err, releases.ErrBusy):
		writeError(w, r, http.StatusConflict, "APK_UPLOAD_BUSY", "已有 APK 正在上传，请稍后重试", true)
	default:
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "暂时无法处理版本发布，请稍后重试", true)
	}
}
