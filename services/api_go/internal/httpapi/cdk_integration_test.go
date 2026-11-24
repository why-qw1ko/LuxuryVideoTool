package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/auth"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/jobs"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/settings"
)

const cdkAdminPath = "/api/v1/admin/cdks"
const cdkLoginPath = "/api/v1/auth/cdk/login"

func TestCDKHTTPFlow(t *testing.T) {
	ctx := context.Background()
	service, db, closeDB := testAuthService(t)
	defer closeDB()
	device := auth.Device{ID: "web-device", Name: "Browser", Platform: "windows", AppVersion: "web-test"}
	const password = "OwnerPass!"
	admin, err := service.CreateUser(ctx, "owner", "Owner", password, auth.RoleAdmin)
	if err != nil {
		t.Fatal(err)
	}
	store, err := settings.New(db, []byte("01234567890123456789012345678901"))
	if err != nil {
		t.Fatal(err)
	}
	policy := settings.DefaultUsage()
	policy.DailyTaskLimit = 1
	if err := store.SetUsage(ctx, policy, admin.ID); err != nil {
		t.Fatal(err)
	}
	handler := New(Dependencies{Auth: service, Jobs: jobs.NewService(jobs.NewSQLiteRepository(db), nil), Settings: store, LoginRateLimit: 20})
	request := func(method, path string, body any, token string, want int) *httptest.ResponseRecorder {
		t.Helper()
		response := performJSON(handler, method, path, body, token)
		if response.Code != want {
			t.Fatalf("%s %s status = %d, want %d", method, path, response.Code, want)
		}
		return response
	}
	adminResponse := request(http.MethodPost, "/api/v1/auth/login", map[string]any{"username": "owner", "password": password, "device": device}, "", http.StatusOK)
	var adminPair tokenResponse
	if err := json.Unmarshal(adminResponse.Body.Bytes(), &adminPair); err != nil || adminPair.User.IsCDK {
		t.Fatalf("decode normal login = %v; isCDK = %v", err, adminPair.User.IsCDK)
	}
	token := adminPair.AccessToken
	request(http.MethodGet, cdkAdminPath, nil, "", http.StatusUnauthorized)
	request(http.MethodPost, cdkAdminPath, map[string]int{"count": 101, "taskLimit": 1}, token, http.StatusBadRequest)
	request(http.MethodPost, cdkAdminPath, map[string]int{"count": 1, "taskLimit": 0}, token, http.StatusBadRequest)
	generate := func(count int) []auth.CDK {
		t.Helper()
		response := request(http.MethodPost, cdkAdminPath, map[string]int{"count": count, "taskLimit": 1}, token, http.StatusCreated)
		var result struct{ Items []auth.CDK }
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || len(result.Items) != count {
			t.Fatalf("decode generation = %v, count = %d", err, len(result.Items))
		}
		if response.Header().Get("Cache-Control") != "private, no-store" {
			t.Fatal("generation response lacks Cache-Control")
		}
		for _, item := range result.Items {
			if len(item.Code) <= len(item.Prefix) || !strings.HasPrefix(item.Code, item.Prefix) || item.Status != "unused" || item.TaskLimit != 1 {
				t.Fatal("invalid CDK generated")
			}
			var hash string
			if err := db.QueryRowContext(ctx, "SELECT code_hash FROM login_cdks WHERE id = ?", item.ID).Scan(&hash); err != nil || hash != auth.HashRefreshToken(item.Code) {
				t.Fatalf("CDK digest lookup failed: %v", err)
			}
		}
		return result.Items
	}
	key := generate(1)[0]
	batch := generate(2)
	if batch[0].Code == batch[1].Code || batch[0].Code == key.Code || batch[1].Code == key.Code {
		t.Fatal("generated duplicate codes")
	}
	page := request(http.MethodGet, cdkAdminPath+"?limit=1&offset=1", nil, token, http.StatusOK)
	var list struct {
		Items []auth.CDK
		Total int
	}
	if err := json.Unmarshal(page.Body.Bytes(), &list); err != nil || list.Total != 3 || len(list.Items) != 1 || list.Items[0].Code != "" {
		t.Fatalf("unexpected CDK list, decode error = %v", err)
	}

	login := func(code string, want int) *httptest.ResponseRecorder {
		t.Helper()
		return request(http.MethodPost, cdkLoginPath, map[string]any{"code": code, "device": device}, "", want)
	}
	crossSite := webRequest(http.MethodPost, cdkLoginPath, map[string]any{"code": key.Code, "device": device}, nil)
	crossSite.Header.Set("Origin", "https://different.example")
	crossSiteResponse := httptest.NewRecorder()
	handler.ServeHTTP(crossSiteResponse, crossSite)
	if crossSiteResponse.Code != http.StatusForbidden {
		t.Fatalf("cross-site CDK login status = %d", crossSiteResponse.Code)
	}
	invalidDevice := device
	invalidDevice.Platform = "unsupported"
	if _, err := service.LoginCDK(ctx, key.Code, invalidDevice); err == nil {
		t.Fatal("invalid session platform should roll back redemption")
	}
	loggedIn := login(" "+strings.ToLower(key.Code)+" ", http.StatusOK)
	var cdkPair tokenResponse
	if err := json.Unmarshal(loggedIn.Body.Bytes(), &cdkPair); err != nil || cdkPair.User.Role != auth.RoleUser || !cdkPair.User.IsCDK || cdkPair.RefreshToken != "" {
		t.Fatalf("invalid web CDK token response, decode error = %v", err)
	}
	cookies := loggedIn.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteStrictMode {
		t.Fatal("web CDK login lacks protected refresh cookie")
	}
	login(key.Code, http.StatusUnauthorized)
	request(http.MethodGet, cdkAdminPath, nil, cdkPair.AccessToken, http.StatusForbidden)
	request(http.MethodPost, "/api/v1/auth/password", map[string]string{"currentPassword": password, "newPassword": password}, cdkPair.AccessToken, http.StatusForbidden)
	request(http.MethodPost, "/api/v1/admin/users/"+cdkPair.User.ID+"/password", map[string]string{"password": password}, token, http.StatusNotFound)
	request(http.MethodPatch, "/api/v1/admin/users/"+cdkPair.User.ID+"/active", map[string]bool{"active": false}, token, http.StatusNotFound)
	request(http.MethodPut, "/api/v1/admin/users/"+cdkPair.User.ID+"/daily-task-limit", map[string]int{"dailyTaskLimit": 0}, token, http.StatusNotFound)
	users := request(http.MethodGet, "/api/v1/admin/users", nil, token, http.StatusOK)
	if strings.Contains(users.Body.String(), cdkPair.User.ID) {
		t.Fatal("CDK account included in regular user management")
	}

	refreshed := httptest.NewRecorder()
	handler.ServeHTTP(refreshed, webRequest(http.MethodPost, "/api/v1/auth/refresh", nil, cookies))
	if err := json.Unmarshal(refreshed.Body.Bytes(), &cdkPair); refreshed.Code != http.StatusOK || err != nil || !cdkPair.User.IsCDK {
		t.Fatalf("CDK refresh status = %d, decode error = %v", refreshed.Code, err)
	}
	active := cdkPair.AccessToken
	submit := func(idempotencyKey string, want int) *httptest.ResponseRecorder {
		t.Helper()
		job := webRequest(http.MethodPost, "/api/v1/jobs", map[string]string{"shareText": "https://www.douyin.com/video/123", "action": "download"}, nil)
		job.Header.Set("Authorization", "Bearer "+active)
		job.Header.Set("Idempotency-Key", idempotencyKey)
		result := httptest.NewRecorder()
		handler.ServeHTTP(result, job)
		if result.Code != want {
			t.Fatalf("job creation %s status = %d, want %d: %s", idempotencyKey, result.Code, want, result.Body.String())
		}
		return result
	}
	first := submit("first", http.StatusAccepted)
	var created struct{ Job jobs.Job }
	if err := json.Unmarshal(first.Body.Bytes(), &created); err != nil || created.Job.ID == "" {
		t.Fatalf("decode job = %v", err)
	}
	if body := submit("first", http.StatusAccepted).Body.String(); !strings.Contains(body, `"reused":true`) {
		t.Fatal(body)
	}
	if body := submit("second", http.StatusTooManyRequests).Body.String(); !strings.Contains(body, "CDK_LIMIT_REACHED") {
		t.Fatal(body)
	}
	request(http.MethodPatch, cdkAdminPath+"/"+key.ID, map[string]int{"taskLimit": 2}, token, http.StatusOK)
	if body := submit("second", http.StatusTooManyRequests).Body.String(); !strings.Contains(body, "DAILY_TASK_LIMIT") {
		t.Fatal(body)
	}
	request(http.MethodPost, "/api/v1/jobs/"+created.Job.ID+"/cancel", nil, active, http.StatusOK)
	request(http.MethodDelete, "/api/v1/jobs/"+created.Job.ID, nil, active, http.StatusOK)
	usage := request(http.MethodGet, "/api/v1/jobs/usage", nil, active, http.StatusOK)
	var result struct {
		Usage struct{ CDK auth.CDKUsage }
	}
	if err := json.Unmarshal(usage.Body.Bytes(), &result); err != nil || result.Usage.CDK != (auth.CDKUsage{Used: 1, Limit: 2}) {
		t.Fatalf("CDK usage = %+v, error = %v", result, err)
	}
	policy.DailyTaskLimit = 0
	if err := store.SetUsage(ctx, policy, admin.ID); err != nil {
		t.Fatal(err)
	}
	submit("second", http.StatusAccepted)
	if body := submit("third", http.StatusTooManyRequests).Body.String(); !strings.Contains(body, "CDK_LIMIT_REACHED") {
		t.Fatal(body)
	}

	request(http.MethodDelete, cdkAdminPath+"/"+batch[0].ID, nil, token, http.StatusOK)
	login(batch[0].Code, http.StatusUnauthorized)
	request(http.MethodDelete, cdkAdminPath+"/"+key.ID, nil, token, http.StatusOK)
	request(http.MethodGet, "/api/v1/jobs/usage", nil, active, http.StatusUnauthorized)
	request(http.MethodPatch, "/api/v1/admin/users/"+cdkPair.User.ID+"/active", map[string]bool{"active": true}, token, http.StatusNotFound)
	request(http.MethodPatch, cdkAdminPath+"/"+key.ID, map[string]int{"taskLimit": 3}, token, http.StatusConflict)
	revoked := httptest.NewRecorder()
	handler.ServeHTTP(revoked, webRequest(http.MethodPost, "/api/v1/auth/refresh", nil, refreshed.Result().Cookies()))
	if revoked.Code != http.StatusUnauthorized {
		t.Fatalf("revoked CDK refresh status = %d", revoked.Code)
	}
}

