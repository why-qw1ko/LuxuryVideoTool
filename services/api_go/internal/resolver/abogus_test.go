package resolver

import (
	"net/url"
	"strings"
	"testing"
)

func TestSignAwemeDetailURLShape(t *testing.T) {
	query, fullURL, err := signAwemeDetailURL("7515037296070364454")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(query, "aweme_id=7515037296070364454") {
		t.Fatalf("query missing aweme_id: %s", query)
	}
	if !strings.Contains(query, "device_platform=webapp") || !strings.Contains(query, "aid=6383") {
		t.Fatalf("query missing base params: %s", query)
	}
	// a_bogus 必须出现在 URL 末尾且为编码形式
	if !strings.Contains(fullURL, "&a_bogus=") {
		t.Fatalf("fullURL missing a_bogus: %s", fullURL)
	}
	bogus := fullURL[strings.Index(fullURL, "&a_bogus=")+len("&a_bogus="):]
	decoded, err := url.QueryUnescape(bogus)
	if err != nil {
		t.Fatalf("a_bogus 应为 URL 编码形式: %v", err)
	}
	if len(decoded) < 100 || len(decoded)%4 != 0 {
		t.Fatalf("unexpected a_bogus length %d", len(decoded))
	}
	for _, r := range decoded {
		if !strings.ContainsRune(aBogusCharset+"=", r) {
			t.Fatalf("unexpected char %q in a_bogus", r)
		}
	}
}

func TestABogusRandomized(t *testing.T) {
	// 两次签名应产生不同结果（含随机浏览器信息）。
	_, fullA, err := signAwemeDetailURL("123")
	if err != nil {
		t.Fatal(err)
	}
	_, fullB, err := signAwemeDetailURL("123")
	if err != nil {
		t.Fatal(err)
	}
	bogusA := fullA[strings.Index(fullA, "&a_bogus=")+len("&a_bogus="):]
	bogusB := fullB[strings.Index(fullB, "&a_bogus=")+len("&a_bogus="):]
	if bogusA == bogusB {
		t.Fatal("two signatures should differ due to random browser info")
	}
}
