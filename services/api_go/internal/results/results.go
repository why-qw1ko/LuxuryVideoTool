package results

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/asr"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/resolver"
)

type Metadata struct { Source string `json:"source"`; VideoID string `json:"videoId"`; Author string `json:"author"`; SourceURL string `json:"sourceUrl"`; CapturedAt time.Time `json:"capturedAt"`; ASRProvider string `json:"asrProvider"`; ASRModel string `json:"asrModel"`; ProviderRequestID string `json:"providerRequestId"`; RawText string `json:"rawText"`; NormalizedText string `json:"normalizedText"`; ProcessingVersion string `json:"processingVersion"` }
type Bundle struct { Markdown, Text string; Meta []byte }
func Build(work resolver.Work, result asr.TranscribeResult, provider, model string, captured time.Time) (Bundle, error) { normalized := result.NormalizedText; if normalized == "" { normalized = asr.Normalize(result.RawText) }; metadata := Metadata{Source: "douyin", VideoID: work.DouyinWorkID, Author: work.AuthorName, SourceURL: work.CanonicalURL, CapturedAt: captured.UTC(), ASRProvider: provider, ASRModel: model, ProviderRequestID: result.RequestID, RawText: result.RawText, NormalizedText: normalized, ProcessingVersion: "normalize-v1"}; meta, err := json.MarshalIndent(metadata, "", "  "); if err != nil { return Bundle{}, err }; markdown := fmt.Sprintf("---\nsource: douyin\nvideo_id: %q\nauthor: %q\nsource_url: %q\ncaptured_at: %q\nasr_provider: %q\n---\n\n# %s\n\n## 发布文案\n\n%s\n\n## 口播文案\n\n%s\n", work.DouyinWorkID, work.AuthorName, work.CanonicalURL, captured.UTC().Format(time.RFC3339), provider, safeHeading(work.Title), work.Description, normalized); return Bundle{Markdown: markdown, Text: normalized+"\n", Meta: meta}, nil }
func safeHeading(value string) string { value = strings.ReplaceAll(value, "\n", " "); value = strings.TrimSpace(value); if value == "" { return "抖音作品" }; return value }
