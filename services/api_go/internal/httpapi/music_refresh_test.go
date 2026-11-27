package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/auth"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/jobs"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/resolver"
)

type musicHTTPAdapter func(context.Context, string) (resolver.Work, error)

func (f musicHTTPAdapter) Resolve(ctx context.Context, input string) (resolver.Work, error) {
	return f(ctx, input)
}

func musicRefreshHTTPHandler(deps Dependencies) http.Handler {
	mux, protected := http.NewServeMux(), http.NewServeMux()
	registerMusicRoutes(mux, protected, deps)
	return mux
}

func prepareMusicRefreshHTTP(t *testing.T, f *archiveDownloadFixture, adapter resolver.Adapter) string {
	t.Helper()
	ctx := context.Background()
	cache := resolver.NewSQLiteCache(f.db)
	work, err := cache.Save(ctx, resolver.Work{
		DouyinWorkID: "123", Type: "note", CanonicalURL: "https://www.douyin.com/note/123", Title: "Saved work",
		MusicURL: "https://example.test/old.mp3", MusicTitle: "Saved music", ResolvedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.ExecContext(ctx, `UPDATE jobs SET work_id = ? WHERE id IN ('job-1', 'other-job')`, work.ID); err != nil {
		t.Fatal(err)
	}
	f.deps.Jobs = jobs.NewService(f.repo, resolver.NewService(adapter, cache, time.Hour, resolver.DouyinResolverVersion))
	admin, err := f.deps.Auth.CreateUser(ctx, "music-admin", "Admin", "OwnerPass!", auth.RoleAdmin)
	if err != nil {
		t.Fatal(err)
	}
	pair, err := f.deps.Auth.Login(ctx, auth.LoginInput{Username: admin.UsernameNormalized, Password: "OwnerPass!",
		Device: auth.Device{ID: "music-admin", Name: "PC", Platform: "windows", AppVersion: "1"}})
	if err != nil {
		t.Fatal(err)
	}
	return pair.AccessToken
}

func TestMusicRefreshRoutesProtectOwnerAndUseOnlySavedSource(t *testing.T) {
	f := newArchiveDownloadFixture(t)
	var calls atomic.Int32
	adminToken := prepareMusicRefreshHTTP(t, f, musicHTTPAdapter(func(ctx context.Context, input string) (resolver.Work, error) {
		calls.Add(1)
		if input != "https://www.douyin.com/note/123" {
			t.Errorf("unexpected refresh source %q", input)
		}
		return resolver.Work{DouyinWorkID: "123", Type: "note", CanonicalURL: input, Title: "Current work",
			MusicURL: "https://example.test/new.mp3", MusicTitle: "Current music"}, nil
	}))
	handler := musicRefreshHTTPHandler(f.deps)
	path := "/api/v1/jobs/job-1/music/refresh"
	for _, check := range []struct {
		path, token string
		body        any
		status      int
	}{
		{path: path, status: http.StatusUnauthorized},
		{path: path, token: f.otherToken, status: http.StatusNotFound},
		{path: "/api/v1/admin/jobs/job-1/music/refresh", token: f.ownerToken, status: http.StatusForbidden},
		{path: path, token: f.ownerToken, body: map[string]string{"url": "https://example.test/arbitrary.mp3"}, status: http.StatusBadRequest},
	} {
		response := performJSON(handler, http.MethodPost, check.path, check.body, check.token)
		if response.Code != check.status {
			t.Fatalf("%s: status=%d want=%d body=%s", check.path, response.Code, check.status, response.Body.String())
		}
	}
	if calls.Load() != 0 {
		t.Fatalf("unauthorized/invalid refresh reached resolver: %d", calls.Load())
	}
	beforeUsage, _, err := f.deps.Jobs.DailyUsage(context.Background(), f.ownerID)
	if err != nil {
		t.Fatal(err)
	}
	response := performJSON(handler, http.MethodPost, path, nil, f.ownerToken)
	var payload struct {
		Job jobs.Job `json:"job"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil || response.Code != http.StatusOK {
		t.Fatalf("refresh status=%d error=%v body=%s", response.Code, err, response.Body.String())
	}
	if payload.Job.ID != "job-1" || payload.Job.Work == nil || payload.Job.Work.MusicURL != "https://example.test/new.mp3" ||
		payload.Job.Music == nil || payload.Job.Music.FallbackURL != "https://example.test/new.mp3" || payload.Job.Music.Source != "server" || calls.Load() != 1 {
		t.Fatalf("refresh payload = %+v calls=%d", payload, calls.Load())
	}
	result, ok := payload.Job.Result.(map[string]any)
	if !ok || len(result["files"].([]any)) != 3 || !strings.Contains(response.Body.String(), `"previewUrl"`) || response.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("refresh should return complete job with previews: %s", response.Body.String())
	}
	afterUsage, _, err := f.deps.Jobs.DailyUsage(context.Background(), f.ownerID)
	if err != nil || beforeUsage != afterUsage {
		t.Fatalf("refresh consumed quota: before=%d after=%d error=%v", beforeUsage, afterUsage, err)
	}
	response = performJSON(handler, http.MethodPost, path, nil, f.ownerToken)
	if response.Code != http.StatusTooManyRequests || response.Header().Get("Retry-After") == "" || calls.Load() != 1 {
		t.Fatalf("refresh cooldown status=%d headers=%v calls=%d", response.Code, response.Header(), calls.Load())
	}
	// The same work in another owner's task shares its refresh cooldown.
	response = performJSON(handler, http.MethodPost, "/api/v1/admin/jobs/other-job/music/refresh", nil, adminToken)
	if response.Code != http.StatusTooManyRequests || calls.Load() != 1 {
		t.Fatalf("admin work cooldown status=%d calls=%d", response.Code, calls.Load())
	}
}

func TestMusicRefreshAdminAndErrorResponses(t *testing.T) {
	f := newArchiveDownloadFixture(t)
	var calls atomic.Int32
	adminToken := prepareMusicRefreshHTTP(t, f, musicHTTPAdapter(func(ctx context.Context, input string) (resolver.Work, error) {
		calls.Add(1)
		return resolver.Work{}, errors.New("private upstream URL must not reach response")
	}))
	handler := musicRefreshHTTPHandler(f.deps)
	path := "/api/v1/admin/jobs/job-1/music/refresh"
	response := performJSON(handler, http.MethodPost, path, nil, adminToken)
	if response.Code != http.StatusBadGateway || calls.Load() != 1 || strings.Contains(response.Body.String(), "private upstream") {
		t.Fatalf("admin refresh failure status=%d calls=%d body=%s", response.Code, calls.Load(), response.Body.String())
	}
	current, err := f.deps.Jobs.Get(context.Background(), f.ownerID, "job-1")
	if err != nil || current.Work.MusicURL != "https://example.test/old.mp3" {
		t.Fatalf("failed refresh lost source fallback: %+v error=%v", current.Work, err)
	}
	response = performJSON(handler, http.MethodPost, path, nil, adminToken)
	if response.Code != http.StatusTooManyRequests || calls.Load() != 1 {
		t.Fatalf("failed refresh cooldown status=%d calls=%d", response.Code, calls.Load())
	}
	f.deps.Jobs = jobs.NewService(f.repo, nil)
	handler = musicRefreshHTTPHandler(f.deps)
	response = performJSON(handler, http.MethodPost, "/api/v1/jobs/job-1/music/refresh", nil, f.ownerToken)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("missing resolver status=%d", response.Code)
	}
	if _, err := f.db.ExecContext(context.Background(), `UPDATE jobs SET status='queued' WHERE id='job-1'`); err != nil {
		t.Fatal(err)
	}
	response = performJSON(handler, http.MethodPost, "/api/v1/jobs/job-1/music/refresh", nil, f.ownerToken)
	if response.Code != http.StatusConflict {
		t.Fatalf("unfinished task refresh status=%d", response.Code)
	}
}
