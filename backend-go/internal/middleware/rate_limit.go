package middleware

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/atharvix/kinjo-backend/internal/observability"
)

type rateBucket struct {
	count       int
	windowStart time.Time
}

type RateLimiter struct {
	mu       sync.Mutex
	buckets  map[string]*rateBucket
	window   time.Duration
	maxLimit int
	metrics  *observability.Metrics
}

func NewRateLimiter(window time.Duration, maxLimit int, metrics *observability.Metrics) *RateLimiter {
	rl := &RateLimiter{
		buckets:  make(map[string]*rateBucket),
		window:   window,
		maxLimit: maxLimit,
		metrics:  metrics,
	}

	// Periodically clean up stale buckets to avoid memory leaks
	go rl.cleanupLoop(window * 2)

	return rl
}

func (rl *RateLimiter) Allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	b, exists := rl.buckets[key]
	if !exists || now.Sub(b.windowStart) > rl.window {
		rl.buckets[key] = &rateBucket{count: 1, windowStart: now}
		return true
	}

	if b.count >= rl.maxLimit {
		return false
	}

	b.count++
	return true
}

func (rl *RateLimiter) Refund(key string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	if b, exists := rl.buckets[key]; exists && b.count > 0 {
		b.count--
	}
}

func (rl *RateLimiter) cleanupLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	for range ticker.C {
		rl.mu.Lock()
		now := time.Now()
		for k, b := range rl.buckets {
			if now.Sub(b.windowStart) > rl.window {
				delete(rl.buckets, k)
			}
		}
		rl.mu.Unlock()
	}
}

func (rl *RateLimiter) Middleware(endpointName string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := GetClientIP(r)
			if !rl.Allow(ip) {
				if rl.metrics != nil {
					rl.metrics.RateLimitExceeded.WithLabelValues("ip", endpointName).Inc()
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				_ = json.NewEncoder(w).Encode(map[string]string{
					"error": "Too many requests. Please slow down.",
				})
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func GetClientIP(r *http.Request) string {
	// Check X-Forwarded-For header
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		if len(parts) > 0 && strings.TrimSpace(parts[0]) != "" {
			return strings.TrimSpace(parts[0])
		}
	}

	// Check X-Real-IP
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}

	// Fallback to RemoteAddr
	addr := r.RemoteAddr
	if idx := strings.LastIndex(addr, ":"); idx != -1 {
		return addr[:idx]
	}
	return addr
}
