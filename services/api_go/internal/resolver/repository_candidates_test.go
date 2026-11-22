package resolver

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/database"
)

func TestSQLiteCachePreservesMediaCandidates(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "candidates.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	cache := NewSQLiteCache(db)
	now := time.Now().UTC()
	for _, work := range []Work{
		{
			DouyinWorkID: "123", Type: "video", CanonicalURL: "https://www.douyin.com/video/123",
			VideoURL: "https://example.test/video-a.mp4", VideoURLCandidates: []string{"https://example.test/video-a.mp4", "https://example.test/video-b.mp4"},
			MusicURL: "https://example.test/music-a.mp3", MusicURLCandidates: []string{"https://example.test/music-a.mp3", "https://example.test/music-b.mp3"},
		},
		{
			DouyinWorkID: "456", Type: "note", CanonicalURL: "https://www.douyin.com/note/456",
			Images: []Image{
				{URL: "https://example.test/still-a.jpg", URLCandidates: []string{"https://example.test/still-a.jpg", "https://example.test/still-b.jpg"}},
				{URL: "https://example.test/poster.jpg", URLCandidates: []string{"https://example.test/poster.jpg"},
					AnimatedURL: "https://example.test/live-a.mp4", AnimatedURLCandidates: []string{"https://example.test/live-a.mp4", "https://example.test/live-b.mp4"}},
			},
			MusicURL: "https://example.test/music.mp3", MusicURLCandidates: []string{"https://example.test/music.mp3"},
		},
	} {
		work.ResolverName, work.ResolverVersion, work.ResolvedAt = "test", DouyinResolverVersion, now
		saved, err := cache.Save(ctx, work)
		if err != nil {
			t.Fatal(err)
		}
		fresh, err := cache.FindFresh(ctx, work.DouyinWorkID, DouyinResolverVersion, now.Add(-time.Second))
		if err != nil {
			t.Fatal(err)
		}
		for _, got := range []Work{saved, fresh} {
			if got.VideoURL != work.VideoURL || got.MusicURL != work.MusicURL ||
				!reflect.DeepEqual(got.VideoURLCandidates, work.VideoURLCandidates) ||
				!reflect.DeepEqual(got.MusicURLCandidates, work.MusicURLCandidates) || !reflect.DeepEqual(got.Images, work.Images) {
				t.Fatalf("cached work %s lost media candidates: %#v", work.DouyinWorkID, got)
			}
		}
		if _, err := cache.FindFresh(ctx, work.DouyinWorkID, "6", now.Add(-time.Second)); !errors.Is(err, ErrCacheMiss) {
			t.Fatalf("previous resolver version error = %v, want cache miss", err)
		}
	}
}

func TestSQLiteCacheReadsLegacyMediaWithoutCandidates(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "legacy.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	work := Work{
		DouyinWorkID: "123", Type: "video", CanonicalURL: "https://www.douyin.com/video/123",
		VideoURL: "https://example.test/video.mp4", MusicURL: "https://example.test/music.mp3",
		ResolverName: "test", ResolverVersion: DouyinResolverVersion, ResolvedAt: time.Now().UTC(),
	}
	cache := NewSQLiteCache(db)
	if _, err := cache.Save(ctx, work); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE works SET metadata_json = ? WHERE douyin_work_id = ?`,
		`{"videoUrl":"https://example.test/video.mp4","musicUrl":"https://example.test/music.mp3"}`, work.DouyinWorkID); err != nil {
		t.Fatal(err)
	}
	got, err := cache.FindFresh(ctx, work.DouyinWorkID, DouyinResolverVersion, work.ResolvedAt.Add(-time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if got.VideoURL != work.VideoURL || got.MusicURL != work.MusicURL || got.VideoURLCandidates != nil || got.MusicURLCandidates != nil {
		t.Fatalf("legacy media = %#v", got)
	}
}
