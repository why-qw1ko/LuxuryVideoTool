package resolver

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const DouyinResolverVersion = "3"

type Douyin struct {
	client *SafeClient
	ttwid  *TtwidManager
	now    func() time.Time
}

func NewDouyin(client *SafeClient, ttwid *TtwidManager) *Douyin {
	return &Douyin{client: client, ttwid: ttwid, now: time.Now}
}

var scriptPatterns = []struct {
	name string
	re   *regexp.Regexp
}{
	{"router_data", regexp.MustCompile(`(?s)<script[^>]*>\s*window\._ROUTER_DATA\s*=\s*(\{.*?\})\s*;?\s*</script>`)},
	{"render_data", regexp.MustCompile(`(?s)<script[^>]+id=["']RENDER_DATA["'][^>]*>(.*?)</script>`)},
}

func (d *Douyin) Resolve(ctx context.Context, shareText string) (Work, error) {
	input, err := ExtractInput(shareText)
	if err != nil {
		return Work{}, err
	}

	// 主路径：aweme/v1/web/aweme/detail 详情 API（a_bogus 签名 + ttwid cookie）。
	// 抖音已全局移除分享页 SSR 内嵌数据（videoInfoRes），这是当前唯一稳定取数方式。
	workID := input.WorkID
	if workID == "" {
		// 短链（v.douyin.com/xxx）需先跟随跳转拿到作品 ID。
		if _, finalURL, redirectErr := d.client.Get(ctx, input.URL); redirectErr == nil {
			if resolvedInput, parseErr := ExtractInput(finalURL.String()); parseErr == nil && resolvedInput.WorkID != "" {
				workID = resolvedInput.WorkID
			}
		}
	}
	var detailErr error
	if workID != "" {
		var work Work
		var ok bool
		work, ok, detailErr = d.resolveViaDetailAPI(ctx, workID)
		if ok {
			return work, nil
		}
		logResolveError(ctx, "detail API failed, falling back to share-page parse", detailErr)
	}

	body, finalURL, err := d.client.Get(ctx, input.URL)
	if err != nil {
		return Work{}, err
	}
	if resolvedInput, inputErr := ExtractInput(finalURL.String()); inputErr == nil && resolvedInput.WorkID != "" {
		shareURL, _ := url.Parse("https://www.iesdouyin.com/share/video/" + resolvedInput.WorkID)
		if shareBody, shareFinalURL, shareErr := d.client.Get(ctx, shareURL); shareErr == nil {
			body, finalURL = shareBody, shareFinalURL
		}
	}
	for _, candidate := range scriptPatterns {
		match := candidate.re.FindSubmatch(body)
		if len(match) != 2 {
			continue
		}
		raw := html.UnescapeString(string(match[1]))
		if candidate.name == "render_data" {
			if decoded, decodeErr := url.QueryUnescape(raw); decodeErr == nil {
				raw = decoded
			}
		}
		var data any
		if json.Unmarshal([]byte(raw), &data) != nil {
			continue
		}
		work, ok := workFromRouterData(data)
		if !ok {
			// 兜底：定向路径未命中时回退整树查找，兼容 RENDER_DATA 旧结构与结构漂移。
			work, ok = workFromTree(data)
		}
		if !ok {
			continue
		}
		// 拒绝空壳：抖音已移除 SSR 数据，整树查找会命中仅含 itemId 的页面级对象，
		// 返回标题/视频/作者全空的作品。这种空壳必须丢弃，否则会污染解析缓存。
		if isEmptyShell(work) {
			continue
		}
		work.VideoURL = withoutWatermark(work.VideoURL)
		work.CanonicalURL = canonicalURL(work.Type, work.DouyinWorkID, finalURL)
		work.ResolverName, work.ResolverVersion, work.ResolvedAt = candidate.name, DouyinResolverVersion, d.now().UTC()
		work.Metadata = map[string]any{"source": candidate.name}
		return work, nil
	}
	if detailErr != nil {
		// 详情 API 有具体错误时优先返回它，便于用户定位（风控/ttwid 等问题）。
		return Work{}, fmt.Errorf("%w: %v", ErrResolveFailed, detailErr)
	}
	return Work{}, ErrResolveFailed
}

