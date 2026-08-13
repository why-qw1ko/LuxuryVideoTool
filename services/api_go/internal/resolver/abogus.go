package resolver

// a_bogus 签名算法（SM3 + RC4 + 自定义 Base64），与抖音网页端 JS 一致。
// 已通过真实 API 验证：裸 HTTP 客户端 + 本算法算出的 a_bogus + ttwid cookie，
// 即可从 aweme/v1/web/aweme/detail 取到完整作品数据。

import (
	"math/rand"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// aBogusCharset 对应 JS 的 CHARSETS.s4。
const aBogusCharset = "Dkdpgh2ZmsQB80/MfvV36XI1R45-WUAlEixNLwoqYTOPuzKFjJnry79HbGcaStCe"

var (
	aBogusUA = [32]byte{
		76, 98, 15, 131, 97, 245, 224, 133, 122, 199,
		241, 166, 79, 34, 90, 191, 128, 126, 122, 98,
		66, 11, 14, 40, 49, 110, 110, 173, 67, 96, 138, 252,
	}
	aBogusEndString = "cus"
)

// rc4EncryptBytes 对 data 做 RC4 加密，key 按字节轮转使用。
func rc4EncryptBytes(key, data []byte) []byte {
	s := make([]byte, 256)
	for i := range s {
		s[i] = byte(i)
	}
	j := 0
	for i := 0; i < 256; i++ {
		j = (j + int(s[i]) + int(key[i%len(key)])) & 0xFF
		s[i], s[j] = s[j], s[i]
	}
	i, j := 0, 0
	out := make([]byte, len(data))
	for k := range data {
		i = (i + 1) & 0xFF
		j = (j + int(s[i])) & 0xFF
		s[i], s[j] = s[j], s[i]
		out[k] = data[k] ^ s[(int(s[i])+int(s[j]))&0xFF]
	}
	return out
}

// aBogusBase64 用自定义字符集做 Base64（等价 JS customBase64Encode）。
func aBogusBase64(data []byte) string {
	cs := aBogusCharset
	var sb strings.Builder
	for i := 0; i < len(data); i += 3 {
		remaining := len(data) - i
		if remaining >= 3 {
			n := uint32(data[i])<<16 | uint32(data[i+1])<<8 | uint32(data[i+2])
			sb.WriteByte(cs[(n>>18)&63])
			sb.WriteByte(cs[(n>>12)&63])
			sb.WriteByte(cs[(n>>6)&63])
			sb.WriteByte(cs[n&63])
		} else if remaining == 2 {
			n := uint32(data[i])<<16 | uint32(data[i+1])<<8
			sb.WriteByte(cs[(n>>18)&63])
			sb.WriteByte(cs[(n>>12)&63])
			sb.WriteByte(cs[(n>>6)&63])
		} else {
			n := uint32(data[i]) << 16
			sb.WriteByte(cs[(n>>18)&63])
			sb.WriteByte(cs[(n>>12)&63])
		}
	}
	for (sb.Len() % 4) != 0 {
		sb.WriteByte('=')
	}
	return sb.String()
}

// sm3BytesToArray 返回 SM3(data) 的 32 个字节。
func sm3BytesToArray(data []byte) []byte {
	sum := sm3Sum(data)
	return sum[:]
}

type aBogus struct {
	browser     string
	browserLen  int
	browserCode []byte
}

func newABogus() *aBogus {
	innerW := 1280 + rand.Intn(1920-1280+1)
	innerH := 720 + rand.Intn(1080-720+1)
	outerW := innerW + rand.Intn(1920-innerW+1)
	outerH := innerH + rand.Intn(1080-innerH+1)
	scrollY := 0
	if rand.Intn(2) == 1 {
		scrollY = 30
	}
	browser := strconv.Itoa(innerW) + "|" + strconv.Itoa(innerH) + "|" + strconv.Itoa(outerW) + "|" + strconv.Itoa(outerH) +
		"|0|" + strconv.Itoa(scrollY) + "|0|0|" +
		strconv.Itoa(outerW) + "|" + strconv.Itoa(outerH) + "|" + strconv.Itoa(outerW) + "|" + strconv.Itoa(outerH) +
		"|" + strconv.Itoa(innerW) + "|" + strconv.Itoa(innerH) + "|24|24|MacIntel"
	return &aBogus{browser: browser, browserLen: len(browser), browserCode: []byte(browser)}
}

func (a *aBogus) randomList(r float64, b, c, d, e, f, g int) []byte {
	ri := int(r)
	v1 := ri & 255
	v2 := (ri >> 8) & 255
	return []byte{byte(v1&b | d), byte(v1&c | e), byte(v2&b | f), byte(v2&c | g)}
}

func (a *aBogus) generateString1() []byte {
	var out []byte
	out = append(out, a.randomList(rand.Float64()*10000, 170, 85, 1, 2, 5, 45&170)...)
	out = append(out, a.randomList(rand.Float64()*10000, 170, 85, 1, 0, 0, 0)...)
	out = append(out, a.randomList(rand.Float64()*10000, 170, 85, 1, 0, 5, 0)...)
	return out
}

func (a *aBogus) endCheckNum(arr []byte) byte {
	var r byte
	for _, v := range arr {
		r ^= v
	}
	return r
}

func (a *aBogus) list4Arr(params, method string, startTime, endTime int64) []byte {
	now := time.Now().UnixMilli()
	if startTime == 0 {
		startTime = now
	}
	if endTime == 0 {
		endTime = now + int64(4+rand.Intn(5))
	}
	paramsArr := sm3BytesToArray([]byte(params + aBogusEndString))
	methodArr := sm3BytesToArray([]byte(method + aBogusEndString))
	_ = aBogusUA
	out := make([]byte, 0, 38)
	out = append(out, 44)
	out = append(out, byte((endTime>>24)&255), 0, 0, 0, 0, 24)
	out = append(out, paramsArr[21], methodArr[21], 0)
	out = append(out, aBogusUA[23], byte((endTime>>16)&255), paramsArr[22])
	out = append(out, aBogusUA[24], byte((endTime>>8)&255), byte(endTime&255))
	out = append(out, 0, 0, 0, 0)
	out = append(out, byte((startTime>>24)&255), byte((startTime>>16)&255))
	out = append(out, 0, 0, 14)
	out = append(out, byte((startTime>>8)&255), byte(startTime&255))
	out = append(out, 0)
	out = append(out, methodArr[22])
	out = append(out, byte((endTime>>32)&0xFF), byte((startTime>>32)&0xFF))
	out = append(out, 3)
	out = append(out, byte(a.browserLen))
	out = append(out, 1, 1, 0, 0, 0)
	return out
}

func (a *aBogus) generateString2(params, method string) []byte {
	arr := a.list4Arr(params, method, 0, 0)
	check := a.endCheckNum(arr)
	arr = append(arr, a.browserCode...)
	arr = append(arr, check)
	return rc4EncryptBytes([]byte("y"), arr)
}

func (a *aBogus) getValue(params string) string {
	string1 := a.generateString1()
	string2 := a.generateString2(params, "GET")
	data := append(string1, string2...)
	return aBogusBase64(data)
}

// encodeURIComponent 精确复刻 JS 同名函数：除 A-Za-z0-9-_.!~*'() 外全部百分号编码。
func encodeURIComponent(s string) string {
	var sb strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' ||
			c == '-' || c == '_' || c == '.' || c == '!' || c == '~' || c == '*' || c == '\'' || c == '(' || c == ')' {
			sb.WriteByte(c)
		} else {
			sb.WriteByte('%')
			sb.WriteByte("0123456789ABCDEF"[c>>4])
			sb.WriteByte("0123456789ABCDEF"[c&0x0F])
		}
	}
	return sb.String()
}

