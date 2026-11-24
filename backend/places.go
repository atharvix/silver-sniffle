package main

import (
	"context"
	"log"
	"math"
	"sort"
	"sync"
	"time"
)

// Places: admin-drawn circles (a venue, a campus) with live visit counts. Memory only, like
// proximity: who is inside right now and who came by today are sets of uids that are never
// stored or shown. Only per-day counts (visitors, peak, entries) reach Postgres (place_days).

const (
	placeStale  = fixFresh         // no report from inside for this long: they've left
	placeSlackM = 25               // a fix's error counts for up to this much (indoor GPS)
	recentKeep  = 60 * time.Minute // the entries feed
)

type geoPlace struct {
	id       int64
	name     string
	lat, lng float64
	r        float64
	inside   map[string]time.Time // uid -> last report from inside
	today    map[string]bool      // visited today (usageTZ day)
	entries  int                  // entries today
	peak     int                  // most inside at once today
	recent   []time.Time          // entry times, last hour
	msg      *PlaceMsg            // sent once to each person found inside during its window
	sent     map[string]bool      // got msg this run (place_sends dedupes across restarts)
	event    int64                // an event's venue (geoWatch.events): live from..until only
	from     time.Time
	until    time.Time
}

// arrival: someone is inside a place whose message is live (the App sends it).
type arrival struct {
	place int64
	event int64 // an event venue: the App checks they registered, then says "you're here"
	uid   string
	msg   PlaceMsg
}

type placeEntry struct {
	Place int64     `json:"place"`
	Name  string    `json:"name"`
	At    time.Time `json:"at"`
}

type geoWatch struct {
	mu     sync.Mutex
	places []*geoPlace
	day    string
	active map[string]bool // reported a fix today, anywhere (the denominator for "% visited")
	feed   []placeEntry    // newest last, capped
	cities chan cityJob
	asked  map[string]bool // uids whose city was looked up this run
	arrive chan arrival    // place messages to send
	events []*geoPlace     // venues of published events starting within a day (eventVenueM around the pin)
}

type cityJob struct {
	uid      string
	lat, lng float64
}

func newGeoWatch() *geoWatch {
	return &geoWatch{active: map[string]bool{}, cities: make(chan cityJob, 1024), arrive: make(chan arrival, 4096), asked: map[string]bool{}, day: usageDay(time.Now())}
}

func usageDay(t time.Time) string {
	if loc, err := time.LoadLocation(usageTZ); err == nil {
		t = t.In(loc)
	}
	return t.Format("2006-01-02")
}

