package main

import (
	"context"
	"encoding/json"
	"errors"
	"html"
	"log"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

// Kinjo Control: the admin control room's data (installs, in-app inbox, website counts,
// campaigns with audiences, places, events, settings). Positions stay in memory: the
// audience's "in this area" is the live hub, never a stored location.
const controlSchema = `;
-- the city a person joined from: nearest known city to their first fix (cities.go). Never the fix.
ALTER TABLE users ADD COLUMN IF NOT EXISTS city text NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN IF NOT EXISTS country text NOT NULL DEFAULT '';
-- "finish your card" reminders sent (only ever to incomplete cards)
ALTER TABLE users ADD COLUMN IF NOT EXISTS reminders int NOT NULL DEFAULT 0;
ALTER TABLE users ADD COLUMN IF NOT EXISTS reminded_at timestamptz;
-- installs: one row per app install (its own random id), signed in or not. Downloads = rows.
CREATE TABLE IF NOT EXISTS installs (
  device_id text PRIMARY KEY,
  platform  text NOT NULL DEFAULT '',
  model     text NOT NULL DEFAULT '',
  os        text NOT NULL DEFAULT '',
  version   text NOT NULL DEFAULT '',
  uid       text REFERENCES users(id) ON DELETE SET NULL,
  first_at  timestamptz NOT NULL DEFAULT now(),
  last_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS installs_first_idx ON installs (first_at);
-- in-app messages: shown inside the app (a card over the deck + the inbox), not a push
CREATE TABLE IF NOT EXISTS inbox (
  id          bigserial PRIMARY KEY,
  uid         text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  campaign_id bigint,
  title       text NOT NULL,
  body        text NOT NULL DEFAULT '',
  cta_label   text NOT NULL DEFAULT '',
  cta_url     text NOT NULL DEFAULT '',
  created_at  timestamptz NOT NULL DEFAULT now(),
  read_at     timestamptz
);
CREATE INDEX IF NOT EXISTS inbox_uid_idx ON inbox (uid, created_at DESC);
CREATE TABLE IF NOT EXISTS campaigns (
  id         bigserial PRIMARY KEY,
  name       text NOT NULL DEFAULT '',
  channels   text[] NOT NULL,
  audience   jsonb NOT NULL,
  content    jsonb NOT NULL,
  status     text NOT NULL DEFAULT 'sending',
  targeted   int NOT NULL DEFAULT 0,
  priority   int NOT NULL DEFAULT 0,
  pushed     int NOT NULL DEFAULT 0,
  inapp      int NOT NULL DEFAULT 0,
  emailed    int NOT NULL DEFAULT 0,
  failed     int NOT NULL DEFAULT 0,
  created_at timestamptz NOT NULL DEFAULT now(),
  done_at    timestamptz
);
-- website: counts per day only (no IPs, no cookies)
CREATE TABLE IF NOT EXISTS site_hits (
  day    date NOT NULL,
  kind   text NOT NULL,
  path   text NOT NULL,
  target text NOT NULL DEFAULT '',
  n      int NOT NULL DEFAULT 0,
  PRIMARY KEY (day, kind, path, target)
);
-- places: admin circles with live visit counts. Only per-day counts are stored (places.go)
CREATE TABLE IF NOT EXISTS places (
  id         bigserial PRIMARY KEY,
  name       text NOT NULL,
  lat        double precision NOT NULL,
  lng        double precision NOT NULL,
  radius_m   int NOT NULL,
  active     boolean NOT NULL DEFAULT true,
  created_at timestamptz NOT NULL DEFAULT now()
);
-- a place's arrival message (PlaceMsg) and how many got it
ALTER TABLE places ADD COLUMN IF NOT EXISTS msg jsonb;
ALTER TABLE places ADD COLUMN IF NOT EXISTS msg_sent int NOT NULL DEFAULT 0;
-- who already got a place's message (once each). Says they were there, so rows go when
-- the message's window ends (expires_at), and with the place.
CREATE TABLE IF NOT EXISTS place_sends (
  place_id   bigint NOT NULL REFERENCES places(id) ON DELETE CASCADE,
  uid        text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  rev        bigint NOT NULL,
  expires_at timestamptz NOT NULL,
  PRIMARY KEY (place_id, uid, rev)
);
CREATE TABLE IF NOT EXISTS place_days (
  place_id bigint NOT NULL REFERENCES places(id) ON DELETE CASCADE,
  day      date NOT NULL,
  visitors int NOT NULL DEFAULT 0,
  peak     int NOT NULL DEFAULT 0,
  entries  int NOT NULL DEFAULT 0,
  PRIMARY KEY (place_id, day)
);
-- events in a city (Luma-style). Registering shows your card to the other people going.
CREATE TABLE IF NOT EXISTS events (
  id          bigserial PRIMARY KEY,
  title       text NOT NULL,
  description text NOT NULL DEFAULT '',
  city        text NOT NULL,
  venue       text NOT NULL DEFAULT '',
  address     text NOT NULL DEFAULT '',
  lat         double precision,
  lng         double precision,
  starts_at   timestamptz NOT NULL,
  ends_at     timestamptz,
  cover       text NOT NULL DEFAULT '',
  capacity    int NOT NULL DEFAULT 0,
  host        text NOT NULL DEFAULT '',
  link        text NOT NULL DEFAULT '',
  published   boolean NOT NULL DEFAULT true,
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS events_city_idx ON events (lower(city), starts_at);
CREATE TABLE IF NOT EXISTS event_rsvps (
  event_id   bigint NOT NULL REFERENCES events(id) ON DELETE CASCADE,
  uid        text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (event_id, uid)
);
CREATE INDEX IF NOT EXISTS event_rsvps_uid_idx ON event_rsvps (uid);
CREATE TABLE IF NOT EXISTS settings (
  key   text PRIMARY KEY,
  value jsonb NOT NULL
)`

/* ---------------- installs (downloads) ---------------- */

func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

// POST /api/install {deviceId, platform, model, os, version}: public (installs count before
// sign-in too); a Bearer token, if any, ties the install to the account.
func (a *App) handleInstall(w http.ResponseWriter, r *http.Request) {
	var in struct {
		DeviceID, Platform, Model, OS, Version string
	}
	if decodeJSON(r, &in) != nil || len(in.DeviceID) < 8 || len(in.DeviceID) > 64 {
		writeJSON(w, http.StatusBadRequest, errBody("deviceId required"))
		return
	}
	switch in.Platform {
	case "android", "ios", "web":
	default:
		in.Platform = "other"
	}
	uid, _ := a.uidFromRequest(r)
	var u any
	if uid != "" {
		u = uid
	}
	_, err := a.store.pool.Exec(r.Context(), `
		INSERT INTO installs (device_id, platform, model, os, version, uid) VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (device_id) DO UPDATE SET platform = EXCLUDED.platform,
		  model = CASE WHEN EXCLUDED.model <> '' THEN EXCLUDED.model ELSE installs.model END,
		  os = CASE WHEN EXCLUDED.os <> '' THEN EXCLUDED.os ELSE installs.os END,
		  version = CASE WHEN EXCLUDED.version <> '' THEN EXCLUDED.version ELSE installs.version END,
		  uid = coalesce(EXCLUDED.uid, installs.uid), last_at = now()`,
		in.DeviceID, in.Platform, clip(in.Model, 60), clip(in.OS, 40), clip(in.Version, 20), u)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, errBody("unavailable"))
		return
	}
	if uid != "" { // the real install replaces the stand-in backfillInstalls made for this account
		_, _ = a.store.pool.Exec(r.Context(), `DELETE FROM installs WHERE device_id = $1`, "u:"+uid)
	}
	w.WriteHeader(http.StatusNoContent)
}

