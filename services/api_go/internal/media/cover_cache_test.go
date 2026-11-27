package media

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

var coverJPEG = []byte{0xff, 0xd8, 0xff, 0xe0, 0xff, 0xd9}

func coverResponse(req *http.Request, body []byte) *http.Response {
	return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(body)), Request: req}
}

func TestCoverCacheExpiryAndResponseIsolation(t *testing.T) {
	fetcher := NewCoverFetcher()
	now := time.Now()
	fetcher.cache.now = func() time.Time { return now }
	var calls atomic.Int64
	fetcher.client.Transport = downloadRoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		calls.Add(1)
		return coverResponse(req, coverJPEG), nil
	})
	candidates := []string{"https://p3.douyinpic.com/image.jpg"}
	first, _, err := fetcher.Fetch(t.Context(), candidates)
	if err != nil {
		t.Fatal(err)
	}
	first[0] = 0
	second, mime, err := fetcher.Fetch(t.Context(), append(candidates, candidates[0]))
	if err != nil || mime != "image/jpeg" || !bytes.Equal(second, coverJPEG) || calls.Load() != 1 {
		t.Fatalf("cached response = %x, %q, %v; upstream calls = %d", second, mime, err, calls.Load())
	}
	now = now.Add(coverCacheTTL)
	if _, _, err := fetcher.Fetch(t.Context(), candidates); err != nil || calls.Load() != 2 {
		t.Fatalf("expired cache was reused: err=%v calls=%d", err, calls.Load())
	}
}

func TestCoverCacheSharesConcurrentFetches(t *testing.T) {
	fetcher := NewCoverFetcher()
	started, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int64
	fetcher.client.Transport = downloadRoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		if calls.Add(1) == 1 {
			close(started)
		}
		select {
		case <-release:
			return coverResponse(req, coverJPEG), nil
		case <-req.Context().Done():
			return nil, req.Context().Err()
		}
	})
	var group sync.WaitGroup
	for range 12 {
		group.Go(func() {
			body, mime, err := fetcher.Fetch(t.Context(), []string{"https://p3.douyinpic.com/shared.jpg"})
			if err != nil || mime != "image/jpeg" || !bytes.Equal(body, coverJPEG) {
				t.Errorf("shared response = %x %q %v", body, mime, err)
			}
		})
	}
	<-started
	close(release)
	group.Wait()
	if calls.Load() != 1 {
		t.Fatalf("concurrent fetches contacted upstream %d times", calls.Load())
	}
}

func waitForCoverWaiters(t *testing.T, fetcher *CoverFetcher, expected int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		fetcher.cache.mu.Lock()
		count := 0
		for _, call := range fetcher.cache.calls {
			count += len(call.waiters)
		}
		fetcher.cache.mu.Unlock()
		if count == expected {
			return
		}
		runtime.Gosched()
	}
	t.Fatalf("expected %d waiting cover callers", expected)
}

func TestCoverCancellationDoesNotCancelOtherCallers(t *testing.T) {
	fetcher := NewCoverFetcher()
	started, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int64
	fetcher.client.Transport = downloadRoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		if calls.Add(1) == 1 {
			close(started)
		}
		select {
		case <-release:
			return coverResponse(req, coverJPEG), nil
		case <-req.Context().Done():
			return nil, req.Context().Err()
		}
	})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	first := make(chan error, 1)
	go func() {
		_, _, err := fetcher.Fetch(ctx, []string{"https://p3.douyinpic.com/shared.jpg"})
		first <- err
	}()
	<-started
	second := make(chan error, 1)
	go func() {
		_, _, err := fetcher.Fetch(t.Context(), []string{"https://p3.douyinpic.com/shared.jpg"})
		second <- err
	}()
	waitForCoverWaiters(t, fetcher, 2)
	cancel()
	if err := <-first; !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled caller returned %v", err)
	}
	close(release)
	if err := <-second; err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Fatalf("remaining caller caused another fetch: %d", calls.Load())
	}
}

