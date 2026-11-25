// Command server is the Kinjo API: sessions, profiles and the realtime nearby hub.
// Postgres keeps accounts and cards, Redis the live state shared by every server,
// NATS JetStream the wake queue, S3-compatible storage (R2) the photos.
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

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/redis/go-redis/v9"
)

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func must(key string) string {
	v := os.Getenv(key)
	if v == "" {
		slog.Error(key + " is required")
		os.Exit(1)
	}
	return v
}

func fatal(what string, err error) {
	slog.Error(what, "err", err)
	os.Exit(1)
}

// A fixed, warm set of Redis connections per node: a crowd opening the app at once queues for a few
// milliseconds instead of opening hundreds of connections in the same second (which the OS refuses).
const redisPool, redisWarm = 64, 16

// openRedis: REDIS_CLUSTER (comma-separated host:port) for a cluster, otherwise REDIS_URL for one node.
func openRedis() redis.UniversalClient {
	if nodes := os.Getenv("REDIS_CLUSTER"); nodes != "" {
		return redis.NewClusterClient(&redis.ClusterOptions{Addrs: strings.Split(nodes, ","), Password: os.Getenv("REDIS_PASSWORD"),
			PoolSize: redisPool, MinIdleConns: redisWarm})
	}
	opts, err := redis.ParseURL(env("REDIS_URL", "redis://127.0.0.1:6379/0"))
	if err != nil {
		fatal("REDIS_URL", err)
	}
	opts.PoolSize, opts.MinIdleConns = redisPool, redisWarm
	return redis.NewClient(opts)
}

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	store, err := openStore(ctx, must("DATABASE_URL"))
	if err != nil {
		fatal("database", err)
	}
	defer store.db.Close()

	rdb := openRedis()
	defer rdb.Close()
	if err := rdb.Ping(ctx).Err(); err != nil {
		fatal("redis", err)
	}

	nc, err := nats.Connect(env("NATS_URL", nats.DefaultURL), nats.Name("kinjo-server"), nats.MaxReconnects(-1))
	if err != nil {
		fatal("nats", err)
	}
	defer nc.Drain()
	js, err := jetstream.New(nc)
	if err != nil {
		fatal("jetstream", err)
	}
	if err := openWakeStream(ctx, js); err != nil {
		fatal("wake stream", err)
	}

	photos := newPhotos(must("S3_ENDPOINT"), env("S3_REGION", "auto"), must("S3_BUCKET"),
		must("S3_ACCESS_KEY"), must("S3_SECRET_KEY"), strings.TrimSuffix(must("PHOTO_BASE_URL"), "/"))

	hub := newHub(rdb)
	hub.wake = queueWake(nc)
	hub.revoked = store.forgetSessions
	go hub.listen(ctx)
	go func() {
		t := time.NewTicker(5 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				hub.sweep(ctx)
				store.forgetSessions("") // drops expired cache entries
			case <-ctx.Done():
				return
			}
		}
	}()

	// Card rebuilds are bundled: changes mark decks, and this sends them at most every 250 ms.
	go func() {
		t := time.NewTicker(250 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				hub.flushDirty(ctx)
			case <-ctx.Done():
				return
			}
		}
	}()

	// Daily totals for the admin dashboard move from Redis to Postgres every minute.
	go func() {
		t := time.NewTicker(time.Minute)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				if err := flushCounts(ctx, rdb, store); err != nil {
					slog.Warn("flush counts", "err", err)
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	if creds, project := os.Getenv("FCM_CREDENTIALS_FILE"), os.Getenv("FCM_PROJECT_ID"); creds != "" && project != "" {
		pusher, err := newPusher(ctx, creds, project, store)
		if err != nil {
			fatal("push", err)
		}
		go func() {
			if err := pusher.run(ctx, js, 16); err != nil {
				fatal("pusher", err)
			}
		}()
		slog.Info("wake pushes on", "project", project)
	} else {
		slog.Warn("wake pushes off: set FCM_CREDENTIALS_FILE and FCM_PROJECT_ID (wakes still queue for 2 minutes)")
	}

	app := &App{store: store, hub: hub, photos: photos, adminToken: os.Getenv("ADMIN_TOKEN")}
	srv := &http.Server{Addr: env("ADDR", ":8001"), Handler: app.routes(), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()
	slog.Info("listening", "addr", srv.Addr, "server", hub.id)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		fatal("server", err)
	}
}