// backfillInstalls counts the phones from before the app reported installs (builds up to
// 2.3.4 never called /api/install, so Downloads read 0): every push-registered phone (its
// own install id, so the app's later report lands on the same row), then a stand-in
// "u:<uid>" for anyone who opened the app with no phone row. Dated at sign-up. Idempotent,
// run on boot.
func (a *App) backfillInstalls(ctx context.Context) error {
	_, err := a.store.pool.Exec(ctx, `
		INSERT INTO installs (device_id, platform, uid, first_at, last_at)
		SELECT DISTINCT ON (d.device_id) d.device_id, d.platform, d.uid, u.created_at, d.updated_at
		FROM devices d JOIN users u ON u.id = d.uid
		WHERE coalesce(d.device_id, '') <> '' AND d.platform IN ('android', 'ios')
		ORDER BY d.device_id, d.updated_at DESC
		ON CONFLICT (device_id) DO NOTHING`)
	if err != nil {
		return err
	}
	_, err = a.store.pool.Exec(ctx, `
		INSERT INTO installs (device_id, platform, uid, first_at, last_at)
		SELECT 'u:' || u.id,
		       coalesce((SELECT o.platform FROM app_opens o WHERE o.uid = u.id ORDER BY o.day DESC LIMIT 1), 'other'),
		       u.id, u.created_at, greatest(u.created_at, (SELECT max(o.day) FROM app_opens o WHERE o.uid = u.id)::timestamptz)
		FROM users u
		WHERE EXISTS (SELECT 1 FROM app_opens o WHERE o.uid = u.id)
		  AND NOT EXISTS (SELECT 1 FROM installs i WHERE i.uid = u.id)
		ON CONFLICT (device_id) DO NOTHING`)
	return err
}

/* ---------------- in-app inbox ---------------- */

type InboxItem struct {
	ID       int64      `json:"id"`
	Title    string     `json:"title"`
	Body     string     `json:"body"`
	CTALabel string     `json:"ctaLabel"`
	CTAURL   string     `json:"ctaUrl"`
	Created  time.Time  `json:"createdAt"`
	Read     *time.Time `json:"readAt"`
}

// safeLink: only https and kinjo:// links go into messages (no javascript:, no plain http).
func safeLink(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "https" && u.Scheme != "kinjo") || (u.Scheme == "https" && u.Host == "") {
		return ""
	}
	return s
}

func (a *App) handleInbox(w http.ResponseWriter, r *http.Request, uid string) {
	rows, err := a.store.pool.Query(r.Context(), `
		SELECT id, title, body, cta_label, cta_url, created_at, read_at FROM inbox
		WHERE uid = $1 ORDER BY created_at DESC LIMIT 50`, uid)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, errBody("unavailable"))
		return
	}
	defer rows.Close()
	items, unread := []InboxItem{}, 0
	for rows.Next() {
		var it InboxItem
		if err := rows.Scan(&it.ID, &it.Title, &it.Body, &it.CTALabel, &it.CTAURL, &it.Created, &it.Read); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, errBody("unavailable"))
			return
		}
		if it.Read == nil {
			unread++
		}
		items = append(items, it)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "unread": unread})
}

// POST /api/inbox/read {id} or {all:true}
func (a *App) handleInboxRead(w http.ResponseWriter, r *http.Request, uid string) {
	var in struct {
		ID  int64 `json:"id"`
		All bool  `json:"all"`
	}
	_ = decodeJSON(r, &in)
	var err error
	if in.All {
		_, err = a.store.pool.Exec(r.Context(), `UPDATE inbox SET read_at = now() WHERE uid = $1 AND read_at IS NULL`, uid)
	} else {
		_, err = a.store.pool.Exec(r.Context(), `UPDATE inbox SET read_at = now() WHERE uid = $1 AND id = $2 AND read_at IS NULL`, uid, in.ID)
	}
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, errBody("unavailable"))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// sendInApp stores an in-app message and shows it at once in any open app.
func (a *App) sendInApp(ctx context.Context, uid string, campaign int64, title, body, ctaLabel, ctaURL string) error {
	var id int64
	var c any
	if campaign > 0 {
		c = campaign
	}
	err := a.store.pool.QueryRow(ctx, `INSERT INTO inbox (uid, campaign_id, title, body, cta_label, cta_url)
		VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`, uid, c, title, body, ctaLabel, ctaURL).Scan(&id)
	if err != nil {
		return err
	}
	a.hub.pushNotification(uid, map[string]any{"type": "inbox",
		"item": InboxItem{ID: id, Title: title, Body: body, CTALabel: ctaLabel, CTAURL: ctaURL, Created: time.Now()}})
	return nil
}

/* ---------------- website counts ---------------- */

type siteKey struct{ kind, path, target string }

// siteCounter batches website hits in memory and writes them every 10 s: one row per
// (day, kind, path, target), so a busy page is one UPDATE, not one per view.
type siteCounter struct {
	mu sync.Mutex
	n  map[siteKey]int
}

var site = &siteCounter{n: map[siteKey]int{}}

func (c *siteCounter) add(k siteKey) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.n) > 5000 { // a flood of made-up paths: drop until the next flush
		return false
	}
	c.n[k]++
	return true
}

func (c *siteCounter) flush(ctx context.Context, s *Store) {
	c.mu.Lock()
	batch := c.n
	c.n = map[siteKey]int{}
	c.mu.Unlock()
	for k, n := range batch {
		_, err := s.pool.Exec(ctx, `INSERT INTO site_hits (day, kind, path, target, n) VALUES (`+today+`, $1, $2, $3, $4)
			ON CONFLICT (day, kind, path, target) DO UPDATE SET n = site_hits.n + EXCLUDED.n`, k.kind, k.path, k.target, n)
		if err != nil {
			log.Printf("site hits: %v", err)
			return
		}
	}
}

func (c *siteCounter) run(ctx context.Context, s *Store) {
	t := time.NewTicker(10 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			c2, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			c.flush(c2, s)
			cancel()
			return
		case <-t.C:
			c.flush(ctx, s)
		}
	}
}

var siteKinds = map[string]bool{"view": true, "click": true, "visit": true, "download": true}

