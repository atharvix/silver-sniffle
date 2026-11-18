package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

/* ---------------- places (admin) ---------------- */

type Place struct {
	ID      int64     `json:"id"`
	Name    string    `json:"name"`
	Lat     float64   `json:"lat"`
	Lng     float64   `json:"lng"`
	RadiusM int       `json:"radius"`
	Active  bool      `json:"active"`
	Created time.Time `json:"createdAt"`
	Msg     *PlaceMsg `json:"msg"`  // sent to each person found inside while it's live
	Sent    int       `json:"sent"` // how many got it
}

// PlaceMsg: "tell everyone who is at (or walks into) this place between From and Until".
// Rev changes on every save, so an edited message goes out again.
type PlaceMsg struct {
	Title    string    `json:"title"`
	Body     string    `json:"body"`
	CTALabel string    `json:"ctaLabel"`
	CTAURL   string    `json:"ctaUrl"`
	Push     bool      `json:"push"`
	InApp    bool      `json:"inapp"`
	From     time.Time `json:"from"`
	Until    time.Time `json:"until"`
	Rev      int64     `json:"rev"`
}

func (m *PlaceMsg) live(now time.Time) bool {
	return m != nil && !now.Before(m.From) && now.Before(m.Until)
}

const maxPlaceMsg = 30 * 24 * time.Hour

type PlaceDay struct {
	Day      string `json:"day"`
	Visitors int    `json:"visitors"`
	Peak     int    `json:"peak"`
	Entries  int    `json:"entries"`
}

