package resolver

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const browserDetailTimeout = 17 * time.Second

const browserDetailCaptureScript = `(() => {
	window.__douyinDetailResponse = null;
	window.__douyinDetailLastResponse = null;
	window.__douyinDetailRequests = 0;
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
		window.__douyinDetailLastResponse = {status, length: typeof body === "string" ? body.length : 0};
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
			window.__douyinDetailRequests++;
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
		const url = typeof input === "string" ? input : input instanceof URL ? input.href : input && input.url;
		const matched = matches(url);
		if (matched) window.__douyinDetailRequests++;
		const response = await originalFetch.apply(this, arguments);
		if (matched) {
			response.clone().text().then(body => remember(response.status, body)).catch(() => {});
		}
		return response;
	};
})();`

// 保留原始脚本文本供 Go 解码，避免 JavaScript 把 19 位数字 ID 转成浮点数。
const browserDetailSnapshotScript = `JSON.stringify((() => {
	const scripts = [];
	let size = 0;
	let oversized = false;
	for (const script of document.scripts) {
		const text = script.textContent || "";
		if (script.id !== "RENDER_DATA" && !/^\s*(?:self\.__pace_f\.push\(|window\._ROUTER_DATA\s*=)/.test(text)) continue;
		size += text.length;
		if (size > 8 * 1024 * 1024) {
			oversized = true;
			break;
		}
		scripts.push(text);
	}
	return {
		detail: window.__douyinDetailResponse || null,
		lastResponse: window.__douyinDetailLastResponse || null,
		requests: window.__douyinDetailRequests || 0,
		state: document.readyState,
		scripts,
		oversized
	};
})())`

type browserDetailSnapshot struct {
	Detail *struct {
		Status int    `json:"status"`
		Body   string `json:"body"`
	} `json:"detail"`
	LastResponse *struct {
		Status int `json:"status"`
		Length int `json:"length"`
	} `json:"lastResponse"`
	Requests  int      `json:"requests"`
	State     string   `json:"state"`
	Scripts   []string `json:"scripts"`
	Oversized bool     `json:"oversized"`
}

// FetchDetail 同时读取浏览器的详情响应和公开页面内嵌数据。
// 服务端直接渲染的页面可能完全不请求详情接口，不能仅等待 XHR / fetch。
func (m *TtwidManager) FetchDetail(ctx context.Context, pageURL, workID string) ([]byte, error) {
	browserCtx, cancel := context.WithTimeout(ctx, browserDetailTimeout)
	defer cancel()
	if err := takeGate(browserCtx, m.browserGate); err != nil {
		return nil, err
	}
	defer func() { <-m.browserGate }()
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
	navigation, err := conn.Call("Page.navigate", map[string]any{"url": pageURL}, 10*time.Second)
	if err != nil {
		return nil, err
	}
	var navigated struct {
		ErrorText string `json:"errorText"`
	}
	if err := json.Unmarshal(navigation, &navigated); err != nil {
		return nil, fmt.Errorf("%w: invalid browser navigation result", ErrResolveFailed)
	}
	if navigated.ErrorText != "" {
		return nil, fmt.Errorf("%w: browser navigation failed: %s", ErrResolveFailed, navigated.ErrorText)
	}

	var snapshot browserDetailSnapshot
	for browserCtx.Err() == nil {
		res, err := conn.Call("Runtime.evaluate", map[string]any{
			"expression":    browserDetailSnapshotScript,
			"returnByValue": true,
		}, 10*time.Second)
		if err != nil && !isBrowserNavigationContextError(err) {
			return nil, fmt.Errorf("%w: browser page read failed: %v", ErrResolveFailed, err)
		}
		if err == nil {
			var evaluated struct {
				Result struct {
					Value string `json:"value"`
				} `json:"result"`
			}
			if json.Unmarshal(res, &evaluated) == nil && evaluated.Result.Value != "" && evaluated.Result.Value != "null" {
				if json.Unmarshal([]byte(evaluated.Result.Value), &snapshot) == nil {
					if snapshot.Detail != nil && snapshot.Detail.Status >= 200 && snapshot.Detail.Status < 300 {
						body := []byte(snapshot.Detail.Body)
						work, parseErr := NewDouyin(nil, nil).workFromDetailBody(body, "browser_detail")
						if parseErr == nil && work.DouyinWorkID == workID && workHasMedia(work) {
							return body, nil
						}
					}
					if body, pageErr := detailFromPageScripts(snapshot.Scripts, workID); pageErr == nil {
						return body, nil
					}
				}
			}
		}
		if !waitForContext(browserCtx, 500*time.Millisecond) {
			break
		}
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if snapshot.LastResponse != nil {
		return nil, fmt.Errorf("%w: browser detail API status %d (%d bytes), no matching embedded media", ErrResolveFailed, snapshot.LastResponse.Status, snapshot.LastResponse.Length)
	}
	if snapshot.Oversized {
		return nil, fmt.Errorf("%w: embedded page data too large", ErrResolveFailed)
	}
	return nil, fmt.Errorf("%w: browser has no matching API response or embedded media (requests=%d, document=%s)", ErrResolveFailed, snapshot.Requests, snapshot.State)
}

// 仅重试 CDP 返回的导航上下文切换错误，连接超时等传输错误必须直接返回。
func isBrowserNavigationContextError(err error) bool {
	if err == nil {
		return false
	}
	const prefix = "CDP Runtime.evaluate: "
	message := err.Error()
	if !strings.HasPrefix(message, prefix) {
		return false
	}
	switch strings.TrimSuffix(strings.TrimPrefix(message, prefix), ".") {
	case "Cannot find context with specified id", "Cannot find default execution context",
		"Execution context was destroyed", "Inspected target navigated or closed":
		return true
	default:
		return false
	}
}
