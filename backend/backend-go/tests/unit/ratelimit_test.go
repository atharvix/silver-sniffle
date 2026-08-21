package unit

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/atharvix/kinjo-backend/internal/middleware"
)

func TestRateLimiter(t *testing.T) {
	window := 100 * time.Millisecond
	maxLimit := 5
	limiter := middleware.NewRateLimiter(window, maxLimit, nil)

	// First 5 requests should pass
	for i := 0; i < maxLimit; i++ {
		if !limiter.Allow("127.0.0.1") {
			t.Errorf("request %d should be allowed", i+1)
		}
	}

	// 6th request should be blocked
	if limiter.Allow("127.0.0.1") {
		t.Errorf("request 6 should be blocked")
	}

	// Different IP should still be allowed
	if !limiter.Allow("192.168.1.1") {
		t.Errorf("different IP should be allowed")
	}

	// Wait for window to slide
	time.Sleep(window + 10*time.Millisecond)

	// Should be allowed again
	if !limiter.Allow("127.0.0.1") {
		t.Errorf("request after window should be allowed")
	}
}

func TestRateLimiter_Concurrent(t *testing.T) {
	window := 200 * time.Millisecond
	maxLimit := 50
	limiter := middleware.NewRateLimiter(window, maxLimit, nil)

	var wg sync.WaitGroup
	allowedCount := 0
	var mu sync.Mutex

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			ip := fmt.Sprintf("10.0.0.%d", id%2) // 2 IPs, 50 requests each
			if limiter.Allow(ip) {
				mu.Lock()
				allowedCount++
				mu.Unlock()
			}
		}(i)
	}

	wg.Wait()

	if allowedCount > 100 {
		t.Errorf("allowedCount %d exceeded total requests", allowedCount)
	}
}