// POST /api/site/hit {kind: view|click|visit|download, path, target}: public, counts only.
// "visit" is sent once per browser per day (the site keeps that flag), so it counts visitors.
func handleSiteHit(w http.ResponseWriter, r *http.Request) {
	var in struct{ Kind, Path, Target string }
	if decodeJSON(r, &in) != nil || !siteKinds[in.Kind] {
		writeJSON(w, http.StatusBadRequest, errBody("invalid"))
		return
	}
	p := clip(in.Path, 80)
	if !strings.HasPrefix(p, "/") {
		p = "/"
	}
	if i := strings.IndexAny(p, "?#"); i >= 0 {
		p = p[:i]
	}
	site.add(siteKey{in.Kind, p, clip(in.Target, 60)})
	w.WriteHeader(http.StatusNoContent)
}

/* ---------------- settings ---------------- */

type ReminderSettings struct {
	Enabled     bool `json:"enabled"`
	FirstHours  int  `json:"firstHours"`  // after sign-up
	SecondHours int  `json:"secondHours"` // after sign-up; 0 = only one reminder
}

// Reminders are a push (+ the in-app notification row) only, never an email.
var defaultReminders = ReminderSettings{Enabled: true, FirstHours: 24, SecondHours: 72}

func (s *Store) getSetting(ctx context.Context, key string, v any) error {
	var raw []byte
	err := s.pool.QueryRow(ctx, `SELECT value FROM settings WHERE key = $1`, key).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil // caller's defaults stand
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, v)
}

func (s *Store) putSetting(ctx context.Context, key string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `INSERT INTO settings (key, value) VALUES ($1, $2)
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value`, key, b)
	return err
}

func (a *App) handleAdminSettings(w http.ResponseWriter, r *http.Request) {
	rem := defaultReminders
	if r.Method == http.MethodPut {
		var in struct {
			Reminders ReminderSettings `json:"reminders"`
		}
		if decodeJSON(r, &in) != nil || in.Reminders.FirstHours < 1 || in.Reminders.FirstHours > 24*30 ||
			in.Reminders.SecondHours < 0 || (in.Reminders.SecondHours != 0 && in.Reminders.SecondHours <= in.Reminders.FirstHours) {
			writeJSON(w, http.StatusBadRequest, errBody("first reminder 1 h to 30 days; the second (optional) after the first"))
			return
		}
		if err := a.store.putSetting(r.Context(), "reminders", in.Reminders); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, errBody("save failed"))
			return
		}
	}
	if err := a.store.getSetting(r.Context(), "reminders", &rem); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, errBody("unavailable"))
		return
	}
	var pending, sent int
	_ = a.store.pool.QueryRow(r.Context(), `SELECT
		count(*) FILTER (WHERE NOT `+completeSQL+`), coalesce(sum(u.reminders), 0)
		FROM users u LEFT JOIN profiles p ON p.uid = u.id`).Scan(&pending, &sent)
	writeJSON(w, http.StatusOK, map[string]any{"reminders": rem, "incomplete": pending, "remindersSent": sent,
		"smtp": a.cfg.smtpConfigured(), "fcm": a.fcm != nil})
}

/* ---------------- "finish your card" reminders ---------------- */

const completeSQL = `(coalesce(p.name,'') <> '' AND coalesce(p.role,'') <> '' AND coalesce(p.look,'') <> '' AND coalesce(p.photo,'') <> '')`

// remindIncomplete: people whose card isn't finished get a push, at most twice
// (FirstHours and SecondHours after signing up). Complete cards never get one. Each row is
// claimed (reminders+1) before sending, so two servers can't both send it.
func (a *App) remindIncomplete(ctx context.Context) (int, error) {
	rem := defaultReminders
	if err := a.store.getSetting(ctx, "reminders", &rem); err != nil {
		return 0, err
	}
	if !rem.Enabled {
		return 0, nil
	}
	second := rem.SecondHours
	if second == 0 {
		second = -1
	}
	rows, err := a.store.pool.Query(ctx, `
		UPDATE users u SET reminders = u.reminders + 1, reminded_at = now()
		FROM users x LEFT JOIN profiles p ON p.uid = x.id
		WHERE u.id = x.id AND NOT `+completeSQL+`
		  AND ((u.reminders = 0 AND u.created_at < now() - make_interval(hours => $1))
		    OR ($2 > 0 AND u.reminders = 1 AND u.created_at < now() - make_interval(hours => $2)))
		  AND u.created_at > now() - interval '60 days'
		RETURNING u.id, u.reminders`, rem.FirstHours, second)
	if err != nil {
		return 0, err
	}
	type target struct {
		uid string
		n   int
	}
	var ts []target
	for rows.Next() {
		var t target
		if err := rows.Scan(&t.uid, &t.n); err != nil {
			rows.Close()
			return 0, err
		}
		ts = append(ts, t)
	}
	rows.Close()
	for _, t := range ts {
		title, body := "Your card isn’t finished", "Add your photo and what you do, so the people within 30 m can find you."
		if t.n > 1 {
			title, body = "People nearby can’t see you yet", "Finish your Kinjo card in a minute: a photo, what you do and what you’re looking for."
		}
		a.notifyUser(t.uid, title, body, map[string]string{"kind": "profile"})
	}
	return len(ts), nil
}

func (a *App) reminderLoop(ctx context.Context) {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			c, cancel := context.WithTimeout(ctx, 5*time.Minute)
			if n, err := a.remindIncomplete(c); err != nil {
				log.Printf("reminders: %v", err)
			} else if n > 0 {
				log.Printf("reminders: %d sent", n)
			}
			cancel()
		}
	}
}

func (a *App) handleAdminRemindNow(w http.ResponseWriter, r *http.Request) {
	n, err := a.remindIncomplete(r.Context())
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, errBody("reminders failed"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"sent": n})
}

/* ---------------- audiences ---------------- */

// Area: a circle on the map. Live = everyone the server places inside it right now
// (memory, never stored). Home adds people who joined from a city whose centre is inside.
type Area struct {
	Lat    float64 `json:"lat"`
	Lng    float64 `json:"lng"`
	Radius float64 `json:"radius"`
	Home   bool    `json:"home"`
}

type Audience struct {
	Group      string   `json:"group"`      // all | active | inactive | incomplete | complete | online | event
	ActiveDays int      `json:"activeDays"` // the window for active/inactive (default 7)
	Platform   string   `json:"platform"`   // android | ios | web
	City       string   `json:"city"`
	Area       *Area    `json:"area"`
	Event      int64    `json:"event"` // group "event": people registered for it
	UIDs       []string `json:"uids"`
}

type audienceMember struct {
	UID      string `json:"uid"`
	Name     string `json:"name"`
	Email    string `json:"-"`
	Verified bool   `json:"-"`
	City     string `json:"city"`
	Rank     int    `json:"rank"` // 0 app open now, 1 inside the area now, 2 active in the window, 3 inactive
	Push     bool   `json:"push"`
	Complete bool   `json:"complete"`
}

