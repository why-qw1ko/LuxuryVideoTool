package httpapi

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	ownedfiles "github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/files"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/jobs"
)

const archiveDownloadTTL = 5 * time.Minute

func archiveNoCache(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Pragma", "no-cache")
}

func handleArchiveDownloadLink(deps Dependencies, w http.ResponseWriter, r *http.Request) {
	archiveNoCache(w)
	if deps.ASRSigner == nil {
		writeError(w, r, http.StatusServiceUnavailable, "SERVICE_NOT_READY", "服务尚未就绪", true)
		return
	}
	principal, _ := Principal(r.Context())
	_, media, ok := archiveMedia(deps, principal.UserID, r.PathValue("id"), w, r)
	if !ok {
		return
	}
	for _, file := range media {
		if err := r.Context().Err(); err != nil {
			writeArchiveError(w, r, err)
			return
		}
		handle, err := openArchiveMedia(deps.Storage, file)
		if err != nil {
			writeArchiveError(w, r, err)
			return
		}
		_ = handle.Close()
	}
	expires := time.Now().UTC().Add(archiveDownloadTTL).Truncate(time.Second)
	writeJSON(w, http.StatusOK, map[string]any{
		"downloadUrl": deps.ASRSigner.ArchiveURL(principal.UserID, r.PathValue("id"), expires),
		"expiresAt":   expires,
	})
}

func handleSignedArchiveDownload(deps Dependencies, w http.ResponseWriter, r *http.Request) {
	archiveNoCache(w)
	if deps.ASRSigner == nil {
		writeError(w, r, http.StatusServiceUnavailable, "SERVICE_NOT_READY", "服务尚未就绪", true)
		return
	}
	query := r.URL.Query()
	userID := query.Get("user")
	if !deps.ASRSigner.ValidateArchive(userID, r.PathValue("id"), query.Get("expires"), query.Get("signature"), time.Now()) {
		http.NotFound(w, r)
		return
	}
	handleImageArchive(deps, userID, w, r)
}

func archiveMedia(deps Dependencies, userID, jobID string, w http.ResponseWriter, r *http.Request) (jobs.Job, []ownedfiles.File, bool) {
	if deps.Jobs == nil || deps.Files == nil || deps.Storage == nil {
		writeError(w, r, http.StatusServiceUnavailable, "SERVICE_NOT_READY", "服务尚未就绪", true)
		return jobs.Job{}, nil, false
	}
	job, err := deps.Jobs.Get(r.Context(), userID, jobID)
	if errors.Is(err, jobs.ErrNotFound) {
		writeError(w, r, http.StatusNotFound, "JOB_NOT_FOUND", "任务不存在", false)
		return jobs.Job{}, nil, false
	}
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "无法读取任务", true)
		return jobs.Job{}, nil, false
	}
	if job.Status != "completed" {
		writeError(w, r, http.StatusConflict, "JOB_NOT_READY", "任务尚未完成，暂不能打包配图", false)
		return jobs.Job{}, nil, false
	}
	owned, err := deps.Files.ListByJob(r.Context(), userID, jobID)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "无法读取媒体文件", true)
		return jobs.Job{}, nil, false
	}
	var media []ownedfiles.File
	now := time.Now()
	for _, file := range owned {
		if (file.Kind == "image" || file.Kind == "animated") && (file.ExpiresAt == nil || file.ExpiresAt.After(now)) {
			media = append(media, file)
		}
	}
	if job.Work != nil && job.Work.Type == "note" && len(job.Work.Images) > 0 {
		available := media
		media = nil
		// 与画廊按类型和 image-NN 序号匹配，旧静图不能替代动图，每项只取首个文件。
		for index, image := range job.Work.Images {
			kind := "image"
			if image.AnimatedURL != "" {
				kind = "animated"
			}
			prefix := fmt.Sprintf("image-%02d.", index+1)
			for _, file := range available {
				if file.Kind == kind && strings.HasPrefix(file.OriginalName, prefix) {
					media = append(media, file)
					break
				}
			}
		}
	}
	if len(media) == 0 {
		writeError(w, r, http.StatusNotFound, "FILES_NOT_FOUND", "该任务没有可打包的配图", false)
		return jobs.Job{}, nil, false
	}
	return job, media, true
}

