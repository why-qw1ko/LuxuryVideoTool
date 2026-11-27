package media

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
)

const maxCoverBytes = 4 << 20

type CoverFetcher struct {
	client *http.Client
	cache  coverCache
}

func NewCoverFetcher() *CoverFetcher {
	return &CoverFetcher{client: &http.Client{
		Transport: newMediaTransport(), Timeout: 3 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}
}

// CoverURLAllowed restricts the signed cover route to Douyin's media CDNs.
func CoverURLAllowed(raw string) bool {
	target, err := url.Parse(raw)
	if err != nil || validateMediaURL(target) != nil {
		return false
	}
	host := strings.ToLower(target.Hostname())
	for _, domain := range []string{"douyinpic.com", "byteimg.com", "douyincdn.com", "douyinvod.com"} {
		if host == domain || strings.HasSuffix(host, "."+domain) {
			return true
		}
	}
	return false
}

// Fetch returns a small raster image from the resolved work's cover candidates.
func (f *CoverFetcher) Fetch(ctx context.Context, candidates []string) ([]byte, string, error) {
	allowed := slices.DeleteFunc(slices.Clone(candidates), func(candidate string) bool { return !CoverURLAllowed(candidate) })
	allowed = uniqueCandidates(allowed)
	if len(allowed) == 0 {
		return nil, "", ErrDownload
	}
	return f.cachedFetch(ctx, allowed)
}

func (f *CoverFetcher) fetchCandidates(ctx context.Context, candidates []string) ([]byte, string, error) {
	for _, candidate := range candidates {
		if ctx.Err() != nil {
			return nil, "", ctx.Err()
		}
		if body, mime, err := f.fetch(ctx, candidate); err == nil {
			return body, mime, nil
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	return nil, "", ErrDownload
}

func (f *CoverFetcher) fetch(ctx context.Context, candidate string) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, candidate, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", douyinMediaUserAgent)
	req.Header.Set("Referer", douyinMediaReferer)
	req.Header.Set("Accept", "image/webp,image/avif,image/jpeg,image/png,image/*;q=0.8")
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.ContentLength > maxCoverBytes {
		return nil, "", ErrDownload
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxCoverBytes+1))
	if err != nil || len(body) > maxCoverBytes {
		return nil, "", ErrDownload
	}
	mime := http.DetectContentType(body)
	switch mime {
	case "image/jpeg", "image/png", "image/webp", "image/gif":
		return body, mime, nil
	}
	if len(body) >= 16 && string(body[4:8]) == "ftyp" && (string(body[8:12]) == "avif" || string(body[8:12]) == "avis") {
		return body, "image/avif", nil
	}
	return nil, "", ErrDownload
}
