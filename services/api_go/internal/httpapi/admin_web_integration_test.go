package httpapi

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/auth"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/database"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/jobs"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/version"
)

func testAuthService(t *testing.T) (*auth.Service, *sql.DB, func()) {
	t.Helper()
	db, err := database.Open(context.Background(), filepath.Join(t.TempDir(), "api.db"))
	if err != nil {
		t.Fatal(err)
	}
	tokens, err := auth.NewTokenManager([]byte("01234567890123456789012345678901"), 15*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	repository := auth.NewSQLiteRepository(db)
	service, err := auth.NewService(repository, tokens, 30*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return service, db, func() { db.Close() }
}

func webRequest(method, path string, body any, cookies []*http.Cookie) *http.Request {
	var encoded []byte
	if body != nil {
		encoded, _ = json.Marshal(body)
	}
	request := httptest.NewRequest(method, path, bytes.NewReader(encoded))
	request.RemoteAddr = "192.0.2.1:1234"
	request.Header.Set("Origin", "http://example.com")
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	return request
}

func TestWebCookieAuthFlow(t *testing.T) {
	service, _, closeDB := testAuthService(t)
	defer closeDB()
	if _, err := service.CreateUser(context.Background(), "owner", "Owner", "correct horse battery staple", auth.RoleAdmin); err != nil {
		t.Fatal(err)
	}
	handler := New(Dependencies{Build: version.Info{Version: "test"}, Auth: service, LoginRateLimit: 5, Ready: func() error { return nil }})

	login := httptest.NewRecorder()
	handler.ServeHTTP(login, webRequest(http.MethodPost, "/api/v1/auth/login", map[string]any{
		"username": "owner", "password": "correct horse battery staple",
		"device": map[string]any{"id": "web-device", "name": "Chrome", "platform": "windows", "appVersion": "web-0.1.0"},
	}, nil))
	if login.Code != http.StatusOK {
		t.Fatalf("web login status = %d, body = %s", login.Code, login.Body.String())
	}
	var pair tokenResponse
	if err := json.Unmarshal(login.Body.Bytes(), &pair); err != nil {
		t.Fatal(err)
	}
	if pair.AccessToken == "" || pair.RefreshToken != "" {
		t.Fatalf("web login must omit refresh token from body, got %#v", pair)
	}
	var refreshCookie *http.Cookie
	for _, cookie := range login.Result().Cookies() {
		if cookie.Name == refreshCookieName {
			refreshCookie = cookie
		}
	}
	if refreshCookie == nil || refreshCookie.Value == "" || !refreshCookie.HttpOnly || refreshCookie.SameSite != http.SameSiteStrictMode {
		t.Fatalf("web login must set httpOnly SameSite=Strict refresh cookie, got %#v", refreshCookie)
	}

	// Cookie 续期：不应携带请求体，也不应返回 refreshToken 给 JS。
	refreshed := httptest.NewRecorder()
	handler.ServeHTTP(refreshed, webRequest(http.MethodPost, "/api/v1/auth/refresh", nil, []*http.Cookie{refreshCookie}))
	if refreshed.Code != http.StatusOK {
		t.Fatalf("cookie refresh status = %d, body = %s", refreshed.Code, refreshed.Body.String())
	}
	var rotated tokenResponse
	if err := json.Unmarshal(refreshed.Body.Bytes(), &rotated); err != nil {
		t.Fatal(err)
	}
	if rotated.AccessToken == "" || rotated.RefreshToken != "" {
		t.Fatalf("cookie refresh must return access token only, got %#v", rotated)
	}
	var rotatedCookie *http.Cookie
	for _, cookie := range refreshed.Result().Cookies() {
		if cookie.Name == refreshCookieName {
			rotatedCookie = cookie
		}
	}
	if rotatedCookie == nil || rotatedCookie.Value == "" || rotatedCookie.Value == refreshCookie.Value {
		t.Fatalf("cookie refresh must rotate refresh token")
	}

	// 跨站 Origin 刷新必须被拒绝（CSRF 纵深防御）。
	crossOrigin := webRequest(http.MethodPost, "/api/v1/auth/refresh", nil, []*http.Cookie{rotatedCookie})
	crossOrigin.Header.Set("Origin", "https://evil.example")
	rejected := httptest.NewRecorder()
	handler.ServeHTTP(rejected, crossOrigin)
	if rejected.Code != http.StatusForbidden {
		t.Fatalf("cross-origin refresh status = %d, want 403", rejected.Code)
	}

	// 登出应清除 Cookie，且旧 refresh token 立即失效。
	logout := httptest.NewRecorder()
	request := webRequest(http.MethodPost, "/api/v1/auth/logout", nil, []*http.Cookie{rotatedCookie})
	request.Header.Set("Authorization", "Bearer "+rotated.AccessToken)
	handler.ServeHTTP(logout, request)
	if logout.Code != http.StatusOK {
		t.Fatalf("logout status = %d, body = %s", logout.Code, logout.Body.String())
	}
	cleared := false
	for _, cookie := range logout.Result().Cookies() {
		if cookie.Name == refreshCookieName && cookie.MaxAge <= 0 {
			cleared = true
		}
	}
	if !cleared {
		t.Fatal("logout must clear refresh cookie")
	}
	expired := httptest.NewRecorder()
	handler.ServeHTTP(expired, webRequest(http.MethodPost, "/api/v1/auth/refresh", nil, []*http.Cookie{rotatedCookie}))
	if expired.Code != http.StatusUnauthorized {
		t.Fatalf("refresh after logout status = %d, want 401", expired.Code)
	}
}

func TestAdminUsersAndJobsFlow(t *testing.T) {
	service, db, closeDB := testAuthService(t)
	defer closeDB()
	admin, err := service.CreateUser(context.Background(), "admin", "Admin", "admin password 123", auth.RoleAdmin)
	if err != nil {
		t.Fatal(err)
	}
	other, err := service.CreateUser(context.Background(), "user-other", "Other", "user password 123", auth.RoleUser)
	if err != nil {
		t.Fatal(err)
	}
	jobRepository := jobs.NewSQLiteRepository(db)
	jobService := jobs.NewService(jobRepository, nil)
	now := time.Now().UTC()
	if err := jobRepository.CreateQueued(context.Background(), jobs.Job{
		ID: "job-other", UserID: other.ID, InputText: "https://www.douyin.com/video/1", InputURL: "https://www.douyin.com/video/1",
		Action: "download", Status: "queued", StatusMessage: "等待处理", IdempotencyKey: "k1", MaxAttempts: 3,
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	handler := New(Dependencies{Build: version.Info{Version: "test"}, Auth: service, Jobs: jobService, LoginRateLimit: 5, Ready: func() error { return nil }})

	adminLogin := performJSON(handler, http.MethodPost, "/api/v1/auth/login", map[string]any{"username": "admin", "password": "admin password 123", "device": map[string]any{"id": "pc", "name": "PC", "platform": "windows", "appVersion": "1.0.0"}}, "")
	if adminLogin.Code != http.StatusOK {
		t.Fatalf("admin login status = %d", adminLogin.Code)
	}
	var pair tokenResponse
	if err := json.Unmarshal(adminLogin.Body.Bytes(), &pair); err != nil {
		t.Fatal(err)
	}

	created := performJSON(handler, http.MethodPost, "/api/v1/admin/users", map[string]any{"username": "bob", "displayName": "Bob", "password": "bob password 123", "role": "user"}, pair.AccessToken)
	if created.Code != http.StatusCreated {
		t.Fatalf("create user status = %d, body = %s", created.Code, created.Body.String())
	}
	duplicate := performJSON(handler, http.MethodPost, "/api/v1/admin/users", map[string]any{"username": "BOB", "displayName": "Bob 2", "password": "bob password 123", "role": "user"}, pair.AccessToken)
	if duplicate.Code != http.StatusConflict {
		t.Fatalf("duplicate user status = %d, want 409", duplicate.Code)
	}

	listUsers := performJSON(handler, http.MethodGet, "/api/v1/admin/users", nil, pair.AccessToken)
	if listUsers.Code != http.StatusOK || !strings.Contains(listUsers.Body.String(), `"username":"bob"`) {
		t.Fatalf("list users status = %d, body = %s", listUsers.Code, listUsers.Body.String())
	}
	disable := performJSON(handler, http.MethodPatch, "/api/v1/admin/users/"+admin.ID+"/active", map[string]any{"active": false}, pair.AccessToken)
	if disable.Code != http.StatusBadRequest {
		t.Fatalf("disable self status = %d, want 400", disable.Code)
	}

	listJobs := performJSON(handler, http.MethodGet, "/api/v1/admin/jobs", nil, pair.AccessToken)
	if listJobs.Code != http.StatusOK {
		t.Fatalf("admin jobs status = %d, body = %s", listJobs.Code, listJobs.Body.String())
	}
	if !strings.Contains(listJobs.Body.String(), `"id":"job-other"`) {
		t.Fatalf("admin jobs must include other users' jobs, body = %s", listJobs.Body.String())
	}
	stats := performJSON(handler, http.MethodGet, "/api/v1/admin/stats", nil, pair.AccessToken)
	if stats.Code != http.StatusOK || !strings.Contains(stats.Body.String(), `"totalJobs":1`) || !strings.Contains(stats.Body.String(), `"todayJobs":1`) {
		t.Fatalf("admin stats status = %d, body = %s", stats.Code, stats.Body.String())
	}
	detail := performJSON(handler, http.MethodGet, "/api/v1/admin/jobs/job-other", nil, pair.AccessToken)
	if detail.Code != http.StatusOK || !strings.Contains(detail.Body.String(), `"id":"job-other"`) {
		t.Fatalf("admin job detail status = %d, body = %s", detail.Code, detail.Body.String())
	}
	cancel := performJSON(handler, http.MethodPost, "/api/v1/admin/jobs/job-other/cancel", nil, pair.AccessToken)
	if cancel.Code != http.StatusOK {
		t.Fatalf("admin cancel status = %d, body = %s", cancel.Code, cancel.Body.String())
	}
	deleted := performJSON(handler, http.MethodDelete, "/api/v1/admin/jobs/job-other", nil, pair.AccessToken)
	if deleted.Code != http.StatusOK {
		t.Fatalf("admin delete status = %d, body = %s", deleted.Code, deleted.Body.String())
	}

	// 普通用户访问管理接口必须 403。
	userLogin := performJSON(handler, http.MethodPost, "/api/v1/auth/login", map[string]any{"username": "bob", "password": "bob password 123", "device": map[string]any{"id": "pc2", "name": "PC", "platform": "windows", "appVersion": "1.0.0"}}, "")
	if userLogin.Code != http.StatusOK {
		t.Fatalf("user login status = %d, body = %s", userLogin.Code, userLogin.Body.String())
	}
	var userPair tokenResponse
	if err := json.Unmarshal(userLogin.Body.Bytes(), &userPair); err != nil {
		t.Fatal(err)
	}
	forbidden := performJSON(handler, http.MethodGet, "/api/v1/admin/users", nil, userPair.AccessToken)
	if forbidden.Code != http.StatusForbidden {
		t.Fatalf("non-admin admin access status = %d, want 403", forbidden.Code)
	}
}

func TestJobsListPagination(t *testing.T) {
	service, db, closeDB := testAuthService(t)
	defer closeDB()
	user, err := service.CreateUser(context.Background(), "user-page", "PageUser", "user password 123", auth.RoleUser)
	if err != nil {
		t.Fatal(err)
	}
	jobRepository := jobs.NewSQLiteRepository(db)
	jobService := jobs.NewService(jobRepository, nil)
	now := time.Now().UTC()
	for index := 0; index < 25; index++ {
		job := jobs.Job{
			ID: fmt.Sprintf("job-%02d", index), UserID: user.ID,
			InputText: "https://www.douyin.com/video/" + fmt.Sprint(index), InputURL: "https://www.douyin.com/video/",
			Action: "download", Status: "queued", StatusMessage: "等待处理",
			IdempotencyKey: fmt.Sprintf("key-%02d", index), MaxAttempts: 3,
			CreatedAt: now.Add(time.Duration(index) * time.Minute), UpdatedAt: now,
		}
		if err := jobRepository.CreateQueued(context.Background(), job); err != nil {
			t.Fatal(err)
		}
	}
	handler := New(Dependencies{Build: version.Info{Version: "test"}, Auth: service, Jobs: jobService, LoginRateLimit: 5, Ready: func() error { return nil }})
	login := performJSON(handler, http.MethodPost, "/api/v1/auth/login", map[string]any{"username": "user-page", "password": "user password 123", "device": map[string]any{"id": "pc", "name": "PC", "platform": "windows", "appVersion": "1.0.0"}}, "")
	if login.Code != http.StatusOK {
		t.Fatalf("login status = %d", login.Code)
	}
	var pair tokenResponse
	if err := json.Unmarshal(login.Body.Bytes(), &pair); err != nil {
		t.Fatal(err)
	}
	first := performJSON(handler, http.MethodGet, "/api/v1/jobs?limit=20&offset=0", nil, pair.AccessToken)
	if first.Code != http.StatusOK || !strings.Contains(first.Body.String(), `"total":25`) {
		t.Fatalf("first page status = %d, body = %s", first.Code, first.Body.String())
	}
	var firstPage struct {
		Jobs []jobs.Job `json:"jobs"`
		Total int       `json:"total"`
	}
	if err := json.Unmarshal(first.Body.Bytes(), &firstPage); err != nil {
		t.Fatal(err)
	}
	if len(firstPage.Jobs) != 20 {
		t.Fatalf("first page item count = %d, want 20", len(firstPage.Jobs))
	}
	second := performJSON(handler, http.MethodGet, "/api/v1/jobs?limit=20&offset=20", nil, pair.AccessToken)
	if second.Code != http.StatusOK {
		t.Fatalf("second page status = %d", second.Code)
	}
	var secondPage struct {
		Jobs []jobs.Job `json:"jobs"`
	}
	if err := json.Unmarshal(second.Body.Bytes(), &secondPage); err != nil {
		t.Fatal(err)
	}
	if len(secondPage.Jobs) != 5 {
		t.Fatalf("second page item count = %d, want 5", len(secondPage.Jobs))
	}
}
