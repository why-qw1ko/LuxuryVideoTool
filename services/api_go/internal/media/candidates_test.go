package media

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

type candidateDownloaderFunc func(context.Context, string, string, string, int64) (DownloadResult, error)

func (f candidateDownloaderFunc) Download(ctx context.Context, mediaURL, temporary, final string, limit int64) (DownloadResult, error) {
	return f(ctx, mediaURL, temporary, final, limit)
}

func TestDownloadCandidatesFailsOverAndCleansTemporary(t *testing.T) {
	dir := t.TempDir()
	temporary, final := filepath.Join(dir, "media.part"), filepath.Join(dir, "media.mp4")
	first, second := "https://first.example/video?signature=secret", "https://second.example/video"
	var calls []string
	downloader := candidateDownloaderFunc(func(_ context.Context, mediaURL, temporary, final string, _ int64) (DownloadResult, error) {
		calls = append(calls, mediaURL)
		if mediaURL == first {
			if err := os.WriteFile(temporary, []byte("incomplete"), 0o600); err != nil {
				t.Fatal(err)
			}
			return DownloadResult{}, &url.Error{Op: "Get", URL: mediaURL, Err: context.DeadlineExceeded}
		}
		assertDownloadPathsAbsent(t, temporary)
		if err := os.WriteFile(temporary, []byte("complete video"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(temporary, final); err != nil {
			t.Fatal(err)
		}
		return DownloadResult{SizeBytes: 14, MIMEType: "video/mp4"}, nil
	})
	result, err := DownloadCandidates(context.Background(), downloader, []string{"", " " + first + " ", first, second, "https://unused.example/video"}, temporary, final, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(calls, []string{first, second}) || result.SizeBytes != 14 {
		t.Fatalf("calls=%v result=%#v", calls, result)
	}
	body, err := os.ReadFile(final)
	if err != nil || string(body) != "complete video" {
		t.Fatalf("final body=%q error=%v", body, err)
	}
	assertDownloadPathsAbsent(t, temporary)
}

func TestDownloadCandidatesStopsOnCancellationOrSize(t *testing.T) {
	for _, scenario := range []string{"cancelled", "oversized"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			dir := t.TempDir()
			temporary, final := filepath.Join(dir, "media.part"), filepath.Join(dir, "media.mp4")
			calls := 0
			downloader := candidateDownloaderFunc(func(context.Context, string, string, string, int64) (DownloadResult, error) {
				calls++
				if err := os.WriteFile(temporary, []byte("partial"), 0o600); err != nil {
					t.Fatal(err)
				}
				if scenario == "cancelled" {
					cancel()
					return DownloadResult{}, ErrDownload
				}
				return DownloadResult{}, ErrTooLarge
			})
			_, err := DownloadCandidates(ctx, downloader, []string{"https://one.example/a", "https://two.example/a"}, temporary, final, 1024)
			want := ErrTooLarge
			if scenario == "cancelled" {
				want = context.Canceled
			}
			if !errors.Is(err, want) || calls != 1 {
				t.Fatalf("error=%v calls=%d", err, calls)
			}
			assertDownloadPathsAbsent(t, temporary, final)
		})
	}
}

func TestDownloadCandidatesCleansFinalWhenCancelledAfterRename(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	dir := t.TempDir()
	temporary, final := filepath.Join(dir, "media.part"), filepath.Join(dir, "media.mp4")
	calls := 0
	downloader := candidateDownloaderFunc(func(context.Context, string, string, string, int64) (DownloadResult, error) {
		calls++
		if err := os.WriteFile(temporary, []byte("complete video"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(temporary, final); err != nil {
			t.Fatal(err)
		}
		cancel()
		return DownloadResult{SizeBytes: 14, MIMEType: "video/mp4"}, nil
	})
	_, err := DownloadCandidates(ctx, downloader, []string{"https://one.example/a", "https://two.example/a"}, temporary, final, 1024)
	if !errors.Is(err, context.Canceled) || calls != 1 {
		t.Fatalf("error=%v calls=%d", err, calls)
	}
	assertDownloadPathsAbsent(t, temporary, final)
}

func TestDownloadCandidatesLimitsAttemptsAndRedactsURL(t *testing.T) {
	dir := t.TempDir()
	var candidates []string
	for i := 0; i < 12; i++ {
		candidates = append(candidates, fmt.Sprintf("https://cdn%d.example/video?token=private", i))
	}
	calls := 0
	downloader := candidateDownloaderFunc(func(_ context.Context, mediaURL, _, _ string, _ int64) (DownloadResult, error) {
		calls++
		return DownloadResult{}, fmt.Errorf("failed to fetch %s", mediaURL)
	})
	_, err := DownloadCandidates(context.Background(), downloader, candidates, filepath.Join(dir, "media.part"), filepath.Join(dir, "media.mp4"), 1024)
	if !errors.Is(err, ErrDownload) || calls != 8 {
		t.Fatalf("error=%v calls=%d", err, calls)
	}
	for _, sensitive := range []string{"private", "token", "https://", "cdn"} {
		if strings.Contains(err.Error(), sensitive) {
			t.Fatalf("candidate error leaked address: %v", err)
		}
	}
}

func TestDownloadCandidatesPreservesExistingFiles(t *testing.T) {
	for _, existing := range []string{"temporary", "final"} {
		t.Run(existing, func(t *testing.T) {
			dir := t.TempDir()
			temporary, final := filepath.Join(dir, "media.part"), filepath.Join(dir, "media.mp4")
			path := temporary
			if existing == "final" {
				path = final
			}
			if err := os.WriteFile(path, []byte("existing file"), 0o600); err != nil {
				t.Fatal(err)
			}
			calls := 0
			downloader := candidateDownloaderFunc(func(context.Context, string, string, string, int64) (DownloadResult, error) {
				calls++
				return DownloadResult{}, nil
			})
			_, err := DownloadCandidates(context.Background(), downloader, []string{"https://cdn.example/a"}, temporary, final, 1024)
			if !errors.Is(err, ErrDownload) || calls != 0 {
				t.Fatalf("error=%v calls=%d", err, calls)
			}
			body, err := os.ReadFile(path)
			if err != nil || string(body) != "existing file" {
				t.Fatalf("existing file changed: %q error=%v", body, err)
			}
		})
	}
}

func TestDownloadCandidatesSharesParentDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	deadline, _ := ctx.Deadline()
	dir := t.TempDir()
	calls := 0
	downloader := candidateDownloaderFunc(func(attempt context.Context, _ string, _, _ string, _ int64) (DownloadResult, error) {
		calls++
		got, ok := attempt.Deadline()
		if !ok || !got.Equal(deadline) {
			t.Fatalf("deadline=%v, want shared parent %v", got, deadline)
		}
		if calls == 1 {
			return DownloadResult{}, &url.Error{Op: "Get", URL: "https://first.example/a?token=secret", Err: context.DeadlineExceeded}
		}
		return DownloadResult{SizeBytes: 10}, nil
	})
	if _, err := DownloadCandidates(ctx, downloader, []string{"https://one.example/a", "https://two.example/a"}, filepath.Join(dir, "media.part"), filepath.Join(dir, "media.mp4"), 1024); err != nil || calls != 2 {
		t.Fatalf("error=%v calls=%d", err, calls)
	}
}

func TestDownloadCandidatesHeaderTimeoutDoesNotLimitBody(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/slow-headers" {
			<-r.Context().Done()
			return
		}
		w.Header().Set("Content-Type", "video/mp4")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		time.Sleep(100 * time.Millisecond)
		_, _ = w.Write([]byte("complete media"))
	}))
	defer server.Close()
	downloader := NewHTTPDownloader()
	transport := downloader.client.Transport.(*http.Transport)
	// 仅测试替换拨号目标和证书校验，以本地TLS服务模拟CDN响应；生产地址校验保持不变。
	transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	transport.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
	}
	transport.ResponseHeaderTimeout = 50 * time.Millisecond
	defer transport.CloseIdleConnections()
	dir := t.TempDir()
	temporary, final := filepath.Join(dir, "media.part"), filepath.Join(dir, "media.mp4")
	_, err := DownloadCandidates(context.Background(), downloader, []string{"https://media.example.test/slow-headers", "https://media.example.test/slow-body"}, temporary, final, 1024)
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(final)
	if err != nil || string(body) != "complete media" {
		t.Fatalf("body=%q error=%v", body, err)
	}
}
