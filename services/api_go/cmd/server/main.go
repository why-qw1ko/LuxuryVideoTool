package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/asr"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/audit"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/auth"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/cleanup"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/config"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/database"
	ownedfiles "github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/files"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/httpapi"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/jobs"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/media"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/releases"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/resolver"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/settings"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/version"
)

const shutdownTimeout = 10 * time.Second

func main() {
	if err := run(); err != nil {
		slog.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))
	slog.SetDefault(logger)
	db, err := database.Open(context.Background(), cfg.DatabasePath)
	if err != nil {
		return err
	}
	defer db.Close()
	signingKey, err := os.ReadFile(cfg.JWTSigningKeyFile)
	if err != nil {
		return errors.New("read JWT signing key file")
	}
	tokenManager, err := auth.NewTokenManager(signingKey, cfg.AccessTokenTTL)
	if err != nil {
		return err
	}
	authService, err := auth.NewService(auth.NewSQLiteRepository(db), tokenManager, cfg.RefreshTokenTTL)
	if err != nil {
		return err
	}
	safeClient := resolver.NewSafeClient(10*time.Second, 4<<20)
	resolverService := resolver.NewService(resolver.NewDouyin(safeClient, resolver.NewTtwidManager(safeClient, cfg.DouyinTtwidFile, cfg.DouyinBrowserPath)), resolver.NewSQLiteCache(db), cfg.ResolverCacheTTL, resolver.DouyinResolverVersion)
	jobRepository := jobs.NewSQLiteRepository(db)
	jobService := jobs.NewService(jobRepository, resolverService)
	fileRepository := ownedfiles.NewRepository(db)
	storage, err := ownedfiles.NewStorage(cfg.DataDir)
	if err != nil {
		return err
	}
	androidReleases, err := releases.New(db, cfg.DataDir)
	if err != nil {
		return err
	}
	runtimeSettings, err := settings.New(db, signingKey)
	if err != nil {
		return err
	}
	runtimeSettings.SetFallback(settings.AliyunKey, cfg.DashScopeAPIKey)
	runtimeSettings.SetFallback(settings.SiliconFlowKey, cfg.SiliconFlowAPIKey)
	runtimeSettings.SetFallback(settings.DeepSeekKey, cfg.DeepSeekAPIKey)
	runtimeSettings.SetFallback(settings.ASRModel, cfg.ASRModel)
	signer := ownedfiles.NewSigner(signingKey, cfg.PublicBaseURL)
	aliyunAvailable := publicHTTPSURL(cfg.PublicBaseURL)
	asrService := asr.NewService(nil, nil, asr.NewRepository(db), asr.Budget{
		DailyCNY: cfg.DailyASRBudgetCNY, MonthlyCNY: cfg.MonthlyASRBudgetCNY, PricePerMinuteCNY: cfg.ASRPricePerMinuteCNY,
	})
	providerFactory := func(config settings.ProviderConfig) asr.Provider {
		if !config.Available {
			return nil
		}
		switch config.ID {
		case settings.ProviderSiliconFlow:
			return asr.NewSiliconFlow(config.APIKey, "", config.Model)
		case settings.ProviderAliyun:
			provider := asr.NewParaformer(config.APIKey, cfg.DashScopeEndpoint, config.Model)
			provider.VocabularyID = cfg.ASRVocabularyID
			return provider
		case settings.ProviderDeepSeek:
			return asr.NewDeepSeek(config.APIKey, "")
		default:
			return nil
		}
	}
	asrService.SetProviderRouter(func(ctx context.Context, requested string) (asr.Provider, asr.Provider) {
		providers, err := runtimeSettings.Providers(ctx, aliyunAvailable)
		if err != nil {
			return nil, nil
		}
		if requested != "" {
			chosen := providers.Config(requested)
			if !chosen.Enabled {
				return nil, nil
			}
			return providerFactory(chosen), nil
		}
		primary := providers.Config(providers.Status.EffectiveDefaultProvider)
		aliyun := providers.Config(settings.ProviderAliyun)
		if providers.Status.DefaultProvider == "" && primary.ID == settings.ProviderSiliconFlow && aliyun.Enabled {
			return providerFactory(primary), providerFactory(aliyun)
		}
		return providerFactory(primary), nil
	})
	transcriber := &jobs.Transcriber{ASR: asrService, FFmpeg: media.FFmpeg{Path: cfg.FFmpegPath, Timeout: 30 * time.Minute, LogLimit: 16 * 1024}, Probe: media.Probe{Path: cfg.FFprobePath, Timeout: time.Minute}, Signer: signer, FileRepo: fileRepository, Storage: storage, TempRetention: cfg.TempRetention}
	thumbnails := media.NewThumbnailer(cfg.FFmpegPath)
	worker := jobs.NewWorker(jobRepository, resolverService, media.NewHTTPDownloader(), fileRepository, storage, transcriber, jobs.WorkerConfig{
		Owner: "server", Concurrency: cfg.WorkerConcurrency, Lease: 60 * time.Second, Heartbeat: 15 * time.Second,
		Poll: time.Second, MaxVideoBytes: cfg.MaxVideoBytes, VideoRetention: cfg.VideoRetention, Thumbnails: thumbnails,
	})
	jobService.SetForceCancel(worker.Cancel)
	cleanupService := cleanup.New(fileRepository, storage, time.Hour)
	readiness := func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		return database.Ready(ctx, db, cfg.DataDir)
	}

	server := &http.Server{
		Addr: cfg.HTTPAddr,
		Handler: httpapi.New(httpapi.Dependencies{
			Build: version.Current(), Auth: authService, Jobs: jobService, Files: fileRepository, Storage: storage, Thumbnails: thumbnails, Covers: media.NewCoverFetcher(), ASRSigner: signer, Audit: audit.New(db, signingKey),
			AndroidReleases: androidReleases, Ready: readiness, LoginRateLimit: cfg.LoginRateLimit, Settings: runtimeSettings, AliyunAvailable: aliyunAvailable,
			ASRProviderFactory: providerFactory, AllowInsecureProviderSettings: cfg.AllowInsecureProviderSettings,
		}),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Minute,
		IdleTimeout:       60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	workerErr := make(chan error, 1)
	go func() { workerErr <- worker.Run(ctx) }()
	go cleanupService.Run(ctx)

	errCh := make(chan error, 1)
	go func() {
		logger.Info("server starting", "addr", cfg.HTTPAddr, "version", version.Version)
		errCh <- server.ListenAndServe()
	}()

	select {
	case err := <-workerErr:
		if err != nil {
			return err
		}
		return nil
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	}
}

func publicHTTPSURL(value string) bool {
	parsed, err := url.Parse(value)
	return err == nil && parsed.Scheme == "https" && parsed.Hostname() != "" &&
		parsed.User == nil && parsed.RawQuery == "" && !parsed.ForceQuery && parsed.Fragment == ""
}