func (s *Store) places(ctx context.Context) ([]Place, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, name, lat, lng, radius_m, active, created_at, msg, msg_sent FROM places ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Place{}
	for rows.Next() {
		var p Place
		var msg []byte
		if err := rows.Scan(&p.ID, &p.Name, &p.Lat, &p.Lng, &p.RadiusM, &p.Active, &p.Created, &msg, &p.Sent); err != nil {
			return nil, err
		}
		if len(msg) > 0 {
			p.Msg = &PlaceMsg{}
			if json.Unmarshal(msg, p.Msg) != nil {
				p.Msg = nil
			}
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// reloadPlaces hands the watched set to the hub (boot, and after every admin edit).
func (a *App) reloadPlaces(ctx context.Context) error {
	ps, err := a.store.places(ctx)
	if err != nil {
		return err
	}
	today := map[int64]PlaceDay{}
	rows, err := a.store.pool.Query(ctx, `SELECT place_id, visitors, peak, entries FROM place_days WHERE day = `+todaySQL)
	if err != nil {
		return err
	}
	for rows.Next() {
		var id int64
		var d PlaceDay
		if rows.Scan(&id, &d.Visitors, &d.Peak, &d.Entries) == nil {
			today[id] = d
		}
	}
	rows.Close()
	a.hub.geo.load(ps, today)
	return a.reloadEventVenues(ctx)
}

// Event venues are watched like places, without an admin making one: a published event
// with a pinned venue, from an hour before it starts to its end (3 h if none is set).
// Someone registered who arrives gets one "you're here" message. Who arrived is kept only
// until the event ends (event_arrivals), like place_sends.
const (
	eventVenueM    = 75 // metres around the pin: a cafe or a hall, plus indoor GPS error (placeSlackM)
	eventEarly     = time.Hour
	eventGeoSchema = `
CREATE TABLE IF NOT EXISTS event_arrivals (
  event_id   bigint NOT NULL REFERENCES events(id) ON DELETE CASCADE,
  uid        text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  expires_at timestamptz NOT NULL,
  PRIMARY KEY (event_id, uid)
)`
)

func (a *App) reloadEventVenues(ctx context.Context) error {
	rows, err := a.store.pool.Query(ctx, `
		SELECT id, title, lat, lng, starts_at, coalesce(ends_at, starts_at + interval '3 hours') FROM events
		WHERE published AND lat IS NOT NULL AND lng IS NOT NULL
		  AND coalesce(ends_at, starts_at + interval '3 hours') > now() AND starts_at < now() + interval '1 day'`)
	if err != nil {
		return err
	}
	defer rows.Close()
	var evs []*geoPlace
	for rows.Next() {
		p := &geoPlace{r: eventVenueM, inside: map[string]time.Time{}, sent: map[string]bool{}}
		var start time.Time
		if err := rows.Scan(&p.event, &p.name, &p.lat, &p.lng, &start, &p.until); err != nil {
			return err
		}
		p.from = start.Add(-eventEarly)
		evs = append(evs, p)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	a.hub.geo.loadEvents(evs)
	return nil
}

// sendEventArrival: someone is at a live event's venue. Only people registered for it hear
// about it, once per event (event_arrivals claimed first).
func (a *App) sendEventArrival(ctx context.Context, ar arrival) {
	var title string
	var until time.Time
	err := a.store.pool.QueryRow(ctx, `SELECT e.title, coalesce(e.ends_at, e.starts_at + interval '3 hours') FROM events e
		JOIN event_rsvps r ON r.event_id = e.id AND r.uid = $2 WHERE e.id = $1`, ar.event, ar.uid).Scan(&title, &until)
	if err != nil {
		return // not registered (or gone)
	}
	tag, err := a.store.pool.Exec(ctx, `INSERT INTO event_arrivals (event_id, uid, expires_at) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`, ar.event, ar.uid, until)
	if err != nil || tag.RowsAffected() == 0 {
		return
	}
	link := "kinjo://events/" + strconv.FormatInt(ar.event, 10)
	t, body := "You're at "+title, "Open Kinjo to see who's in the room."
	a.notifyUser(ar.uid, t, body, map[string]string{"link": link})
	if err := a.sendInApp(ctx, ar.uid, 0, t, body, "See who's here", link); err != nil {
		log.Printf("event arrival %d: %v", ar.event, err)
	}
}

const todaySQL = today

// GET /admin/places: every place with 14 days of counts. POST: create/update. DELETE ?id=.
func (a *App) handleAdminPlaces(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	switch r.Method {
	case http.MethodPost:
		var in struct {
			Place
			ClearMsg bool `json:"clearMsg"`
		}
		if decodeJSON(r, &in) != nil || strings.TrimSpace(in.Name) == "" || in.RadiusM < 10 || in.RadiusM > 50000 ||
			in.Lat < -90 || in.Lat > 90 || in.Lng < -180 || in.Lng > 180 {
			writeJSON(w, http.StatusBadRequest, errBody("name, a point and a radius of 10 m to 50 km"))
			return
		}
		var msg []byte
		if m := in.Msg; m != nil {
			m.Title, m.Body, m.CTALabel = clip(m.Title, 80), clip(m.Body, 600), clip(m.CTALabel, 30)
			if m.From.IsZero() {
				m.From = time.Now()
			}
			switch {
			case m.Title == "":
				writeJSON(w, http.StatusBadRequest, errBody("the message needs a title"))
				return
			case !m.Push && !m.InApp:
				writeJSON(w, http.StatusBadRequest, errBody("send the message as a push, in-app, or both"))
				return
			case !m.Until.After(m.From) || m.Until.Sub(m.From) > maxPlaceMsg:
				writeJSON(w, http.StatusBadRequest, errBody("the message runs for up to 30 days and must end after it starts"))
				return
			case m.CTAURL != "" && safeLink(m.CTAURL) == "":
				writeJSON(w, http.StatusBadRequest, errBody("button link must be https:// or kinjo://"))
				return
			}
			m.Rev = time.Now().UnixNano()
			msg, _ = json.Marshal(m)
		}
		var err error
		if in.ID > 0 {
			// keep the old message (and its rev, so nobody gets it twice) unless one was sent
			_, err = a.store.pool.Exec(ctx, `UPDATE places SET name = $2, lat = $3, lng = $4, radius_m = $5, active = $6,
				msg = CASE WHEN $8 THEN NULL WHEN $7::jsonb IS NULL THEN msg ELSE $7::jsonb END WHERE id = $1`,
				in.ID, clip(in.Name, 60), in.Lat, in.Lng, in.RadiusM, in.Active, msg, in.ClearMsg)
			if err == nil && (msg != nil || in.ClearMsg) {
				_, err = a.store.pool.Exec(ctx, `DELETE FROM place_sends WHERE place_id = $1`, in.ID) // the old message's record goes
			}
		} else {
			err = a.store.pool.QueryRow(ctx, `INSERT INTO places (name, lat, lng, radius_m, msg) VALUES ($1, $2, $3, $4, $5) RETURNING id`,
				clip(in.Name, 60), in.Lat, in.Lng, in.RadiusM, msg).Scan(&in.ID)
		}
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, errBody("save failed"))
			return
		}
	case http.MethodDelete:
		id, _ := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
		if _, err := a.store.pool.Exec(ctx, `DELETE FROM places WHERE id = $1`, id); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, errBody("delete failed"))
			return
		}
	}
	if r.Method != http.MethodGet {
		if err := a.reloadPlaces(ctx); err != nil {
			log.Printf("places reload: %v", err)
		}
	}
	ps, err := a.store.places(ctx)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, errBody("unavailable"))
		return
	}
	hist := map[int64][]PlaceDay{}
	rows, err := a.store.pool.Query(ctx, `SELECT place_id, to_char(day, 'YYYY-MM-DD'), visitors, peak, entries FROM place_days
		WHERE day > `+today+` - 14 ORDER BY day`)
	if err == nil {
		for rows.Next() {
			var id int64
			var d PlaceDay
			if rows.Scan(&id, &d.Day, &d.Visitors, &d.Peak, &d.Entries) == nil {
				hist[id] = append(hist[id], d)
			}
		}
		rows.Close()
	}
	type out struct {
		Place
		History []PlaceDay `json:"history"`
	}
	list := make([]out, 0, len(ps))
	for _, p := range ps {
		list = append(list, out{p, hist[p.ID]})
	}
	writeJSON(w, http.StatusOK, map[string]any{"places": list})
}

