package resolver

import (
	"encoding/json"
	"reflect"
	"testing"
)

func candidateDetailWork(t *testing.T, detail map[string]any) Work {
	t.Helper()
	body, err := json.Marshal(map[string]any{"status_code": 0, "aweme_detail": detail})
	if err != nil {
		t.Fatal(err)
	}
	work, err := NewDouyin(nil, nil).workFromDetailBody(body, "test")
	if err != nil {
		t.Fatal(err)
	}
	return work
}

func TestMediaURLCandidatesShapesAndFiltering(t *testing.T) {
	object := map[string]any{
		"primary": []any{
			"https://example.test/first", "not-a-url", "http://example.test/insecure",
			"https://user:password@example.test/private", "https:///missing-host", "https://example.test/%zz",
			map[string]any{"src": "https://example.test/second", "poster": "https://example.test/wrong-poster"},
			map[string]any{"src": "https://example.test/first"},
			map[string]any{"metadata": map[string]any{"src": "https://example.test/wrong-nested"}},
		},
		"secondary": map[string]any{
			"url_list": []any{"https://example.test/third", "https://example.test/first"},
			"urlList":  []string{"https://example.test/fourth"},
			"src":      "https://example.test/wrong-direct-src",
		},
		"last": "https://example.test/fifth",
	}
	want := []string{
		"https://example.test/first", "https://example.test/second", "https://example.test/third",
		"https://example.test/fourth", "https://example.test/fifth",
	}
	if got := mediaURLCandidates(object, "primary", "secondary", "last"); !reflect.DeepEqual(got, want) {
		t.Fatalf("candidates = %#v, want %#v", got, want)
	}
}

func TestWorkFromDetailBodyVideoCandidatePriority(t *testing.T) {
	work := candidateDetailWork(t, map[string]any{
		"aweme_id": "123",
		"video": map[string]any{
			"play_addr": map[string]any{"url_list": []string{
				"https://example.test/playwm/top.mp4", "https://example.test/play/top.mp4",
			}},
			"playAddr": "https://example.test/play/top.mp4",
			"bit_rate": []any{map[string]any{
				"play_addr": map[string]any{"url_list": []string{"https://example.test/rate-one.mp4"}},
				"cover":     "https://example.test/wrong-cover.jpg",
			}},
			"bitRate": []any{map[string]any{
				"playAddr": []any{map[string]any{"src": "https://example.test/rate-two.mp4"}},
			}},
			"download_addr": map[string]any{"url_list": []string{
				"https://example.test/rate-one.mp4", "https://example.test/playwm/download.mp4",
			}},
		},
	})
	want := []string{
		"https://example.test/play/top.mp4", "https://example.test/rate-one.mp4",
		"https://example.test/rate-two.mp4", "https://example.test/play/download.mp4",
	}
	if work.Type != "video" || work.VideoURL != want[0] || !reflect.DeepEqual(work.VideoURLCandidates, want) {
		t.Fatalf("video URL = %q, candidates = %#v", work.VideoURL, work.VideoURLCandidates)
	}
}

func TestWorkFromDetailBodyBitRateOnlyVideo(t *testing.T) {
	for _, variant := range []struct{ rates, play string }{{"bit_rate", "play_addr"}, {"bitRate", "playAddr"}} {
		t.Run(variant.rates, func(t *testing.T) {
			work := candidateDetailWork(t, map[string]any{
				"aweme_id": "123",
				"video": map[string]any{variant.rates: []any{map[string]any{
					variant.play: map[string]any{"urlList": []string{"https://example.test/rate.mp4"}},
				}}},
			})
			if work.VideoURL != "https://example.test/rate.mp4" || !reflect.DeepEqual(work.VideoURLCandidates, []string{work.VideoURL}) {
				t.Fatalf("work = %#v", work)
			}
		})
	}
}

func TestWorkFromDetailBodyNoteCandidateTypes(t *testing.T) {
	work := candidateDetailWork(t, map[string]any{
		"aweme_id": "123",
		"video":    map[string]any{"playAddr": "https://example.test/unrelated-note-video.mp4"},
		"images": []any{
			map[string]any{
				"url_list":          []string{"https://example.test/still-a.jpg", "https://example.test/still-b.jpg"},
				"download_url_list": []string{"https://example.test/still-a.jpg", "https://example.test/still-c.jpg"},
			},
			map[string]any{
				"display_image":     map[string]any{"urlList": []string{"https://example.test/live-poster.jpg"}},
				"download_url_list": []string{"https://example.test/live-poster-backup.jpg"},
				"video": map[string]any{
					"playAddr": []any{
						map[string]any{"src": "https://example.test/playwm/live-a.mp4"},
						map[string]any{"src": "https://example.test/play/live-a.mp4"},
						map[string]any{"src": "https://example.test/live-b.mp4"},
					},
					"downloadAddr": "https://example.test/live-c.mp4",
				},
			},
			map[string]any{"displayImage": map[string]any{
				"urlList": []string{"https://example.test/nested-poster.jpg"},
				"video":   map[string]any{"play_addr": "https://example.test/nested-live.mp4"},
			}},
		},
		"music": map[string]any{"play_url": map[string]any{"url_list": []string{
			"https://example.test/music-a.mp3", "https://example.test/music-b.mp3", "https://example.test/music-a.mp3",
		}}},
	})
	if work.Type != "note" || work.VideoURL != "" || len(work.VideoURLCandidates) != 0 || len(work.Images) != 3 {
		t.Fatalf("note work = %#v", work)
	}
	wantImages := []Image{
		{URL: "https://example.test/still-a.jpg", URLCandidates: []string{
			"https://example.test/still-a.jpg", "https://example.test/still-b.jpg", "https://example.test/still-c.jpg",
		}},
		{URL: "https://example.test/live-poster.jpg", URLCandidates: []string{"https://example.test/live-poster.jpg", "https://example.test/live-poster-backup.jpg"},
			AnimatedURL: "https://example.test/play/live-a.mp4", AnimatedURLCandidates: []string{
				"https://example.test/play/live-a.mp4", "https://example.test/live-b.mp4", "https://example.test/live-c.mp4",
			}},
		{URL: "https://example.test/nested-poster.jpg", URLCandidates: []string{"https://example.test/nested-poster.jpg"},
			AnimatedURL: "https://example.test/nested-live.mp4", AnimatedURLCandidates: []string{"https://example.test/nested-live.mp4"}},
	}
	if !reflect.DeepEqual(work.Images, wantImages) {
		t.Fatalf("images = %#v, want %#v", work.Images, wantImages)
	}
	wantMusic := []string{"https://example.test/music-a.mp3", "https://example.test/music-b.mp3"}
	if work.MusicURL != wantMusic[0] || !reflect.DeepEqual(work.MusicURLCandidates, wantMusic) {
		t.Fatalf("music URL = %q, candidates = %#v", work.MusicURL, work.MusicURLCandidates)
	}
}
