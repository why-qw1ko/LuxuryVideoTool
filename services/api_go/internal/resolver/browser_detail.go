package resolver

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)

const browserDetailTimeout = 45 * time.Second

const browserDetailCaptureScript = `(() => {
	window.__douyinDetailResponse = null;
	const workID = %s;
	const matches = rawURL => {
		try {
			const parsed = new URL(String(rawURL), location.href);
			return parsed.pathname.includes("/aweme/v1/web/aweme/detail/") &&
				parsed.searchParams.get("aweme_id") === workID;
		} catch (_) {
			return false;
		}
	};
	const remember = (status, body) => {
		if (status >= 200 && status < 300 && typeof body === "string" && body.includes("\"aweme_detail\"")) {
			window.__douyinDetailResponse = {status, body};
		}
	};

	const originalOpen = XMLHttpRequest.prototype.open;
	const originalSend = XMLHttpRequest.prototype.send;
	XMLHttpRequest.prototype.open = function(method, url) {
		this.__douyinURL = url;
		return originalOpen.apply(this, arguments);
	};
	XMLHttpRequest.prototype.send = function() {
		if (matches(this.__douyinURL)) {
			this.addEventListener("loadend", () => {
				let body = "";
				try {
					body = this.responseText;
				} catch (_) {
					try {
						body = JSON.stringify(this.response);
					} catch (_) {}
				}
				remember(this.status, body);
			});
		}
		return originalSend.apply(this, arguments);
	};

	const originalFetch = window.fetch;
	window.fetch = async function(input, init) {
		const url = typeof input === "string" ? input : input && input.url;
		const response = await originalFetch.apply(this, arguments);
		if (matches(url)) {
			response.clone().text().then(body => remember(response.status, body)).catch(() => {});
		}
		return response;
	};
})();`

// FetchDetail 让真实浏览器执行抖音当前版本的前端签名逻辑，并从详情接口响应中取回 JSON。
// 原生的 a_bogus 实现在抖音更新参数与安全头后会失效，此路径作为详情 API 的兼容兜底。
func (m *TtwidManager) FetchDetail(ctx context.Context, pageURL, workID string) ([]byte, error) {
	m.browserMu.Lock()
	defer m.browserMu.Unlock()

	browserCtx, cancel := context.WithTimeout(ctx, browserDetailTimeout+15*time.Second)
	defer cancel()
	browser, err := m.launchHeadlessBrowser(browserCtx)
	if err != nil {
		return nil, err
	}
	defer browser.Close()

	conn := browser.conn
	if _, err := conn.Call("Network.enable", nil, 5*time.Second); err != nil {
		return nil, err
	}
	if _, err := conn.Call("Page.enable", nil, 5*time.Second); err != nil {
		return nil, err
	}
	if _, err := conn.Call("Runtime.enable", nil, 5*time.Second); err != nil {
		return nil, err
	}
	script := fmt.Sprintf(browserDetailCaptureScript, strconv.Quote(workID))
	if _, err := conn.Call("Page.addScriptToEvaluateOnNewDocument", map[string]any{"source": script}, 5*time.Second); err != nil {
		return nil, err
	}
	if _, err := conn.Call("Page.navigate", map[string]any{"url": pageURL}, 10*time.Second); err != nil {
		return nil, err
	}

	deadline := time.Now().Add(browserDetailTimeout)
	for time.Now().Before(deadline) {
		select {
		case <-browserCtx.Done():
			return nil, browserCtx.Err()
		default:
		}
		res, err := conn.Call("Runtime.evaluate", map[string]any{
			"expression":    "JSON.stringify(window.__douyinDetailResponse || null)",
			"returnByValue": true,
		}, 10*time.Second)
		if err == nil {
			var evaluated struct {
				Result struct {
					Value string `json:"value"`
				} `json:"result"`
			}
			if json.Unmarshal(res, &evaluated) == nil && evaluated.Result.Value != "" && evaluated.Result.Value != "null" {
				var captured struct {
					Status int    `json:"status"`
					Body   string `json:"body"`
				}
				if json.Unmarshal([]byte(evaluated.Result.Value), &captured) == nil {
					if captured.Status < 200 || captured.Status >= 300 || captured.Body == "" {
						return nil, fmt.Errorf("%w: browser detail API status %d", ErrResolveFailed, captured.Status)
					}
					return []byte(captured.Body), nil
				}
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	return nil, fmt.Errorf("%w: browser detail API response timed out", ErrResolveFailed)
}
