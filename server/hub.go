package main

import (
	"encoding/json"
	"math"
	"slices"
	"strings"
	"sync"
	"time"
)

const (
	nearM     = 30.0                   // the product promise
	errCapM   = 40.0                   // most GPS error we forgive on top of it
	farM      = 45.0                   // a shown card drops only past this (+ error): no flicker at the edge
	maxAccM   = 100.0                  // fixes worse than this are ignored
	fixTTL    = 60 * time.Second       // a position nobody refreshed is forgotten
	stillTTL  = 10 * time.Minute       // a phone that says it isn't moving keeps its spot longer (it reports rarely)
	holdFor   = 60 * time.Second       // a card stays this long after the last time the pair was near
	minFixGap = 500 * time.Millisecond // faster reports are dropped

	tokenLife   = 30 * time.Minute // a Bluetooth token resolves to its owner this long
	tokenRotate = 15 * time.Minute // phones get a fresh one this often, so a token can't track anyone for long
	minRSSI     = -90              // weaker hearings are noise
	outdoorAccM = 15.0             // both fixes this good: GPS may overrule Bluetooth, which carries 80 m+ outdoors
	vetoM       = 35.0

	lastLife    = 12 * time.Hour   // how long a phone's coarse area is kept to decide whom to wake
	wakeGap     = 20 * time.Second // at most one wake per phone in this window
	wakeBudget  = 6                // and this many per wakeWindow
	wakeWindow  = 10 * time.Minute
	wakesPerFix = 50
)

// area is a ~550 m square: the only position kept beyond a minute, and only in memory, to know whom to wake.
type area struct{ lat, lng int32 }

func areaOf(lat, lng float64) area {
	return area{int32(math.Floor(lat * 200)), int32(math.Floor(lng * 200))}
}

type lastSeen struct {
	a  area
	at time.Time
}

// Card is what other people see. It never carries a position or a distance.
type Card struct {
	UID     string `json:"uid"`
	Name    string `json:"name"`
	Role    string `json:"role"`
	Looking string `json:"looking"`
	Img     string `json:"img"`
}

type fix struct {
	lat, lng, acc float64
	at            time.Time
	still         bool
}

func (f fix) fresh(now time.Time) bool {
	ttl := fixTTL
	if f.still {
		ttl = stillTTL
	}
	return now.Sub(f.at) <= ttl
}

type bleToken struct {
	uid string
	at  time.Time
}

// Sighting is one Bluetooth hearing of another phone's token.
type Sighting struct {
	Token string `json:"t"`
	RSSI  int    `json:"rssi"`
}

type client struct {
	uid  string
	send chan []byte
}

type pair [2]string

func pairOf(a, b string) pair {
	if a < b {
		return pair{a, b}
	}
	return pair{b, a}
}

// Hub holds live positions in memory only; nothing here is ever written to disk.
// shortcut: one process; positions move to Redis when a second server is added.
type Hub struct {
	mu      sync.Mutex
	fixes   map[string]fix
	cells   map[cell]map[string]struct{}
	near    map[pair]time.Time
	cards   map[string]Card
	clients map[string]map[*client]struct{}
	sent    map[string]string // uid -> signature of the last deck sent, so unchanged decks aren't resent
	hidden  map[string]bool
	tokens  map[string]bleToken // token -> owner
	tokenOf map[string]string   // uid -> current token
	last    map[string]lastSeen
	inArea  map[area]map[string]struct{}
	woken   map[string][]time.Time // recent wakes per uid, for the budget
	wake    func(uid string)       // nil when push isn't configured; must not block
}

func newHub() *Hub {
	return &Hub{
		fixes:   map[string]fix{},
		cells:   map[cell]map[string]struct{}{},
		near:    map[pair]time.Time{},
		cards:   map[string]Card{},
		clients: map[string]map[*client]struct{}{},
		sent:    map[string]string{},
		hidden:  map[string]bool{},
		tokens:  map[string]bleToken{},
		tokenOf: map[string]string{},
		last:    map[string]lastSeen{},
		inArea:  map[area]map[string]struct{}{},
		woken:   map[string][]time.Time{},
	}
}

