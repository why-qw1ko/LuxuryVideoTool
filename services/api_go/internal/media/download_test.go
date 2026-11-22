package media

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"image"
	"image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type downloadRoundTripperFunc func(*http.Request) (*http.Response, error)

func (f downloadRoundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func responseDownloader(status int, body io.Reader, length int64, mime string) *HTTPDownloader {
	return &HTTPDownloader{client: &http.Client{Transport: downloadRoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode:    status,
			Header:        http.Header{"Content-Type": []string{mime}},
			Body:          io.NopCloser(body),
			ContentLength: length,
			Request:       req,
		}, nil
	})}}
}

func assertDownloadPathsAbsent(t *testing.T, paths ...string) {
	t.Helper()
	for _, path := range paths {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("download path %q should not exist, stat error = %v", path, err)
		}
	}
}

func TestHTTPDownloaderRejectsEmptyResponse(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusNoContent} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			dir := t.TempDir()
			temporary := filepath.Join(dir, "image.png.part")
			final := filepath.Join(dir, "image.png")
			downloader := responseDownloader(status, strings.NewReader(""), 0, "image/png")
			_, err := downloader.Download(context.Background(), "https://example.test/image.png", temporary, final, 1024)
			if !errors.Is(err, ErrDownload) {
				t.Fatalf("empty response error = %v, want ErrDownload", err)
			}
			assertDownloadPathsAbsent(t, temporary, final)
		})
	}
}

func TestHTTPDownloaderAcceptsSmallMedia(t *testing.T) {
	var payload bytes.Buffer
	if err := png.Encode(&payload, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	body := payload.Bytes()
	if len(body) >= 1024 {
		t.Fatalf("small PNG fixture is %d bytes, want less than 1024", len(body))
	}
	dir := t.TempDir()
	temporary := filepath.Join(dir, "image.png.part")
	final := filepath.Join(dir, "image.png")
	downloader := responseDownloader(http.StatusOK, bytes.NewReader(body), int64(len(body)), "image/png")
	result, err := downloader.Download(context.Background(), "https://example.test/image.png", temporary, final, 1024)
	if err != nil {
		t.Fatalf("download small PNG: %v", err)
	}
	saved, err := os.ReadFile(final)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(saved, body) {
		t.Fatal("saved PNG differs from the complete response")
	}
	sum := sha256.Sum256(body)
	if result.SizeBytes != int64(len(body)) || result.SHA256 != hex.EncodeToString(sum[:]) || result.MIMEType != "image/png" {
		t.Fatalf("download result = %#v", result)
	}
	assertDownloadPathsAbsent(t, temporary)
}

type interruptedDownloadReader struct{}

func (interruptedDownloadReader) Read([]byte) (int, error) {
	return 0, io.ErrUnexpectedEOF
}

func TestHTTPDownloaderRejectsInterruptedResponse(t *testing.T) {
	dir := t.TempDir()
	temporary := filepath.Join(dir, "image.png.part")
	final := filepath.Join(dir, "image.png")
	body := io.MultiReader(strings.NewReader("incomplete image bytes"), interruptedDownloadReader{})
	downloader := responseDownloader(http.StatusOK, body, -1, "image/png")
	_, err := downloader.Download(context.Background(), "https://example.test/image.png", temporary, final, 1024)
	if !errors.Is(err, ErrDownload) {
		t.Fatalf("interrupted response error = %v, want ErrDownload", err)
	}
	assertDownloadPathsAbsent(t, temporary, final)
}
