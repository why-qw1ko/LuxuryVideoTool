package auth

import (
	"context"
	"errors"
	"strings"
	"time"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrInactiveUser       = errors.New("inactive user")
	ErrInvalidToken       = errors.New("invalid token")
	ErrExpiredToken       = errors.New("expired token")
	ErrRevokedSession     = errors.New("revoked session")
	ErrForbidden          = errors.New("forbidden")
	ErrNotFound           = errors.New("not found")
)

type Role string

const (
	RoleAdmin Role = "admin"
	RoleUser  Role = "user"
)

type User struct {
	ID                 string
	UsernameNormalized string
	DisplayName        string
	PasswordHash       string
	Role               Role
	IsActive           bool
	CreatedAt          time.Time
	UpdatedAt          time.Time
	LastLoginAt        *time.Time
}

type Device struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Platform   string `json:"platform"`
	AppVersion string `json:"appVersion"`
}

type Session struct {
	ID         string
	UserID     string
	TokenHash  string
	Device     Device
	ExpiresAt  time.Time
	RevokedAt  *time.Time
	ReplacedBy *string
	CreatedAt  time.Time
	LastUsedAt time.Time
}

type Principal struct {
	UserID    string
	SessionID string
	Role      Role
}

type Repository interface {
	CreateUser(ctx context.Context, user User) error
	UpdatePassword(ctx context.Context, userID, passwordHash string, now time.Time) error
	SetUserActive(ctx context.Context, userID string, active bool, now time.Time) error
	FindUserByUsername(ctx context.Context, usernameNormalized string) (User, error)
	FindUserByID(ctx context.Context, userID string) (User, error)
	FindSessionByID(ctx context.Context, userID, sessionID string) (Session, error)
	RecordLogin(ctx context.Context, userID string, now time.Time) error
	CreateSession(ctx context.Context, session Session) error
	FindSessionByTokenHash(ctx context.Context, tokenHash string) (Session, User, error)
	RotateSession(ctx context.Context, oldSessionID string, replacement Session, now time.Time) error
	ListSessions(ctx context.Context, userID string, now time.Time) ([]Session, error)
	RevokeSession(ctx context.Context, userID, sessionID string, now time.Time) error
	RevokeAllUserSessions(ctx context.Context, userID string, now time.Time) error
}

func NormalizeUsername(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}
