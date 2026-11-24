package resolver

import (
	"context"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"
)

type resolveTransport func(*http.Request) (*http.Response, error)

func (f resolveTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func resolveClient(transport resolveTransport) *SafeClient {
	client := NewSafeClient(time.Minute, 4096)
	client.client.Transport = transport
	return client
}

func resolveResponse(r *http.Request, status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Request: r, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}

func TestResolveUnusableShortLinkOnlyOnce(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		want   error
	}{
		{"not found", http.StatusNotFound, ErrWorkUnavailable},
		{"gone", http.StatusGone, ErrWorkUnavailable},
		{"disallowed redirect", http.StatusFound, ErrURLNotAllowed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			client := resolveClient(func(r *http.Request) (*http.Response, error) {
				calls++
				res := resolveResponse(r, tc.status, "")
				if tc.status == http.StatusFound {
					res.Header.Set("Location", "https://outside.example/test")
				}
				return res, nil
			})
			_, err := NewDouyin(client, nil).Resolve(context.Background(), "https://v.douyin.com/invalid/")
			if !errors.Is(err, tc.want) || calls != 1 {
				t.Fatalf("Resolve() error = %v, requests = %d, want %v after one request", err, calls, tc.want)
			}
		})
	}
}

const testPublicWork = `<script>window._ROUTER_DATA = {"aweme_id":"123","desc":"work title","video":{"play_addr":{"url_list":["https://media.example/video"]}}};</script>`

func TestResolveReusesShortLinkTargetBody(t *testing.T) {
	var requests []string
	client := resolveClient(func(r *http.Request) (*http.Response, error) {
		requests = append(requests, r.URL.String())
		if r.URL.Host == "v.douyin.com" {
			res := resolveResponse(r, http.StatusFound, "")
			res.Header.Set("Location", "https://www.iesdouyin.com/share/video/123/")
			return res, nil
		}
		return resolveResponse(r, http.StatusOK, testPublicWork), nil
	})
	work, err := NewDouyin(client, nil).Resolve(context.Background(), "https://v.douyin.com/short/")
	if err != nil || work.DouyinWorkID != "123" || !workHasMedia(work) {
		t.Fatalf("Resolve() = %+v, %v", work, err)
	}
	want := []string{"https://v.douyin.com/short/", "https://www.iesdouyin.com/share/video/123/"}
	if !reflect.DeepEqual(requests, want) {
		t.Fatalf("requests = %v, want %v", requests, want)
	}
}

func TestResolveDetailNotFoundStillTriesPublicPage(t *testing.T) {
	var requests []string
	client := resolveClient(func(r *http.Request) (*http.Response, error) {
		requests = append(requests, r.URL.Path)
		if strings.HasPrefix(r.URL.Path, "/aweme/") {
			return resolveResponse(r, http.StatusNotFound, ""), nil
		}
		return resolveResponse(r, http.StatusOK, testPublicWork), nil
	})
	manager := NewTtwidManager(client, "", "")
	manager.current = &douyinTtwid{Value: "test", ExpiresAt: time.Now().Add(ttwidTTL)}
	work, err := NewDouyin(client, manager).Resolve(context.Background(), "https://www.douyin.com/video/123")
	if err != nil || work.DouyinWorkID != "123" || len(requests) != 2 || requests[1] != "/video/123" {
		t.Fatalf("Resolve() = %+v, %v, requests = %v", work, err, requests)
	}
}

func TestResolveAlternatePageNotFoundPreservesPrimaryMetadata(t *testing.T) {
	var calls int
	client := resolveClient(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Host == "www.iesdouyin.com" {
			return resolveResponse(r, http.StatusNotFound, ""), nil
		}
		return resolveResponse(r, http.StatusOK, `<script>window._ROUTER_DATA = {"aweme_id":"123","desc":"work title"};</script>`), nil
	})
	work, err := NewDouyin(client, nil).Resolve(context.Background(), "https://www.douyin.com/video/123")
	if err != nil || work.Title != "work title" || calls != 2 {
		t.Fatalf("Resolve() = %+v, %v, requests = %d", work, err, calls)
	}
}

func TestResolveBrowserFallbackUsesRemainingBudget(t *testing.T) {
	client := resolveClient(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/video/123" {
			return resolveResponse(r, http.StatusOK, "<html></html>"), nil
		}
		return resolveResponse(r, http.StatusNotFound, ""), nil
	})
	manager := NewTtwidManager(client, "", "")
	manager.current = &douyinTtwid{Value: "test", ExpiresAt: time.Now().Add(ttwidTTL)}
	manager.browserGate <- struct{}{}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err := NewDouyin(client, manager).Resolve(ctx, "https://www.douyin.com/video/123")
	if !errors.Is(err, ErrResolveTimeout) || time.Since(started) > time.Second || len(manager.browserGate) != 1 {
		t.Fatalf("Resolve() error = %v, elapsed = %v, held gate = %d", err, time.Since(started), len(manager.browserGate))
	}
}

func TestResolveRespectsCallerDeadlineAndCancellation(t *testing.T) {
	for _, tc := range []struct {
		name  string
		limit time.Duration
		want  error
	}{
		{"timeout", 100 * time.Millisecond, ErrResolveTimeout},
		{"cancel", time.Second, context.Canceled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), tc.limit)
			defer cancel()
			calls := 0
			client := resolveClient(func(r *http.Request) (*http.Response, error) {
				calls++
				if tc.name == "cancel" {
					cancel()
				}
				<-r.Context().Done()
				return nil, r.Context().Err()
			})
			started := time.Now()
			_, err := NewDouyin(client, nil).Resolve(ctx, "https://v.douyin.com/slow/")
			if !errors.Is(err, tc.want) || calls != 1 || time.Since(started) > time.Second {
				t.Fatalf("Resolve() error = %v, requests = %d, elapsed = %v", err, calls, time.Since(started))
			}
		})
	}
}
