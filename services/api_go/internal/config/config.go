package config

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
)

const defaultHTTPAddr = "127.0.0.1:8080"

type Config struct {
	HTTPAddr string
	LogLevel slog.Level
}

func Load() (Config, error) {
	addr := strings.TrimSpace(os.Getenv("HTTP_ADDR"))
	if addr == "" {
		addr = defaultHTTPAddr
	}

	var level slog.Level
	if raw := strings.TrimSpace(os.Getenv("LOG_LEVEL")); raw != "" {
		if err := level.UnmarshalText([]byte(raw)); err != nil {
			return Config{}, fmt.Errorf("parse LOG_LEVEL: %w", err)
		}
	}

	return Config{HTTPAddr: addr, LogLevel: level}, nil
}

