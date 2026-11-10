package main

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"
)

// These pin the rules in docs/proximity.md end to end through the hub.

const mPerDeg = 6371000.0 * math.Pi / 180

var x0 = [2]float64{12.9716, 77.5946}

func newClient(uid string) *Client { return &Client{uid: uid, send: make(chan []byte, 64)} }

// newWorld: a hub with no database; every uid has a complete card.
func newWorld(uids ...string) *Hub {
	h := newHub(nil)
	for _, u := range uids {
		h.prof.items[u] = profEntry{prof: Profile{Name: u, Role: "r", Look: "l", HasPhoto: true}, ok: true, at: time.Now().Add(time.Hour)}
	}
	return h
}

func tok(t *testing.T, h *Hub, uid string, now time.Time) string {
	t.Helper()
	s, err := h.bleToken(uid, now)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func rssi(v int) *int { return &v }

func hear(h *Hub, who, tk string, r *int, now time.Time) int {
	return h.addSightings(who, []sighting{{Tok: tk, RSSI: r}}, now)
}

// deck is what viewer would be shown at now.
func deck(h *Hub, viewer string, now time.Time) map[string]bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := map[string]bool{}
	for _, s := range h.candidates(viewer, now) {
		out[s.uid] = true
	}
	return out
}

// sent is the last nearby list a client got, and whether it got a "scan".
func sent(c *Client) (people []string, scan bool) {
	for len(c.send) > 0 {
		var m struct {
			Type   string
			People []nearbyPerson
		}
		_ = json.Unmarshal(<-c.send, &m)
		switch m.Type {
		case "nearby":
			people = people[:0]
			for _, p := range m.People {
				people = append(people, p.UID)
			}
		case "scan":
			scan = true
		}
	}
	return people, scan
}

func at(northM float64) (float64, float64) { return x0[0] + northM/mPerDeg, x0[1] }

func TestBluetoothShowsBothWays(t *testing.T) {
	h := newWorld("a", "b")
	now := time.Now()
	tok(t, h, "a", now)
	tb := tok(t, h, "b", now)
	if hear(h, "a", tb, rssi(-70), now) != 1 {
		t.Fatal("sighting not counted")
	}
	if !deck(h, "a", now)["b"] || !deck(h, "b", now)["a"] {
		t.Fatal("one phone hearing the other counts for both")
	}
	ca, cb := newClient("a"), newClient("b")
	h.attach(ca)
	h.attach(cb)
	if p, _ := sent(ca); len(p) != 1 || p[0] != "b" {
		t.Fatalf("a socket's first list is sent at once: %v", p)
	}
}

func TestOverConnectionAndWeak(t *testing.T) {
	h := newWorld("a", "b", "c")
	now := time.Now()
	tok(t, h, "a", now)
	tb, tc := tok(t, h, "b", now), tok(t, h, "c", now)
	hear(h, "a", tb, nil, now) // over a connection: no RSSI
	if hear(h, "a", tc, rssi(minRSSI-1), now) != 0 {
		t.Fatal("below minRSSI must not count")
	}
	d := deck(h, "a", now)
	if !d["b"] || d["c"] {
		t.Fatalf("connection counts, too weak doesn't: %v", d)
	}
}

func TestRelayAloneNoCardButScan(t *testing.T) {
	h := newWorld("a", "b")
	now := time.Now()
	tok(t, h, "a", now)
	tb := tok(t, h, "b", now)
	ca, cb := newClient("a"), newClient("b")
	h.attach(ca)
	h.attach(cb)
	h.addSightings("a", []sighting{{Tok: tb, Via: "relay"}}, now)
	if deck(h, "a", now)["b"] || deck(h, "b", now)["a"] {
		t.Fatal("a relay alone must never show a card")
	}
	if _, s := sent(ca); !s {
		t.Fatal("a relay asks both phones to scan")
	}
	if _, s := sent(cb); !s {
		t.Fatal("a relay asks both phones to scan")
	}
	h.addSightings("a", []sighting{{Tok: tb, Via: "relay"}}, now.Add(time.Second))
	if _, s := sent(ca); s {
		t.Fatal("scan requests are rate limited")
	}
}

