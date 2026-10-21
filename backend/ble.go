package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log"
	"net/http"
	"time"
)

// Bluetooth is the second proximity signal, next to GPS. Each phone advertises a
// short random token (never its uid) and reports the tokens it hears. The server
// maps tokens back to users and treats "heard each other in the last freshFor" as
// close, which works indoors where GPS is too vague. Tokens rotate so a passer-by
// can't follow one phone around by its advertisement.
const (
	bleTokenEvery = 15 * time.Minute   // a phone fetches a new token this often
	bleTokenGrace = 15 * time.Minute   // a replaced token still resolves this long (phones heard it a moment ago)
	bleTokenKeep  = 7 * 24 * time.Hour // a phone's latest token resolves until replaced, at most this long (a phone asleep keeps advertising it: expiring it only hides them)
	minRSSI       = -95                // dBm; phones in pockets or bags read -85 to -100 at 5-15 m (15-20 dB body loss)
	maxSightings  = 32                 // tokens per report
)

type bleTok struct {
	uid string
	exp time.Time
}

// bleState lives inside Hub and is guarded by Hub.mu.
type bleState struct {
	byTok  map[string]bleTok // token -> owner
	cur    map[string]string // uid -> current token
	prev   map[string]string // uid -> previous token (still valid until it expires)
	issued map[string]time.Time
}

func newBLEState() bleState {
	return bleState{byTok: map[string]bleTok{}, cur: map[string]string{}, prev: map[string]string{}, issued: map[string]time.Time{}}
}

// bleToken returns the user's current token, minting a new one every bleTokenEvery.
// The latest token stays valid until it's replaced (up to bleTokenKeep): a phone asleep
// in a pocket keeps advertising it but may not wake to fetch the next one, and it must
// stay recognisable to the open phones around it.
func (h *Hub) bleToken(uid string, now time.Time) (string, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if t, ok := h.ble.cur[uid]; ok && now.Sub(h.ble.issued[uid]) < bleTokenEvery {
		return t, nil
	}
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	t := hex.EncodeToString(b[:])
	if old, ok := h.ble.cur[uid]; ok {
		delete(h.ble.byTok, h.ble.prev[uid]) // the one before last is done
		h.ble.prev[uid] = old
		if e, ok := h.ble.byTok[old]; ok && now.Add(bleTokenGrace).Before(e.exp) {
			e.exp = now.Add(bleTokenGrace)
			h.ble.byTok[old] = e
		}
	}
	h.ble.byTok[t] = bleTok{uid: uid, exp: now.Add(bleTokenKeep)}
	h.ble.cur[uid], h.ble.issued[uid] = t, now
	h.stat.bleTokens.Add(1)
	return t, nil
}

// bleRow is a user's token state as stored, so tokens survive a restart: a sleeping
// phone keeps advertising the token it has, and must stay recognisable after a deploy.
type bleRow struct {
	cur, prev       string
	curExp, prevExp time.Time
}

func (h *Hub) bleSnapshot(uid string) (bleRow, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	cur, ok := h.ble.cur[uid]
	if !ok {
		return bleRow{}, false
	}
	r := bleRow{cur: cur, curExp: h.ble.byTok[cur].exp}
	if p, ok := h.ble.byTok[h.ble.prev[uid]]; ok {
		r.prev, r.prevExp = h.ble.prev[uid], p.exp
	}
	return r, true
}

// loadBLE restores tokens saved before a restart (rows ordered by exp: the latest is current).
func (h *Hub) loadBLE(rows []bleStored, now time.Time) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, r := range rows {
		if !now.Before(r.exp) {
			continue
		}
		if old, ok := h.ble.cur[r.uid]; ok {
			h.ble.prev[r.uid] = old
		}
		h.ble.byTok[r.tok] = bleTok{uid: r.uid, exp: r.exp}
		h.ble.cur[r.uid], h.ble.issued[r.uid] = r.tok, r.exp.Add(-bleTokenKeep)
	}
}

