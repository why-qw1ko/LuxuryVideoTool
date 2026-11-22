package resolver

// 集成测试：对真实抖音链接走接口与浏览器页面的完整解析链路。
// 需要联网，且首次会调用无头浏览器引导 ttwid，故默认跳过：
//
//	RUN_LIVE=1 go test ./internal/resolver/ -run TestResolveLive -v

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestResolveLive(t *testing.T) {
	if os.Getenv("RUN_LIVE") == "" {
		t.Skip("RUN_LIVE not set")
	}
	client := NewSafeClient(15*time.Second, 8<<20)
	ttwid := NewTtwidManager(client, os.Getenv("DOUYIN_TTWID_FILE"), os.Getenv("DOUYIN_BROWSER_PATH"))
	d := NewDouyin(client, ttwid)

	link := os.Getenv("DOUYIN_LIVE_URL")
	if link == "" {
		link = "https://v.douyin.com/GkQbHe4KKnk/"
	}
	expectedType := os.Getenv("DOUYIN_LIVE_TYPE")
	if expectedType == "" {
		expectedType = "video"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	work, err := d.Resolve(ctx, link)
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if work.DouyinWorkID == "" || work.Title == "" || work.AuthorName == "" {
		t.Fatalf("work missing key fields: idPresent=%t titlePresent=%t authorPresent=%t", work.DouyinWorkID != "", work.Title != "", work.AuthorName != "")
	}
	if work.Type != expectedType || !workHasMedia(work) {
		t.Fatalf("expected %s with media; got type=%s images=%d videoPresent=%t", expectedType, work.Type, len(work.Images), work.VideoURL != "")
	}
	if strings.Contains(work.VideoURL, "playwm") {
		t.Fatal("video URL still has watermark")
	}
	t.Logf("OK: id=%s type=%s images=%d author=%s source=%s", work.DouyinWorkID, work.Type, len(work.Images), work.AuthorName, work.ResolverName)
}
