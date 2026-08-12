package config

import (
	"log/slog"
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	t.Setenv("HTTP_ADDR", "")
	t.Setenv("LOG_LEVEL", "")
	t.Setenv("JWT_SIGNING_KEY_FILE", "test.key")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.HTTPAddr != defaultHTTPAddr {
		t.Fatalf("HTTPAddr = %q, want %q", cfg.HTTPAddr, defaultHTTPAddr)
	}
	if cfg.LogLevel != slog.LevelInfo {
		t.Fatalf("LogLevel = %v, want info", cfg.LogLevel)
	}
	if cfg.ResolverCacheTTL != 6*time.Hour { t.Fatalf("ResolverCacheTTL = %v", cfg.ResolverCacheTTL) }
	if cfg.WorkerConcurrency != 1 || cfg.MaxVideoBytes != 2*1024*1024*1024 || cfg.VideoRetention != 168*time.Hour { t.Fatalf("M3 defaults = %#v", cfg) }
	if cfg.ASRProvider != "aliyun_paraformer" || cfg.ASRModel != "paraformer-v2" || cfg.MonthlyASRBudgetCNY != 20 { t.Fatalf("M4 defaults = %#v", cfg) }
}

func TestLoadRejectsExcessiveWorkerConcurrency(t *testing.T) { t.Setenv("JWT_SIGNING_KEY_FILE", "test.key"); t.Setenv("WORKER_CONCURRENCY", "3"); if _, err := Load(); err == nil { t.Fatal("expected WORKER_CONCURRENCY error") } }

func TestLoadRejectsInvalidLogLevel(t *testing.T) {
	t.Setenv("LOG_LEVEL", "verbose")
	t.Setenv("JWT_SIGNING_KEY_FILE", "test.key")

	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want invalid log level error")
	}
}

func TestLoadRequiresSigningKeyFile(t *testing.T) {
	t.Setenv("JWT_SIGNING_KEY_FILE", "")
	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want missing signing key error")
	}
}