func TestOutdoorsGPSDecides(t *testing.T) {
	now := time.Now()
	place := func(northM float64, acc int, r int) map[string]bool {
		h := newWorld("a", "b")
		tok(t, h, "a", now)
		hear(h, "a", tok(t, h, "b", now), rssi(r), now)
		la, lo := at(0)
		h.addFix("a", la, lo, acc, 0, true, now)
		lb, _ := at(northM)
		h.addFix("b", lb, lo, acc, 0, true, now)
		return deck(h, "a", now)
	}
	if place(80, 12, -95)["b"] || place(80, 12, -80)["b"] {
		t.Fatal("open-sky fixes 80 m apart overrule Bluetooth, however loud")
	}
	if !place(40, 12, -93)["b"] {
		t.Fatal("outdoors, fixes inside 35 m + their error: Bluetooth stands (no extra delay)")
	}
	if !place(25, 12, -93)["b"] {
		t.Fatal("outdoors 25 m apart: GPS matches")
	}
	if !place(100, 25, -93)["b"] {
		t.Fatal("indoor (vague) fixes never veto: Bluetooth decides")
	}
}

func TestGPSMatch(t *testing.T) {
	h := newWorld("a", "b")
	now := time.Now()
	la, lo := at(0)
	h.addFix("a", la, lo, 10, 0, true, now)
	lb, _ := at(25)
	h.addFix("b", lb, lo, 10, 0, true, now)
	if !deck(h, "a", now)["b"] {
		t.Fatal("good fixes 25 m apart match")
	}
	lb, _ = at(45)
	h3 := newWorld("a", "b")
	h3.addFix("a", la, lo, 5, 0, true, now)
	h3.addFix("b", lb, lo, 5, 0, true, now)
	if deck(h3, "a", now)["b"] {
		t.Fatal("good fixes 45 m apart: no match (35 m + 7 m error)")
	}
}

func TestIndoorFixesWidenNotMiss(t *testing.T) {
	now := time.Now()
	la, lo := at(0)
	place := func(apart float64, acc int) bool {
		h := newWorld("a", "b")
		lb, _ := at(apart)
		h.addFix("a", la, lo, acc, 0, true, now)
		h.addFix("b", lb, lo, acc, 0, true, now)
		return deck(h, "a", now)["b"]
	}
	if !place(60, 25) {
		t.Fatal("indoor fixes (25 m each) 60 m apart: could be within 35 m, shown")
	}
	if place(80, 40) {
		t.Fatal("the widening is capped at 35 + 40 m")
	}
	if place(10, maxAccM+1) {
		t.Fatal("approximate location never matches")
	}
}

func TestLeaveAtOnceWithMargin(t *testing.T) {
	h := newWorld("a", "b")
	now := time.Now()
	la, lo := at(0)
	h.addFix("a", la, lo, 5, 0, true, now)
	lb, _ := at(30)
	h.addFix("b", lb, lo, 5, 0, true, now)
	if !deck(h, "a", now)["b"] {
		t.Fatal("30 m: shown")
	}
	h.onDeck["a"] = map[string]bool{"b": true}
	lb, _ = at(45) // past 35 + 7, inside the keep margin
	h.addFix("b", lb, lo, 5, 0, true, now.Add(time.Second))
	if !deck(h, "a", now.Add(time.Second))["b"] {
		t.Fatal("just past the line: kept (margin), no blink")
	}
	lb, _ = at(50)
	h.addFix("b", lb, lo, 5, 0, true, now.Add(2*time.Second))
	if deck(h, "a", now.Add(2*time.Second))["b"] {
		t.Fatal("past line + margin: gone at once, no 60 s hold")
	}
}

func TestStillPhoneStaysPlaced(t *testing.T) {
	h := newWorld("a", "b")
	now := time.Now()
	la, lo := at(0)
	lb, _ := at(20)
	h.addFix("a", la, lo, 10, 0, true, now)
	h.addFixStill("b", lb, lo, 10, 0, true, false, now)
	later := now.Add(fixFresh + time.Minute)
	h.addFix("a", la, lo, 10, 0, true, later)
	h.addFixStill("b", lb, lo, 10, int64((fixFresh+time.Minute).Milliseconds())+7, true, false, later) // same fix, re-sent (age off by 7 ms)
	if !deck(h, "a", later)["b"] {
		t.Fatal("a still phone that keeps reporting stays on decks past fixFresh")
	}
	if deck(h, "a", later.Add(stillFresh+time.Second))["b"] {
		t.Fatal("a still phone that stops reporting goes after stillFresh")
	}
}

