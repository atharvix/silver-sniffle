package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// K2: a database outage must answer 503 (retry), never 401 (app deletes its token).
func TestRequireAuthDBDown(t *testing.T) {
	pool, err := pgxpool.New(context.Background(), "postgres://x@127.0.0.1:1/x?connect_timeout=1")
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	a := &App{store: &Store{pool: pool}}
	h := a.requireAuth(func(http.ResponseWriter, *http.Request, string) { t.Fatal("handler ran") })

	for _, tc := range []struct {
		auth string
		want int
	}{{"", http.StatusUnauthorized}, {"Bearer some-token", http.StatusServiceUnavailable}} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
		if tc.auth != "" {
			req.Header.Set("Authorization", tc.auth)
		}
		h(rec, req)
		if rec.Code != tc.want {
			t.Errorf("auth %q: got %d, want %d", tc.auth, rec.Code, tc.want)
		}
	}
}

// K3: expired cache entries (which hold photos) must be evicted.
func TestProfileCachePurge(t *testing.T) {
	pc := newProfileCache(nil)
	pc.items["old"] = profEntry{at: time.Now().Add(-profileTTL - time.Second)}
	pc.items["new"] = profEntry{at: time.Now()}
	pc.purge()
	if _, ok := pc.items["old"]; ok {
		t.Error("expired entry kept")
	}
	if _, ok := pc.items["new"]; !ok {
		t.Error("fresh entry evicted")
	}
}

// K5: only inline raster images (and legacy LinkedIn CDN links) are accepted.
func TestValidPhoto(t *testing.T) {
	for s, want := range map[string]bool{
		"data:image/jpeg;base64,AAAA":                true,
		"data:image/webp;base64,AAAA":                true,
		"https://media.licdn.com/dms/image/x":        true,
		"https://tracker.example/pixel.png":          false, // IP/location tracking
		"data:image/svg+xml;base64,PHN2Zz4=":         false, // can carry script
		"data:image/jpeg,raw-not-base64":             false,
		"":                                           false,
		"javascript:alert(1)":                        false,
		"https://media.licdn.com.evil.example/x.png": false,
	} {
		if got := validPhoto(s); got != want {
			t.Errorf("validPhoto(%q) = %v, want %v", s, got, want)
		}
	}
}