func TestCoverCacheDoesNotRetainCanceledOrFailedFetches(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(fmt.Sprintf("failure-%t", fail), func(t *testing.T) {
			fetcher := NewCoverFetcher()
			started, canceled := make(chan struct{}), make(chan struct{})
			var calls atomic.Int64
			fetcher.client.Transport = downloadRoundTripperFunc(func(req *http.Request) (*http.Response, error) {
				if calls.Add(1) == 1 {
					close(started)
					if fail {
						return &http.Response{StatusCode: http.StatusBadGateway, Body: http.NoBody, Header: make(http.Header), Request: req}, nil
					}
					<-req.Context().Done()
					close(canceled)
					return nil, req.Context().Err()
				}
				return coverResponse(req, coverJPEG), nil
			})
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			result := make(chan error, 1)
			go func() {
				_, _, err := fetcher.Fetch(ctx, []string{"https://p3.douyinpic.com/retry.jpg"})
				result <- err
			}()
			<-started
			if !fail {
				cancel()
			}
			if err := <-result; err == nil {
				t.Fatal("initial failed fetch returned success")
			}
			if !fail {
				<-canceled
			}
			if body, _, err := fetcher.Fetch(t.Context(), []string{"https://p3.douyinpic.com/retry.jpg"}); err != nil || !bytes.Equal(body, coverJPEG) || calls.Load() != 2 {
				t.Fatalf("fetch did not recover: body=%x err=%v calls=%d", body, err, calls.Load())
			}
		})
	}
}

func TestCoverCacheBounds(t *testing.T) {
	for _, size := range []int{len(coverJPEG), maxCoverBytes} {
		t.Run(fmt.Sprintf("body-%d", size), func(t *testing.T) {
			fetcher := NewCoverFetcher()
			body := make([]byte, size)
			copy(body, coverJPEG)
			var calls atomic.Int64
			fetcher.client.Transport = downloadRoundTripperFunc(func(req *http.Request) (*http.Response, error) {
				calls.Add(1)
				return coverResponse(req, body), nil
			})
			count := coverCacheEntries + 1
			if size == maxCoverBytes {
				count = coverCacheMaxBytes/maxCoverBytes + 1
			}
			for i := range count {
				if _, _, err := fetcher.Fetch(t.Context(), []string{fmt.Sprintf("https://p3.douyinpic.com/%d.jpg", i)}); err != nil {
					t.Fatal(err)
				}
			}
			fetcher.cache.mu.Lock()
			entries, retained := len(fetcher.cache.entries), fetcher.cache.bytes
			fetcher.cache.mu.Unlock()
			if entries > coverCacheEntries || retained > coverCacheMaxBytes {
				t.Fatalf("cache exceeded bounds: %d entries, %d bytes", entries, retained)
			}
			if _, _, err := fetcher.Fetch(t.Context(), []string{fmt.Sprintf("https://p3.douyinpic.com/%d.jpg", count-1)}); err != nil || calls.Load() != int64(count) {
				t.Fatalf("newest response was evicted: %v, calls=%d", err, calls.Load())
			}
			if _, _, err := fetcher.Fetch(t.Context(), []string{"https://p3.douyinpic.com/0.jpg"}); err != nil || calls.Load() != int64(count+1) {
				t.Fatalf("oldest response remained cached: %v, calls=%d", err, calls.Load())
			}
		})
	}
}

