package main

import (
	"context"
	"log"
	"log/slog"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Proximity v2: who is within ~30 m of whom. The contract with the phones and the
// reasons for every rule are in docs/proximity.md. Everything here is memory only:
// nothing about who was near whom, or where anyone was, is stored.
const (
	maxDeck      = 50
	maxSightings = 32
	flushEvery   = 500 * time.Millisecond // decks go out at most twice a second; a new pair goes at once
	expireEvery  = 2 * time.Second
	profileTTL   = 5 * time.Minute
	bootGrace    = 15 * time.Second // after a restart, pairs come back with the phones' next reports

	nearFor = 30 * time.Second // a direct Bluetooth hearing counts this long
	holdFor = 60 * time.Second // a card stays this long after the last signal that put the pair close
	minRSSI = -100             // dBm, the radios' floor: pocketed phones 20-30 m apart or across a wall read -95 to -100
	// inferRSSI: a third phone that hears both A and B this loud (each within ~15 m of it,
	// closer through pockets) puts A and B within ~30 m of each other. That is how two
	// locked iPhones, which can't hear each other (iOS), find each other in a crowd.
	inferRSSI = -85
	fixFresh  = 2 * time.Minute

	gpsAccM    = 20 // GPS-only match: both fixes at least this good...
	gpsRadiusM = 30 // ...and this close
	// Outdoors (both have fresh open-sky fixes) Bluetooth carries 80 m+ at -95 dBm, so fixes
	// further apart than vetoM plus their mean error overrule it. Indoor fixes (vaguer, and
	// sometimes 40 m off while claiming 10 m) never veto: same-room pairs must not blink.
	vetoFresh = 60 * time.Second
	vetoAccM  = 15
	vetoM     = 35

	wakeAfter  = 75 * time.Second
	wakeWindow = 12 * time.Hour
	wakeGap    = 10 * time.Minute
	scanGap    = time.Minute // relay-driven scan requests, per user
)

// nearbyPerson is the card the app renders. Img is a cacheable URL, never the photo.
type nearbyPerson struct {
	UID  string `json:"uid"`
	Name string `json:"name"`
	Role string `json:"role"`
	Desc string `json:"desc"`
	Img  string `json:"img"`
}

type fix struct {
	lat, lng float64
	acc      int
	t        time.Time
	cell     string
}

type pairKey [2]string

func key(a, b string) pairKey {
	if a < b {
		return pairKey{a, b}
	}
	return pairKey{b, a}
}

// pair is what two accounts' phones said about each other. Either side hearing counts for both.
type pair struct {
	near  time.Time // last direct hearing at or above minRSSI
	rssi  int       // strongest since the pair became near (0: over a connection, the strongest)
	relay time.Time // last time a third phone said they were near (never shows a card)
	loud  time.Time // last direct hearing at or above inferRSSI
}

// Hub owns the realtime state: sockets, fixes, Bluetooth pairs and decks.
type Hub struct {
	mu        sync.Mutex
	clients   map[string]map[*Client]bool     // uid -> open sockets (one per device: never close the older)
	dirty     map[*Client]bool                // owed a fresh deck on the next flush
	arrived   chan struct{}                   // a new pair: flush now
	fixes     map[string]*fix                 // uid -> latest fix
	cells     map[string]map[string]bool      // geohash -> uids with a fix there
	pairs     map[pairKey]*pair               //
	partners  map[string]map[string]bool      // uid -> uids it has a pair with
	evidence  map[string]map[string]time.Time // viewer -> uid -> time of the latest signal that put them close (holds)
	onDeck    map[string]map[string]bool      // viewer -> uids on the last list sent
	gone      map[string]time.Time            // hide guard: HTTP fixes taken before this are dropped
	hidden    map[string]bool                 // hid their profile (every device)
	demo      string                          // App Review's account (demo.go)
	ble       bleState
	lastSeen  map[string]time.Time // last report, for wake pushes
	firstSeen map[string]time.Time // first report since starting, for the first_match latency
	wokenAt   map[string]time.Time
	scannedAt map[string]time.Time
	wake      func(uids []string) // silent push (set by App; nil in tests)

	store     *Store
	prof      *profileCache
	photoBase string

	lastRetention time.Time
	bootAt        time.Time
	stat          proxStats
}

// proxStats are per-minute counters for the logs and admin telemetry. Counts only.
type proxStats struct {
	fixes, vague, sightings, relays atomic.Int64
	pairsBLE, pairsGPS, vetoed      atomic.Int64 // new pairs by signal; Bluetooth pairs overruled by GPS
	bleTokens, wakes, scans         atomic.Int64
	dropped, flushMaxMS             atomic.Int64
}

func newHub(store *Store) *Hub {
	return &Hub{
		clients: map[string]map[*Client]bool{}, dirty: map[*Client]bool{}, arrived: make(chan struct{}, 1),
		fixes: map[string]*fix{}, cells: map[string]map[string]bool{},
		pairs: map[pairKey]*pair{}, partners: map[string]map[string]bool{},
		evidence: map[string]map[string]time.Time{}, onDeck: map[string]map[string]bool{},
		gone: map[string]time.Time{}, hidden: map[string]bool{}, ble: newBLEState(),
		lastSeen: map[string]time.Time{}, firstSeen: map[string]time.Time{},
		wokenAt: map[string]time.Time{}, scannedAt: map[string]time.Time{},
		store: store, prof: newProfileCache(store),
	}
}

// ---- sockets ----

// attach registers a socket and sends its deck at once when we can place the user
// (an empty list that only means "we don't know yet" would blank the app's deck).
func (h *Hub) attach(c *Client) {
	h.mu.Lock()
	if h.clients[c.uid] == nil {
		h.clients[c.uid] = map[*Client]bool{}
	}
	h.clients[c.uid][c] = true
	known := h.knows(c.uid, time.Now())
	h.mu.Unlock()
	if known {
		h.recompute(c)
	}
}

func (h *Hub) detach(c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if set := h.clients[c.uid]; set != nil {
		delete(set, c)
		if len(set) == 0 {
			delete(h.clients, c.uid)
		}
	}
	delete(h.dirty, c)
}

// knows: a fresh fix, a near pair, or the demo account. Caller holds h.mu.
func (h *Hub) knows(uid string, now time.Time) bool {
	if uid == h.demo && !h.hidden[uid] {
		return true
	}
	if f := h.fixes[uid]; f != nil && now.Sub(f.t) <= fixFresh {
		return true
	}
	for o := range h.partners[uid] {
		if p := h.pairs[key(uid, o)]; p != nil && now.Sub(p.near) <= nearFor {
			return true
		}
	}
	return false
}

func (h *Hub) clientsOf(uid string) []*Client {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]*Client, 0, len(h.clients[uid]))
	for c := range h.clients[uid] {
		out = append(out, c)
	}
	return out
}