// observe is called with every accepted fix (hub lock held: never call back into the hub).
func (g *geoWatch) observe(uid string, lat, lng float64, acc int, now time.Time) {
	if g == nil {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.rollDay(now)
	g.active[uid] = true
	if !g.asked[uid] && acc > 0 && acc <= 2000 {
		g.asked[uid] = true
		select {
		case g.cities <- cityJob{uid, lat, lng}:
		default:
			delete(g.asked, uid) // busy: the next fix asks again
		}
	}
	if acc > maxAccM {
		return
	}
	slack := math.Min(float64(acc), placeSlackM)
	for _, p := range g.places {
		in := distM(lat, lng, p.lat, p.lng) <= p.r+slack
		_, was := p.inside[uid]
		switch {
		case in && !was:
			p.inside[uid] = now
			p.today[uid] = true
			p.entries++
			p.recent = append(p.recent, now)
			p.peak = max(p.peak, len(p.inside))
			g.feed = append(g.feed, placeEntry{p.id, p.name, now})
			if len(g.feed) > 200 {
				g.feed = g.feed[len(g.feed)-200:]
			}
		case in:
			p.inside[uid] = now
		case was:
			delete(p.inside, uid)
		}
		if in && p.msg != nil && !p.sent[uid] && p.msg.live(now) {
			select {
			case g.arrive <- arrival{place: p.id, uid: uid, msg: *p.msg}:
				p.sent[uid] = true
			default: // backed up: the next fix from inside tries again
			}
		}
	}
	for _, p := range g.events {
		if now.Before(p.from) || !now.Before(p.until) {
			delete(p.inside, uid)
			continue
		}
		if distM(lat, lng, p.lat, p.lng) > p.r+slack {
			delete(p.inside, uid)
			continue
		}
		p.inside[uid] = now
		if !p.sent[uid] {
			select {
			case g.arrive <- arrival{event: p.event, uid: uid}:
				p.sent[uid] = true
			default:
			}
		}
	}
}

// loadEvents replaces the watched event venues, keeping who's inside and who was told.
func (g *geoWatch) loadEvents(evs []*geoPlace) {
	g.mu.Lock()
	defer g.mu.Unlock()
	old := map[int64]*geoPlace{}
	for _, p := range g.events {
		old[p.event] = p
	}
	for _, e := range evs {
		if o := old[e.event]; o != nil {
			e.inside, e.sent = o.inside, o.sent
		}
	}
	g.events = evs
}

// eventsInside: how many people are at each live event venue right now.
func (g *geoWatch) eventsInside() map[int64]int {
	g.mu.Lock()
	defer g.mu.Unlock()
	out := map[int64]int{}
	for _, p := range g.events {
		out[p.event] = len(p.inside)
	}
	return out
}

func (g *geoWatch) forget(uid string) {
	if g == nil {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	for _, p := range g.places {
		delete(p.inside, uid)
	}
	for _, p := range g.events {
		delete(p.inside, uid)
	}
}

// rollDay starts a new day's counts. Caller holds g.mu.
func (g *geoWatch) rollDay(now time.Time) {
	d := usageDay(now)
	if d == g.day {
		return
	}
	g.day = d
	g.active = map[string]bool{}
	for _, p := range g.places {
		p.today, p.entries, p.peak = map[string]bool{}, 0, len(p.inside)
		for uid := range p.inside {
			p.today[uid] = true
		}
	}
}

// expire drops people nothing has been heard from inside a place for placeStale.
func (g *geoWatch) expire(now time.Time) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.rollDay(now)
	for _, p := range g.places {
		for uid, at := range p.inside {
			if now.Sub(at) > placeStale {
				delete(p.inside, uid)
			}
		}
		i := 0
		for i < len(p.recent) && now.Sub(p.recent[i]) > recentKeep {
			i++
		}
		p.recent = p.recent[i:]
	}
	for _, p := range g.events {
		for uid, at := range p.inside {
			if now.Sub(at) > placeStale {
				delete(p.inside, uid)
			}
		}
	}
}

// load replaces the watched places (after an admin edit), keeping live counts for kept ones.
func (g *geoWatch) load(rows []Place, today map[int64]PlaceDay) {
	g.mu.Lock()
	defer g.mu.Unlock()
	old := map[int64]*geoPlace{}
	for _, p := range g.places {
		old[p.id] = p
	}
	g.places = g.places[:0]
	for _, r := range rows {
		if !r.Active {
			continue
		}
		p := old[r.ID]
		if p == nil {
			p = &geoPlace{id: r.ID, inside: map[string]time.Time{}, today: map[string]bool{}, sent: map[string]bool{}}
			if d, ok := today[r.ID]; ok { // a restart: today's counts so far carry on
				p.entries, p.peak = d.Entries, d.Peak
			}
		}
		p.name, p.lat, p.lng, p.r = r.Name, r.Lat, r.Lng, float64(r.RadiusM)
		if r.Msg == nil || p.msg == nil || *r.Msg != *p.msg {
			p.sent = map[string]bool{} // a new or edited message: place_sends still stops repeats of the same one
		}
		p.msg = r.Msg
		g.places = append(g.places, p)
	}
}

type PlaceLive struct {
	ID       int64   `json:"id"`
	Inside   int     `json:"inside"`
	Today    int     `json:"today"`
	Entries  int     `json:"entries"`
	LastHour int     `json:"lastHour"`
	Peak     int     `json:"peak"`
	PctToday float64 `json:"pctToday"` // of everyone who reported a fix today
}

func (g *geoWatch) snapshot(now time.Time) (out []PlaceLive, active int, feed []placeEntry) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.rollDay(now)
	active = len(g.active)
	for _, p := range g.places {
		l := PlaceLive{ID: p.id, Inside: len(p.inside), Today: len(p.today), Entries: p.entries, Peak: p.peak}
		for _, t := range p.recent {
			if now.Sub(t) <= time.Hour {
				l.LastHour++
			}
		}
		if active > 0 {
			l.PctToday = math.Round(float64(l.Today)/float64(active)*1000) / 10
		}
		out = append(out, l)
	}
	feed = append([]placeEntry(nil), g.feed...)
	sort.Slice(feed, func(i, j int) bool { return feed[i].At.After(feed[j].At) })
	if len(feed) > 30 {
		feed = feed[:30]
	}
	return out, active, feed
}

