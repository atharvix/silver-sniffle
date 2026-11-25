package main

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	nearM     = 30.0                   // the product promise
	errCapM   = 40.0                   // most GPS error we forgive on top of it
	farM      = 35.0                   // a shown card drops past this (+ error): a 5 m band so the edge doesn't flicker
	maxAccM   = 100.0                  // fixes worse than this are ignored
	fixTTL    = 60 * time.Second       // a position nobody refreshed is forgotten
	stillTTL  = 2 * time.Minute        // a resting phone reports every 20 s; while connected, pings keep it placed
	holdFor   = 30 * time.Second       // a card stays this long after the last time the pair was near
	leaveAt   = 20 * time.Second       // no connection this long: off the map (a quick reconnect keeps the card up)
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
	wakeScanMax = 500 // candidates looked at per fix
	maxDeck     = 100 // cards in one deck, nearest first

	// Crowd limits: a stadium costs each report the same few Redis commands as a café.
	maxNear      = 2 * maxDeck            // pairs one report keeps fresh, nearest first
	refreshNear  = holdFor / 3            // an existing pair is re-saved this often, not on every report
	squareLife   = 500 * time.Millisecond // how old a copied map square may be (another server's arrivals show within it)
	flushWorkers = 16                     // decks rebuilt side by side when a crowd arrives (well under the Redis pool)

	onlineTTL = 90 * time.Second // a server refreshes this for its sockets every sweep
	channel   = "k:out"          // pub/sub between servers
)

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

func validFix(f fix) bool {
	return f.lat >= -89 && f.lat <= 89 && f.lng >= -180 && f.lng <= 180 && f.acc > 0 && f.acc <= maxAccM
}

// Sighting is one Bluetooth hearing of another phone's token.
type Sighting struct {
	Token string `json:"t"`
	RSSI  int    `json:"rssi"`
}

// area is a ~550 m square: the only position kept beyond minutes, to know whom to wake.
type area struct{ lat, lng int32 }

func areaOf(lat, lng float64) area {
	return area{int32(math.Floor(lat * 200)), int32(math.Floor(lng * 200))}
}

type client struct {
	uid  string
	send chan []byte
}

// Redis keys. Live state only, every key expires or is removed with its owner; nothing here is ever written to disk
// (run Redis without persistence). Each command touches one key, so Redis Cluster works unchanged.
func kFix(u string) string     { return "k:pos:" + u }                            // packed fix (was k:fix, a hash)
func kCell(c cell) string      { return fmt.Sprintf("k:sq:%d:%d", c.lat, c.lng) } // hash uid -> packed fix (was k:cell, a zset)
func kNear(u string) string    { return "k:near:" + u }                           // zset other uid -> last time near
func kCard(u string) string    { return "k:card:" + u }
func kHidden(u string) string  { return "k:hidden:" + u }
func kTok(t string) string     { return "k:tok:" + t }                              // Bluetooth token -> owner
func kTokOf(u string) string   { return "k:tokof:" + u }                            // owner -> current token
func kLast(u string) string    { return "k:last:" + u }                             // "lat:lng" of the last area
func kArea(a area) string      { return fmt.Sprintf("k:area:%d:%d", a.lat, a.lng) } // zset uid -> last seen
func kWoken(u string) string   { return "k:woken:" + u }                            // zset of recent wake times
func kScan(a area) string      { return fmt.Sprintf("k:scan:%d:%d", a.lat, a.lng) } // fix time of the last wake scan of an area
func kOnline(u string) string  { return "k:online:" + u }
func kBlocked(u string) string { return "k:blocked:" + u } // set: everyone this user blocked or was blocked by

// blockedSet loads a user's blocks (both directions).
func (h *Hub) blockedSet(ctx context.Context, uid string) (map[string]bool, error) {
	members, err := h.rdb.SMembers(ctx, kBlocked(uid)).Result()
	set := make(map[string]bool, len(members))
	for _, m := range members {
		set[m] = true
	}
	return set, err
}

// block hides two people from each other at once, on every server.
func (h *Hub) block(ctx context.Context, a, b string) error {
	pipe := h.rdb.Pipeline()
	pipe.SAdd(ctx, kBlocked(a), b)
	pipe.SAdd(ctx, kBlocked(b), a)
	pipe.ZRem(ctx, kNear(a), b)
	pipe.ZRem(ctx, kNear(b), a)
	if _, err := pipe.Exec(ctx); err != nil {
		return err
	}
	h.changed(ctx, a, b)
	return nil
}

func ms(t time.Time) float64 { return float64(t.UnixMilli()) }

func score(t time.Time) string { return strconv.FormatInt(t.UnixMilli(), 10) }

