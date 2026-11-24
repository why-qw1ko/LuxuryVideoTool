package httpapi

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/jobs"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/media"
)

func TestSignedThumbnailAndOriginalPreview(t *testing.T) {
	f := newArchiveDownloadFixture(t)
	f.deps.Thumbnails = media.NewThumbnailer(filepath.Join(t.TempDir(), "unavailable-ffmpeg"))
	f.handler = New(f.deps)
	webp, err := base64.StdEncoding.DecodeString("UklGRiQAAABXRUJQVlA4IBgAAAAwAQCdASoCAAIAAgA0JaQAA3AA/vuUAAA=")
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range f.media {
		path, err := f.deps.Storage.Resolve(source.Thumbnail().RelativePath)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, webp, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	detail := performJSON(f.handler, http.MethodGet, "/api/v1/jobs/job-1", nil, f.ownerToken)
	if detail.Code != http.StatusOK {
		t.Fatalf("job detail status=%d body=%s", detail.Code, detail.Body.String())
	}
	var payload struct {
		Job struct {
			Result struct {
				Files []jobs.JobFile `json:"files"`
			} `json:"result"`
		} `json:"job"`
	}
	if err := json.Unmarshal(detail.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	files := make(map[string]jobs.JobFile)
	for _, file := range payload.Job.Result.Files {
		files[file.ID] = file
		if file.Kind == "music" && file.ThumbnailURL != "" {
			t.Fatalf("audio has thumbnail URL: %q", file.ThumbnailURL)
		}
	}
	for _, source := range f.media {
		file := files[source.ID]
		original, err := url.Parse(file.PreviewURL)
		if err != nil || original.Host != "" || original.IsAbs() || original.Path != "/api/v1/media-preview/"+source.ID || original.RawQuery == "" {
			t.Fatalf("%s original preview URL=%q error=%v", source.Kind, file.PreviewURL, err)
		}
		thumbnail, err := url.Parse(file.ThumbnailURL)
		if err != nil || thumbnail.Host != "" || thumbnail.IsAbs() || thumbnail.Path != original.Path+"/thumbnail" || thumbnail.RawQuery == "" {
			t.Fatalf("%s thumbnail URL=%q error=%v", source.Kind, file.ThumbnailURL, err)
		}
		response := performJSON(f.handler, http.MethodGet, file.ThumbnailURL, nil, "")
		if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "image/webp" || !bytes.Equal(response.Body.Bytes(), webp) {
			t.Fatalf("%s thumbnail status=%d headers=%v body=%q", source.Kind, response.Code, response.Header(), response.Body.Bytes())
		}

		for name, target := range map[string]string{
			"original signature on thumbnail route": thumbnail.Path + "?" + original.RawQuery,
			"thumbnail signature on original route": original.Path + "?" + thumbnail.RawQuery,
			"expired thumbnail signature":           f.deps.ASRSigner.ThumbnailURL(source.ID, time.Now().Add(-time.Minute)),
		} {
			if rejected := performJSON(f.handler, http.MethodGet, target, nil, ""); rejected.Code != http.StatusNotFound {
				t.Errorf("%s %s status=%d, want 404", source.Kind, name, rejected.Code)
			}
		}
	}

	video := files[f.media[1].ID]
	request := httptest.NewRequest(http.MethodGet, video.PreviewURL, nil)
	request.Header.Set("Range", "bytes=9-12")
	ranged := httptest.NewRecorder()
	f.handler.ServeHTTP(ranged, request)
	if ranged.Code != http.StatusPartialContent || ranged.Header().Get("Content-Range") != "bytes 9-12/13" || ranged.Body.String() != "clip" {
		t.Fatalf("original video range status=%d headers=%v body=%q", ranged.Code, ranged.Header(), ranged.Body.String())
	}
}
