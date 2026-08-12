package asr

import (
	"testing"
	"time"
)

func TestNormalizeRemovesControlCharacters(t *testing.T) { got := Normalize("  你好\r\n世\u0000界  "); if got != "你好\n世界" { t.Fatalf("Normalize() = %q", got) } }
func TestMergeConservativelyRemovesOverlap(t *testing.T) { got := Merge([]string{"今天我们开始测试", "开始测试新的功能"}); if got != "今天我们开始测试新的功能" { t.Fatalf("Merge() = %q", got) } }
func TestNormalizeUsesNFC(t *testing.T) { if got := Normalize("e\u0301"); got != "é" { t.Fatalf("Normalize() = %q", got) } }
func TestFallbackPolicy(t *testing.T) { if fallbackAllowed(ErrAuth) || fallbackAllowed(ErrBudgetExceeded) || fallbackAllowed(ErrInputRejected) { t.Fatal("non-retryable errors must not trigger fallback") }; if !fallbackAllowed(ErrRateLimited) { t.Fatal("rate limits should trigger fallback") } }
func TestRetryAfterPolicy(t *testing.T) { if got := RetryAfter(ErrRateLimited, 1); got != 60*time.Second { t.Fatalf("RetryAfter() = %v", got) }; if got := RetryAfter(ErrAuth, 0); got != 0 { t.Fatalf("auth retry = %v", got) } }