func TestMode(t *testing.T) {
	h := newWorld("a", "b", "c")
	now := time.Now()
	la, lo := at(0)
	if m, _ := h.mode("a", now); m != "idle" {
		t.Fatalf("unplaced: idle, got %s", m)
	}
	h.addFix("a", la, lo, 10, 0, false, now)
	if m, n := h.mode("a", now); m != "idle" || n != idleNext {
		t.Fatalf("alone: idle, got %s %d", m, n)
	}
	lc, _ := at(700)
	h.addFix("c", lc, lo, 10, 0, false, now)
	if m, n := h.mode("a", now); m != "around" || n != aroundNext {
		t.Fatalf("someone ~700 m away: around, got %s %d", m, n)
	}
	lc, _ = at(120)
	h.addFix("c", lc, lo, 10, 0, false, now)
	if m, n := h.mode("a", now); m != "near" || n != nearNext {
		t.Fatalf("someone ~120 m away: near (GPS on), got %s %d", m, n)
	}
	lb, _ := at(20)
	h.addFix("b", lb, lo, 10, 0, false, now)
	if m, n := h.mode("a", now); m != "with" || n != withNext {
		t.Fatalf("someone in range: with, got %s %d", m, n)
	}
}

func TestHoldThenDrop(t *testing.T) {
	h := newWorld("a", "b")
	now := time.Now()
	tok(t, h, "a", now)
	tb := tok(t, h, "b", now)
	hear(h, "a", tb, rssi(-80), now)
	if !deck(h, "a", now)["b"] {
		t.Fatal("heard: shown")
	}
	if !deck(h, "a", now.Add(nearFor+15*time.Second))["b"] {
		t.Fatal("missed a few reports: still held")
	}
	if deck(h, "a", now.Add(holdFor+time.Second))["b"] {
		t.Fatal("silent past holdFor: dropped")
	}
}

func TestLeaveDropsAtOnce(t *testing.T) {
	h := newWorld("a", "b")
	now := time.Now()
	tok(t, h, "a", now)
	tb := tok(t, h, "b", now)
	hear(h, "a", tb, rssi(-70), now)
	la, lo := at(0)
	h.addFix("b", la, lo, 8, 0, false, now)
	deck(h, "a", now)
	if err := h.leave(context.Background(), "b"); err != nil {
		t.Fatal(err)
	}
	if deck(h, "a", now)["b"] {
		t.Fatal("left: off every deck at once, held or not")
	}
	if hear(h, "b", tok(t, h, "a", now), rssi(-70), now) != 0 {
		t.Fatal("a report from before the hide (old token) is refused")
	}
	h.addFix("b", la, lo, 8, 1000, false, now) // HTTP fix taken before the hide
	h.mu.Lock()
	got := h.fixes["b"]
	h.mu.Unlock()
	if got != nil {
		t.Fatal("hide guard: a fix taken before leaving is dropped")
	}
	h.addFix("b", la, lo, 8, 0, true, now.Add(time.Second)) // socket: ordered, lifts the guard
	h.mu.Lock()
	got = h.fixes["b"]
	h.mu.Unlock()
	if got == nil {
		t.Fatal("a socket fix after leaving is taken")
	}
}

func TestHiddenRefused(t *testing.T) {
	h := newWorld("a", "b")
	now := time.Now()
	ta := tok(t, h, "a", now)
	tb := tok(t, h, "b", now)
	h.setHidden("b", true)
	if hear(h, "b", ta, rssi(-60), now) != 0 || hear(h, "a", tb, rssi(-60), now) != 0 {
		t.Fatal("a hidden account neither reports nor is heard")
	}
	if h.needsToken("b", now) {
		t.Fatal("a hidden phone is not told to fetch a token")
	}
}

func TestNeedsToken(t *testing.T) {
	h := newWorld()
	now := time.Now()
	if !h.needsToken("a", now) {
		t.Fatal("no token: needs one")
	}
	tok(t, h, "a", now)
	if h.needsToken("a", now) {
		t.Fatal("a live token needs none")
	}
}

func TestBluetoothTokenRotation(t *testing.T) {
	h := newWorld()
	now := time.Now()
	t1 := tok(t, h, "b", now)
	if tok(t, h, "b", now.Add(time.Minute)) != t1 {
		t.Fatal("token should stay the same within a rotation")
	}
	t2 := tok(t, h, "b", now.Add(bleTokenEvery))
	t3 := tok(t, h, "b", now.Add(2*bleTokenEvery))
	if t2 == t1 || t3 == t2 {
		t.Fatal("token should rotate")
	}
	n := now.Add(2 * bleTokenEvery)
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.bleOwner(t2, n) != "b" || h.bleOwner(t3, n) != "b" || h.bleOwner(t1, n) != "" {
		t.Fatal("current and previous resolve, the one before doesn't")
	}
	if h.bleOwner(t2, n.Add(bleTokenGrace+time.Second)) != "" {
		t.Fatal("a replaced token stops resolving after its grace")
	}
}

