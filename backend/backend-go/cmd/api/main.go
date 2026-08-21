package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/atharvix/kinjo-backend/internal/ai"
	"github.com/atharvix/kinjo-backend/internal/auth"
	"github.com/atharvix/kinjo-backend/internal/config"
	"github.com/atharvix/kinjo-backend/internal/database"
	"github.com/atharvix/kinjo-backend/internal/discovery"
	"github.com/atharvix/kinjo-backend/internal/email"
	kinjohttp "github.com/atharvix/kinjo-backend/internal/http"
	"github.com/atharvix/kinjo-backend/internal/observability"
	"github.com/atharvix/kinjo-backend/internal/presence"
	"github.com/atharvix/kinjo-backend/internal/profile"
	"github.com/atharvix/kinjo-backend/internal/storage"
	"github.com/prometheus/client_golang/prometheus"
)

func main() {
	// 1. Load Configuration
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// 2. Initialize Structured Logger
	logger := observability.NewLogger(cfg.LogLevel, cfg.IsProduction())
	slog.SetDefault(logger)

	logger.Info("starting Kinjo Go API server",
		slog.Int("port", cfg.Port),
		slog.String("environment", cfg.Environment),
		slog.String("log_level", cfg.LogLevel),
	)

	// 3. Initialize Observability & Metrics
	metrics := observability.NewMetrics(prometheus.DefaultRegisterer)

	// 4. Initialize Database
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	var db *database.DB
	if cfg.DatabaseURL != "" {
		db, err = database.New(ctx, cfg, logger)
		if err != nil {
			logger.Error("failed to connect to database", slog.String("error", err.Error()))
			os.Exit(1)
		}
		defer db.Close()

		// Run database migrations
		if err := db.Migrate(ctx); err != nil {
			logger.Error("failed to run database migrations", slog.String("error", err.Error()))
			os.Exit(1)
		}
	} else {
		logger.Warn("DATABASE_URL is not set; running in mock/demo mode without DB")
	}

	// 5. Initialize Storage Driver
	var storageService storage.Storage
	if cfg.StorageDriver == "local" || cfg.StorageDriver == "" {
		localStore, err := storage.NewLocalStorage(cfg.StorageDir, cfg.BaseURL)
		if err != nil {
			logger.Error("failed to initialize local storage", slog.String("error", err.Error()))
			os.Exit(1)
		}
		storageService = localStore
	}

	// 6. Initialize External Services
	var emailService email.Service
	if cfg.BrevoAPIKey != "" {
		emailService = email.NewBrevoService(cfg.BrevoAPIKey, cfg.BrevoSenderMail, logger, metrics)
		logger.Info("using Brevo email service")
	} else {
		emailService = email.NewMockService(logger)
		logger.Warn("using Mock email service (Brevo not configured)")
	}

	aiService := ai.NewOpenAIService(cfg.OpenAIAPIKey, cfg.OpenAIBaseURL, logger, metrics)

	// 7. Initialize Repositories & Services
	var authRepo auth.Repository
	var profileRepo profile.Repository
	var presenceRepo presence.Repository
	var discoveryRepo discovery.Repository

	if db != nil {
		authRepo = auth.NewRepository(db)
		profileRepo = profile.NewRepository(db)
		presenceRepo = presence.NewRepository(db)
		discoveryRepo = discovery.NewRepository(db)
	}

	authService := auth.NewService(authRepo, emailService, cfg, logger, metrics)
	profileService := profile.NewService(profileRepo, storageService, cfg, logger)
	presenceService := presence.NewService(presenceRepo, authService, logger)
	discoveryService := discovery.NewService(discoveryRepo, aiService, cfg, logger, metrics)

	// 8. Initialize HTTP Handlers
	handlers := kinjohttp.Handlers{
		Health:    observability.NewHealthHandler(db),
		Auth:      auth.NewHandler(authService, metrics),
		Profile:   profile.NewHandler(profileService),
		Presence:  presence.NewHandler(presenceService),
		Discovery: discovery.NewHandler(discoveryService),
	}

	router := kinjohttp.NewRouter(cfg, logger, metrics, handlers, authService)

	// 9. Start HTTP Server
	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      router,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		IdleTimeout:  cfg.IdleTimeout,
	}

	serverErrors := make(chan error, 1)
	go func() {
		logger.Info("HTTP server listening", slog.String("addr", server.Addr))
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrors <- err
		}
	}()

	// 10. Graceful Shutdown Handling
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-serverErrors:
		logger.Error("server startup error", slog.String("error", err.Error()))
		os.Exit(1)
	case sig := <-shutdown:
		logger.Info("shutdown signal received", slog.String("signal", sig.String()))

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer shutdownCancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Error("graceful server shutdown failed", slog.String("error", err.Error()))
			_ = server.Close()
		} else {
			logger.Info("server shutdown completed cleanly")
		}
	}
}
