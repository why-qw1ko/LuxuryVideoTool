package resolver

// ttwid 管理：detail API 的必要 cookie，由抖音网页端 JS（webid SDK）生成。
//
// 生命周期：
//   - 持久化到 storePath（默认 ./data/douyin_ttwid.json），有效期约 1 年；
//   - 临近过期时用 ttwid/check 接口纯 HTTP 续期（只需携带现有 ttwid）；
//   - 完全没有 ttwid 时，用本机 Edge/Chrome 无头浏览器引导一次：导航到
//     douyin.com 让 webid SDK 生成 ttwid，再经 CDP 读取并持久化。

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	ttwidTTL          = 365 * 24 * time.Hour
	ttwidRefreshAfter = 300 * 24 * time.Hour // 剩余少于 ~65 天时续期
	ttwidBootstrapURL = "https://www.douyin.com/"
)

type douyinTtwid struct {
	Value     string    `json:"value"`
	ExpiresAt time.Time `json:"expiresAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// TtwidManager 负责 ttwid 的加载、续期与引导。并发安全。
type TtwidManager struct {
	mu          sync.Mutex
	client      *SafeClient
	storePath   string
	browserPath string
	current     *douyinTtwid
}

func NewTtwidManager(client *SafeClient, storePath, browserPath string) *TtwidManager {
	return &TtwidManager{client: client, storePath: storePath, browserPath: browserPath}
}

// Get 返回一个仍有效的 ttwid（必要时加载 / 续期 / 无头浏览器引导）。
// hintURL 仅在完全没有 ttwid 需要浏览器引导时使用：抖音首页不生成 ttwid，
// 必须导航到视频等完整应用页才会触发 webid SDK 生成，故传入作品页 URL。
func (m *TtwidManager) Get(ctx context.Context, hintURL string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	cur := m.current
	if cur == nil {
		cur = m.load()
		m.current = cur
	}
	if cur != nil && cur.Value != "" && time.Until(cur.ExpiresAt) > ttwidRefreshAfter {
		return cur.Value, nil
	}

	// 已有 ttwid：先尝试纯 HTTP 续期
	if cur != nil && cur.Value != "" {
		if value, err := m.refresh(ctx, cur.Value); err == nil {
			m.store(value)
			return value, nil
		}
	}

	// 无可用 ttwid：无头浏览器引导一次
	value, err := m.bootstrap(ctx, hintURL)
	if err != nil {
		return "", err
	}
	m.store(value)
	return value, nil
}

// refresh 用现有 ttwid 调用 ttwid/check 换新。
func (m *TtwidManager) refresh(ctx context.Context, old string) (string, error) {
	endpoint, err := url.Parse("https://www.douyin.com/ttwid/check/")
	if err != nil {
		return "", err
	}
	body := strings.NewReader(`{"aid":6383,"service":"www.douyin.com"}`)
	headers := map[string]string{
		"Content-Type": "application/json",
		"Referer":      "https://www.douyin.com/",
		"Cookie":       "ttwid=" + old,
		"Accept":       "application/json, text/plain, */*",
	}
	_, respHeaders, err := m.client.Request(ctx, http.MethodPost, endpoint, headers, body)
	if err != nil {
		return "", err
	}
	for _, line := range respHeaders.Values("Set-Cookie") {
		if value, ok := cookieValue(line, "ttwid"); ok && value != "" {
			return value, nil
		}
	}
	return "", fmt.Errorf("ttwid/check 未返回新 ttwid")
}

func (m *TtwidManager) store(value string) {
	now := time.Now()
	m.current = &douyinTtwid{Value: value, ExpiresAt: now.Add(ttwidTTL), UpdatedAt: now}
	m.save(m.current)
}

func (m *TtwidManager) load() *douyinTtwid {
	if m.storePath == "" {
		return nil
	}
	data, err := os.ReadFile(m.storePath)
	if err != nil {
		return nil
	}
	var t douyinTtwid
	if json.Unmarshal(data, &t) != nil || t.Value == "" {
		return nil
	}
	return &t
}

func (m *TtwidManager) save(t *douyinTtwid) {
	if m.storePath == "" {
		return
	}
	data, err := json.Marshal(t)
	if err != nil {
		return
	}
	if dir := filepath.Dir(m.storePath); dir != "" && dir != "." {
		_ = os.MkdirAll(dir, 0o755)
	}
	_ = os.WriteFile(m.storePath, data, 0o600)
}

// bootstrap 用无头浏览器引导 ttwid：启动 Edge/Chrome → 导航 douyin.com →
// 轮询 CDP cookies 直到出现 ttwid → 关闭浏览器。
// 注意：抖音首页不触发 ttwid 生成，hintURL 需指向视频/图集等完整应用页。
func (m *TtwidManager) bootstrap(ctx context.Context, hintURL string) (string, error) {
	browser := m.browserPath
	if browser == "" {
		browser = findBrowserExecutable()
	}
	if browser == "" {
		return "", fmt.Errorf("未找到 Edge/Chrome，无法引导 ttwid；可在环境变量 DOUYIN_BROWSER_PATH 指定浏览器路径")
	}

	port, err := freeTCPPort()
	if err != nil {
		return "", err
	}
	userData, err := os.MkdirTemp("", "douyin-cdp-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(userData)

	args := []string{
		"--headless=new", "--disable-gpu", "--no-sandbox",
		fmt.Sprintf("--remote-debugging-port=%d", port),
		"--user-data-dir=" + userData, "--no-first-run",
		"--disable-background-networking", "--disable-extensions",
		"--remote-allow-origins=*", "about:blank",
	}
	cmd := exec.CommandContext(ctx, browser, args...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("启动无头浏览器失败: %w", err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}()

	pageWS, err := waitForDevtoolsPage(ctx, port)
	if err != nil {
		return "", err
	}

	conn, err := dialCDP(pageWS)
	if err != nil {
		return "", err
	}
	defer conn.Close()

	if _, err := conn.Call("Network.enable", nil, 5*time.Second); err != nil {
		return "", err
	}
	if _, err := conn.Call("Page.enable", nil, 5*time.Second); err != nil {
		return "", err
	}
	targetURL := hintURL
	if targetURL == "" {
		targetURL = ttwidBootstrapURL
	}
	if _, err := conn.Call("Page.navigate", map[string]any{"url": targetURL}, 10*time.Second); err != nil {
		return "", err
	}

	deadline := time.Now().Add(25 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}
		res, err := conn.Call("Network.getCookies", map[string]any{"urls": []string{"https://www.douyin.com/"}}, 10*time.Second)
		if err == nil {
			var result struct {
				Cookies []struct {
					Name  string `json:"name"`
					Value string `json:"value"`
				} `json:"cookies"`
			}
			if json.Unmarshal(res, &result) == nil {
				for _, c := range result.Cookies {
					if c.Name == "ttwid" && c.Value != "" {
						return c.Value, nil
					}
				}
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	return "", fmt.Errorf("无头浏览器在 %d 秒内未取得 ttwid（请检查网络与浏览器可用性）", 25)
}

// --- CDP 最小客户端 ---

type cdpConn struct {
	ws *websocket.Conn
	id int
}

func dialCDP(rawURL string) (*cdpConn, error) {
	dialer := &websocket.Dialer{HandshakeTimeout: 10 * time.Second}
	ws, _, err := dialer.Dial(rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("连接 CDP 失败: %w", err)
	}
	return &cdpConn{ws: ws}, nil
}

func (c *cdpConn) Call(method string, params map[string]any, timeout time.Duration) (json.RawMessage, error) {
	c.id++
	msg := map[string]any{"id": c.id, "method": method, "params": params}
	if err := c.ws.WriteJSON(msg); err != nil {
		return nil, err
	}
	_ = c.ws.SetReadDeadline(time.Now().Add(timeout))
	for {
		var resp struct {
			ID     int             `json:"id"`
			Error  *struct {
				Message string `json:"message"`
			} `json:"error"`
			Result json.RawMessage `json:"result"`
		}
		if err := c.ws.ReadJSON(&resp); err != nil {
			return nil, err
		}
		if resp.ID == c.id {
			if resp.Error != nil {
				return nil, fmt.Errorf("CDP %s: %s", method, resp.Error.Message)
			}
			return resp.Result, nil
		}
	}
}

func (c *cdpConn) Close() { _ = c.ws.Close() }

// waitForDevtoolsPage 等待调试端口就绪并返回第一个 page 目标的 WebSocket 地址。
func waitForDevtoolsPage(ctx context.Context, port int) (string, error) {
	deadline := time.Now().Add(20 * time.Second)
	client := &http.Client{Timeout: 3 * time.Second}
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}
		resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/json", port))
		if err == nil {
			var tabs []struct {
				Type                string `json:"type"`
				WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
			}
			readErr := json.NewDecoder(resp.Body).Decode(&tabs)
			_ = resp.Body.Close()
			if readErr == nil {
				for _, tab := range tabs {
					if tab.Type == "page" && tab.WebSocketDebuggerURL != "" {
						return tab.WebSocketDebuggerURL, nil
					}
				}
			}
		}
		time.Sleep(300 * time.Millisecond)
	}
	return "", fmt.Errorf("无头浏览器调试端口未就绪")
}

func freeTCPPort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port, nil
}

// cookieValue 从 Set-Cookie / Cookie 单行中提取指定名字的值。
func cookieValue(line, name string) (string, bool) {
	for _, part := range strings.Split(line, ";") {
		part = strings.TrimSpace(part)
		if k, v, ok := strings.Cut(part, "="); ok && strings.EqualFold(strings.TrimSpace(k), name) {
			return strings.TrimSpace(v), true
		}
	}
	return "", false
}

// findBrowserExecutable 探测常见 Edge/Chrome 路径。
func findBrowserExecutable() string {
	var candidates []string
	if runtime.GOOS == "windows" {
		appData := os.Getenv("LOCALAPPDATA")
		if appData != "" {
			candidates = append(candidates,
				filepath.Join(appData, "Microsoft", "Edge", "Application", "msedge.exe"),
				filepath.Join(appData, "Google", "Chrome", "Application", "chrome.exe"),
			)
		}
		candidates = append(candidates,
			`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
			`C:\Program Files\Microsoft\Edge\Application\msedge.exe`,
			`C:\Program Files\Google\Chrome\Application\chrome.exe`,
			`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
		)
	} else {
		candidates = append(candidates,
			"/usr/bin/chromium", "/usr/bin/chromium-browser",
			"/usr/bin/google-chrome", "/usr/bin/google-chrome-stable",
			"/usr/bin/msedge", "/opt/microsoft/msedge/microsoft-edge",
		)
	}
	for _, path := range candidates {
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path
		}
	}
	return ""
}
