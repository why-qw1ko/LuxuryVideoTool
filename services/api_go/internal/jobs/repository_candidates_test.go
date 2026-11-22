package jobs

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/auth"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/database"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/resolver"
)

func TestJobRepositoryReadsMediaCandidates(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "job-candidates.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now().UTC()
	if err := auth.NewSQLiteRepository(db).CreateUser(ctx, auth.User{
		ID: "owner", UsernameNormalized: "owner", DisplayName: "Owner", PasswordHash: "hash",
		Role: auth.RoleUser, IsActive: true, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	repo := NewSQLiteRepository(db)
	wantByJob := make(map[string]resolver.Work)
	for _, input := range []resolver.Work{
		{
			DouyinWorkID: "123", Type: "video", CanonicalURL: "https://www.douyin.com/video/123",
			VideoURL: "https://example.test/video-a.mp4", VideoURLCandidates: []string{"https://example.test/video-a.mp4", "https://example.test/video-b.mp4"},
			MusicURL: "https://example.test/music-a.mp3", MusicURLCandidates: []string{"https://example.test/music-a.mp3", "https://example.test/music-b.mp3"},
		},
		{
			DouyinWorkID: "456", Type: "note", CanonicalURL: "https://www.douyin.com/note/456",
			Images: []resolver.Image{
				{URL: "https://example.test/still.jpg", URLCandidates: []string{"https://example.test/still.jpg", "https://example.test/still-backup.jpg"}},
				{URL: "https://example.test/poster.jpg", URLCandidates: []string{"https://example.test/poster.jpg"},
					AnimatedURL: "https://example.test/live.mp4", AnimatedURLCandidates: []string{"https://example.test/live.mp4", "https://example.test/live-backup.mp4"}},
			},
			MusicURL: "https://example.test/music.mp3", MusicURLCandidates: []string{"https://example.test/music.mp3"},
		},
	} {
		input.ResolverName, input.ResolverVersion, input.ResolvedAt = "test", resolver.DouyinResolverVersion, now
		work, err := resolver.NewSQLiteCache(db).Save(ctx, input)
		if err != nil {
			t.Fatal(err)
		}
		job := Job{
			ID: "job-" + input.DouyinWorkID, UserID: "owner", InputText: input.CanonicalURL, InputURL: input.CanonicalURL,
			Action: "info", Status: "resolving", IdempotencyKey: input.DouyinWorkID, CreatedAt: now, UpdatedAt: now,
		}
		if err := repo.CreateInfo(ctx, job); err != nil {
			t.Fatal(err)
		}
		if err := repo.CompleteInfo(ctx, job.ID, work, now); err != nil {
			t.Fatal(err)
		}
		wantByJob[job.ID] = input
		for _, read := range []func() (Job, error){
			func() (Job, error) { return repo.FindByID(ctx, "owner", job.ID) },
			func() (Job, error) { return repo.FindAnyByID(ctx, job.ID) },
		} {
			got, err := read()
			if err != nil {
				t.Fatal(err)
			}
			assertJobMediaCandidates(t, got, input)
		}
	}
	page, err := repo.List(ctx, ListInput{UserID: "owner", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != len(wantByJob) {
		t.Fatalf("job list has %d entries", len(page.Items))
	}
	for _, job := range page.Items {
		assertJobMediaCandidates(t, job, wantByJob[job.ID])
	}
}

func assertJobMediaCandidates(t *testing.T, job Job, want resolver.Work) {
	t.Helper()
	got := job.Work
	if got == nil || got.VideoURL != want.VideoURL || got.MusicURL != want.MusicURL ||
		!reflect.DeepEqual(got.VideoURLCandidates, want.VideoURLCandidates) ||
		!reflect.DeepEqual(got.MusicURLCandidates, want.MusicURLCandidates) || !reflect.DeepEqual(got.Images, want.Images) {
		t.Fatalf("job %s lost media candidates: %#v", job.ID, got)
	}
}
