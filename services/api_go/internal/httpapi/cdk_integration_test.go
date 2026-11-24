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
	"time"

	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/audit"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/auth"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/database"
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
	handler := New(Dependencies{Auth: service, Jobs: jobs.NewService(jobs.NewSQLiteRepository(db), nil), Settings: store,
		Audit: audit.New(db, []byte("01234567890123456789012345678901")), LoginRateLimit: 20})
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
	keyPath := cdkAdminPath + "/" + key.ID
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
	for _, action := range []string{"/reset", "/rotate"} {
		request(http.MethodPost, keyPath+action, nil, "", http.StatusUnauthorized)
		request(http.MethodPost, keyPath+action, nil, cdkPair.AccessToken, http.StatusForbidden)
		request(http.MethodPost, cdkAdminPath+"/missing"+action, nil, token, http.StatusNotFound)
	}
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
	request(http.MethodPatch, keyPath, map[string]int{"taskLimit": 2}, token, http.StatusOK)
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
	second := submit("second", http.StatusAccepted)
	if err := json.Unmarshal(second.Body.Bytes(), &created); err != nil || created.Job.ID == "" {
		t.Fatalf("decode second job = %v", err)
	}
	if body := submit("third", http.StatusTooManyRequests).Body.String(); !strings.Contains(body, "CDK_LIMIT_REACHED") {
		t.Fatal(body)
	}

	// A replay in flight when the key is rotated must not revoke the replacement account session.
	repository := auth.NewSQLiteRepository(db)
	staleSession, _, err := repository.FindSessionByTokenHash(ctx, auth.HashRefreshToken(cookies[0].Value))
	if err != nil || staleSession.RevokedAt == nil {
		t.Fatalf("read previous refresh session: %v", err)
	}
	var updated struct{ Item auth.CDK }
	rotation := request(http.MethodPost, keyPath+"/rotate", nil, token, http.StatusOK)
	if err := json.Unmarshal(rotation.Body.Bytes(), &updated); err != nil || updated.Item.ID != key.ID || updated.Item.Code == "" ||
		updated.Item.Code == key.Code || !strings.HasPrefix(updated.Item.Code, updated.Item.Prefix) ||
		updated.Item.Status != "unused" || updated.Item.RedeemedAt != nil || updated.Item.TaskLimit != 2 || updated.Item.TaskCount != 2 ||
		rotation.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("invalid rotation response, decode error = %v", err)
	}
	newCode := updated.Item.Code
	if page := request(http.MethodGet, cdkAdminPath, nil, token, http.StatusOK); strings.Contains(page.Body.String(), newCode) {
		t.Fatal("rotated CDK disclosed in list response")
	}
	login(key.Code, http.StatusUnauthorized)
	request(http.MethodGet, "/api/v1/jobs/usage", nil, active, http.StatusUnauthorized)
	newLogin := login(newCode, http.StatusOK)
	var newPair tokenResponse
	if err := json.Unmarshal(newLogin.Body.Bytes(), &newPair); err != nil || newPair.User.ID != cdkPair.User.ID || newPair.User.DisplayName != updated.Item.Prefix {
		t.Fatalf("rotated CDK did not reuse its account, decode error = %v", err)
	}
	login(newCode, http.StatusUnauthorized)
	for _, oldCookies := range [][]*http.Cookie{cookies, refreshed.Result().Cookies()} {
		replayed := httptest.NewRecorder()
		handler.ServeHTTP(replayed, webRequest(http.MethodPost, "/api/v1/auth/refresh", nil, oldCookies))
		if replayed.Code != http.StatusUnauthorized {
			t.Fatalf("pre-rotation refresh replay status = %d", replayed.Code)
		}
	}
	if err := repository.RevokeSessionsAfterRefreshReplay(ctx, staleSession, time.Now()); err != nil {
		t.Fatal(err)
	}
	active = newPair.AccessToken
	request(http.MethodGet, "/api/v1/jobs/"+created.Job.ID, nil, active, http.StatusOK)
	if body := submit("third", http.StatusTooManyRequests).Body.String(); !strings.Contains(body, "CDK_LIMIT_REACHED") {
		t.Fatal(body)
	}

	updated.Item = auth.CDK{}
	reset := request(http.MethodPost, keyPath+"/reset", nil, token, http.StatusOK)
	if err := json.Unmarshal(reset.Body.Bytes(), &updated); err != nil || updated.Item.ID != key.ID || updated.Item.Code != "" ||
		updated.Item.Status != "redeemed" || updated.Item.TaskLimit != 2 || updated.Item.TaskCount != 0 {
		t.Fatalf("invalid reset response, decode error = %v", err)
	}
	policy.DailyTaskLimit = 1
	if err := store.SetUsage(ctx, policy, admin.ID); err != nil {
		t.Fatal(err)
	}
	if body := submit("third", http.StatusTooManyRequests).Body.String(); !strings.Contains(body, "DAILY_TASK_LIMIT") {
		t.Fatal(body)
	}
	policy.DailyTaskLimit = 0
	if err := store.SetUsage(ctx, policy, admin.ID); err != nil {
		t.Fatal(err)
	}
	if body := submit("second", http.StatusAccepted).Body.String(); !strings.Contains(body, `"reused":true`) {
		t.Fatal(body)
	}
	submit("third", http.StatusAccepted)
	for _, action := range []string{"admin.cdk.rotate", "admin.cdk.reset_task_count"} {
		var metadata string
		if err := db.QueryRowContext(ctx, `SELECT metadata_json FROM audit_logs WHERE action = ? AND target_id = ? AND actor_user_id = ?`,
			action, key.ID, admin.ID).Scan(&metadata); err != nil || strings.Contains(metadata, newCode) || strings.Contains(metadata, key.Code) {
			t.Fatalf("invalid %s audit record, query error = %v", action, err)
		}
	}

	request(http.MethodDelete, cdkAdminPath+"/"+batch[0].ID, nil, token, http.StatusOK)
	login(batch[0].Code, http.StatusUnauthorized)
	request(http.MethodDelete, keyPath, nil, token, http.StatusOK)
	request(http.MethodGet, "/api/v1/jobs/usage", nil, active, http.StatusUnauthorized)
	request(http.MethodPatch, "/api/v1/admin/users/"+cdkPair.User.ID+"/active", map[string]bool{"active": true}, token, http.StatusNotFound)
	request(http.MethodPatch, keyPath, map[string]int{"taskLimit": 3}, token, http.StatusConflict)
	request(http.MethodPost, keyPath+"/reset", nil, token, http.StatusConflict)
	request(http.MethodPost, keyPath+"/rotate", nil, token, http.StatusConflict)
	revoked := httptest.NewRecorder()
	handler.ServeHTTP(revoked, webRequest(http.MethodPost, "/api/v1/auth/refresh", nil, newLogin.Result().Cookies()))
	if revoked.Code != http.StatusUnauthorized {
		t.Fatalf("revoked CDK refresh status = %d", revoked.Code)
	}
}

func TestCDKCredentialMigrationPreservesRedemptions(t *testing.T) {
	ctx := context.Background()
	service, db, closeDB := testAuthService(t)
	defer closeDB()
	items, err := service.CreateCDKs(ctx, 2, 2)
	if err != nil {
		t.Fatal(err)
	}
	device := auth.Device{ID: "pc", Name: "PC", Platform: "windows", AppVersion: "1"}
	if _, err := service.LoginCDK(ctx, items[0].Code, device); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `ALTER TABLE login_cdks DROP COLUMN code_redeemed_at;
		DELETE FROM schema_migrations WHERE version = '0010_cdk_credential_redemption.sql'`); err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if _, err := service.LoginCDK(ctx, items[0].Code, device); !errors.Is(err, auth.ErrInvalidCDK) {
		t.Fatalf("previously used CDK login after migration: %v", err)
	}
	if _, err := service.LoginCDK(ctx, items[1].Code, device); err != nil {
		t.Fatalf("unused CDK login after migration: %v", err)
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
