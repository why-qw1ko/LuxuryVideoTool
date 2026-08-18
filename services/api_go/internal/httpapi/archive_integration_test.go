package httpapi

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/auth"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/database"
	ownedfiles "github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/files"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/jobs"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/version"
)

// 图文配图打包下载：/api/v1/jobs/{id}/images/archive 返回合法的 ZIP。
func TestNoteImageArchiveHTTP(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "archive.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	tokens, err := auth.NewTokenManager([]byte("01234567890123456789012345678901"), 15*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	authService, err := auth.NewService(auth.NewSQLiteRepository(db), tokens, 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	user, err := authService.CreateUser(ctx, "owner", "Owner", "correct horse battery staple", auth.RoleUser)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	jobRepo := jobs.NewSQLiteRepository(db)
	job := jobs.Job{ID: "job-1", UserID: user.ID, InputText: "https://www.douyin.com/note/123", InputURL: "https://www.douyin.com/note/123", Action: "download", Status: "completed", IdempotencyKey: "k", MaxAttempts: 3, CreatedAt: now, UpdatedAt: now, CompletedAt: &now}
	if err := jobRepo.CreateQueued(ctx, job); err != nil {
		t.Fatal(err)
	}

	fileRepo := ownedfiles.NewRepository(db)
	storage, err := ownedfiles.NewStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	payload := []struct {
		name, kind, mime string
		body             []byte
	}{
		{"image-01.jpg", "image", "image/jpeg", []byte("first-image")},
		{"image-02.mp4", "animated", "video/mp4", []byte("animated-clip")},
	}
	for i, p := range payload {
		relative, temporary, final, err := storage.NewTarget(user.ID, job.ID, filepath.Ext(p.name))
		if err != nil {
			t.Fatal(err)
		}
		if err := storage.WriteAtomic(temporary, final, p.body); err != nil {
			t.Fatal(err)
		}
		// 每张图递增时间戳，确保 ListByJob 按图片顺序返回。
		file, err := ownedfiles.NewFile(now.Add(time.Duration(i)*time.Millisecond), user.ID, job.ID, p.kind, relative, p.name, p.mime, "sha", int64(len(p.body)), nil)
		if err != nil {
			t.Fatal(err)
		}
		if err := fileRepo.Create(ctx, file); err != nil {
			t.Fatal(err)
		}
	}

	handler := New(Dependencies{Build: version.Info{Version: "test"}, Auth: authService, Jobs: jobs.NewService(jobRepo, nil), Files: fileRepo, Storage: storage, LoginRateLimit: 5, Ready: func() error { return nil }})
	login := performJSON(handler, http.MethodPost, "/api/v1/auth/login", map[string]any{"username": "owner", "password": "correct horse battery staple", "device": map[string]any{"id": "device", "name": "PC", "platform": "windows", "appVersion": "1"}}, "")
	var pair tokenResponse
	if err := json.Unmarshal(login.Body.Bytes(), &pair); err != nil {
		t.Fatal(err)
	}

	archive := performJSON(handler, http.MethodGet, "/api/v1/jobs/job-1/images/archive", nil, pair.AccessToken)
	if archive.Code != http.StatusOK {
		t.Fatalf("archive status = %d, body = %s", archive.Code, archive.Body.String())
	}
	if ct := archive.Header().Get("Content-Type"); ct != "application/zip" {
		t.Fatalf("content-type = %q", ct)
	}
	body := archive.Body.Bytes()
	if !bytes.HasPrefix(body, []byte("PK\x03\x04")) {
		t.Fatalf("not a zip: prefix = %q", body[:min(4, len(body))])
	}
	reader, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	if len(reader.File) != 2 {
		t.Fatalf("zip entries = %d, want 2", len(reader.File))
	}
	names := map[string]bool{}
	for _, f := range reader.File {
		names[f.Name] = true
	}
	if !names["01_image-01.jpg"] || !names["02_image-02.mp4"] {
		t.Fatalf("zip names = %v", names)
	}

	// 无配图的任务应返回 404
	empty := performJSON(handler, http.MethodGet, "/api/v1/jobs/does-not-exist/images/archive", nil, pair.AccessToken)
	if empty.Code != http.StatusNotFound {
		t.Fatalf("missing job archive status = %d", empty.Code)
	}
}

