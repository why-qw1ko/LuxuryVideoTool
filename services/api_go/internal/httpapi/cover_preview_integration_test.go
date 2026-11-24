package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/jobs"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/resolver"
)

type coverFetcherFunc func(context.Context, []string) ([]byte, string, error)

func (f coverFetcherFunc) Fetch(ctx context.Context, candidates []string) ([]byte, string, error) {
	return f(ctx, candidates)
}

func TestInfoJobServesSignedVideoCover(t *testing.T) {
	f := newArchiveDownloadFixture(t)
	candidates := []string{"https://p3.douyinpic.com/first.webp", "https://p9.douyinpic.com/second.webp"}
	adapter := &resolver.Fake{Work: resolver.Work{
		DouyinWorkID: "123", Type: "video", CanonicalURL: "https://www.douyin.com/video/123",
		CoverURL: candidates[0], CoverURLCandidates: candidates,
		ResolverName: "fake", ResolverVersion: resolver.DouyinResolverVersion,
	}}
	f.deps.Jobs = jobs.NewService(f.repo, resolver.NewService(adapter, resolver.NewSQLiteCache(f.db), time.Hour, resolver.DouyinResolverVersion))
	image := []byte("fixture webp")
	fetchCalls := 0
	f.deps.Covers = coverFetcherFunc(func(_ context.Context, got []string) ([]byte, string, error) {
		fetchCalls++
		if !reflect.DeepEqual(got, candidates) {
			t.Fatalf("upstream covers = %v, want %v", got, candidates)
		}
		return image, "image/webp", nil
	})
	f.handler = New(f.deps)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/jobs", strings.NewReader(`{"shareText":"https://www.douyin.com/video/123","action":"info"}`))
	request.Header.Set("Authorization", "Bearer "+f.ownerToken)
	request.Header.Set("Idempotency-Key", "video-cover")
	create := httptest.NewRecorder()
	f.handler.ServeHTTP(create, request)
	if create.Code != http.StatusAccepted {
		t.Fatalf("create status = %d, body = %s", create.Code, create.Body.String())
	}
	var result struct{ Job jobs.Job }
	if err := json.Unmarshal(create.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Job.Work == nil || result.Job.Work.CoverPreviewURL == "" {
		t.Fatalf("create response missing cover preview URL: %s", create.Body.String())
	}
	coverURL := result.Job.Work.CoverPreviewURL
	parsed, err := url.Parse(coverURL)
	if err != nil || parsed.IsAbs() || parsed.Host != "" || parsed.Path != "/api/v1/job-cover/"+result.Job.ID || parsed.RawQuery == "" {
		t.Fatalf("invalid signed cover URL %q: %v", coverURL, err)
	}
	detailRoute := "/api/v1/jobs/" + result.Job.ID
	detail := performJSON(f.handler, http.MethodGet, detailRoute, nil, f.ownerToken)
	if detail.Code != http.StatusOK || !bytes.Contains(detail.Body.Bytes(), []byte(`"coverPreviewUrl"`)) {
		t.Fatalf("missing cover preview on detail: %d %s", detail.Code, detail.Body.String())
	}
	if detail := performJSON(f.handler, http.MethodGet, detailRoute, nil, f.otherToken); detail.Code != http.StatusNotFound {
		t.Fatalf("other user read private cover URL: %d", detail.Code)
	}
	query := parsed.Query()
	query.Set("signature", "invalid")
	for _, rejected := range []string{parsed.Path, parsed.Path + "?" + query.Encode(), "/api/v1/job-cover/other-job?" + parsed.RawQuery,
		f.deps.ASRSigner.CoverPreviewURL(result.Job.ID, time.Now().Add(-time.Minute))} {
		if response := performJSON(f.handler, http.MethodGet, rejected, nil, ""); response.Code != http.StatusNotFound {
			t.Fatalf("invalid cover URL status = %d", response.Code)
		}
	}
	if fetchCalls != 0 {
		t.Fatalf("unexpected upstream calls before valid cover request: %d", fetchCalls)
	}
	response := performJSON(f.handler, http.MethodGet, coverURL, nil, "")
	if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "image/webp" ||
		response.Header().Get("X-Content-Type-Options") != "nosniff" || !bytes.Equal(response.Body.Bytes(), image) || fetchCalls != 1 {
		t.Fatalf("cover status = %d, headers = %v, calls = %d, body = %q", response.Code, response.Header(), fetchCalls, response.Body.Bytes())
	}
}
