package resolver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const DouyinResolverVersion = "8"

const (
	resolveTimeout   = 25 * time.Second
	sharePageTimeout = 5 * time.Second
	detailAPITimeout = 7 * time.Second
)

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
	resolveCtx, cancel := context.WithTimeout(ctx, resolveTimeout)
	defer cancel()
	work, err := d.resolve(resolveCtx, input)
	if err != nil {
		switch resolveCtx.Err() {
		case context.Canceled:
			return Work{}, context.Canceled
		case context.DeadlineExceeded:
			return Work{}, fmt.Errorf("%w: %w", ErrResolveTimeout, context.DeadlineExceeded)
		}
	}
	return work, err
}

func (d *Douyin) resolve(ctx context.Context, input Input) (Work, error) {
	if ctx.Err() != nil {
		return Work{}, ctx.Err()
	}
	var body []byte
	var finalURL *url.URL
	if input.WorkID == "" {
		// 保留短链跳转的响应，公开页面兜底时无需重复请求短链。
		pageCtx, cancel := context.WithTimeout(ctx, sharePageTimeout)
		var err error
		body, finalURL, err = d.client.Get(pageCtx, input.URL)
		cancel()
		if errors.Is(err, context.DeadlineExceeded) {
			return Work{}, fmt.Errorf("%w: %w", ErrResolveTimeout, err)
		}
		if err != nil {
			return Work{}, err
		}
		if resolved, err := ExtractInput(finalURL.String()); err == nil {
			input.WorkID, input.WorkType = resolved.WorkID, resolved.WorkType
		}
	}
	if input.WorkID != "" {
		detailCtx, cancel := context.WithTimeout(ctx, detailAPITimeout)
		work, ok, err := d.resolveViaDetailAPI(detailCtx, input.WorkID, input.WorkType)
		cancel()
		if ok {
			return work, nil
		}
		if errors.Is(err, ErrURLNotAllowed) {
			return Work{}, err
		}
		logResolveError(ctx, "detail API failed, falling back to public page", err)
	}
	if ctx.Err() != nil {
		return Work{}, ctx.Err()
	}
	pageCtx, cancel := context.WithTimeout(ctx, sharePageTimeout)
	pageWork, pageErr := d.resolveViaPublicPage(pageCtx, input, body, finalURL)
	cancel()
	if input.WorkID == "" || errors.Is(pageErr, ErrURLNotAllowed) || errors.Is(pageErr, ErrWorkUnavailable) || (pageErr == nil && workHasMedia(pageWork)) {
		return pageWork, pageErr
	}
	if ctx.Err() != nil {
		return pageWork, pageErr
	}
	work, err := d.resolveViaBrowser(ctx, input.WorkID, input.WorkType)
	if err == nil {
		return work, nil
	}
	logResolveError(ctx, "browser detail fallback failed", err)
	if pageErr == nil {
		return pageWork, nil
	}
	return Work{}, ErrResolveFailed
}

func (d *Douyin) resolveViaPublicPage(ctx context.Context, input Input, body []byte, finalURL *url.URL) (Work, error) {
	if finalURL == nil {
		var err error
		body, finalURL, err = d.client.Get(ctx, input.URL)
		if err != nil {
			return Work{}, err
		}
	}
	pageWork, found := d.parsePublicPage(body, finalURL, input.WorkID)
	if found && workHasMedia(pageWork) {
		return pageWork, nil
	}
	if input.WorkID != "" {
		kind := "video"
		if input.WorkType == "note" {
			kind = "note"
		}
		sharePath := "/share/" + kind + "/" + input.WorkID
		host := strings.TrimSuffix(strings.ToLower(finalURL.Hostname()), ".")
		alreadyReadSharePage := (host == "iesdouyin.com" || host == "www.iesdouyin.com") && strings.TrimRight(finalURL.Path, "/") == sharePath
		if !alreadyReadSharePage {
			shareURL, _ := url.Parse("https://www.iesdouyin.com" + sharePath)
			if shareBody, shareFinalURL, shareErr := d.client.Get(ctx, shareURL); shareErr == nil {
				if work, ok := d.parsePublicPage(shareBody, shareFinalURL, input.WorkID); ok && (workHasMedia(work) || !found) {
					return work, nil
				}
			} else if errors.Is(shareErr, ErrURLNotAllowed) {
				return Work{}, shareErr
			}
		}
	}
	if found {
		return pageWork, nil
	}
	return Work{}, ErrResolveFailed
}

