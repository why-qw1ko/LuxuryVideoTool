package results

import (
	"strings"
	"testing"
	"time"

	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/asr"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/resolver"
)
func TestBuildPreservesRawTextAndMetadata(t *testing.T) { bundle, err := Build(resolver.Work{DouyinWorkID: "123", Title: "标题", Description: "发布文案", AuthorName: "作者", CanonicalURL: "https://www.douyin.com/video/123"}, asr.TranscribeResult{RawText: "第一段\n开始测试新的功能", NormalizedText: "第一段新的功能", RequestID: "request-1"}, "aliyun_paraformer", "paraformer-v2", time.Unix(0, 0)); if err != nil { t.Fatal(err) }; if !strings.Contains(bundle.Markdown, `video_id: "123"`) || !strings.Contains(bundle.Markdown, "第一段新的功能") || !strings.Contains(string(bundle.Meta), `"rawText": "第一段\n开始测试新的功能"`) { t.Fatalf("bundle = %#v", bundle) } }
