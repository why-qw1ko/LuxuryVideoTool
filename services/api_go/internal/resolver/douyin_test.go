package resolver

import "testing"

func TestWorkFromRouterDataVideo(t *testing.T) {
	data := map[string]any{"loaderData": map[string]any{"item": map[string]any{
		"aweme_id": "123", "desc": "标题", "author": map[string]any{"uid": "u1", "nickname": "作者"},
		"video":      map[string]any{"duration": float64(1200), "width": float64(1080), "height": float64(1920), "play_addr": map[string]any{"url_list": []any{"https://example.test/playwm/video"}}},
		"text_extra": []any{map[string]any{"hashtag_name": "测试"}},
	}}}
	work, ok := workFromTree(data)
	if !ok || work.DouyinWorkID != "123" || work.AuthorName != "作者" || work.VideoURL == "" || len(work.Hashtags) != 1 {
		t.Fatalf("work = %#v", work)
	}
}

func TestWorkFromRouterDataNotePreservesOrder(t *testing.T) {
	data := map[string]any{"aweme_id": "456", "desc": "图文", "images": []any{
		map[string]any{"url_list": []any{"https://example.test/1"}}, map[string]any{"url_list": []any{"https://example.test/2"}},
	}}
	work, ok := workFromTree(data)
	if !ok || work.Type != "note" || len(work.Images) != 2 || work.Images[1].URL != "https://example.test/2" {
		t.Fatalf("work = %#v", work)
	}
}

func TestWithoutWatermark(t *testing.T) {
	got := withoutWatermark("https://example.test/playwm/video")
	if got != "https://example.test/play/video" {
		t.Fatalf("url = %q", got)
	}
}
