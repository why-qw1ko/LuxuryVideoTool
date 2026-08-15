package jobs

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/auth"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/database"
	ownedfiles "github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/files"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/media"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/resolver"
)

// blockingDownloader 阻塞直到上下文取消，模拟外部 CDN 卡死。
type blockingDownloader struct{}

func (b *blockingDownloader) Download(ctx context.Context, _, _, _ string, _ int64) (media.DownloadResult, error) {
	<-ctx.Done()
	return media.DownloadResult{}, ctx.Err()
}

// 图文/动图作品 full 动作：下载配图（动图下载动态版 MP4）+ 用配文当文案生成结果。
func TestWorkerNoteFullDownloadsImagesAndUsesCaption(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now().UTC()
	users := auth.NewSQLiteRepository(db)
	if err := users.CreateUser(ctx, auth.User{ID: "user-1", UsernameNormalized: "user", DisplayName: "User", PasswordHash: "hash", Role: auth.RoleUser, IsActive: true, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}

	repo := NewSQLiteRepository(db)
	job := Job{ID: "job-note", UserID: "user-1", InputText: "https://www.douyin.com/note/111", InputURL: "https://www.douyin.com/note/111", Action: "full", Status: "queued", IdempotencyKey: "key", MaxAttempts: 3, CreatedAt: now, UpdatedAt: now}
	if err := repo.CreateQueued(ctx, job); err != nil {
		t.Fatal(err)
	}
	claimed, err := repo.ClaimNext(ctx, "worker", now, time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	noteWork := resolver.Work{
		ID: "work-note", DouyinWorkID: "111", Type: "note",
		CanonicalURL: "https://www.douyin.com/note/111", Title: "图文标题", Description: "图文配文内容",
		AuthorName: "作者",
		Images: []resolver.Image{
			{URL: "https://p1.douyinpic.com/static.jpg", Width: 100, Height: 100, AnimatedURL: "https://v1.douyinvod.com/animated.mp4"},
			{URL: "https://p2.douyinpic.com/static2.jpg", Width: 200, Height: 200},
		},
		ResolverName: "fake", ResolverVersion: resolver.DouyinResolverVersion, ResolvedAt: now,
	}
	resolverService := resolver.NewService(&resolver.Fake{Work: noteWork}, resolver.NewSQLiteCache(db), 6*time.Hour, resolver.DouyinResolverVersion)

	fileRepo := ownedfiles.NewRepository(db)
	storage, err := ownedfiles.NewStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	worker := NewWorker(repo, resolverService, &media.FakeDownloader{Result: media.DownloadResult{MIMEType: "image/jpeg", SizeBytes: 10, SHA256: "aa"}}, fileRepo, storage, &Transcriber{FileRepo: fileRepo, Storage: storage}, WorkerConfig{Concurrency: 1, Lease: time.Minute, Heartbeat: time.Minute, Poll: time.Second, MaxVideoBytes: 1 << 30, VideoRetention: time.Hour})

	if err := worker.process(ctx, "worker", claimed); err != nil {
		t.Fatalf("process note job: %v", err)
	}

	finished, err := repo.FindByID(ctx, "user-1", "job-note")
	if err != nil {
		t.Fatal(err)
	}
	if finished.Status != "completed" {
		t.Fatalf("status = %q, want completed", finished.Status)
	}
	files, err := repo.FindFiles(ctx, "user-1", "job-note")
	if err != nil {
		t.Fatal(err)
	}
	var resultTextCount, mediaCount, mediaExpired int
	for _, f := range files {
		switch f.Kind {
		case "image", "animated":
			mediaCount++
			if f.ExpiresAt == nil {
				t.Fatalf("media file %s (%s) missing ExpiresAt", f.Name, f.Kind)
			}
			mediaExpired++
		case "result_text":
			resultTextCount++
		}
	}
	if mediaCount != 2 || mediaExpired != 2 {
		t.Fatalf("media files=%d (with expiry=%d), want 2 each; files=%#v", mediaCount, mediaExpired, files)
	}
	if resultTextCount != 1 {
		t.Fatalf("result_text=%d, want 1; files=%#v", resultTextCount, files)
	}
	result, ok := finished.Result.(map[string]any)
	if !ok {
		t.Fatalf("result not a map: %#v", finished.Result)
	}
	if got := result["normalizedText"]; got != "图文配文内容" {
		t.Fatalf("normalizedText = %v, want caption", got)
	}
}

// 图文/动图统一行为：download 动作同样下载配图并生成配文结果。
func TestWorkerNoteDownloadProducesMediaAndCaption(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now().UTC()
	users := auth.NewSQLiteRepository(db)
	if err := users.CreateUser(ctx, auth.User{ID: "user-1", UsernameNormalized: "user", DisplayName: "User", PasswordHash: "hash", Role: auth.RoleUser, IsActive: true, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	repo := NewSQLiteRepository(db)
	job := Job{ID: "job-dl", UserID: "user-1", InputText: "https://www.douyin.com/note/222", InputURL: "https://www.douyin.com/note/222", Action: "download", Status: "queued", IdempotencyKey: "key", MaxAttempts: 3, CreatedAt: now, UpdatedAt: now}
	if err := repo.CreateQueued(ctx, job); err != nil {
		t.Fatal(err)
	}
	claimed, err := repo.ClaimNext(ctx, "worker", now, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	noteWork := resolver.Work{
		ID: "work-dl", DouyinWorkID: "222", Type: "note", CanonicalURL: "https://www.douyin.com/note/222",
		Title: "图文", Description: "配文", AuthorName: "作者",
		Images: []resolver.Image{{URL: "https://p1.douyinpic.com/a.jpg"}},
		ResolverName: "fake", ResolverVersion: resolver.DouyinResolverVersion, ResolvedAt: now,
	}
	resolverService := resolver.NewService(&resolver.Fake{Work: noteWork}, resolver.NewSQLiteCache(db), 6*time.Hour, resolver.DouyinResolverVersion)
	fileRepo := ownedfiles.NewRepository(db)
	storage, _ := ownedfiles.NewStorage(t.TempDir())
	worker := NewWorker(repo, resolverService, &media.FakeDownloader{Result: media.DownloadResult{MIMEType: "image/jpeg", SizeBytes: 10, SHA256: "aa"}}, fileRepo, storage, &Transcriber{FileRepo: fileRepo, Storage: storage}, WorkerConfig{Concurrency: 1, Lease: time.Minute, Heartbeat: time.Minute, Poll: time.Second, MaxVideoBytes: 1 << 30, VideoRetention: time.Hour})

	if err := worker.process(ctx, "worker", claimed); err != nil {
		t.Fatalf("process note download: %v", err)
	}
	finished, err := repo.FindByID(ctx, "user-1", "job-dl")
	if err != nil {
		t.Fatal(err)
	}
	if finished.Status != "completed" {
		t.Fatalf("status = %q, want completed", finished.Status)
	}
	files, _ := repo.FindFiles(ctx, "user-1", "job-dl")
	var imageCount, resultTextCount int
	for _, f := range files {
		switch f.Kind {
		case "image", "animated":
			imageCount++
		case "result_text":
			resultTextCount++
		}
	}
	if imageCount != 1 || resultTextCount != 1 {
		t.Fatalf("image=%d result_text=%d, want 1 each; files=%#v", imageCount, resultTextCount, files)
	}
	result, ok := finished.Result.(map[string]any)
	if !ok {
		t.Fatalf("result not a map: %#v", finished.Result)
	}
	if result["normalizedText"] != "配文" {
		t.Fatalf("normalizedText = %v, want 配文", result["normalizedText"])
	}
}

// 下载阶段取消：processNote 应返回 context.Canceled 并将任务标记为 cancelled（而非 failed 或一直挂着）。
func TestWorkerNoteCancelDuringDownload(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now().UTC()
	users := auth.NewSQLiteRepository(db)
	if err := users.CreateUser(ctx, auth.User{ID: "user-1", UsernameNormalized: "user", DisplayName: "User", PasswordHash: "hash", Role: auth.RoleUser, IsActive: true, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	repo := NewSQLiteRepository(db)
	job := Job{ID: "job-cancel", UserID: "user-1", InputText: "https://www.douyin.com/note/333", InputURL: "https://www.douyin.com/note/333", Action: "download", Status: "queued", IdempotencyKey: "k", MaxAttempts: 3, CreatedAt: now, UpdatedAt: now}
	if err := repo.CreateQueued(ctx, job); err != nil {
		t.Fatal(err)
	}
	claimed, err := repo.ClaimNext(ctx, "worker", now, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	noteWork := resolver.Work{
		ID: "work-c", DouyinWorkID: "333", Type: "note", CanonicalURL: "https://www.douyin.com/note/333",
		Title: "图文", Description: "配文", AuthorName: "作者",
		Images: []resolver.Image{{URL: "https://p1.douyinpic.com/a.jpg", AnimatedURL: "https://v1.douyinvod.com/a.mp4"}},
		ResolverName: "fake", ResolverVersion: resolver.DouyinResolverVersion, ResolvedAt: now,
	}
	resolverService := resolver.NewService(&resolver.Fake{Work: noteWork}, resolver.NewSQLiteCache(db), 6*time.Hour, resolver.DouyinResolverVersion)
	fileRepo := ownedfiles.NewRepository(db)
	storage, _ := ownedfiles.NewStorage(t.TempDir())
	worker := NewWorker(repo, resolverService, &blockingDownloader{}, fileRepo, storage, &Transcriber{FileRepo: fileRepo, Storage: storage}, WorkerConfig{Concurrency: 1, Lease: time.Minute, Heartbeat: time.Minute, Poll: time.Second, MaxVideoBytes: 1 << 30, VideoRetention: time.Hour})

	done := make(chan error, 1)
	go func() { done <- worker.process(ctx, "worker", claimed) }()
	time.Sleep(150 * time.Millisecond) // 等待进入下载阶段
	worker.Cancel("job-cancel")
	err = <-done
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("process err = %v, want context.Canceled", err)
	}
	finished, err := repo.FindByID(ctx, "user-1", "job-cancel")
	if err != nil {
		t.Fatal(err)
	}
	if finished.Status != "cancelled" {
		t.Fatalf("status = %q, want cancelled (not failed/stuck)", finished.Status)
	}
}
