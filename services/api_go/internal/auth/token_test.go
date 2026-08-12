package auth

import (
	"errors"
	"testing"
	"time"
)

func TestAccessTokenRoundTripAndExpiry(t *testing.T) {
	manager, err := NewTokenManager([]byte("01234567890123456789012345678901"), 15*time.Minute)
	if err != nil { t.Fatal(err) }
	now := time.Date(2026, 8, 12, 0, 0, 0, 0, time.UTC)
	manager.now = func() time.Time { return now }
	token, _, err := manager.IssueAccess(User{ID: "user-id", Role: RoleAdmin}, "session-id")
	if err != nil { t.Fatal(err) }
	principal, err := manager.ParseAccess(token)
	if err != nil || principal.UserID != "user-id" || principal.SessionID != "session-id" { t.Fatalf("ParseAccess() = %#v, %v", principal, err) }
	manager.now = func() time.Time { return now.Add(16 * time.Minute) }
	if _, err := manager.ParseAccess(token); !errors.Is(err, ErrExpiredToken) { t.Fatalf("expired error = %v", err) }
}

func TestRefreshTokenIsHashed(t *testing.T) {
	raw, hash, err := GenerateRefreshToken()
	if err != nil { t.Fatal(err) }
	if raw == hash || HashRefreshToken(raw) != hash { t.Fatal("refresh token hash contract failed") }
}

