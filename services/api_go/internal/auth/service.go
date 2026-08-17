package auth

import (
	"context"
	"errors"
	"fmt"
	"time"
)

type Service struct {
	repository Repository
	tokens     *TokenManager
	refreshTTL time.Duration
	dummyHash  string
	now        func() time.Time
}

type LoginInput struct {
	Username string
	Password string
	Device   Device
}

type TokenPair struct {
	AccessToken         string
	AccessTokenExpires  time.Time
	RefreshToken        string
	RefreshTokenExpires time.Time
	User                User
	SessionID           string
}

func NewService(repository Repository, tokens *TokenManager, refreshTTL time.Duration) (*Service, error) {
	if repository == nil || tokens == nil || refreshTTL <= 0 {
		return nil, fmt.Errorf("invalid authentication service configuration")
	}
	dummyHash, err := HashPassword("not-a-real-account-password")
	if err != nil {
		return nil, fmt.Errorf("create authentication timing hash: %w", err)
	}
	return &Service{repository: repository, tokens: tokens, refreshTTL: refreshTTL, dummyHash: dummyHash, now: time.Now}, nil
}

func (s *Service) Login(ctx context.Context, input LoginInput) (TokenPair, error) {
	user, err := s.repository.FindUserByUsername(ctx, NormalizeUsername(input.Username))
	if errors.Is(err, ErrNotFound) {
		_, _ = VerifyPassword(input.Password, s.dummyHash)
		return TokenPair{}, ErrInvalidCredentials
	}
	if err != nil {
		return TokenPair{}, err
	}
	valid, err := VerifyPassword(input.Password, user.PasswordHash)
	if err != nil || !valid {
		return TokenPair{}, ErrInvalidCredentials
	}
	if !user.IsActive {
		return TokenPair{}, ErrInactiveUser
	}
	now := s.now().UTC()
	pair, session, err := s.newSession(user, input.Device, now)
	if err != nil {
		return TokenPair{}, err
	}
	if err := s.repository.CreateSession(ctx, session); err != nil {
		return TokenPair{}, err
	}
	if err := s.repository.RecordLogin(ctx, user.ID, now); err != nil {
		_ = s.repository.RevokeSession(ctx, user.ID, session.ID, now)
		return TokenPair{}, err
	}
	return pair, nil
}

func (s *Service) Refresh(ctx context.Context, raw string) (TokenPair, error) {
	now := s.now().UTC()
	oldSession, user, err := s.repository.FindSessionByTokenHash(ctx, HashRefreshToken(raw))
	if err != nil {
		return TokenPair{}, err
	}
	if oldSession.RevokedAt != nil {
		_ = s.repository.RevokeAllUserSessions(ctx, user.ID, now)
		return TokenPair{}, ErrRevokedSession
	}
	if !oldSession.ExpiresAt.After(now) {
		return TokenPair{}, ErrExpiredToken
	}
	if !user.IsActive {
		return TokenPair{}, ErrInactiveUser
	}
	pair, replacement, err := s.newSession(user, oldSession.Device, now)
	if err != nil {
		return TokenPair{}, err
	}
	if err := s.repository.RotateSession(ctx, oldSession.ID, replacement, now); err != nil {
		return TokenPair{}, err
	}
	return pair, nil
}

func (s *Service) Logout(ctx context.Context, userID, sessionID string) error {
	err := s.repository.RevokeSession(ctx, userID, sessionID, s.now().UTC())
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	return err
}

func (s *Service) ListSessions(ctx context.Context, userID string) ([]Session, error) {
	return s.repository.ListSessions(ctx, userID, s.now().UTC())
}

func (s *Service) RevokeSession(ctx context.Context, principal Principal, sessionID string) error {
	return s.repository.RevokeSession(ctx, principal.UserID, sessionID, s.now().UTC())
}