// bleOwner resolves a token heard over the air; "" if unknown or expired. Caller holds h.mu.
func (h *Hub) bleOwner(tok string, now time.Time) string {
	e, ok := h.ble.byTok[tok]
	if !ok || now.After(e.exp) {
		return ""
	}
	return e.uid
}

// revokeBLE forgets a user's tokens (hide, logout, delete). Caller holds h.mu.
func (h *Hub) revokeBLE(uid string) {
	delete(h.ble.byTok, h.ble.cur[uid])
	delete(h.ble.byTok, h.ble.prev[uid])
	delete(h.ble.cur, uid)
	delete(h.ble.prev, uid)
	delete(h.ble.issued, uid)
}

// purgeBLE drops expired tokens. Caller holds h.mu.
func (h *Hub) purgeBLE(now time.Time) {
	for t, e := range h.ble.byTok {
		if now.After(e.exp) {
			delete(h.ble.byTok, t)
		}
	}
	for uid, t := range h.ble.cur {
		if _, ok := h.ble.byTok[t]; !ok {
			delete(h.ble.cur, uid)
			delete(h.ble.prev, uid)
			delete(h.ble.issued, uid)
		}
	}
}

type sighting struct {
	Tok  string `json:"tok"`
	RSSI int    `json:"rssi"`
}

// addSightings records what uid's phone heard. The reporter must hold a live
// token itself: hiding revokes it, so a report still in flight from before a hide
// can't make anyone visible. Returns how many pairs were recorded.
func (h *Hub) addSightings(uid string, seen []sighting, now time.Time) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	if t, ok := h.ble.cur[uid]; !ok || h.bleOwner(t, now) != uid {
		return 0
	}
	n := 0
	for _, s := range seen {
		if s.RSSI >= 0 || s.RSSI < minRSSI {
			continue
		}
		other := h.bleOwner(s.Tok, now)
		if other == "" || other == uid {
			continue
		}
		fresh := now.Sub(h.heard[uid][other]) > freshFor // a new pair: decks change
		h.hear(uid, other, now)
		h.hear(other, uid, now)
		if fresh {
			h.markDirty(uid, other)
		}
		n++
	}
	h.stat.sightings.Add(int64(n))
	if n > 0 {
		h.lastSeen[uid] = now
		h.seen(uid, now)
	}
	return n
}

func (h *Hub) hear(a, b string, now time.Time) {
	if h.heard[a] == nil {
		h.heard[a] = map[string]time.Time{}
	}
	h.heard[a][b] = now
}

// markDirty queues fresh lists for these users' open apps. Caller holds h.mu.
func (h *Hub) markDirty(uids ...string) {
	for _, u := range uids {
		for c := range h.clients[u] {
			h.dirty[c] = true
		}
	}
}

// GET /api/ble-token -> {"token": 16 hex chars, "every": seconds until the phone should fetch again}
func (a *App) handleBLEToken(w http.ResponseWriter, r *http.Request, uid string) {
	t, err := a.hub.bleToken(uid, time.Now())
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, errBody("unavailable"))
		return
	}
	if row, ok := a.hub.bleSnapshot(uid); ok && row.cur == t { // a hide since the mint has revoked it: don't save
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		if err := a.store.saveBLE(ctx, uid, row); err != nil {
			log.Printf("ble token save: %v", err) // memory still works; only a restart forgets it
		}
		cancel()
	}
	writeJSON(w, http.StatusOK, map[string]any{"token": t, "every": int(bleTokenEvery / time.Second)})
}

// POST /api/sightings {"seen":[{"tok":"…","rssi":-70}]}
func (a *App) handleSightings(w http.ResponseWriter, r *http.Request, uid string) {
	var in struct{ Seen []sighting }
	if decodeJSON(r, &in) != nil || len(in.Seen) > maxSightings {
		writeJSON(w, http.StatusBadRequest, errBody("invalid body"))
		return
	}
	a.hub.addSightings(uid, in.Seen, time.Now())
	w.WriteHeader(http.StatusNoContent)
}
