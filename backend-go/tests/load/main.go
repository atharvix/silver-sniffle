package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

type LatencyStats struct {
	TotalRequests int64
	SuccessCount  int64
	ErrorCount    int64
	Latencies     []time.Duration
	mu            sync.Mutex
}

func (s *LatencyStats) Record(d time.Duration, success bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.TotalRequests++
	if success {
		s.SuccessCount++
	} else {
		s.ErrorCount++
	}
	s.Latencies = append(s.Latencies, d)
}

func (s *LatencyStats) Summary(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.Latencies) == 0 {
		fmt.Printf("[%s] No requests recorded\n", name)
		return
	}

	sort.Slice(s.Latencies, func(i, j int) bool {
		return s.Latencies[i] < s.Latencies[j]
	})

	n := len(s.Latencies)
	p50 := s.Latencies[n*50/100]
	p95 := s.Latencies[n*95/100]
	p99 := s.Latencies[n*99/100]
	max := s.Latencies[n-1]
	min := s.Latencies[0]

	var sum time.Duration
	for _, l := range s.Latencies {
		sum += l
	}
	avg := sum / time.Duration(n)

	fmt.Printf("\n=== %s ===\n", name)
	fmt.Printf("Total Requests: %d | Success: %d | Errors: %d\n", s.TotalRequests, s.SuccessCount, s.ErrorCount)
	fmt.Printf("Latency: Min: %v | Avg: %v | p50: %v | p95: %v | p99: %v | Max: %v\n",
		min, avg, p50, p95, p99, max)
}