// out is a pub/sub message: decks to recompute, a message for a user's sockets, or a revoked account.
type out struct {
	From   string          `json:"from"`
	Decks  []string        `json:"decks,omitempty"`
	UID    string          `json:"uid,omitempty"`
	Msg    json.RawMessage `json:"msg,omitempty"`
	Revoke bool            `json:"revoke,omitempty"`
}

// Hub matches people near each other. Live state is in Redis, so any number of servers share it;
// this struct holds only this server's sockets.
type Hub struct {
	rdb     redis.UniversalClient
	id      string
	wake    func(uid string) // queues a wake push; nil when off
	revoked func(uid string) // drops cached sessions of a deleted account

	mu      sync.Mutex
	clients map[string]map[*client]struct{}
	sent    map[string]string    // uid -> signature of the last deck sent, so unchanged decks aren't resent
	dirty   map[string]bool      // local users whose deck is rebuilt at the next flushDirty
	gone    map[string]time.Time // uid -> when its last socket here closed; sweep takes it off the map after leaveAt
	squares *squares             // half-second copies of map squares, shared by every report in a busy room
}

func newHub(rdb redis.UniversalClient) *Hub {
	return &Hub{rdb: rdb, id: randomHex(6), clients: map[string]map[*client]struct{}{}, sent: map[string]string{},
		dirty: map[string]bool{}, gone: map[string]time.Time{}, squares: newSquares(squareLife)}
}

func ignoreNil(err error) error {
	if errors.Is(err, redis.Nil) {
		return nil
	}
	return err
}

// packFix is how a fix is stored: "lat,lng,acc,unix ms,still".
func packFix(f fix) string {
	still := "0"
	if f.still {
		still = "1"
	}
	return fmt.Sprintf("%.7f,%.7f,%.1f,%d,%s", f.lat, f.lng, f.acc, f.at.UnixMilli(), still)
}

func parseFix(s string) (fix, bool) {
	p := strings.Split(s, ",")
	if len(p) != 5 {
		return fix{}, false
	}
	lat, e1 := strconv.ParseFloat(p[0], 64)
	lng, e2 := strconv.ParseFloat(p[1], 64)
	acc, e3 := strconv.ParseFloat(p[2], 64)
	at, e4 := strconv.ParseInt(p[3], 10, 64)
	if e1 != nil || e2 != nil || e3 != nil || e4 != nil {
		return fix{}, false
	}
	return fix{lat, lng, acc, time.UnixMilli(at), p[4] == "1"}, true
}

// fixes reads a few users' positions (the ones heard over Bluetooth).
func (h *Hub) fixes(ctx context.Context, uids []string) (map[string]fix, error) {
	pipe := h.rdb.Pipeline()
	cmds := make([]*redis.StringCmd, len(uids))
	for i, u := range uids {
		cmds[i] = pipe.Get(ctx, kFix(u))
	}
	if _, err := pipe.Exec(ctx); ignoreNil(err) != nil {
		return nil, err
	}
	out := make(map[string]fix, len(uids))
	for i, u := range uids {
		if f, ok := parseFix(cmds[i].Val()); ok {
			out[u] = f
		}
	}
	return out, nil
}

