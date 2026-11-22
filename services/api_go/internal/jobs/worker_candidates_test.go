package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/auth"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/database"
	ownedfiles "github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/files"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/media"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/resolver"
)

type workerCandidateDownloader struct {
	fail  map[string]bool
	mimes map[string]string
	calls []string
}

func (d *workerCandidateDownloader) Download(_ context.Context, mediaURL, temporary, final string, _ int64) (media.DownloadResult, error) {
	d.calls = append(d.calls, mediaURL)
	if err := os.WriteFile(temporary, []byte("media content"), 0o600); err != nil {
		return media.DownloadResult{}, err
	}
	if d.fail[mediaURL] {
		return media.DownloadResult{}, media.ErrDownload
	}
	if err := os.Rename(temporary, final); err != nil {
		return media.DownloadResult{}, err
	}
	return media.DownloadResult{SizeBytes: 13, SHA256: "test-sha", MIMEType: d.mimes[mediaURL]}, nil
}

func runWorkerFromCandidateCache(t *testing.T, work resolver.Work, downloader media.Downloader) (Job, []ownedfiles.File, map[string]any) {
	t.Helper()
	ctx := context.Background()
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now().UTC()
	users := auth.NewSQLiteRepository(db)
	if err := users.CreateUser(ctx, auth.User{ID: "candidate-user", UsernameNormalized: "candidate", DisplayName: "User", PasswordHash: "hash", Role: auth.RoleUser, IsActive: true, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	work.ResolverName, work.ResolverVersion, work.ResolvedAt = "fake", resolver.DouyinResolverVersion, now
	cache := resolver.NewSQLiteCache(db)
	if _, err := cache.Save(ctx, work); err != nil {
		t.Fatal(err)
	}
	adapter := &resolver.Fake{Err: errors.New("worker must read saved candidates from cache")}
	service := resolver.NewService(adapter, cache, time.Hour, resolver.DouyinResolverVersion)
	repo := NewSQLiteRepository(db)
	job := Job{ID: "candidate-job", UserID: "candidate-user", InputText: work.CanonicalURL, InputURL: work.CanonicalURL, Action: "download", Status: "queued", IdempotencyKey: "candidate-key", MaxAttempts: 3, CreatedAt: now, UpdatedAt: now}
	if err := repo.CreateQueued(ctx, job); err != nil {
		t.Fatal(err)
	}
	claimed, err := repo.ClaimNext(ctx, "candidate-worker", now, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	fileRepo := ownedfiles.NewRepository(db)
	storage, err := ownedfiles.NewStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	worker := NewWorker(repo, service, downloader, fileRepo, storage, nil, WorkerConfig{Concurrency: 1, Lease: time.Minute, Heartbeat: time.Minute, Poll: time.Second, MaxVideoBytes: 1 << 20, VideoRetention: time.Hour})
	if err := worker.process(ctx, "candidate-worker", claimed); err != nil {
		t.Fatal(err)
	}
	if adapter.Calls != 0 {
		t.Fatalf("resolver called %d times, want cache hit", adapter.Calls)
	}
	finished, err := repo.FindByID(ctx, job.UserID, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	files, err := fileRepo.ListByJob(ctx, job.UserID, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	var detailsJSON string
	if err := db.QueryRowContext(ctx, "SELECT details_json FROM job_steps WHERE job_id = ? AND step_name = 'download'", job.ID).Scan(&detailsJSON); err != nil {
		t.Fatal(err)
	}
	var details map[string]any
	if err := json.Unmarshal([]byte(detailsJSON), &details); err != nil {
		t.Fatal(err)
	}
	return finished, files, details
}

func TestWorkerVideoUsesCachedCandidates(t *testing.T) {
	primary, backup := "https://video-one.example/a.mp4", "https://video-two.example/a.mp4"
	downloader := &workerCandidateDownloader{fail: map[string]bool{primary: true}, mimes: map[string]string{backup: "video/mp4"}}
	work := resolver.Work{DouyinWorkID: "888", Type: "video", CanonicalURL: "https://www.douyin.com/video/888", Title: "视频", VideoURL: primary, VideoURLCandidates: []string{primary, backup}}
	finished, files, _ := runWorkerFromCandidateCache(t, work, downloader)
	if finished.Status != "completed" || !reflect.DeepEqual(downloader.calls, []string{primary, backup}) {
		t.Fatalf("status=%s calls=%v", finished.Status, downloader.calls)
	}
	if len(files) != 1 || files[0].Kind != "video" || files[0].OriginalName != "888.mp4" {
		t.Fatalf("video files=%#v", files)
	}
	if finished.Work == nil || !reflect.DeepEqual(finished.Work.VideoURLCandidates, []string{primary, backup}) {
		t.Fatalf("job lost video candidates: %#v", finished.Work)
	}
}

func TestWorkerNoteUsesCandidatesAndReportsMissingMedia(t *testing.T) {
	imagePrimary, imageBackup := "https://image-one.example/a.png", "https://image-two.example/a.png"
	animatedPrimary, animatedBackup := "https://animated-one.example/a.mp4", "https://animated-two.example/a.mp4"
	musicPrimary, musicBackup := "https://music-one.example/a.mp3", "https://music-two.example/a.mp3"
	webp, missing := "https://image.example/a.webp", "https://image.example/missing.jpg"
	downloader := &workerCandidateDownloader{
		fail:  map[string]bool{imagePrimary: true, animatedPrimary: true, musicPrimary: true, missing: true},
		mimes: map[string]string{imageBackup: "image/png; charset=binary", animatedBackup: "video/mp4", webp: "image/webp", musicBackup: "audio/mpeg"},
	}
	work := resolver.Work{
		DouyinWorkID: "777", Type: "note", CanonicalURL: "https://www.douyin.com/note/777", Title: "图文", Description: "配文",
		Images: []resolver.Image{
			{URL: imagePrimary, URLCandidates: []string{imagePrimary, imageBackup}},
			{URL: "https://image.example/animated-cover.jpg", URLCandidates: []string{"https://image.example/unused-static.jpg"}, AnimatedURL: animatedPrimary, AnimatedURLCandidates: []string{animatedPrimary, animatedBackup}},
			{URL: webp},
			{URL: missing},
		},
		MusicURL: musicPrimary, MusicURLCandidates: []string{musicPrimary, musicBackup},
	}
	finished, files, details := runWorkerFromCandidateCache(t, work, downloader)
	wantCalls := []string{imagePrimary, imageBackup, animatedPrimary, animatedBackup, webp, missing, musicPrimary, musicBackup}
	if finished.Status != "completed" || !reflect.DeepEqual(downloader.calls, wantCalls) {
		t.Fatalf("status=%s calls=%v", finished.Status, downloader.calls)
	}
	if !strings.Contains(finished.StatusMessage, "3/4") || !strings.Contains(finished.StatusMessage, "第 4 项") {
		t.Fatalf("partial status=%q", finished.StatusMessage)
	}
	names := map[string]string{}
	for _, file := range files {
		names[file.OriginalName] = file.Kind
	}
	for name, kind := range map[string]string{"image-01.png": "image", "image-02.mp4": "animated", "image-03.webp": "image", "777.mp3": "music"} {
		if names[name] != kind {
			t.Fatalf("expected %s (%s), files=%v", name, kind, names)
		}
	}
	if _, exists := names["image-04.jpg"]; exists {
		t.Fatal("failed image was registered as downloaded")
	}
	for _, stats := range []map[string]any{details, finished.Result.(map[string]any)["mediaDownload"].(map[string]any)} {
		if stats["expected"] != float64(4) || stats["downloaded"] != float64(3) || stats["skipped"] != float64(1) || stats["files"] != float64(4) || stats["musicDownloaded"] != true {
			t.Fatalf("download statistics=%#v", stats)
		}
		if !reflect.DeepEqual(stats["skippedIndices"], []any{float64(4)}) {
			t.Fatalf("missing indices=%#v", stats["skippedIndices"])
		}
	}
	if finished.Work == nil || !reflect.DeepEqual(finished.Work.MusicURLCandidates, []string{musicPrimary, musicBackup}) {
		t.Fatalf("job lost music candidates: %#v", finished.Work)
	}
}
