package httpapi

import (
	"context"
	"net/http"
	"slices"
	"time"

	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/media"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/resolver"
)

type coverFetcher interface {
	Fetch(context.Context, []string) ([]byte, string, error)
}

func videoCoverSources(work *resolver.Work) []string {
	if work == nil || work.Type != "video" {
		return nil
	}
	if len(work.CoverURLCandidates) != 0 {
		return work.CoverURLCandidates
	}
	return []string{work.CoverURL}
}

func addCoverPreview(deps Dependencies, work *resolver.Work, jobID string, expires time.Time) {
	if deps.Covers != nil && slices.ContainsFunc(videoCoverSources(work), media.CoverURLAllowed) {
		work.CoverPreviewURL = deps.ASRSigner.CoverPreviewURL(jobID, expires)
	}
}

func handleCoverPreview(deps Dependencies, w http.ResponseWriter, r *http.Request) {
	id, query := r.PathValue("id"), r.URL.Query()
	if deps.Jobs == nil || deps.Covers == nil || deps.ASRSigner == nil ||
		!deps.ASRSigner.ValidateCoverPreview(id, query.Get("expires"), query.Get("signature"), time.Now()) {
		http.NotFound(w, r)
		return
	}
	job, err := deps.Jobs.GetAny(r.Context(), id)
	if err != nil || job.Status != "completed" {
		http.NotFound(w, r)
		return
	}
	body, mime, err := deps.Covers.Fetch(r.Context(), videoCoverSources(job.Work))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", mime)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, max-age=300")
	_, _ = w.Write(body)
}
