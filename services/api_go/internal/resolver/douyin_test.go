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

// TestWorkFromRouterDataNavigatesToItemList 复现真实抖音 _ROUTER_DATA 结构：
// 页面级对象含纯数字 itemId，整树查找会先命中它返回只有 ID 的空壳；
// 定向导航必须越过它，取到 videoInfoRes.item_list[0] 里的真实作品。
func TestWorkFromRouterDataNavigatesToItemList(t *testing.T) {
	data := map[string]any{"loaderData": map[string]any{
		"video_(id)/page": map[string]any{
			"itemId": "123",
			"videoInfoRes": map[string]any{
				"item_list": []any{map[string]any{
					"aweme_id": "123", "desc": "标题",
					"author": map[string]any{"sec_uid": "u1", "nickname": "作者"},
					"video":  map[string]any{"play_addr": map[string]any{"url_list": []any{"https://example.test/playwm/x"}}},
				}},
			},
		},
	}}
	work, ok := workFromRouterData(data)
	if !ok {
		t.Fatal("workFromRouterData should resolve the real work")
	}
	if work.VideoURL != "https://example.test/playwm/x" {
		t.Fatalf("VideoURL = %q, want the item_list work URL", work.VideoURL)
	}
	if work.AuthorName != "作者" {
		t.Fatalf("AuthorName = %q", work.AuthorName)
	}
}

// TestWorkFromRouterDataNote 验证图文作品的定向导航路径 note_(id)/page。
func TestWorkFromRouterDataNote(t *testing.T) {
	data := map[string]any{"loaderData": map[string]any{
		"note_(id)/page": map[string]any{
			"itemId": "789",
			"videoInfoRes": map[string]any{
				"item_list": []any{map[string]any{
					"aweme_id": "789", "desc": "图集",
					"images": []any{
						map[string]any{"url_list": []any{"https://example.test/1"}},
						map[string]any{"url_list": []any{"https://example.test/2"}},
					},
				}},
			},
		},
	}}
	work, ok := workFromRouterData(data)
	if !ok || work.Type != "note" || len(work.Images) != 2 || work.Images[1].URL != "https://example.test/2" {
		t.Fatalf("work = %#v", work)
	}
}
