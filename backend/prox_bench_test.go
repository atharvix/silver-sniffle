package main

import (
	"math/rand"
	"runtime"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

// 100k phones in crowds of 50; each report carries what a phone hears in its crowd (~20 tokens).
func BenchmarkSightings100k(b *testing.B) {
	const users, crowd = 100_000, 50
	h := newWorld()
	now := time.Now()
	toks := make([]string, users)
	uids := make([]string, users)
	for i := range toks {
		uids[i] = "u" + strconv.Itoa(i)
		toks[i], _ = h.bleToken(uids[i], now)
		if i%10 == 0 { // one in ten has the app open
			h.clients[uids[i]] = map[*Client]bool{newClient(uids[i]): true}
		}
	}
	var n, clock atomic.Int64
	start := now
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		r := rand.New(rand.NewSource(n.Add(1)))
		seen := make([]sighting, 20)
		for pb.Next() {
			i := r.Intn(users)
			base := i / crowd * crowd
			for j := range seen {
				v := -60 - r.Intn(35)
				seen[j] = sighting{Tok: toks[base+r.Intn(crowd)], RSSI: &v}
			}
			// simulated clock: every phone reports once per 10 s, as the plugins do in the background
			h.addSightings(uids[i], seen, start.Add(time.Duration(clock.Add(1))*(10*time.Second/users)))
		}
	})
	b.StopTimer()
	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	b.ReportMetric(float64(m.HeapAlloc)/1e6, "heapMB")
	b.ReportMetric(float64(len(h.pairs)), "pairs")
}