func (s *Service) RevokeAnySession(ctx context.Context, principal Principal, sessionID string) error {
	if principal.Role != RoleAdmin {
		return ErrForbidden
	}
	session, err := s.repository.FindSessionByID(ctx, "", sessionID)
	if err != nil {
		return err
	}
	return s.repository.RevokeSession(ctx, session.UserID, sessionID, s.now().UTC())
}

func (s *Service) ValidateAccess(ctx context.Context, raw string) (Principal, error) {
	principal, err := s.tokens.ParseAccess(raw)
	if err != nil {
		return Principal{}, err
	}
	user, err := s.repository.FindUserByID(ctx, principal.UserID)
	if err != nil || !user.IsActive {
		return Principal{}, ErrInvalidToken
	}
	session, err := s.repository.FindSessionByID(ctx, principal.UserID, principal.SessionID)
	if err != nil || session.RevokedAt != nil || !session.ExpiresAt.After(s.now().UTC()) {
		return Principal{}, ErrRevokedSession
	}
	return principal, nil
}

func (s *Service) CreateUser(ctx context.Context, username, displayName, password string, role Role) (User, error) {
	username = NormalizeUsername(username)
	if username == "" || len(username) > 64 || displayName == "" || len(displayName) > 100 {
		return User{}, fmt.Errorf("invalid username or display name")
	}
	if _, err := s.repository.FindUserByUsername(ctx, username); err == nil {
		return User{}, ErrUsernameTaken
	} else if !errors.Is(err, ErrNotFound) {
		return User{}, err
	}
	if role != RoleAdmin && role != RoleUser {
		return User{}, fmt.Errorf("invalid role")
	}
	hash, err := HashPassword(password)
	if err != nil {
		return User{}, err
	}
	now := s.now().UTC()
	id, err := NewID(now)
	if err != nil {
		return User{}, err
	}
	user := User{ID: id, UsernameNormalized: username, DisplayName: displayName, PasswordHash: hash, Role: role, IsActive: true, CreatedAt: now, UpdatedAt: now}
	if err := s.repository.CreateUser(ctx, user); err != nil {
		return User{}, err
	}
	return user, nil
}

// ListUsers 返回全部用户（不含密码哈希），供管理界面使用。
func (s *Service) ListUsers(ctx context.Context) ([]User, error) {
	users, err := s.repository.ListUsers(ctx)
	if err != nil {
		return nil, err
	}
	for index := range users {
		users[index].PasswordHash = ""
	}
	return users, nil
}

func (s *Service) ResetPassword(ctx context.Context, userID, password string) error {
	hash, err := HashPassword(password)
	if err != nil {
		return err
	}
	now := s.now().UTC()
	if err := s.repository.UpdatePassword(ctx, userID, hash, now); err != nil {
		return err
	}
	return s.repository.RevokeAllUserSessions(ctx, userID, now)
}

func (s *Service) SetUserActive(ctx context.Context, userID string, active bool) error {
	now := s.now().UTC()
	if err := s.repository.SetUserActive(ctx, userID, active, now); err != nil {
		return err
	}
	if !active {
		return s.repository.RevokeAllUserSessions(ctx, userID, now)
	}
	return nil
}

func (s *Service) newSession(user User, device Device, now time.Time) (TokenPair, Session, error) {
	id, err := NewID(now)
	if err != nil {
		return TokenPair{}, Session{}, err
	}
	raw, hash, err := GenerateRefreshToken()
	if err != nil {
		return TokenPair{}, Session{}, err
	}
	access, accessExpiry, err := s.tokens.IssueAccess(user, id)
	if err != nil {
		return TokenPair{}, Session{}, err
	}
	refreshExpiry := now.Add(s.refreshTTL)
	session := Session{ID: id, UserID: user.ID, TokenHash: hash, Device: device, ExpiresAt: refreshExpiry, CreatedAt: now, LastUsedAt: now}
	return TokenPair{AccessToken: access, AccessTokenExpires: accessExpiry, RefreshToken: raw, RefreshTokenExpires: refreshExpiry, User: user, SessionID: id}, session, nil
}
