package http

import (
	"log/slog"
	"net/http"
	"os"

	"github.com/atharvix/kinjo-backend/internal/auth"
	"github.com/atharvix/kinjo-backend/internal/config"
	"github.com/atharvix/kinjo-backend/internal/connection"
	"github.com/atharvix/kinjo-backend/internal/discovery"
	"github.com/atharvix/kinjo-backend/internal/middleware"
	"github.com/atharvix/kinjo-backend/internal/observability"
	"github.com/atharvix/kinjo-backend/internal/presence"
	"github.com/atharvix/kinjo-backend/internal/profile"
	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Handlers struct {
	Health    *observability.HealthHandler
	Auth      *auth.Handler
	Profile   *profile.Handler
	Presence  *presence.Handler
	Discovery *discovery.Handler
	Connection *connection.Handler
}

func NewRouter(
	cfg *config.Config,
	logger *slog.Logger,
	metrics *observability.Metrics,
	handlers Handlers,
	tokenValidator middleware.TokenValidator,
) http.Handler {
	r := chi.NewRouter()

	// Base middlewares
	r.Use(middleware.RequestID)
	r.Use(chimiddleware.RealIP)
	r.Use(middleware.Logger(logger, metrics))
	r.Use(middleware.Recovery(logger))
	r.Use(middleware.CORS(cfg.AllowedOrigins))
	r.Use(middleware.SecurityHeaders(cfg.IsProduction()))

	// Metrics endpoint
	r.Handle("/metrics", promhttp.Handler())

	// Health check endpoints
	r.Get("/api/healthz", handlers.Health.Healthz)
	r.Get("/api/readyz", handlers.Health.Readyz)

	// Static file serving for uploads (local storage)
	if cfg.StorageDriver == "local" {
		if _, err := os.Stat(cfg.StorageDir); err == nil {
			fileServer := http.FileServer(http.Dir(cfg.StorageDir))
			r.Handle("/uploads/*", http.StripPrefix("/uploads/", fileServer))
		}
	}

	// API Routes
	r.Route("/api", func(api chi.Router) {
		// Public Auth Endpoints
		api.Post("/auth/send-otp", handlers.Auth.SendOTP)
		api.Post("/auth/verify-otp", handlers.Auth.VerifyOTP)
		api.Post("/auth/send-welcome", handlers.Auth.SendWelcome)

		// Public offline endpoint (sendBeacon cannot set Authorization header; carries token in body)
		api.Post("/profiles/offline", handlers.Presence.GoOffline)

		// Token-Gated Profile & Presence Endpoints
		api.Group(func(protected chi.Router) {
			protected.Use(middleware.RequireAuth(tokenValidator))

			protected.Post("/profiles", handlers.Profile.UpsertProfile)
			protected.Get("/profiles/me", handlers.Profile.GetMyProfile)
			protected.Post("/profiles/location", handlers.Presence.UpdateLocation)
			protected.Post("/profiles/heartbeat", handlers.Presence.Heartbeat)
			protected.Get("/profiles/nearby", handlers.Discovery.GetNearbyProfiles)
			protected.Post("/connections", handlers.Connection.Create)
			protected.Get("/connections/incoming", handlers.Connection.ListIncoming)
		})
	})

	return r
}