// persist writes today's counts (never who): GREATEST keeps a restart from lowering them.
func (g *geoWatch) persist(ctx context.Context, s *Store) {
	g.mu.Lock()
	day := g.day
	type row struct{ id, visitors, peak, entries int64 }
	var rows []row
	for _, p := range g.places {
		rows = append(rows, row{p.id, int64(len(p.today)), int64(p.peak), int64(p.entries)})
	}
	g.mu.Unlock()
	for _, r := range rows {
		if _, err := s.pool.Exec(ctx, `
			INSERT INTO place_days (place_id, day, visitors, peak, entries) VALUES ($1, $2::date, $3, $4, $5)
			ON CONFLICT (place_id, day) DO UPDATE SET visitors = GREATEST(place_days.visitors, EXCLUDED.visitors),
			  peak = GREATEST(place_days.peak, EXCLUDED.peak), entries = GREATEST(place_days.entries, EXCLUDED.entries)`,
			r.id, day, r.visitors, r.peak, r.entries); err != nil {
			log.Printf("place persist: %v", err)
			return
		}
	}
}

// run: city lookups for new fixes, expiry, and a per-minute write of today's counts.
func (g *geoWatch) run(ctx context.Context, s *Store) {
	exp, save := time.NewTicker(5*time.Second), time.NewTicker(time.Minute)
	defer exp.Stop()
	defer save.Stop()
	for {
		select {
		case <-ctx.Done():
			c, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			g.persist(c, s)
			cancel()
			return
		case j := <-g.cities:
			if city, country := nearestCity(j.lat, j.lng); city != "" {
				c, cancel := context.WithTimeout(ctx, 3*time.Second)
				_, _ = s.pool.Exec(c, `UPDATE users SET city = $2, country = $3 WHERE id = $1 AND city = ''`, j.uid, city, country)
				cancel()
			}
		case now := <-exp.C:
			g.expire(now)
		case <-save.C:
			c, cancel := context.WithTimeout(ctx, 10*time.Second)
			g.persist(c, s)
			cancel()
		}
	}
}

// livePoint is one placed person for the admin's live views (never sent out as a point).
type livePoint struct {
	uid      string
	lat, lng float64
	open     bool // the app is on screen (a socket open)
}

// livePoints: everyone the server can place right now, hidden accounts excluded.
func (h *Hub) livePoints(now time.Time) []livePoint {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]livePoint, 0, len(h.fixes))
	for uid, f := range h.fixes {
		if !f.fresh(now) || h.hidden[uid] {
			continue
		}
		lat, lng := f.pos(now)
		out = append(out, livePoint{uid, lat, lng, len(h.clients[uid]) > 0})
	}
	return out
}

// onlineUIDs: accounts with the app open right now.
func (h *Hub) onlineUIDs() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]string, 0, len(h.clients))
	for uid := range h.clients {
		out = append(out, uid)
	}
	return out
}