// pushNotification delivers a payload to a user's open sockets, if any.
func (h *Hub) pushNotification(uid string, payload any) {
	for _, c := range h.clientsOf(uid) {
		c.sendJSON(payload)
	}
}

func (h *Hub) online() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.clients)
}

// stats: open sockets, and people we can place right now (fresh fix or near pair).
func (h *Hub) stats() (conns, live int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	now := time.Now()
	for _, set := range h.clients {
		conns += len(set)
	}
	seen := map[string]bool{}
	for uid := range h.fixes {
		seen[uid] = true
	}
	for k, p := range h.pairs {
		if now.Sub(p.near) <= nearFor {
			seen[k[0]], seen[k[1]] = true, true
		}
	}
	return conns, len(seen)
}

// live is one user's realtime state for the admin user view (never a position).
func (h *Hub) live(uid string) map[string]any {
	h.mu.Lock()
	defer h.mu.Unlock()
	now := time.Now()
	out := map[string]any{"sockets": len(h.clients[uid]), "onDeck": len(h.onDeck[uid]), "visible": h.knows(uid, now)}
	if f := h.fixes[uid]; f != nil {
		out["fixAgeS"] = int(now.Sub(f.t).Seconds())
		out["fixAccM"] = f.acc
	}
	return out
}

func (h *Hub) setHidden(uid string, on bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if on {
		h.hidden[uid] = true
	} else {
		delete(h.hidden, uid)
	}
}

