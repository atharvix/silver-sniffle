// Command server is the Kinjo API: sessions, profiles, photos and the realtime nearby hub.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		slog.Error("DATABASE_URL is required")
		os.Exit(1)
	}
	store, err := openStore(ctx, dbURL)
	if err != nil {
		slog.Error("database", "err", err)
		os.Exit(1)
	}
	defer store.db.Close()

	app := &App{store: store, hub: newHub(), publicURL: strings.TrimSuffix(env("PUBLIC_URL", "http://localhost:8080"), "/")}
	if creds, project := os.Getenv("FCM_CREDENTIALS_FILE"), os.Getenv("FCM_PROJECT_ID"); creds != "" && project != "" {
		waker, err := newWaker(ctx, creds, project, store)
		if err != nil {
			slog.Error("push", "err", err)
			os.Exit(1)
		}
		waker.run(ctx, 8)
		app.hub.wake = waker.enqueue
		slog.Info("wake pushes on", "project", project)
	} else {
		slog.Warn("wake pushes off: set FCM_CREDENTIALS_FILE and FCM_PROJECT_ID")
	}
	go func() {
		t := time.NewTicker(5 * time.Second)
		defer t.Stop()
		for {
			select {
			case now := <-t.C:
				app.hub.sweep(now)
			case <-ctx.Done():
				return
			}
		}
	}()

	srv := &http.Server{Addr: env("ADDR", ":8080"), Handler: app.routes(), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()
	slog.Info("listening", "addr", srv.Addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("server", "err", err)
		os.Exit(1)
	}
}
