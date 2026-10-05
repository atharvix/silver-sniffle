package main

import (
	"context"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
)

// App holds the shared dependencies passed to every handler.
type App struct {
	cfg       Config
	store     *Store
	hub       *Hub
	fcm       *FCM // nil when FCM isn't configured
	http      *http.Client
	upgrader  websocket.Upgrader
	startedAt time.Time
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix("kinjo: ")

	cfg, err := loadConfig()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	// Root context cancelled on SIGINT/SIGTERM for a clean shutdown.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	store, err := openStore(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer store.Close()

	seed, err := store.loadRecentPresence(ctx)
	if err != nil {
		log.Printf("warning: could not load presence: %v", err)
		seed = map[string]*Pres{}
	}
	hub := newHub(store, seed)
	go hub.runSweeper(ctx)

	fcm, err := newFCM(ctx, cfg)
	if err != nil {
		log.Printf("warning: FCM disabled: %v", err)
	} else if fcm != nil {
		log.Printf("FCM push enabled (project %s)", fcm.projectID)
	}

	app := &App{
		cfg:       cfg,
		store:     store,
		hub:       hub,
		fcm:       fcm,
		http:      &http.Client{Timeout: 10 * time.Second},
		startedAt: time.Now(),
	}
	app.upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			origin := r.Header.Get("Origin")
			return origin == "" || cfg.originAllowed(origin) // "" = native app
		},
	}

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           app.cors(app.routes()),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("listening on :%s", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
}

func (a *App) routes() http.Handler {
	mux := http.NewServeMux()

	// LinkedIn OAuth (browser redirects, no Bearer token yet).
	mux.HandleFunc("GET /auth/linkedin", a.startLogin)
	mux.HandleFunc("GET /auth/linkedin/callback", a.callback)

	// Authenticated JSON API.
	mux.HandleFunc("GET /api/me", a.requireAuth(a.handleMe))
	mux.HandleFunc("PUT /api/profile", a.requireAuth(a.handleSaveProfile))
	mux.HandleFunc("POST /api/presence", a.requireAuth(a.handlePresence))
	mux.HandleFunc("POST /api/logout", a.requireAuth(a.handleLogout))
	mux.HandleFunc("DELETE /api/account", a.requireAuth(a.handleDeleteAccount))

	// Push devices + notifications feed.
	mux.HandleFunc("POST /api/device", a.requireAuth(a.handleRegisterDevice))
	mux.HandleFunc("DELETE /api/device", a.requireAuth(a.handleDeleteDevice))
	mux.HandleFunc("GET /api/notifications", a.requireAuth(a.handleNotifications))
	mux.HandleFunc("POST /api/notifications/read", a.requireAuth(a.handleReadNotifications))

	// Email verification: request a link (authed) + the link target (public).
	mux.HandleFunc("POST /api/verify/send", a.requireAuth(a.handleSendVerification))
	mux.HandleFunc("GET /auth/verify", a.handleVerifyEmail)

	// Native advertising: sponsored cards in the feed + separate ad analytics.
	mux.HandleFunc("GET /api/ads", a.requireAuth(a.handleGetAds))
	mux.HandleFunc("POST /api/ad-event", a.requireAuth(a.handleAdEvent))

	// Admin panel API (guarded by ADMIN_TOKEN; off entirely if that's unset).
	mux.HandleFunc("GET /admin/metrics", a.requireAdmin(a.handleAdminMetrics))
	mux.HandleFunc("GET /admin/users", a.requireAdmin(a.handleAdminUsers))
	mux.HandleFunc("POST /admin/email", a.requireAdmin(a.handleAdminEmail))
	mux.HandleFunc("POST /admin/notify", a.requireAdmin(a.handleAdminNotify))
	mux.HandleFunc("GET /admin/ads", a.requireAdmin(a.handleAdminAds))
	mux.HandleFunc("POST /admin/ads", a.requireAdmin(a.handleAdminCreateAd))
	mux.HandleFunc("POST /admin/ads/toggle", a.requireAdmin(a.handleAdminToggleAd))
	mux.HandleFunc("DELETE /admin/ads", a.requireAdmin(a.handleAdminDeleteAd))

	// Realtime presence (auth via ?token=).
	mux.HandleFunc("GET /ws", a.serveWS)

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	return mux
}