func (h *Hub) isHidden(uid string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.hidden[uid]
}

// markDirty queues fresh decks for these users' sockets. Caller holds h.mu.
func (h *Hub) markDirty(uids ...string) {
	for _, u := range uids {
		for c := range h.clients[u] {
			h.dirty[c] = true
		}
	}
}

func (h *Hub) markClient(c *Client) {
	h.mu.Lock()
	h.dirty[c] = true
	h.mu.Unlock()
}

func (h *Hub) flushSoon() {
	select { // one pending signal covers a crowd arriving
	case h.arrived <- struct{}{}:
	default:
	}
}

// ---- signals ----

func (h *Hub) seen(uid string, t time.Time) {
	h.lastSeen[uid] = t
	if _, ok := h.firstSeen[uid]; !ok {
		h.firstSeen[uid] = t
	}
}

// addFix records a GPS fix: from the browser's socket (ordered: lifts the hide guard)
// or the phone's POST /api/presence (dropped if taken before a hide). Its time is when
// the phone took it; an older fix never replaces a newer one.
func (h *Hub) addFix(uid string, lat, lng float64, acc int, age int64, socket bool, now time.Time) {
	if lat < -90 || lat > 90 || lng < -180 || lng > 180 || (lat == 0 && lng == 0) {
		return
	}
	acc = max(0, min(acc, 999))
	age = max(0, min(age, int64(24*time.Hour/time.Millisecond)))
	t := now.Add(-time.Duration(age) * time.Millisecond)
	h.stat.fixes.Add(1)
	if acc == 0 || acc > gpsAccM {
		h.stat.vague.Add(1)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.hidden[uid] {
		return
	}
	if socket {
		delete(h.gone, uid)
	} else if g, ok := h.gone[uid]; ok && t.Before(g) {
		return
	}
	old := h.fixes[uid]
	if old != nil && t.Before(old.t) {
		return
	}
	f := &fix{lat: lat, lng: lng, acc: acc, t: t, cell: geohash(lat, lng)}
	if old != nil && old.cell != f.cell {
		h.unindex(uid, old.cell)
	}
	if h.cells[f.cell] == nil {
		h.cells[f.cell] = map[string]bool{}
	}
	h.cells[f.cell][uid] = true
	h.fixes[uid] = f
	h.seen(uid, now)
	h.markDirty(uid)
	for _, cell := range geohashAround(lat, lng) { // GPS neighbours may gain or lose this card
		for o := range h.cells[cell] {
			h.markDirty(o)
		}
	}
	for o := range h.partners[uid] { // a veto may change
		h.markDirty(o)
	}
	if old == nil || now.Sub(old.t) > fixFresh {
		h.flushSoon() // the app may be waiting on its scan screen for this
	}
}

func (h *Hub) unindex(uid, cell string) {
	if set := h.cells[cell]; set != nil {
		delete(set, uid)
		if len(set) == 0 {
			delete(h.cells, cell)
		}
	}
}

type sighting struct {
	Tok  string `json:"tok"`
	RSSI *int   `json:"rssi"` // nil: heard over a connection
	Via  string `json:"via"`  // "relay": a third phone heard it, not this one
}

// addSightings records what uid's phone heard. The reporter must hold a live token
// itself: hiding revokes it, so a report in flight from before a hide can't make anyone
// visible. Returns how many tokens counted.
func (h *Hub) addSightings(uid string, seen []sighting, now time.Time) int {
	var scan []string
	h.mu.Lock()
	if t, ok := h.ble.cur[uid]; !ok || h.bleOwner(t, now) != uid || h.hidden[uid] {
		h.mu.Unlock()
		return 0
	}
	n := 0
	for _, s := range seen {
		other := h.bleOwner(s.Tok, now)
		if other == "" || other == uid || h.hidden[other] {
			continue
		}
		k := key(uid, other)
		p := h.pairs[k]
		if p == nil {
			p = &pair{}
			h.pairs[k] = p
			h.link(uid, other)
		}
		n++
		if s.Via == "relay" {
			h.stat.relays.Add(1)
			p.relay = now
			if now.Sub(p.near) > nearFor { // ask both phones to check directly
				scan = append(scan, h.scanDue(uid, now)...)
				scan = append(scan, h.scanDue(other, now)...)
			}
			continue
		}
		rssi := 0
		if s.RSSI != nil {
			if *s.RSSI >= 0 || *s.RSSI < minRSSI {
				n--
				continue
			}
			rssi = *s.RSSI
		}
		fresh := now.Sub(p.near) > nearFor
		if fresh || rssi == 0 || (p.rssi != 0 && rssi > p.rssi) {
			p.rssi = rssi
		}
		p.near = now
		if fresh {
			h.stat.pairsBLE.Add(1)
			h.markNew(uid, other)
			h.markNew(other, uid)
			h.flushSoon()
		}
		if rssi != 0 && rssi >= inferRSSI {
			if now.Sub(p.loud) > nearFor { // newly loud: decks of whoever else is loud with either may gain a card
				h.markLoud(uid, other, now)
				h.markLoud(other, uid, now)
				h.flushSoon()
			}
			p.loud = now
		}
	}
	h.stat.sightings.Add(int64(n))
	if n > 0 {
		h.seen(uid, now)
	}
	clients := map[string][]*Client{}
	for _, u := range scan {
		for c := range h.clients[u] {
			clients[u] = append(clients[u], c)
		}
	}
	h.mu.Unlock()
	var push []string
	for _, u := range scan {
		if len(clients[u]) == 0 {
			push = append(push, u)
		}
		for _, c := range clients[u] {
			c.sendJSON(map[string]string{"type": "scan"})
		}
	}
	if len(push) > 0 && h.wake != nil {
		go h.wake(push)
	}
	return n
}

// markNew queues viewer's deck if other isn't on it yet: in a crowd the same pairs are
// heard over and over, and rebuilding decks that already show the card swamped the server
// (expire still rebuilds each deck when its holds run out). Caller holds h.mu.
func (h *Hub) markNew(viewer, other string) {
	if !h.onDeck[viewer][other] {
		h.markDirty(viewer)
	}
}

// markLoud queues the decks of everyone loud with common: x may now be inferred near them
// (see inferRSSI). Only on a pair becoming loud, so a crowd costs this once per pair. Caller holds h.mu.
func (h *Hub) markLoud(common, x string, now time.Time) {
	for y := range h.partners[common] {
		if len(h.clients[y]) == 0 || y == x || h.onDeck[y][x] { // only open apps, only if the card isn't there yet
			continue
		}
		if q := h.pairs[key(common, y)]; q != nil && now.Sub(q.loud) <= nearFor {
			h.markDirty(y)
		}
	}
}

// inferred: everyone heard loudly, within nearFor, by a phone that heard viewer loudly, with
// the time of the older of the two hearings. Built per deck (open apps only), not per report:
// per report it was ~1,500 map lookups under h.mu and capped the server near 4,500 reports/s.
// Caller holds h.mu.
func (h *Hub) inferred(viewer string, now time.Time) map[string]time.Time {
	var out map[string]time.Time
	for c := range h.partners[viewer] {
		p := h.pairs[key(viewer, c)]
		if p == nil || now.Sub(p.loud) > nearFor {
			continue
		}
		for y := range h.partners[c] {
			if y == viewer {
				continue
			}
			q := h.pairs[key(c, y)]
			if q == nil || now.Sub(q.loud) > nearFor {
				continue
			}
			t := p.loud
			if q.loud.Before(t) {
				t = q.loud
			}
			if out == nil {
				out = map[string]time.Time{}
			}
			if t.After(out[y]) {
				out[y] = t
			}
		}
	}
	return out
}

func (h *Hub) link(a, b string) {
	for _, x := range [][2]string{{a, b}, {b, a}} {
		if h.partners[x[0]] == nil {
			h.partners[x[0]] = map[string]bool{}
		}
		h.partners[x[0]][x[1]] = true
	}
}

// scanDue rate-limits relay-driven scan requests. Caller holds h.mu.
func (h *Hub) scanDue(uid string, now time.Time) []string {
	if now.Sub(h.scannedAt[uid]) < scanGap {
		return nil
	}
	h.scannedAt[uid] = now
	h.stat.scans.Add(1)
	return []string{uid}
}

// ---- rules ----

// gpsNear: a GPS-only match (browsers, or a phone with Bluetooth off).
func (h *Hub) gpsNear(a, b *fix, now time.Time) (float64, bool) {
	if a == nil || b == nil || now.Sub(a.t) > fixFresh || now.Sub(b.t) > fixFresh ||
		a.acc <= 0 || b.acc <= 0 || a.acc > gpsAccM || b.acc > gpsAccM {
		return 0, false
	}
	d := distM(a.lat, a.lng, b.lat, b.lng)
	return d, d <= gpsRadiusM
}

// outdoors: both have fresh open-sky fixes.
func outdoors(a, b *fix, now time.Time) bool {
	return a != nil && b != nil && now.Sub(a.t) <= vetoFresh && now.Sub(b.t) <= vetoFresh &&
		a.acc > 0 && b.acc > 0 && a.acc <= vetoAccM && b.acc <= vetoAccM
}

// vetoed: outdoors and too far apart for 30 m, whatever Bluetooth says.
func vetoed(a, b *fix, now time.Time) bool {
	return outdoors(a, b, now) && distM(a.lat, a.lng, b.lat, b.lng) > vetoM+float64(a.acc+b.acc)/2
}

// rssiM is a rough distance from signal strength (free-space, -59 dBm at 1 m), for ordering only.
func rssiM(rssi int) float64 {
	if rssi == 0 {
		return 1
	}
	return math.Pow(10, float64(-59-rssi)/20)
}

type scored struct {
	uid string
	d   float64
	ble bool
}

// candidates: who belongs on viewer's deck now. A pair is close while Bluetooth heard it
// within nearFor or GPS matches, unless vetoed; once on the deck it is held until holdFor
// passes after the last such signal (timed by the signal, never by when we looked).
// Caller holds h.mu.
func (h *Hub) candidates(viewer string, now time.Time) []scored {
	if h.hidden[viewer] {
		return nil
	}
	look := map[string]bool{}
	for o := range h.partners[viewer] {
		look[o] = true
	}
	me := h.fixes[viewer]
	if me != nil && now.Sub(me.t) <= fixFresh {
		for _, cell := range geohashAround(me.lat, me.lng) {
			for o := range h.cells[cell] {
				look[o] = true
			}
		}
	}
	ev := h.evidence[viewer]
	for o := range ev {
		look[o] = true
	}
	inf := h.inferred(viewer, now)
	for o := range inf {
		look[o] = true
	}
	var out []scored
	for o := range look {
		if o == viewer || h.hidden[o] {
			continue
		}
		them := h.fixes[o]
		if vetoed(me, them, now) {
			delete(ev, o)
			h.stat.vetoed.Add(1)
			continue
		}
		var sig time.Time
		d, ble := math.MaxFloat64, false
		if p := h.pairs[key(viewer, o)]; p != nil && now.Sub(p.near) <= nearFor {
			sig, d, ble = p.near, rssiM(p.rssi), true
		}
		if t, ok := inf[o]; ok && t.After(sig) { // through a third phone
			sig, ble = t, true
			d = min(d, 20)
		}
		if gd, ok := h.gpsNear(me, them, now); ok {
			t := me.t
			if them.t.Before(t) {
				t = them.t
			}
			if t.After(sig) {
				sig = t
			}
			if gd < d {
				d, ble = gd, false
			}
		}
		if !sig.IsZero() {
			if ev == nil {
				ev = map[string]time.Time{}
				h.evidence[viewer] = ev
			}
			if sig.After(ev[o]) {
				ev[o] = sig
			}
		} else if t, ok := ev[o]; !ok || now.Sub(t) > holdFor {
			delete(ev, o)
			continue
		}
		if d == math.MaxFloat64 { // held: last place on the deck
			d = 1e9
		}
		out = append(out, scored{uid: o, d: d, ble: ble})
	}
	if viewer == h.demo {
		for i, dc := range demoCards {
			out = append(out, scored{uid: dc.uid, d: float64(i)})
		}
	}
	return out
}

// recompute builds and pushes one socket's deck, closest first. Profile lookups run
// outside the lock (they may hit Postgres).
func (h *Hub) recompute(c *Client) {
	if wait := bootGrace - time.Since(h.bootAt); !h.bootAt.IsZero() && wait > 0 {
		time.AfterFunc(wait, func() { h.markClient(c); h.flushSoon() })
		return
	}
	now := time.Now()
	h.mu.Lock()
	cands := h.candidates(c.uid, now)
	h.mu.Unlock()
	sort.Slice(cands, func(i, j int) bool { return cands[i].d < cands[j].d })
	uids := make([]string, 0, min(len(cands), 2*maxDeck))
	for _, s := range cands[:min(len(cands), 2*maxDeck)] {
		uids = append(uids, s.uid)
	}
	h.prof.warm(uids)
	people := make([]nearbyPerson, 0, min(len(cands), maxDeck))
	shown := map[string]bool{}
	via := map[string]bool{}
	for _, s := range cands {
		if len(people) == maxDeck {
			break
		}
		p, ok := h.prof.get(s.uid)
		if !ok || !p.complete() {
			continue
		}
		shown[s.uid], via[s.uid] = true, s.ble
		people = append(people, nearbyPerson{
			UID: s.uid, Name: p.Name, Role: p.Role, Desc: p.Look,
			Img: h.photoBase + "/api/photo/" + s.uid + "?v=" + strconv.FormatInt(p.UpdatedAt.Unix(), 10),
		})
	}
	h.mu.Lock()
	if h.clients[c.uid][c] || len(h.clients[c.uid]) == 0 {
		for uid := range shown { // a card arriving: how long since the later of the two started reporting
			if h.onDeck[c.uid][uid] || strings.HasPrefix(uid, demoPrefix) {
				continue
			}
			start := h.firstSeen[c.uid]
			if t := h.firstSeen[uid]; t.After(start) {
				start = t
			}
			if !start.IsZero() {
				how := "gps"
				if via[uid] {
					how = "ble"
				}
				slog.Info("first_match", "via", how, "after_ms", now.Sub(start).Milliseconds())
				tel.matched(now.Sub(start).Milliseconds(), via[uid])
			}
		}
		for uid := range h.onDeck[c.uid] {
			if !shown[uid] {
				h.stat.dropped.Add(1)
			}
		}
		h.onDeck[c.uid] = shown
	}
	h.mu.Unlock()
	if !c.sendJSON(map[string]any{"type": "nearby", "people": people}) {
		h.markClient(c) // buffer full: try again on the next flush, don't lose the change
	}
}

func (h *Hub) flush() {
	h.mu.Lock()
	batch := h.dirty
	h.dirty = map[*Client]bool{}
	h.mu.Unlock()
	for c := range batch {
		h.recompute(c)
	}
}

// expire queues the decks whose holds ran out and forgets old fixes and pairs.
func (h *Hub) expire(now time.Time) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for viewer, deck := range h.onDeck {
		for uid := range deck {
			if now.Sub(h.evidence[viewer][uid]) > holdFor {
				h.markDirty(viewer)
				break
			}
		}
	}
	for uid, f := range h.fixes {
		if now.Sub(f.t) > fixFresh {
			h.unindex(uid, f.cell)
			delete(h.fixes, uid)
		}
	}
	for k, p := range h.pairs {
		if now.Sub(p.near) > holdFor+nearFor && now.Sub(p.relay) > holdFor {
			delete(h.pairs, k)
			delete(h.partners[k[0]], k[1])
			delete(h.partners[k[1]], k[0])
		}
	}
	for uid, set := range h.partners {
		if len(set) == 0 {
			delete(h.partners, uid)
		}
	}
	for viewer, ev := range h.evidence {
		for uid, t := range ev {
			if now.Sub(t) > holdFor && !h.onDeck[viewer][uid] {
				delete(ev, uid)
			}
		}
		if len(ev) == 0 {
			delete(h.evidence, viewer)
		}
	}
	for viewer := range h.onDeck {
		if len(h.clients[viewer]) == 0 {
			delete(h.onDeck, viewer)
		}
	}
}

