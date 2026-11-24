package resolver

import (
	"net/url"
	"strings"
)

// mediaURLCandidates 只读取媒体地址字段，保留来源顺序且不沿无关字段搜索 URL。
func mediaURLCandidates(object map[string]any, keys ...string) []string {
	var candidates []string
	seen := make(map[string]bool)
	var collect func(any)
	add := func(raw string) {
		raw = strings.TrimSpace(raw)
		target, err := url.Parse(raw)
		if err != nil || !strings.EqualFold(target.Scheme, "https") || target.Hostname() == "" || target.User != nil || seen[raw] {
			return
		}
		seen[raw] = true
		candidates = append(candidates, raw)
	}
	collect = func(value any) {
		switch value := value.(type) {
		case string:
			add(value)
		case []string:
			for _, item := range value {
				add(item)
			}
		case []any:
			for _, item := range value {
				if source, ok := item.(map[string]any); ok {
					collect(source["src"])
				} else if raw, ok := item.(string); ok {
					add(raw)
				}
			}
		case map[string]any:
			collect(value["url_list"])
			collect(value["urlList"])
		}
	}
	for _, key := range keys {
		collect(object[key])
	}
	return candidates
}

func videoCoverURLCandidates(object map[string]any) []string {
	var candidates []string
	for _, key := range []string{"cover", "origin_cover", "originCover", "cover_url", "coverUrl",
		"cover_url_list", "coverUrlList", "origin_cover_url_list", "originCoverUrlList", "dynamic_cover", "dynamicCover"} {
		candidates = append(candidates, mediaURLCandidates(object, key)...)
		if source := firstMap(object, key); source != nil {
			candidates = append(candidates, mediaURLCandidates(source, "src", "url")...)
		}
	}
	return candidates
}

// videoURLCandidates 保持现有顶层播放地址优先，补充码率播放地址，最后才用下载地址。
func videoURLCandidates(video map[string]any) []string {
	candidates := mediaURLCandidates(video, "play_addr", "playAddr")
	for _, key := range []string{"bit_rate", "bitRate"} {
		for _, item := range firstSlice(video, key) {
			if rate, ok := item.(map[string]any); ok {
				candidates = append(candidates, mediaURLCandidates(rate, "play_addr", "playAddr")...)
			}
		}
	}
	candidates = append(candidates, mediaURLCandidates(video, "download_addr", "downloadAddr")...)
	return uniqueMediaURLs(candidates, false)
}

func uniqueMediaURLs(candidates []string, removeWatermark bool) []string {
	var result []string
	seen := make(map[string]bool)
	for _, candidate := range candidates {
		if removeWatermark {
			candidate = withoutWatermark(candidate)
		}
		if !seen[candidate] {
			seen[candidate] = true
			result = append(result, candidate)
		}
	}
	return result
}

func firstMediaURL(candidates []string) string {
	if len(candidates) == 0 {
		return ""
	}
	return candidates[0]
}
