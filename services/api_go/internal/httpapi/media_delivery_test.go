package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	ownedfiles "github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/files"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/jobs"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/media"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/resolver"
)

func TestExpiredServerMediaCannotBeReadBeforeCleanup(t *testing.T) {
	f := newArchiveDownloadFixture(t)
	past := time.Now().Add(-time.Minute)
	file := f.addFile(t, "job-1", f.ownerID, "music", "expired.mp3", []byte("expired bytes still on disk"), &past)
	signed := f.deps.ASRSigner.PreviewURL(file.ID, time.Now().Add(time.Hour))
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		request := httptest.NewRequest(method, signed, nil)
		request.Header.Set("Range", "bytes=0-3")
		response := httptest.NewRecorder()
		f.handler.ServeHTTP(response, request)
		if response.Code != http.StatusNotFound || response.Header().Get("Cache-Control") != "private, no-store" {
			t.Fatalf("%s preview status=%d cache=%q", method, response.Code, response.Header().Get("Cache-Control"))
		}
	}
	download := performJSON(f.handler, http.MethodGet, "/api/v1/files/"+file.ID, nil, f.ownerToken)
	if download.Code != http.StatusGone || !strings.Contains(download.Body.String(), `"FILE_EXPIRED"`) {
		t.Fatalf("expired download status=%d body=%s", download.Code, download.Body.String())
	}
	if denied := performJSON(f.handler, http.MethodGet, "/api/v1/files/"+file.ID, nil, f.otherToken); denied.Code != http.StatusNotFound {
		t.Fatalf("expired private file status=%d", denied.Code)
	}
	if handle, err := f.deps.Storage.Open(file); err != nil {
		t.Fatal("fixture should remain physically present", err)
	} else {
		handle.Close()
	}
}