// ---- leaving ----

// leave takes someone off every deck now (hide, logout, account deletion): pairs,
// fix and tokens go, their own decks are emptied, and reports in flight from before
// are refused (the hide guard; a socket "pos" lifts it).
func (h *Hub) leave(ctx context.Context, uid string) error {
	h.mu.Lock()
	h.gone[uid] = time.Now()
	h.revokeBLE(uid)
	for o := range h.partners[uid] {
		delete(h.pairs, key(uid, o))
		delete(h.partners[o], uid)
		h.markDirty(o)
	}
	delete(h.partners, uid)
	for viewer, ev := range h.evidence {
		if _, ok := ev[uid]; ok {
			delete(ev, uid)
			h.markDirty(viewer)
		}
	}
	if f := h.fixes[uid]; f != nil {
		h.unindex(uid, f.cell)
		delete(h.fixes, uid)
	}
	delete(h.evidence, uid)
	delete(h.onDeck, uid)
	delete(h.lastSeen, uid)
	delete(h.firstSeen, uid)
	mine := make([]*Client, 0, len(h.clients[uid]))
	for c := range h.clients[uid] {
		mine = append(mine, c)
	}
	h.mu.Unlock()
	h.flushSoon()
	for _, c := range mine {
		c.sendJSON(map[string]any{"type": "nearby", "people": []nearbyPerson{}})
	}
	if h.store == nil {
		return nil
	}
	return h.store.deleteBLE(ctx, uid)
}

