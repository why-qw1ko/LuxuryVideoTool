package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/auth"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/jobs"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/settings"
)

func TestUsageSettingsControlAcceptedTasks(t *testing.T) {
	ctx := context.Background()
	authService, db, closeDB := testAuthService(t)
	defer closeDB()
	const password = "test password 12345"
	login := func(username string, role auth.Role) (auth.User, string) {
		t.Helper()
		user, err := authService.CreateUser(ctx, username, username, password, role)
		if err != nil {
			t.Fatal(err)
		}
		pair, err := authService.Login(ctx, auth.LoginInput{Username: username, Password: password, Device: auth.Device{ID: "pc", Name: "PC", Platform: "windows", AppVersion: "1"}})
		if err != nil {
			t.Fatal(err)
		}
		return user, pair.AccessToken
	}
	_, adminToken := login("admin", auth.RoleAdmin)
	user, userToken := login("member", auth.RoleUser)
	key := []byte("01234567890123456789012345678901")
	store, err := settings.New(db, key)
	if err != nil {
		t.Fatal(err)
	}
	handler := New(Dependencies{Auth: authService, Jobs: jobs.NewService(jobs.NewSQLiteRepository(db), nil), Settings: store})
	request := func(method, path string, body any, token string, want int) *httptest.ResponseRecorder {
		t.Helper()
		response := performJSON(handler, method, path, body, token)
		if response.Code != want {
			t.Fatalf("%s %s status=%d, want %d: %s", method, path, response.Code, want, response.Body.String())
		}
		return response
	}
	const settingsPath = "/api/v1/admin/settings/usage"
	const usagePath = "/api/v1/jobs/usage"
	overridePath := "/api/v1/admin/users/" + user.ID + "/daily-task-limit"
	policy := settings.DefaultUsage()
	policy.DailyTaskLimit = 1
	policy.AllowInfo = false
	policy.DailyASRBudgetCNY = 12.5
	policy.MonthlyASRBudgetCNY = 100
	policy.ASRPricePerMinuteCNY = 0.06
	request(http.MethodGet, settingsPath, nil, "", http.StatusUnauthorized)
	request(http.MethodGet, settingsPath, nil, userToken, http.StatusForbidden)
	request(http.MethodPut, settingsPath, policy, userToken, http.StatusForbidden)
	request(http.MethodPut, overridePath, map[string]any{"dailyTaskLimit": 0}, userToken, http.StatusForbidden)
	request(http.MethodPut, settingsPath, map[string]any{"dailyTaskLimit": 1}, adminToken, http.StatusBadRequest)
	request(http.MethodPut, settingsPath, policy, adminToken, http.StatusOK)
	storeAfterRestart, err := settings.New(db, key)
	if err != nil {
		t.Fatal(err)
	}
	if value, err := storeAfterRestart.Usage(ctx); err != nil || value != policy {
		t.Fatalf("stored usage=%+v, want %+v, error=%v", value, policy, err)
	}

	submit := func(action, key, token string, want int) *httptest.ResponseRecorder {
		t.Helper()
		payload, err := json.Marshal(map[string]string{"shareText": "https://www.douyin.com/video/123", "action": action})
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs", bytes.NewReader(payload))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Idempotency-Key", key)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		if response.Code != want {
			t.Fatalf("task %s/%s status=%d, want %d: %s", action, key, response.Code, want, response.Body.String())
		}
		return response
	}
	denied := submit("info", "paused", userToken, http.StatusForbidden)
	if !strings.Contains(denied.Body.String(), `"code":"TASK_ACTION_DISABLED"`) {
		t.Fatal(denied.Body.String())
	}
	submit("download", "first", userToken, http.StatusAccepted)
	replayed := submit("download", "first", userToken, http.StatusAccepted)
	if !strings.Contains(replayed.Body.String(), `"reused":true`) {
		t.Fatal(replayed.Body.String())
	}
	limited := submit("full", "second", userToken, http.StatusTooManyRequests)
	if limited.Header().Get("Retry-After") == "" || !strings.Contains(limited.Body.String(), `"code":"DAILY_TASK_LIMIT"`) {
		t.Fatalf("missing quota error metadata: %v %s", limited.Header(), limited.Body.String())
	}
	submit("download", "admin-first", adminToken, http.StatusAccepted)
	submit("download", "admin-second", adminToken, http.StatusTooManyRequests)

	request(http.MethodPut, overridePath, map[string]any{"dailyTaskLimit": 100001}, adminToken, http.StatusBadRequest)
	request(http.MethodPut, "/api/v1/admin/users/missing/daily-task-limit", map[string]any{"dailyTaskLimit": nil}, adminToken, http.StatusNotFound)
	request(http.MethodPut, overridePath, map[string]any{"dailyTaskLimit": 0}, adminToken, http.StatusOK)
	submit("full", "second", userToken, http.StatusAccepted)
	list := request(http.MethodGet, "/api/v1/admin/users", nil, adminToken, http.StatusOK)
	if !strings.Contains(list.Body.String(), `"dailyTaskLimitOverride":0`) {
		t.Fatal(list.Body.String())
	}
	request(http.MethodPut, overridePath, map[string]any{"dailyTaskLimit": nil}, adminToken, http.StatusOK)
	usage := request(http.MethodGet, usagePath, nil, userToken, http.StatusOK)
	if !strings.Contains(usage.Body.String(), `"used":2`) || !strings.Contains(usage.Body.String(), `"limit":1`) {
		t.Fatal(usage.Body.String())
	}
	submit("download", "third", userToken, http.StatusTooManyRequests)
}
