package auth

import (
	"context"
	"errors"
	"testing"
	"time"
)

type memoryRepository struct {
	user User
	sessions map[string]Session
}
func (r *memoryRepository) CreateUser(context.Context, User) error { return nil }
func (r *memoryRepository) UpdatePassword(context.Context, string, string, time.Time) error { return nil }
func (r *memoryRepository) SetUserActive(context.Context, string, bool, time.Time) error { return nil }
func (r *memoryRepository) ListUsers(context.Context) ([]User, error) { return []User{r.user}, nil }
func (r *memoryRepository) FindUserByUsername(_ context.Context, username string) (User, error) { if r.user.UsernameNormalized != username { return User{}, ErrNotFound }; return r.user, nil }
func (r *memoryRepository) FindUserByID(_ context.Context, id string) (User, error) { if r.user.ID != id { return User{}, ErrNotFound }; return r.user, nil }
func (r *memoryRepository) FindSessionByID(_ context.Context, userID, id string) (Session, error) { value, ok := r.sessions[id]; if !ok || value.UserID != userID { return Session{}, ErrNotFound }; return value, nil }
func (r *memoryRepository) RecordLogin(context.Context, string, time.Time) error { return nil }
func (r *memoryRepository) CreateSession(_ context.Context, session Session) error { r.sessions[session.ID] = session; return nil }
func (r *memoryRepository) FindSessionByTokenHash(_ context.Context, hash string) (Session, User, error) { for _, value := range r.sessions { if value.TokenHash == hash { return value, r.user, nil } }; return Session{}, User{}, ErrInvalidToken }
func (r *memoryRepository) RotateSession(_ context.Context, old string, replacement Session, now time.Time) error { value, ok := r.sessions[old]; if !ok || value.RevokedAt != nil { return ErrRevokedSession }; value.RevokedAt = &now; value.ReplacedBy = &replacement.ID; r.sessions[old] = value; r.sessions[replacement.ID] = replacement; return nil }
func (r *memoryRepository) ListSessions(context.Context, string, time.Time) ([]Session, error) { return nil, nil }
func (r *memoryRepository) RevokeSession(_ context.Context, userID, id string, now time.Time) error { value, ok := r.sessions[id]; if !ok || value.UserID != userID { return ErrNotFound }; value.RevokedAt = &now; r.sessions[id] = value; return nil }
func (r *memoryRepository) RevokeAllUserSessions(context.Context, string, time.Time) error { return nil }

func TestLoginRefreshAndRevocation(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil { t.Fatal(err) }
	now := time.Date(2026, 8, 12, 0, 0, 0, 0, time.UTC)
	repo := &memoryRepository{user: User{ID: "user", UsernameNormalized: "owner", PasswordHash: hash, DisplayName: "Owner", Role: RoleAdmin, IsActive: true}, sessions: map[string]Session{}}
	tokens, err := NewTokenManager([]byte("01234567890123456789012345678901"), 15*time.Minute)
	if err != nil { t.Fatal(err) }
	tokens.now = func() time.Time { return now }
	service, err := NewService(repo, tokens, 30*24*time.Hour)
	if err != nil { t.Fatal(err) }
	service.now = func() time.Time { return now }
	pair, err := service.Login(context.Background(), LoginInput{Username: "OWNER", Password: "correct horse battery staple", Device: Device{ID: "device", Name: "PC", Platform: "windows", AppVersion: "1.0.0"}})
	if err != nil { t.Fatal(err) }
	principal, err := service.ValidateAccess(context.Background(), pair.AccessToken)
	if err != nil || principal.SessionID != pair.SessionID { t.Fatalf("ValidateAccess() = %#v, %v", principal, err) }
	rotated, err := service.Refresh(context.Background(), pair.RefreshToken)
	if err != nil { t.Fatal(err) }
	if rotated.RefreshToken == pair.RefreshToken { t.Fatal("refresh token was not rotated") }
	if _, err := service.Refresh(context.Background(), pair.RefreshToken); !errors.Is(err, ErrRevokedSession) { t.Fatalf("reused refresh token error = %v", err) }
	if _, err := service.ValidateAccess(context.Background(), pair.AccessToken); !errors.Is(err, ErrRevokedSession) { t.Fatalf("old access token error = %v", err) }
}