func TestCDKConcurrentRedemptionAndTaskQuota(t *testing.T) {
	ctx := context.Background()
	service, db, closeDB := testAuthService(t)
	defer closeDB()
	items, err := service.CreateCDKs(ctx, 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	device := auth.Device{ID: "pc", Name: "PC", Platform: "windows", AppVersion: "1"}
	results := make(chan auth.TokenPair, 8)
	var attempts sync.WaitGroup
	for range cap(results) {
		attempts.Go(func() {
			pair, err := service.LoginCDK(ctx, items[0].Code, device)
			switch {
			case err == nil:
				results <- pair
			case !errors.Is(err, auth.ErrInvalidCDK):
				t.Errorf("redeem CDK: %v", err)
			}
		})
	}
	attempts.Wait()
	close(results)
	if len(results) != 1 {
		t.Fatalf("successful concurrent redemptions = %d, want 1", len(results))
	}
	userID := (<-results).User.ID
	jobService := jobs.NewService(jobs.NewSQLiteRepository(db), nil)
	accepted := make(chan struct{}, 8)
	for index := range cap(accepted) {
		attempts.Go(func() {
			_, _, err := jobService.CreateDownload(ctx, jobs.CreateInput{
				UserID: userID, ShareText: "https://www.douyin.com/video/123", IdempotencyKey: string(rune('a' + index)),
			})
			switch {
			case err == nil:
				accepted <- struct{}{}
			case !errors.Is(err, jobs.ErrCDKTaskLimit):
				t.Errorf("create CDK job: %v", err)
			}
		})
	}
	attempts.Wait()
	close(accepted)
	if len(accepted) != 2 {
		t.Fatalf("concurrent new jobs accepted = %d, want 2", len(accepted))
	}
	usage, err := service.CDKUsage(ctx, userID)
	if err != nil || usage != (auth.CDKUsage{Used: 2, Limit: 2}) {
		t.Fatalf("CDK usage = %+v, error = %v", usage, err)
	}
}

func TestCDKUsesSharedLoginRateLimit(t *testing.T) {
	service, _, closeDB := testAuthService(t)
	defer closeDB()
	handler := New(Dependencies{Auth: service, LoginRateLimit: 1})
	device := auth.Device{ID: "pc", Name: "PC", Platform: "windows", AppVersion: "1"}
	cdk := performJSON(handler, http.MethodPost, cdkLoginPath, map[string]any{"code": "CDK-" + strings.Repeat("A", 32), "device": device}, "")
	if cdk.Code != http.StatusUnauthorized {
		t.Fatalf("invalid CDK login status = %d", cdk.Code)
	}
	account := performJSON(handler, http.MethodPost, "/api/v1/auth/login", map[string]any{"username": "owner", "password": "OwnerPass!", "device": device}, "")
	if account.Code != http.StatusTooManyRequests {
		t.Fatalf("account login after CDK attempt status = %d", account.Code)
	}
}
