// loadgen drives a Kinjo backend with N phones in crowds of 50, the way the v2 plugins
// report (docs/proximity.md), and measures what users would feel:
//   - HTTP latency and errors for /api/sightings and /api/presence at each rate step;
//   - end to end: a phone reports a new neighbour -> the neighbour's open socket gets a
//     "nearby" list with that card (the number that must stay under 5 s).
//
// Accounts lt_1..lt_N with sessions "lt_<i>" and complete profiles must exist (seed.sql).
// Never point it at production: it creates load and needs its own database.
//
//	loadgen -api http://127.0.0.1:8090 -users 100000 -steps 2500,5000,10000 -step 60s -sockets 5000
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

var (
	api     = flag.String("api", "http://127.0.0.1:8090", "backend base URL")
	users   = flag.Int("users", 100000, "phones")
	crowd   = flag.Int("crowd", 50, "phones per crowd (all within 20 m)")
	steps   = flag.String("steps", "2500,5000,10000", "sighting reports per second, one step each")
	stepDur = flag.Duration("step", 60*time.Second, "duration of each step")
	sockets = flag.Int("sockets", 5000, "open apps holding a WebSocket (they also send a fix every 10 s)")
	probes  = flag.Int("probes", 20, "end-to-end probes per second (new pair -> card on the socket)")
)

var client = &http.Client{Timeout: 15 * time.Second, Transport: &http.Transport{MaxIdleConns: 4000, MaxIdleConnsPerHost: 4000, IdleConnTimeout: time.Minute}}

func sess(i int) string { return "lt_" + strconv.Itoa(i+1) }

type stat struct {
	mu   sync.Mutex
	ms   []float64
	errs map[string]int
}

func (s *stat) add(d time.Duration, err string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.errs == nil {
		s.errs = map[string]int{}
	}
	if err != "" {
		s.errs[err]++
		return
	}
	s.ms = append(s.ms, float64(d.Microseconds())/1000)
}

func (s *stat) line(name string, secs float64) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	sort.Float64s(s.ms)
	q := func(p float64) float64 {
		if len(s.ms) == 0 {
			return 0
		}
		return s.ms[min(len(s.ms)-1, int(p*float64(len(s.ms))))]
	}
	n := len(s.ms)
	for _, v := range s.errs {
		n += v
	}
	return fmt.Sprintf("%-10s %7.0f/s  p50 %6.1f ms  p95 %6.1f  p99 %6.1f  max %7.1f  errors %v", name, float64(n)/secs, q(.5), q(.95), q(.99), q(1), s.errs)
}

func do(method, path, tok string, body any) (int, []byte, error) {
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, *api+path, r)
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, b, nil
}

func call(s *stat, method, path, tok string, body any) {
	t := time.Now()
	code, _, err := do(method, path, tok, body)
	switch {
	case err != nil:
		e := err.Error()
		if i := strings.LastIndex(e, ": "); i >= 0 {
			e = e[i+2:]
		}
		s.add(0, e)
	case code >= 300 && code != 409:
		s.add(0, strconv.Itoa(code))
	default:
		s.add(time.Since(t), "")
	}
}

// socket: one open app. Remembers when each card first showed up.
type socket struct {
	mu   sync.Mutex
	seen map[string]time.Time
}