// signAwemeDetailURL 构造带 a_bogus 签名的详情 API URL，返回查询串与完整 URL。
// params 按固定顺序排列以保证签名与请求一致。
func signAwemeDetailURL(awemeID string) (query string, fullURL string, err error) {
	params := [][2]string{
		{"device_platform", "webapp"},
		{"aid", "6383"},
		{"channel", "channel_pc_web"},
		{"pc_client_type", "1"},
		{"version_code", "170400"},
		{"version_name", "17.4.0"},
		{"cookie_enabled", "true"},
		{"browser_language", "zh-CN"},
		{"browser_platform", "MacIntel"},
		{"browser_name", "Chrome"},
		{"browser_online", "true"},
		{"engine_name", "Blink"},
		{"os_name", "Mac OS"},
		{"os_version", "10"},
		{"platform", "PC"},
		{"screen_width", "1920"},
		{"screen_height", "1080"},
		{"aweme_id", awemeID},
	}
	var sb strings.Builder
	for i, kv := range params {
		if i > 0 {
			sb.WriteByte('&')
		}
		sb.WriteString(url.QueryEscape(kv[0]))
		sb.WriteByte('=')
		sb.WriteString(url.QueryEscape(kv[1]))
	}
	query = sb.String()

	bogus := newABogus().getValue(query)
	fullURL = "https://www.douyin.com/aweme/v1/web/aweme/detail/?" + query + "&a_bogus=" + encodeURIComponent(bogus)
	return query, fullURL, nil
}
