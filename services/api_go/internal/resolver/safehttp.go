package resolver

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const maxRedirects = 5
const mobileUserAgent = "Mozilla/5.0 (iPhone; CPU iPhone OS 17_2 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) EdgiOS/121.0.2277.107 Version/17.0 Mobile/15E148 Safari/604.1"

type SafeClient struct {
	client  *http.Client
	maxBody int64
}

func NewSafeClient(timeout time.Duration, maxBody int64) *SafeClient {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	if maxBody <= 0 {
		maxBody = 4 << 20
	}
	resolver := net.DefaultResolver
	dialer := &net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}
	transport := &http.Transport{
		Proxy:           nil,
		TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, ErrURLNotAllowed
			}
			ips, err := resolver.LookupIP(ctx, "ip", host)
			if err != nil {
				return nil, fmt.Errorf("resolve host: %w", err)
			}
			for _, ip := range ips {
				if !PublicIP(ip) {
					return nil, ErrURLNotAllowed
				}
			}
			if len(ips) == 0 {
				return nil, ErrURLNotAllowed
			}
			return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].String(), port))
		},
	}
	client := &http.Client{Transport: transport, Timeout: timeout}
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) > maxRedirects || validateURL(req.URL) != nil {
			return ErrURLNotAllowed
		}
		if len(via) > 0 {
			req.Header.Del("Authorization")
			req.Header.Del("Cookie")
		}
		return nil
	}
	return &SafeClient{client: client, maxBody: maxBody}
}

func (c *SafeClient) Get(ctx context.Context, target *url.URL) ([]byte, *url.URL, error) {
	body, respURL, _, err := c.request(ctx, http.MethodGet, target, nil, nil)
	return body, respURL, err
}

// Request 发送带自定义请求头 / Cookie / 请求体的请求，返回响应体与响应头。
// 仅用于对受信任域名（如 www.douyin.com 的 detail API）的调用。
func (c *SafeClient) Request(ctx context.Context, method string, target *url.URL, headers map[string]string, body io.Reader) ([]byte, http.Header, error) {
	bodyBytes, _, respHeaders, err := c.request(ctx, method, target, headers, body)
	return bodyBytes, respHeaders, err
}

func (c *SafeClient) request(ctx context.Context, method string, target *url.URL, headers map[string]string, body io.Reader) ([]byte, *url.URL, http.Header, error) {
	if err := validateURL(target); err != nil {
		return nil, nil, nil, err
	}
	req, err := http.NewRequestWithContext(ctx, method, target.String(), body)
	if err != nil {
		return nil, nil, nil, err
	}
	req.Header.Set("User-Agent", mobileUserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/json;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("%w: request failed: %v", ErrResolveFailed, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone {
		return nil, resp.Request.URL, nil, ErrWorkUnavailable
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, resp.Request.URL, nil, fmt.Errorf("%w: upstream status %d", ErrResolveFailed, resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, c.maxBody+1))
	if err != nil {
		return nil, resp.Request.URL, nil, fmt.Errorf("%w: read response: %v", ErrResolveFailed, err)
	}
	if int64(len(data)) > c.maxBody {
		return nil, resp.Request.URL, nil, fmt.Errorf("%w: response too large", ErrResolveFailed)
	}
	return data, resp.Request.URL, resp.Header, nil
}

func validateURL(u *url.URL) error {
	if u == nil || !strings.EqualFold(u.Scheme, "https") || u.User != nil || !AllowedHost(u.Hostname()) {
		return ErrURLNotAllowed
	}
	if port := u.Port(); port != "" && port != strconv.Itoa(443) {
		return ErrURLNotAllowed
	}
	return nil
}