// isEmptyShell 判断 Work 是否只是含 ID 的空壳（无标题/作者/视频/图片/封面）。
func isEmptyShell(w Work) bool {
	return w.Title == "" && w.AuthorName == "" && w.VideoURL == "" && len(w.Images) == 0 && w.CoverURL == ""
}

// resolveViaDetailAPI 调用 aweme/v1/web/aweme/detail 获取作品数据。
// 返回 (work, true, nil) 表示成功；(_, false, err) 表示该路径不可用。
func (d *Douyin) resolveViaDetailAPI(ctx context.Context, workID string) (Work, bool, error) {
	if d.ttwid == nil {
		return Work{}, false, fmt.Errorf("ttwid manager not configured")
	}
	// 引导 ttwid 时需要导航到作品页（首页不生成 ttwid）。
	ttwidValue, err := d.ttwid.Get(ctx, "https://www.douyin.com/video/"+workID)
	if err != nil {
		return Work{}, false, fmt.Errorf("ttwid unavailable: %w", err)
	}
	headers := map[string]string{
		"User-Agent":      "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36",
		"Referer":         "https://www.douyin.com/video/" + workID,
		"Accept":          "application/json, text/plain, */*",
		"Accept-Language": "zh-CN,zh;q=0.9",
		"Cookie":          "ttwid=" + ttwidValue,
		"Sec-Fetch-Site":  "same-origin",
		"Sec-Fetch-Mode":  "cors",
		"Sec-Fetch-Dest":  "empty",
	}

	// 详情 API 偶发空响应 / 403（限流或软拦截），用新签名重试数次；
	// 每次调用都会生成新的 a_bogus（含新时间戳与随机浏览器信息）。
	const maxDetailAttempts = 3
	var lastErr error
	for attempt := 0; attempt < maxDetailAttempts; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return Work{}, false, ctx.Err()
			default:
			}
			time.Sleep(400 * time.Millisecond)
		}
		_, fullURL, signErr := signAwemeDetailURL(workID)
		if signErr != nil {
			return Work{}, false, signErr
		}
		target, parseErr := url.Parse(fullURL)
		if parseErr != nil {
			return Work{}, false, parseErr
		}
		body, _, reqErr := d.client.Request(ctx, http.MethodGet, target, headers, nil)
		if reqErr != nil {
			lastErr = reqErr
			continue
		}
		var payload struct {
			StatusCode  int            `json:"status_code"`
			AwemeDetail map[string]any `json:"aweme_detail"`
			StatusMsg   string         `json:"status_msg"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			lastErr = err
			continue
		}
		if payload.StatusCode != 0 || payload.AwemeDetail == nil {
			msg := payload.StatusMsg
			if payload.StatusCode == 403 {
				msg = "被抖音风控拦截（可能 ttwid 失效，会自动续期后重试）"
			}
			lastErr = fmt.Errorf("detail API status %d: %s", payload.StatusCode, msg)
			continue
		}
		work, ok := parseWorkObject(payload.AwemeDetail)
		if !ok {
			return Work{}, false, ErrResolveFailed
		}
		work.VideoURL = withoutWatermark(work.VideoURL)
		work.CanonicalURL = canonicalURL(work.Type, work.DouyinWorkID, nil)
		work.ResolverName, work.ResolverVersion, work.ResolvedAt = "aweme_detail", DouyinResolverVersion, d.now().UTC()
		work.Metadata = map[string]any{"source": "aweme_detail"}
		return work, true, nil
	}
	return Work{}, false, lastErr
}

func workFromTree(value any) (Work, bool) {
	if object, ok := value.(map[string]any); ok {
		if work, ok := parseWorkObject(object); ok {
			return work, true
		}
		for _, child := range object {
			if work, ok := workFromTree(child); ok {
				return work, true
			}
		}
	}
	if items, ok := value.([]any); ok {
		for _, child := range items {
			if work, ok := workFromTree(child); ok {
				return work, true
			}
		}
	}
	return Work{}, false
}

// workFromRouterData 沿 loaderData["video_(id)/page"|"note_(id)/page"] -> videoInfoRes -> item_list[0]
// 定向定位当前作品。整树查找 workFromTree 可能先命中页面级对象里仅含 itemId 的空壳，
// 导致返回只有 ID、其余字段全空的 Work，因此优先走这条精确路径。
func workFromRouterData(value any) (Work, bool) {
	root, ok := value.(map[string]any)
	if !ok {
		return Work{}, false
	}
	loader, ok := root["loaderData"].(map[string]any)
	if !ok {
		return Work{}, false
	}
	for _, route := range []string{"video_(id)/page", "note_(id)/page"} {
		page, ok := loader[route].(map[string]any)
		if !ok {
			continue
		}
		info, ok := page["videoInfoRes"].(map[string]any)
		if !ok {
			continue
		}
		items, ok := info["item_list"].([]any)
		if !ok || len(items) == 0 {
			continue
		}
		if object, ok := items[0].(map[string]any); ok {
			if work, ok := parseWorkObject(object); ok {
				return work, true
			}
		}
	}
	return Work{}, false
}

func parseWorkObject(object map[string]any) (Work, bool) {
	id := firstString(object, "aweme_id", "awemeId", "itemId")
	if id == "" || !allDigits(id) {
		return Work{}, false
	}
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
				if nested := firstMap(image, "display_image", "displayImage"); nested != nil {
					image = nested
				}
				if imageURL := findURL(image, "url_list", "urlList", "download_url_list", "downloadUrlList"); imageURL != "" {
					work.Images = append(work.Images, Image{URL: imageURL, Width: int(firstNumber(image, "width")), Height: int(firstNumber(image, "height"))})
				}
			}
		}
	}
	if ts := int64(firstNumber(object, "create_time", "createTime")); ts > 0 {
		value := time.Unix(ts, 0).UTC()
		work.PublishedAt = &value
	}
	work.Hashtags = hashtags(object)
	return work, true
}

func canonicalURL(kind, id string, fallback *url.URL) string {
	if id != "" {
		return fmt.Sprintf("https://www.douyin.com/%s/%s", kind, id)
	}
	if fallback == nil {
		return ""
	}
	return fallback.String()
}

func firstMap(m map[string]any, keys ...string) map[string]any {
	for _, key := range keys {
		if v, ok := m[key].(map[string]any); ok {
			return v
		}
	}
	return nil
}
func firstSlice(m map[string]any, keys ...string) []any {
	for _, key := range keys {
		if v, ok := m[key].([]any); ok {
			return v
		}
		if v, ok := m[key].(map[string]any); ok {
			if images := firstSlice(v, "images"); images != nil {
				return images
			}
		}
	}
	return nil
}
func firstString(m map[string]any, keys ...string) string {
	for _, key := range keys {
		switch v := m[key].(type) {
		case string:
			if v != "" {
				return v
			}
		case json.Number:
			return v.String()
		case float64:
			return strconv.FormatInt(int64(v), 10)
		}
	}
	return ""
}
func firstNumber(m map[string]any, keys ...string) float64 {
	for _, key := range keys {
		switch v := m[key].(type) {
		case float64:
			return v
		case json.Number:
			n, _ := v.Float64()
			return n
		}
	}
	return 0
}
func findURL(m map[string]any, keys ...string) string {
	if m == nil {
		return ""
	}
	for _, key := range keys {
		switch v := m[key].(type) {
		case string:
			if strings.HasPrefix(v, "http") {
				return v
			}
		case []any:
			for _, item := range v {
				if s, ok := item.(string); ok && strings.HasPrefix(s, "http") {
					return s
				}
			}
		case map[string]any:
			if found := findURL(v, "url_list", "urlList"); found != "" {
				return found
			}
		}
	}
	return ""
}
func withoutWatermark(value string) string { return strings.Replace(value, "playwm", "play", 1) }
func allDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
func hashtags(m map[string]any) []string {
	var result []string
	for _, item := range firstSlice(m, "text_extra", "textExtra") {
		if value, ok := item.(map[string]any); ok {
			tag := firstString(value, "hashtag_name", "hashtagName")
			if tag != "" {
				result = append(result, tag)
			}
		}
	}
	return result
}

func logResolveError(ctx context.Context, msg string, err error) {
	slog.LogAttrs(ctx, slog.LevelWarn, msg, slog.Any("error", err))
}