// GET /admin/places/live: the real-time numbers (poll every few seconds): who's inside now,
// visitors today and their share of everyone out with Kinjo today, entries in the last hour.
func (a *App) handleAdminPlacesLive(w http.ResponseWriter, r *http.Request) {
	live, active, feed := a.hub.geo.snapshot(time.Now())
	var dau int
	_ = a.store.pool.QueryRow(r.Context(), `SELECT count(*) FROM app_opens WHERE day = `+today).Scan(&dau)
	if live == nil {
		live = []PlaceLive{}
	}
	if feed == nil {
		feed = []placeEntry{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"places": live, "activeToday": active, "dau": dau, "feed": feed, "at": time.Now()})
}

/* ---------------- events ---------------- */

type Event struct {
	ID          int64      `json:"id"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	City        string     `json:"city"`
	Venue       string     `json:"venue"`
	Address     string     `json:"address"`
	Lat         *float64   `json:"lat"`
	Lng         *float64   `json:"lng"`
	StartsAt    time.Time  `json:"startsAt"`
	EndsAt      *time.Time `json:"endsAt"`
	Cover       string     `json:"cover,omitempty"` // admin writes a data: URL; readers get CoverURL
	CoverURL    string     `json:"coverUrl"`
	Capacity    int        `json:"capacity"`
	Host        string     `json:"host"`
	Link        string     `json:"link"`
	Published   bool       `json:"published"`
	Going       int        `json:"going"`
	Registered  bool       `json:"registered"`
	Here        int        `json:"here,omitempty"` // admin: people at the venue right now (live, never stored)
	Updated     time.Time  `json:"updatedAt"`
}

const eventCols = `e.id, e.title, e.description, e.city, e.venue, e.address, e.lat, e.lng, e.starts_at, e.ends_at,
	e.cover <> '', e.capacity, e.host, e.link, e.published, e.updated_at,
	(SELECT count(*) FROM event_rsvps r WHERE r.event_id = e.id)`

func (a *App) scanEvents(rows pgx.Rows) ([]Event, error) {
	defer rows.Close()
	out := []Event{}
	for rows.Next() {
		var e Event
		var cover bool
		if err := rows.Scan(&e.ID, &e.Title, &e.Description, &e.City, &e.Venue, &e.Address, &e.Lat, &e.Lng, &e.StartsAt, &e.EndsAt,
			&cover, &e.Capacity, &e.Host, &e.Link, &e.Published, &e.Updated, &e.Going, &e.Registered); err != nil {
			return nil, err
		}
		if cover {
			e.CoverURL = a.publicURL + "/api/event-cover/" + strconv.FormatInt(e.ID, 10) + "?v=" + strconv.FormatInt(e.Updated.Unix(), 10)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// GET /api/events?city=: upcoming published events, the city's first (public: the website
// lists them too). With a session, each says whether you're registered.
func (a *App) handleEvents(w http.ResponseWriter, r *http.Request) {
	uid, _ := a.uidFromRequest(r)
	city := strings.TrimSpace(r.URL.Query().Get("city"))
	if city == "" && uid != "" {
		_ = a.store.pool.QueryRow(r.Context(), `SELECT city FROM users WHERE id = $1`, uid).Scan(&city)
	}
	rows, err := a.store.pool.Query(r.Context(), `SELECT `+eventCols+`,
		EXISTS (SELECT 1 FROM event_rsvps r WHERE r.event_id = e.id AND r.uid = $2)
		FROM events e WHERE e.published AND coalesce(e.ends_at, e.starts_at + interval '3 hours') > now()
		ORDER BY lower(e.city) = lower($1) DESC, e.starts_at LIMIT 100`, city, uid)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, errBody("unavailable"))
		return
	}
	evs, err := a.scanEvents(rows)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, errBody("unavailable"))
		return
	}
	cities := map[string]int{}
	for _, e := range evs {
		cities[e.City]++
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": evs, "city": city, "cities": topN(cities, 30)})
}

func (a *App) event(ctx context.Context, id int64, uid string, admin bool) (Event, error) {
	rows, err := a.store.pool.Query(ctx, `SELECT `+eventCols+`,
		EXISTS (SELECT 1 FROM event_rsvps r WHERE r.event_id = e.id AND r.uid = $2)
		FROM events e WHERE e.id = $1 AND (e.published OR $3)`, id, uid, admin)
	if err != nil {
		return Event{}, err
	}
	evs, err := a.scanEvents(rows)
	if err != nil {
		return Event{}, err
	}
	if len(evs) == 0 {
		return Event{}, pgx.ErrNoRows
	}
	return evs[0], nil
}

func eventID(r *http.Request) int64 {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	return id
}

// GET /api/events/{id}
func (a *App) handleEvent(w http.ResponseWriter, r *http.Request) {
	uid, _ := a.uidFromRequest(r)
	e, err := a.event(r.Context(), eventID(r), uid, false)
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, errBody("no such event"))
		return
	}
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, errBody("unavailable"))
		return
	}
	writeJSON(w, http.StatusOK, e)
}

// POST /api/events/{id}/rsvp registers (needs a finished card: the people going see it).
// DELETE cancels.
func (a *App) handleRSVP(w http.ResponseWriter, r *http.Request, uid string) {
	ctx := r.Context()
	e, err := a.event(ctx, eventID(r), uid, false)
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, errBody("no such event"))
		return
	}
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, errBody("unavailable"))
		return
	}
	if r.Method == http.MethodDelete {
		_, err = a.store.pool.Exec(ctx, `DELETE FROM event_rsvps WHERE event_id = $1 AND uid = $2`, e.ID, uid)
	} else {
		p, ok, perr := a.store.getProfile(ctx, uid)
		if perr != nil {
			writeJSON(w, http.StatusServiceUnavailable, errBody("unavailable"))
			return
		}
		if !ok || !p.complete() {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "Finish your Kinjo card first: the people going see it.", "code": "profile"})
			return
		}
		ends := e.StartsAt.Add(3 * time.Hour)
		if e.EndsAt != nil {
			ends = *e.EndsAt
		}
		if time.Now().After(ends) {
			writeJSON(w, http.StatusConflict, errBody("This event has ended."))
			return
		}
		if e.Capacity > 0 && !e.Registered && e.Going >= e.Capacity {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "This event is full.", "code": "full"})
			return
		}
		_, err = a.store.pool.Exec(ctx, `INSERT INTO event_rsvps (event_id, uid) VALUES ($1, $2) ON CONFLICT DO NOTHING`, e.ID, uid)
	}
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, errBody("unavailable"))
		return
	}
	e, _ = a.event(ctx, e.ID, uid, false)
	writeJSON(w, http.StatusOK, e)
}

// GET /api/events/{id}/people: the cards of everyone going (registered people only; hidden
// accounts and unfinished cards are left out). Same card shape as the nearby deck.
func (a *App) handleEventPeople(w http.ResponseWriter, r *http.Request, uid string) {
	ctx := r.Context()
	id := eventID(r)
	var registered bool
	if err := a.store.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM event_rsvps WHERE event_id = $1 AND uid = $2)`, id, uid).Scan(&registered); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, errBody("unavailable"))
		return
	}
	if !registered {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "Register to see who's going.", "code": "register"})
		return
	}
	rows, err := a.store.pool.Query(ctx, `SELECT p.uid, p.name, p.role, p.look, p.updated_at FROM event_rsvps r
		JOIN users u ON u.id = r.uid AND NOT u.hidden JOIN profiles p ON p.uid = r.uid
		WHERE r.event_id = $1 AND r.uid <> $2 AND p.name <> '' AND p.role <> '' AND p.look <> '' AND p.photo <> ''
		ORDER BY r.created_at DESC LIMIT 500`, id, uid)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, errBody("unavailable"))
		return
	}
	defer rows.Close()
	people := []nearbyPerson{}
	for rows.Next() {
		var p nearbyPerson
		var at time.Time
		if err := rows.Scan(&p.UID, &p.Name, &p.Role, &p.Desc, &at); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, errBody("unavailable"))
			return
		}
		p.Img = a.hub.photoBase + "/api/photo/" + p.UID + "?v=" + strconv.FormatInt(at.Unix(), 10)
		people = append(people, p)
	}
	writeJSON(w, http.StatusOK, map[string]any{"people": people})
}

