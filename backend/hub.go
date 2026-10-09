package main

import (
	"context"
	"log"
	"log/slog"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

const (
	maxDeck    = 50                     // cards per push: the closest 50 is more than anyone swipes through
	flushEvery = 500 * time.Millisecond // nearby lists go out at most twice a second per client
	saveEvery  = 5 * time.Second        // positions reach Postgres in one batch this often (memory is the live copy)
	profileTTL = 5 * time.Minute
)

// Pres is one person's live position, held in memory for fast matching and
// mirrored to Postgres for durability across restarts.
type Pres struct {
	UID  string
	Cell string
	Lat  float64
	Lng  float64
	Acc  int
	T    time.Time
}

// nearbyPerson is the card the client renders. Img is a cacheable URL, never
// the photo itself: inlining photos made each push (people x ~80 KB).
type nearbyPerson struct {
	UID  string `json:"uid"`
	Name string `json:"name"`
	Role string `json:"role"`
	Desc string `json:"desc"`
	Img  string `json:"img"`
}

// Hub owns all realtime state: who is where, and who is listening.
type Hub struct {
	mu        sync.Mutex
	presence  map[string]*Pres                // uid -> last position
	cellIndex map[string]map[string]bool      // cell -> set of uids (neighbourhood lookup)
	clients   map[string]*Client              // uid -> connected client (one per uid)
	cellSubs  map[string]map[*Client]bool     // cell -> clients listening to that cell
	dirty     map[*Client]bool                // clients owed a fresh nearby list on the next flush
	gone      map[string]time.Time            // uid -> when they hid/logged out: HTTP fixes taken before that are dropped
	heard     map[string]map[string]time.Time // uid -> uid -> last time their phones heard each other over Bluetooth (both directions)
	onDeck    map[string]map[string]bool      // viewer uid -> uids on the last list sent (they get the keep limits)
	ble       bleState
	lastSeen  map[string]time.Time // uid -> last fix or sighting, for waking phones that went quiet
	wokenAt   map[string]time.Time // uid -> last wake push
	wake      func(uids []string)  // sends the wake push (set by App; nil in tests)
	unsaved   map[string]*Pres     // positions not yet written to Postgres (latest per user)
	saveMu    sync.Mutex           // orders batch saves against deletes, so a save in flight can't bring back someone who hid

	store     *Store
	prof      *profileCache
	photoBase string // this server's public origin: absolute photo URLs work in every app build

	lastRetention time.Time // touched only by the sweeper goroutine
	stat          proxStats
}

// proxStats are per-minute proximity counters for the logs. Counts only: no
// coordinates, no uids.
type proxStats struct {
	fixes, vague, late          atomic.Int64 // fixes received; accuracy unknown or > maxAccM; already stale on arrival
	shown, far, stale, accuracy atomic.Int64 // pair decisions made by candidates, by outcome
	bleShown                    atomic.Int64 // pairs shown because their phones heard each other (GPS alone said no)
	bleTokens, sightings, wakes atomic.Int64 // tokens minted; Bluetooth pairs reported; wake pushes sent
	flushMaxMS                  atomic.Int64 // slowest flush of all dirty decks
}

func (s *proxStats) count(why string) {
	switch why {
	case "":
		s.shown.Add(1)
	case "far":
		s.far.Add(1)
	case "stale":
		s.stale.Add(1)
	case "accuracy":
		s.accuracy.Add(1)
	}
}

func (h *Hub) countFix(p *Pres) {
	h.stat.fixes.Add(1)
	if p.Acc <= 0 || p.Acc > maxAccM {
		h.stat.vague.Add(1)
	}
	if time.Since(p.T) > freshFor {
		h.stat.late.Add(1)
	}
}

func newHub(store *Store, seed map[string]*Pres) *Hub {
	h := &Hub{
		presence:  map[string]*Pres{},
		cellIndex: map[string]map[string]bool{},
		clients:   map[string]*Client{},
		cellSubs:  map[string]map[*Client]bool{},
		dirty:     map[*Client]bool{},
		gone:      map[string]time.Time{},
		heard:     map[string]map[string]time.Time{},
		onDeck:    map[string]map[string]bool{},
		unsaved:   map[string]*Pres{},
		ble:       newBLEState(),
		lastSeen:  map[string]time.Time{},
		wokenAt:   map[string]time.Time{},
		store:     store,
		prof:      newProfileCache(store),
	}
	for uid, p := range seed {
		h.presence[uid] = p
		h.indexAdd(p.Cell, uid)
	}
	return h
}

func (h *Hub) indexAdd(cell, uid string) {
	if h.cellIndex[cell] == nil {
		h.cellIndex[cell] = map[string]bool{}
	}
	h.cellIndex[cell][uid] = true
}

func (h *Hub) indexRemove(cell, uid string) {
	if set := h.cellIndex[cell]; set != nil {
		delete(set, uid)
		if len(set) == 0 {
			delete(h.cellIndex, cell)
		}
	}
}

// setPresence updates the in-memory position and returns the cells that need
// re-evaluation (the old cell, if the person moved, plus the new one), or nil
// if p is older than the fix already held.
func (h *Hub) setPresence(p *Pres) []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	if g, ok := h.gone[p.UID]; ok {
		if !p.T.After(g) {
			return nil // taken before they hid: a request still in flight must not bring them back
		}
		delete(h.gone, p.UID)
	}
	old := h.presence[p.UID]
	if old != nil && p.T.Before(old.T) {
		return nil
	}
	touched := map[string]bool{p.Cell: true}
	if old != nil && old.Cell != p.Cell {
		h.indexRemove(old.Cell, p.UID)
		touched[old.Cell] = true
	}
	h.presence[p.UID] = p
	h.indexAdd(p.Cell, p.UID)
	h.lastSeen[p.UID] = p.T
	h.unsaved[p.UID] = p
	return keys(touched)
}