func validFix(f fix) bool {
	return f.lat >= -89 && f.lat <= 89 && f.lng >= -180 && f.lng <= 180 && f.acc > 0 && f.acc <= maxAccM
}

// report records a position and updates who is near whom. It returns false if the fix was refused.
func (h *Hub) report(uid string, f fix) bool {
	if !validFix(f) {
		return false
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.hidden[uid] {
		return false
	}
	prev, had := h.fixes[uid]
	if had && f.at.Sub(prev.at) < minFixGap {
		return false
	}
	if had {
		h.leaveCell(uid, prev)
	}
	h.fixes[uid] = f
	c := cellOf(f.lat, f.lng)
	if h.cells[c] == nil {
		h.cells[c] = map[string]struct{}{}
	}
	h.cells[c][uid] = struct{}{}

	changed := []string{uid}
	for _, nc := range neighbourhood(f.lat, f.lng) {
		for other := range h.cells[nc] {
			o := h.fixes[other]
			if other == uid || !o.fresh(f.at) {
				continue
			}
			d := distanceM(f.lat, f.lng, o.lat, o.lng)
			slack := math.Min(f.acc+o.acc, errCapM)
			p := pairOf(uid, other)
			switch {
			case d <= nearM+slack:
				h.near[p] = f.at
				changed = append(changed, other)
			case d > farM+slack:
				if _, ok := h.near[p]; ok {
					delete(h.near, p)
					changed = append(changed, other)
				}
			}
		}
	}
	h.flush(changed, f.at)
	h.setLast(uid, areaOf(f.lat, f.lng), f.at)
	h.wakeAround(uid, f)
	return true
}

func (h *Hub) setLast(uid string, a area, at time.Time) {
	if prev, ok := h.last[uid]; ok && prev.a != a {
		h.leaveArea(uid, prev.a)
	}
	h.last[uid] = lastSeen{a, at}
	if h.inArea[a] == nil {
		h.inArea[a] = map[string]struct{}{}
	}
	h.inArea[a][uid] = struct{}{}
}

func (h *Hub) leaveArea(uid string, a area) {
	delete(h.inArea[a], uid)
	if len(h.inArea[a]) == 0 {
		delete(h.inArea, a)
	}
}

// wakeAround wakes phones last seen in or next to the reporter's area that have gone quiet,
// so a phone whose app was swiped away reports and can be matched. Caller holds mu.
// shortcut: wakes the whole ~1.6 km block around the fix (capped); a radius that grows with how
// fast the phone was moving (docs/ARCHITECTURE.md) would wake fewer phones.
func (h *Hub) wakeAround(uid string, f fix) {
	if h.wake == nil {
		return
	}
	center := areaOf(f.lat, f.lng)
	sent := 0
	for dy := int32(-1); dy <= 1; dy++ {
		for dx := int32(-1); dx <= 1; dx++ {
			for other := range h.inArea[area{center.lat + dy, center.lng + dx}] {
				if sent == wakesPerFix {
					return
				}
				if other == uid || h.hidden[other] || len(h.clients[other]) > 0 {
					continue // the reporter, a hidden user, or a phone already connected
				}
				if o, ok := h.fixes[other]; ok && o.fresh(f.at) {
					continue // already findable
				}
				if !h.mayWake(other, f.at) {
					continue
				}
				h.woken[other] = append(h.woken[other], f.at)
				h.wake(other)
				sent++
			}
		}
	}
}

func (h *Hub) mayWake(uid string, now time.Time) bool {
	recent := h.woken[uid][:0]
	for _, t := range h.woken[uid] {
		if now.Sub(t) < wakeWindow {
			recent = append(recent, t)
		}
	}
	h.woken[uid] = recent
	return len(recent) < wakeBudget && (len(recent) == 0 || now.Sub(recent[len(recent)-1]) >= wakeGap)
}

func (h *Hub) leaveCell(uid string, f fix) {
	c := cellOf(f.lat, f.lng)
	delete(h.cells[c], uid)
	if len(h.cells[c]) == 0 {
		delete(h.cells, c)
	}
}

// sweep forgets stale positions and expired pairs, then resends any deck that changed.
func (h *Hub) sweep(now time.Time) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for uid, f := range h.fixes {
		if !f.fresh(now) {
			h.leaveCell(uid, f)
			delete(h.fixes, uid)
		}
	}
	for p, at := range h.near {
		if now.Sub(at) > holdFor {
			delete(h.near, p)
		}
	}
	for t, owner := range h.tokens {
		if now.Sub(owner.at) > tokenLife {
			delete(h.tokens, t)
		}
	}
	for uid, l := range h.last {
		if now.Sub(l.at) > lastLife {
			h.leaveArea(uid, l.a)
			delete(h.last, uid)
			delete(h.woken, uid)
		}
	}
	for uid := range h.clients {
		if t, ok := h.tokens[h.tokenOf[uid]]; !ok || now.Sub(t.at) > tokenRotate {
			h.issueToken(uid, now)
		}
	}
	h.flush(mapKeys(h.clients), now)
}

