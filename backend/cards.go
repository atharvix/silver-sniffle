package main

import (
	"context"
	"sync"
	"time"
)

// profileCache keeps recently-seen cards (profiles without the photo) in memory so
// matching doesn't hit Postgres for every recompute. Mirrors the 5-minute client cache.
type profileCache struct {
	mu    sync.Mutex
	store *Store
	items map[string]profEntry
	gen   uint64 // bumped by invalidate: a lookup it overtook must not store the card read before the save
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
	gen := pc.gen
	pc.mu.Unlock()
	if found && time.Since(e.at) < profileTTL {
		return e.prof, e.ok
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	got, err := pc.store.getCards(ctx, []string{uid})
	p, ok := got[uid]
	if err != nil {
		// On a transient DB error, serve the stale copy if we have one.
		if found {
			return e.prof, e.ok
		}
		return Profile{}, false
	}
	pc.mu.Lock()
	if pc.gen == gen {
		pc.items[uid] = profEntry{prof: p, ok: ok, at: time.Now()}
	}
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
	gen := pc.gen
	pc.mu.Unlock()
	if len(miss) < 2 { // one is no faster in a batch
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	got, err := pc.store.getCards(ctx, miss)
	if err != nil {
		return // get falls back to one at a time
	}
	now := time.Now()
	pc.mu.Lock()
	defer pc.mu.Unlock()
	if pc.gen != gen {
		return // a card was saved meanwhile: get re-reads one at a time
	}
	for _, u := range miss {
		p, ok := got[u]
		pc.items[u] = profEntry{prof: p, ok: ok, at: now}
	}
}

func (pc *profileCache) invalidate(uid string) {
	pc.mu.Lock()
	delete(pc.items, uid)
	pc.gen++
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