// clearGone lifts the hide guard: the fix that follows came after the hide.
func (h *Hub) clearGone(uid string) {
	h.mu.Lock()
	delete(h.gone, uid)
	h.mu.Unlock()
}

// dropPresence takes someone off the map on purpose (hide, logout, account deletion).
func (h *Hub) dropPresence(uid string) []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.gone[uid] = time.Now()
	delete(h.lastSeen, uid)
	h.revokeBLE(uid)
	for other := range h.heard[uid] { // their Bluetooth matches go too
		delete(h.heard[other], uid)
		h.markDirty(other)
	}
	delete(h.heard, uid)
	delete(h.onDeck, uid)
	delete(h.unsaved, uid)
	old := h.presence[uid]
	if old == nil {
		return nil
	}
	h.indexRemove(old.Cell, uid)
	delete(h.presence, uid)
	return []string{old.Cell}
}

// subscribe sets a client's position (nil = stop sharing) and points it at the
// cells around it, updating the reverse index used to find who to notify.
func (h *Hub) subscribe(c *Client, p *Pres) {
	want := map[string]bool{}
	if p != nil {
		for _, cell := range cellsAround(p.Lat, p.Lng) {
			want[cell] = true
		}
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if p != nil && c.pos != nil && p.T.Before(c.pos.T) {
		return // a newer fix already re-pointed this client (attach raced applyPos)
	}
	c.pos = p
	for cell := range c.cells { // drop stale subscriptions
		if !want[cell] {
			if set := h.cellSubs[cell]; set != nil {
				delete(set, c)
				if len(set) == 0 {
					delete(h.cellSubs, cell)
				}
			}
		}
	}
	for cell := range want { // add new ones
		if h.cellSubs[cell] == nil {
			h.cellSubs[cell] = map[*Client]bool{}
		}
		h.cellSubs[cell][c] = true
	}
	c.cells = want
}

// attach registers a new connection as the user's only one and queues their
// nearby list straight away from the position we already hold, so a phone
// reconnecting after screen-off sees who is around without sending a fix first.
// The older connection is replaced by closing its socket (its pumps then exit
// and unsubscribe). Never close its send channel: other goroutines may still be
// sending to it, and a send on a closed channel panics.
func (h *Hub) attach(c *Client) {
	h.mu.Lock()
	if old := h.clients[c.uid]; old != nil && old.conn != nil {
		old.conn.Close()
	}
	h.clients[c.uid] = c
	p := h.presence[c.uid]
	known := h.knows(c.uid, time.Now())
	h.mu.Unlock()
	h.subscribe(c, p)
	if known { // now, not on the next flush: the app is waiting on its scan screen
		h.recompute(c)
	}
	// otherwise say nothing yet: an empty list would only mean "we don't know where you
	// are", and the app would blank a deck that is about to be refilled. The user's next
	// fix (applyPos) or Bluetooth sighting sends the list.
}

// knows reports whether we can place this user right now: a fresh fix or a fresh
// Bluetooth pair. Caller holds h.mu.
func (h *Hub) knows(uid string, now time.Time) bool {
	if p := h.presence[uid]; p != nil && now.Sub(p.T) <= freshFor {
		return true
	}
	for _, t := range h.heard[uid] {
		if now.Sub(t) <= freshFor {
			return true
		}
	}
	return false
}

// position returns a user's current presence, or nil.
func (h *Hub) position(uid string) *Pres {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.presence[uid]
}

// client returns a user's live connection, or nil.
func (h *Hub) client(uid string) *Client {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.clients[uid]
}

// stats reports live counts for the admin overview telemetry.
func (h *Hub) stats() (connectedClients, livePresence int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.clients), len(h.presence)
}