// 媒体预览：job 响应携带 previewUrl，且签名地址可无 Authorization 直接加载。
func TestMediaPreviewSigned(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "preview.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	tokens, _ := auth.NewTokenManager([]byte("01234567890123456789012345678901"), 15*time.Minute)
	authService, _ := auth.NewService(auth.NewSQLiteRepository(db), tokens, 24*time.Hour)
	user, err := authService.CreateUser(ctx, "owner", "Owner", "correct horse battery staple", auth.RoleUser)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	jobRepo := jobs.NewSQLiteRepository(db)
	if err := jobRepo.CreateQueued(ctx, jobs.Job{ID: "job-p", UserID: user.ID, InputText: "https://www.douyin.com/note/9", InputURL: "https://www.douyin.com/note/9", Action: "download", Status: "completed", IdempotencyKey: "k", MaxAttempts: 3, CreatedAt: now, UpdatedAt: now, CompletedAt: &now}); err != nil {
		t.Fatal(err)
	}
	fileRepo := ownedfiles.NewRepository(db)
	storage, _ := ownedfiles.NewStorage(t.TempDir())
	relative, temporary, final, _ := storage.NewTarget(user.ID, "job-p", ".jpg")
	if err := storage.WriteAtomic(temporary, final, []byte("preview-image-bytes")); err != nil {
		t.Fatal(err)
	}
	file, err := ownedfiles.NewFile(now, user.ID, "job-p", "image", relative, "image-01.jpg", "image/jpeg", "sha", 19, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := fileRepo.Create(ctx, file); err != nil {
		t.Fatal(err)
	}
	musicRelative, musicTemporary, musicFinal, _ := storage.NewScopedTarget("music", user.ID, "job-p", ".mp3")
	if err := storage.WriteAtomic(musicTemporary, musicFinal, []byte("preview-music-bytes")); err != nil {
		t.Fatal(err)
	}
	music, err := ownedfiles.NewFile(now.Add(time.Millisecond), user.ID, "job-p", "music", musicRelative, "music.mp3", "audio/mpeg", "sha-music", 19, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := fileRepo.Create(ctx, music); err != nil {
		t.Fatal(err)
	}

	signer := ownedfiles.NewSigner([]byte("01234567890123456789012345678901"), "")
	handler := New(Dependencies{Build: version.Info{Version: "test"}, Auth: authService, Jobs: jobs.NewService(jobRepo, nil), Files: fileRepo, Storage: storage, ASRSigner: signer, LoginRateLimit: 5, Ready: func() error { return nil }})
	login := performJSON(handler, http.MethodPost, "/api/v1/auth/login", map[string]any{"username": "owner", "password": "correct horse battery staple", "device": map[string]any{"id": "device", "name": "PC", "platform": "windows", "appVersion": "1"}}, "")
	var pair tokenResponse
	if err := json.Unmarshal(login.Body.Bytes(), &pair); err != nil {
		t.Fatal(err)
	}

	resp := performJSON(handler, http.MethodGet, "/api/v1/jobs/job-p", nil, pair.AccessToken)
	if resp.Code != http.StatusOK {
		t.Fatalf("get job status = %d, body=%s", resp.Code, resp.Body.String())
	}
	var payload struct {
		Job map[string]any `json:"job"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	files, _ := payload.Job["result"].(map[string]any)["files"].([]any)
	if len(files) != 2 {
		t.Fatalf("files = %#v", files)
	}
	var imagePreview, musicPreview string
	for _, item := range files {
		file := item.(map[string]any)
		preview, ok := file["previewUrl"].(string)
		if !ok || !bytes.Contains([]byte(preview), []byte("/api/v1/media-preview/")) {
			t.Fatalf("missing previewUrl: %#v", file)
		}
		if file["kind"] == "image" {
			imagePreview = preview
		}
		if file["kind"] == "music" {
			musicPreview = preview
		}
	}
	if imagePreview == "" || musicPreview == "" {
		t.Fatalf("missing typed previews: image=%q music=%q files=%#v", imagePreview, musicPreview, files)
	}

	req := httptest.NewRequest(http.MethodGet, imagePreview, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("media-preview status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("preview-image-bytes")) {
		t.Fatalf("media-preview body mismatch: %s", rec.Body.String())
	}
	musicReq := httptest.NewRequest(http.MethodGet, musicPreview, nil)
	musicRec := httptest.NewRecorder()
	handler.ServeHTTP(musicRec, musicReq)
	if musicRec.Code != http.StatusOK {
		t.Fatalf("music media-preview status = %d body=%s", musicRec.Code, musicRec.Body.String())
	}
	if !bytes.Contains(musicRec.Body.Bytes(), []byte("preview-music-bytes")) {
		t.Fatalf("music media-preview body mismatch: %s", musicRec.Body.String())
	}

	// 篡改签名应 404
	bad := httptest.NewRequest(http.MethodGet, strings.Replace(imagePreview, "signature=", "signature=0000", 1), nil)
	badRec := httptest.NewRecorder()
	handler.ServeHTTP(badRec, bad)
	if badRec.Code != http.StatusNotFound {
		t.Fatalf("tampered preview status = %d", badRec.Code)
	}
}