func TestDemoDeck(t *testing.T) {
	h := newWorld()
	h.setDemo("rev")
	if d := deck(h, "rev", time.Now()); len(d) != len(demoCards) {
		t.Fatalf("demo account sees the demo cards: %v", d)
	}
}

func TestGeohash(t *testing.T) {
	if g := geohash(57.64911, 10.40744); g != "u4pruyd" {
		t.Fatalf("geohash = %s", g)
	}
	cells := geohashAround(57.64911, 10.40744)
	if len(cells) != 9 || !strings.HasPrefix(strings.Join(cells, ","), "") {
		t.Fatalf("around = %v", cells)
	}
	for _, m := range []float64{0, 10, 29, 30} { // anyone within 30 m is in the cell or a neighbour
		lb, lo := at(m)
		la, _ := at(0)
		found := false
		for _, c := range geohashAround(la, lo) {
			found = found || c == geohash(lb, lo)
		}
		if !found {
			t.Fatalf("%v m away not in the neighbourhood", m)
		}
	}
}

// Two locked iPhones can't hear each other; a third phone hearing both loudly puts them near.
func TestInferredThroughThirdPhone(t *testing.T) {
	h := newWorld("a", "b", "r", "far")
	now := time.Now()
	ta, tb, tf := tok(t, h, "a", now), tok(t, h, "b", now), tok(t, h, "far", now)
	tok(t, h, "r", now)
	h.addSightings("r", []sighting{{Tok: ta, RSSI: rssi(-70)}, {Tok: tb, RSSI: rssi(-80)}, {Tok: tf, RSSI: rssi(-95)}}, now)
	if !deck(h, "a", now)["b"] || !deck(h, "b", now)["a"] {
		t.Fatal("both heard loudly by r: a and b are near")
	}
	if deck(h, "a", now)["far"] {
		t.Fatal("heard weakly by r: could be 60 m from a, no card")
	}
	if deck(h, "a", now.Add(holdFor+nearFor))["b"] {
		t.Fatal("an inferred pair expires like any other")
	}
}

func TestWifiConfirmsAndVetoes(t *testing.T) {
	now := time.Now()
	la, lo := at(0)
	place := func(apart float64, acc int, wa, wb []uint32) bool {
		h := newWorld("a", "b")
		lb, _ := at(apart)
		h.addReport("a", report{lat: la, lng: lo, acc: acc, wifi: wa}, true, now)
		h.addReport("b", report{lat: lb, lng: lo, acc: acc, wifi: wb}, true, now)
		return deck(h, "a", now)["b"]
	}
	same := []uint32{1, 2, 3, 4, 5, 6}
	if !place(120, 30, same, []uint32{2, 3, 4, 5, 9}) {
		t.Fatal("indoor GPS 120 m apart, same Wi-Fi networks: same place, shown")
	}
	if place(200, 30, same, same) {
		t.Fatal("Wi-Fi can't stretch past 150 m")
	}
	if place(50, 25, same, []uint32{11, 12, 13, 14, 15}) {
		t.Fatal("50 m apart (inside 35 + 35 m), no network in common, plenty each: other building, not shown")
	}
	if !place(50, 25, same, []uint32{11, 12}) {
		t.Fatal("too few networks to judge: GPS decides")
	}
	if got := wifiIDs([]string{"0000000a", "00000002", "0000000a", "zz", "123"}); len(got) != 2 || got[0] != 2 || got[1] != 10 {
		t.Fatalf("wifi ids parsed sorted, unique, valid only: %v", got)
	}
}

func TestReporterSocketClosedLeavesSoon(t *testing.T) {
	h := newWorld("a", "b")
	now := time.Now()
	la, lo := at(0)
	lb, _ := at(10)
	h.addFix("a", la, lo, 10, 0, true, now)
	r := &Client{uid: "b", reporter: true, send: make(chan []byte, 4)}
	h.attach(r)
	h.addFixStill("b", lb, lo, 10, 0, true, true, now)
	h.detach(r)
	if !deck(h, "a", now.Add(lostGrace-time.Second))["b"] {
		t.Fatal("reporter gone briefly (network handover): still shown")
	}
	h.expire(now.Add(lostGrace + time.Second))
	if deck(h, "a", now.Add(lostGrace+time.Second))["b"] {
		t.Fatal("reporter gone past lostGrace: off the deck, no hold")
	}
	if len(r.send) != 0 {
		t.Fatal("a reporter socket never gets decks")
	}
}
