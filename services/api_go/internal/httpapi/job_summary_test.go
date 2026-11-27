package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/auth"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/jobs"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/media"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/resolver"
)

func TestAdminCanFilterHistoricalJobsByDeletedOwner(t *testing.T) {
	fixture := newArchiveDownloadFixture(t)
	ctx := context.Background()
	admin, err := fixture.deps.Auth.CreateUser(ctx, "maintainer", "Admin", "OwnerPass!", auth.RoleAdmin)
	if err != nil {
		t.Fatal(err)
	}
	pair, err := fixture.deps.Auth.Login(ctx, auth.LoginInput{
		Username: admin.UsernameNormalized, Password: "OwnerPass!",
		Device: auth.Device{ID: "job-owner-test", Name: "PC", Platform: "windows", AppVersion: "1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	withoutJobs, err := fixture.deps.Auth.CreateUser(ctx, "unused", "Unused", "OwnerPass!", auth.RoleUser)
	if err != nil {
		t.Fatal(err)
	}
	fixture.addJob(t, "second-job", fixture.ownerID, "completed")

	for _, id := range []string{fixture.ownerID, withoutJobs.ID} {
		response := performJSON(fixture.handler, http.MethodDelete, "/api/v1/admin/users/"+id, nil, pair.AccessToken)
		if response.Code != http.StatusOK {
			t.Fatalf("delete user: %d %s", response.Code, response.Body.String())
		}
	}

	const usersPath = "/api/v1/admin/users"
	const ownersPath = usersPath + "?includeDeletedJobOwners=1"
	if response := performJSON(fixture.handler, http.MethodGet, ownersPath, nil, fixture.otherToken); response.Code != http.StatusForbidden {
		t.Fatalf("non-admin read of job owners: %d", response.Code)
	}
	if response := performJSON(fixture.handler, http.MethodGet, usersPath, nil, pair.AccessToken); response.Code != http.StatusOK || strings.Contains(response.Body.String(), fixture.ownerID) {
		t.Fatalf("default user list includes deleted owners: %d %s", response.Code, response.Body.String())
	}

	response := performJSON(fixture.handler, http.MethodGet, ownersPath, nil, pair.AccessToken)
	if response.Code != http.StatusOK {
		t.Fatalf("list deleted job owners: %d %s", response.Code, response.Body.String())
	}
	var list struct {
		Users            []struct{ ID string }  `json:"users"`
		DeletedJobOwners []jobs.DeletedJobOwner `json:"deletedJobOwners"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if slices.ContainsFunc(list.Users, func(user struct{ ID string }) bool {
		return user.ID == fixture.ownerID || user.ID == withoutJobs.ID
	}) {
		t.Fatalf("deleted account in user management: %+v", list.Users)
	}
	want := []jobs.DeletedJobOwner{{ID: fixture.ownerID, Username: "owner", DisplayName: "Owner"}}
	if !slices.Equal(list.DeletedJobOwners, want) {
		t.Fatalf("deleted job owners = %+v, want %+v", list.DeletedJobOwners, want)
	}

	response = performJSON(fixture.handler, http.MethodGet, "/api/v1/admin/jobs?view=summary&userId="+url.QueryEscape(list.DeletedJobOwners[0].ID), nil, pair.AccessToken)
	if response.Code != http.StatusOK {
		t.Fatalf("list historical jobs: %d %s", response.Code, response.Body.String())
	}
	var page struct {
		Jobs []jobs.JobSummary `json:"jobs"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Jobs) != 2 || slices.ContainsFunc(page.Jobs, func(job jobs.JobSummary) bool { return job.OwnerUsername != "owner" }) {
		t.Fatalf("unexpected jobs for deleted owner: %+v", page.Jobs)
	}
}

func TestJobSummaryListsPreservePermissionsAndFullDetails(t *testing.T) {
	fixture := newArchiveDownloadFixture(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)
	earlier, later := now.Add(time.Hour), now.Add(2*time.Hour)
	earliestFile := fixture.addFile(t, "job-1", fixture.ownerID, "image", "earliest.jpg", []byte("early"), &earlier)
	fixture.addFile(t, "job-1", fixture.ownerID, "image", "later.jpg", []byte("later"), &later)
	temporaryExpiry := now.Add(30 * time.Minute)
	fixture.addFile(t, "job-1", fixture.ownerID, "asr_source", "source.mp3", []byte("source"), &temporaryExpiry)
	removedEarly := fixture.addFile(t, "job-1", fixture.ownerID, "image", "removed.jpg", []byte("removed"), &temporaryExpiry)
	if err := fixture.deps.Files.MarkDeleted(ctx, removedEarly.ID, now); err != nil {
		t.Fatal(err)
	}
	work, err := resolver.NewSQLiteCache(fixture.db).Save(ctx, resolver.Work{
		DouyinWorkID: "123", Type: "note", CanonicalURL: "https://www.douyin.com/note/123",
		Title: "Gallery summary", AuthorName: "Artist", DurationMS: 1200, Hashtags: []string{"gallery"},
		CoverURL: "https://example.com/summary-cover.jpg",
		Images:   []resolver.Image{{URL: "https://example.com/original.jpg"}}, ResolvedAt: now,
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
	if summary.Work == nil || summary.Work.Title != "Gallery summary" || summary.Work.AuthorName != "Artist" || summary.Work.DurationMS != 1200 || !slices.Equal(summary.Work.Hashtags, []string{"gallery"}) ||
		summary.Work.Type != "note" || summary.Work.CoverURL != work.CoverURL {
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

	// Retention cleanup leaves a soft-deleted file. Both detail and list
	// responses must still report the earliest media deadline.
	if err := fixture.deps.Files.MarkDeleted(ctx, earliestFile.ID, earlier); err != nil {
		t.Fatal(err)
	}
	for _, check := range []struct{ path, token string }{
		{"/api/v1/jobs/job-1", fixture.ownerToken},
		{"/api/v1/jobs?q=Gallery", fixture.ownerToken},
		{frontPath, fixture.ownerToken},
		{"/api/v1/admin/jobs/job-1", credentials.AccessToken},
		{"/api/v1/admin/jobs?userId=" + url.QueryEscape(fixture.ownerID), credentials.AccessToken},
		{"/api/v1/admin/jobs?view=summary&userId=" + url.QueryEscape(fixture.ownerID), credentials.AccessToken},
	} {
		response := performJSON(fixture.handler, http.MethodGet, check.path, nil, check.token)
		var payload struct {
			Job  jobs.JobSummary   `json:"job"`
			Jobs []jobs.JobSummary `json:"jobs"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil || response.Code != http.StatusOK {
			t.Fatalf("%s: status %d, decode error %v", check.path, response.Code, err)
		}
		if len(payload.Jobs) == 1 {
			payload.Job = payload.Jobs[0]
		}
		if payload.Job.ID != "job-1" || payload.Job.MediaExpiresAt == nil || !payload.Job.MediaExpiresAt.Equal(earlier) {
			t.Fatalf("%s: expiry did not survive cleanup: %+v", check.path, payload.Job)
		}
		if strings.Contains(response.Body.String(), earliestFile.ID) {
			t.Fatalf("%s: soft-deleted file should not be offered", check.path)
		}
	}
}

func TestJobSummaryCoverUsesOwnedUnexpiredMedia(t *testing.T) {
	fixture := newArchiveDownloadFixture(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)
	for _, file := range fixture.media {
		if err := fixture.deps.Files.MarkDeleted(ctx, file.ID, now); err != nil {
			t.Fatal(err)
		}
	}
	fixture.addFile(t, "job-1", fixture.otherID, "image", "wrong-owner.jpg", []byte("private"), nil)
	expired := now.Add(-time.Minute)
	fixture.addFile(t, "job-1", fixture.ownerID, "image", "expired.jpg", []byte("expired"), &expired)
	removed := fixture.addFile(t, "job-1", fixture.ownerID, "image", "deleted.jpg", []byte("deleted"), nil)
	if err := fixture.deps.Files.MarkDeleted(ctx, removed.ID, now); err != nil {
		t.Fatal(err)
	}
	deadline := now.Add(time.Hour)
	cover := fixture.addFile(t, "job-1", fixture.ownerID, "animated", "current.mp4", []byte("current"), &deadline)
	fixture.addFile(t, "job-1", fixture.ownerID, "image", "later.jpg", []byte("later"), nil)
	work, err := resolver.NewSQLiteCache(fixture.db).Save(ctx, resolver.Work{
		DouyinWorkID: "summary-cover", Type: "note", CanonicalURL: "https://www.douyin.com/note/123",
		Title: "Summary cover", CoverURL: "https://example.test/cover.jpg", ResolvedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.db.ExecContext(ctx, `UPDATE jobs SET work_id = ? WHERE id = ?`, work.ID, "job-1"); err != nil {
		t.Fatal(err)
	}
	page, err := fixture.repo.ListSummaries(ctx, jobs.ListInput{UserID: fixture.ownerID, Query: "Summary cover", Limit: 10}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].Work == nil {
		t.Fatalf("summary page = %+v", page)
	}
	summary := page.Items[0].Work
	if summary.Type != "note" || summary.CoverURL != work.CoverURL || summary.CoverFileID != cover.ID ||
		summary.CoverExpiresAt == nil || !summary.CoverExpiresAt.Equal(deadline) {
		t.Fatalf("summary cover = %+v", summary)
	}

	fixture.deps.Thumbnails = media.NewThumbnailer(filepath.Join(t.TempDir(), "unused-ffmpeg"))
	fixture.handler = New(fixture.deps)
	response := performJSON(fixture.handler, http.MethodGet, "/api/v1/jobs?view=summary&q=Summary+cover", nil, fixture.ownerToken)
	var payload struct {
		Jobs           []jobs.JobSummary `json:"jobs"`
		SummaryVersion int               `json:"summaryVersion"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil || response.Code != http.StatusOK || len(payload.Jobs) != 1 {
		t.Fatalf("summary response: status=%d error=%v body=%s", response.Code, err, response.Body.String())
	}
	if payload.SummaryVersion != 2 || payload.Jobs[0].Work == nil {
		t.Fatalf("summary capabilities = %+v", payload)
	}
	preview, err := url.Parse(payload.Jobs[0].Work.CoverPreviewURL)
	if err != nil || preview.Path != "/api/v1/media-preview/"+cover.ID+"/thumbnail" ||
		!fixture.deps.ASRSigner.ValidateThumbnail(cover.ID, preview.Query().Get("expires"), preview.Query().Get("signature"), now) {
		t.Fatalf("summary cover preview = %q, error=%v", payload.Jobs[0].Work.CoverPreviewURL, err)
	}
	for _, field := range []string{`"coverFileId"`, `"coverExpiresAt"`, `"result"`, `"files"`, `"images"`} {
		if strings.Contains(response.Body.String(), field) {
			t.Fatalf("summary exposes internal or full-detail field %s", field)
		}
	}

	if _, err := fixture.db.ExecContext(ctx, `UPDATE jobs SET status = 'queued' WHERE id = ?`, "job-1"); err != nil {
		t.Fatal(err)
	}
	queued, err := fixture.repo.ListSummaries(ctx, jobs.ListInput{UserID: fixture.ownerID, Query: "Summary cover", Limit: 10}, false)
	if err != nil || len(queued.Items) != 1 || queued.Items[0].Work.CoverFileID != "" {
		t.Fatalf("unfinished task must not expose stale cover file: page=%+v err=%v", queued, err)
	}
}