func handleImageArchive(deps Dependencies, userID string, w http.ResponseWriter, r *http.Request) {
	archiveNoCache(w)
	job, media, ok := archiveMedia(deps, userID, r.PathValue("id"), w, r)
	if !ok {
		return
	}
	tmp, err := buildImageArchive(r.Context(), deps.Storage, media)
	if err != nil {
		writeArchiveError(w, r, err)
		return
	}
	defer removeImageArchive(tmp)
	if err := r.Context().Err(); err != nil {
		writeArchiveError(w, r, err)
		return
	}
	filename := jobWorkID(job) + "_images.zip"
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Disposition", "attachment; filename*=UTF-8''"+url.PathEscape(filename))
	// 完整成包后 ServeContent 才计算 Content-Length，并处理浏览器的 Range 请求。
	http.ServeContent(w, r, filename, time.Time{}, tmp)
}

func writeArchiveError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, os.ErrNotExist) {
		writeError(w, r, http.StatusNotFound, "FILE_NOT_FOUND", "配图文件不存在", false)
		return
	}
	writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "打包配图失败", true)
}

type archiveStorage interface {
	Open(ownedfiles.File) (*os.File, error)
}

func openArchiveMedia(storage archiveStorage, file ownedfiles.File) (*os.File, error) {
	handle, err := storage.Open(file)
	if err != nil {
		return nil, err
	}
	stat, err := handle.Stat()
	if err != nil || !stat.Mode().IsRegular() {
		_ = handle.Close()
		if err != nil {
			return nil, err
		}
		return nil, os.ErrNotExist
	}
	return handle, nil
}

func buildImageArchive(ctx context.Context, storage archiveStorage, media []ownedfiles.File) (*os.File, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	tmp, err := os.CreateTemp("", "douyin-images-*.zip")
	if err != nil {
		return nil, err
	}
	keep := false
	defer func() {
		if !keep {
			removeImageArchive(tmp)
		}
	}()
	// Create 使用默认 Deflate 压缩；检查读写时的取消，避免客户端离开后继续打包。
	zw := zip.NewWriter(archiveContextWriter{ctx: ctx, writer: tmp})
	defer func() {
		if zw != nil {
			_ = zw.Close()
		}
	}()
	for index, file := range media {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		name := fmt.Sprintf("%02d_%s", index+1, url.PathEscape(file.OriginalName))
		if err := writeImageArchiveEntry(ctx, storage, zw, name, file); err != nil {
			return nil, err
		}
	}
	err = zw.Close()
	zw = nil
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	keep = true
	return tmp, nil
}

func writeImageArchiveEntry(ctx context.Context, storage archiveStorage, zw *zip.Writer, name string, file ownedfiles.File) error {
	handle, err := openArchiveMedia(storage, file)
	if err != nil {
		return err
	}
	defer handle.Close()
	entry, err := zw.Create(name)
	if err != nil {
		return err
	}
	_, err = io.Copy(entry, archiveContextReader{ctx: ctx, reader: handle})
	return err
}

func removeImageArchive(file *os.File) {
	name := file.Name()
	_ = file.Close()
	_ = os.Remove(name)
}

type archiveContextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r archiveContextReader) Read(body []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	n, err := r.reader.Read(body)
	if cancelled := r.ctx.Err(); cancelled != nil {
		return 0, cancelled
	}
	return n, err
}

type archiveContextWriter struct {
	ctx    context.Context
	writer io.Writer
}

func (w archiveContextWriter) Write(body []byte) (int, error) {
	if err := w.ctx.Err(); err != nil {
		return 0, err
	}
	return w.writer.Write(body)
}
