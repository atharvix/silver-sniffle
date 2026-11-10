// loadv3 drives a Kinjo backend the way v3 phones do (docs/proximity.md): every phone holds a
// reporter socket and sends fixes (walkers once a second, the rest a still heartbeat every
// 2 min), a share of them also has the app open on a delta socket. It measures what people
// feel: a probe phone steps into a viewer's range and out again, and the time until the
// viewer's socket shows the change. Then every reporter drops and redials at once (a restart).
//
// Accounts lt_1..lt_N with sessions "lt_<i>" and complete profiles must exist (seed below).
// Never point it at production: it creates load and needs its own database.
//
//	loadv3 -api http://127.0.0.1:8090 -phones 20000 -apps 2000 -walk 0.2 -for 60s
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"math"
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
	api    = flag.String("api", "http://127.0.0.1:8090", "backend")
	phones = flag.Int("phones", 20000, "reporter sockets")
	apps   = flag.Int("apps", 2000, "of them with the app open (delta socket)")
	walk   = flag.Float64("walk", 0.2, "share walking (a fix a second)")
	crowd  = flag.Int("crowd", 50, "phones per crowd, within 20 m")
	dur    = flag.Duration("for", 60*time.Second, "steady load before the probes")
	probes = flag.Int("probes", 200, "step-in / step-out probes")
)

const mPerDeg = 111320.0

func spot(i int) (float64, float64) { // crowd centres 1 km apart, people within 20 m
	c := i / *crowd
	lat, lng := 12.9+float64(c%40)*0.01, 77.5+float64(c/40)*0.01
	return lat + (rand.Float64()-0.5)*20/mPerDeg, lng + (rand.Float64()-0.5)*20/mPerDeg
}

func dial(uid string, reporter bool) (*websocket.Conn, error) {
	proto := []string{"kinjo", "lt_" + uid[3:]}
	if reporter {
		proto = append(proto, "reporter")
	}
	d := websocket.Dialer{Subprotocols: proto, HandshakeTimeout: 15 * time.Second}
	c, resp, err := d.Dial("ws"+strings.TrimPrefix(*api, "http")+"/ws", nil)
	if err != nil && resp != nil && resp.StatusCode == http.StatusServiceUnavailable {
		return nil, errBusy
	}
	return c, err
}

var errBusy = fmt.Errorf("503 busy")

type phone struct {
	uid      string
	lat, lng float64
	walking  bool
	mu       sync.Mutex
	c        *websocket.Conn
}

