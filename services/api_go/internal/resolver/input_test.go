package resolver

import (
	"net"
	"testing"
)

func TestExtractInput(t *testing.T) {
	input, err := ExtractInput("复制打开抖音 https://evil.example/x 再看 https://www.douyin.com/note/1234567890/?x=1")
	if err != nil { t.Fatal(err) }
	if input.WorkID != "1234567890" || input.WorkType != "note" { t.Fatalf("input = %#v", input) }
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
