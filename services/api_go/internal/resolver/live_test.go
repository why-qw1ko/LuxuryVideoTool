package resolver

// 集成测试：对真实抖音链接走完整解析链路（a_bogus + ttwid 引导 + detail API）。
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

	link := "https://v.douyin.com/GkQbHe4KKnk/"
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	work, err := d.Resolve(ctx, link)
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if work.DouyinWorkID == "" || work.Title == "" || work.AuthorName == "" {
		t.Fatalf("work missing key fields: %+v", work)
	}
	if work.Type != "video" || work.VideoURL == "" {
		t.Fatalf("expected video work with URL: %+v", work)
	}
	if strings.Contains(work.VideoURL, "playwm") {
		t.Fatalf("video URL still has watermark: %s", work.VideoURL)
	}
	t.Logf("OK: %s | %s | video=%s", work.AuthorName, work.Title, work.VideoURL)
}
