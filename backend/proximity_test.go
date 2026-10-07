package main

import (
	"encoding/json"
	"math"
	"sync"
	"testing"
	"time"
)

// Proximity is the access boundary: these pin the rule "fresh, precise fixes,
// centres <= 30 m" end to end through the hub.

const mPerDeg = 6371000.0 * math.Pi / 180 // metres per degree of latitude, same R as distM

var x0 = [2]float64{12.9716, 77.5946}

func fix(uid string, northM float64, acc int, t time.Time) *Pres {
	lat, lng := x0[0]+northM/mPerDeg, x0[1]
	return &Pres{UID: uid, Cell: cellOf(lat, lng), Lat: lat, Lng: lng, Acc: acc, T: t}
}

func TestEligibleDistance(t *testing.T) {
	now := time.Now()
	a := fix("a", 0, 8, now)
	for _, tc := range []struct {
		m    float64
		want bool
	}{{0, true}, {5, true}, {10, true}, {29, true}, {30, true}, {31, false}, {50, false}, {100, false}, {1000, false}} {
		b := fix("b", tc.m, 8, now)
		d, why := eligible(a, b, now)
		if math.Abs(d-tc.m) > 0.001 && why != "stale" {
			t.Errorf("%v m: distM = %.4f", tc.m, d)
		}
		if (why == "") != tc.want {
			t.Errorf("%v m: eligible=%v (%q), want %v", tc.m, why == "", why, tc.want)
		}
	}
	// east-west at 60°N: longitude degrees are half as long, lat/lng order matters
	p := &Pres{Lat: 60, Lng: 10, Acc: 5, T: now}
	q := &Pres{Lat: 60, Lng: 10 + 25/(mPerDeg*math.Cos(60*math.Pi/180)), Acc: 5, T: now}
	if d, why := eligible(p, q, now); why != "" || math.Abs(d-25) > 0.01 {
		t.Errorf("25 m east at 60N: d=%.3f why=%q", d, why)
	}
}

func TestEligibleFreshnessAndAccuracy(t *testing.T) {
	now := time.Now()
	a := fix("a", 0, 8, now)
	for _, tc := range []struct {
		name string
		b    *Pres
		want string
	}{
		{"fresh", fix("b", 10, 8, now.Add(-10*time.Second)), ""},
		{"at limit", fix("b", 10, 8, now.Add(-freshFor)), ""},
		{"recently stale", fix("b", 10, 8, now.Add(-freshFor-time.Second)), "stale"},
		{"very stale", fix("b", 10, 8, now.Add(-time.Hour)), "stale"},
		{"missing timestamp", &Pres{Lat: x0[0], Lng: x0[1], Acc: 8}, "stale"},
		{"high accuracy", fix("b", 10, 3, now), ""},
		{"accuracy at limit", fix("b", 10, maxAccM, now), ""},
		{"low accuracy", fix("b", 10, maxAccM+1, now), "accuracy"},
		{"very low accuracy", fix("b", 10, 500, now), "accuracy"},
		{"unknown accuracy", fix("b", 10, 0, now), "accuracy"},
	} {
		if _, why := eligible(a, tc.b, now); why != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, why, tc.want)
		}
	}
	// the viewer's own fix counts too: a stale or vague viewer sees nobody
	if _, why := eligible(fix("a", 0, 8, now.Add(-time.Hour)), fix("b", 10, 8, now), now); why != "stale" {
		t.Errorf("stale viewer: %q", why)
	}
	if _, why := eligible(fix("a", 0, 80, now), fix("b", 10, 8, now), now); why != "accuracy" {
		t.Errorf("vague viewer: %q", why)
	}
}

// world is a hub with two subscribed clients, A and B.
type world struct {
	h    *Hub
	a, b *Client
}

func newWorld() *world {
	h := newHub(nil, nil)
	w := &world{h: h, a: &Client{uid: "a", cells: map[string]bool{}}, b: &Client{uid: "b", cells: map[string]bool{}}}
	h.clients["a"], h.clients["b"] = w.a, w.b
	return w
}

func (w *world) put(p *Pres) []string {
	touched := w.h.setPresence(p)
	if touched != nil {
		w.h.subscribe(w.h.clients[p.UID], p)
	}
	return touched
}

func sees(h *Hub, c *Client, uid string, now time.Time) bool {
	for _, s := range h.candidates(c, now) {
		if s.uid == uid {
			return true
		}
	}
	return false
}

func TestScenarioMoveAway(t *testing.T) {
	w := newWorld()
	now := time.Now()
	w.put(fix("a", 0, 8, now))
	for _, step := range []struct {
		m    float64
		want bool
	}{{20, true}, {31, false}, {100, false}, {500, false}, {20, true}} { // far -> nearby again at the end
		now = now.Add(time.Second)
		w.put(fix("a", 0, 8, now))
		w.put(fix("b", step.m, 8, now))
		if got := sees(w.h, w.a, "b", now); got != step.want {
			t.Errorf("B at %v m: A sees B = %v, want %v", step.m, got, step.want)
		}
		if got := sees(w.h, w.b, "a", now); got != step.want {
			t.Errorf("B at %v m: B sees A = %v, want %v (reverse)", step.m, got, step.want)
		}
	}
}

