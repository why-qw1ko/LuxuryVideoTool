package httpapi

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/auth"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/database"
	ownedfiles "github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/files"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/jobs"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/resolver"
)

type archiveDownloadFixture struct {
	deps                   Dependencies
	handler                http.Handler
	db                     *sql.DB
	repo                   *jobs.SQLiteRepository
	ownerID, otherID       string
	ownerToken, otherToken string
	media                  []ownedfiles.File
	nextFileTime           time.Time
}

func newArchiveDownloadFixture(t *testing.T) *archiveDownloadFixture {
	t.Helper()
	ctx := context.Background()
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "archive-download.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	key := []byte("01234567890123456789012345678901")
	tokens, err := auth.NewTokenManager(key, 15*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	service, err := auth.NewService(auth.NewSQLiteRepository(db), tokens, 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := service.CreateUser(ctx, "owner", "Owner", "OwnerPass!", auth.RoleUser)
	if err != nil {
		t.Fatal(err)
	}
	other, err := service.CreateUser(ctx, "other", "Other", "OwnerPass!", auth.RoleUser)
	if err != nil {
		t.Fatal(err)
	}
	storage, err := ownedfiles.NewStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repo := jobs.NewSQLiteRepository(db)
	fixture := &archiveDownloadFixture{
		deps: Dependencies{
			Auth: service, Jobs: jobs.NewService(repo, nil), Files: ownedfiles.NewRepository(db),
			Storage: storage, ASRSigner: ownedfiles.NewSigner(key, ""),
		},
		db: db, repo: repo, ownerID: owner.ID, otherID: other.ID, nextFileTime: time.Now().UTC(),
	}
	fixture.handler = New(fixture.deps)
	for username, target := range map[string]*string{"owner": &fixture.ownerToken, "other": &fixture.otherToken} {
		login := performJSON(fixture.handler, http.MethodPost, "/api/v1/auth/login", map[string]any{
			"username": username, "password": "OwnerPass!",
			"device": map[string]any{"id": "device", "name": "PC", "platform": "windows", "appVersion": "1"},
		}, "")
		if login.Code != http.StatusOK {
			t.Fatalf("login status=%d body=%s", login.Code, login.Body.String())
		}
		var pair tokenResponse
		if err := json.Unmarshal(login.Body.Bytes(), &pair); err != nil {
			t.Fatal(err)
		}
		*target = pair.AccessToken
	}
	fixture.addJob(t, "job-1", owner.ID, "completed")
	fixture.media = []ownedfiles.File{
		fixture.addFile(t, "job-1", owner.ID, "image", "image-01.jpg", []byte("first-image"), nil),
		fixture.addFile(t, "job-1", owner.ID, "animated", "image-02.mp4", []byte("animated-clip"), nil),
	}
	fixture.addFile(t, "job-1", owner.ID, "music", "music.mp3", []byte("excluded-music"), nil)
	fixture.addJob(t, "other-job", other.ID, "completed")
	fixture.addFile(t, "other-job", other.ID, "image", "private.jpg", []byte("other-user-image"), nil)
	return fixture
}

func (f *archiveDownloadFixture) addJob(t *testing.T, jobID, userID, status string) {
	t.Helper()
	now := time.Now().UTC()
	job := jobs.Job{
		ID: jobID, UserID: userID, InputText: "https://www.douyin.com/note/123", InputURL: "https://www.douyin.com/note/123",
		Action: "download", Status: status, IdempotencyKey: jobID, MaxAttempts: 3, CreatedAt: now, UpdatedAt: now,
	}
	if status == "completed" {
		job.CompletedAt = &now
	}
	if err := f.repo.CreateQueued(context.Background(), job); err != nil {
		t.Fatal(err)
	}
}

func (f *archiveDownloadFixture) addFile(t *testing.T, jobID, userID, kind, name string, body []byte, expires *time.Time) ownedfiles.File {
	t.Helper()
	relative, temporary, final, err := f.deps.Storage.NewTarget(userID, jobID, filepath.Ext(name))
	if err != nil {
		t.Fatal(err)
	}
	if err := f.deps.Storage.WriteAtomic(temporary, final, body); err != nil {
		t.Fatal(err)
	}
	file, err := ownedfiles.NewFile(f.nextFileTime, userID, jobID, kind, relative, name, "application/octet-stream", "sha", int64(len(body)), expires)
	f.nextFileTime = f.nextFileTime.Add(time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.deps.Files.Create(context.Background(), file); err != nil {
		t.Fatal(err)
	}
	return file
}

func archiveDownloadLink(t *testing.T, f *archiveDownloadFixture) (string, time.Time) {
	t.Helper()
	response := performJSON(f.handler, http.MethodPost, "/api/v1/jobs/job-1/images/archive-download", nil, f.ownerToken)
	if response.Code != http.StatusOK {
		t.Fatalf("create archive URL status=%d body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Header().Get("Cache-Control"), "no-store") {
		t.Fatalf("signed URL cache-control=%q", response.Header().Get("Cache-Control"))
	}
	var payload struct {
		DownloadURL string    `json:"downloadUrl"`
		ExpiresAt   time.Time `json:"expiresAt"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	return payload.DownloadURL, payload.ExpiresAt
}

func archiveTempDirectory(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(name, dir)
	}
	return dir
}

func assertArchiveTempEmpty(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("archive temporary files leaked: %v", entries)
	}
}

func assertArchiveBody(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "application/zip" {
		t.Fatalf("archive status=%d content-type=%q body=%s", response.Code, response.Header().Get("Content-Type"), response.Body.String())
	}
	if response.Header().Get("Content-Length") != strconv.Itoa(response.Body.Len()) || response.Header().Get("Accept-Ranges") != "bytes" {
		t.Fatalf("archive length/range headers=%v body bytes=%d", response.Header(), response.Body.Len())
	}
	if !strings.Contains(response.Header().Get("Cache-Control"), "no-store") || !strings.HasPrefix(response.Header().Get("Content-Disposition"), "attachment;") {
		t.Fatalf("archive response headers=%v", response.Header())
	}
	reader, err := zip.NewReader(bytes.NewReader(response.Body.Bytes()), int64(response.Body.Len()))
	if err != nil {
		t.Fatalf("open archive: %v", err)
	}
	want := []struct{ name, body string }{{"01_image-01.jpg", "first-image"}, {"02_image-02.mp4", "animated-clip"}}
	if len(reader.File) != len(want) {
		t.Fatalf("archive entries=%d, want %d", len(reader.File), len(want))
	}
	for i, entry := range reader.File {
		if entry.Name != want[i].name || entry.Method != zip.Deflate {
			t.Fatalf("archive entry %d name=%q method=%d", i, entry.Name, entry.Method)
		}
		handle, err := entry.Open()
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(handle)
		_ = handle.Close()
		if err != nil || string(body) != want[i].body {
			t.Fatalf("archive entry %d body=%q error=%v", i, body, err)
		}
	}
}

func TestArchiveDownloadSignedHTTP(t *testing.T) {
	f := newArchiveDownloadFixture(t)
	temporary := archiveTempDirectory(t)
	before := time.Now().UTC()
	downloadURL, expires := archiveDownloadLink(t, f)
	parsed, err := url.Parse(downloadURL)
	if err != nil || parsed.IsAbs() || parsed.Host != "" || parsed.Path != "/api/v1/archive-download/job-1" {
		t.Fatalf("download URL must be same-origin relative: %q error=%v", downloadURL, err)
	}
	if parsed.Query().Get("user") != f.ownerID || parsed.Query().Get("expires") != strconv.FormatInt(expires.Unix(), 10) {
		t.Fatalf("download query=%v expiry=%v", parsed.Query(), expires)
	}
	if remaining := expires.Sub(before); remaining < archiveDownloadTTL-2*time.Second || remaining > archiveDownloadTTL {
		t.Fatalf("download lifetime=%v", remaining)
	}
	response := performJSON(f.handler, http.MethodGet, downloadURL, nil, "")
	assertArchiveBody(t, response)
	request := httptest.NewRequest(http.MethodGet, downloadURL, nil)
	request.Header.Set("Range", "bytes=0-3")
	ranged := httptest.NewRecorder()
	f.handler.ServeHTTP(ranged, request)
	if ranged.Code != http.StatusPartialContent || ranged.Body.String() != "PK\x03\x04" || ranged.Header().Get("Content-Length") != "4" || ranged.Header().Get("Content-Range") != "bytes 0-3/"+strconv.Itoa(response.Body.Len()) {
		t.Fatalf("range status=%d headers=%v body=%q", ranged.Code, ranged.Header(), ranged.Body.String())
	}
	head := performJSON(f.handler, http.MethodHead, downloadURL, nil, "")
	if head.Code != http.StatusOK || head.Body.Len() != 0 || head.Header().Get("Content-Length") != response.Header().Get("Content-Length") {
		t.Fatalf("HEAD status=%d headers=%v body bytes=%d", head.Code, head.Header(), head.Body.Len())
	}
	legacy := performJSON(f.handler, http.MethodGet, "/api/v1/jobs/job-1/images/archive", nil, f.ownerToken)
	assertArchiveBody(t, legacy)
	assertArchiveTempEmpty(t, temporary)
}

func TestArchiveDownloadAdminCanPackageAnotherUsersJob(t *testing.T) {
	f := newArchiveDownloadFixture(t)
	ctx := context.Background()
	const password = "AdminPass!"
	if _, err := f.deps.Auth.CreateUser(ctx, "admin", "Admin", password, auth.RoleAdmin); err != nil {
		t.Fatal(err)
	}
	admin, err := f.deps.Auth.Login(ctx, auth.LoginInput{
		Username: "admin", Password: password,
		Device: auth.Device{ID: "admin-device", Name: "PC", Platform: "windows", AppVersion: "1"},
	})
	if err != nil {
		t.Fatal(err)
	}

	const adminPath = "/api/v1/admin/jobs/job-1/images/archive-download"
	for _, tc := range []struct {
		path, token string
		status      int
	}{
		{adminPath, "", http.StatusUnauthorized},
		{adminPath, f.otherToken, http.StatusForbidden},
		{"/api/v1/jobs/job-1/images/archive-download", admin.AccessToken, http.StatusNotFound},
		{"/api/v1/admin/jobs/missing-job/images/archive-download", admin.AccessToken, http.StatusNotFound},
	} {
		response := performJSON(f.handler, http.MethodPost, tc.path, nil, tc.token)
		if response.Code != tc.status {
			t.Fatalf("%s: got %d, want %d", tc.path, response.Code, tc.status)
		}
	}

	response := performJSON(f.handler, http.MethodPost, adminPath, nil, admin.AccessToken)
	if response.Code != http.StatusOK {
		t.Fatalf("create admin archive URL: %d %s", response.Code, response.Body.String())
	}
	var payload struct {
		DownloadURL string `json:"downloadUrl"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	download, err := url.Parse(payload.DownloadURL)
	if err != nil || download.Query().Get("user") != f.ownerID {
		t.Fatalf("archive URL must identify the job owner: %q, error=%v", payload.DownloadURL, err)
	}
	assertArchiveBody(t, performJSON(f.handler, http.MethodGet, payload.DownloadURL, nil, ""))
}

func TestArchiveDownloadAuthorizationAndSignatures(t *testing.T) {
	f := newArchiveDownloadFixture(t)
	downloadURL, _ := archiveDownloadLink(t, f)
	future := time.Now().Add(time.Minute)
	mutateQuery := func(key, value string) string {
		parsed, err := url.Parse(downloadURL)
		if err != nil {
			t.Fatal(err)
		}
		query := parsed.Query()
		query.Set(key, value)
		parsed.RawQuery = query.Encode()
		return parsed.String()
	}
	for name, target := range map[string]string{
		"tampered user":      mutateQuery("user", f.otherID),
		"tampered job":       strings.Replace(downloadURL, "/job-1?", "/other-job?", 1),
		"tampered expiry":    mutateQuery("expires", strconv.FormatInt(future.Add(time.Hour).Unix(), 10)),
		"tampered signature": mutateQuery("signature", strings.Repeat("0", 64)),
		"missing user":       mutateQuery("user", ""),
		"missing expiry":     mutateQuery("expires", ""),
		"missing signature":  mutateQuery("signature", ""),
		"expired":            f.deps.ASRSigner.ArchiveURL(f.ownerID, "job-1", time.Now().Add(-time.Minute)),
		"wrong owner":        f.deps.ASRSigner.ArchiveURL(f.otherID, "job-1", future),
		"missing job":        f.deps.ASRSigner.ArchiveURL(f.ownerID, "missing-job", future),
	} {
		t.Run(name, func(t *testing.T) {
			response := performJSON(f.handler, http.MethodGet, target, nil, "")
			if response.Code != http.StatusNotFound || !strings.Contains(response.Header().Get("Cache-Control"), "no-store") {
				t.Fatalf("signed archive status=%d headers=%v body=%s", response.Code, response.Header(), response.Body.String())
			}
		})
	}
	for name, source := range map[string]string{"preview": f.deps.ASRSigner.PreviewURL("job-1", future), "asr": f.deps.ASRSigner.URL("job-1", future)} {
		t.Run("cross purpose "+name, func(t *testing.T) {
			parsed, err := url.Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			query := parsed.Query()
			query.Set("user", f.ownerID)
			response := performJSON(f.handler, http.MethodGet, "/api/v1/archive-download/job-1?"+query.Encode(), nil, "")
			if response.Code != http.StatusNotFound {
				t.Fatalf("cross-purpose signature status=%d", response.Code)
			}
		})
	}
	for name, token := range map[string]string{"unauthenticated": "", "other user": f.otherToken} {
		t.Run("create "+name, func(t *testing.T) {
			response := performJSON(f.handler, http.MethodPost, "/api/v1/jobs/job-1/images/archive-download", nil, token)
			want := http.StatusNotFound
			if token == "" {
				want = http.StatusUnauthorized
			}
			if response.Code != want || strings.Contains(response.Body.String(), "downloadUrl") {
				t.Fatalf("create status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
	legacy := performJSON(f.handler, http.MethodGet, "/api/v1/jobs/job-1/images/archive", nil, f.otherToken)
	if legacy.Code != http.StatusNotFound {
		t.Fatalf("other user's legacy archive status=%d", legacy.Code)
	}
	deps := f.deps
	deps.ASRSigner = nil
	withoutSigner := New(deps)
	for _, response := range []*httptest.ResponseRecorder{
		performJSON(withoutSigner, http.MethodPost, "/api/v1/jobs/job-1/images/archive-download", nil, f.ownerToken),
		performJSON(withoutSigner, http.MethodGet, downloadURL, nil, ""),
	} {
		if response.Code != http.StatusServiceUnavailable {
			t.Fatalf("missing signer status=%d body=%s", response.Code, response.Body.String())
		}
	}
}

func TestArchiveDownloadRechecksJobAndMedia(t *testing.T) {
	f := newArchiveDownloadFixture(t)
	downloadURL, _ := archiveDownloadLink(t, f)
	if _, err := f.db.Exec("UPDATE jobs SET status = 'queued' WHERE id = 'job-1'"); err != nil {
		t.Fatal(err)
	}
	if response := performJSON(f.handler, http.MethodGet, downloadURL, nil, ""); response.Code != http.StatusConflict {
		t.Fatalf("changed job status=%d body=%s", response.Code, response.Body.String())
	}
	if _, err := f.db.Exec("UPDATE jobs SET status = 'completed', user_id = ? WHERE id = 'job-1'", f.otherID); err != nil {
		t.Fatal(err)
	}
	if response := performJSON(f.handler, http.MethodGet, downloadURL, nil, ""); response.Code != http.StatusNotFound {
		t.Fatalf("changed owner status=%d body=%s", response.Code, response.Body.String())
	}
	past := time.Now().Add(-time.Minute)
	cases := []struct {
		id, status, kind string
		expires          *time.Time
		want             int
	}{
		{"unfinished", "queued", "image", nil, http.StatusConflict},
		{"empty", "completed", "", nil, http.StatusNotFound},
		{"music-only", "completed", "music", nil, http.StatusNotFound},
		{"expired-media", "completed", "image", &past, http.StatusNotFound},
		{"deleted-media", "completed", "image", nil, http.StatusNotFound},
		{"foreign-media", "completed", "image", nil, http.StatusNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			f.addJob(t, tc.id, f.ownerID, tc.status)
			if tc.kind != "" {
				userID := f.ownerID
				if tc.id == "foreign-media" {
					userID = f.otherID
				}
				file := f.addFile(t, tc.id, userID, tc.kind, "media.jpg", []byte("media"), tc.expires)
				if tc.id == "deleted-media" {
					if err := f.deps.Files.MarkDeleted(context.Background(), file.ID, time.Now()); err != nil {
						t.Fatal(err)
					}
				}
			}
			target := f.deps.ASRSigner.ArchiveURL(f.ownerID, tc.id, time.Now().Add(time.Minute))
			for _, response := range []*httptest.ResponseRecorder{
				performJSON(f.handler, http.MethodPost, "/api/v1/jobs/"+tc.id+"/images/archive-download", nil, f.ownerToken),
				performJSON(f.handler, http.MethodGet, target, nil, ""),
				performJSON(f.handler, http.MethodGet, "/api/v1/jobs/"+tc.id+"/images/archive", nil, f.ownerToken),
			} {
				if response.Code != tc.want {
					t.Fatalf("archive status=%d want=%d body=%s", response.Code, tc.want, response.Body.String())
				}
			}
		})
	}
}

func TestArchiveDownloadMissingSourceFailsBeforeResponse(t *testing.T) {
	f := newArchiveDownloadFixture(t)
	downloadURL, _ := archiveDownloadLink(t, f)
	temporary := archiveTempDirectory(t)
	if err := f.deps.Storage.Remove(f.media[1]); err != nil {
		t.Fatal(err)
	}
	for _, response := range []*httptest.ResponseRecorder{
		performJSON(f.handler, http.MethodPost, "/api/v1/jobs/job-1/images/archive-download", nil, f.ownerToken),
		performJSON(f.handler, http.MethodGet, downloadURL, nil, ""),
		performJSON(f.handler, http.MethodGet, "/api/v1/jobs/job-1/images/archive", nil, f.ownerToken),
	} {
		if response.Code != http.StatusNotFound || response.Header().Get("Content-Type") == "application/zip" || bytes.HasPrefix(response.Body.Bytes(), []byte("PK")) {
			t.Fatalf("missing source status=%d headers=%v body=%s", response.Code, response.Header(), response.Body.String())
		}
	}
	assertArchiveTempEmpty(t, temporary)
}

func TestArchiveDownloadMatchesSavedNoteMedia(t *testing.T) {
	f := newArchiveDownloadFixture(t)
	setImages := func(images []resolver.Image) {
		t.Helper()
		work, err := resolver.NewSQLiteCache(f.db).Save(context.Background(), resolver.Work{
			DouyinWorkID: "123", Type: "note", CanonicalURL: "https://www.douyin.com/note/123",
			Images: images, ResolverName: "test", ResolverVersion: resolver.DouyinResolverVersion,
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.db.Exec("UPDATE jobs SET work_id = ? WHERE id = 'job-1'", work.ID); err != nil {
			t.Fatal(err)
		}
	}
	setImages([]resolver.Image{
		{URL: "https://example.test/first.jpg"},
		{URL: "https://example.test/poster.jpg", AnimatedURL: "https://example.test/live.mp4"},
	})
	if err := f.deps.Files.MarkDeleted(context.Background(), f.media[1].ID, time.Now()); err != nil {
		t.Fatal(err)
	}
	f.addFile(t, "job-1", f.ownerID, "image", "image-02.jpg", []byte("old-static-cover"), nil)
	f.addFile(t, "job-1", f.ownerID, "image", "image-01.webp", []byte("duplicate-first-image"), nil)
	downloadURL, _ := archiveDownloadLink(t, f)
	for _, response := range []*httptest.ResponseRecorder{
		performJSON(f.handler, http.MethodGet, downloadURL, nil, ""),
		performJSON(f.handler, http.MethodGet, "/api/v1/jobs/job-1/images/archive", nil, f.ownerToken),
	} {
		if response.Code != http.StatusOK {
			t.Fatalf("partial note archive status=%d body=%s", response.Code, response.Body.String())
		}
		reader, err := zip.NewReader(bytes.NewReader(response.Body.Bytes()), int64(response.Body.Len()))
		if err != nil {
			t.Fatal(err)
		}
		if len(reader.File) != 1 || reader.File[0].Name != "01_image-01.jpg" {
			t.Fatalf("partial note archive must omit old static and duplicate media: %#v", reader.File)
		}
	}
	// 文件时间刻意与作品顺序相反，ZIP 仍按作品项顺序，只保留首个正确类型的文件。
	f.nextFileTime = f.media[0].CreatedAt.Add(-time.Second)
	f.addFile(t, "job-1", f.ownerID, "animated", "image-02.mp4", []byte("animated-clip"), nil)
	f.addFile(t, "job-1", f.ownerID, "animated", "image-02.mp4", []byte("duplicate-live-clip"), nil)
	_, _ = archiveDownloadLink(t, f)
	for _, response := range []*httptest.ResponseRecorder{
		performJSON(f.handler, http.MethodGet, downloadURL, nil, ""),
		performJSON(f.handler, http.MethodGet, "/api/v1/jobs/job-1/images/archive", nil, f.ownerToken),
	} {
		assertArchiveBody(t, response)
	}
	// 没有结构化配图的历史任务仍兼容原来的全部有效媒体文件。
	setImages(nil)
	_, _ = archiveDownloadLink(t, f)
	response := performJSON(f.handler, http.MethodGet, downloadURL, nil, "")
	if response.Code != http.StatusOK {
		t.Fatalf("legacy note archive status=%d body=%s", response.Code, response.Body.String())
	}
	reader, err := zip.NewReader(bytes.NewReader(response.Body.Bytes()), int64(response.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	if len(reader.File) != 5 {
		t.Fatalf("legacy note archive entries=%d, want all 5 valid media files", len(reader.File))
	}
}

func TestArchiveDownloadImageNumberBoundary(t *testing.T) {
	f := newArchiveDownloadFixture(t)
	images := make([]resolver.Image, 100)
	for i := range images {
		images[i].URL = "https://example.test/still.jpg"
	}
	work, err := resolver.NewSQLiteCache(f.db).Save(context.Background(), resolver.Work{
		DouyinWorkID: "123", Type: "note", CanonicalURL: "https://www.douyin.com/note/123", Images: images,
		ResolverName: "test", ResolverVersion: resolver.DouyinResolverVersion,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec("UPDATE jobs SET work_id = ? WHERE id = 'job-1'", work.ID); err != nil {
		t.Fatal(err)
	}
	for _, file := range f.media {
		if err := f.deps.Files.MarkDeleted(context.Background(), file.ID, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	f.addFile(t, "job-1", f.ownerID, "image", "image-100.jpg", []byte("hundredth-image"), nil)
	downloadURL, _ := archiveDownloadLink(t, f)
	for _, response := range []*httptest.ResponseRecorder{
		performJSON(f.handler, http.MethodGet, downloadURL, nil, ""),
		performJSON(f.handler, http.MethodGet, "/api/v1/jobs/job-1/images/archive", nil, f.ownerToken),
	} {
		if response.Code != http.StatusOK {
			t.Fatalf("archive status=%d body=%s", response.Code, response.Body.String())
		}
		reader, err := zip.NewReader(bytes.NewReader(response.Body.Bytes()), int64(response.Body.Len()))
		if err != nil {
			t.Fatal(err)
		}
		if len(reader.File) != 1 || reader.File[0].Name != "01_image-100.jpg" {
			t.Fatalf("image-100 must match only the hundredth item once: %#v", reader.File)
		}
		handle, err := reader.File[0].Open()
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(handle)
		_ = handle.Close()
		if err != nil || string(body) != "hundredth-image" {
			t.Fatalf("hundredth image body=%q error=%v", body, err)
		}
	}
}

type archiveCancellingStorage struct {
	storage *ownedfiles.Storage
	cancel  context.CancelFunc
}

func (s archiveCancellingStorage) Open(file ownedfiles.File) (*os.File, error) {
	handle, err := s.storage.Open(file)
	s.cancel()
	return handle, err
}

func TestBuildImageArchiveCancellationAndCleanup(t *testing.T) {
	storage, err := ownedfiles.NewStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	relative, temporary, final, err := storage.NewTarget("owner", "job-1", ".jpg")
	if err != nil {
		t.Fatal(err)
	}
	if err := storage.WriteAtomic(temporary, final, bytes.Repeat([]byte("image-bytes"), 10000)); err != nil {
		t.Fatal(err)
	}
	media := []ownedfiles.File{{RelativePath: relative, OriginalName: "image.jpg"}}
	dir := archiveTempDirectory(t)
	for _, duringBuild := range []bool{false, true} {
		ctx, cancel := context.WithCancel(context.Background())
		var source archiveStorage = storage
		if duringBuild {
			source = archiveCancellingStorage{storage: storage, cancel: cancel}
		} else {
			cancel()
		}
		archive, err := buildImageArchive(ctx, source, media)
		cancel()
		if archive != nil || !errors.Is(err, context.Canceled) {
			if archive != nil {
				removeImageArchive(archive)
			}
			t.Fatalf("cancel during build=%v archive=%v error=%v", duringBuild, archive, err)
		}
		assertArchiveTempEmpty(t, dir)
	}
}

type archiveCancelReader struct {
	reader io.Reader
	cancel context.CancelFunc
}

func (r archiveCancelReader) Read(body []byte) (int, error) {
	n, err := r.reader.Read(body)
	r.cancel()
	return n, err
}

func TestArchiveContextReaderStopsWhenCancelledDuringRead(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	source := archiveCancelReader{reader: bytes.NewReader(bytes.Repeat([]byte("image"), 10000)), cancel: cancel}
	n, err := io.Copy(io.Discard, archiveContextReader{ctx: ctx, reader: source})
	if n != 0 || !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled read copied=%d error=%v", n, err)
	}
}