func (p *phone) send(still bool, spd, hdg float64) error {
	m := map[string]any{"type": "pos", "lat": p.lat, "lng": p.lng, "acc": 8, "age": 0, "still": still}
	if spd > 0 {
		m["spd"], m["hdg"] = spd, hdg
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.c == nil {
		return fmt.Errorf("closed")
	}
	_ = p.c.SetWriteDeadline(time.Now().Add(10 * time.Second))
	return p.c.WriteJSON(m)
}

// connect dials with jittered backoff on 503 or errors, like the phones' Live.java.
func (p *phone) connect(busy *atomic.Int64) {
	for retry := 0; ; retry++ {
		c, err := dial(p.uid, true)
		if err == nil {
			p.mu.Lock()
			p.c = c
			p.mu.Unlock()
			go func() { // drain "mode" answers
				for {
					if _, _, err := c.ReadMessage(); err != nil {
						return
					}
				}
			}()
			return
		}
		if err == errBusy {
			busy.Add(1)
		}
		wait := math.Min(60, math.Pow(2, float64(min(retry, 6)))) * (0.5 + rand.Float64())
		time.Sleep(time.Duration(wait * float64(time.Second)))
	}
}

// viewer is an open app: it records when each uid appears on or leaves its deck.
type viewer struct {
	mu      sync.Mutex
	deck    map[string]bool
	waitFor string
	want    bool
	got     chan time.Time
}

func (v *viewer) read(c *websocket.Conn) {
	for {
		_, b, err := c.ReadMessage()
		if err != nil {
			return
		}
		var m struct {
			Type   string
			People []struct{ UID string }
			Add    []struct{ UID string }
			Del    []string
			Order  []string
		}
		if json.Unmarshal(b, &m) != nil {
			continue
		}
		now := time.Now()
		v.mu.Lock()
		switch m.Type {
		case "nearby":
			v.deck = map[string]bool{}
			for _, p := range m.People {
				v.deck[p.UID] = true
			}
		case "delta":
			v.deck = map[string]bool{}
			for _, u := range m.Order {
				v.deck[u] = true
			}
		default:
			v.mu.Unlock()
			continue
		}
		if v.waitFor != "" && v.deck[v.waitFor] == v.want {
			v.waitFor = ""
			v.got <- now
		}
		v.mu.Unlock()
	}
}

func pct(a []float64, q float64) float64 {
	if len(a) == 0 {
		return 0
	}
	sort.Float64s(a)
	return a[min(len(a)-1, int(q*float64(len(a))))]
}

func main() {
	flag.Parse()
	rand.Seed(7)
	ps := make([]*phone, *phones)
	var busy atomic.Int64
	start := time.Now()
	var wg sync.WaitGroup
	sem := make(chan struct{}, 200)
	for i := range ps {
		lat, lng := spot(i)
		ps[i] = &phone{uid: "lt_" + strconv.Itoa(i+1), lat: lat, lng: lng, walking: rand.Float64() < *walk}
		wg.Add(1)
		sem <- struct{}{}
		go func(p *phone) {
			defer wg.Done()
			defer func() { <-sem }()
			p.connect(&busy)
			_ = p.send(!p.walking, 0, 0)
		}(ps[i])
	}
	wg.Wait()
	fmt.Printf("connected %d reporters in %v (503 busy answers: %d)\n", len(ps), time.Since(start).Round(time.Millisecond), busy.Load())

	// apps: the first phone of each crowd slot up to -apps, holding a delta socket
	vs := map[int]*viewer{}
	for i := 0; i < *phones && len(vs) < *apps; i += *phones / *apps {
		c, err := dial(ps[i].uid, false)
		if err != nil {
			continue
		}
		_ = c.WriteJSON(map[string]any{"type": "hello", "delta": true})
		v := &viewer{deck: map[string]bool{}, got: make(chan time.Time, 1)}
		vs[i] = v
		go v.read(c)
	}
	fmt.Printf("apps open: %d\n", len(vs))

	// steady load: walkers 1 fix/s (1.4 m/s, random heading), still phones every 120 s
	stop := make(chan struct{})
	var sent atomic.Int64
	for _, p := range ps {
		go func(p *phone) {
			hdg := rand.Float64() * 360
			t := time.NewTicker(time.Second)
			beat := time.Duration(120+rand.Intn(30)) * time.Second
			last := time.Now()
			for {
				select {
				case <-stop:
					return
				case <-t.C:
				}
				if p.walking {
					r := hdg * math.Pi / 180
					p.lat += 1.4 * math.Cos(r) / mPerDeg
					p.lng += 1.4 * math.Sin(r) / mPerDeg
					if p.send(false, 1.4, hdg) == nil {
						sent.Add(1)
					}
				} else if time.Since(last) > beat {
					last = time.Now()
					if p.send(true, 0, 0) == nil {
						sent.Add(1)
					}
				}
			}
		}(p)
	}
	time.Sleep(*dur)
	fmt.Printf("steady: %.0f fixes/s from %d phones (%.0f%% walking)\n", float64(sent.Load())/dur.Seconds(), *phones, *walk*100)

	// probes: a still phone of another crowd jumps next to a viewer (in), then 200 m away (out)
	var in, out []float64
	keys := make([]int, 0, len(vs))
	for k := range vs {
		keys = append(keys, k)
	}
	misses := 0
	for n := 0; n < *probes; n++ {
		vi := keys[n%len(keys)]
		v := vs[vi]
		pi := (vi + *crowd*7 + n) % *phones
		p := ps[pi]
		if p.walking || pi/(*crowd) == vi/(*crowd) {
			continue
		}
		home := [2]float64{p.lat, p.lng}
		for _, step := range []struct {
			lat, lng float64
			want     bool
		}{{ps[vi].lat + 10/mPerDeg, ps[vi].lng, true}, {ps[vi].lat + 300/mPerDeg, ps[vi].lng, false}} {
			v.mu.Lock()
			v.waitFor, v.want = p.uid, step.want
			v.mu.Unlock()
			p.lat, p.lng = step.lat, step.lng
			t0 := time.Now()
			_ = p.send(true, 0, 0)
			select {
			case t := <-v.got:
				ms := float64(t.Sub(t0).Microseconds()) / 1000
				if step.want {
					in = append(in, ms)
				} else {
					out = append(out, ms)
				}
			case <-time.After(5 * time.Second):
				v.mu.Lock()
				v.waitFor = ""
				v.mu.Unlock()
				misses++
			}
		}
		p.lat, p.lng = home[0], home[1]
		_ = p.send(true, 0, 0)
	}
	fmt.Printf("step in  -> card on screen: p50 %.1f ms  p95 %.1f ms  p99 %.1f ms  (n=%d)\n", pct(in, .5), pct(in, .95), pct(in, .99), len(in))
	fmt.Printf("step out -> card gone:      p50 %.1f ms  p95 %.1f ms  p99 %.1f ms  (n=%d)\n", pct(out, .5), pct(out, .95), pct(out, .99), len(out))
	fmt.Printf("probes over 5 s: %d\n", misses)

	// reconnect storm: every reporter drops at once and redials (what a restart does)
	close(stop)
	for _, p := range ps {
		p.mu.Lock()
		if p.c != nil {
			p.c.Close()
			p.c = nil
		}
		p.mu.Unlock()
	}
	busy.Store(0)
	start = time.Now()
	var done atomic.Int64
	for _, p := range ps {
		wg.Add(1)
		go func(p *phone) {
			defer wg.Done()
			time.Sleep(time.Duration(rand.Intn(1000)) * time.Millisecond)
			p.connect(&busy)
			done.Add(1)
		}(p)
	}
	wg.Wait()
	fmt.Printf("reconnect storm: %d back in %v (503 busy answers: %d)\n", done.Load(), time.Since(start).Round(time.Millisecond), busy.Load())
	os.Exit(0)
}
