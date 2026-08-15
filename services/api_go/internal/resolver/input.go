package resolver

import (
	"net"
	"net/url"
	"regexp"
	"strings"
)

var (
	httpsURLPattern = regexp.MustCompile(`https://[^\s<>"'，。；！？、]+`)
	workPathPattern = regexp.MustCompile(`^/(?:video|note|share/(?:video|note|slides))/(\d+)(?:/|$)`)
)

var allowedHosts = map[string]bool{
	"v.douyin.com": true, "www.douyin.com": true, "douyin.com": true,
	"www.iesdouyin.com": true, "iesdouyin.com": true,
}

type Input struct {
	URL     *url.URL
	WorkID  string
	WorkType string
}

func ExtractInput(text string) (Input, error) {
	for _, raw := range httpsURLPattern.FindAllString(text, -1) {
		raw = strings.TrimRight(raw, ")]}〉》」』】.,;:!?，。；：！？")
		u, err := url.Parse(raw)
		if err != nil || validateURL(u) != nil {
			continue
		}
		input := Input{URL: u}
		if match := workPathPattern.FindStringSubmatch(u.EscapedPath()); len(match) == 2 {
			input.WorkID = match[1]
			// note 与 slides（轮播图文）都是图文/动图类作品，统一按 note 处理。
			if strings.Contains(u.Path, "/note/") || strings.Contains(u.Path, "/slides/") {
				input.WorkType = "note"
			} else {
				input.WorkType = "video"
			}
		}
		return input, nil
	}
	return Input{}, ErrInvalidShareLink
}

func AllowedHost(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
	return allowedHosts[host]
}

func PublicIP(ip net.IP) bool {
	if ip == nil || ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() || ip.IsMulticast() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return false
	}
	if v4 := ip.To4(); v4 != nil {
		return !(v4[0] == 0 || v4[0] == 127 || (v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127) || (v4[0] == 169 && v4[1] == 254) || v4[0] >= 224)
	}
	return ip.IsGlobalUnicast()
}