const (
	rankOnline = iota
	rankHere
	rankActive
	rankInactive
)

// resolve finds an audience, most active first (the priority queue: people with the app
// open, then people in the area now, then recently active, then everyone else).
func (a *App) resolve(ctx context.Context, au Audience, limit int) ([]audienceMember, error) {
	days := au.ActiveDays
	if days <= 0 || days > 365 {
		days = 7
	}
	online := a.hub.onlineUIDs()
	var here []string
	areaOn := au.Area != nil && au.Area.Radius > 0
	if areaOn {
		r := min(max(au.Area.Radius, 10), 500000)
		for _, p := range a.hub.livePoints(time.Now()) {
			if haversineM(p.lat, p.lng, au.Area.Lat, au.Area.Lng) <= r {
				here = append(here, p.uid)
			}
		}
	}
	var homeCities []string
	if areaOn && au.Area.Home {
		homeCities = citiesWithin(au.Area.Lat, au.Area.Lng, au.Area.Radius)
	}
	if limit <= 0 {
		limit = 1000000
	}
	rows, err := a.store.pool.Query(ctx, `
		WITH m AS (
		  SELECT u.id, coalesce(p.name, '') AS name, u.email, u.email_verified, u.city,
		         `+completeSQL+` AS complete,
		         EXISTS (SELECT 1 FROM app_opens o WHERE o.uid = u.id AND o.day > `+today+` - $1::int) AS active,
		         EXISTS (SELECT 1 FROM devices d WHERE d.uid = u.id AND d.notify AND d.platform <> 'ioslocation') AS push
		  FROM users u LEFT JOIN profiles p ON p.uid = u.id
		  WHERE ($2 = '' OR EXISTS (SELECT 1 FROM devices d WHERE d.uid = u.id AND d.platform = $2)
		                 OR EXISTS (SELECT 1 FROM app_opens o WHERE o.uid = u.id AND o.platform = $2))
		    AND ($3 = '' OR lower(u.city) = lower($3))
		    AND (NOT $4 OR u.id = ANY($5) OR u.city = ANY($6))
		    AND (coalesce(cardinality($7::text[]), 0) = 0 OR u.id = ANY($7))
		    AND ($8::bigint = 0 OR EXISTS (SELECT 1 FROM event_rsvps r WHERE r.event_id = $8 AND r.uid = u.id))
		)
		SELECT id, name, email, email_verified, city, complete, active, push FROM m
		WHERE CASE $9
		  WHEN 'active' THEN active
		  WHEN 'inactive' THEN NOT active
		  WHEN 'incomplete' THEN NOT complete
		  WHEN 'complete' THEN complete
		  WHEN 'online' THEN id = ANY($10)
		  ELSE true END
		LIMIT $11`,
		days, au.Platform, strings.TrimSpace(au.City), areaOn, here, homeCities, au.UIDs, au.Event, au.Group, online, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	isOnline, isHere := set(online), set(here)
	var out []audienceMember
	for rows.Next() {
		var m audienceMember
		var active bool
		if err := rows.Scan(&m.UID, &m.Name, &m.Email, &m.Verified, &m.City, &m.Complete, &active, &m.Push); err != nil {
			return nil, err
		}
		switch {
		case isOnline[m.UID]:
			m.Rank = rankOnline
		case isHere[m.UID]:
			m.Rank = rankHere
		case active:
			m.Rank = rankActive
		default:
			m.Rank = rankInactive
		}
		out = append(out, m)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Rank < out[j].Rank })
	return out, rows.Err()
}

func set(xs []string) map[string]bool {
	m := make(map[string]bool, len(xs))
	for _, x := range xs {
		m[x] = true
	}
	return m
}

// POST /admin/audience: how many people an audience reaches, by priority and channel.
func (a *App) handleAdminAudience(w http.ResponseWriter, r *http.Request) {
	var au Audience
	if decodeJSON(r, &au) != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid audience"))
		return
	}
	ms, err := a.resolve(r.Context(), au, 0)
	if err != nil {
		log.Printf("audience: %v", err)
		writeJSON(w, http.StatusServiceUnavailable, errBody("audience failed"))
		return
	}
	out := map[string]any{"total": len(ms)}
	var ranks [4]int
	push, email, incomplete := 0, 0, 0
	cities := map[string]int{}
	for _, m := range ms {
		ranks[m.Rank]++
		if m.Push {
			push++
		}
		if m.Email != "" && m.Verified {
			email++
		}
		if !m.Complete {
			incomplete++
		}
		if m.City != "" {
			cities[m.City]++
		}
	}
	sample := ms
	if len(sample) > 8 {
		sample = sample[:8]
	}
	out["online"], out["here"], out["active"], out["inactive"] = ranks[0], ranks[1], ranks[2], ranks[3]
	out["push"], out["email"], out["incomplete"], out["sample"], out["cities"] = push, email, incomplete, sample, topN(cities, 6)
	writeJSON(w, http.StatusOK, out)
}

type kv struct {
	Key string `json:"key"`
	N   int    `json:"n"`
}

func topN(m map[string]int, n int) []kv {
	out := make([]kv, 0, len(m))
	for k, v := range m {
		out = append(out, kv{k, v})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].N > out[j].N || (out[i].N == out[j].N && out[i].Key < out[j].Key) })
	if len(out) > n {
		out = out[:n]
	}
	return out
}

/* ---------------- campaigns ---------------- */

type CampaignContent struct {
	Title    string `json:"title"`
	Body     string `json:"body"`
	CTALabel string `json:"ctaLabel"`
	CTAURL   string `json:"ctaUrl"`
	Subject  string `json:"subject"` // email (defaults to the title)
}

type Campaign struct {
	ID       int64           `json:"id"`
	Name     string          `json:"name"`
	Channels []string        `json:"channels"`
	Audience json.RawMessage `json:"audience"`
	Content  CampaignContent `json:"content"`
	Status   string          `json:"status"`
	Targeted int             `json:"targeted"`
	Priority int             `json:"priority"` // of them, active ones (sent first)
	Pushed   int             `json:"pushed"`
	InApp    int             `json:"inapp"`
	Emailed  int             `json:"emailed"`
	Failed   int             `json:"failed"`
	Read     int             `json:"read"` // in-app messages opened
	Created  time.Time       `json:"createdAt"`
	Done     *time.Time      `json:"doneAt"`
}