func main() {
	baseURL := flag.String("url", "http://localhost:8080", "Target API base URL")
	concurrency := flag.Int("c", 100, "Number of concurrent simulated users")
	duration := flag.Duration("d", 15*time.Second, "Test duration")
	flag.Parse()

	fmt.Printf("Starting Kinjo Load Test\nTarget: %s\nVirtual Users (Concurrency): %d\nDuration: %v\n\n",
		*baseURL, *concurrency, *duration)

	stats := &LatencyStats{}
	endpointStats := map[string]*LatencyStats{
		"send_otp":        {},
		"verify_otp":      {},
		"upsert_profile":  {},
		"update_location": {},
		"heartbeat":       {},
		"get_nearby":      {},
	}

	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        *concurrency * 2,
			MaxIdleConnsPerHost: *concurrency * 2,
			MaxConnsPerHost:     *concurrency * 2,
			IdleConnTimeout:     90 * time.Second,
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), *duration)
	defer cancel()

	var activeUsers atomic.Int64
	var wg sync.WaitGroup

	startTime := time.Now()

	for i := 0; i < *concurrency; i++ {
		wg.Add(1)
		go func(userID int) {
			defer wg.Done()
			activeUsers.Add(1)
			defer activeUsers.Add(-1)

			email := fmt.Sprintf("user_%d_%d@loadtest.local", userID, time.Now().UnixNano())
			var token string

			// 1. Send OTP
			{
				start := time.Now()
				reqBody, _ := json.Marshal(map[string]string{"email": email})
				resp, err := client.Post(*baseURL+"/api/auth/send-otp", "application/json", bytes.NewReader(reqBody))
				dur := time.Since(start)

				if err == nil && (resp.StatusCode == 200 || resp.StatusCode == 429) {
					var res struct {
						DevOTP *string `json:"devOtp"`
					}
					_ = json.NewDecoder(resp.Body).Decode(&res)
					resp.Body.Close()
					endpointStats["send_otp"].Record(dur, resp.StatusCode == 200)
					stats.Record(dur, resp.StatusCode == 200)

					// 2. Verify OTP
					if res.DevOTP != nil {
						vStart := time.Now()
						vBody, _ := json.Marshal(map[string]string{"email": email, "otp": *res.DevOTP})
						vResp, vErr := client.Post(*baseURL+"/api/auth/verify-otp", "application/json", bytes.NewReader(vBody))
						vDur := time.Since(vStart)

						if vErr == nil && vResp.StatusCode == 200 {
							var vRes struct {
								VerificationToken string `json:"verificationToken"`
							}
							_ = json.NewDecoder(vResp.Body).Decode(&vRes)
							vResp.Body.Close()
							token = vRes.VerificationToken
							endpointStats["verify_otp"].Record(vDur, true)
							stats.Record(vDur, true)
						} else {
							if vResp != nil {
								vResp.Body.Close()
							}
							endpointStats["verify_otp"].Record(vDur, false)
							stats.Record(vDur, false)
						}
					}
				} else {
					if resp != nil {
						resp.Body.Close()
					}
					endpointStats["send_otp"].Record(dur, false)
					stats.Record(dur, false)
				}
			}

			// If token obtained, run continuous presence loop
			if token != "" {
				// Upsert profile
				pStart := time.Now()
				pBody, _ := json.Marshal(map[string]string{
					"name":  fmt.Sprintf("LoadUser %d", userID),
					"about": "Simulated active user nearby",
				})
				req, _ := http.NewRequestWithContext(ctx, http.MethodPost, *baseURL+"/api/profiles", bytes.NewReader(pBody))
				req.Header.Set("Authorization", "Bearer "+token)
				req.Header.Set("Content-Type", "application/json")
				pResp, pErr := client.Do(req)
				pDur := time.Since(pStart)
				if pErr == nil && pResp.StatusCode == 200 {
					endpointStats["upsert_profile"].Record(pDur, true)
					stats.Record(pDur, true)
					pResp.Body.Close()
				} else {
					if pResp != nil {
						pResp.Body.Close()
					}
					endpointStats["upsert_profile"].Record(pDur, false)
					stats.Record(pDur, false)
				}

				// Simulated coordinates near San Francisco
				lat := 37.7749 + (rand.Float64()-0.5)*0.0004
				lon := -122.4194 + (rand.Float64()-0.5)*0.0004

				// Initial location
				lStart := time.Now()
				lBody, _ := json.Marshal(map[string]float64{"latitude": lat, "longitude": lon})
				lReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, *baseURL+"/api/profiles/location", bytes.NewReader(lBody))
				lReq.Header.Set("Authorization", "Bearer "+token)
				lReq.Header.Set("Content-Type", "application/json")
				lResp, lErr := client.Do(lReq)
				lDur := time.Since(lStart)
				if lErr == nil && lResp.StatusCode == 200 {
					endpointStats["update_location"].Record(lDur, true)
					stats.Record(lDur, true)
					lResp.Body.Close()
				} else {
					if lResp != nil {
						lResp.Body.Close()
					}
					endpointStats["update_location"].Record(lDur, false)
					stats.Record(lDur, false)
				}

				// Loop until context finishes
				nearbyTicker := time.NewTicker(4 * time.Second)
				defer nearbyTicker.Stop()
				hbTicker := time.NewTicker(6 * time.Second)
				defer hbTicker.Stop()

				for {
					select {
					case <-ctx.Done():
						return
					case <-nearbyTicker.C:
						nStart := time.Now()
						nReq, _ := http.NewRequestWithContext(ctx, http.MethodGet, *baseURL+"/api/profiles/nearby", nil)
						nReq.Header.Set("Authorization", "Bearer "+token)
						nResp, nErr := client.Do(nReq)
						nDur := time.Since(nStart)
						if nErr == nil && nResp.StatusCode == 200 {
							_, _ = io.Copy(io.Discard, nResp.Body)
							nResp.Body.Close()
							endpointStats["get_nearby"].Record(nDur, true)
							stats.Record(nDur, true)
						} else {
							if nResp != nil {
								nResp.Body.Close()
							}
							endpointStats["get_nearby"].Record(nDur, false)
							stats.Record(nDur, false)
						}
					case <-hbTicker.C:
						hStart := time.Now()
						hReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, *baseURL+"/api/profiles/heartbeat", nil)
						hReq.Header.Set("Authorization", "Bearer "+token)
						hResp, hErr := client.Do(hReq)
						hDur := time.Since(hStart)
						if hErr == nil && hResp.StatusCode == 200 {
							hResp.Body.Close()
							endpointStats["heartbeat"].Record(hDur, true)
							stats.Record(hDur, true)
						} else {
							if hResp != nil {
								hResp.Body.Close()
							}
							endpointStats["heartbeat"].Record(hDur, false)
							stats.Record(hDur, false)
						}
					}
				}
			}
		}(i)
	}

	wg.Wait()
	totalElapsed := time.Since(startTime)

	fmt.Printf("\n======================================================\n")
	fmt.Printf("LOAD TEST COMPLETE in %v\n", totalElapsed)
	fmt.Printf("Total Throughput: %.2f requests/sec\n", float64(stats.TotalRequests)/totalElapsed.Seconds())
	fmt.Printf("======================================================\n")

	stats.Summary("ALL ENDPOINTS COMBINED")
	for name, s := range endpointStats {
		s.Summary(name)
	}
}
