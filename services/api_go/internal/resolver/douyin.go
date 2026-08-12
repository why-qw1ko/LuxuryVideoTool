package resolver

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const DouyinResolverVersion = "1"

type Douyin struct {
	client *SafeClient
	now    func() time.Time
}

func NewDouyin(client *SafeClient) *Douyin { return &Douyin{client: client, now: time.Now} }

var scriptPatterns = []struct {
	name string
	re   *regexp.Regexp
}{
	{"router_data", regexp.MustCompile(`(?s)<script[^>]*>\s*window\._ROUTER_DATA\s*=\s*(\{.*?\})\s*;?\s*</script>`)},
	{"render_data", regexp.MustCompile(`(?s)<script[^>]+id=["']RENDER_DATA["'][^>]*>(.*?)</script>`)},
}

func (d *Douyin) Resolve(ctx context.Context, shareText string) (Work, error) {
	input, err := ExtractInput(shareText)
	if err != nil { return Work{}, err }
	body, finalURL, err := d.client.Get(ctx, input.URL)
	if err != nil { return Work{}, err }
	for _, candidate := range scriptPatterns {
		match := candidate.re.FindSubmatch(body)
		if len(match) != 2 { continue }
		raw := html.UnescapeString(string(match[1]))
		if candidate.name == "render_data" {
			if decoded, decodeErr := url.QueryUnescape(raw); decodeErr == nil { raw = decoded }
		}
		var data any
		if json.Unmarshal([]byte(raw), &data) != nil { continue }
		work, ok := workFromTree(data)
		if !ok { continue }
		work.CanonicalURL = canonicalURL(work.Type, work.DouyinWorkID, finalURL)
		work.ResolverName, work.ResolverVersion, work.ResolvedAt = candidate.name, DouyinResolverVersion, d.now().UTC()
		work.Metadata = map[string]any{"source": candidate.name}
		return work, nil
	}
	return Work{}, ErrResolveFailed
}

func workFromTree(value any) (Work, bool) {
	if object, ok := value.(map[string]any); ok {
		if work, ok := parseWorkObject(object); ok { return work, true }
		for _, child := range object { if work, ok := workFromTree(child); ok { return work, true } }
	}
	if items, ok := value.([]any); ok {
		for _, child := range items { if work, ok := workFromTree(child); ok { return work, true } }
	}
	return Work{}, false
}

func parseWorkObject(object map[string]any) (Work, bool) {
	id := firstString(object, "aweme_id", "awemeId", "itemId")
	if id == "" || !allDigits(id) { return Work{}, false }
	description := firstString(object, "desc", "description")
	work := Work{DouyinWorkID: id, Type: "video", Title: description, Description: description}
	if author := firstMap(object, "author"); author != nil {
		work.AuthorID = firstString(author, "uid", "sec_uid", "secUid", "id")
		work.AuthorName = firstString(author, "nickname", "name")
	}
	work.CoverURL = findURL(firstMap(object, "video"), "cover", "origin_cover", "dynamic_cover")
	work.VideoURL = findURL(firstMap(object, "video"), "play_addr", "playAddr", "download_addr", "downloadAddr")
	if video := firstMap(object, "video"); video != nil {
		work.DurationMS = int64(firstNumber(video, "duration"))
		work.Width, work.Height = int(firstNumber(video, "width")), int(firstNumber(video, "height"))
	}
	images := firstSlice(object, "images", "image_post_info", "imagePostInfo")
	if len(images) > 0 {
		work.Type = "note"
		work.VideoURL = ""
		for _, item := range images {
			if image, ok := item.(map[string]any); ok {
				if nested := firstMap(image, "display_image", "displayImage"); nested != nil { image = nested }
				if imageURL := findURL(image, "url_list", "urlList", "download_url_list", "downloadUrlList"); imageURL != "" {
					work.Images = append(work.Images, Image{URL: imageURL, Width: int(firstNumber(image, "width")), Height: int(firstNumber(image, "height"))})
				}
			}
		}
	}
	if ts := int64(firstNumber(object, "create_time", "createTime")); ts > 0 { value := time.Unix(ts, 0).UTC(); work.PublishedAt = &value }
	work.Hashtags = hashtags(object)
	return work, true
}

func canonicalURL(kind, id string, fallback *url.URL) string {
	if id != "" { return fmt.Sprintf("https://www.douyin.com/%s/%s", kind, id) }
	if fallback == nil { return "" }
	return fallback.String()
}

func firstMap(m map[string]any, keys ...string) map[string]any { for _, key := range keys { if v, ok := m[key].(map[string]any); ok { return v } }; return nil }
func firstSlice(m map[string]any, keys ...string) []any { for _, key := range keys { if v, ok := m[key].([]any); ok { return v }; if v, ok := m[key].(map[string]any); ok { if images := firstSlice(v, "images"); images != nil { return images } } }; return nil }
func firstString(m map[string]any, keys ...string) string { for _, key := range keys { switch v := m[key].(type) { case string: if v != "" { return v }; case json.Number: return v.String(); case float64: return strconv.FormatInt(int64(v), 10) } }; return "" }
func firstNumber(m map[string]any, keys ...string) float64 { for _, key := range keys { switch v := m[key].(type) { case float64: return v; case json.Number: n, _ := v.Float64(); return n } }; return 0 }
func findURL(m map[string]any, keys ...string) string { if m == nil { return "" }; for _, key := range keys { switch v := m[key].(type) { case string: if strings.HasPrefix(v, "http") { return v }; case []any: for _, item := range v { if s, ok := item.(string); ok && strings.HasPrefix(s, "http") { return s } }; case map[string]any: if found := findURL(v, "url_list", "urlList"); found != "" { return found } } }; return "" }
func allDigits(value string) bool { if value == "" { return false }; for _, r := range value { if r < '0' || r > '9' { return false } }; return true }
func hashtags(m map[string]any) []string { var result []string; for _, item := range firstSlice(m, "text_extra", "textExtra") { if value, ok := item.(map[string]any); ok { tag := firstString(value, "hashtag_name", "hashtagName"); if tag != "" { result = append(result, tag) } } }; return result }