func (a *App) handleAdminCampaigns(w http.ResponseWriter, r *http.Request) {
	rows, err := a.store.pool.Query(r.Context(), `
		SELECT c.id, c.name, c.channels, c.audience, c.content, c.status, c.targeted, c.priority, c.pushed, c.inapp, c.emailed, c.failed,
		       (SELECT count(*) FROM inbox i WHERE i.campaign_id = c.id AND i.read_at IS NOT NULL), c.created_at, c.done_at
		FROM campaigns c ORDER BY c.created_at DESC LIMIT 100`)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, errBody("unavailable"))
		return
	}
	defer rows.Close()
	out := []Campaign{}
	for rows.Next() {
		var c Campaign
		var content []byte
		if err := rows.Scan(&c.ID, &c.Name, &c.Channels, &c.Audience, &content, &c.Status, &c.Targeted, &c.Priority,
			&c.Pushed, &c.InApp, &c.Emailed, &c.Failed, &c.Read, &c.Created, &c.Done); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, errBody("unavailable"))
			return
		}
		_ = json.Unmarshal(content, &c.Content)
		out = append(out, c)
	}
	writeJSON(w, http.StatusOK, map[string]any{"campaigns": out})
}

var channelOK = map[string]bool{"push": true, "inapp": true, "email": true}

// POST /admin/campaigns {name, channels, audience, content}: resolves the audience now and
// sends in the background, most active people first.
func (a *App) handleAdminSendCampaign(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name     string          `json:"name"`
		Channels []string        `json:"channels"`
		Audience json.RawMessage `json:"audience"`
		Content  CampaignContent `json:"content"`
	}
	if decodeJSON(r, &in) != nil || len(in.Channels) == 0 {
		writeJSON(w, http.StatusBadRequest, errBody("pick at least one channel"))
		return
	}
	for _, c := range in.Channels {
		if !channelOK[c] {
			writeJSON(w, http.StatusBadRequest, errBody("unknown channel "+c))
			return
		}
	}
	c := &in.Content
	c.Title, c.Body, c.CTALabel = clip(c.Title, 80), clip(c.Body, 600), clip(c.CTALabel, 30)
	c.Subject = clip(c.Subject, 120)
	if c.CTAURL != "" && safeLink(c.CTAURL) == "" {
		writeJSON(w, http.StatusBadRequest, errBody("button link must be https:// or kinjo://"))
		return
	}
	if c.Title == "" {
		writeJSON(w, http.StatusBadRequest, errBody("title required"))
		return
	}
	var au Audience
	if json.Unmarshal(in.Audience, &au) != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid audience"))
		return
	}
	ms, err := a.resolve(r.Context(), au, 0)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, errBody("audience failed"))
		return
	}
	prio := 0
	for _, m := range ms {
		if m.Rank < rankInactive {
			prio++
		}
	}
	content, _ := json.Marshal(c)
	var id int64
	if err := a.store.pool.QueryRow(r.Context(), `INSERT INTO campaigns (name, channels, audience, content, targeted, priority)
		VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`, clip(in.Name, 80), in.Channels, in.Audience, content, len(ms), prio).Scan(&id); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, errBody("save failed"))
		return
	}
	go a.runCampaign(id, in.Channels, *c, ms)
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "targeted": len(ms), "priority": prio})
}

// runCampaign delivers in rank order: the active tiers go out first, all at once (8 at a
// time), then the inactive tail. Counters are written as it goes, so the report is live.
func (a *App) runCampaign(id int64, channels []string, c CampaignContent, ms []audienceMember) {
	ch := set(channels)
	var pushed, inapp, emailed, failed atomic.Int64
	ctx := context.Background()
	save := func(status string) {
		c2, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		done := any(nil)
		if status != "sending" {
			done = time.Now()
		}
		_, _ = a.store.pool.Exec(c2, `UPDATE campaigns SET pushed = $2, inapp = $3, emailed = $4, failed = $5, status = $6,
			done_at = coalesce($7, done_at) WHERE id = $1`, id, pushed.Load(), inapp.Load(), emailed.Load(), failed.Load(), status, done)
	}
	subject := c.Subject
	if subject == "" {
		subject = c.Title
	}
	var data map[string]string
	if c.CTAURL != "" {
		data = map[string]string{"link": c.CTAURL}
	}
	emailHTML := func(name string) string {
		inner := mailP(strings.ReplaceAll(html.EscapeString(personal(c.Body, name)), "\n", "<br>"))
		if c.CTAURL != "" && strings.HasPrefix(c.CTAURL, "https://") {
			label := c.CTALabel
			if label == "" {
				label = "Open"
			}
			inner += mailButton(c.CTAURL, html.EscapeString(label))
		}
		return emailShell(html.EscapeString(personal(c.Title, name)), html.EscapeString(personal(c.Body, name)), inner)
	}
	sem, wg := make(chan struct{}, 8), sync.WaitGroup{}
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for i, m := range ms {
		select {
		case <-ticker.C:
			save("sending")
		default:
		}
		if i > 0 && m.Rank == rankInactive && ms[i-1].Rank != rankInactive {
			wg.Wait() // the priority tiers are all out before the inactive tail starts
		}
		sem <- struct{}{}
		wg.Add(1)
		go func(m audienceMember) {
			defer func() { <-sem; wg.Done() }()
			c3, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			title, body := personal(c.Title, m.Name), personal(c.Body, m.Name)
			if ch["push"] {
				nid, err := a.store.insertNotification(c3, m.UID, title, body, mustJSON(data))
				if err != nil {
					failed.Add(1)
				} else {
					a.deliver(notifTarget{id: nid, uid: m.UID}, title, body, mustJSON(data))
					pushed.Add(1)
				}
			}
			if ch["inapp"] {
				if err := a.sendInApp(c3, m.UID, id, title, body, c.CTALabel, c.CTAURL); err != nil {
					failed.Add(1)
				} else {
					inapp.Add(1)
				}
			}
			if ch["email"] && m.Email != "" && m.Verified {
				text := body
				if c.CTAURL != "" {
					text += "\r\n\r\n" + c.CTAURL
				}
				if err := a.sendMailMIME(m.Email, personal(subject, m.Name), text+"\r\n\r\n— The Kinjo Team", emailHTML(m.Name)); err != nil {
					failed.Add(1)
				} else {
					emailed.Add(1)
				}
			}
		}(m)
	}
	wg.Wait()
	save("sent")
	log.Printf("campaign %d: %d people (push %d, in-app %d, email %d, failed %d)", id, len(ms), pushed.Load(), inapp.Load(), emailed.Load(), failed.Load())
}

// personal fills {{first_name}} / {{name}} (with a fallback: {{first_name|there}}).
func personal(s, name string) string {
	if !strings.Contains(s, "{{") {
		return s
	}
	first := ""
	if f := strings.Fields(name); len(f) > 0 {
		first = f[0]
	}
	for _, tag := range []struct{ key, val string }{{"first_name", first}, {"name", strings.TrimSpace(name)}} {
		for {
			i := strings.Index(s, "{{"+tag.key)
			if i < 0 {
				break
			}
			j := strings.Index(s[i:], "}}")
			if j < 0 {
				break
			}
			inner := s[i+2 : i+j]
			v := tag.val
			if k := strings.IndexByte(inner, '|'); k >= 0 && v == "" {
				v = inner[k+1:]
			} else if inner != tag.key && !strings.HasPrefix(inner, tag.key+"|") {
				break
			}
			s = s[:i] + v + s[i+j+2:]
		}
	}
	return s
}

