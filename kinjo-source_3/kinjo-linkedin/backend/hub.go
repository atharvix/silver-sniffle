package main

import (
	"context"
	"math"
	"sync"
	"time"
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

// nearbyPerson is the card the client renders. Field names match what the deck
// UI already expects (uid, name, role, desc, img, d).
type nearbyPerson struct {
	UID  string `json:"uid"`
	Name string `json:"name"`
	Role string `json:"role"`
	Desc string `json:"desc"`
	Img  string `json:"img"`
	D    int    `json:"-"` // ordering only; exact distance never leaves the server
}

// Hub owns all realtime state: who is where, and who is listening.
type Hub struct {
	mu        sync.Mutex
	presence  map[string]*Pres            // uid -> last position
	cellIndex map[string]map[string]bool  // cell -> set of uids (neighbourhood lookup)
	clients   map[string]*Client          // uid -> connected client (one per uid)
	cellSubs  map[string]map[*Client]bool // cell -> clients listening to that cell

	store *Store
	prof  *profileCache
}

func newHub(store *Store, seed map[string]*Pres) *Hub {
	h := &Hub{
		presence:  map[string]*Pres{},
		cellIndex: map[string]map[string]bool{},
		clients:   map[string]*Client{},
		cellSubs:  map[string]map[*Client]bool{},
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
// re-evaluation (the old cell, if the person moved, plus the new one).
func (h *Hub) setPresence(p *Pres) []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	touched := map[string]bool{p.Cell: true}
	if old := h.presence[p.UID]; old != nil && old.Cell != p.Cell {
		h.indexRemove(old.Cell, p.UID)
		touched[old.Cell] = true
	}
	h.presence[p.UID] = p
	h.indexAdd(p.Cell, p.UID)
	return keys(touched)
}

// dropPresence removes someone from the map entirely (hidden or gone stale).
func (h *Hub) dropPresence(uid string) []string {
	h.mu.Lock()
	defer h.mu.Unlock()
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
}

// candidates snapshots the uids+distance visible to a client, applying the same
// distance/staleness/hysteresis test as the old client-side matcher. Runs under
// the lock; does no I/O.
func (h *Hub) candidates(c *Client, now time.Time) []scored {
	h.mu.Lock()
	defer h.mu.Unlock()
	if c.pos == nil {
		return nil
	}
	seen := map[string]bool{}
	var out []scored
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
			if now.Sub(p.T).Milliseconds() > staleMS {
				continue
			}
			d := distM(c.pos.Lat, c.pos.Lng, p.Lat, p.Lng)
			if inside(d, c.pos.Acc, p.Acc, c.shown[uid]) {
				out = append(out, scored{uid: uid, d: d})
			}
		}
	}
	// Refresh the hysteresis set to exactly who is inside now.
	c.shown = make(map[string]bool, len(out))
	for _, s := range out {
		c.shown[s.uid] = true
	}
	return out
}

type scored struct {
	uid string
	d   float64
}

// recompute builds and pushes a client's nearby list. Profile lookups happen
// outside the lock (they may hit Postgres).
func (h *Hub) recompute(c *Client) {
	cands := h.candidates(c, time.Now())
	people := make([]nearbyPerson, 0, len(cands))
	for _, s := range cands {
		p, ok := h.prof.get(s.uid)
		if !ok || !p.complete() {
			continue
		}
		d := int(math.Max(1, math.Round(math.Min(s.d, radiusM))))
		people = append(people, nearbyPerson{
			UID: s.uid, Name: p.Name, Role: p.Role, Desc: p.Look, Img: p.Photo, D: d,
		})
	}
	// Closest first, matching the deck's ordering.
	sortByDist(people)
	c.sendJSON(map[string]any{"type": "nearby", "people": people})
}

// notify re-runs matching for the given client and everyone listening to the
// touched cells, so arrivals/departures reach affected people immediately.
func (h *Hub) notify(origin *Client, touched []string) {
	targets := map[*Client]bool{}
	if origin != nil {
		targets[origin] = true
	}
	h.mu.Lock()
	for _, cell := range touched {
		for c := range h.cellSubs[cell] {
			targets[c] = true
		}
	}
	h.mu.Unlock()
	for c := range targets {
		h.recompute(c)
	}
}

// sweep drops presence older than staleMS and notifies anyone still watching.
func (h *Hub) sweep() {
	cutoff := time.Now().Add(-time.Duration(staleMS) * time.Millisecond)
	h.mu.Lock()
	var stale []*Pres
	for _, p := range h.presence {
		if p.T.Before(cutoff) {
			stale = append(stale, p)
		}
	}
	h.mu.Unlock()
	for _, p := range stale {
		touched := h.dropPresence(p.UID)
		h.notify(nil, touched)
	}
}

func (h *Hub) runSweeper(ctx context.Context) {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			h.sweep()
		}
	}
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// sortByDist is an insertion sort — lists are tiny (a single neighbourhood), so
// this beats pulling in sort's overhead and stays allocation-free.
func sortByDist(p []nearbyPerson) {
	for i := 1; i < len(p); i++ {
		for j := i; j > 0 && p[j].D < p[j-1].D; j-- {
			p[j], p[j-1] = p[j-1], p[j]
		}
	}
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
	if found && time.Since(e.at) < 5*time.Minute {
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
func (pc *profileCache) invalidate(uid string) {
	pc.mu.Lock()
	delete(pc.items, uid)
	pc.mu.Unlock()
}
