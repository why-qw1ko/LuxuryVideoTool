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

const (
	AliyunKey      = "aliyun_api_key"
	SiliconFlowKey = "siliconflow_api_key"
	ASRModel       = "asr_model"
	disabledValue  = "\x00"
)

type Service struct {
	db       *sql.DB
	aead     cipher.AEAD
	mu       sync.RWMutex
	fallback map[string]string
}

type Status struct {
	AliyunConfigured      bool   `json:"aliyunConfigured"`
	AliyunAvailable       bool   `json:"aliyunAvailable"`
	SiliconFlowConfigured bool   `json:"siliconFlowConfigured"`
	ASRModel              string `json:"asrModel"`
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

func (s *Service) Resolve(ctx context.Context, name, fallback string) string {
	value, err := s.Get(ctx, name)
	if errors.Is(err, sql.ErrNoRows) {
		return fallback
	}
	if err != nil || value == disabledValue {
		return ""
	}
	return value
}

func (s *Service) Set(ctx context.Context, name, value, userID string) error {
	if name != AliyunKey && name != SiliconFlowKey && name != ASRModel {
		return errors.New("unsupported runtime setting")
	}
	value = strings.TrimSpace(value)
	if name == ASRModel {
		if len(value) > 256 {
			return errors.New("ASR model is too long")
		}
		if strings.ContainsAny(value, "\r\n\t ") {
			return errors.New("ASR model contains invalid characters")
		}
	} else if len(value) > 4096 {
		return errors.New("API key is too long")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if value == "" {
		value = disabledValue
	}
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return err
	}
	encrypted := s.aead.Seal(nonce, nonce, []byte(value), []byte(name))
	_, err := s.db.ExecContext(ctx, `INSERT INTO runtime_settings(name, encrypted_value, updated_at, updated_by)
		VALUES (?, ?, ?, ?) ON CONFLICT(name) DO UPDATE SET encrypted_value=excluded.encrypted_value,
		updated_at=excluded.updated_at, updated_by=excluded.updated_by`, name, encrypted, time.Now().UTC().UnixMilli(), userID)
	if err != nil {
		return fmt.Errorf("save runtime setting: %w", err)
	}
	return nil
}

func (s *Service) Status(ctx context.Context) (Status, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := Status{AliyunConfigured: s.fallback[AliyunKey] != "", SiliconFlowConfigured: s.fallback[SiliconFlowKey] != "", ASRModel: s.fallback[ASRModel]}
	rows, err := s.db.QueryContext(ctx, `SELECT name, encrypted_value FROM runtime_settings WHERE name IN (?, ?, ?)`, AliyunKey, SiliconFlowKey, ASRModel)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		var encrypted []byte
		if err := rows.Scan(&name, &encrypted); err != nil {
			return result, err
		}
		nonceSize := s.aead.NonceSize()
		if len(encrypted) <= nonceSize {
			continue
		}
		value, err := s.aead.Open(nil, encrypted[:nonceSize], encrypted[nonceSize:], []byte(name))
		if err != nil {
			return result, errors.New("decrypt runtime setting status")
		}
		configured := string(value) != disabledValue
		if name == AliyunKey {
			result.AliyunConfigured = configured
		}
		if name == SiliconFlowKey {
			result.SiliconFlowConfigured = configured
		}
		if name == ASRModel && configured {
			result.ASRModel = string(value)
		}
	}
	return result, rows.Err()
}
