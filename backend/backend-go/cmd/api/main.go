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

	"github.com/atharvix/kinjo-backend/internal/auth"
	"github.com/atharvix/kinjo-backend/internal/config"
	"github.com/atharvix/kinjo-backend/internal/database"
	"github.com/atharvix/kinjo-backend/internal/discovery"
	"github.com/atharvix/kinjo-backend/internal/email"
	kinjohttp "github.com/atharvix/kinjo-backend/internal/http"
	"github.com/atharvix/kinjo-backend/internal/notification"
	"github.com/atharvix/kinjo-backend/internal/observability"
	"github.com/atharvix/kinjo-backend/internal/presence"
	"github.com/atharvix/kinjo-backend/internal/profile"
	"github.com/atharvix/kinjo-backend/internal/storage"
	"github.com/joho/godotenv"
	"github.com/prometheus/client_golang/prometheus"
)

func main() {
	_ = godotenv.Overload()

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

	// A database is mandatory: every repository (auth, profile, presence,
	// discovery, notifications) is backed by Postgres. Starting without one
	// would leave those repositories nil and turn every request into a 500.
	if cfg.DatabaseURL == "" {
		logger.Error("DATABASE_URL is required; refusing to start without a database")
		os.Exit(1)
	}

	db, err := database.New(ctx, cfg, logger)
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

	// 5. Initialize Storage Driver
	var storageService storage.Storage
	if (cfg.StorageDriver == "supabase" || cfg.StorageDriver == "") && cfg.SupabaseURL != "" && cfg.SupabaseServiceRoleKey != "" {
		supaStore, err := storage.NewSupabaseStorage(cfg.SupabaseURL, cfg.SupabaseServiceRoleKey, cfg.SupabaseBucket)
		if err != nil {
			logger.Error("failed to initialize Supabase storage", slog.String("error", err.Error()))
			os.Exit(1)
		}
		storageService = supaStore
		logger.Info("using Supabase Cloud Storage provider", slog.String("bucket", cfg.SupabaseBucket))
	} else {
		localStore, err := storage.NewLocalStorage(cfg.StorageDir, cfg.BaseURL)
		if err != nil {
			logger.Error("failed to initialize local storage", slog.String("error", err.Error()))
			os.Exit(1)
		}
		storageService = localStore
		logger.Info("using Local File Storage provider", slog.String("dir", cfg.StorageDir))
	}

	// 6. Initialize External Services
	var emailService email.Service
	if cfg.SMTPHost != "" {
		emailService = email.NewSMTPService(
			cfg.SMTPHost,
			cfg.SMTPPort,
			cfg.SMTPUsername,
			cfg.SMTPPassword,
			cfg.SMTPSenderEmail,
			cfg.SMTPSenderName,
			cfg.SMTPEncryption,
			logger,
			metrics,
		)
		logger.Info("using SMTP email service", slog.String("host", cfg.SMTPHost), slog.Int("port", cfg.SMTPPort))
	} else {
		emailService = email.NewMockService(logger)
		logger.Warn("using Mock email service (SMTP_HOST not configured)")
	}

	fcmService := notification.NewFCMService(cfg.FCMProjectID, cfg.FCMAccountKey, cfg.FCMServerKey, logger)
	if fcmService.IsConfigured() {
		logger.Info("using FCM push notification service")
	} else {
		logger.Warn("using Mock FCM push notification service (credentials not configured)")
	}

	// 7. Initialize Repositories & Services
	authRepo := auth.NewRepository(db)
	profileRepo := profile.NewRepository(db)
	presenceRepo := presence.NewRepository(db)
	discoveryRepo := discovery.NewRepository(db)
	notificationRepo := notification.NewRepository(db)

	authService := auth.NewService(authRepo, emailService, cfg, logger, metrics)
	profileService := profile.NewService(profileRepo, storageService, cfg, emailService, logger)
	discoveryService := discovery.NewService(discoveryRepo, cfg, logger, metrics)
	notificationService := notification.NewService(notificationRepo, fcmService, discoveryRepo)
	presenceService := presence.NewService(presenceRepo, authService, logger)
	presenceService.SetNotifier(notificationService)
	notificationHandler := notification.NewHandler(notificationService, cfg.AdminEmails)
	if len(cfg.AdminEmails) == 0 {
		logger.Warn("ADMIN_EMAILS is not set; the broadcast notification endpoint is disabled")
	}

	// 8. Initialize HTTP Handlers
	handlers := kinjohttp.Handlers{
		Health:       observability.NewHealthHandler(db),
		Auth:         auth.NewHandler(authService, metrics),
		Profile:      profile.NewHandler(profileService),
		Presence:     presence.NewHandler(presenceService),
		Discovery:    discovery.NewHandler(discoveryService),
		Notification: notificationHandler,
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