func TestCoverFetchLimitPreservesCacheHitsAndSharedCallers(t *testing.T) {
	fetcher := NewCoverFetcher()
	started := make(chan struct{}, coverFetchLimit)
	release := make(chan struct{})
	var released sync.Once
	t.Cleanup(func() { released.Do(func() { close(release) }) })
	var calls atomic.Int64
	fetcher.client.Transport = downloadRoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		calls.Add(1)
		if req.URL.Path == "/cached.jpg" || req.URL.Path == "/new.jpg" {
			return coverResponse(req, coverJPEG), nil
		}
		started <- struct{}{}
		select {
		case <-release:
			return coverResponse(req, coverJPEG), nil
		case <-req.Context().Done():
			return nil, req.Context().Err()
		}
	})
	if _, _, err := fetcher.Fetch(t.Context(), []string{"https://p3.douyinpic.com/cached.jpg"}); err != nil {
		t.Fatal(err)
	}
	results := make(chan error, coverFetchLimit+1)
	for i := range coverFetchLimit {
		go func() {
			_, _, err := fetcher.Fetch(t.Context(), []string{fmt.Sprintf("https://p3.douyinpic.com/active-%d.jpg", i)})
			results <- err
		}()
	}
	for range coverFetchLimit {
		<-started
	}
	go func() {
		_, _, err := fetcher.Fetch(t.Context(), []string{"https://p3.douyinpic.com/active-0.jpg"})
		results <- err
	}()
	waitForCoverWaiters(t, fetcher, coverFetchLimit+1)
	if _, _, err := fetcher.Fetch(t.Context(), []string{"https://p3.douyinpic.com/rejected.jpg"}); !errors.Is(err, ErrDownload) {
		t.Fatalf("saturated new key returned %v", err)
	}
	if body, _, err := fetcher.Fetch(t.Context(), []string{"https://p3.douyinpic.com/cached.jpg"}); err != nil || !bytes.Equal(body, coverJPEG) {
		t.Fatalf("saturation blocked cached response: %x %v", body, err)
	}
	if calls.Load() != coverFetchLimit+1 {
		t.Fatalf("saturated fetches exceeded upstream limit: %d", calls.Load())
	}
	released.Do(func() { close(release) })
	for range coverFetchLimit + 1 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := fetcher.Fetch(t.Context(), []string{"https://p3.douyinpic.com/new.jpg"}); err != nil || calls.Load() != coverFetchLimit+2 {
		t.Fatalf("completed fetches did not release capacity: %v, calls=%d", err, calls.Load())
	}
}

func TestCanceledCoverFetchesRetainCapacityUntilUpstreamReturns(t *testing.T) {
	fetcher := NewCoverFetcher()
	started := make(chan struct{}, coverFetchLimit)
	release := make(chan struct{})
	var released sync.Once
	t.Cleanup(func() { released.Do(func() { close(release) }) })
	var calls atomic.Int64
	fetcher.client.Transport = downloadRoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		calls.Add(1)
		if req.URL.Path != "/next.jpg" {
			started <- struct{}{}
			// Deliberately delay cancellation to exercise the bound on actual upstream work.
			<-release
		}
		return coverResponse(req, coverJPEG), nil
	})
	results := make(chan error, coverFetchLimit)
	cancels := make([]context.CancelFunc, 0, coverFetchLimit)
	for i := range coverFetchLimit {
		ctx, cancel := context.WithCancel(t.Context())
		cancels = append(cancels, cancel)
		defer cancel()
		go func() {
			_, _, err := fetcher.Fetch(ctx, []string{fmt.Sprintf("https://p3.douyinpic.com/canceled-%d.jpg", i)})
			results <- err
		}()
	}
	for range coverFetchLimit {
		<-started
	}
	for _, cancel := range cancels {
		cancel()
	}
	for range coverFetchLimit {
		if err := <-results; !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled caller returned %v", err)
		}
	}
	if _, _, err := fetcher.Fetch(t.Context(), []string{"https://p3.douyinpic.com/next.jpg"}); !errors.Is(err, ErrDownload) || calls.Load() != coverFetchLimit {
		t.Fatalf("canceled upstream work bypassed limit: %v, calls=%d", err, calls.Load())
	}
	released.Do(func() { close(release) })
	deadline := time.Now().Add(2 * time.Second)
	for {
		fetcher.cache.mu.Lock()
		active := fetcher.cache.active
		fetcher.cache.mu.Unlock()
		if active == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("canceled upstream requests did not release capacity")
		}
		runtime.Gosched()
	}
	if _, _, err := fetcher.Fetch(t.Context(), []string{"https://p3.douyinpic.com/next.jpg"}); err != nil || calls.Load() != coverFetchLimit+1 {
		t.Fatalf("canceled fetches did not release capacity: %v, calls=%d", err, calls.Load())
	}
}
