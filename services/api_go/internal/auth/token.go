package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

type TokenManager struct {
	key       []byte
	accessTTL time.Duration
	now       func() time.Time
}

type accessClaims struct {
	Issuer    string `json:"iss"`
	Subject   string `json:"sub"`
	Audience  string `json:"aud"`
	SessionID string `json:"sid"`
	Role      Role   `json:"role"`
	IssuedAt  int64  `json:"iat"`
	ExpiresAt int64  `json:"exp"`
}

func NewTokenManager(key []byte, accessTTL time.Duration) (*TokenManager, error) {
	if len(key) < 32 {
		return nil, fmt.Errorf("JWT signing key must contain at least 32 bytes")
	}
	if accessTTL <= 0 {
		return nil, fmt.Errorf("access token TTL must be positive")
	}
	return &TokenManager{key: append([]byte(nil), key...), accessTTL: accessTTL, now: time.Now}, nil
}

func (m *TokenManager) IssueAccess(user User, sessionID string) (string, time.Time, error) {
	now := m.now().UTC()
	expires := now.Add(m.accessTTL)
	header, _ := json.Marshal(map[string]string{"alg": "HS256", "typ": "JWT"})
	claims, _ := json.Marshal(accessClaims{
		Issuer: "douyin-capture", Subject: user.ID, Audience: "douyin-capture-client",
		SessionID: sessionID, Role: user.Role, IssuedAt: now.Unix(), ExpiresAt: expires.Unix(),
	})
	unsigned := encodeSegment(header) + "." + encodeSegment(claims)
	return unsigned + "." + m.signature(unsigned), expires, nil
}

func (m *TokenManager) ParseAccess(token string) (Principal, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return Principal{}, ErrInvalidToken
	}
	unsigned := parts[0] + "." + parts[1]
	want, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || !hmac.Equal(want, m.signatureBytes(unsigned)) {
		return Principal{}, ErrInvalidToken
	}
	var header map[string]string
	if err := decodeSegment(parts[0], &header); err != nil || header["alg"] != "HS256" || header["typ"] != "JWT" {
		return Principal{}, ErrInvalidToken
	}
	var claims accessClaims
	if err := decodeSegment(parts[1], &claims); err != nil {
		return Principal{}, ErrInvalidToken
	}
	now := m.now().UTC().Unix()
	if claims.Issuer != "douyin-capture" || claims.Audience != "douyin-capture-client" || claims.Subject == "" || claims.SessionID == "" || claims.IssuedAt > now+60 {
		return Principal{}, ErrInvalidToken
	}
	if claims.ExpiresAt <= now {
		return Principal{}, ErrExpiredToken
	}
	if claims.Role != RoleAdmin && claims.Role != RoleUser {
		return Principal{}, ErrInvalidToken
	}
	return Principal{UserID: claims.Subject, SessionID: claims.SessionID, Role: claims.Role}, nil
}

func GenerateRefreshToken() (raw, hash string, err error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", "", fmt.Errorf("generate refresh token: %w", err)
	}
	raw = base64.RawURLEncoding.EncodeToString(bytes)
	return raw, HashRefreshToken(raw), nil
}

func HashRefreshToken(raw string) string {
	digest := sha256.Sum256([]byte(raw))
	return base64.RawURLEncoding.EncodeToString(digest[:])
}

func (m *TokenManager) signature(unsigned string) string {
	return base64.RawURLEncoding.EncodeToString(m.signatureBytes(unsigned))
}

func (m *TokenManager) signatureBytes(unsigned string) []byte {
	mac := hmac.New(sha256.New, m.key)
	_, _ = mac.Write([]byte(unsigned))
	return mac.Sum(nil)
}

func encodeSegment(value []byte) string { return base64.RawURLEncoding.EncodeToString(value) }

func decodeSegment(segment string, target any) error {
	bytes, err := base64.RawURLEncoding.DecodeString(segment)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(strings.NewReader(string(bytes)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return ErrInvalidToken
	}
	return nil
}
