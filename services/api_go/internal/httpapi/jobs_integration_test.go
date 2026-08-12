package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/auth"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/database"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/jobs"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/resolver"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/version"
)

func TestInfoJobHTTPFlow(t *testing.T) {
	db, err := database.Open(context.Background(), filepath.Join(t.TempDir(), "api.db")); if err != nil { t.Fatal(err) }; defer db.Close()
	tokens, err := auth.NewTokenManager([]byte("01234567890123456789012345678901"), 15*time.Minute); if err != nil { t.Fatal(err) }
	authService, err := auth.NewService(auth.NewSQLiteRepository(db), tokens, 24*time.Hour); if err != nil { t.Fatal(err) }
	if _, err := authService.CreateUser(context.Background(), "owner", "Owner", "correct horse battery staple", auth.RoleUser); err != nil { t.Fatal(err) }
	fake := &resolver.Fake{Work: resolver.Work{DouyinWorkID: "123", Type: "video", CanonicalURL: "https://www.douyin.com/video/123", Title: "标题", ResolverName: "fake", ResolverVersion: resolver.DouyinResolverVersion, ResolvedAt: time.Now().UTC()}}
	jobService := jobs.NewService(jobs.NewSQLiteRepository(db), resolver.NewService(fake, resolver.NewSQLiteCache(db), 6*time.Hour, resolver.DouyinResolverVersion))
	handler := New(Dependencies{Build: version.Info{Version: "test"}, Auth: authService, Jobs: jobService, LoginRateLimit: 5, Ready: func() error { return nil }})
	login := performJSON(handler, http.MethodPost, "/api/v1/auth/login", map[string]any{"username": "owner", "password": "correct horse battery staple", "device": map[string]any{"id": "device", "name": "PC", "platform": "windows", "appVersion": "1"}}, "")
	var pair tokenResponse; if err := json.Unmarshal(login.Body.Bytes(), &pair); err != nil { t.Fatal(err) }
	request := performJSON(handler, http.MethodPost, "/api/v1/jobs", map[string]any{"shareText": "https://www.douyin.com/video/123", "action": "info", "options": map[string]any{"force": false}}, pair.AccessToken)
	if request.Code != http.StatusBadRequest { t.Fatalf("missing idempotency status = %d", request.Code) }

	// Build the request explicitly because performJSON intentionally has no extra-header parameter.
	body := `{"shareText":"https://www.douyin.com/video/123","action":"info","options":{"force":false}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+pair.AccessToken); req.Header.Set("Idempotency-Key", "key-1")
	recorder := httptest.NewRecorder(); handler.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusAccepted { t.Fatalf("create status=%d body=%s", recorder.Code, recorder.Body.String()) }
}

func TestDownloadJobHTTPIsQueued(t *testing.T) {
	db, err := database.Open(context.Background(), filepath.Join(t.TempDir(), "download.db")); if err != nil { t.Fatal(err) }; defer db.Close()
	tokens, err := auth.NewTokenManager([]byte("01234567890123456789012345678901"), 15*time.Minute); if err != nil { t.Fatal(err) }
	authService, err := auth.NewService(auth.NewSQLiteRepository(db), tokens, 24*time.Hour); if err != nil { t.Fatal(err) }
	if _, err := authService.CreateUser(context.Background(), "owner", "Owner", "correct horse battery staple", auth.RoleUser); err != nil { t.Fatal(err) }
	jobService := jobs.NewService(jobs.NewSQLiteRepository(db), nil)
	handler := New(Dependencies{Build: version.Info{Version: "test"}, Auth: authService, Jobs: jobService, LoginRateLimit: 5, Ready: func() error { return nil }})
	login := performJSON(handler, http.MethodPost, "/api/v1/auth/login", map[string]any{"username": "owner", "password": "correct horse battery staple", "device": map[string]any{"id": "device", "name": "PC", "platform": "windows", "appVersion": "1"}}, "")
	var pair tokenResponse; if err := json.Unmarshal(login.Body.Bytes(), &pair); err != nil { t.Fatal(err) }
	body := `{"shareText":"https://www.douyin.com/video/123","action":"download","options":{"force":false}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs", bytes.NewBufferString(body)); req.Header.Set("Authorization", "Bearer "+pair.AccessToken); req.Header.Set("Idempotency-Key", "download-key")
	recorder := httptest.NewRecorder(); handler.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusAccepted || !bytes.Contains(recorder.Body.Bytes(), []byte(`"status":"queued"`)) { t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String()) }
}
