package resolver

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"testing"
)

const browserPageTestID = "7704588889991183434"

func browserPageJSON(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func browserPageFlight(t *testing.T, fragments ...string) []string {
	t.Helper()
	scripts := make([]string, 0, len(fragments))
	for _, fragment := range fragments {
		scripts = append(scripts, "self.__pace_f.push("+browserPageJSON(t, []any{1, fragment})+")")
	}
	return scripts
}

func browserPageRecord(t *testing.T, detail map[string]any) string {
	t.Helper()
	props := map[string]any{
		"awemeId": browserPageTestID,
		"aweme":   map[string]any{"statusCode": 0, "detail": detail},
	}
	return "7:" + browserPageJSON(t, []any{"$", "$L9", nil, props}) + "\n"
}

func browserPageWork(t *testing.T, scripts []string) Work {
	t.Helper()
	body, err := detailFromPageScripts(scripts, browserPageTestID)
	if err != nil {
		t.Fatalf("detailFromPageScripts: %v", err)
	}
	work, err := NewDouyin(nil, nil).workFromDetailBody(body, "browser_page")
	if err != nil {
		t.Fatalf("workFromDetailBody: %v", err)
	}
	if work.DouyinWorkID != browserPageTestID {
		t.Fatalf("work ID = %q, want %q", work.DouyinWorkID, browserPageTestID)
	}
	return work
}

func TestDetailFromPageScriptsFlightNote55Images(t *testing.T) {
	images := make([]any, 55)
	for i := range images {
		images[i] = map[string]any{
			"width": 1080, "height": 1080,
			"urlList": []string{fmt.Sprintf("https://example.test/image-%02d.jpg", i)},
		}
	}
	detail := map[string]any{
		"awemeId": browserPageTestID, "awemeType": 68, "desc": "图文配文 #头像",
		"authorInfo": map[string]any{"uid": "author-1", "nickname": "作者"},
		"images":     images, "createTime": 1793864391,
		"textExtra": []any{map[string]any{"hashtagName": "头像"}},
	}
	work := browserPageWork(t, browserPageFlight(t, browserPageRecord(t, detail)))
	if work.Type != "note" || len(work.Images) != 55 || work.VideoURL != "" {
		t.Fatalf("type=%q images=%d video=%q", work.Type, len(work.Images), work.VideoURL)
	}
	for i, image := range work.Images {
		if image.URL != fmt.Sprintf("https://example.test/image-%02d.jpg", i) || image.Width != 1080 || image.Height != 1080 {
			t.Fatalf("image %d = %#v", i, image)
		}
	}
	if work.AuthorID != "author-1" || work.AuthorName != "作者" || work.Description != "图文配文 #头像" {
		t.Fatalf("author=%q/%q description=%q", work.AuthorID, work.AuthorName, work.Description)
	}
	if work.PublishedAt == nil || work.PublishedAt.Unix() != 1793864391 || len(work.Hashtags) != 1 || work.Hashtags[0] != "头像" {
		t.Fatalf("published=%v hashtags=%v", work.PublishedAt, work.Hashtags)
	}
}

func TestDetailFromPageScriptsFlightVideoAndAnimatedImage(t *testing.T) {
	t.Run("video", func(t *testing.T) {
		detail := map[string]any{
			"awemeId": browserPageTestID, "desc": "视频",
			"authorInfo": map[string]any{"nickname": "作者"},
			"video": map[string]any{
				"playAddr": "https://example.test/playwm/video.mp4", "duration": 1200,
				"width": 1080, "height": 1920, "cover": "https://example.test/cover.jpg",
			},
		}
		work := browserPageWork(t, browserPageFlight(t, browserPageRecord(t, detail)))
		if work.Type != "video" || work.VideoURL != "https://example.test/play/video.mp4" || work.CoverURL != "https://example.test/cover.jpg" || work.DurationMS != 1200 {
			t.Fatalf("work = %#v", work)
		}
	})
	t.Run("animated image", func(t *testing.T) {
		detail := map[string]any{
			"awemeId": browserPageTestID,
			"images": []any{map[string]any{
				"urlList": []string{"https://example.test/still.jpg"},
				"video":   map[string]any{"playAddr": "https://example.test/playwm/animated.mp4"},
			}},
		}
		work := browserPageWork(t, browserPageFlight(t, browserPageRecord(t, detail)))
		if work.Type != "note" || len(work.Images) != 1 || work.Images[0].AnimatedURL != "https://example.test/play/animated.mp4" {
			t.Fatalf("work = %#v", work)
		}
	})
}

func TestDetailFromPageScriptsJoinsFlightFragments(t *testing.T) {
	detail := map[string]any{"awemeId": browserPageTestID, "images": []any{
		map[string]any{"urlList": []string{"https://example.test/a.jpg"}},
	}}
	record := browserPageRecord(t, detail)
	cut := strings.Index(record, "https://") + len("https://example")
	work := browserPageWork(t, browserPageFlight(t, record[:cut], record[cut:]))
	if len(work.Images) != 1 || work.Images[0].URL != "https://example.test/a.jpg" {
		t.Fatalf("images = %#v", work.Images)
	}
}

func TestDetailFromPageScriptsPreservesNumericID(t *testing.T) {
	detail := map[string]any{
		"awemeId": json.Number(browserPageTestID),
		"images":  []any{map[string]any{"urlList": []string{"https://example.test/a.jpg"}}},
	}
	work := browserPageWork(t, browserPageFlight(t, browserPageRecord(t, detail)))
	if len(work.Images) != 1 {
		t.Fatalf("images = %#v", work.Images)
	}
}

func TestDetailFromPageScriptsLegacyRouterAndRenderData(t *testing.T) {
	imageURL := "https://example.test/a+b.jpg?signature=a+b%2Bc"
	target := map[string]any{"aweme_id": browserPageTestID, "desc": "A+B", "images": []any{
		map[string]any{"url_list": []string{imageURL}},
	}}
	recommended := map[string]any{"aweme_id": "1111111111111111111", "images": []any{
		map[string]any{"url_list": []string{"https://example.test/recommended.jpg"}},
	}}
	data := map[string]any{"loaderData": map[string]any{"note_(id)/page": map[string]any{
		"itemId": browserPageTestID,
		"videoInfoRes": map[string]any{"item_list": []any{
			recommended, map[string]any{"aweme_id": browserPageTestID}, target,
		}},
	}}}
	raw := browserPageJSON(t, data)
	for name, script := range map[string]string{
		"router assignment":   "window._ROUTER_DATA = " + raw + ";",
		"plain render data":   raw,
		"encoded render data": url.QueryEscape(raw),
	} {
		t.Run(name, func(t *testing.T) {
			work := browserPageWork(t, []string{script})
			if len(work.Images) != 1 || work.Images[0].URL != imageURL || work.Description != "A+B" {
				t.Fatalf("images=%#v description=%q", work.Images, work.Description)
			}
		})
	}
}

func TestDetailFromPageScriptsIgnoresOtherWorksAndShells(t *testing.T) {
	target := map[string]any{"awemeId": browserPageTestID, "images": []any{
		map[string]any{"urlList": []string{"https://example.test/target.jpg"}},
	}}
	recommended := map[string]any{"awemeId": "1111111111111111111", "images": []any{
		map[string]any{"urlList": []string{"https://example.test/recommended.jpg"}},
	}}
	stream := browserPageRecord(t, recommended) + browserPageRecord(t, map[string]any{"awemeId": browserPageTestID}) + browserPageRecord(t, target)
	work := browserPageWork(t, browserPageFlight(t, stream))
	if len(work.Images) != 1 || work.Images[0].URL != "https://example.test/target.jpg" {
		t.Fatalf("images = %#v", work.Images)
	}
}

func TestDetailFromPageScriptsRejectsUnusableData(t *testing.T) {
	recommended := map[string]any{"awemeId": "1111111111111111111", "images": []any{
		map[string]any{"urlList": []string{"https://example.test/other.jpg"}},
	}}
	mediaWithoutID := map[string]any{"images": []any{
		map[string]any{"urlList": []string{"https://example.test/unattributed.jpg"}},
	}}
	cases := map[string][]string{
		"no data":    nil,
		"wrong work": browserPageFlight(t, browserPageRecord(t, recommended)),
		"only target shell": browserPageFlight(t, browserPageRecord(t, map[string]any{
			"awemeId": browserPageTestID, "desc": "有标题和作者但没有媒体", "authorInfo": map[string]any{"nickname": "作者"},
		})),
		"do not attach expected ID": browserPageFlight(t, browserPageRecord(t, mediaWithoutID)),
		"do not combine works":      browserPageFlight(t, browserPageRecord(t, map[string]any{"awemeId": browserPageTestID})+browserPageRecord(t, recommended)),
		"broken flight":             {`self.__pace_f.push([1,"7:broken"])`},
		"text mentions target ID": browserPageFlight(t, browserPageRecord(t, map[string]any{
			"awemeId": "1111111111111111111", "desc": browserPageTestID,
			"images": []any{map[string]any{"urlList": []string{"https://example.test/other.jpg"}}},
		})),
	}
	for name, scripts := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := detailFromPageScripts(scripts, browserPageTestID); err == nil {
				t.Fatal("unusable data must not resolve a work")
			}
		})
	}
}

func TestDetailFromPageScriptsDoesNotEvaluateArbitraryCode(t *testing.T) {
	detail := map[string]any{"awemeId": browserPageTestID, "images": []any{
		map[string]any{"urlList": []string{"https://example.test/a.jpg"}},
	}}
	flight := browserPageFlight(t, browserPageRecord(t, detail))[0]
	for name, script := range map[string]string{
		"unrelated assignment":       "window.unrelated = " + browserPageJSON(t, detail) + ";",
		"script in a string":         "const quoted = " + browserPageJSON(t, flight) + ";",
		"expression instead of JSON": "self.__pace_f.push((()=>" + browserPageJSON(t, []any{1, browserPageRecord(t, detail)}) + ")())",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := detailFromPageScripts([]string{script}, browserPageTestID); err == nil {
				t.Fatal("unrecognized JavaScript must not be treated as page data")
			}
		})
	}
}