// issueToken gives a user a new Bluetooth token and sends it to their phones. Caller holds mu.
// The token is random, so a stranger who records it learns nothing and can't follow it past tokenLife.
func (h *Hub) issueToken(uid string, now time.Time) {
	if h.hidden[uid] {
		return
	}
	t := randomHex(8)
	h.tokens[t] = bleToken{uid, now}
	h.tokenOf[uid] = t
	h.sendAll(uid, map[string]any{"type": "ble", "token": t})
}

// sighted records the tokens a phone heard over Bluetooth: a direct hearing means the pair is close.
func (h *Hub) sighted(uid string, heard []Sighting, now time.Time) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.hidden[uid] {
		return
	}
	changed := []string{uid}
	for _, s := range heard {
		owner, ok := h.tokens[s.Token]
		if !ok || owner.uid == uid || s.RSSI < minRSSI || h.gpsVetoes(uid, owner.uid, now) {
			continue
		}
		h.near[pairOf(uid, owner.uid)] = now
		changed = append(changed, owner.uid)
	}
	h.flush(changed, now)
}

// gpsVetoes: outdoors, two good fixes clearly apart overrule Bluetooth. Indoor (poor) fixes never do.
func (h *Hub) gpsVetoes(a, b string, now time.Time) bool {
	fa, okA := h.fixes[a]
	fb, okB := h.fixes[b]
	if !okA || !okB || !fa.fresh(now) || !fb.fresh(now) || fa.acc > outdoorAccM || fb.acc > outdoorAccM {
		return false
	}
	return distanceM(fa.lat, fa.lng, fb.lat, fb.lng) > vetoM+fa.acc+fb.acc
}

// setHidden hides or shows a user everywhere at once: no card, no position, no token, empty deck.
func (h *Hub) setHidden(uid string, hidden bool, card *Card) {
	h.mu.Lock()
	defer h.mu.Unlock()
	now := time.Now()
	h.hidden[uid] = hidden
	if hidden {
		h.forget(uid)
	} else {
		if card != nil {
			h.cards[uid] = *card
		}
		if len(h.clients[uid]) > 0 {
			h.issueToken(uid, now)
		}
	}
	h.sendAll(uid, map[string]any{"type": "vis", "visible": !hidden})
	h.flush(mapKeys(h.clients), now)
}

// remove drops everything the hub knows about a deleted account. Sockets still open for it are
// treated as hidden (they see nobody and are refused) until they drop; their session is gone, so they can't return.
func (h *Hub) remove(uid string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.forget(uid)
	h.hidden[uid] = true
	h.sendAll(uid, map[string]any{"type": "vis", "visible": false})
	h.flush(mapKeys(h.clients), time.Now())
}

