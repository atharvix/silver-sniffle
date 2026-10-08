package main

import (
	"bufio"
	"context"
	"log"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
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
	publicURL string // e.g. https://kinjo.world
}

func main() {
	// JSON logs (journald / any log shipper can parse them). SetDefault also routes
	// every existing log.Printf through this handler.
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil)))

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
	// Our public origin: the OAuth callback lives on this server. Used for absolute
	// photo URLs and email links (never the request's spoofable Host header).
	publicURL := ""
	if u, err := url.Parse(cfg.LinkedInRedirectURL); err == nil {
		publicURL = u.Scheme + "://" + u.Host
	}
	hub := newHub(store, seed)
	hub.photoBase = publicURL

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
		publicURL: publicURL,
	}
	hub.wake = app.wakeUsers
	go hub.run(ctx)
	app.upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		Subprotocols:    []string{"kinjo"}, // echo the marker, never the token
		CheckOrigin: func(r *http.Request) bool {
			origin := r.Header.Get("Origin")
			return origin == "" || cfg.originAllowed(origin) // "" = native app
		},
	}

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           logRequests(app.cors(app.routes())),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second, // whole request: a 750 KB photo on a slow link fits
		WriteTimeout:      30 * time.Second, // LinkedIn callback makes two <=10 s calls
		IdleTimeout:       2 * time.Minute,  // (sockets are unaffected: gorilla clears deadlines on upgrade)
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

	// LinkedIn / Google OAuth (browser redirects, no Bearer token yet).
	li, g := linkedInProvider(a.cfg), googleProvider(a.cfg, a.publicURL)
	mux.HandleFunc("GET /auth/linkedin", a.startLogin(li))
	mux.HandleFunc("GET /auth/linkedin/callback", a.callback(li))
	mux.HandleFunc("GET /auth/google", a.startLogin(g))
	mux.HandleFunc("GET /auth/google/callback", a.callback(g))
	mux.HandleFunc("POST /auth/email/start", a.emailStart)
	mux.HandleFunc("POST /auth/email/verify", a.emailVerify)
	mux.HandleFunc("POST /auth/exchange", a.exchange)

	// Authenticated JSON API.
	mux.HandleFunc("GET /api/me", a.requireAuth(a.handleMe))
	mux.HandleFunc("PUT /api/profile", a.requireAuth(a.handleSaveProfile))
	mux.HandleFunc("POST /api/presence", a.requireAuth(a.handlePresence))
	mux.HandleFunc("GET /api/ble-token", a.requireAuth(a.handleBLEToken))
	mux.HandleFunc("POST /api/sightings", a.requireAuth(a.handleSightings))
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

	// Profile photos by reference (public: <img> can't send a Bearer token).
	mux.HandleFunc("GET /api/photo/{uid}", a.handlePhoto)

	// Realtime presence (auth via ?token=).
	mux.HandleFunc("GET /ws", a.serveWS)

	// Readiness: only "ok" if the database answers, so a monitor sees real outages.
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := a.store.pool.Ping(ctx); err != nil {
			http.Error(w, "db unavailable", http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("ok"))
	})
	return mux
}

// logRequests writes one structured line per request — method, path (never the
// query string: it can carry tokens), status, latency — tagged with a request ID
// that is echoed back, so a user's error report can be matched to the log line.
func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			next.ServeHTTP(w, r) // monitors poll this constantly; don't drown the logs
			return
		}
		id := r.Header.Get("X-Request-ID") // set by nginx when configured
		if id == "" {
			id, _ = randToken(8)
		}
		w.Header().Set("X-Request-ID", id)
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		start := time.Now()
		next.ServeHTTP(rec, r)
		slog.Info("request", "id", id, "method", r.Method, "path", r.URL.Path,
			"status", rec.status, "ms", time.Since(start).Milliseconds())
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// Hijack keeps WebSocket upgrades working: gorilla asserts http.Hijacker directly.
func (s *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	s.status = http.StatusSwitchingProtocols
	return s.ResponseWriter.(http.Hijacker).Hijack()
}
