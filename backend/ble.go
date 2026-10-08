package main

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"time"
)

// Bluetooth is the second proximity signal, next to GPS. Each phone advertises a
// short random token (never its uid) and reports the tokens it hears. The server
// maps tokens back to users and treats "heard each other in the last freshFor" as
// close, which works indoors where GPS is too vague. Tokens rotate so a passer-by
// can't follow one phone around by its advertisement.
const (
	bleTokenEvery = 15 * time.Minute // a phone fetches a new token this often
	bleTokenTTL   = 30 * time.Minute // a token still resolves for one rotation after it's replaced
	minRSSI       = -90              // dBm; weaker than this is too far (or through too many walls) to count
	maxSightings  = 32               // tokens per report
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
	}
	h.ble.byTok[t] = bleTok{uid: uid, exp: now.Add(bleTokenTTL)}
	h.ble.cur[uid], h.ble.issued[uid] = t, now
	h.stat.bleTokens.Add(1)
	return t, nil
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
		if c := h.clients[u]; c != nil {
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