func main() {
	flag.Parse()
	n := *users
	pos := func(i int) (float64, float64) { // crowds ~1 km apart, so GPS cells don't overlap
		c := i / *crowd
		return 12.0 + float64(c%300)*0.01, 77.0 + float64(c/300)*0.01 + float64(i%*crowd)*0.00001
	}

	fmt.Printf("tokens for %d phones...\n", n)
	toks := make([]string, n)
	var bad atomic.Int64
	var wg sync.WaitGroup
	sem := make(chan struct{}, 200)
	t0 := time.Now()
	for i := 0; i < n; i++ {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int) {
			defer wg.Done()
			defer func() { <-sem }()
			code, b, err := do("GET", "/api/ble-token", sess(i), nil)
			var o struct{ Token string }
			if err != nil || code != 200 || json.Unmarshal(b, &o) != nil {
				bad.Add(1)
				return
			}
			toks[i] = o.Token
		}(i)
	}
	wg.Wait()
	fmt.Printf("  %d tokens in %.1f s (%d failed)\n", n-int(bad.Load()), time.Since(t0).Seconds(), bad.Load())

	// open apps, spread evenly over the crowds
	open := []int{}
	for i := 0; i < n && len(open) < *sockets; i += max(1, n / *sockets) {
		open = append(open, i)
	}
	socks := map[int]*socket{}
	var smu sync.Mutex
	var wsErr atomic.Int64
	dialer := websocket.Dialer{Subprotocols: []string{"kinjo", ""}, HandshakeTimeout: 15 * time.Second}
	wsURL := "ws" + strings.TrimPrefix(*api, "http") + "/ws"
	for _, i := range open {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int) {
			defer wg.Done()
			defer func() { <-sem }()
			d := dialer
			d.Subprotocols = []string{"kinjo", sess(i)}
			c, _, err := d.Dial(wsURL, nil)
			if err != nil {
				wsErr.Add(1)
				return
			}
			s := &socket{seen: map[string]time.Time{}}
			smu.Lock()
			socks[i] = s
			smu.Unlock()
			go func() {
				for {
					_, b, err := c.ReadMessage()
					if err != nil {
						return
					}
					var m struct {
						Type   string
						People []struct{ UID string }
					}
					if json.Unmarshal(b, &m) != nil || m.Type != "nearby" {
						continue
					}
					now := time.Now()
					s.mu.Lock()
					for _, p := range m.People {
						if _, ok := s.seen[p.UID]; !ok {
							s.seen[p.UID] = now
						}
					}
					s.mu.Unlock()
				}
			}()
		}(i)
	}
	wg.Wait()
	fmt.Printf("  %d sockets open (%d failed)\n", len(socks), wsErr.Load())

	// fixes from open apps, every 10 s, for the whole run
	var fixStat stat
	stop := make(chan struct{})
	go func() {
		tick := time.NewTicker(10 * time.Second / time.Duration(max(1, len(open))))
		defer tick.Stop()
		k := 0
		for {
			select {
			case <-stop:
				return
			case <-tick.C:
				i := open[k%len(open)]
				k++
				la, lo := pos(i)
				go call(&fixStat, "POST", "/api/presence", sess(i), map[string]any{"lat": la, "lng": lo, "acc": 30, "age": 0})
			}
		}
	}()

	r := rand.New(rand.NewSource(1))
	var rmu sync.Mutex
	report := func(i int, st *stat) {
		c0 := i / *crowd * *crowd
		seen := make([]map[string]any, 0, 20)
		rmu.Lock()
		for j := 0; j < 20; j++ {
			o := c0 + r.Intn(*crowd)
			if o != i && o < n && toks[o] != "" {
				seen = append(seen, map[string]any{"tok": toks[o], "rssi": -60 - r.Intn(35)})
			}
		}
		rmu.Unlock()
		call(st, "POST", "/api/sightings", sess(i), map[string]any{"seen": seen})
	}

	// end-to-end probes: a phone from the far end of the list (another crowd, so the pair is
	// new) reports hearing an open app; we time how long until the card is on that app's socket.
	type probe struct {
		viewer, step int
		other        string
		at           time.Time
	}
	var curStep atomic.Int64
	var pmu sync.Mutex
	var probesSent []probe
	nextOther := n - 1
	go func() {
		tick := time.NewTicker(time.Second / time.Duration(max(1, *probes)))
		defer tick.Stop()
		k := 0
		for {
			select {
			case <-stop:
				return
			case <-tick.C:
			}
			smu.Lock()
			v := open[k%len(open)]
			_, ok := socks[v]
			smu.Unlock()
			k++
			if !ok || toks[v] == "" {
				continue
			}
			pmu.Lock()
			o := nextOther
			nextOther--
			pmu.Unlock()
			if o <= v || toks[o] == "" {
				continue
			}
			// the far phone o hears the open app v: a brand-new pair, both decks change
			at := time.Now()
			go do("POST", "/api/sightings", sess(o), map[string]any{"seen": []map[string]any{{"tok": toks[v], "rssi": -65}}})
			pmu.Lock()
			probesSent = append(probesSent, probe{v, int(curStep.Load()), "lt_" + strconv.Itoa(o+1), at})
			pmu.Unlock()
		}
	}()

	stepNames := strings.Split(*steps, ",")
	for si, sv := range stepNames {
		curStep.Store(int64(si))
		rate, _ := strconv.Atoi(sv)
		var st stat
		fmt.Printf("\nstep: %d sighting reports/s for %v\n", rate, *stepDur)
		end := time.Now().Add(*stepDur)
		gap := time.Second / time.Duration(rate)
		next := time.Now()
		k := 0
		inflight := make(chan struct{}, 3000)
		for time.Now().Before(end) {
			next = next.Add(gap)
			if d := time.Until(next); d > 0 {
				time.Sleep(d)
			}
			i := k % n
			k++
			select {
			case inflight <- struct{}{}:
				go func() { report(i, &st); <-inflight }()
			default:
				st.add(0, "client backlog") // 3000 requests already waiting: the server is behind
			}
		}
		secs := stepDur.Seconds()
		fmt.Println(" ", st.line("sightings", secs))
		fmt.Println(" ", fixStat.line("fixes", secs))
		fixStat = stat{}
	}
	close(stop)
	time.Sleep(3 * time.Second)

	// end to end, per step
	fmt.Println("\nend to end (new pair -> card on the open app's socket):")
	for si, name := range stepNames {
		var e2e []float64
		missed := 0
		pmu.Lock()
		for _, p := range probesSent {
			if p.step != si {
				continue
			}
			smu.Lock()
			s := socks[p.viewer]
			smu.Unlock()
			s.mu.Lock()
			t, ok := s.seen[p.other]
			s.mu.Unlock()
			if !ok {
				missed++
				continue
			}
			e2e = append(e2e, t.Sub(p.at).Seconds()*1000)
		}
		pmu.Unlock()
		sort.Float64s(e2e)
		q := func(p float64) float64 {
			if len(e2e) == 0 {
				return 0
			}
			return e2e[min(len(e2e)-1, int(p*float64(len(e2e))))]
		}
		fmt.Printf("  at %5s/s: %4d probes, %4d never arrived   p50 %5.0f ms  p95 %5.0f ms  p99 %5.0f ms  max %5.0f ms\n",
			name, len(e2e)+missed, missed, q(.5), q(.95), q(.99), q(1))
	}
	os.Exit(0)
}