// around returns every fresh position in the cells around a point: one command per cell, however full the room,
// and none for a cell copied in the last squareLife. Stale entries found on the way are removed.
func (h *Hub) around(ctx context.Context, lat, lng float64, now time.Time) (map[string]fix, error) {
	out := map[string]fix{}
	cells := h.squares.get(neighbourhood(lat, lng), now, out)
	pipe := h.rdb.Pipeline()
	cmds := make([]*redis.MapStringStringCmd, len(cells))
	for i, c := range cells {
		cmds[i] = pipe.HGetAll(ctx, kCell(c))
	}
	if _, err := pipe.Exec(ctx); ignoreNil(err) != nil {
		return nil, err
	}
	pipe = h.rdb.Pipeline()
	stale := false
	for i, cmd := range cmds {
		fixes := map[string]fix{}
		for u, s := range cmd.Val() {
			if f, ok := parseFix(s); ok && f.fresh(now) {
				fixes[u], out[u] = f, f
			} else {
				pipe.HDel(ctx, kCell(cells[i]), u)
				stale = true
			}
		}
		h.squares.put(cells[i], fixes)
	}
	if stale {
		if _, err := pipe.Exec(ctx); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// report records a position and updates who is near whom. It returns false if the fix was refused.
func (h *Hub) report(ctx context.Context, uid string, f fix) (bool, error) {
	if !validFix(f) {
		return false, nil
	}
	pipe := h.rdb.Pipeline()
	hidden := pipe.Exists(ctx, kHidden(uid))
	prevCmd := pipe.Get(ctx, kFix(uid))
	if _, err := pipe.Exec(ctx); ignoreNil(err) != nil {
		return false, err
	}
	if hidden.Val() > 0 {
		return false, nil
	}
	prev, had := parseFix(prevCmd.Val())
	if had && f.at.Sub(prev.at) < minFixGap {
		return false, nil
	}
	if err := h.place(ctx, uid, f, prev, had); err != nil {
		return false, err
	}

	// Who is around me, and whom am I already paired with?
	theirs, err := h.around(ctx, f.lat, f.lng, f.at)
	if err != nil {
		return false, err
	}
	paired, err := h.rdb.ZRangeByScoreWithScores(ctx, kNear(uid), &redis.ZRangeBy{Min: score(f.at.Add(-holdFor)), Max: "+inf"}).Result()
	if err != nil {
		return false, err
	}
	since := make(map[string]float64, len(paired))
	for _, z := range paired {
		since[z.Member.(string)] = z.Score
	}
	blocked, err := h.blockedSet(ctx, uid)
	if err != nil {
		return false, err
	}

	type near struct {
		uid string
		d   float64
	}
	var nears []near
	pipe = h.rdb.Pipeline()
	var drops, adds []*redis.IntCmd
	var dropped, neared []string
	for other, o := range theirs {
		if other == uid || blocked[other] {
			continue
		}
		d := distanceM(f.lat, f.lng, o.lat, o.lng)
		slack := math.Min(f.acc+o.acc, errCapM)
		_, isPaired := since[other]
		switch {
		case d <= nearM+slack:
			nears = append(nears, near{other, d})
		case d > farM+slack && isPaired:
			drops = append(drops, pipe.ZRem(ctx, kNear(uid), other))
			pipe.ZRem(ctx, kNear(other), uid)
			dropped = append(dropped, other)
		}
	}
	// Save only pairs that are new or getting old, for the nearest maxNear people.
	slices.SortFunc(nears, func(a, b near) int { return cmp.Compare(a.d, b.d) })
	for _, n := range nears[:min(len(nears), maxNear)] {
		if s, ok := since[n.uid]; !ok || ms(f.at)-s >= float64(refreshNear.Milliseconds()) {
			adds = append(adds, h.markNear(ctx, pipe, uid, n.uid, f.at))
			neared = append(neared, n.uid)
		}
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return false, err
	}
	// Only arrivals and departures change anyone's cards. Re-confirming a pair that already exists changes
	// nothing, so a room of people standing together costs no card rebuilds at all.
	arrivals := hits(adds, neared)
	changed := append(arrivals, hits(drops, dropped)...)
	if len(changed) > 0 {
		changed = append(changed, uid)
	}
	h.count(ctx, countMatches, int64(len(arrivals)))

	if err := h.setLast(ctx, uid, areaOf(f.lat, f.lng), f.at); err != nil {
		return false, err
	}
	h.changed(ctx, changed...)
	if err := h.wakeAround(ctx, uid, f); err != nil {
		slog.Warn("wake selection", "err", err)
	}
	return true, nil
}

// place saves a user's position and moves them to its cell. prev is their last fix, if had.
func (h *Hub) place(ctx context.Context, uid string, f, prev fix, had bool) error {
	ttl := fixTTL
	if f.still {
		ttl = stillTTL
	}
	c, packed := cellOf(f.lat, f.lng), packFix(f)
	pipe := h.rdb.Pipeline()
	if had && cellOf(prev.lat, prev.lng) != c {
		pipe.HDel(ctx, kCell(cellOf(prev.lat, prev.lng)), uid)
	}
	pipe.Set(ctx, kFix(uid), packed, ttl)
	pipe.HSet(ctx, kCell(c), uid, packed)
	pipe.Expire(ctx, kCell(c), stillTTL)
	if _, err := pipe.Exec(ctx); err != nil {
		return err
	}
	if had && cellOf(prev.lat, prev.lng) != c {
		h.squares.move(cellOf(prev.lat, prev.lng), uid, nil)
	}
	h.squares.move(c, uid, &f)
	return nil
}

// markNear records that a and b are near now. The returned command, after Exec, is 1 when the pair is new.
func (h *Hub) markNear(ctx context.Context, pipe redis.Pipeliner, a, b string, at time.Time) *redis.IntCmd {
	added := pipe.ZAdd(ctx, kNear(a), redis.Z{Score: ms(at), Member: b})
	pipe.ZAdd(ctx, kNear(b), redis.Z{Score: ms(at), Member: a})
	pipe.Expire(ctx, kNear(a), holdFor+time.Minute)
	pipe.Expire(ctx, kNear(b), holdFor+time.Minute)
	return added
}

// hits returns who[i] for every command that changed something (a pair that is new, or one that was removed).
func hits(cmds []*redis.IntCmd, who []string) []string {
	var out []string
	for i, c := range cmds {
		if c.Val() > 0 {
			out = append(out, who[i])
		}
	}
	return out
}

func (h *Hub) setLast(ctx context.Context, uid string, a area, at time.Time) error {
	prev, err := h.rdb.Get(ctx, kLast(uid)).Result()
	if ignoreNil(err) != nil {
		return err
	}
	now := fmt.Sprintf("%d:%d", a.lat, a.lng)
	pipe := h.rdb.Pipeline()
	if prev != "" && prev != now {
		var p area
		if _, err := fmt.Sscanf(prev, "%d:%d", &p.lat, &p.lng); err == nil {
			pipe.ZRem(ctx, kArea(p), uid)
		}
	}
	pipe.Set(ctx, kLast(uid), now, lastLife)
	pipe.ZAdd(ctx, kArea(a), redis.Z{Score: ms(at), Member: uid})
	pipe.ZRemRangeByScore(ctx, kArea(a), "-inf", score(at.Add(-lastLife)))
	pipe.Expire(ctx, kArea(a), lastLife)
	_, err = pipe.Exec(ctx)
	return err
}

// wakeAround wakes phones last seen in or next to the reporter's area that have gone quiet,
// so a phone whose app was swiped away reports and can be matched.
// shortcut: wakes the whole ~1.6 km block around the fix (capped); a radius that grows with how
// fast the phone was moving (docs/ARCHITECTURE.md) would wake fewer phones.
func (h *Hub) wakeAround(ctx context.Context, uid string, f fix) error {
	if h.wake == nil {
		return nil
	}
	// One scan per area per wakeGap, shared by everyone there on every server: nobody can be woken twice
	// within wakeGap, so a room of 1000 people scans as rarely as one person.
	center := areaOf(f.lat, f.lng)
	last, err := h.rdb.Get(ctx, kScan(center)).Int64()
	if ignoreNil(err) != nil {
		return err
	}
	if f.at.UnixMilli()-last < wakeGap.Milliseconds() {
		return nil
	}
	if err := h.rdb.Set(ctx, kScan(center), f.at.UnixMilli(), 3*wakeGap).Err(); err != nil {
		return err
	}

	pipe := h.rdb.Pipeline()
	var ranges []*redis.StringSliceCmd
	for dy := int32(-1); dy <= 1; dy++ {
		for dx := int32(-1); dx <= 1; dx++ {
			// Only phones quiet for longer than a fix lives.
			key := kArea(area{center.lat + dy, center.lng + dx})
			ranges = append(ranges, pipe.ZRangeByScore(ctx, key, &redis.ZRangeBy{
				Min: score(f.at.Add(-lastLife)), Max: score(f.at.Add(-fixTTL)), Count: wakeScanMax}))
		}
	}
	if _, err := pipe.Exec(ctx); ignoreNil(err) != nil {
		return err
	}
	var cands []string
	for _, r := range ranges {
		for _, o := range r.Val() {
			if o != uid && len(cands) < wakeScanMax && !slices.Contains(cands, o) {
				cands = append(cands, o)
			}
		}
	}
	if len(cands) == 0 {
		return nil
	}

	// Hidden, connected, already findable, or woken too recently: skip. All checked in one round trip.
	pipe = h.rdb.Pipeline()
	type check struct {
		hidden, online, located *redis.IntCmd
		woken                   *redis.ZSliceCmd
	}
	checks := make([]check, len(cands))
	for i, o := range cands {
		pipe.ZRemRangeByScore(ctx, kWoken(o), "-inf", score(f.at.Add(-wakeWindow)))
		checks[i] = check{pipe.Exists(ctx, kHidden(o)), pipe.Exists(ctx, kOnline(o)), pipe.Exists(ctx, kFix(o)),
			pipe.ZRangeWithScores(ctx, kWoken(o), 0, -1)}
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return err
	}
	pipe = h.rdb.Pipeline()
	var chosen []string
	for i, o := range cands {
		c := checks[i]
		times := c.woken.Val()
		tooSoon := len(times) >= wakeBudget || (len(times) > 0 && ms(f.at)-times[len(times)-1].Score < float64(wakeGap.Milliseconds()))
		if c.hidden.Val() > 0 || c.online.Val() > 0 || c.located.Val() > 0 || tooSoon {
			continue
		}
		pipe.ZAdd(ctx, kWoken(o), redis.Z{Score: ms(f.at), Member: strconv.FormatInt(f.at.UnixNano(), 10)})
		pipe.Expire(ctx, kWoken(o), wakeWindow)
		if chosen = append(chosen, o); len(chosen) == wakesPerFix {
			break
		}
	}
	// shortcut: two servers choosing at the same instant can each wake a phone once; a Lua script makes it atomic.
	if _, err := pipe.Exec(ctx); err != nil {
		return err
	}
	for _, o := range chosen {
		h.wake(o)
	}
	h.count(ctx, countWakes, int64(len(chosen)))
	return nil
}

// sighted records the tokens a phone heard over Bluetooth: a direct hearing means the pair is close.
func (h *Hub) sighted(ctx context.Context, uid string, heard []Sighting, now time.Time) error {
	if n, err := h.rdb.Exists(ctx, kHidden(uid)).Result(); err != nil || n > 0 {
		return err
	}
	pipe := h.rdb.Pipeline()
	owners := make([]*redis.StringCmd, len(heard))
	for i, s := range heard {
		owners[i] = pipe.Get(ctx, kTok(s.Token))
	}
	if _, err := pipe.Exec(ctx); ignoreNil(err) != nil {
		return err
	}
	blocked, err := h.blockedSet(ctx, uid)
	if err != nil {
		return err
	}
	var heardFrom []string
	for i, s := range heard {
		owner := owners[i].Val()
		if owner != "" && owner != uid && !blocked[owner] && s.RSSI >= minRSSI && !slices.Contains(heardFrom, owner) {
			heardFrom = append(heardFrom, owner)
		}
	}
	if len(heardFrom) == 0 {
		return nil
	}
	fx, err := h.fixes(ctx, append([]string{uid}, heardFrom...))
	if err != nil {
		return err
	}
	var adds []*redis.IntCmd
	var neared []string
	pipe = h.rdb.Pipeline()
	for _, owner := range heardFrom {
		if gpsVetoes(fx[uid], fx[owner], now) {
			continue
		}
		adds = append(adds, h.markNear(ctx, pipe, uid, owner, now))
		neared = append(neared, owner)
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return err
	}
	arrivals := hits(adds, neared)
	h.count(ctx, countBLEMatches, int64(len(arrivals)))
	if len(arrivals) > 0 {
		h.changed(ctx, append(arrivals, uid)...)
	}
	return nil
}

// gpsVetoes: outdoors, two good fixes clearly apart overrule Bluetooth. Indoor (poor) or missing fixes never do.
func gpsVetoes(a, b fix, now time.Time) bool {
	if a.at.IsZero() || b.at.IsZero() || !a.fresh(now) || !b.fresh(now) || a.acc > outdoorAccM || b.acc > outdoorAccM {
		return false
	}
	return distanceM(a.lat, a.lng, b.lat, b.lng) > vetoM+a.acc+b.acc
}

// deck lists the cards a user should see now, nearest first.
func (h *Hub) deck(ctx context.Context, uid string, now time.Time) ([]Card, error) {
	if n, err := h.rdb.Exists(ctx, kHidden(uid)).Result(); err != nil || n > 0 {
		return nil, err
	}
	others, err := h.rdb.ZRangeByScore(ctx, kNear(uid), &redis.ZRangeBy{Min: score(now.Add(-holdFor)), Max: "+inf"}).Result()
	if err != nil || len(others) == 0 {
		return nil, err
	}
	blocked, err := h.blockedSet(ctx, uid)
	if err != nil {
		return nil, err
	}
	others = slices.DeleteFunc(others, func(o string) bool { return blocked[o] })

	// Nearest first, by the positions around me (one read per cell). Someone matched only by Bluetooth
	// whose position isn't around goes last.
	dist := map[string]float64{}
	if s, err := h.rdb.Get(ctx, kFix(uid)).Result(); ignoreNil(err) != nil {
		return nil, err
	} else if me, ok := parseFix(s); ok {
		fx, err := h.around(ctx, me.lat, me.lng, now)
		if err != nil {
			return nil, err
		}
		for _, o := range others {
			if f, ok := fx[o]; ok {
				dist[o] = distanceM(me.lat, me.lng, f.lat, f.lng)
			}
		}
	}
	far := func(o string) float64 {
		if d, ok := dist[o]; ok {
			return d
		}
		return math.MaxFloat64
	}
	slices.SortFunc(others, func(a, b string) int {
		return cmp.Or(cmp.Compare(far(a), far(b)), strings.Compare(a, b))
	})
	others = others[:min(len(others), maxDeck)] // a stadium can't flood a phone: the nearest 100

	pipe := h.rdb.Pipeline()
	cardCmds := make([]*redis.StringCmd, len(others))
	for i, o := range others {
		cardCmds[i] = pipe.Get(ctx, kCard(o))
	}
	if _, err := pipe.Exec(ctx); ignoreNil(err) != nil {
		return nil, err
	}
	var cards []Card
	for _, cmd := range cardCmds {
		var c Card
		if cmd.Val() != "" && json.Unmarshal([]byte(cmd.Val()), &c) == nil {
			cards = append(cards, c)
		}
	}
	return cards, nil
}

// changed marks these users' decks for rebuilding here and tells the other servers to do the same.
// The rebuild happens in flushDirty, so a crowd arriving at once costs one rebuild per person, not one per arrival.
func (h *Hub) changed(ctx context.Context, uids ...string) {
	if len(uids) == 0 {
		return
	}
	h.markDirty(uids)
	h.publish(ctx, out{Decks: uids})
}

func (h *Hub) markDirty(uids []string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, u := range uids {
		if len(h.clients[u]) > 0 {
			h.dirty[u] = true
		}
	}
}

// flushDirty rebuilds and sends the decks marked since the last call. main runs it every 250 ms.
func (h *Hub) flushDirty(ctx context.Context) {
	h.mu.Lock()
	marked := h.dirty
	h.dirty = map[string]bool{}
	h.mu.Unlock()
	var wg sync.WaitGroup
	busy := make(chan struct{}, flushWorkers)
	for u := range marked {
		busy <- struct{}{}
		wg.Add(1)
		go func() {
			defer func() { <-busy; wg.Done() }()
			h.flush(ctx, u)
		}()
	}
	wg.Wait()
}

// alive runs when a phone answers a ping. A still phone that keeps answering keeps its spot for as long as it is
// connected, however long Android delays its location; a moving phone's spot still needs fresh fixes.
func (h *Hub) alive(ctx context.Context, uid string) error {
	s, err := h.rdb.Get(ctx, kFix(uid)).Result()
	if ignoreNil(err) != nil {
		return err
	}
	f, ok := parseFix(s)
	if !ok || !f.still {
		return nil
	}
	now := f
	now.at = time.Now()
	if err := h.place(ctx, uid, now, f, true); err != nil {
		return err
	}
	// Still answering, so not a phone to wake.
	return h.rdb.ZAdd(ctx, kArea(areaOf(f.lat, f.lng)), redis.Z{Score: ms(now.at), Member: uid}).Err()
}

// flush sends a user's deck to this server's sockets if it differs from the last one sent.
func (h *Hub) flush(ctx context.Context, uid string) {
	if !h.local(uid) {
		return
	}
	cards, err := h.deck(ctx, uid, time.Now())
	if err != nil {
		slog.Warn("deck", "err", err)
		return
	}
	var sig strings.Builder
	for _, c := range cards {
		sig.WriteString(c.UID + c.Img + ";")
	}
	if cards == nil {
		cards = []Card{}
	}
	msg, _ := json.Marshal(map[string]any{"type": "deck", "cards": cards})
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.sent[uid] == sig.String() {
		return
	}
	h.sent[uid] = sig.String()
	if !h.deliver(uid, msg) {
		delete(h.sent, uid) // a slow socket missed it; the next sweep resends
	}
}

func (h *Hub) local(uid string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.clients[uid]) > 0
}

// deliver sends to every local socket of a user without blocking. Caller holds mu.
func (h *Hub) deliver(uid string, msg []byte) bool {
	ok := true
	for c := range h.clients[uid] {
		select {
		case c.send <- msg:
		default:
			ok = false
		}
	}
	return ok
}

// say sends one message to all of a user's sockets, on every server.
func (h *Hub) say(ctx context.Context, uid string, v any) {
	msg, _ := json.Marshal(v)
	h.mu.Lock()
	h.deliver(uid, msg)
	h.mu.Unlock()
	h.publish(ctx, out{UID: uid, Msg: msg})
}

func (h *Hub) publish(ctx context.Context, m out) {
	m.From = h.id
	b, _ := json.Marshal(m)
	if err := h.rdb.Publish(ctx, channel, b).Err(); err != nil {
		slog.Warn("publish", "err", err)
	}
}

// listen applies other servers' messages to this server's sockets until ctx ends.
func (h *Hub) listen(ctx context.Context) {
	sub := h.rdb.Subscribe(ctx, channel)
	defer sub.Close()
	for m := range sub.Channel() {
		var o out
		if json.Unmarshal([]byte(m.Payload), &o) != nil || o.From == h.id {
			continue
		}
		if o.Revoke && h.revoked != nil {
			h.revoked(o.UID)
		}
		if len(o.Msg) > 0 {
			h.mu.Lock()
			h.deliver(o.UID, o.Msg)
			h.mu.Unlock()
		}
		h.markDirty(o.Decks)
	}
}

// attach registers a socket. card is nil when the user has no card yet; hidden and blocked come from the database.
func (h *Hub) attach(ctx context.Context, c *client, card *Card, hidden bool, blocked []string) error {
	h.mu.Lock()
	if h.clients[c.uid] == nil {
		h.clients[c.uid] = map[*client]struct{}{}
	}
	h.clients[c.uid][c] = struct{}{}
	delete(h.sent, c.uid) // a new socket always gets the current deck
	delete(h.gone, c.uid)
	h.mu.Unlock()

	pipe := h.rdb.Pipeline()
	if hidden {
		pipe.Set(ctx, kHidden(c.uid), "1", 0)
	} else {
		pipe.Del(ctx, kHidden(c.uid))
	}
	if card != nil && !hidden {
		b, _ := json.Marshal(card)
		pipe.Set(ctx, kCard(c.uid), b, 0)
	}
	pipe.Set(ctx, kOnline(c.uid), h.id, onlineTTL)
	pipe.Del(ctx, kBlocked(c.uid))
	for _, b := range blocked {
		pipe.SAdd(ctx, kBlocked(c.uid), b)
	}
	token := pipe.Get(ctx, kTokOf(c.uid))
	if _, err := pipe.Exec(ctx); ignoreNil(err) != nil {
		return err
	}
	if !hidden {
		if t := token.Val(); t != "" {
			msg, _ := json.Marshal(map[string]any{"type": "ble", "token": t})
			select {
			case c.send <- msg:
			default:
			}
		} else if err := h.issueToken(ctx, c.uid); err != nil {
			return err
		}
	}
	h.flush(ctx, c.uid)
	return nil
}

func (h *Hub) detach(ctx context.Context, c *client) {
	h.mu.Lock()
	delete(h.clients[c.uid], c)
	last := len(h.clients[c.uid]) == 0
	if last {
		delete(h.clients, c.uid)
		delete(h.sent, c.uid)
		h.gone[c.uid] = time.Now()
	}
	h.mu.Unlock()
	if last {
		// Another server with a socket for this user sets it again within one sweep.
		h.rdb.Del(ctx, kOnline(c.uid))
	}
}

// issueToken gives a user a new Bluetooth token and sends it to their phones.
// The token is random, so a stranger who records it learns nothing and can't follow it past tokenLife.
func (h *Hub) issueToken(ctx context.Context, uid string) error {
	t := randomHex(8)
	pipe := h.rdb.Pipeline()
	pipe.Set(ctx, kTok(t), uid, tokenLife)
	pipe.Set(ctx, kTokOf(uid), t, tokenRotate)
	if _, err := pipe.Exec(ctx); err != nil {
		return err
	}
	h.say(ctx, uid, map[string]any{"type": "ble", "token": t})
	return nil
}

// sweep runs every few seconds on each server: keeps its users marked online, rotates their
// Bluetooth tokens and resends decks that changed as cards expired.
func (h *Hub) sweep(ctx context.Context) {
	h.mu.Lock()
	uids := make([]string, 0, len(h.clients))
	for u := range h.clients {
		uids = append(uids, u)
	}
	var left []string
	for u, at := range h.gone {
		if time.Since(at) >= leaveAt {
			left = append(left, u)
			delete(h.gone, u)
		}
	}
	h.mu.Unlock()
	h.squares.prune()
	for _, u := range left {
		if err := h.leave(ctx, u); err != nil {
			slog.Warn("leave", "err", err)
		}
	}
	for _, u := range uids {
		pipe := h.rdb.Pipeline()
		pipe.Set(ctx, kOnline(u), h.id, onlineTTL)
		hidden := pipe.Exists(ctx, kHidden(u))
		token := pipe.Exists(ctx, kTokOf(u))
		if _, err := pipe.Exec(ctx); err != nil {
			slog.Warn("sweep", "err", err)
			return
		}
		if hidden.Val() == 0 && token.Val() == 0 {
			if err := h.issueToken(ctx, u); err != nil {
				slog.Warn("token", "err", err)
			}
		}
		h.flush(ctx, u)
	}
}

// setCard publishes a new or edited card to everyone who can see it.
func (h *Hub) setCard(ctx context.Context, card Card) error {
	if n, err := h.rdb.Exists(ctx, kHidden(card.UID)).Result(); err != nil || n > 0 {
		return err // a hidden card is published by setHidden(false)
	}
	b, _ := json.Marshal(card)
	if err := h.rdb.Set(ctx, kCard(card.UID), b, 0).Err(); err != nil {
		return err
	}
	others, err := h.rdb.ZRange(ctx, kNear(card.UID), 0, -1).Result()
	if err != nil {
		return err
	}
	h.changed(ctx, append(others, card.UID)...)
	return nil
}

// setHidden hides or shows a user everywhere at once: no card, no position, no token, empty deck.
func (h *Hub) setHidden(ctx context.Context, uid string, hidden bool, card *Card) error {
	if hidden {
		others, err := h.forget(ctx, uid)
		if err != nil {
			return err
		}
		if err := h.rdb.Set(ctx, kHidden(uid), "1", 0).Err(); err != nil {
			return err
		}
		h.say(ctx, uid, map[string]any{"type": "vis", "visible": false})
		h.changed(ctx, append(others, uid)...)
		return nil
	}
	pipe := h.rdb.Pipeline()
	pipe.Del(ctx, kHidden(uid))
	if card != nil {
		b, _ := json.Marshal(card)
		pipe.Set(ctx, kCard(uid), b, 0)
	}
	online := pipe.Exists(ctx, kOnline(uid))
	if _, err := pipe.Exec(ctx); err != nil {
		return err
	}
	h.say(ctx, uid, map[string]any{"type": "vis", "visible": true})
	if online.Val() > 0 {
		if err := h.issueToken(ctx, uid); err != nil {
			return err
		}
	}
	h.changed(ctx, uid)
	return nil
}

// remove drops everything about a deleted account. Sockets still open for it are refused until they drop;
// its sessions are gone, so they can't come back.
func (h *Hub) remove(ctx context.Context, uid string) error {
	others, err := h.forget(ctx, uid)
	if err != nil {
		return err
	}
	if err := h.rdb.Del(ctx, kBlocked(uid)).Err(); err != nil {
		return err
	}
	if err := h.rdb.Set(ctx, kHidden(uid), "1", time.Hour).Err(); err != nil {
		return err
	}
	h.say(ctx, uid, map[string]any{"type": "vis", "visible": false})
	if h.revoked != nil {
		h.revoked(uid)
	}
	h.publish(ctx, out{UID: uid, Revoke: true})
	h.changed(ctx, append(others, uid)...)
	return nil
}

// forget drops a user's card, position, pairs, token and area. It returns who had them in their deck.
func (h *Hub) forget(ctx context.Context, uid string) ([]string, error) {
	others, err := h.unplace(ctx, uid)
	if err != nil {
		return nil, err
	}
	pipe := h.rdb.Pipeline()
	tokCmd := pipe.Get(ctx, kTokOf(uid))
	lastCmd := pipe.Get(ctx, kLast(uid))
	if _, err := pipe.Exec(ctx); ignoreNil(err) != nil {
		return nil, err
	}
	pipe = h.rdb.Pipeline()
	if t := tokCmd.Val(); t != "" {
		pipe.Del(ctx, kTok(t))
	}
	var a area
	if _, err := fmt.Sscanf(lastCmd.Val(), "%d:%d", &a.lat, &a.lng); err == nil {
		pipe.ZRem(ctx, kArea(a), uid)
	}
	for _, k := range []string{kCard(uid), kTokOf(uid), kLast(uid), kWoken(uid)} {
		pipe.Del(ctx, k)
	}
	_, err = pipe.Exec(ctx)
	return others, err
}

// unplace removes a user's position and pairs. It returns who had them in their deck.
func (h *Hub) unplace(ctx context.Context, uid string) ([]string, error) {
	pipe := h.rdb.Pipeline()
	fixCmd := pipe.Get(ctx, kFix(uid))
	nearCmd := pipe.ZRange(ctx, kNear(uid), 0, -1)
	if _, err := pipe.Exec(ctx); ignoreNil(err) != nil {
		return nil, err
	}
	others := nearCmd.Val()
	pipe = h.rdb.Pipeline()
	if f, ok := parseFix(fixCmd.Val()); ok {
		pipe.HDel(ctx, kCell(cellOf(f.lat, f.lng)), uid)
		h.squares.move(cellOf(f.lat, f.lng), uid, nil)
	}
	for _, o := range others {
		pipe.ZRem(ctx, kNear(o), uid)
	}
	pipe.Del(ctx, kFix(uid))
	pipe.Del(ctx, kNear(uid))
	_, err := pipe.Exec(ctx)
	return others, err
}

// leave takes a phone that lost its connection off the map, unless it came back on another server.
// Its card, token and area stay: it reappears the moment it reconnects, and can still be woken.
func (h *Hub) leave(ctx context.Context, uid string) error {
	if n, err := h.rdb.Exists(ctx, kOnline(uid)).Result(); err != nil || n > 0 {
		return err
	}
	others, err := h.unplace(ctx, uid)
	if err == nil {
		h.changed(ctx, append(others, uid)...)
	}
	return err
}