func (d *Douyin) parsePublicPage(body []byte, finalURL *url.URL, workID string) (Work, bool) {
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
		decoder := json.NewDecoder(strings.NewReader(raw))
		decoder.UseNumber()
		if decoder.Decode(&data) != nil || decoder.Decode(&struct{}{}) != io.EOF {
			continue
		}
		work, ok := workFromRouterData(data)
		if !ok {
			// 兜底：定向路径未命中时回退整树查找，兼容 RENDER_DATA 旧结构与结构漂移。
			work, ok = workFromTree(data)
		}
		if !ok || isEmptyShell(work) || (workID != "" && workID != work.DouyinWorkID) {
			continue
		}
		work.VideoURL = withoutWatermark(work.VideoURL)
		work.VideoURLCandidates = uniqueMediaURLs(work.VideoURLCandidates, true)
		work.CanonicalURL = canonicalURL(work.Type, work.DouyinWorkID, finalURL)
		work.ResolverName, work.ResolverVersion, work.ResolvedAt = candidate.name, DouyinResolverVersion, d.now().UTC()
		work.Metadata = map[string]any{"source": candidate.name}
		return work, true
	}
	return Work{}, false
}

// isEmptyShell 判断 Work 是否只是含 ID 的空壳（无标题/作者/视频/图片/封面）。
func isEmptyShell(w Work) bool {
	return w.Title == "" && w.AuthorName == "" && w.VideoURL == "" && len(w.Images) == 0 && w.CoverURL == ""
}

// resolveViaDetailAPI 调用 aweme/v1/web/aweme/detail 获取作品数据。
// 返回 (work, true, nil) 表示成功；(_, false, err) 表示该路径不可用。
func (d *Douyin) resolveViaDetailAPI(ctx context.Context, workID, workType string) (Work, bool, error) {
	if d.ttwid == nil {
		return Work{}, false, fmt.Errorf("ttwid manager not configured")
	}
	// 图文/动图作品使用 /note/ 应用页；其余按视频处理。引导 ttwid 与 Referer
	// 都应指向对应的应用页（首页不生成 ttwid，且需与作品类型一致）。
	kind := "video"
	if workType == "note" {
		kind = "note"
	}
	pageURL := "https://www.douyin.com/" + kind + "/" + workID
	ttwidValue, err := d.ttwid.Get(ctx, pageURL)
	if err != nil {
		return Work{}, false, fmt.Errorf("ttwid unavailable: %w", err)
	}
	headers := map[string]string{
		"User-Agent":      "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36",
		"Referer":         pageURL,
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
		if attempt > 0 && !waitForContext(ctx, 400*time.Millisecond) {
			return Work{}, false, ctx.Err()
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
			if ctx.Err() != nil || errors.Is(reqErr, ErrURLNotAllowed) || errors.Is(reqErr, ErrWorkUnavailable) {
				return Work{}, false, reqErr
			}
			lastErr = reqErr
			continue
		}
		work, parseErr := d.workFromDetailBody(body, "aweme_detail")
		if parseErr != nil {
			lastErr = parseErr
			continue
		}
		if work.DouyinWorkID != workID || isEmptyShell(work) {
			return Work{}, false, ErrResolveFailed
		}
		return work, true, nil
	}
	return Work{}, false, lastErr
}

func (d *Douyin) resolveViaBrowser(ctx context.Context, workID, workType string) (Work, error) {
	if d.ttwid == nil {
		return Work{}, fmt.Errorf("%w: browser session not configured", ErrResolveFailed)
	}
	kind := "video"
	if workType == "note" {
		kind = "note"
	}
	pageURL := "https://www.douyin.com/" + kind + "/" + workID
	body, err := d.ttwid.FetchDetail(ctx, pageURL, workID)
	if err != nil {
		return Work{}, err
	}
	work, err := d.workFromDetailBody(body, "browser_detail")
	if err != nil {
		return Work{}, err
	}
	if work.DouyinWorkID != workID || !workHasMedia(work) {
		return Work{}, fmt.Errorf("%w: browser returned no media for requested work", ErrResolveFailed)
	}
	return work, nil
}