// pushNotification delivers a payload to a user's live connection, if any.
// (If they're offline it just doesn't send; the row is still in their feed.)
func (h *Hub) pushNotification(uid string, payload any) {
	h.mu.Lock()
	c := h.clients[uid]
	h.mu.Unlock()
	if c != nil {
		c.sendJSON(payload)
	}
}

func (h *Hub) unsubscribe(c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for cell := range c.cells {
		if set := h.cellSubs[cell]; set != nil {
			delete(set, c)
			if len(set) == 0 {
				delete(h.cellSubs, cell)
			}
		}
	}
	if h.clients[c.uid] == c {
		delete(h.clients, c.uid)
	}
	delete(h.dirty, c)
}

// candidates snapshots the uids+distance eligible for a client's deck right now:
// close by GPS (eligible, searched through the grid) or heard over Bluetooth
// within freshFor. Runs under the lock; does no I/O.
func (h *Hub) candidates(c *Client, now time.Time) []scored {
	h.mu.Lock()
	defer h.mu.Unlock()
	heard, kept := h.heard[c.uid], h.onDeck[c.uid]
	near := func(uid string) bool { t, ok := heard[uid]; return ok && now.Sub(t) <= freshFor }
	seen := map[string]bool{}
	var out []scored
	if c.pos != nil {
		for cell := range c.cells {
			for uid := range h.cellIndex[cell] {
				if uid == c.uid || seen[uid] {
					continue
				}
				seen[uid] = true
				p := h.presence[uid]
				if p == nil {
					continue
				}
				check := eligible
				if kept[uid] {
					check = stillEligible
				}
				d, why := check(c.pos, p, now)
				h.stat.count(why)
				if why == "" {
					out = append(out, scored{uid: uid, d: d})
				} else if near(uid) && !farApart(c.pos, p, now) {
					h.stat.bleShown.Add(1)
					out = append(out, scored{uid: uid, d: radiusM})
				}
			}
		}
	}
	for uid := range heard { // heard but not in the GPS neighbourhood (no fix, or a vague one)
		if !seen[uid] && near(uid) && !farApart(c.pos, h.presence[uid], now) {
			h.stat.bleShown.Add(1)
			out = append(out, scored{uid: uid, d: radiusM})
		}
	}
	return out
}

type scored struct {
	uid string
	d   float64
}

// recompute builds and pushes a client's nearby list, closest first. Profile
// lookups happen outside the lock (they may hit Postgres).
func (h *Hub) recompute(c *Client) {
	cands := h.candidates(c, time.Now())
	sort.Slice(cands, func(i, j int) bool { return cands[i].d < cands[j].d })
	uids := make([]string, 0, min(len(cands), 2*maxDeck))
	for _, s := range cands[:min(len(cands), 2*maxDeck)] {
		uids = append(uids, s.uid)
	}
	h.prof.warm(uids)
	people := make([]nearbyPerson, 0, min(len(cands), maxDeck))
	shown := make(map[string]bool, len(cands))
	for _, s := range cands {
		if len(people) == maxDeck {
			break
		}
		p, ok := h.prof.get(s.uid)
		if !ok || !p.complete() {
			continue
		}
		shown[s.uid] = true
		people = append(people, nearbyPerson{
			UID: s.uid, Name: p.Name, Role: p.Role, Desc: p.Look,
			Img: h.photoBase + "/api/photo/" + s.uid + "?v=" + strconv.FormatInt(p.UpdatedAt.Unix(), 10),
		})
	}
	h.mu.Lock()
	if h.clients[c.uid] == c || h.clients[c.uid] == nil {
		h.onDeck[c.uid] = shown
	}
	h.mu.Unlock()
	c.sendJSON(map[string]any{"type": "nearby", "people": people})
}