func mustJSON(v any) []byte {
	if v == nil {
		return nil
	}
	if m, ok := v.(map[string]string); ok && m == nil {
		return nil
	}
	b, _ := json.Marshal(v)
	return b
}

/* ---------------- admin: overview, people, live ---------------- */

func (a *App) handleAdminOverview(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	out := map[string]any{}
	var users, installs, installs7, incomplete, unplaced, events, rsvps int
	err := a.store.pool.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM users),
		(SELECT count(*) FROM installs),
		(SELECT count(*) FROM installs WHERE first_at > now() - interval '7 days'),
		(SELECT count(*) FROM users u LEFT JOIN profiles p ON p.uid = u.id WHERE NOT `+completeSQL+`),
		(SELECT count(*) FROM users WHERE city = ''),
		(SELECT count(*) FROM events WHERE published AND coalesce(ends_at, starts_at + interval '3 hours') > now()),
		(SELECT count(*) FROM event_rsvps r JOIN events e ON e.id = r.event_id WHERE coalesce(e.ends_at, e.starts_at + interval '3 hours') > now())`).
		Scan(&users, &installs, &installs7, &incomplete, &unplaced, &events, &rsvps)
	if err != nil {
		log.Printf("overview: %v", err)
		writeJSON(w, http.StatusServiceUnavailable, errBody("overview failed"))
		return
	}
	out["users"], out["installs"], out["installs7"], out["incomplete"], out["noCity"] = users, installs, installs7, incomplete, unplaced
	out["upcomingEvents"], out["rsvps"] = events, rsvps
	q := func(key, sql string, args ...any) {
		if err != nil {
			return
		}
		var rows pgx.Rows
		rows, err = a.store.pool.Query(ctx, sql, args...)
		if err != nil {
			return
		}
		defer rows.Close()
		list := []map[string]any{}
		for rows.Next() {
			var k string
			var n, m int
			if err = rows.Scan(&k, &n, &m); err != nil {
				return
			}
			list = append(list, map[string]any{"key": k, "n": n, "m": m})
		}
		out[key] = list
	}
	// downloads: by platform (n) with those signed in (m), top device models, OS versions, per day
	q("installPlatforms", `SELECT platform, count(*), count(uid) FROM installs GROUP BY 1 ORDER BY 2 DESC`)
	q("installModels", `SELECT CASE WHEN model = '' THEN 'Unknown' ELSE model END, count(*), count(*) FILTER (WHERE platform = 'ios') FROM installs GROUP BY 1 ORDER BY 2 DESC LIMIT 15`)
	q("installOS", `SELECT CASE WHEN os = '' THEN platform ELSE os END, count(*), 0 FROM installs GROUP BY 1 ORDER BY 2 DESC LIMIT 10`)
	q("installDays", `SELECT to_char(d::date, 'YYYY-MM-DD'), coalesce(count(i.*) FILTER (WHERE i.platform = 'android'), 0), coalesce(count(i.*) FILTER (WHERE i.platform = 'ios'), 0)
		FROM generate_series(`+today+` - 29, `+today+`, interval '1 day') d
		LEFT JOIN installs i ON (i.first_at AT TIME ZONE '`+usageTZ+`')::date = d::date GROUP BY d ORDER BY d`)
	// where people joined from (n) and how many of them were active this week (m)
	q("cities", `SELECT CASE WHEN u.city = '' THEN 'Not yet known' ELSE u.city END, count(*),
		count(*) FILTER (WHERE EXISTS (SELECT 1 FROM app_opens o WHERE o.uid = u.id AND o.day > `+today+` - 7))
		FROM users u GROUP BY 1 ORDER BY 2 DESC LIMIT 25`)
	q("citySignups", `SELECT u.city, count(*), 0 FROM users u WHERE u.city <> '' AND u.created_at > now() - interval '7 days' GROUP BY 1 ORDER BY 2 DESC LIMIT 10`)
	if err != nil {
		log.Printf("overview: %v", err)
		writeJSON(w, http.StatusServiceUnavailable, errBody("overview failed"))
		return
	}
	pts := a.hub.livePoints(time.Now())
	open := 0
	liveCity := map[string]int{}
	for _, p := range pts {
		if p.open {
			open++
		}
		if c, _ := nearestCity(p.lat, p.lng); c != "" {
			liveCity[c]++
		} else {
			liveCity["Elsewhere"]++
		}
	}
	out["online"], out["placed"], out["placedOpen"], out["liveCities"] = a.hub.online(), len(pts), open, topN(liveCity, 12)
	out["cityStates"] = cityState
	writeJSON(w, http.StatusOK, out)
}

// GET /admin/people?q=&filter=all|online|active|inactive|incomplete|complete|hidden&city=&platform=&offset=&limit=
func (a *App) handleAdminPeople(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	people, total, err := a.queryPeople(r.Context(), q, limit, max(offset, 0))
	if err != nil {
		log.Printf("people: %v", err)
		writeJSON(w, http.StatusServiceUnavailable, errBody("people failed"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"people": people, "total": total})
}

type adminPerson struct {
	UID       string    `json:"uid"`
	Email     string    `json:"email"`
	Verified  bool      `json:"verified"`
	CreatedAt time.Time `json:"createdAt"`
	City      string    `json:"city"`
	Country   string    `json:"country"`
	Hidden    bool      `json:"hidden"`
	Reminders int       `json:"reminders"`
	Name      string    `json:"name"`
	Role      string    `json:"role"`
	Look      string    `json:"look"`
	HasPhoto  bool      `json:"hasPhoto"`
	Complete  bool      `json:"complete"`
	Provider  string    `json:"provider"`
	LastOpen  *string   `json:"lastOpen"`
	Platform  string    `json:"platform"`
	Events    int       `json:"events"`
	Online    bool      `json:"online"`
}

// queryPeople is the People list (and its export): the same filters, newest first. limit 0 = all.
func (a *App) queryPeople(ctx context.Context, q url.Values, limit, offset int) ([]adminPerson, int, error) {
	like := "%" + strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(strings.TrimSpace(q.Get("q"))) + "%"
	online := a.hub.onlineUIDs()
	var lim any
	if limit > 0 {
		lim = limit
	}
	rows, err := a.store.pool.Query(ctx, `
		WITH m AS (
		  SELECT u.id, u.email, u.email_verified, u.created_at, u.city, coalesce(u.country, '') AS country, u.hidden, u.reminders,
		         coalesce(p.name, '') AS name, coalesce(p.role, '') AS role, coalesce(p.look, '') AS look,
		         coalesce(p.photo, '') <> '' AS has_photo, `+completeSQL+` AS complete,
		         `+providerSQL+` AS provider,
		         (SELECT max(day) FROM app_opens o WHERE o.uid = u.id) AS last_open,
		         coalesce((SELECT platform FROM app_opens o WHERE o.uid = u.id ORDER BY day DESC LIMIT 1), '') AS platform,
		         (SELECT count(*) FROM event_rsvps e WHERE e.uid = u.id) AS events
		  FROM users u LEFT JOIN profiles p ON p.uid = u.id
		  WHERE ($1 = '%%' OR u.email ILIKE $1 OR p.name ILIKE $1 OR u.id ILIKE $1 OR p.role ILIKE $1)
		    AND ($2 = '' OR lower(u.city) = lower($2))
		)
		SELECT id, email, email_verified, created_at, city, country, hidden, reminders, name, role, look, has_photo, complete, provider,
		       to_char(last_open, 'YYYY-MM-DD'), platform, events, count(*) OVER ()
		FROM m
		WHERE ($3 = '' OR platform = $3)
		  AND CASE $4
		    WHEN 'online' THEN id = ANY($5)
		    WHEN 'active' THEN last_open > `+today+` - 7
		    WHEN 'inactive' THEN last_open IS NULL OR last_open <= `+today+` - 7
		    WHEN 'incomplete' THEN NOT complete
		    WHEN 'complete' THEN complete
		    WHEN 'hidden' THEN hidden
		    ELSE true END
		ORDER BY created_at DESC LIMIT $6 OFFSET $7`,
		like, strings.TrimSpace(q.Get("city")), q.Get("platform"), q.Get("filter"), online, lim, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	on := set(online)
	people, total := []adminPerson{}, 0
	for rows.Next() {
		var p adminPerson
		if err := rows.Scan(&p.UID, &p.Email, &p.Verified, &p.CreatedAt, &p.City, &p.Country, &p.Hidden, &p.Reminders, &p.Name, &p.Role,
			&p.Look, &p.HasPhoto, &p.Complete, &p.Provider, &p.LastOpen, &p.Platform, &p.Events, &total); err != nil {
			return nil, 0, err
		}
		p.Online = on[p.UID]
		people = append(people, p)
	}
	return people, total, rows.Err()
}

// GET /admin/live?s=&w=&n=&e=&cell=: placed people as grid squares (never points). Squares
// with fewer than liveFloor people are left out, so nobody can be singled out.
// Squares are never finer than areaCell (~550 m) and show from one person up: the founder chose
// neighbourhood precision for the admin (2026-11-18). Never the fix itself, never stored.
const liveFloor = 1

func (a *App) handleAdminLive(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := func(k string, d float64) float64 {
		if v, err := strconv.ParseFloat(q.Get(k), 64); err == nil {
			return v
		}
		return d
	}
	s, wst, n, e := f("s", -90), f("w", -180), f("n", 90), f("e", 180)
	cell := min(max(f("cell", 0.01), areaCell), 5)
	type sq struct {
		Lat  float64 `json:"lat"`
		Lng  float64 `json:"lng"`
		N    int     `json:"n"`
		Open int     `json:"open"`
	}
	grid := map[[2]int64]*sq{}
	pts := a.hub.livePoints(time.Now())
	total, open, inView := len(pts), 0, 0
	for _, p := range pts {
		if p.open {
			open++
		}
		if p.lat < s || p.lat > n || p.lng < wst || p.lng > e {
			continue
		}
		inView++
		k := [2]int64{int64(math.Floor(p.lat / cell)), int64(math.Floor(p.lng / cell))}
		c := grid[k]
		if c == nil {
			c = &sq{Lat: (float64(k[0]) + .5) * cell, Lng: (float64(k[1]) + .5) * cell}
			grid[k] = c
		}
		c.N++
		if p.open {
			c.Open++
		}
	}
	cells, hidden := []sq{}, 0
	for _, c := range grid {
		if c.N < liveFloor {
			hidden += c.N
			continue
		}
		cells = append(cells, *c)
	}
	writeJSON(w, http.StatusOK, map[string]any{"cells": cells, "cell": cell, "total": total, "open": open, "inView": inView, "belowFloor": hidden, "floor": liveFloor})
}

// GET /admin/live/people?s=&w=&n=&e=: everyone placed right now in the view, each at
// neighbourhood precision (the centre of their ~550 m square) with their nearest city and
// state. For the live map's people layer and its list. Never the fix, never stored.
func (a *App) handleAdminLivePeople(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := func(k string, d float64) float64 {
		if v, err := strconv.ParseFloat(q.Get(k), 64); err == nil {
			return v
		}
		return d
	}
	s, wst, n, e := f("s", -90), f("w", -180), f("n", 90), f("e", 180)
	type person struct {
		UID      string  `json:"uid"`
		Name     string  `json:"name"`
		Role     string  `json:"role"`
		HasPhoto bool    `json:"hasPhoto"`
		Open     bool    `json:"open"`
		Lat      float64 `json:"lat"`
		Lng      float64 `json:"lng"`
		City     string  `json:"city"`
		State    string  `json:"state"`
		Country  string  `json:"country"`
	}
	var out []person
	var uids []string
	for _, p := range a.hub.livePoints(time.Now()) {
		if p.lat < s || p.lat > n || p.lng < wst || p.lng > e {
			continue
		}
		x := person{UID: p.uid, Open: p.open}
		x.Lat, x.Lng = snapArea(p.lat, p.lng)
		x.City, x.Country = nearestCity(p.lat, p.lng)
		x.State = cityState[x.City]
		out = append(out, x)
		uids = append(uids, p.uid)
		if len(out) >= 2000 {
			break
		}
	}
	if len(uids) > 0 {
		rows, err := a.store.pool.Query(r.Context(), `SELECT uid, coalesce(name, ''), coalesce(role, ''), coalesce(photo, '') <> '' FROM profiles WHERE uid = ANY($1)`, uids)
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, errBody("unavailable"))
			return
		}
		prof := map[string]person{}
		for rows.Next() {
			var p person
			if rows.Scan(&p.UID, &p.Name, &p.Role, &p.HasPhoto) == nil {
				prof[p.UID] = p
			}
		}
		rows.Close()
		for i := range out {
			p := prof[out[i].UID]
			out[i].Name, out[i].Role, out[i].HasPhoto = p.Name, p.Role, p.HasPhoto
		}
	}
	if out == nil {
		out = []person{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"people": out, "precisionM": 550})
}

// GET /admin/live/area?lat=&lng=&r=: how many people are inside a circle right now.
func (a *App) handleAdminLiveArea(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	lat, e1 := strconv.ParseFloat(q.Get("lat"), 64)
	lng, e2 := strconv.ParseFloat(q.Get("lng"), 64)
	rad, e3 := strconv.ParseFloat(q.Get("r"), 64)
	if e1 != nil || e2 != nil || e3 != nil || rad <= 0 {
		writeJSON(w, http.StatusBadRequest, errBody("lat, lng and r required"))
		return
	}
	now, open := 0, 0
	for _, p := range a.hub.livePoints(time.Now()) {
		if haversineM(p.lat, p.lng, lat, lng) <= rad {
			now++
			if p.open {
				open++
			}
		}
	}
	cities := citiesWithin(lat, lng, rad)
	home := 0
	if len(cities) > 0 {
		_ = a.store.pool.QueryRow(r.Context(), `SELECT count(*) FROM users WHERE city = ANY($1)`, cities).Scan(&home)
	}
	writeJSON(w, http.StatusOK, map[string]any{"now": now, "open": open, "background": now - open, "home": home, "cities": cities})
}

// GET /admin/site?days=30: website views, visitors, clicks and downloads.
func (a *App) handleAdminSite(w http.ResponseWriter, r *http.Request) {
	days, _ := strconv.Atoi(r.URL.Query().Get("days"))
	if days < 7 || days > 180 {
		days = 30
	}
	ctx := r.Context()
	type day struct {
		Day       string `json:"day"`
		Views     int    `json:"views"`
		Visitors  int    `json:"visitors"`
		Clicks    int    `json:"clicks"`
		Downloads int    `json:"downloads"`
	}
	rows, err := a.store.pool.Query(ctx, `SELECT to_char(d::date, 'YYYY-MM-DD'),
		coalesce(sum(h.n) FILTER (WHERE h.kind = 'view'), 0), coalesce(sum(h.n) FILTER (WHERE h.kind = 'visit'), 0),
		coalesce(sum(h.n) FILTER (WHERE h.kind = 'click'), 0), coalesce(sum(h.n) FILTER (WHERE h.kind = 'download'), 0)
		FROM generate_series(`+today+` - ($1::int - 1), `+today+`, interval '1 day') d
		LEFT JOIN site_hits h ON h.day = d::date GROUP BY d ORDER BY d`, days)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, errBody("site failed"))
		return
	}
	var ds []day
	for rows.Next() {
		var d day
		if err := rows.Scan(&d.Day, &d.Views, &d.Visitors, &d.Clicks, &d.Downloads); err != nil {
			rows.Close()
			writeJSON(w, http.StatusServiceUnavailable, errBody("site failed"))
			return
		}
		ds = append(ds, d)
	}
	rows.Close()
	top := func(kind, col string) []kv {
		out := []kv{}
		rows, err := a.store.pool.Query(ctx, `SELECT `+col+`, sum(n) FROM site_hits WHERE kind = $1 AND day > `+today+` - $2::int
			GROUP BY 1 ORDER BY 2 DESC LIMIT 15`, kind, days)
		if err != nil {
			return out
		}
		defer rows.Close()
		for rows.Next() {
			var k kv
			if rows.Scan(&k.Key, &k.N) == nil {
				out = append(out, k)
			}
		}
		return out
	}
	writeJSON(w, http.StatusOK, map[string]any{"days": ds, "pages": top("view", "path"),
		"clicks": top("click", "target || ' · ' || path"), "downloads": top("download", "target")})
}

// GET /admin/installs?platform=&offset=: the Downloads drill-down. Breakdowns for one platform
// (or all), how many still run (opened in the last 7 days), and the newest installs with
// who signed in on them. Never device ids.
func (a *App) handleAdminInstalls(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	plat, forUID := r.URL.Query().Get("platform"), r.URL.Query().Get("uid")
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	offset = max(offset, 0)
	out := map[string]any{"platform": plat}
	var total, signed, active7, new7, new30 int
	err := a.store.pool.QueryRow(ctx, `SELECT count(*), count(uid),
		count(*) FILTER (WHERE last_at > now() - interval '7 days'),
		count(*) FILTER (WHERE first_at > now() - interval '7 days'),
		count(*) FILTER (WHERE first_at > now() - interval '30 days')
		FROM installs WHERE ($1 = '' OR platform = $1)`, plat).Scan(&total, &signed, &active7, &new7, &new30)
	out["total"], out["signedIn"], out["active7"], out["new7"], out["new30"] = total, signed, active7, new7, new30
	q := func(key, sql string) {
		if err != nil {
			return
		}
		var rows pgx.Rows
		if rows, err = a.store.pool.Query(ctx, sql, plat); err != nil {
			return
		}
		defer rows.Close()
		list := []map[string]any{}
		for rows.Next() {
			var k string
			var n, m int
			if err = rows.Scan(&k, &n, &m); err != nil {
				return
			}
			list = append(list, map[string]any{"key": k, "n": n, "m": m})
		}
		out[key] = list
	}
	// n = installs, m = signed in
	q("models", `SELECT CASE WHEN model = '' THEN 'Unknown' ELSE model END, count(*), count(uid) FROM installs WHERE ($1 = '' OR platform = $1) GROUP BY 1 ORDER BY 2 DESC LIMIT 40`)
	q("os", `SELECT CASE WHEN os = '' THEN platform ELSE os END, count(*), count(uid) FROM installs WHERE ($1 = '' OR platform = $1) GROUP BY 1 ORDER BY 2 DESC LIMIT 20`)
	q("versions", `SELECT CASE WHEN version = '' THEN 'Unknown' ELSE version END, count(*), count(*) FILTER (WHERE last_at > now() - interval '7 days') FROM installs WHERE ($1 = '' OR platform = $1) GROUP BY 1 ORDER BY 2 DESC LIMIT 20`)
	q("days", `SELECT to_char(d::date, 'YYYY-MM-DD'), count(i.*), count(i.uid)
		FROM generate_series(`+today+` - 89, `+today+`, interval '1 day') d
		LEFT JOIN installs i ON (i.first_at AT TIME ZONE '`+usageTZ+`')::date = d::date AND ($1 = '' OR i.platform = $1) GROUP BY d ORDER BY d`)
	if err == nil {
		var rows pgx.Rows
		rows, err = a.store.pool.Query(ctx, `
			SELECT i.platform, i.model, i.os, i.version, i.first_at, i.last_at, coalesce(i.uid, ''), coalesce(p.name, ''), coalesce(u.city, '')
			FROM installs i LEFT JOIN users u ON u.id = i.uid LEFT JOIN profiles p ON p.uid = i.uid
			WHERE ($1 = '' OR i.platform = $1) AND ($3 = '' OR i.uid = $3) ORDER BY i.first_at DESC LIMIT 50 OFFSET $2`, plat, offset, forUID)
		if err == nil {
			list := []map[string]any{}
			for rows.Next() {
				var pf, model, os, ver, uid, name, city string
				var first, last time.Time
				if err = rows.Scan(&pf, &model, &os, &ver, &first, &last, &uid, &name, &city); err != nil {
					break
				}
				list = append(list, map[string]any{"platform": pf, "model": model, "os": os, "version": ver,
					"firstAt": first, "lastAt": last, "uid": uid, "name": name, "city": city})
			}
			rows.Close()
			out["recent"] = list
		}
	}
	if err != nil {
		log.Printf("installs: %v", err)
		writeJSON(w, http.StatusServiceUnavailable, errBody("installs failed"))
		return
	}
	writeJSON(w, http.StatusOK, out)
}
