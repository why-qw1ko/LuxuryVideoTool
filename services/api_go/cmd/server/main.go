package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/audit"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/auth"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/cleanup"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/config"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/database"
	ownedfiles "github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/files"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/httpapi"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/jobs"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/media"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/resolver"
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
	if err != nil { return err }
	defer db.Close()
	signingKey, err := os.ReadFile(cfg.JWTSigningKeyFile)
	if err != nil { return errors.New("read JWT signing key file") }
	tokenManager, err := auth.NewTokenManager(signingKey, cfg.AccessTokenTTL)
	if err != nil { return err }
	authService, err := auth.NewService(auth.NewSQLiteRepository(db), tokenManager, cfg.RefreshTokenTTL)
	if err != nil { return err }
	resolverService := resolver.NewService(resolver.NewDouyin(resolver.NewSafeClient(10*time.Second, 4<<20)), resolver.NewSQLiteCache(db), cfg.ResolverCacheTTL, resolver.DouyinResolverVersion)
	jobRepository := jobs.NewSQLiteRepository(db); jobService := jobs.NewService(jobRepository, resolverService)
	fileRepository := ownedfiles.NewRepository(db); storage, err := ownedfiles.NewStorage(cfg.DataDir); if err != nil { return err }
	worker := jobs.NewWorker(jobRepository, resolverService, media.NewHTTPDownloader(), fileRepository, storage, jobs.WorkerConfig{
		Owner: "server", Concurrency: cfg.WorkerConcurrency, Lease: 60*time.Second, Heartbeat: 15*time.Second,
		Poll: time.Second, MaxVideoBytes: cfg.MaxVideoBytes, VideoRetention: cfg.VideoRetention,
	})
	jobService.SetForceCancel(worker.Cancel)
	cleanupService := cleanup.New(fileRepository, storage, time.Hour)
	readiness := func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		return database.Ready(ctx, db, cfg.DataDir)
	}

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler: httpapi.New(httpapi.Dependencies{
			Build: version.Current(), Auth: authService, Jobs: jobService, Files: fileRepository, Storage: storage, Audit: audit.New(db, signingKey),
			Ready: readiness, LoginRateLimit: cfg.LoginRateLimit,
		}),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	workerErr := make(chan error, 1); go func() { workerErr <- worker.Run(ctx) }(); go cleanupService.Run(ctx)

	errCh := make(chan error, 1)
	go func() {
		logger.Info("server starting", "addr", cfg.HTTPAddr, "version", version.Version)
		errCh <- server.ListenAndServe()
	}()

	select {
	case err := <-workerErr:
		if err != nil { return err }; return nil
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
