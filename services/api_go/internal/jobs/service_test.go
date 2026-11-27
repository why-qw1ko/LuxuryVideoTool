package jobs

import (
	"context"
	"database/sql"
	"path/filepath"
	"reflect"
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

type countingJobFileQuerier struct {
	db    *sql.DB
	calls int
}

func (q *countingJobFileQuerier) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	q.calls++
	return q.db.QueryContext(ctx, query, args...)
}

func TestListBatchesFilesWithoutCrossingPageOrOwner(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "batched-files.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now().UTC().Truncate(time.Millisecond)
	users := auth.NewSQLiteRepository(db)
	for _, id := range []string{"owner", "other"} {
		if err := users.CreateUser(ctx, auth.User{
			ID: id, UsernameNormalized: id, DisplayName: id, PasswordHash: "hash", Role: auth.RoleUser,
			IsActive: true, CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			t.Fatal(err)
		}
	}
	repo := NewSQLiteRepository(db)
	jobs := []Job{
		{ID: "current", UserID: "owner", Status: "completed"},
		{ID: "empty", UserID: "owner", Status: "completed"},
		{ID: "private", UserID: "other", Status: "completed"},
		{ID: "pending", UserID: "owner", Status: "queued"},
		{ID: "outside-page", UserID: "owner", Status: "completed"},
	}
	for index := range jobs {
		job := &jobs[index]
		job.InputText, job.InputURL = "https://www.douyin.com/video/123", "https://www.douyin.com/video/123"
		job.Action, job.IdempotencyKey, job.MaxAttempts = "download", job.ID, 3
		job.CreatedAt, job.UpdatedAt = now.Add(-time.Duration(index)*time.Minute), now
		if job.Status == "completed" {
			job.CompletedAt = &now
		}
		if err := repo.CreateQueued(ctx, *job); err != nil {
			t.Fatal(err)
		}
	}
	files := ownedfiles.NewRepository(db)
	addFile := func(userID, jobID, name string, at time.Time, expires *time.Time) ownedfiles.File {
		t.Helper()
		file, err := ownedfiles.NewFile(at, userID, jobID, "video", filepath.Join("media", userID, jobID, name), name, "video/mp4", "hash", 100, expires)
		if err != nil {
			t.Fatal(err)
		}
		if err := files.Create(ctx, file); err != nil {
			t.Fatal(err)
		}
		return file
	}
	deadline := now.Add(time.Hour)
	second := addFile("owner", "current", "second.mp4", now.Add(2*time.Millisecond), &deadline)
	first := addFile("owner", "current", "first.mp4", now, nil)
	addFile("other", "current", "wrong-owner.mp4", now, nil)
	removed := addFile("owner", "current", "deleted.mp4", now, nil)
	if err := files.MarkDeleted(ctx, removed.ID, now); err != nil {
		t.Fatal(err)
	}
	private := addFile("other", "private", "private.mp4", now, nil)
	addFile("owner", "pending", "unfinished.mp4", now, nil)
	addFile("owner", "outside-page", "outside.mp4", now, nil)

	ownerPage, err := repo.List(ctx, ListInput{UserID: "owner", Limit: 2})
	if err != nil || ownerPage.Total != 4 || len(ownerPage.Items) != 2 || ownerPage.Items[0].ID != "current" || ownerPage.Items[1].ID != "empty" {
		t.Fatalf("owner page=%+v error=%v", ownerPage, err)
	}
	assertFiles := func(job Job, want []string) {
		t.Helper()
		result, ok := job.Result.(map[string]any)
		if !ok {
			t.Fatalf("job %s result=%#v", job.ID, job.Result)
		}
		items, ok := result["files"].([]JobFile)
		if !ok {
			t.Fatalf("job %s files=%#v", job.ID, result["files"])
		}
		var ids []string
		for _, file := range items {
			ids = append(ids, file.ID)
			if file.ID == second.ID && (file.ExpiresAt == nil || !file.ExpiresAt.Equal(deadline)) {
				t.Fatalf("expiry lost in batch: %+v", file)
			}
		}
		if !reflect.DeepEqual(ids, want) || len(want) == 0 && items != nil {
			t.Fatalf("job %s files=%v want=%v", job.ID, ids, want)
		}
	}
	assertFiles(ownerPage.Items[0], []string{first.ID, second.ID})
	assertFiles(ownerPage.Items[1], nil)
	adminPage, err := repo.ListAll(ctx, ListInput{Limit: 4})
	if err != nil || adminPage.Total != 5 || len(adminPage.Items) != 4 {
		t.Fatalf("admin page=%+v error=%v", adminPage, err)
	}
	assertFiles(adminPage.Items[0], []string{first.ID, second.ID})
	assertFiles(adminPage.Items[1], nil)
	assertFiles(adminPage.Items[2], []string{private.ID})
	if adminPage.Items[3].ID != "pending" || adminPage.Items[3].Result != nil {
		t.Fatalf("unfinished job was hydrated: %+v", adminPage.Items[3])
	}
	emptyPage, err := repo.List(ctx, ListInput{UserID: "owner", Limit: 2, Offset: 10})
	if err != nil || emptyPage.Total != 4 || len(emptyPage.Items) != 0 || emptyPage.Items == nil {
		t.Fatalf("empty page=%+v error=%v", emptyPage, err)
	}

	counter := &countingJobFileQuerier{db: db}
	batch, err := findPageJobFiles(ctx, counter, adminPage.Items)
	if err != nil || counter.calls != 1 || len(batch) != 2 || len(batch["current"]) != 2 || len(batch["private"]) != 1 {
		t.Fatalf("batch=%+v calls=%d error=%v", batch, counter.calls, err)
	}
	counter.calls = 0
	if _, err := findPageJobFiles(ctx, counter, []Job{jobs[3]}); err != nil || counter.calls != 0 {
		t.Fatalf("unfinished-only page queried files: calls=%d error=%v", counter.calls, err)
	}
}