// GET /api/event-cover/{id}: the cover image (public, immutable per ?v=, like photos).
func (a *App) handleEventCover(w http.ResponseWriter, r *http.Request) {
	var cover string
	if err := a.store.pool.QueryRow(r.Context(), `SELECT cover FROM events WHERE id = $1`, eventID(r)).Scan(&cover); err != nil {
		http.NotFound(w, r)
		return
	}
	meta, data, found := strings.Cut(strings.TrimPrefix(cover, "data:"), ";base64,")
	b, err := base64.StdEncoding.DecodeString(data)
	if !found || err != nil || !photoTypes[meta] {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", meta)
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(b)
}

/* ---------------- events (admin) ---------------- */

const maxCover = 1500000

func (a *App) handleAdminEvents(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	switch r.Method {
	case http.MethodPost:
		var in Event
		if json.NewDecoder(io.LimitReader(r.Body, 2*maxCover)).Decode(&in) != nil { // covers are bigger than photos
			writeJSON(w, http.StatusBadRequest, errBody("invalid event"))
			return
		}
		in.Title, in.City = clip(in.Title, 100), clip(in.City, 60)
		if in.Title == "" || in.City == "" || in.StartsAt.IsZero() {
			writeJSON(w, http.StatusBadRequest, errBody("title, city and start time are required"))
			return
		}
		if in.EndsAt != nil && !in.EndsAt.After(in.StartsAt) {
			writeJSON(w, http.StatusBadRequest, errBody("the end must be after the start"))
			return
		}
		if in.Cover != "" && in.Cover != "keep" && (!validPhoto(in.Cover) || len(in.Cover) > maxCover) {
			writeJSON(w, http.StatusBadRequest, errBody("cover must be a JPEG, PNG or WebP under 1 MB"))
			return
		}
		if in.Link != "" && safeLink(in.Link) == "" {
			writeJSON(w, http.StatusBadRequest, errBody("link must be https://"))
			return
		}
		args := []any{in.Title, clip(in.Description, 4000), in.City, clip(in.Venue, 100), clip(in.Address, 200), in.Lat, in.Lng,
			in.StartsAt, in.EndsAt, max(in.Capacity, 0), clip(in.Host, 80), in.Link, in.Published}
		var err error
		if in.ID > 0 {
			args = append(args, in.ID, in.Cover)
			_, err = a.store.pool.Exec(ctx, `UPDATE events SET title = $1, description = $2, city = $3, venue = $4, address = $5, lat = $6, lng = $7,
				starts_at = $8, ends_at = $9, capacity = $10, host = $11, link = $12, published = $13,
				cover = CASE WHEN $15 = 'keep' THEN cover ELSE $15 END, updated_at = now() WHERE id = $14`, args...)
		} else {
			if in.Cover == "keep" {
				in.Cover = ""
			}
			args = append(args, in.Cover)
			err = a.store.pool.QueryRow(ctx, `INSERT INTO events (title, description, city, venue, address, lat, lng, starts_at, ends_at,
				capacity, host, link, published, cover) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14) RETURNING id`, args...).Scan(&in.ID)
		}
		if err != nil {
			log.Printf("event save: %v", err)
			writeJSON(w, http.StatusServiceUnavailable, errBody("save failed"))
			return
		}
		if err := a.reloadEventVenues(ctx); err != nil {
			log.Printf("event venues: %v", err)
		}
		e, _ := a.event(ctx, in.ID, "", true)
		writeJSON(w, http.StatusOK, e)
		return
	case http.MethodDelete:
		id, _ := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
		if _, err := a.store.pool.Exec(ctx, `DELETE FROM events WHERE id = $1`, id); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, errBody("delete failed"))
			return
		}
		_ = a.reloadEventVenues(ctx)
	}
	rows, err := a.store.pool.Query(ctx, `SELECT `+eventCols+`, false FROM events e ORDER BY e.starts_at DESC LIMIT 200`)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, errBody("unavailable"))
		return
	}
	evs, err := a.scanEvents(rows)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, errBody("unavailable"))
		return
	}
	here := a.hub.geo.eventsInside()
	for i := range evs {
		evs[i].Here = here[evs[i].ID]
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": evs})
}