func TestScenarioStaleUnavailableVague(t *testing.T) {
	w := newWorld()
	now := time.Now()
	w.put(fix("a", 0, 8, now))
	w.put(fix("b", 20, 8, now))
	if !sees(w.h, w.a, "b", now) {
		t.Fatal("B at 20 m should be visible")
	}
	// B goes silent; A keeps reporting
	later := now.Add(freshFor + time.Second)
	w.put(fix("a", 0, 8, later))
	if sees(w.h, w.a, "b", later) {
		t.Error("B silent past freshFor must not be visible")
	}
	// B's newest fix is vague: no pinning at the old precise spot
	w.put(fix("b", 20, 8, later))
	w.put(fix("b", 25, 200, later.Add(time.Second)))
	if sees(w.h, w.a, "b", later.Add(time.Second)) {
		t.Error("B with a 200 m fix must not be visible")
	}
	// B turns location off (hide) -> unavailable
	w.put(fix("b", 20, 8, later.Add(2*time.Second)))
	w.h.notify(nil, w.h.dropPresence("b"))
	if sees(w.h, w.a, "b", later.Add(2*time.Second)) {
		t.Error("hidden B must not be visible")
	}
}

func TestReplayedFixCannotRefresh(t *testing.T) {
	w := newWorld()
	now := time.Now()
	w.put(fix("b", 20, 8, now))
	if w.put(fix("b", 0, 8, now.Add(-time.Minute))) != nil {
		t.Fatal("an older fix must not replace a newer one")
	}
	if p := w.h.position("b"); p.T != now {
		t.Fatal("older fix changed stored presence")
	}
}

func TestExpirePushesRemoval(t *testing.T) {
	w := newWorld()
	now := time.Now()
	w.put(fix("a", 0, 8, now))
	w.put(fix("b", 20, 8, now.Add(-freshFor-time.Second)))
	if n := w.h.expire(); n != 1 {
		t.Fatalf("expire dropped %d, want 1", n)
	}
	if w.h.position("b") != nil || !w.h.dirty[w.a] {
		t.Fatal("stale B should be dropped and A's deck marked for a fresh push")
	}
	if w.h.position("a") == nil {
		t.Fatal("fresh A must survive expire")
	}
}

// The client controls only lat/lng/acc/age of its own fix: extra fields such as
// a radius or someone else's uid are ignored by the decoder.
func TestInboundIgnoresRadiusAndUID(t *testing.T) {
	var in inbound
	if err := json.Unmarshal([]byte(`{"type":"pos","lat":1,"lng":2,"acc":5,"age":10,"radius":5000,"uid":"victim","nearby":["x"]}`), &in); err != nil {
		t.Fatal(err)
	}
	if in != (inbound{Type: "pos", Lat: 1, Lng: 2, Acc: 5, Age: 10}) {
		t.Fatalf("unexpected decode %+v", in)
	}
	// spoofed coordinates only move your own search: from 500 m away you see nobody at X
	w := newWorld()
	now := time.Now()
	w.put(fix("a", 500, 8, now))
	w.put(fix("b", 0, 8, now))
	if sees(w.h, w.a, "b", now) {
		t.Fatal("A at 500 m must not see B")
	}
}

// Location updates racing with deck computation: run with -race. The final
// state must win.
func TestConcurrentUpdates(t *testing.T) {
	w := newWorld()
	start := time.Now()
	w.put(fix("a", 0, 8, start))
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 1; i <= 500; i++ {
			m := 20.0
			if i%2 == 0 {
				m = 200
			}
			w.put(fix("b", m, 8, start.Add(time.Duration(i)*time.Millisecond)))
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			w.h.candidates(w.a, start.Add(time.Second))
		}
	}()
	wg.Wait()
	if sees(w.h, w.a, "b", start.Add(time.Second)) {
		t.Fatal("B's last fix is 200 m away; A must not see B")
	}
}

// Boundary crossings in both directions, each leg re-evaluated from scratch.
func TestReEntry(t *testing.T) {
	for _, legs := range [][3]float64{{10, 100, 10}, {29, 31, 29}, {5, 50, 5}} {
		w := newWorld()
		now := time.Now()
		for i, m := range legs {
			now = now.Add(5 * time.Second)
			w.put(fix("a", 0, 8, now))
			w.put(fix("b", m, 8, now))
			want := i != 1
			if got := sees(w.h, w.a, "b", now); got != want {
				t.Errorf("%v leg %d (%v m): A sees B = %v, want %v", legs, i, m, got, want)
			}
		}
	}
}

// Screen off: A's socket died while B left and came back. A's new socket must
// get a list immediately, from A's stored fix, with B on it.
func TestReconnectGetsListWithoutNewFix(t *testing.T) {
	w := newWorld()
	now := time.Now()
	w.put(fix("a", 0, 8, now))
	w.put(fix("b", 100, 8, now))
	w.h.unsubscribe(w.a) // socket dropped
	w.put(fix("b", 10, 8, now.Add(time.Second)))
	if w.h.dirty[w.a] {
		t.Fatal("a dropped socket must not be owed pushes")
	}
	fresh := &Client{uid: "a", cells: map[string]bool{}}
	w.h.attach(fresh)
	if !w.h.dirty[fresh] {
		t.Fatal("new socket should be queued for a push on connect")
	}
	if !sees(w.h, fresh, "b", now.Add(time.Second)) {
		t.Fatal("B back at 10 m should be on the reconnected deck")
	}
}

// Hiding while a position update is still in flight: that older fix must not
// put the person back on anyone's deck; a fix taken after un-hiding must.
func TestHideBeatsInFlightFix(t *testing.T) {
	w := newWorld()
	now := time.Now()
	w.put(fix("a", 0, 8, now))
	w.put(fix("b", 10, 8, now))
	w.h.dropPresence("b") // hide
	if w.put(fix("b", 10, 8, now.Add(-time.Second))) != nil {
		t.Fatal("a fix taken before hiding brought B back")
	}
	later := time.Now().Add(time.Second)
	w.put(fix("a", 0, 8, later))
	w.put(fix("b", 10, 8, later))
	if !sees(w.h, w.a, "b", later) {
		t.Fatal("a fix taken after un-hiding should show B again")
	}
}