// profileChanged sends a saved card to everyone already showing it.
func (h *Hub) profileChanged(uid string) {
	h.prof.invalidate(uid)
	h.mu.Lock()
	for viewer, deck := range h.onDeck {
		if deck[uid] {
			h.markDirty(viewer)
		}
	}
	h.mu.Unlock()
	h.flushSoon()
}

// ---- wake pushes ----

// quiet returns the phones that reported and went silent: a silent push at wakeAfter
// (before their cards expire), then one per wakeGap, for wakeWindow.
func (h *Hub) quiet(now time.Time) []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []string
	for uid, t := range h.lastSeen {
		switch age := now.Sub(t); {
		case age > wakeWindow:
			delete(h.lastSeen, uid)
			delete(h.firstSeen, uid)
		case age >= wakeAfter && now.Sub(h.wokenAt[uid]) >= wakeGap:
			h.wokenAt[uid] = now
			out = append(out, uid)
		}
	}
	for uid, t := range h.wokenAt {
		if now.Sub(t) > wakeGap {
			delete(h.wokenAt, uid)
		}
	}
	for uid, t := range h.scannedAt {
		if now.Sub(t) > scanGap {
			delete(h.scannedAt, uid)
		}
	}
	return out
}

func (h *Hub) wakeQuiet(now time.Time) {
	uids := h.quiet(now)
	if len(uids) == 0 || h.wake == nil {
		return
	}
	h.stat.wakes.Add(int64(len(uids)))
	go h.wake(uids)
}

