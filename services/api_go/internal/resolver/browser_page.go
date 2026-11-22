package resolver

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

const maxBrowserPageBytes = 8 << 20

var pacePushPattern = regexp.MustCompile(`(?s)^self\.__pace_f\.push\((\[.*\])\)\s*;?$`)

// detailFromPageScripts 读取页面已经公开的 JSON 与 PACE Flight 数据，不执行脚本。
// 同时保留旧版 RENDER_DATA / _ROUTER_DATA，且只接受包含目标 ID 和媒体的作品。
func detailFromPageScripts(scripts []string, workID string) ([]byte, error) {
	if !allDigits(workID) {
		return nil, ErrInvalidShareLink
	}
	total := 0
	for _, script := range scripts {
		if len(script) > maxBrowserPageBytes-total {
			return nil, fmt.Errorf("%w: embedded page data too large", ErrResolveFailed)
		}
		total += len(script)
	}
	var chunks []string
	for _, script := range scripts {
		raw := strings.TrimSpace(script)
		if match := pacePushPattern.FindStringSubmatch(raw); len(match) == 2 {
			var entry []json.RawMessage
			if json.Unmarshal([]byte(match[1]), &entry) != nil || len(entry) != 2 || string(entry[0]) != "1" {
				continue
			}
			var chunk string
			if json.Unmarshal(entry[1], &chunk) == nil {
				chunks = append(chunks, chunk)
			}
			continue
		}
		if strings.HasPrefix(raw, "window._ROUTER_DATA") {
			rest := strings.TrimSpace(strings.TrimPrefix(raw, "window._ROUTER_DATA"))
			if !strings.HasPrefix(rest, "=") {
				continue
			}
			raw = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(rest[1:]), ";"))
		}
		if strings.HasPrefix(raw, "%7B") || strings.HasPrefix(raw, "%7b") || strings.HasPrefix(raw, "%5B") || strings.HasPrefix(raw, "%5b") {
			if decoded, err := url.PathUnescape(raw); err == nil {
				raw = decoded
			}
		}
		if detail := detailFromPageJSON(raw, workID); detail != nil {
			return detail, nil
		}
	}
	// 页面既可能每次 push 一整条记录，也可能把同一条 JSON 记录拆成多次 push。
	// 单独读取完整块，避免 app 配置等非 Flight 块干扰；再拼接处理跨块记录。
	for _, chunk := range append(chunks, strings.Join(chunks, "")) {
		for _, line := range strings.Split(chunk, "\n") {
			recordID, raw, ok := strings.Cut(line, ":")
			if !ok || !flightRecordID(recordID) {
				continue
			}
			if detail := detailFromPageJSON(raw, workID); detail != nil {
				return detail, nil
			}
		}
	}
	return nil, fmt.Errorf("%w: embedded page data has no media for requested work", ErrResolveFailed)
}

func flightRecordID(value string) bool {
	if value == "" || len(value) > 16 {
		return false
	}
	for _, r := range value {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
			return false
		}
	}
	return true
}

func detailFromPageJSON(raw, workID string) []byte {
	if !strings.Contains(raw, workID) || !json.Valid([]byte(raw)) {
		return nil
	}
	var value any
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	if decoder.Decode(&value) != nil {
		return nil
	}
	remaining := 100000
	object := pageWorkObject(value, workID, 0, &remaining)
	if object == nil {
		return nil
	}
	body, err := json.Marshal(map[string]any{"status_code": 0, "aweme_detail": object})
	if err != nil {
		return nil
	}
	return body
}

func pageWorkObject(value any, workID string, depth int, remaining *int) map[string]any {
	if depth > 64 || *remaining <= 0 {
		return nil
	}
	(*remaining)--
	switch value := value.(type) {
	case map[string]any:
		if firstString(value, "aweme_id", "awemeId", "itemId") == workID {
			if work, ok := parseWorkObject(value); ok && workHasMedia(work) {
				return value
			}
		}
		for _, child := range value {
			if object := pageWorkObject(child, workID, depth+1, remaining); object != nil {
				return object
			}
		}
	case []any:
		for _, child := range value {
			if object := pageWorkObject(child, workID, depth+1, remaining); object != nil {
				return object
			}
		}
	}
	return nil
}

func workHasMedia(work Work) bool {
	if work.Type == "note" {
		return len(work.Images) > 0
	}
	return work.VideoURL != ""
}