// GET /admin/events/attendees?id=: who registered (admin view: names and emails, no positions).
func (a *App) handleAdminAttendees(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
	rows, err := a.store.pool.Query(r.Context(), `SELECT u.id, u.email, coalesce(p.name, ''), coalesce(p.role, ''), u.city, r.created_at
		FROM event_rsvps r JOIN users u ON u.id = r.uid LEFT JOIN profiles p ON p.uid = u.id
		WHERE r.event_id = $1 ORDER BY r.created_at DESC`, id)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, errBody("unavailable"))
		return
	}
	defer rows.Close()
	type att struct {
		UID   string    `json:"uid"`
		Email string    `json:"email"`
		Name  string    `json:"name"`
		Role  string    `json:"role"`
		City  string    `json:"city"`
		At    time.Time `json:"at"`
	}
	out := []att{}
	for rows.Next() {
		var x att
		if rows.Scan(&x.UID, &x.Email, &x.Name, &x.Role, &x.City, &x.At) == nil {
			out = append(out, x)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"attendees": out})
}

// placeMessages sends a place's message to each person the hub finds inside it while it's
// live: once per person per message (place_sends, claimed before sending), most recent first.
func (a *App) placeMessages(ctx context.Context) {
	tidy := time.NewTicker(10 * time.Minute)
	defer tidy.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tidy.C:
			c, cancel := context.WithTimeout(ctx, 10*time.Second)
			_, _ = a.store.pool.Exec(c, `DELETE FROM place_sends WHERE expires_at < now()`)
			_, _ = a.store.pool.Exec(c, `DELETE FROM event_arrivals WHERE expires_at < now()`)
			if err := a.reloadEventVenues(c); err != nil { // windows open and close as time passes
				log.Printf("event venues: %v", err)
			}
			cancel()
		case ar := <-a.hub.geo.arrive:
			c, cancel := context.WithTimeout(ctx, 30*time.Second)
			if ar.event != 0 {
				a.sendEventArrival(c, ar)
			} else {
				a.sendPlaceMsg(c, ar)
			}
			cancel()
		}
	}
}

func (a *App) sendPlaceMsg(ctx context.Context, ar arrival) {
	tag, err := a.store.pool.Exec(ctx, `INSERT INTO place_sends (place_id, uid, rev, expires_at) VALUES ($1, $2, $3, $4)
		ON CONFLICT DO NOTHING`, ar.place, ar.uid, ar.msg.Rev, ar.msg.Until)
	if err != nil || tag.RowsAffected() == 0 {
		return // already had it (or the account just went)
	}
	name := ""
	if p, ok, _ := a.store.getProfile(ctx, ar.uid); ok {
		name = p.Name
	}
	m := ar.msg
	title, body := personal(m.Title, name), personal(m.Body, name)
	if m.Push {
		var data any
		if m.CTAURL != "" {
			data = map[string]string{"link": m.CTAURL}
		}
		a.notifyUser(ar.uid, title, body, data)
	}
	if m.InApp {
		if err := a.sendInApp(ctx, ar.uid, 0, title, body, m.CTALabel, m.CTAURL); err != nil {
			log.Printf("place message %d: %v", ar.place, err)
		}
	}
	_, _ = a.store.pool.Exec(ctx, `UPDATE places SET msg_sent = msg_sent + 1 WHERE id = $1`, ar.place)
}
