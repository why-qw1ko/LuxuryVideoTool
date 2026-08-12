package media

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/resolver"
)

var (
	ErrTooLarge = errors.New("media too large")
	ErrDownload = errors.New("media download failed")
)

type DownloadResult struct { SizeBytes int64; SHA256 string; MIMEType string }
type Downloader interface { Download(ctx context.Context, mediaURL, temporaryPath, finalPath string, maxBytes int64) (DownloadResult, error) }

type HTTPDownloader struct{ client *http.Client }
func NewHTTPDownloader() *HTTPDownloader {
	resolverDNS := net.DefaultResolver; dialer := &net.Dialer{Timeout: 10*time.Second, KeepAlive: 30*time.Second}
	transport := &http.Transport{Proxy: nil, TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}, DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address); if err != nil { return nil, resolver.ErrURLNotAllowed }
		ips, err := resolverDNS.LookupIP(ctx, "ip", host); if err != nil || len(ips) == 0 { return nil, resolver.ErrURLNotAllowed }
		for _, ip := range ips { if !resolver.PublicIP(ip) { return nil, resolver.ErrURLNotAllowed } }
		return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].String(), port))
	}}
	client := &http.Client{Transport: transport}; client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) > 5 { return resolver.ErrURLNotAllowed }; return validateMediaURL(req.URL)
	}
	return &HTTPDownloader{client: client}
}

func (d *HTTPDownloader) Download(ctx context.Context, mediaURL, temporaryPath, finalPath string, maxBytes int64) (DownloadResult, error) {
	target, err := url.Parse(mediaURL); if err != nil || validateMediaURL(target) != nil { return DownloadResult{}, resolver.ErrURLNotAllowed }
	if maxBytes <= 0 { return DownloadResult{}, ErrTooLarge }
	// 连接超时由 Dialer 控制；总超时按配置上限估算，最低 2 分钟、最高 2 小时。
	total := 2*time.Minute + time.Duration(maxBytes/(2<<20))*time.Second; if total > 2*time.Hour { total = 2*time.Hour }
	downloadCtx, cancel := context.WithTimeout(ctx, total); defer cancel()
	req, err := http.NewRequestWithContext(downloadCtx, http.MethodGet, target.String(), nil); if err != nil { return DownloadResult{}, err }
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; DouyinCapture/1.0)")
	resp, err := d.client.Do(req); if err != nil { return DownloadResult{}, fmt.Errorf("%w: %v", ErrDownload, err) }; defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 { return DownloadResult{}, fmt.Errorf("%w: upstream status %d", ErrDownload, resp.StatusCode) }
	if resp.ContentLength > maxBytes { return DownloadResult{}, ErrTooLarge }
	file, err := os.OpenFile(temporaryPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600); if err != nil { return DownloadResult{}, err }
	keep := false; defer func() { file.Close(); if !keep { os.Remove(temporaryPath) } }()
	hash := sha256.New(); written, err := io.Copy(io.MultiWriter(file, hash), io.LimitReader(resp.Body, maxBytes+1))
	if err != nil { return DownloadResult{}, fmt.Errorf("%w: %v", ErrDownload, err) }; if written > maxBytes { return DownloadResult{}, ErrTooLarge }
	if err := file.Sync(); err != nil { return DownloadResult{}, err }; if err := file.Close(); err != nil { return DownloadResult{}, err }
	if err := os.Rename(temporaryPath, finalPath); err != nil { return DownloadResult{}, fmt.Errorf("finalize media: %w", err) }
	keep = true; mime := strings.TrimSpace(strings.Split(resp.Header.Get("Content-Type"), ";")[0]); if mime == "" { mime = "application/octet-stream" }
	return DownloadResult{SizeBytes: written, SHA256: hex.EncodeToString(hash.Sum(nil)), MIMEType: mime}, nil
}

func validateMediaURL(target *url.URL) error {
	if target == nil || !strings.EqualFold(target.Scheme, "https") || target.User != nil || target.Hostname() == "" { return resolver.ErrURLNotAllowed }
	if ip := net.ParseIP(target.Hostname()); ip != nil && !resolver.PublicIP(ip) { return resolver.ErrURLNotAllowed }
	if port := target.Port(); port != "" && port != "443" { return resolver.ErrURLNotAllowed }; return nil
}

type FakeDownloader struct { Result DownloadResult; Err error; Calls int }
func (f *FakeDownloader) Download(_ context.Context, _, _, _ string, _ int64) (DownloadResult, error) { f.Calls++; return f.Result, f.Err }