// ---- loop ----

func (h *Hub) run(ctx context.Context) {
	flush, expire, sweep := time.NewTicker(flushEvery), time.NewTicker(expireEvery), time.NewTicker(time.Minute)
	defer flush.Stop()
	defer expire.Stop()
	defer sweep.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-h.arrived:
			h.flush()
		case <-flush.C:
			start := time.Now()
			h.flush()
			if ms := time.Since(start).Milliseconds(); ms > h.stat.flushMaxMS.Load() {
				h.stat.flushMaxMS.Store(ms)
			}
		case <-expire.C:
			now := time.Now()
			h.expire(now)
			h.wakeQuiet(now)
		case <-sweep.C:
			h.sweep()
		}
	}
}

// sweep is the once-a-minute housekeeping: profile cache, sessions, tokens, counters,
// and the daily retention.
func (h *Hub) sweep() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	h.prof.purge()
	h.store.purgeSessions()
	h.mu.Lock()
	h.purgeBLE(time.Now())
	h.mu.Unlock()
	conns, live := h.stats()
	s := &h.stat
	fixes, vague, sightings, ble := s.fixes.Swap(0), s.vague.Swap(0), s.sightings.Swap(0), s.pairsBLE.Swap(0)
	tokens, wakes := s.bleTokens.Swap(0), s.wakes.Swap(0)
	slog.Info("proximity", "clients", conns, "live", live, "fixes", fixes, "vague_fixes", vague,
		"sightings", sightings, "relays", s.relays.Swap(0), "pairs_ble", ble, "vetoed", s.vetoed.Swap(0),
		"ble_tokens", tokens, "wakes", wakes, "scans", s.scans.Swap(0), "deck_drops", s.dropped.Swap(0),
		"flush_max_ms", s.flushMaxMS.Swap(0))
	tel.proximity(fixes, vague, ble, ble, sightings, tokens, wakes)
	if time.Since(h.lastRetention) > 24*time.Hour {
		h.lastRetention = time.Now()
		if err := h.store.applyRetention(ctx); err != nil {
			log.Printf("retention: %v", err)
		}
	}
}

// ---- HTTP ----

// POST /api/sightings {"seen":[{"tok":"…","rssi":-70},{"tok":"…","via":"relay"}]}
// 409: the reporter holds no live token, so nobody can hear it; the phone fetches one now.
func (a *App) handleSightings(w http.ResponseWriter, r *http.Request, uid string) {
	var in struct{ Seen []sighting }
	if decodeJSON(r, &in) != nil || len(in.Seen) > maxSightings {
		writeJSON(w, http.StatusBadRequest, errBody("invalid body"))
		return
	}
	now := time.Now()
	if a.hub.addSightings(uid, in.Seen, now) == 0 && a.hub.needsToken(uid, now) {
		writeJSON(w, http.StatusConflict, errBody("token"))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /api/presence {lat,lng,acc,age}: a phone's fix (sent while the app is open).
func (a *App) handlePresence(w http.ResponseWriter, r *http.Request, uid string) {
	var in inbound
	if decodeJSON(r, &in) != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid body"))
		return
	}
	a.hub.addFix(uid, in.Lat, in.Lng, in.Acc, in.Age, false, time.Now())
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
