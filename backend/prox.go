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
	flushEvery   = 500 * time.Millisecond // the periodic flush; a fix or a new pair flushes at once...
	flushGap     = 50 * time.Millisecond  // ...but at most this often, so a crowd reporting is ≤ 20 flushes/s
	expireEvery  = 2 * time.Second
	profileTTL   = 5 * time.Minute
	bootGrace    = 15 * time.Second // after a restart, pairs come back with the phones' next reports

	nearFor = 45 * time.Second // a direct Bluetooth hearing counts this long: phones refresh every 20 s closed, so one missed report is covered
	holdFor = 60 * time.Second // a card stays this long after the last signal that put the pair close
	minRSSI = -100             // dBm, the radios' floor: pocketed phones 20-30 m apart or across a wall read -95 to -100
	// inferRSSI: a third phone that hears both A and B this loud (each within ~15 m of it,
	// closer through pockets) puts A and B within ~30 m of each other. That is how two
	// locked iPhones, which can't hear each other (iOS), find each other in a crowd.
	inferRSSI = -85
	fixFresh  = 2 * time.Minute

	// Location match (v3): shown within gpsRadiusM plus the two fixes' combined error (capped
	// at gpsSlackM), so nobody inside 35 m is missed indoors. Once shown, a card goes as soon as
	// both fresh fixes put the pair past that line by keepGoodM (both fixes ≤ vetoAccM) or
	// keepVagueM: the margin is what keeps noisy fixes from blinking it.
	gpsRadiusM = 35
	gpsSlackM  = 40
	maxAccM    = 100 // vaguer fixes (Android "approximate" is snapped ~2 km) never match
	gpsAccM    = 20  // vaguer than this counts as vague (stats)
	keepGoodM  = 5
	keepVagueM = 15
	stillFresh = 10 * time.Minute // a phone that says it isn't moving stays placed this long after its last report
	sameFix    = 2 * time.Second  // a re-sent fix's time (now − age) differs by network delay
	lostGrace  = 30 * time.Second // its reporter socket closed (app killed, location off, or a network
	// handover that reconnects in seconds): the fix stays this long, then goes

	// Wi-Fi (Android; hashed network ids, memory only): phones that see the same networks are
	// in the same place, so a strong overlap shows a pair GPS puts up to wifiMaxM apart (indoor
	// GPS drifts), and none at all, each seeing wifiVetoN+, vetoes a pair over wifiVetoM apart
	// (the building next door).
	wifiMax   = 12
	wifiSame  = 0.3
	wifiMinN  = 3
	wifiMaxM  = 150
	wifiVetoN = 5
	wifiVetoM = 20

	// What /api/presence tells a phone (mode, next = longest wait in seconds before it reports
	// again, moving or not): "with" someone in range, "near" anyone placed within ~1 km, else "idle".
	// The GPS chip runs only in with/near (and while the app is open): that is the battery.
	withNext   = 10
	nearNext   = 15  // someone within the geohash cells around (~150-450 m)
	aroundNext = 30  // someone within ~1 km: Wi-Fi/cell fixes every 25 m
	idleNext   = 120 // nobody: Wi-Fi/cell every 100 m. Also how long a still idle phone can be missing after a server restart
	// Outdoors (both have fresh open-sky fixes) Bluetooth carries 80 m+ at -95 dBm, so the
	// location match overrules it. Indoor fixes (vaguer, and sometimes 40 m off while
	// claiming 10 m) never veto Bluetooth: same-room pairs must not blink.
	vetoFresh = 60 * time.Second
	vetoAccM  = 15

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
	t        time.Time // taken
	at       time.Time // last reported: a still phone re-sends the same fix
	still    bool
	cell     string
	wifi     []uint32  // hashed Wi-Fi network ids seen with this fix, sorted
	lost     time.Time // its last reporter socket closed at
}

// report is one fix as a phone sends it.
type report struct {
	lat, lng float64
	acc      int
	age      int64 // ms since taken
	still    bool
	wifi     []uint32
}

