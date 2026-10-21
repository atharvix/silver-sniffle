package main

import (
	"os"
	"runtime"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Telemetry for the admin dashboard: in memory only, counts and timings, never
// uids, tokens or places. A server restart starts it over (uptime says when).
// Per-minute buckets for the last hour; latency and match-time samples for percentiles.

const telMinutes = 60

type telMinute struct {
	T                          int64 `json:"t"` // unix minute start
	Reqs, E4, E5, Slow         int64 // requests; 4xx; 5xx; over slowReq
	Fixes, VagueFixes          int64 // from the hub's per-minute proximity counters
	Pairs, PairsBLE            int64 // pairs shown (any reason); shown only because of Bluetooth
	MatchGPS, MatchBLE         int64 // cards arriving on a deck for the first time
	Sightings, Tokens, Wakes   int64
	PushOK, PushFail, PushDead int64 // FCM + APNs: accepted, failed, token gone
	MailOK, MailFail           int64
}

type routeStat struct {
	n, e5   int64
	totalMS int64
	maxMS   int64
}

type telemetry struct {
	mu      sync.Mutex
	minutes [telMinutes]telMinute
	lat     []int32 // request latencies, ms, ring (percentiles)
	latAt   int
	match   []matchSample // first-match times, ring
	matchAt int
	routes  map[string]*routeStat // since start
}

type matchSample struct {
	ms  int64
	ble bool
}

const slowReq = time.Second

var tel = &telemetry{routes: map[string]*routeStat{}}

// cur returns this minute's bucket (resetting a reused slot). Caller holds mu.
func (t *telemetry) cur() *telMinute {
	m := time.Now().Unix() / 60 * 60
	b := &t.minutes[(m/60)%telMinutes]
	if b.T != m {
		*b = telMinute{T: m}
	}
	return b
}

func (t *telemetry) add(f func(*telMinute)) {
	t.mu.Lock()
	f(t.cur())
	t.mu.Unlock()
}

// request is called once per HTTP request (logRequests). route is the mux pattern,
// so /api/photo/{uid} is one row, not one per user.
func (t *telemetry) request(route string, status int, d time.Duration) {
	ms := d.Milliseconds()
	if route == "" {
		route = "(no such endpoint)"
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	b := t.cur()
	b.Reqs++
	switch {
	case status >= 500:
		b.E5++
	case status >= 400:
		b.E4++
	}
	if d >= slowReq {
		b.Slow++
	}
	if len(t.lat) < 2000 {
		t.lat = append(t.lat, int32(ms))
	} else {
		t.lat[t.latAt] = int32(ms)
		t.latAt = (t.latAt + 1) % len(t.lat)
	}
	r := t.routes[route]
	if r == nil {
		if len(t.routes) > 200 { // patterns are finite; this only guards against surprises
			return
		}
		r = &routeStat{}
		t.routes[route] = r
	}
	r.n++
	r.totalMS += ms
	if ms > r.maxMS {
		r.maxMS = ms
	}
	if status >= 500 {
		r.e5++
	}
}

// matched: a card arrived on a deck for the first time, ms after the later of the two started reporting.
func (t *telemetry) matched(ms int64, ble bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if ble {
		t.cur().MatchBLE++
	} else {
		t.cur().MatchGPS++
	}
	s := matchSample{ms, ble}
	if len(t.match) < 1000 {
		t.match = append(t.match, s)
	} else {
		t.match[t.matchAt] = s
		t.matchAt = (t.matchAt + 1) % len(t.match)
	}
}

func (t *telemetry) pushed(ok, failed, dead int) {
	t.add(func(m *telMinute) { m.PushOK += int64(ok); m.PushFail += int64(failed); m.PushDead += int64(dead) })
}

func (t *telemetry) mailed(err error) {
	t.add(func(m *telMinute) {
		if err == nil {
			m.MailOK++
		} else {
			m.MailFail++
		}
	})
}

// proximity takes the hub's per-minute counters as the sweeper resets them.
func (t *telemetry) proximity(fixes, vague, pairs, pairsBLE, sightings, tokens, wakes int64) {
	t.add(func(m *telMinute) {
		m.Fixes += fixes
		m.VagueFixes += vague
		m.Pairs += pairs
		m.PairsBLE += pairsBLE
		m.Sightings += sightings
		m.Tokens += tokens
		m.Wakes += wakes
	})
}

type pct struct {
	P50 int64 `json:"p50"`
	P90 int64 `json:"p90"`
	P99 int64 `json:"p99"`
	N   int   `json:"n"`
}

func percentiles(v []int64) pct {
	if len(v) == 0 {
		return pct{}
	}
	sort.Slice(v, func(i, j int) bool { return v[i] < v[j] })
	at := func(p float64) int64 { return v[int(p*float64(len(v)-1))] }
	return pct{at(.5), at(.9), at(.99), len(v)}
}

type routeOut struct {
	Route string `json:"route"`
	N     int64  `json:"n"`
	E5    int64  `json:"e5"`
	AvgMS int64  `json:"avgMs"`
	MaxMS int64  `json:"maxMs"`
}

// snapshot is the dashboard's view: the last hour by minute (oldest first, gaps
// filled), percentiles, and the busiest routes.
func (t *telemetry) snapshot() map[string]any {
	t.mu.Lock()
	now := time.Now().Unix() / 60 * 60
	series := make([]telMinute, 0, telMinutes)
	for i := telMinutes - 1; i >= 0; i-- {
		m := now - int64(i)*60
		b := t.minutes[(m/60)%telMinutes]
		if b.T != m {
			b = telMinute{T: m}
		}
		series = append(series, b)
	}
	lat := make([]int64, len(t.lat))
	for i, v := range t.lat {
		lat[i] = int64(v)
	}
	var gps, ble, all []int64
	for _, s := range t.match {
		all = append(all, s.ms)
		if s.ble {
			ble = append(ble, s.ms)
		} else {
			gps = append(gps, s.ms)
		}
	}
	routes := make([]routeOut, 0, len(t.routes))
	for k, r := range t.routes {
		routes = append(routes, routeOut{k, r.n, r.e5, r.totalMS / max(r.n, 1), r.maxMS})
	}
	t.mu.Unlock()
	sort.Slice(routes, func(i, j int) bool { return routes[i].N > routes[j].N })
	if len(routes) > 12 {
		routes = routes[:12]
	}
	return map[string]any{
		"minutes": series,
		"latency": percentiles(lat),
		"match":   map[string]pct{"all": percentiles(all), "gps": percentiles(gps), "ble": percentiles(ble)},
		"routes":  routes,
	}
}

// host reports the machine: load, disk and memory (Linux; zeros elsewhere).
func host() map[string]any {
	out := map[string]any{"cpus": runtime.NumCPU()}
	if b, err := os.ReadFile("/proc/loadavg"); err == nil {
		if f := strings.Fields(string(b)); len(f) >= 3 {
			out["load"] = f[:3]
		}
	}
	var fs syscall.Statfs_t
	if syscall.Statfs("/", &fs) == nil {
		out["diskTotalGB"] = float64(fs.Blocks*uint64(fs.Bsize)) / 1e9
		out["diskFreeGB"] = float64(fs.Bavail*uint64(fs.Bsize)) / 1e9
	}
	if b, err := os.ReadFile("/proc/meminfo"); err == nil {
		for _, line := range strings.Split(string(b), "\n") {
			f := strings.Fields(line)
			if len(f) >= 2 && (f[0] == "MemTotal:" || f[0] == "MemAvailable:") {
				var kb int64
				for _, c := range f[1] {
					kb = kb*10 + int64(c-'0')
				}
				out[strings.TrimSuffix(f[0], ":")+"MB"] = kb / 1024
			}
		}
	}
	return out
}
