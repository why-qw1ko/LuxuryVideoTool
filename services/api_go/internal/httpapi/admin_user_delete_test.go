package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/audit"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/auth"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/jobs"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/settings"
)

func TestAdminDeleteUserPreservesHistory(t *testing.T) {
	fixture := newArchiveDownloadFixture(t)
	ctx := context.Background()
	service := fixture.deps.Auth
	const password = "OwnerPass!"
	device := auth.Device{ID: "delete-test-device", Name: "PC", Platform: "windows", AppVersion: "1"}
	admin, err := service.CreateUser(ctx, "maintainer", "Admin", password, auth.RoleAdmin)
	if err != nil {
		t.Fatal(err)
	}
	adminPair, err := service.Login(ctx, auth.LoginInput{Username: admin.UsernameNormalized, Password: password, Device: device})
	if err != nil {
		t.Fatal(err)
	}
	ownerPair, err := service.Login(ctx, auth.LoginInput{Username: "owner", Password: password, Device: device})
	if err != nil {
		t.Fatal(err)
	}
	store, err := settings.New(fixture.db, []byte("01234567890123456789012345678901"))
	if err != nil {
		t.Fatal(err)
	}
	recorder := audit.New(fixture.db, []byte("01234567890123456789012345678901"))
	if err := recorder.Record(ctx, audit.Event{
		ActorUserID: fixture.ownerID, Action: "job.create", TargetType: "job", TargetID: "job-1", RequestID: "history",
	}); err != nil {
		t.Fatal(err)
	}
	fixture.deps.Settings, fixture.deps.Audit = store, recorder
	handler := New(fixture.deps)
	request := func(method, path string, body any, token string, want int) *httptest.ResponseRecorder {
		t.Helper()
		res := performJSON(handler, method, path, body, token)
		if res.Code != want {
			t.Fatalf("%s %s status = %d, want %d, body = %s", method, path, res.Code, want, res.Body.String())
		}
		return res
	}

	const usersPath = "/api/v1/admin/users"
	ownerPath := usersPath + "/" + fixture.ownerID
	token := adminPair.AccessToken
	request(http.MethodDelete, ownerPath, nil, "", http.StatusUnauthorized)
	request(http.MethodDelete, ownerPath, nil, fixture.otherToken, http.StatusForbidden)
	request(http.MethodDelete, usersPath+"/"+admin.ID, nil, token, http.StatusBadRequest)
	codes, err := service.CreateCDKs(ctx, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	cdkPair, err := service.LoginCDK(ctx, codes[0].Code, device)
	if err != nil {
		t.Fatal(err)
	}
	request(http.MethodDelete, usersPath+"/"+cdkPair.User.ID, nil, token, http.StatusNotFound)
	deleted := request(http.MethodDelete, ownerPath, nil, token, http.StatusOK)
	if !strings.Contains(deleted.Body.String(), `"requestId":`) {
		t.Fatal("successful deletion did not return JSON with a request ID")
	}
	request(http.MethodDelete, ownerPath, nil, token, http.StatusNotFound)
	request(http.MethodPatch, ownerPath+"/active", map[string]bool{"active": true}, token, http.StatusNotFound)
	request(http.MethodPost, ownerPath+"/password", map[string]string{"password": "NewPass!"}, token, http.StatusNotFound)
	request(http.MethodPut, ownerPath+"/daily-task-limit", map[string]int{"dailyTaskLimit": 1}, token, http.StatusNotFound)
	request(http.MethodGet, "/api/v1/auth/sessions", nil, fixture.ownerToken, http.StatusUnauthorized)
	request(http.MethodPost, "/api/v1/auth/refresh", map[string]string{"refreshToken": ownerPair.RefreshToken}, "", http.StatusUnauthorized)
	request(http.MethodPost, "/api/v1/auth/login", map[string]any{"username": "owner", "password": password, "device": device}, "", http.StatusUnauthorized)
	request(http.MethodPost, usersPath, map[string]string{"username": "Owner", "displayName": "New owner", "password": password, "role": "user"}, token, http.StatusConflict)

	list := request(http.MethodGet, usersPath, nil, token, http.StatusOK)
	if strings.Contains(list.Body.String(), fixture.ownerID) {
		t.Fatal("deleted user remains in user management")
	}
	stats := request(http.MethodGet, "/api/v1/admin/stats", nil, token, http.StatusOK)
	if body := stats.Body.String(); !strings.Contains(body, `"users":2`) || !strings.Contains(body, `"totalJobs":2`) {
		t.Fatalf("unexpected user or job statistics after deletion: %s", body)
	}
	request(http.MethodGet, "/api/v1/admin/jobs/job-1", nil, token, http.StatusOK)
	file := request(http.MethodGet, "/api/v1/admin/files/"+fixture.media[0].ID, nil, token, http.StatusOK)
	if file.Body.String() != "first-image" {
		t.Fatal("deleted user's task file was not preserved")
	}
	if _, _, err := fixture.deps.Jobs.CreateDownload(ctx, jobs.CreateInput{
		UserID: fixture.ownerID, ShareText: "https://www.douyin.com/video/123", IdempotencyKey: "in-flight",
	}); !errors.Is(err, auth.ErrRevokedSession) {
		t.Fatalf("preauthenticated request can create tasks for a deleted account: %v", err)
	}

	var auditCount int
	if err := fixture.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_logs
		WHERE (actor_user_id = ? AND action = 'job.create' AND target_id = 'job-1')
		OR (actor_user_id = ? AND action = 'admin.user.delete' AND target_type = 'user' AND target_id = ?)`,
		fixture.ownerID, admin.ID, fixture.ownerID).Scan(&auditCount); err != nil || auditCount != 2 {
		t.Fatalf("historical and deletion audit count = %d, error = %v", auditCount, err)
	}
}

func TestAdminDeleteUserRollsBackWhenAuditFails(t *testing.T) {
	fixture := newArchiveDownloadFixture(t)
	ctx := context.Background()
	service := fixture.deps.Auth
	const password = "OwnerPass!"
	admin, err := service.CreateUser(ctx, "maintainer", "Admin", password, auth.RoleAdmin)
	if err != nil {
		t.Fatal(err)
	}
	device := auth.Device{ID: "delete-test-device", Name: "PC", Platform: "windows", AppVersion: "1"}
	pair, err := service.Login(ctx, auth.LoginInput{Username: admin.UsernameNormalized, Password: password, Device: device})
	if err != nil {
		t.Fatal(err)
	}
	fixture.deps.Audit = audit.New(fixture.db, []byte("01234567890123456789012345678901"))
	if _, err := fixture.db.ExecContext(ctx, `CREATE TRIGGER fail_user_delete_audit BEFORE INSERT ON audit_logs
		WHEN NEW.action = 'admin.user.delete' BEGIN SELECT RAISE(FAIL, 'test audit failure'); END`); err != nil {
		t.Fatal(err)
	}
	handler := New(fixture.deps)
	ownerPath := "/api/v1/admin/users/" + fixture.ownerID
	rejected := performJSON(handler, http.MethodDelete, ownerPath, nil, pair.AccessToken)
	if rejected.Code != http.StatusInternalServerError || !strings.Contains(rejected.Body.String(), `"code":"INTERNAL_ERROR"`) {
		t.Fatalf("delete with failed audit: status = %d, body = %s", rejected.Code, rejected.Body.String())
	}
	sessions := performJSON(handler, http.MethodGet, "/api/v1/auth/sessions", nil, fixture.ownerToken)
	if sessions.Code != http.StatusOK {
		t.Fatalf("failed deletion revoked the existing session: status = %d, body = %s", sessions.Code, sessions.Body.String())
	}
	if _, err := fixture.db.ExecContext(ctx, "DROP TRIGGER fail_user_delete_audit"); err != nil {
		t.Fatal(err)
	}
	retried := performJSON(handler, http.MethodDelete, ownerPath, nil, pair.AccessToken)
	if retried.Code != http.StatusOK {
		t.Fatalf("retry deletion after audit restored: status = %d, body = %s", retried.Code, retried.Body.String())
	}
}
