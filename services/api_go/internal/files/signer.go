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
func (s *Signer) Validate(fileID, expiry, signature string, now time.Time) bool { value, err := strconv.ParseInt(expiry, 10, 64); if err != nil || now.UTC().Unix() > value { return false }; expected, err := hex.DecodeString(s.sign(fileID, expiry)); if err != nil { return false }; received, err := hex.DecodeString(signature); if err != nil { return false }; return hmac.Equal(expected, received) }
func (s *Signer) sign(fileID, expiry string) string { mac := hmac.New(sha256.New, s.key); mac.Write([]byte(fileID)); mac.Write([]byte{0}); mac.Write([]byte(expiry)); return hex.EncodeToString(mac.Sum(nil)) }
