package files

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Signer struct { key []byte; baseURL string }
func NewSigner(key []byte, baseURL string) *Signer { return &Signer{key: append([]byte(nil), key...), baseURL: strings.TrimRight(baseURL, "/")} }
func (s *Signer) Enabled() bool { return s != nil && s.baseURL != "" }
func (s *Signer) URL(fileID string, expires time.Time) string { expiry := strconv.FormatInt(expires.UTC().Unix(), 10); signature := s.sign(fileID, expiry); return s.baseURL+"/api/v1/asr-source/"+url.PathEscape(fileID)+"?expires="+expiry+"&signature="+signature }
// PreviewURL 生成同源（相对路径）的媒体预览地址，供 <img>/<video> 直接加载，无需 Authorization 头。
func (s *Signer) PreviewURL(fileID string, expires time.Time) string { expiry := strconv.FormatInt(expires.UTC().Unix(), 10); signature := s.sign(fileID, expiry); return "/api/v1/media-preview/" + url.PathEscape(fileID) + "?expires=" + expiry + "&signature=" + signature }
func (s *Signer) Validate(fileID, expiry, signature string, now time.Time) bool { value, err := strconv.ParseInt(expiry, 10, 64); if err != nil || now.UTC().Unix() > value { return false }; expected, err := hex.DecodeString(s.sign(fileID, expiry)); if err != nil { return false }; received, err := hex.DecodeString(signature); if err != nil { return false }; return hmac.Equal(expected, received) }
func (s *Signer) sign(fileID, expiry string) string { mac := hmac.New(sha256.New, s.key); mac.Write([]byte(fileID)); mac.Write([]byte{0}); mac.Write([]byte(expiry)); return hex.EncodeToString(mac.Sum(nil)) }

// ArchiveURL 生成同源的短时 ZIP 下载地址，签名独立绑定用户、任务和有效期。
func (s *Signer) ArchiveURL(userID, jobID string, expires time.Time) string {
	if s == nil || userID == "" || jobID == "" {
		return ""
	}
	expiry := strconv.FormatInt(expires.UTC().Unix(), 10)
	signature := hex.EncodeToString(s.archiveMAC(userID, jobID, expiry))
	return "/api/v1/archive-download/" + url.PathEscape(jobID) + "?user=" + url.QueryEscape(userID) + "&expires=" + expiry + "&signature=" + signature
}

func (s *Signer) ValidateArchive(userID, jobID, expiry, signature string, now time.Time) bool {
	if s == nil || userID == "" || jobID == "" {
		return false
	}
	value, err := strconv.ParseInt(expiry, 10, 64)
	if err != nil || value <= now.UTC().Unix() {
		return false
	}
	received, err := hex.DecodeString(signature)
	if err != nil {
		return false
	}
	return hmac.Equal(s.archiveMAC(userID, jobID, expiry), received)
}

func (s *Signer) archiveMAC(userID, jobID, expiry string) []byte {
	mac := hmac.New(sha256.New, s.key)
	mac.Write([]byte("archive-download\x00"))
	for _, value := range []string{userID, jobID, expiry} {
		mac.Write([]byte(strconv.Itoa(len(value)) + ":"))
		mac.Write([]byte(value))
	}
	return mac.Sum(nil)
}
