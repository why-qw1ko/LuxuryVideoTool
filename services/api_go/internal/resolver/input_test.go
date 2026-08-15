package resolver

import (
	"net"
	"testing"
)

func TestExtractInput(t *testing.T) {
	cases := []struct {
		text     string
		id       string
		workType string
	}{
		{"复制打开抖音 https://evil.example/x 再看 https://www.douyin.com/note/1234567890/?x=1", "1234567890", "note"},
		{"https://www.iesdouyin.com/share/video/2222222222222222222/", "2222222222222222222", "video"},
		{"https://www.iesdouyin.com/share/note/3333333333333333333/?region=CN", "3333333333333333333", "note"},
		{"https://www.iesdouyin.com/share/slides/4444444444444444444/?is_slides=1", "4444444444444444444", "note"},
	}
	for _, c := range cases {
		input, err := ExtractInput(c.text)
		if err != nil { t.Fatalf("%q: %v", c.text, err) }
		if input.WorkID != c.id || input.WorkType != c.workType { t.Fatalf("%q: input = %#v, want id=%s type=%s", c.text, input, c.id, c.workType) }
	}
}

func TestExtractInputRejectsHTTPAndLookalikeHost(t *testing.T) {
	for _, value := range []string{"http://www.douyin.com/video/1", "https://www.douyin.com.evil.test/video/1", "https://localhost/video/1"} {
		if _, err := ExtractInput(value); err != ErrInvalidShareLink { t.Fatalf("%q: err = %v", value, err) }
	}
}

func TestPublicIP(t *testing.T) {
	for _, value := range []string{"127.0.0.1", "10.0.0.1", "169.254.169.254", "::1", "fc00::1"} {
		if PublicIP(net.ParseIP(value)) { t.Fatalf("%s must be blocked", value) }
	}
	if !PublicIP(net.ParseIP("8.8.8.8")) { t.Fatal("public address was blocked") }
}