func TestMusicContractFallsBackToOriginalAfterServerExpiry(t *testing.T) {
	f := newArchiveDownloadFixture(t)
	work, err := resolver.NewSQLiteCache(f.db).Save(context.Background(), resolver.Work{
		DouyinWorkID: "123", Type: "note", CanonicalURL: "https://www.douyin.com/note/123", Title: "work",
		MusicURL: "https://example.invalid/original.mp3", MusicTitle: "music", MusicArtist: "artist",
		ResolverName: "fake", ResolverVersion: "test", ResolvedAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec(`UPDATE jobs SET work_id = ? WHERE id = 'job-1'`, work.ID); err != nil {
		t.Fatal(err)
	}
	expires := time.Now().Add(90 * time.Second)
	if _, err := f.db.Exec(`UPDATE files SET expires_at = ? WHERE job_id = 'job-1' AND kind = 'music'`, expires.UnixMilli()); err != nil {
		t.Fatal(err)
	}
	read := func() jobs.Job {
		t.Helper()
		response := performJSON(f.handler, http.MethodGet, "/api/v1/jobs/job-1", nil, f.ownerToken)
		var payload struct {
			Job jobs.Job `json:"job"`
		}
		if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &payload) != nil {
			t.Fatalf("detail status=%d body=%s", response.Code, response.Body.String())
		}
		return payload.Job
	}
	current := read()
	if current.Music == nil || current.Music.Source != "server" || current.Music.FileID == "" || current.Music.FallbackURL != work.MusicURL || current.Music.Title != "music" {
		t.Fatalf("server music = %#v", current.Music)
	}
	parsed, err := url.Parse(current.Music.URL)
	if err != nil {
		t.Fatal(err)
	}
	signedExpiry, err := strconv.ParseInt(parsed.Query().Get("expires"), 10, 64)
	if err != nil || signedExpiry > expires.Unix() {
		t.Fatalf("preview outlives media: %s", current.Music.URL)
	}
	past := time.Now().Add(-time.Minute)
	if _, err := f.db.Exec(`UPDATE files SET expires_at = ? WHERE job_id = 'job-1' AND kind = 'music'`, past.UnixMilli()); err != nil {
		t.Fatal(err)
	}
	current = read()
	if current.Music == nil || current.Music.Source != "original" || current.Music.URL != work.MusicURL || current.Music.FileID != "" || current.Music.ExpiresAt != nil {
		t.Fatalf("original fallback = %#v", current.Music)
	}
	var payload struct {
		Job struct {
			Result struct {
				Files []jobs.JobFile `json:"files"`
			} `json:"result"`
		} `json:"job"`
	}
	response := performJSON(f.handler, http.MethodGet, "/api/v1/jobs/job-1", nil, f.ownerToken)
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	for _, file := range payload.Job.Result.Files {
		if file.Kind == "music" && (file.PreviewURL != "" || file.ThumbnailURL != "") {
			t.Fatalf("expired file was signed: %#v", file)
		}
	}
}

func TestMediaRangeHEADAndConditionalRequests(t *testing.T) {
	f := newArchiveDownloadFixture(t)
	expires := time.Now().Add(90 * time.Second)
	file := f.addFile(t, "job-1", f.ownerID, "video", "clip.mp4", []byte("0123456789abcdef"), &expires)
	signed := f.deps.ASRSigner.PreviewURL(file.ID, time.Now().Add(40*time.Second))
	call := func(method string, headers map[string]string) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, signed, nil)
		for key, value := range headers {
			r.Header.Set(key, value)
		}
		w := httptest.NewRecorder()
		f.handler.ServeHTTP(w, r)
		return w
	}
	full := call(http.MethodGet, nil)
	etag := full.Header().Get("ETag")
	if full.Code != 200 || etag == "" || full.Header().Get("Accept-Ranges") != "bytes" {
		t.Fatalf("full=%d %v", full.Code, full.Header())
	}
	maxAge, _ := strconv.Atoi(strings.Split(strings.TrimPrefix(full.Header().Get("Cache-Control"), "private, max-age="), ",")[0])
	if maxAge < 0 || maxAge > 40 {
		t.Fatalf("unbounded max-age=%d", maxAge)
	}
	if w := call(http.MethodGet, map[string]string{"If-None-Match": etag}); w.Code != 304 || w.Body.Len() != 0 {
		t.Fatalf("conditional=%d body=%q", w.Code, w.Body.String())
	}
	if w := call(http.MethodHead, nil); w.Code != 200 || w.Body.Len() != 0 || w.Header().Get("Content-Length") != "16" {
		t.Fatalf("HEAD=%d %v body=%q", w.Code, w.Header(), w.Body.String())
	}
	for _, tc := range []struct{ header, body, contentRange string }{
		{"bytes=4-7", "4567", "bytes 4-7/16"},
		{"bytes=-4", "cdef", "bytes 12-15/16"},
	} {
		w := call(http.MethodGet, map[string]string{"Range": tc.header, "If-Range": etag})
		if w.Code != 206 || w.Body.String() != tc.body || w.Header().Get("Content-Range") != tc.contentRange {
			t.Fatalf("range %s=%d %v %q", tc.header, w.Code, w.Header(), w.Body.String())
		}
	}
	if w := call(http.MethodGet, map[string]string{"Range": "bytes=4-7", "If-Range": `"different"`}); w.Code != 200 || w.Body.Len() != 16 {
		t.Fatalf("stale If-Range=%d", w.Code)
	}
	if w := call(http.MethodGet, map[string]string{"Range": "bytes=99-100"}); w.Code != 416 {
		t.Fatalf("invalid range=%d", w.Code)
	}
}

func TestThumbnailExpirySharesOriginalLifetime(t *testing.T) {
	f := newArchiveDownloadFixture(t)
	expires := time.Now().Add(2 * time.Minute)
	file := f.addFile(t, "job-1", f.ownerID, "image", "photo.jpg", []byte("image"), &expires)
	f.deps.Thumbnails = media.NewThumbnailer(filepath.Join(t.TempDir(), "missing-ffmpeg"))
	past := time.Now().Add(-time.Second)
	if _, err := f.db.Exec(`UPDATE files SET expires_at = ? WHERE id = ?`, past.UnixMilli(), file.ID); err != nil {
		t.Fatal(err)
	}
	f.handler = New(f.deps)
	signed := f.deps.ASRSigner.ThumbnailURL(file.ID, time.Now().Add(time.Hour))
	response := performJSON(f.handler, http.MethodGet, signed, nil, "")
	if response.Code != 404 || response.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("expired thumbnail=%d %v", response.Code, response.Header())
	}
}

func TestPreviewDoesNotRequireOriginalMusicURL(t *testing.T) {
	now := time.Now()
	signer := ownedfiles.NewSigner([]byte("01234567890123456789012345678901"), "")
	items := withMediaPreviews([]jobs.Job{{Result: map[string]any{"files": []jobs.JobFile{{ID: "music-only", Kind: "music", Name: "saved.mp3"}}}}}, Dependencies{ASRSigner: signer}, now)
	if items[0].Music == nil || items[0].Music.Source != "server" || items[0].Music.URL == "" {
		t.Fatalf("saved-only music=%#v", items[0].Music)
	}
}
