package httpapi

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	ownedfiles "github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/files"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/jobs"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/resolver"
)

func mediaFileExpired(expires *time.Time, now time.Time) bool {
	return expires != nil && !expires.After(now)
}

func mediaLinkExpiry(now time.Time, fileExpiry *time.Time) time.Time {
	expires := now.UTC().Truncate(time.Hour).Add(24 * time.Hour)
	if fileExpiry != nil && fileExpiry.Before(expires) {
		expires = *fileExpiry
	}
	return expires
}

func setMediaCacheHeaders(w http.ResponseWriter, r *http.Request, file ownedfiles.File, info os.FileInfo, now time.Time) {
	maxAge := int64(time.Hour / time.Second)
	if file.ExpiresAt != nil {
		maxAge = min(maxAge, int64(file.ExpiresAt.Sub(now)/time.Second))
	}
	if signedExpiry, err := strconv.ParseInt(r.URL.Query().Get("expires"), 10, 64); err == nil {
		maxAge = min(maxAge, int64(time.Unix(signedExpiry, 0).Sub(now)/time.Second))
	}
	maxAge = max(maxAge, 0)
	w.Header().Set("Cache-Control", fmt.Sprintf("private, max-age=%d, must-revalidate", maxAge))
	identity := fmt.Sprintf("%s\x00%s\x00%d\x00%d", file.RelativePath, file.SHA256, info.Size(), info.ModTime().UnixNano())
	w.Header().Set("ETag", fmt.Sprintf(`"%x"`, sha256.Sum256([]byte(identity))))
}

func jobMusic(work *resolver.Work, files []jobs.JobFile, now time.Time) *jobs.JobMusic {
	music := &jobs.JobMusic{Source: "unavailable"}
	if work != nil {
		music.Title, music.Artist = work.MusicTitle, work.MusicArtist
		music.FallbackURL = work.MusicURL
		if work.MusicURL != "" {
			music.Source, music.URL = "original", work.MusicURL
		}
	}
	for _, file := range files {
		if file.Kind == "music" && file.PreviewURL != "" && !mediaFileExpired(file.ExpiresAt, now) {
			music.Source, music.URL, music.FileID, music.ExpiresAt = "server", file.PreviewURL, file.ID, file.ExpiresAt
			if music.Title == "" {
				music.Title = file.Name
			}
			break
		}
	}
	if music.URL == "" && music.Title == "" && music.Artist == "" {
		return nil
	}
	return music
}

func withSummaryPreviews(items []jobs.JobSummary, deps Dependencies, now time.Time) {
	if deps.ASRSigner == nil {
		return
	}
	for i := range items {
		item := &items[i]
		work := item.Work
		if work == nil {
			continue
		}
		if deps.Thumbnails != nil && work.CoverFileID != "" && !mediaFileExpired(work.CoverExpiresAt, now) {
			work.CoverPreviewURL = deps.ASRSigner.ThumbnailURL(work.CoverFileID, mediaLinkExpiry(now, work.CoverExpiresAt))
			continue
		}
		if item.Status == "completed" {
			cover := resolver.Work{Type: work.Type, CoverURL: work.CoverURL}
			addCoverPreview(deps, &cover, item.ID, mediaLinkExpiry(now, nil))
			work.CoverPreviewURL = cover.CoverPreviewURL
		}
	}
}
