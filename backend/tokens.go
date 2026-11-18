package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log"
	"net/http"
	"time"
)

// Bluetooth tokens (docs/proximity.md). Each phone advertises a short random token
// (never its uid) and reports the tokens it hears; the server maps them back to users.
// Tokens rotate so a passer-by can't follow one phone around by its advertisement.
const (
	bleTokenEvery = 15 * time.Minute   // a phone fetches a new token this often
	bleTokenGrace = 15 * time.Minute   // a replaced token still resolves this long (phones heard it a moment ago)
	bleTokenKeep  = 7 * 24 * time.Hour // a phone's latest token resolves until replaced, at most this long (a phone asleep keeps advertising it: expiring it only hides them)
	// iosOverflowBit: a backgrounded iPhone can't advertise our service UUID where Android sees
	// it; iOS folds service UUIDs into Apple's manufacturer data (0x004C, 0x01 + a 128-bit
	// bitmap, one bit per UUID by an unpublished hash). The iPhone also advertises
	// iosOverflowUUID, whose bit is known (mapped empirically, davidgyoung/BackgroundAdvertiser),
	// so Android filters on that bit, connects and reads the token. Sent with every token, so
	// if Apple changes the hash this is fixed by a deploy, not an app update. -1 turns it off.
	iosOverflowBit = 101 // iosOverflowUUID 00000000-0000-0000-0000-000000000071; never 69 (Apple Watch setup prompt)
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
	if !h.hidden[uid] {
		h.lastSeen[uid] = now // alive: a Bluetooth-only phone with nobody around reports nothing else, and still needs its wake pushes
	}
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

// GET /api/ble-token -> {"token": 16 hex chars, "every": seconds until the phone should fetch again, "iosBit": iosOverflowBit}
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
	writeJSON(w, http.StatusOK, map[string]any{"token": t, "every": int(bleTokenEvery / time.Second), "iosBit": iosOverflowBit})
}

// needsToken: uid isn't hidden but holds no live token of its own.
func (h *Hub) needsToken(uid string, now time.Time) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	t, ok := h.ble.cur[uid]
	return !h.hidden[uid] && (!ok || h.bleOwner(t, now) != uid)
}
