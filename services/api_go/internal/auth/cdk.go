package auth

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"errors"
	"strings"
	"time"
)

const (
	MaxCDKBatch   = 100
	MaxCDKTasks   = 100_000
	cdkPrefixSize = 10
	cdkCodePrefix = "CDK-"
	cdkSecretSize = 20
)

var (
	ErrInvalidCDK = errors.New("invalid or unavailable CDK")
	ErrCDKInput   = errors.New("invalid CDK settings")
	ErrCDKRevoked = errors.New("CDK is revoked")
	cdkEncoding   = base32.StdEncoding.WithPadding(base32.NoPadding)
)

type CDK struct {
	ID         string     `json:"id"`
	Prefix     string     `json:"prefix"`
	Code       string     `json:"code,omitempty"`
	TaskLimit  int        `json:"taskLimit"`
	TaskCount  int        `json:"taskCount"`
	Status     string     `json:"status"`
	CreatedAt  time.Time  `json:"createdAt"`
	RedeemedAt *time.Time `json:"redeemedAt,omitempty"`
	RevokedAt  *time.Time `json:"revokedAt,omitempty"`
	codeHash   string
}

type CDKUsage struct {
	Used  int `json:"used"`
	Limit int `json:"limit"`
}

type CDKRepository interface {
	CreateCDKs(context.Context, []CDK) error
	ListCDKs(ctx context.Context, limit, offset int) ([]CDK, int, error)
	SetCDKTaskLimit(ctx context.Context, id string, limit int) (CDK, error)
	ResetCDKTaskCount(ctx context.Context, id string) (CDK, error)
	RotateCDK(ctx context.Context, id, prefix, hash string, now time.Time) (CDK, error)
	RevokeCDK(ctx context.Context, id string, now time.Time) (CDK, error)
	RedeemCDK(ctx context.Context, hash string, user User, newSession func(User) (Session, error)) error
	CDKUsage(ctx context.Context, userID string) (CDKUsage, error)
}

func (s *Service) LoginCDK(ctx context.Context, code string, device Device) (TokenPair, error) {
	code = strings.ToUpper(strings.TrimSpace(code))
	secret, valid := strings.CutPrefix(code, cdkCodePrefix)
	if !valid || len(secret) != cdkEncoding.EncodedLen(cdkSecretSize) {
		return TokenPair{}, ErrInvalidCDK
	}
	if _, err := cdkEncoding.DecodeString(secret); err != nil {
		return TokenPair{}, ErrInvalidCDK
	}
	now := s.now().UTC()
	id, err := NewID(now)
	if err != nil {
		return TokenPair{}, err
	}
	user := User{
		ID: id, UsernameNormalized: "cdk-" + strings.ToLower(id), DisplayName: code[:cdkPrefixSize],
		Role: RoleUser, IsActive: true, IsCDK: true, CreatedAt: now, UpdatedAt: now, LastLoginAt: &now,
	}
	var pair TokenPair
	if err := s.repository.RedeemCDK(ctx, HashRefreshToken(code), user, func(user User) (Session, error) {
		var session Session
		var err error
		pair, session, err = s.newSession(user, device, now)
		return session, err
	}); err != nil {
		return TokenPair{}, err
	}
	return pair, nil
}

func (s *Service) CreateCDKs(ctx context.Context, count, taskLimit int) ([]CDK, error) {
	if count < 1 || count > MaxCDKBatch || taskLimit < 1 || taskLimit > MaxCDKTasks {
		return nil, ErrCDKInput
	}
	now := s.now().UTC()
	items := make([]CDK, count)
	for i := range items {
		id, err := NewID(now)
		if err != nil {
			return nil, err
		}
		code, err := newCDKCode()
		if err != nil {
			return nil, err
		}
		items[i] = CDK{ID: id, Prefix: code[:cdkPrefixSize], Code: code, TaskLimit: taskLimit,
			Status: "unused", CreatedAt: now, codeHash: HashRefreshToken(code)}
	}
	if err := s.repository.CreateCDKs(ctx, items); err != nil {
		return nil, err
	}
	return items, nil
}

func newCDKCode() (string, error) {
	var secret [cdkSecretSize]byte
	if _, err := rand.Read(secret[:]); err != nil {
		return "", err
	}
	return cdkCodePrefix + cdkEncoding.EncodeToString(secret[:]), nil
}

func (s *Service) ListCDKs(ctx context.Context, limit, offset int) ([]CDK, int, error) {
	if limit < 1 || limit > 100 || offset < 0 {
		return nil, 0, ErrCDKInput
	}
	return s.repository.ListCDKs(ctx, limit, offset)
}

func (s *Service) SetCDKTaskLimit(ctx context.Context, id string, taskLimit int) (CDK, error) {
	if taskLimit < 1 || taskLimit > MaxCDKTasks {
		return CDK{}, ErrCDKInput
	}
	return s.repository.SetCDKTaskLimit(ctx, id, taskLimit)
}

func (s *Service) ResetCDKTaskCount(ctx context.Context, id string) (CDK, error) {
	return s.repository.ResetCDKTaskCount(ctx, id)
}

func (s *Service) RotateCDK(ctx context.Context, id string) (CDK, error) {
	code, err := newCDKCode()
	if err != nil {
		return CDK{}, err
	}
	item, err := s.repository.RotateCDK(ctx, id, code[:cdkPrefixSize], HashRefreshToken(code), s.now().UTC())
	if err != nil {
		return CDK{}, err
	}
	item.Code = code
	return item, nil
}

func (s *Service) RevokeCDK(ctx context.Context, id string) (CDK, error) {
	return s.repository.RevokeCDK(ctx, id, s.now().UTC())
}

func (s *Service) CDKUsage(ctx context.Context, userID string) (CDKUsage, error) {
	return s.repository.CDKUsage(ctx, userID)
}