func (d *Douyin) workFromDetailBody(body []byte, source string) (Work, error) {
	var payload struct {
		StatusCode  int            `json:"status_code"`
		AwemeDetail map[string]any `json:"aweme_detail"`
		StatusMsg   string         `json:"status_msg"`
	}
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil {
		return Work{}, fmt.Errorf("%w: invalid detail response: %v", ErrResolveFailed, err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return Work{}, fmt.Errorf("%w: invalid detail response: trailing data", ErrResolveFailed)
	}
	if payload.StatusCode != 0 || payload.AwemeDetail == nil {
		msg := payload.StatusMsg
		if payload.StatusCode == 403 {
			msg = "被抖音风控拦截"
		}
		return Work{}, fmt.Errorf("%w: detail API status %d: %s", ErrResolveFailed, payload.StatusCode, msg)
	}
	work, ok := parseWorkObject(payload.AwemeDetail)
	if !ok {
		return Work{}, ErrResolveFailed
	}
	work.VideoURL = withoutWatermark(work.VideoURL)
	work.VideoURLCandidates = uniqueMediaURLs(work.VideoURLCandidates, true)
	work.CanonicalURL = canonicalURL(work.Type, work.DouyinWorkID, nil)
	work.ResolverName, work.ResolverVersion, work.ResolvedAt = source, DouyinResolverVersion, d.now().UTC()
	work.Metadata = map[string]any{"source": source}
	return work, nil
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
	if author := firstMap(object, "author", "authorInfo"); author != nil {
		work.AuthorID = firstString(author, "uid", "sec_uid", "secUid", "id")
		work.AuthorName = firstString(author, "nickname", "name")
	}
	video := firstMap(object, "video")
	work.CoverURLCandidates = uniqueMediaURLs(append(videoCoverURLCandidates(video), videoCoverURLCandidates(object)...), false)
	work.CoverURL = firstMediaURL(work.CoverURLCandidates)
	work.VideoURLCandidates = videoURLCandidates(video)
	work.VideoURL = firstMediaURL(work.VideoURLCandidates)
	if video != nil {
		work.DurationMS = int64(firstNumber(video, "duration"))
		work.Width, work.Height = int(firstNumber(video, "width")), int(firstNumber(video, "height"))
	}
	images := firstSlice(object, "images", "image_post_info", "imagePostInfo")
	if len(images) > 0 {
		work.Type = "note"
		work.VideoURL = ""
		work.VideoURLCandidates = nil
		for _, item := range images {
			if image, ok := item.(map[string]any); ok {
				// 动图（live photo）的动态版 MP4 在 images[i].video，是 display_image 的兄弟节点，
				// 必须先保留原 map 引用，rebind 到 display_image 后再取 video 会永远取不到。
				source := image
				imageURLs := mediaURLCandidates(image, "url_list", "urlList", "download_url_list", "downloadUrlList")
				if nested := firstMap(image, "display_image", "displayImage"); nested != nil {
					image = nested
					imageURLs = uniqueMediaURLs(append(mediaURLCandidates(image, "url_list", "urlList", "download_url_list", "downloadUrlList"), imageURLs...), false)
				}
				if imageURL := firstMediaURL(imageURLs); imageURL != "" {
					entry := Image{URL: imageURL, URLCandidates: imageURLs, Width: int(firstNumber(image, "width")), Height: int(firstNumber(image, "height"))}
					// 优先取 CDN play_addr（直接文件、可靠），download_addr 是会 302 的签名接口。
					// 动图动态版 MP4 通常位于 images[i].video（display_image 的兄弟节点）；个别接口变体会把
					// video 嵌进 display_image 内部，两种形状都兜底取用，避免漏掉 AnimatedURL。
					video := firstMap(source, "video")
					if video == nil {
						video = firstMap(image, "video")
					}
					if video != nil {
						entry.AnimatedURLCandidates = uniqueMediaURLs(videoURLCandidates(video), true)
						entry.AnimatedURL = firstMediaURL(entry.AnimatedURLCandidates)
					}
					work.Images = append(work.Images, entry)
				}
			}
		}
	}
	if ts := int64(firstNumber(object, "create_time", "createTime")); ts > 0 {
		value := time.Unix(ts, 0).UTC()
		work.PublishedAt = &value
	}
	work.Hashtags = hashtags(object)
	// 作品背景音乐：music.play_url.url_list[0] + title + artists[0].name。
	// 图文/动图作品通常配有一首背景音乐，查看时随画面播放。
	if music := firstMap(object, "music"); music != nil {
		work.MusicURLCandidates = mediaURLCandidates(music, "play_url", "playUrl")
		work.MusicURL = firstMediaURL(work.MusicURLCandidates)
		work.MusicTitle = firstString(music, "title")
		if artists := firstSlice(music, "artists"); len(artists) > 0 {
			if artist, ok := artists[0].(map[string]any); ok {
				work.MusicArtist = firstString(artist, "name")
			}
		}
		if work.MusicArtist == "" {
			work.MusicArtist = firstString(music, "author", "ownerNickname")
		}
		if work.MusicArtist == "" {
			if author := firstMap(music, "author"); author != nil {
				work.MusicArtist = firstString(author, "nickname", "name")
			}
		}
	}
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
