package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

const defaultHTTPAddr = "127.0.0.1:8080"

type Config struct {
	HTTPAddr          string
	LogLevel          slog.Level
	DatabasePath      string
	DataDir           string
	JWTSigningKeyFile string
	AccessTokenTTL    time.Duration
	RefreshTokenTTL   time.Duration
	LoginRateLimit    int
	ResolverCacheTTL  time.Duration
	DouyinTtwidFile   string
	DouyinBrowserPath string
	WorkerConcurrency int
	FFmpegPath        string
	FFprobePath       string
	MaxVideoBytes     int64
	VideoRetention    time.Duration
	TempRetention     time.Duration
	PublicBaseURL     string
	AllowInsecureProviderSettings bool
	ASRProvider       string
	ASRModel          string
	DashScopeAPIKey   string
	DashScopeEndpoint string
	ASRVocabularyID   string
	SiliconFlowAPIKey string
	DailyASRBudgetCNY float64
	MonthlyASRBudgetCNY float64
	ASRPricePerMinuteCNY float64
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

	accessTTL, err := durationEnv("ACCESS_TOKEN_TTL", 15*time.Minute)
	if err != nil {
		return Config{}, err
	}
	refreshTTL, err := durationEnv("REFRESH_TOKEN_TTL", 30*24*time.Hour)
	if err != nil {
		return Config{}, err
	}
	loginLimit, err := positiveIntEnv("LOGIN_RATE_LIMIT_PER_MINUTE", 5)
	if err != nil {
		return Config{}, err
	}
	resolverCacheTTL, err := durationEnv("RESOLVER_CACHE_TTL", 6*time.Hour)
	if err != nil { return Config{}, err }
	workerConcurrency, err := boundedIntEnv("WORKER_CONCURRENCY", 1, 1, 2); if err != nil { return Config{}, err }
	maxVideoBytes, err := positiveInt64Env("MAX_VIDEO_BYTES", 2*1024*1024*1024); if err != nil { return Config{}, err }
	videoHours, err := positiveIntEnv("RETENTION_VIDEO_HOURS", 168); if err != nil { return Config{}, err }
	tempHours, err := positiveIntEnv("RETENTION_TEMP_HOURS", 24); if err != nil { return Config{}, err }
	dailyBudget, err := nonNegativeFloatEnv("DAILY_ASR_BUDGET_CNY", 5); if err != nil { return Config{}, err }
	monthlyBudget, err := nonNegativeFloatEnv("MONTHLY_ASR_BUDGET_CNY", 20); if err != nil { return Config{}, err }
	pricePerMinute, err := nonNegativeFloatEnv("ASR_PRICE_PER_MINUTE_CNY", 0); if err != nil { return Config{}, err }

	cfg := Config{
		HTTPAddr:          addr,
		LogLevel:          level,
		DatabasePath:      stringEnv("DATABASE_PATH", "./data/app.db"),
		DataDir:           stringEnv("DATA_DIR", "./data"),
		JWTSigningKeyFile: strings.TrimSpace(os.Getenv("JWT_SIGNING_KEY_FILE")),
		AccessTokenTTL:    accessTTL,
		RefreshTokenTTL:   refreshTTL,
		LoginRateLimit:    loginLimit,
		ResolverCacheTTL:  resolverCacheTTL,
		DouyinTtwidFile:   stringEnv("DOUYIN_TTWID_FILE", "./data/douyin_ttwid.json"),
		DouyinBrowserPath: strings.TrimSpace(os.Getenv("DOUYIN_BROWSER_PATH")),
		WorkerConcurrency: workerConcurrency,
		FFmpegPath:        stringEnv("FFMPEG_PATH", "ffmpeg"),
		FFprobePath:       stringEnv("FFPROBE_PATH", "ffprobe"),
		MaxVideoBytes:     maxVideoBytes,
		VideoRetention:    time.Duration(videoHours)*time.Hour,
		TempRetention:     time.Duration(tempHours)*time.Hour,
		PublicBaseURL:     strings.TrimRight(strings.TrimSpace(os.Getenv("PUBLIC_BASE_URL")), "/"),
		AllowInsecureProviderSettings: boolEnv("ALLOW_INSECURE_PROVIDER_SETTINGS", false),
		ASRProvider:       stringEnv("ASR_PROVIDER", "siliconflow_sensevoice"),
		ASRModel:          stringEnv("ASR_MODEL", "FunAudioLLM/SenseVoiceSmall"),
		DashScopeAPIKey:   strings.TrimSpace(os.Getenv("ALIYUN_DASHSCOPE_API_KEY")),
		DashScopeEndpoint: stringEnv("DASHSCOPE_ENDPOINT", "https://dashscope.aliyuncs.com/api/v1"),
		ASRVocabularyID:   strings.TrimSpace(os.Getenv("ASR_VOCABULARY_ID")),
		SiliconFlowAPIKey: strings.TrimSpace(os.Getenv("SILICONFLOW_API_KEY")),
		DailyASRBudgetCNY: dailyBudget,
		MonthlyASRBudgetCNY: monthlyBudget,
		ASRPricePerMinuteCNY: pricePerMinute,
	}
	if cfg.JWTSigningKeyFile == "" {
		return Config{}, fmt.Errorf("JWT_SIGNING_KEY_FILE is required")
	}
	if len(cfg.DatabasePath) > 4096 || len(cfg.DataDir) > 4096 || len(cfg.JWTSigningKeyFile) > 4096 {
		return Config{}, fmt.Errorf("configured path is too long")
	}
	if cfg.ASRProvider != "siliconflow_sensevoice" { return Config{}, fmt.Errorf("ASR_PROVIDER must be siliconflow_sensevoice") }
	if cfg.DashScopeAPIKey != "" && cfg.PublicBaseURL == "" { return Config{}, fmt.Errorf("PUBLIC_BASE_URL is required when Aliyun ASR is configured") }
	return cfg, nil
}

func stringEnv(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func durationEnv(name string, fallback time.Duration) (time.Duration, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback, nil
	}
	value, err := time.ParseDuration(raw)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("%s must be a positive duration", name)
	}
	return value, nil
}

func positiveIntEnv(name string, fallback int) (int, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", name)
	}
	return value, nil
}

func boundedIntEnv(name string, fallback, minValue, maxValue int) (int, error) { value, err := positiveIntEnv(name, fallback); if err != nil { return 0, err }; if value < minValue || value > maxValue { return 0, fmt.Errorf("%s must be between %d and %d", name, minValue, maxValue) }; return value, nil }
func positiveInt64Env(name string, fallback int64) (int64, error) { raw := strings.TrimSpace(os.Getenv(name)); if raw == "" { return fallback, nil }; value, err := strconv.ParseInt(raw, 10, 64); if err != nil || value <= 0 { return 0, fmt.Errorf("%s must be a positive integer", name) }; return value, nil }
func nonNegativeFloatEnv(name string, fallback float64) (float64, error) { raw := strings.TrimSpace(os.Getenv(name)); if raw == "" { return fallback, nil }; value, err := strconv.ParseFloat(raw, 64); if err != nil || value < 0 { return 0, fmt.Errorf("%s must be a non-negative number", name) }; return value, nil }
func boolEnv(name string, fallback bool) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}
