package jobs

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/auth"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/database"
	ownedfiles "github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/files"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/resolver"
)

func TestCreateInfoAndReuseIdempotencyKey(t *testing.T) {
	db, err := database.Open(context.Background(), filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	users := auth.NewSQLiteRepository(db)
	now := time.Now().UTC()
	if err := users.CreateUser(context.Background(), auth.User{ID: "user-1", UsernameNormalized: "user", DisplayName: "User", PasswordHash: "hash", Role: auth.RoleUser, IsActive: true, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	fake := &resolver.Fake{Work: resolver.Work{DouyinWorkID: "123", Type: "video", CanonicalURL: "https://www.douyin.com/video/123", Title: "标题", ResolverName: "fake", ResolverVersion: resolver.DouyinResolverVersion, ResolvedAt: now}}
	service := NewService(NewSQLiteRepository(db), resolver.NewService(fake, resolver.NewSQLiteCache(db), 6*time.Hour, resolver.DouyinResolverVersion))
	input := CreateInput{UserID: "user-1", ShareText: "https://www.douyin.com/video/123", IdempotencyKey: "key-1"}
	job, reused, err := service.CreateInfo(context.Background(), input)
	if err != nil || reused || job.Status != "completed" || job.Work == nil {
		t.Fatalf("job=%#v reused=%v err=%v", job, reused, err)
	}
	second, reused, err := service.CreateInfo(context.Background(), input)
	if err != nil || !reused || second.ID != job.ID || fake.Calls != 1 {
		t.Fatalf("second=%#v reused=%v calls=%d err=%v", second, reused, fake.Calls, err)
	}
}

// 仅解析（info）：对视频和图文/动图都同步完成、不下载媒体（"仅解析信息"勾选）。
func TestCreateInfoIsParseOnlyForNotes(t *testing.T) {
	db, err := database.Open(context.Background(), filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	users := auth.NewSQLiteRepository(db)
	now := time.Now().UTC()
	if err := users.CreateUser(context.Background(), auth.User{ID: "user-1", UsernameNormalized: "user", DisplayName: "User", PasswordHash: "hash", Role: auth.RoleUser, IsActive: true, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	noteWork := resolver.Work{
		ID: "work-n", DouyinWorkID: "123", Type: "note", CanonicalURL: "https://www.douyin.com/note/123",
		Title: "图文", Description: "配文", AuthorName: "作者",
		Images: []resolver.Image{{URL: "https://p1.douyinpic.com/a.jpg"}},
		ResolverName: "fake", ResolverVersion: resolver.DouyinResolverVersion, ResolvedAt: now,
	}
	service := NewService(NewSQLiteRepository(db), resolver.NewService(&resolver.Fake{Work: noteWork}, resolver.NewSQLiteCache(db), 6*time.Hour, resolver.DouyinResolverVersion))
	job, reused, err := service.CreateInfo(context.Background(), CreateInput{UserID: "user-1", ShareText: "https://www.douyin.com/note/123", IdempotencyKey: "key-note"})
	if err != nil || reused {
		t.Fatalf("job=%#v reused=%v err=%v", job, reused, err)
	}
	// info 同步完成、仅解析（不下载媒体）
	if job.Status != "completed" {
		t.Fatalf("note info status = %q, want completed (parse-only)", job.Status)
	}
	files, err := NewSQLiteRepository(db).FindFiles(context.Background(), "user-1", job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 0 {
		t.Fatalf("parse-only job has files: %#v", files)
	}
	if job.Work == nil || job.Work.Type != "note" || len(job.Work.Images) != 1 {
		t.Fatalf("parse-only work = %#v", job.Work)
	}

	// 同一 key 再次提交 → 复用
	second, reused, err := service.CreateInfo(context.Background(), CreateInput{UserID: "user-1", ShareText: "https://www.douyin.com/note/123", IdempotencyKey: "key-note"})
	if err != nil || !reused || second.ID != job.ID {
		t.Fatalf("second=%#v reused=%v err=%v", second, reused, err)
	}
}

func TestListCompletedDownloadIncludesFiles(t *testing.T) {
	db, err := database.Open(context.Background(), filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now().UTC()
	users := auth.NewSQLiteRepository(db)
	if err := users.CreateUser(context.Background(), auth.User{ID: "user-1", UsernameNormalized: "user", DisplayName: "User", PasswordHash: "hash", Role: auth.RoleUser, IsActive: true, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	repo := NewSQLiteRepository(db)
	job := Job{ID: "job-1", UserID: "user-1", InputText: "https://www.douyin.com/video/123", InputURL: "https://www.douyin.com/video/123", Action: "download", Status: "queued", IdempotencyKey: "key", MaxAttempts: 3, CreatedAt: now, UpdatedAt: now}
	if err := repo.CreateQueued(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	claimed, err := repo.ClaimNext(context.Background(), "worker", now, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	work, err := resolver.NewSQLiteCache(db).Save(context.Background(), resolver.Work{ID: "work-1", DouyinWorkID: "123", Type: "video", CanonicalURL: "https://www.douyin.com/video/123", ResolverName: "test", ResolverVersion: resolver.DouyinResolverVersion, ResolvedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.SetResolved(context.Background(), claimed.ID, "worker", work, now); err != nil {
		t.Fatal(err)
	}
	fileRepo := ownedfiles.NewRepository(db)
	file, err := ownedfiles.NewFile(now, "user-1", job.ID, "video", filepath.Join("media", "user-1", job.ID, "video.mp4"), "123.mp4", "video/mp4", "hash", 100, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := fileRepo.Create(context.Background(), file); err != nil {
		t.Fatal(err)
	}
	if err := repo.CompleteDownload(context.Background(), job.ID, "worker", file.ID, now); err != nil {
		t.Fatal(err)
	}
	page, err := repo.List(context.Background(), ListInput{UserID: "user-1", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	result, ok := page.Items[0].Result.(map[string]any)
	if !ok {
		t.Fatalf("result = %#v", page.Items[0].Result)
	}
	files, ok := result["files"].([]JobFile)
	if !ok || len(files) != 1 || files[0].ID != file.ID {
		t.Fatalf("files = %#v", result["files"])
	}
}