// forget drops a user's card, position, pairs and token. Caller holds mu.
func (h *Hub) forget(uid string) {
	delete(h.cards, uid)
	if f, ok := h.fixes[uid]; ok {
		h.leaveCell(uid, f)
		delete(h.fixes, uid)
	}
	for p := range h.near {
		if p[0] == uid || p[1] == uid {
			delete(h.near, p)
		}
	}
	delete(h.tokens, h.tokenOf[uid])
	delete(h.tokenOf, uid)
	if l, ok := h.last[uid]; ok {
		h.leaveArea(uid, l.a)
		delete(h.last, uid)
	}
	delete(h.woken, uid)
}

// sendAll sends one message to every socket of a user. Caller holds mu.
func (h *Hub) sendAll(uid string, v any) {
	msg, _ := json.Marshal(v)
	for c := range h.clients[uid] {
		select {
		case c.send <- msg:
		default:
		}
	}
}

// deck lists the cards a user should see now, nearest first.
func (h *Hub) deck(uid string, now time.Time) []Card {
	me, located := h.fixes[uid]
	var out []Card
	if h.hidden[uid] {
		return out
	}
	for p, at := range h.near {
		other := p[0]
		if other == uid {
			other = p[1]
		} else if p[1] != uid {
			continue
		}
		if card, ok := h.cards[other]; ok && now.Sub(at) <= holdFor {
			out = append(out, card)
		}
	}
	dist := func(c Card) float64 {
		o, ok := h.fixes[c.UID]
		if !ok || !located {
			return math.MaxFloat64
		}
		return distanceM(me.lat, me.lng, o.lat, o.lng)
	}
	slices.SortFunc(out, func(a, b Card) int {
		if da, db := dist(a), dist(b); da != db {
			if da < db {
				return -1
			}
			return 1
		}
		return strings.Compare(a.UID, b.UID)
	})
	return out
}

// flush sends each listed user their deck if it differs from the last one sent. Caller holds mu.
func (h *Hub) flush(uids []string, now time.Time) {
	for _, uid := range uids {
		conns := h.clients[uid]
		if len(conns) == 0 {
			continue
		}
		cards := h.deck(uid, now)
		sig := signature(cards)
		if h.sent[uid] == sig {
			continue
		}
		msg, _ := json.Marshal(map[string]any{"type": "deck", "cards": orEmpty(cards)})
		h.sent[uid] = sig
		for c := range conns {
			select {
			case c.send <- msg:
			default:
				delete(h.sent, uid) // a slow socket missed it; the next sweep resends
			}
		}
	}
}

// attach registers a socket. card is nil when the user has no card yet or is hidden.
func (h *Hub) attach(c *client, card *Card, hidden bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	now := time.Now()
	if h.clients[c.uid] == nil {
		h.clients[c.uid] = map[*client]struct{}{}
	}
	h.clients[c.uid][c] = struct{}{}
	h.hidden[c.uid] = hidden
	if card != nil && !hidden {
		h.cards[c.uid] = *card
	}
	delete(h.sent, c.uid) // a new socket always gets the current deck and token
	if t, ok := h.tokens[h.tokenOf[c.uid]]; ok && now.Sub(t.at) <= tokenRotate {
		h.sendAll(c.uid, map[string]any{"type": "ble", "token": h.tokenOf[c.uid]})
	} else {
		h.issueToken(c.uid, now)
	}
	h.flush([]string{c.uid}, now)
}

func (h *Hub) detach(c *client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.clients[c.uid], c)
	if len(h.clients[c.uid]) == 0 {
		delete(h.clients, c.uid)
		delete(h.sent, c.uid)
		delete(h.hidden, c.uid) // reloaded from the database on the next attach
	}
}

// setCard publishes a new or edited card to everyone who can see it.
func (h *Hub) setCard(card Card) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.hidden[card.UID] {
		return // shown again by setHidden(false), which loads the card
	}
	h.cards[card.UID] = card
	h.flush(mapKeys(h.clients), time.Now())
}

func signature(cards []Card) string {
	var b strings.Builder
	for _, c := range cards {
		b.WriteString(c.UID)
		b.WriteString(c.Img)
		b.WriteByte(';')
	}
	return b.String()
}

func orEmpty(cards []Card) []Card {
	if cards == nil {
		return []Card{}
	}
	return cards
}

func mapKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
