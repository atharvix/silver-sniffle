package http

import (
	"log/slog"
	"net/http"
	"os"

	"github.com/atharvix/kinjo-backend/internal/auth"
	"github.com/atharvix/kinjo-backend/internal/config"
	"github.com/atharvix/kinjo-backend/internal/discovery"
	"github.com/atharvix/kinjo-backend/internal/middleware"
	"github.com/atharvix/kinjo-backend/internal/observability"
	"github.com/atharvix/kinjo-backend/internal/presence"
	"github.com/atharvix/kinjo-backend/internal/profile"
	"github.com/atharvix/kinjo-backend/internal/notification"
	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Handlers struct {
	Health       *observability.HealthHandler
	Auth         *auth.Handler
	Profile      *profile.Handler
	Presence     *presence.Handler
	Discovery    *discovery.Handler
	Notification *notification.Handler
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
		_ = os.MkdirAll(cfg.StorageDir, 0755)
		fileServer := http.FileServer(http.Dir(cfg.StorageDir))
		r.Handle("/uploads/*", http.StripPrefix("/uploads/", fileServer))
	}

	// API Router setup (supports both /api and /api/v1 for backwards/forwards compatibility)
	registerAPIRoutes := func(api chi.Router) {
		// Public Auth Endpoints
		api.Post("/auth/sign-up", handlers.Auth.SignUp)
		api.Post("/auth/sign-in", handlers.Auth.SignIn)
		api.Post("/auth/google", handlers.Auth.GoogleSignIn)
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
			protected.Delete("/auth/account", handlers.Auth.DeleteAccount)
			protected.Post("/profiles/location", handlers.Presence.UpdateLocation)
			protected.Post("/profiles/heartbeat", handlers.Presence.Heartbeat)
			protected.Get("/profiles/nearby", handlers.Discovery.GetNearbyProfiles)

			// Notification Endpoints
			if handlers.Notification != nil {
				protected.Post("/notifications/register-token", handlers.Notification.RegisterToken)
				protected.Post("/notifications/send-custom", handlers.Notification.SendCustomNotification)
			}
		})
	}

	r.Route("/api", registerAPIRoutes)
	r.Route("/api/v1", registerAPIRoutes)

	return r
}
