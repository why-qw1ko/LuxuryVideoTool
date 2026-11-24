package media

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"reflect"
	"testing"
)

func TestCoverFetcherUsesAllowedRasterCandidates(t *testing.T) {
	jpeg := []byte{0xff, 0xd8, 0xff, 0xe0, 0xff, 0xd9}
	fetcher := NewCoverFetcher()
	var visited []string
	fetcher.client.Transport = downloadRoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		visited = append(visited, req.URL.Path)
		if got := req.Referer(); got != "https://www.douyin.com/" {
			t.Errorf("Referer = %q", got)
		}
		response := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Request: req}
		switch req.URL.Path {
		case "/redirect":
			response.StatusCode = http.StatusFound
			response.Header.Set("Location", "https://p3.douyinpic.com/redirect-target")
		case "/large":
			response.ContentLength = maxCoverBytes + 1
		case "/html":
			response.Header.Set("Content-Type", "image/jpeg")
			response.Body = io.NopCloser(bytes.NewBufferString("<html>upstream error</html>"))
		case "/valid":
			response.Body = io.NopCloser(bytes.NewReader(jpeg))
		default:
			t.Errorf("unexpected request: %s", req.URL)
		}
		if response.Body == nil {
			response.Body = http.NoBody
		}
		return response, nil
	})
	body, mime, err := fetcher.Fetch(context.Background(), []string{
		"https://douyinpic.com.example.test/external",
		"http://p3.douyinpic.com/insecure", "https://127.0.0.1/local",
		"https://p3.douyinpic.com:8443/port", "https://p3.douyinpic.com/redirect",
		"https://p3.douyinpic.com/large", "https://p3.douyinpic.com/html",
		"https://p6.byteimg.com/valid",
	})
	if err != nil || mime != "image/jpeg" || !bytes.Equal(body, jpeg) {
		t.Fatalf("fetched cover = %x, %q, %v", body, mime, err)
	}
	if want := []string{"/redirect", "/large", "/html", "/valid"}; !reflect.DeepEqual(visited, want) {
		t.Fatalf("requested %v, want %v", visited, want)
	}
}