// fresh: still placed. A still phone's fix stays good as long as it keeps reporting.
func (f *fix) fresh(now time.Time) bool {
	if f == nil || (!f.lost.IsZero() && now.Sub(f.lost) > lostGrace) {
		return false
	}
	if f.still {
		return now.Sub(f.at) <= stillFresh
	}
	return now.Sub(f.t) <= fixFresh
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
	reporters map[string]map[*Client]bool     // uid -> phone services' sockets: fixes in, mode out, never decks
	dirty     map[*Client]bool                // owed a fresh deck on the next flush
	arrived   chan struct{}                   // a new pair: flush now
	fixes     map[string]*fix                 // uid -> latest fix
	cells     map[string]map[string]bool      // geohash -> uids with a fix there
	areas     map[string]int                  // geohash-6 (~1.2 x 0.6 km) -> fixes there, for mode
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
		clients: map[string]map[*Client]bool{}, reporters: map[string]map[*Client]bool{}, dirty: map[*Client]bool{}, arrived: make(chan struct{}, 1),
		fixes: map[string]*fix{}, cells: map[string]map[string]bool{}, areas: map[string]int{},
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
	if c.reporter {
		if h.reporters[c.uid] == nil {
			h.reporters[c.uid] = map[*Client]bool{}
		}
		h.reporters[c.uid][c] = true
		h.mu.Unlock()
		return
	}
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
	if c.reporter {
		if set := h.reporters[c.uid]; set != nil {
			delete(set, c)
			if len(set) == 0 {
				delete(h.reporters, c.uid)
				if f := h.fixes[c.uid]; f != nil { // nothing reports for them now: gone in lostGrace unless it reconnects
					f.lost = time.Now()
				}
			}
		}
		return
	}
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
	if h.fixes[uid].fresh(now) {
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
	out := make([]*Client, 0, len(h.clients[uid])+len(h.reporters[uid]))
	for c := range h.clients[uid] {
		out = append(out, c)
	}
	for c := range h.reporters[uid] { // "vis": a hide stops the phone's service at once
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
	h.addFixStill(uid, lat, lng, acc, age, false, socket, now)
}

// addFixStill: still means the phone isn't moving, so this fix stays good while it keeps
// reporting (fix.fresh). The same fix sent again only refreshes it: no deck work.
func (h *Hub) addFixStill(uid string, lat, lng float64, acc int, age int64, still, socket bool, now time.Time) {
	h.addReport(uid, report{lat: lat, lng: lng, acc: acc, age: age, still: still}, socket, now)
}

// addReport records a phone's fix with what it saw (Wi-Fi). Socket reports are ordered, so
// they came after any hide on that socket.
func (h *Hub) addReport(uid string, r report, socket bool, now time.Time) {
	lat, lng, acc, age, still := r.lat, r.lng, r.acc, r.age, r.still
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
	if old != nil && old.fresh(now) && lat == old.lat && lng == old.lng && acc == old.acc && (t.Sub(old.t)).Abs() < sameFix {
		h.seen(uid, now) // the same fix re-sent (its age wobbles by the network's ms)
		old.at, old.still, old.lost = now, still, time.Time{}
		if r.wifi != nil {
			old.wifi = r.wifi
		}
		return
	}
	if old != nil && t.Before(old.t) {
		return
	}
	h.seen(uid, now)
	f := &fix{lat: lat, lng: lng, acc: acc, t: t, at: now, still: still, cell: geohash(lat, lng), wifi: r.wifi}
	if old != nil && old.cell != f.cell {
		h.unindex(uid, old.cell)
	}
	if h.cells[f.cell] == nil {
		h.cells[f.cell] = map[string]bool{}
	}
	if !h.cells[f.cell][uid] {
		h.cells[f.cell][uid] = true
		h.areas[f.cell[:ghPrec-1]]++
	}
	h.fixes[uid] = f
	h.markDirty(uid)
	for _, cell := range geohashAround(lat, lng) { // GPS neighbours may gain or lose this card
		for o := range h.cells[cell] {
			h.markDirty(o)
		}
	}
	for o := range h.partners[uid] { // a veto may change
		h.markDirty(o)
	}
	h.flushSoon() // arrivals and departures go out now, not on the next tick
}

func (h *Hub) unindex(uid, cell string) {
	if set := h.cells[cell]; set != nil && set[uid] {
		delete(set, uid)
		if len(set) == 0 {
			delete(h.cells, cell)
		}
		a := cell[:ghPrec-1]
		if h.areas[a]--; h.areas[a] <= 0 {
			delete(h.areas, a)
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

// gpsNear is the location match. known: both fixes are fresh and usable, so a miss means
// apart rather than unknown. shown: the card is on the deck now (the keep margin applies).
func gpsNear(a, b *fix, now time.Time, shown bool) (d float64, in, known bool) {
	if !a.fresh(now) || !b.fresh(now) || a.acc > maxAccM || b.acc > maxAccM {
		return 0, false, false
	}
	d = distM(a.lat, a.lng, b.lat, b.lng)
	shared, na, nb := overlap(a.wifi, b.wifi)
	if na >= wifiMinN && nb >= wifiMinN && float64(shared) >= wifiSame*float64(na+nb-shared) && d <= wifiMaxM {
		return d, true, true // same networks around them: same place, whatever indoor GPS says
	}
	if shared == 0 && na >= wifiVetoN && nb >= wifiVetoN && d > wifiVetoM {
		return d, false, true // each sees plenty of networks, none in common: different buildings
	}
	limit := gpsRadiusM + min(math.Hypot(errM(a.acc), errM(b.acc)), gpsSlackM)
	if shown {
		if a.acc > 0 && b.acc > 0 && a.acc <= vetoAccM && b.acc <= vetoAccM {
			limit += keepGoodM
		} else {
			limit += keepVagueM
		}
	}
	return d, d <= limit, true
}

// overlap counts the Wi-Fi networks two sorted id lists share.
func overlap(a, b []uint32) (shared, na, nb int) {
	for i, j := 0, 0; i < len(a) && j < len(b); {
		switch {
		case a[i] == b[j]:
			shared++
			i++
			j++
		case a[i] < b[j]:
			i++
		default:
			j++
		}
	}
	return shared, len(a), len(b)
}

// errM: a fix's error in metres (0 = not given: assume the worst we allow for).
func errM(acc int) float64 {
	if acc <= 0 {
		return gpsSlackM
	}
	return float64(acc)
}

// outdoors: both have fresh open-sky fixes.
func outdoors(a, b *fix, now time.Time) bool {
	return a != nil && b != nil && now.Sub(a.t) <= vetoFresh && now.Sub(b.t) <= vetoFresh &&
		a.acc > 0 && b.acc > 0 && a.acc <= vetoAccM && b.acc <= vetoAccM
}

// vetoed: outdoors, the location match decides whatever Bluetooth says (it carries 80 m+).
func vetoed(a, b *fix, now time.Time, shown bool) bool {
	if !outdoors(a, b, now) {
		return false
	}
	_, in, known := gpsNear(a, b, now, shown)
	return known && !in
}

// mode is how hard uid's phone should work (see withNext): everyone else's position stays
// current only as long as their phones report, so a phone with nobody around can rest.
func (h *Hub) mode(uid string, now time.Time) (string, int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	me := h.fixes[uid]
	if h.hidden[uid] || !me.fresh(now) {
		return "idle", idleNext
	}
	for o := range h.partners[uid] {
		if p := h.pairs[key(uid, o)]; p != nil && now.Sub(p.near) <= nearFor {
			return "with", withNext
		}
	}
	near := false
	for _, cell := range geohashAround(me.lat, me.lng) {
		for o := range h.cells[cell] {
			if o == uid || h.hidden[o] {
				continue
			}
			if _, in, _ := gpsNear(me, h.fixes[o], now, true); in {
				return "with", withNext
			}
			near = true
		}
	}
	if near {
		return "near", nearNext
	}
	for _, a := range geohashAroundP(me.lat, me.lng, ghPrec-1) {
		n := h.areas[a]
		if a == me.cell[:ghPrec-1] {
			n--
		}
		if n > 0 {
			return "around", aroundNext
		}
	}
	return "idle", idleNext
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
	if me.fresh(now) {
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
		if vetoed(me, them, now, h.onDeck[viewer][o]) {
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
		if gd, in, known := gpsNear(me, them, now, h.onDeck[viewer][o]); in {
			sig = now // both fixes are current: the hold starts when one goes stale
			if gd < d {
				d, ble = gd, false
			}
		} else if known && sig.IsZero() { // both placed and apart, nothing else says close: gone now, no hold
			delete(ev, o)
			continue
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
		if !f.fresh(now) { // their phone stopped reporting: off every deck now (Bluetooth, if heard, puts it back)
			h.unindex(uid, f.cell)
			delete(h.fixes, uid)
			for viewer, ev := range h.evidence {
				if _, ok := ev[uid]; ok {
					delete(ev, uid)
					h.markDirty(viewer)
				}
			}
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
	var last time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case <-h.arrived:
			if wait := flushGap - time.Since(last); wait > 0 {
				time.Sleep(wait) // everything arriving meanwhile goes out in this flush
			}
			last = time.Now()
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

// POST /api/presence {lat,lng,acc,age,still}: a phone's fix → {ok, mode, next}: how hard the
// phone should work and the longest it may wait before reporting again (docs/proximity.md).
func (a *App) handlePresence(w http.ResponseWriter, r *http.Request, uid string) {
	var in inbound
	if decodeJSON(r, &in) != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid body"))
		return
	}
	now := time.Now()
	a.hub.addReport(uid, in.report(), false, now)
	mode, next := a.hub.mode(uid, now)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "mode": mode, "next": next})
}