// notify marks the given client and everyone watching the touched cells as owed
// a fresh nearby list. flush sends them, so a crowded venue costs one push per
// watcher per tick instead of one per watcher per position update (was O(N^2)).
func (h *Hub) notify(origin *Client, touched []string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if origin != nil {
		h.dirty[origin] = true
	}
	for _, cell := range touched {
		for c := range h.cellSubs[cell] {
			h.dirty[c] = true
		}
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

// expire drops presence that has gone stale and pushes the change to anyone who
// could see it, so a person who went silent leaves decks within freshFor plus
// one tick, not whenever someone else happens to move.
func (h *Hub) expire() int {
	cutoff := time.Now().Add(-freshFor)
	h.mu.Lock()
	var touched []string
	for uid, p := range h.presence { // check and drop under one lock: a fix landing in between must survive
		if p.T.Before(cutoff) {
			h.indexRemove(p.Cell, uid)
			delete(h.presence, uid)
			touched = append(touched, p.Cell)
		}
	}
	for uid, t := range h.gone { // past freshFor, an old fix can't make anyone visible anyway
		if t.Before(cutoff) {
			delete(h.gone, uid)
		}
	}
	for a, peers := range h.heard { // Bluetooth pairs that went quiet
		for b, t := range peers {
			if t.Before(cutoff) {
				delete(peers, b)
				h.markDirty(a)
			}
		}
		if len(peers) == 0 {
			delete(h.heard, a)
		}
	}
	h.mu.Unlock()
	h.notify(nil, touched)
	return len(touched)
}

// sweep is the once-a-minute housekeeping: purge stale rows, trim the profile
// cache, log proximity counters, and run daily retention.
func (h *Hub) sweep(expired int) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// Don't keep precise locations nobody can see any more.
	if err := h.store.deleteStalePresence(ctx, time.Now().Add(-freshFor)); err != nil {
		log.Printf("stale presence purge: %v", err)
	}
	h.prof.purge()
	h.store.purgeSessions()
	if quiet := h.quiet(time.Now()); len(quiet) > 0 && h.wake != nil {
		h.stat.wakes.Add(int64(len(quiet)))
		go h.wake(quiet)
	}
	conn, live := h.stats()
	s := &h.stat
	slog.Info("proximity", "clients", conn, "live", live, "expired", expired,
		"fixes", s.fixes.Swap(0), "vague_fixes", s.vague.Swap(0), "late_fixes", s.late.Swap(0),
		"pairs_shown", s.shown.Swap(0), "pairs_far", s.far.Swap(0), "pairs_stale", s.stale.Swap(0),
		"pairs_accuracy", s.accuracy.Swap(0), "pairs_ble", s.bleShown.Swap(0),
		"ble_tokens", s.bleTokens.Swap(0), "ble_sightings", s.sightings.Swap(0), "wakes", s.wakes.Swap(0),
		"flush_max_ms", s.flushMaxMS.Swap(0))
	if time.Since(h.lastRetention) > 24*time.Hour {
		h.lastRetention = time.Now()
		if err := h.store.applyRetention(ctx); err != nil {
			log.Printf("retention: %v", err)
		}
	}
}

// Wake pushes: a phone that was reporting and went silent was probably stopped by
// its maker's battery manager. A data push may restart its service (Android lets
// a high-priority push start one). One push per wakeGap, only for wakeWindow.
const (
	wakeAfter  = 3 * time.Minute
	wakeWindow = 10 * time.Minute
	wakeGap    = 10 * time.Minute
)

// quiet returns the users owed a wake push now and records that they got one.
func (h *Hub) quiet(now time.Time) []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.purgeBLE(now)
	var out []string
	for uid, t := range h.lastSeen {
		switch age := now.Sub(t); {
		case age > wakeWindow:
			delete(h.lastSeen, uid)
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
	return out
}

func (h *Hub) run(ctx context.Context) {
	flush, expire, sweep := time.NewTicker(flushEvery), time.NewTicker(10*time.Second), time.NewTicker(time.Minute)
	save := time.NewTicker(saveEvery)
	defer save.Stop()
	defer flush.Stop()
	defer expire.Stop()
	defer sweep.Stop()
	expired := 0
	for {
		select {
		case <-ctx.Done():
			sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			h.save(sctx) // shutting down: keep the last positions for the next start
			cancel()
			return
		case <-save.C:
			sctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			h.save(sctx)
			cancel()
		case <-flush.C:
			start := time.Now()
			h.flush()
			if ms := time.Since(start).Milliseconds(); ms > h.stat.flushMaxMS.Load() {
				h.stat.flushMaxMS.Store(ms)
			}
		case <-expire.C:
			expired += h.expire()
		case <-sweep.C:
			h.sweep(expired)
			expired = 0
		}
	}
}

// Positions are written to Postgres behind the live copy (write-behind): the hub
// matches from memory, and the table only lets a restarted server pick up the
// last 2 minutes. One batched statement every saveEvery replaces one write per
// fix (phones report every 5-30 s each) and keeps the database off the path
// between a fix and the decks it changes.
func (h *Hub) save(ctx context.Context) {
	if h.store == nil {
		return
	}
	h.saveMu.Lock()
	defer h.saveMu.Unlock()
	h.mu.Lock()
	batch := make([]Pres, 0, len(h.unsaved))
	for _, p := range h.unsaved {
		batch = append(batch, *p)
	}
	h.unsaved = map[string]*Pres{}
	h.mu.Unlock()
	if len(batch) == 0 {
		return
	}
	if err := h.store.upsertPresences(ctx, batch); err != nil {
		log.Printf("presence save failed (%d rows, retrying next round): %v", len(batch), err)
		h.mu.Lock()
		for i := range batch {
			p := &batch[i]
			if q, ok := h.unsaved[p.UID]; (!ok || q.T.Before(p.T)) && h.presence[p.UID] != nil {
				h.unsaved[p.UID] = p // not superseded, not hidden meanwhile
			}
		}
		h.mu.Unlock()
	}
}

// forget deletes a user's saved position (hide, logout). It waits for a save in
// flight, so that save can't write the row back after the delete.
func (h *Hub) forget(ctx context.Context, uid string) error {
	h.saveMu.Lock()
	defer h.saveMu.Unlock()
	h.mu.Lock()
	delete(h.unsaved, uid)
	h.mu.Unlock()
	if h.store == nil {
		return nil
	}
	return h.store.deletePresence(ctx, uid)
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// profileCache keeps recently-seen profiles in memory so matching doesn't hit
// Postgres for every recompute. Mirrors the 5-minute client cache.
type profileCache struct {
	mu    sync.Mutex
	store *Store
	items map[string]profEntry
}

type profEntry struct {
	prof Profile
	ok   bool
	at   time.Time
}

func newProfileCache(store *Store) *profileCache {
	return &profileCache{store: store, items: map[string]profEntry{}}
}

func (pc *profileCache) get(uid string) (Profile, bool) {
	pc.mu.Lock()
	e, found := pc.items[uid]
	pc.mu.Unlock()
	if found && time.Since(e.at) < profileTTL {
		return e.prof, e.ok
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	p, ok, err := pc.store.getProfile(ctx, uid)
	if err != nil {
		// On a transient DB error, serve the stale copy if we have one.
		if found {
			return e.prof, e.ok
		}
		return Profile{}, false
	}
	pc.mu.Lock()
	pc.items[uid] = profEntry{prof: p, ok: ok, at: time.Now()}
	pc.mu.Unlock()
	return p, ok
}

// invalidate forces the next lookup to re-read from Postgres (after a save).
// warm loads every uncached profile among uids in one query, so a crowd's first
// list doesn't wait on one database round trip per person (50 people arriving at
// once took seconds).
func (pc *profileCache) warm(uids []string) {
	if pc.store == nil {
		return
	}
	pc.mu.Lock()
	var miss []string
	for _, u := range uids {
		if e, ok := pc.items[u]; !ok || time.Since(e.at) >= profileTTL {
			miss = append(miss, u)
		}
	}
	pc.mu.Unlock()
	if len(miss) < 2 { // one is no faster in a batch
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	got, err := pc.store.getProfiles(ctx, miss)
	if err != nil {
		return // get falls back to one at a time
	}
	now := time.Now()
	pc.mu.Lock()
	for _, u := range miss {
		p, ok := got[u]
		pc.items[u] = profEntry{prof: p, ok: ok, at: now}
	}
	pc.mu.Unlock()
}

func (pc *profileCache) invalidate(uid string) {
	pc.mu.Lock()
	delete(pc.items, uid)
	pc.mu.Unlock()
}

// purge drops expired entries. Without it the cache held every profile ever
// seen (photos included) until restart: ~4 GB at 50k users.
func (pc *profileCache) purge() {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	for uid, e := range pc.items {
		if time.Since(e.at) >= profileTTL {
			delete(pc.items, uid)
		}
	}
}
