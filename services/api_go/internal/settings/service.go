package settings

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
)

type Service struct {
	db       *sql.DB
	aead     cipher.AEAD
	mu       sync.RWMutex
	fallback map[string]string
}

func New(db *sql.DB, masterKey []byte) (*Service, error) {
	if db == nil || len(masterKey) < 32 {
		return nil, errors.New("invalid runtime settings configuration")
	}
	derived := sha256.Sum256(append([]byte("runtime-settings:"), masterKey...))
	block, err := aes.NewCipher(derived[:])
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Service{db: db, aead: aead, fallback: make(map[string]string)}, nil
}

func (s *Service) SetFallback(name, value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fallback[name] = strings.TrimSpace(value)
}

func (s *Service) Get(ctx context.Context, name string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var encrypted []byte
	if err := s.db.QueryRowContext(ctx, `SELECT encrypted_value FROM runtime_settings WHERE name=?`, name).Scan(&encrypted); err != nil {
		return "", err
	}
	return s.decrypt(name, encrypted)
}

func (s *Service) decrypt(name string, encrypted []byte) (string, error) {
	nonceSize := s.aead.NonceSize()
	if len(encrypted) <= nonceSize {
		return "", errors.New("invalid encrypted setting")
	}
	plain, err := s.aead.Open(nil, encrypted[:nonceSize], encrypted[nonceSize:], []byte(name))
	if err != nil {
		return "", errors.New("decrypt runtime setting")
	}
	return string(plain), nil
}

func (s *Service) store(ctx context.Context, name, value, userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.put(ctx, s.db, name, value, userID)
}

type settingWriter interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func (s *Service) put(ctx context.Context, writer settingWriter, name, value, userID string) error {
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return err
	}
	encrypted := s.aead.Seal(nonce, nonce, []byte(value), []byte(name))
	_, err := writer.ExecContext(ctx, `INSERT INTO runtime_settings(name, encrypted_value, updated_at, updated_by)
		VALUES (?, ?, ?, ?) ON CONFLICT(name) DO UPDATE SET encrypted_value=excluded.encrypted_value,
		updated_at=excluded.updated_at, updated_by=excluded.updated_by`, name, encrypted, time.Now().UTC().UnixMilli(), userID)
	if err != nil {
		return fmt.Errorf("save runtime setting: %w", err)
	}
	return nil
}
