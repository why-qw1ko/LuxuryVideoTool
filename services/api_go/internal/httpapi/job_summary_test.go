package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/auth"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/jobs"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/resolver"
)

func TestJobSummaryListsPreservePermissionsAndFullDetails(t *testing.T) {
	fixture := newArchiveDownloadFixture(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)
	earlier, later := now.Add(time.Hour), now.Add(2*time.Hour)
	fixture.addFile(t, "job-1", fixture.ownerID, "image", "earliest.jpg", []byte("early"), &earlier)
	fixture.addFile(t, "job-1", fixture.ownerID, "image", "later.jpg", []byte("later"), &later)
	work, err := resolver.NewSQLiteCache(fixture.db).Save(ctx, resolver.Work{
		DouyinWorkID: "123", Type: "note", CanonicalURL: "https://www.douyin.com/note/123",
		Title: "Gallery summary", AuthorName: "Artist", DurationMS: 1200, Hashtags: []string{"gallery"},
		Images: []resolver.Image{{URL: "https://example.com/original.jpg"}}, ResolvedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := json.Marshal(map[string]string{"normalizedText": strings.Repeat("full transcript with a longer sample of extracted caption text. ", 30)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.db.ExecContext(ctx, `UPDATE jobs SET work_id = ?, result_json = ? WHERE id = ?`, work.ID, string(result), "job-1"); err != nil {
		t.Fatal(err)
	}
	const frontPath = "/api/v1/jobs?view=summary&status=completed&q=Gallery"
	response := performJSON(fixture.handler, http.MethodGet, frontPath+"&userId="+url.QueryEscape(fixture.otherID), nil, fixture.ownerToken)
	if response.Code != http.StatusOK {
		t.Fatalf("list summary: %d %s", response.Code, response.Body.String())
	}
	var page struct {
		Jobs   []jobs.JobSummary `json:"jobs"`
		Total  int               `json:"total"`
		Limit  int               `json:"limit"`
		Offset int               `json:"offset"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || page.Limit != 20 || page.Offset != 0 || len(page.Jobs) != 1 {
		t.Fatalf("unexpected page: %+v", page)
	}
	summary := page.Jobs[0]
	if summary.ID != "job-1" || summary.OwnerUsername != "" || summary.MediaExpiresAt == nil || !summary.MediaExpiresAt.Equal(earlier) {
		t.Fatalf("unexpected job: %+v", summary)
	}
	if summary.Work == nil || summary.Work.Title != "Gallery summary" || summary.Work.AuthorName != "Artist" || summary.Work.DurationMS != 1200 || !slices.Equal(summary.Work.Hashtags, []string{"gallery"}) {
		t.Fatalf("unexpected work: %+v", summary.Work)
	}
	for _, field := range []string{`"result"`, `"files"`, `"previewUrl"`, `"images"`, `"ownerDisplayName"`} {
		if strings.Contains(response.Body.String(), field) {
			t.Fatalf("summary should omit %s", field)
		}
	}
	for _, path := range []string{"/api/v1/jobs?status=completed&q=Gallery", "/api/v1/jobs/job-1"} {
		full := performJSON(fixture.handler, http.MethodGet, path, nil, fixture.ownerToken)
		if full.Code != http.StatusOK || !strings.Contains(full.Body.String(), `"previewUrl"`) || !strings.Contains(full.Body.String(), "full transcript with") || !strings.Contains(full.Body.String(), `"images"`) {
			t.Fatalf("default response changed %s: status %d, %d bytes", path, full.Code, full.Body.Len())
		}
		if strings.HasPrefix(path, "/api/v1/jobs?") {
			t.Logf("same job fixture: full list %d bytes, summary list %d bytes", full.Body.Len(), response.Body.Len())
		}
	}
	for _, test := range []struct {
		path, token string
		status      int
	}{
		{frontPath, "", http.StatusUnauthorized},
		{"/api/v1/admin/jobs?view=summary", fixture.ownerToken, http.StatusForbidden},
		{"/api/v1/jobs/other-job", fixture.ownerToken, http.StatusNotFound},
	} {
		response := performJSON(fixture.handler, http.MethodGet, test.path, nil, test.token)
		if response.Code != test.status {
			t.Fatalf("%s: got %d, want %d", test.path, response.Code, test.status)
		}
	}
	const password = "OwnerPass!"
	if _, err := fixture.deps.Auth.CreateUser(ctx, "admin", "Admin", password, auth.RoleAdmin); err != nil {
		t.Fatal(err)
	}
	login := performJSON(fixture.handler, http.MethodPost, "/api/v1/auth/login", map[string]any{
		"username": "admin", "password": password,
		"device": map[string]string{"id": "admin-device", "name": "PC", "platform": "windows", "appVersion": "1"},
	}, "")
	var credentials tokenResponse
	if err := json.Unmarshal(login.Body.Bytes(), &credentials); err != nil || login.Code != http.StatusOK {
		t.Fatalf("admin login: %d %s", login.Code, login.Body.String())
	}
	admin := performJSON(fixture.handler, http.MethodGet, "/api/v1/admin/jobs?view=summary&userId="+url.QueryEscape(fixture.otherID), nil, credentials.AccessToken)
	if admin.Code != http.StatusOK {
		t.Fatalf("admin list: %d %s", admin.Code, admin.Body.String())
	}
	page.Jobs = nil
	if err := json.Unmarshal(admin.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || len(page.Jobs) != 1 || page.Jobs[0].ID != "other-job" || page.Jobs[0].OwnerDisplayName != "Other" {
		t.Fatalf("admin owner filter: %+v", page)
	}
}
