package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestPersonal(t *testing.T) {
	for _, c := range [][3]string{
		{"Hi {{first_name}}", "Riya Sharma", "Hi Riya"},
		{"Hi {{first_name|there}}", "", "Hi there"},
		{"Hi {{first_name|there}}", "Ana", "Hi Ana"},
		{"{{name}} is here", "Ana Li", "Ana Li is here"},
		{"no tags", "x", "no tags"},
		{"{{unknown}} {{first_name}}", "Bo", "{{unknown}} Bo"},
	} {
		if got := personal(c[0], c[1]); got != c[2] {
			t.Errorf("personal(%q, %q) = %q, want %q", c[0], c[1], got, c[2])
		}
	}
}

func TestSafeLink(t *testing.T) {
	for in, ok := range map[string]bool{"https://kinjo.world/x": true, "kinjo://events/3": true, "http://x.com": false,
		"javascript:alert(1)": false, "https:///nohost": false, "": false} {
		if (safeLink(in) != "") != ok {
			t.Errorf("safeLink(%q): want ok=%v", in, ok)
		}
	}
}

func TestNearestCity(t *testing.T) {
	if c, _ := nearestCity(26.86, 75.81); c != "Jaipur" {
		t.Fatalf("MNIT Jaipur → %q", c)
	}
	if c, _ := nearestCity(0, -30); c != "" {
		t.Fatalf("mid-Atlantic → %q", c)
	}
}

// A place counts who is inside now and who came by today, from fixes, in memory.
func TestGeoWatch(t *testing.T) {
	g := newGeoWatch()
	g.load([]Place{{ID: 1, Name: "Cafe", Lat: 26.9, Lng: 75.8, RadiusM: 30, Active: true}}, nil)
	now := time.Now()
	g.observe("a", 26.9, 75.8, 10, now)
	g.observe("b", 26.9001, 75.8, 10, now) // ~11 m: inside
	g.observe("c", 26.91, 75.8, 10, now)   // ~1.1 km: outside, but active today
	live, active, feed := g.snapshot(now)
	if active != 3 || live[0].Inside != 2 || live[0].Today != 2 || live[0].Entries != 2 || len(feed) != 2 {
		t.Fatalf("got %+v active %d feed %d", live[0], active, len(feed))
	}
	if live[0].PctToday != 66.7 {
		t.Fatalf("pct %v", live[0].PctToday)
	}
	g.observe("a", 26.95, 75.8, 10, now) // walked out
	g.expire(now.Add(placeStale + time.Second))
	live, _, _ = g.snapshot(now)
	if live[0].Inside != 0 || live[0].Today != 2 {
		t.Fatalf("after leaving: %+v", live[0])
	}
}

// Audiences: groups, the live area, priority order; reminders only to unfinished cards;
// events: register needs a finished card, attendees see each other.
func TestControlFlows(t *testing.T) {
	dsn := os.Getenv("KINJO_TEST_DSN")
	if dsn == "" {
		t.Skip("KINJO_TEST_DSN not set")
	}
	ctx := context.Background()
	s, err := openStore(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close) // after the accounts' cleanups (LIFO)
	a := &App{store: s, hub: newHub(s), publicURL: "https://kinjo.test"}
	a.hub.photoBase = a.publicURL
	stamp := strconv.FormatInt(time.Now().UnixNano(), 10)
	mk := func(name string, complete bool) string {
		uid, _, err := s.emailUser(ctx, name+"-"+stamp+"@example.com")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = s.deleteAccount(context.Background(), uid) })
		p := Profile{Name: name, Role: "Builds things", Look: "Co-founders", Photo: "data:image/jpeg;base64,/9j/"}
		if !complete {
			p.Photo = ""
		}
		if err := s.saveProfile(ctx, uid, p); err != nil {
			t.Fatal(err)
		}
		return uid
	}
	done, open, unfinished := mk("Done", true), mk("Open", true), mk("Unfinished", false)
	if _, err := s.pool.Exec(ctx, `UPDATE users SET city = 'Testville' WHERE id = ANY($1)`, []string{done, open, unfinished}); err != nil {
		t.Fatal(err)
	}
	if err := s.recordOpen(ctx, open, "android"); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	a.hub.addFix(open, 26.9, 75.8, 10, 0, true, now)
	a.hub.addFix(done, 26.9001, 75.8, 10, 0, true, now)

	city := func(g string) Audience { return Audience{Group: g, City: "Testville"} }
	got := func(au Audience) []audienceMember {
		ms, err := a.resolve(ctx, au, 0)
		if err != nil {
			t.Fatal(err)
		}
		return ms
	}
	if n := len(got(city("all"))); n != 3 {
		t.Fatalf("all: %d", n)
	}
	if ms := got(city("incomplete")); len(ms) != 1 || ms[0].UID != unfinished {
		t.Fatalf("incomplete: %+v", ms)
	}
	if ms := got(city("active")); len(ms) != 1 || ms[0].UID != open {
		t.Fatalf("active: %+v", ms)
	}
	if n := len(got(city("inactive"))); n != 2 {
		t.Fatalf("inactive: %d", n)
	}
	area := Audience{Group: "all", Area: &Area{Lat: 26.9, Lng: 75.8, Radius: 50}}
	ms := got(area)
	if len(ms) != 2 || ms[0].Rank > ms[1].Rank {
		t.Fatalf("area: %+v", ms)
	}
	if ms[0].UID != open && ms[0].Rank != rankHere {
		t.Fatalf("area priority: %+v", ms)
	}
	if n := len(got(Audience{Area: &Area{Lat: 26.9, Lng: 75.8, Radius: 5}})); n != 1 {
		t.Fatalf("5 m area: %d", n)
	}
	if n := len(got(Audience{UIDs: []string{done}})); n != 1 {
		t.Fatalf("uids: %d", n)
	}

	// reminders: the unfinished card only, once per step
	if _, err := s.pool.Exec(ctx, `UPDATE users SET created_at = now() - interval '25 hours' WHERE id = ANY($1)`, []string{done, unfinished}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.remindIncomplete(ctx); err != nil {
		t.Fatal(err)
	}
	var rd, ru int
	_ = s.pool.QueryRow(ctx, `SELECT (SELECT reminders FROM users WHERE id = $1), (SELECT reminders FROM users WHERE id = $2)`, done, unfinished).Scan(&rd, &ru)
	if rd != 0 || ru != 1 {
		t.Fatalf("reminders done=%d unfinished=%d", rd, ru)
	}
	if _, err := a.remindIncomplete(ctx); err != nil {
		t.Fatal(err)
	}
	_ = s.pool.QueryRow(ctx, `SELECT reminders FROM users WHERE id = $1`, unfinished).Scan(&ru)
	if ru != 1 {
		t.Fatalf("second reminder before its time: %d", ru)
	}
	var notifs int
	_ = s.pool.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE uid = $1`, unfinished).Scan(&notifs)
	if notifs != 1 {
		t.Fatalf("reminder push stored: %d", notifs)
	}

	// events
	rec := httptest.NewRecorder()
	body := `{"title":"Founders breakfast","city":"Testville","startsAt":"` + now.Add(24*time.Hour).Format(time.RFC3339) + `","published":true,"capacity":2}`
	a.handleAdminEvents(rec, httptest.NewRequest("POST", "/admin/events", strings.NewReader(body)))
	var ev Event
	if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &ev) != nil || ev.ID == 0 {
		t.Fatalf("create event: %d %s", rec.Code, rec.Body)
	}
	defer s.pool.Exec(context.Background(), `DELETE FROM events WHERE id = $1`, ev.ID)
	rsvp := func(uid string, method string) int {
		r := httptest.NewRequest(method, "/api/events/x/rsvp", nil)
		r.SetPathValue("id", strconv.FormatInt(ev.ID, 10))
		w := httptest.NewRecorder()
		a.handleRSVP(w, r, uid)
		return w.Code
	}
	if c := rsvp(unfinished, "POST"); c != http.StatusConflict {
		t.Fatalf("unfinished card registers: %d", c)
	}
	if rsvp(done, "POST") != 200 || rsvp(open, "POST") != 200 {
		t.Fatal("register")
	}
	people := func(uid string) (int, []nearbyPerson) {
		r := httptest.NewRequest("GET", "/api/events/x/people", nil)
		r.SetPathValue("id", strconv.FormatInt(ev.ID, 10))
		w := httptest.NewRecorder()
		a.handleEventPeople(w, r, uid)
		var out struct{ People []nearbyPerson }
		_ = json.Unmarshal(w.Body.Bytes(), &out)
		return w.Code, out.People
	}
	if c, ps := people(done); c != 200 || len(ps) != 1 || ps[0].UID != open || !strings.HasPrefix(ps[0].Img, "https://kinjo.test/api/photo/") {
		t.Fatalf("people: %d %+v", c, ps)
	}
	if c, _ := people(unfinished); c != http.StatusForbidden {
		t.Fatalf("not registered sees people: %d", c)
	}
	if n := len(got(Audience{Group: "event", Event: ev.ID})); n != 2 {
		t.Fatalf("event audience: %d", n)
	}
	if rsvp(open, "DELETE") != 200 {
		t.Fatal("cancel")
	}
	if _, ps := people(done); len(ps) != 0 {
		t.Fatalf("cancelled still listed: %+v", ps)
	}

	// in-app message + campaign runs to the end
	a.runCampaign(0, []string{"inapp"}, CampaignContent{Title: "Hi {{first_name|there}}", Body: "b"}, got(Audience{UIDs: []string{done}}))
	var title string
	_ = s.pool.QueryRow(ctx, `SELECT title FROM inbox WHERE uid = $1`, done).Scan(&title)
	if title != "Hi Done" {
		t.Fatalf("inbox title %q", title)
	}
}

// A place's message reaches each person found inside while it's live, once, and its record of
// who got it goes when the message is replaced.
func TestPlaceMessage(t *testing.T) {
	dsn := os.Getenv("KINJO_TEST_DSN")
	if dsn == "" {
		t.Skip("KINJO_TEST_DSN not set")
	}
	ctx := context.Background()
	s, err := openStore(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	a := &App{store: s, hub: newHub(s)}
	uid, _, err := s.emailUser(ctx, "placemsg-"+strconv.FormatInt(time.Now().UnixNano(), 10)+"@example.com")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.deleteAccount(context.Background(), uid) })
	until := time.Now().Add(time.Hour).Format(time.RFC3339)
	rec := httptest.NewRecorder()
	a.handleAdminPlaces(rec, httptest.NewRequest("POST", "/admin/places", strings.NewReader(
		`{"name":"Cafe","lat":26.85,"lng":75.81,"radius":50,"active":true,"msg":{"title":"Hi {{first_name|there}}","body":"Coffee at 7","inapp":true,"until":"`+until+`"}}`)))
	var out struct{ Places []Place }
	if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &out) != nil || len(out.Places) == 0 || out.Places[0].Msg == nil {
		t.Fatalf("save: %d %s", rec.Code, rec.Body)
	}
	pid := out.Places[0].ID
	t.Cleanup(func() { _, _ = s.pool.Exec(context.Background(), `DELETE FROM places WHERE id = $1`, pid) })
	drain := func() int {
		n := 0
		for {
			select {
			case ar := <-a.hub.geo.arrive:
				a.sendPlaceMsg(ctx, ar)
				n++
			default:
				return n
			}
		}
	}
	inbox := func() int {
		var n int
		_ = s.pool.QueryRow(ctx, `SELECT count(*) FROM inbox WHERE uid = $1`, uid).Scan(&n)
		return n
	}
	now := time.Now()
	a.hub.addFix(uid, 26.95, 75.81, 10, 0, true, now) // outside
	if drain() != 0 {
		t.Fatal("outside got it")
	}
	a.hub.addFix(uid, 26.8501, 75.81, 10, 0, true, now.Add(time.Second)) // walks in
	drain()
	a.hub.addFix(uid, 26.8502, 75.81, 10, 0, true, now.Add(2*time.Second))
	drain()
	if n := inbox(); n != 1 {
		t.Fatalf("inbox %d, want 1", n)
	}
	// a restart forgets the in-memory set: place_sends still stops a repeat
	a.hub.geo = newGeoWatch()
	if err := a.reloadPlaces(ctx); err != nil {
		t.Fatal(err)
	}
	a.hub.addFix(uid, 26.8501, 75.81, 10, 0, true, now.Add(3*time.Second))
	drain()
	if n := inbox(); n != 1 {
		t.Fatalf("after restart inbox %d, want 1", n)
	}
	var title string
	_ = s.pool.QueryRow(ctx, `SELECT title FROM inbox WHERE uid = $1`, uid).Scan(&title)
	if title != "Hi there" {
		t.Fatalf("title %q", title)
	}
	// clearing the message drops who got it
	rec = httptest.NewRecorder()
	a.handleAdminPlaces(rec, httptest.NewRequest("POST", "/admin/places", strings.NewReader(
		`{"id":`+strconv.FormatInt(pid, 10)+`,"name":"Cafe","lat":26.85,"lng":75.81,"radius":50,"active":true,"clearMsg":true}`)))
	var left int
	_ = s.pool.QueryRow(ctx, `SELECT count(*) FROM place_sends WHERE place_id = $1`, pid).Scan(&left)
	if rec.Code != 200 || left != 0 {
		t.Fatalf("clear: %d, %d rows left", rec.Code, left)
	}
}

// The Downloads drill-down: per-platform breakdowns and the newest installs, no device ids.
func TestAdminInstalls(t *testing.T) {
	dsn := os.Getenv("KINJO_TEST_DSN")
	if dsn == "" {
		t.Skip("KINJO_TEST_DSN not set")
	}
	ctx := context.Background()
	s, err := openStore(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	a := &App{store: s, hub: newHub(s), publicURL: "https://kinjo.test"}
	stamp := strconv.FormatInt(time.Now().UnixNano(), 10)
	ids := []string{"t-a-" + stamp, "t-b-" + stamp, "t-c-" + stamp}
	t.Cleanup(func() {
		_, _ = s.pool.Exec(context.Background(), `DELETE FROM installs WHERE device_id = ANY($1)`, ids)
	})
	for i, pf := range []string{"android", "android", "ios"} {
		if _, err := s.pool.Exec(ctx, `INSERT INTO installs (device_id, platform, model, os, version) VALUES ($1, $2, 'Pixel 8', 'Android 15', '2.3.4')`, ids[i], pf); err != nil {
			t.Fatal(err)
		}
	}
	rec := httptest.NewRecorder()
	a.handleAdminInstalls(rec, httptest.NewRequest("GET", "/admin/installs?platform=android", nil))
	var out struct {
		Total  int              `json:"total"`
		New7   int              `json:"new7"`
		Days   []map[string]any `json:"days"`
		Recent []map[string]any `json:"recent"`
	}
	if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &out) != nil {
		t.Fatalf("installs: %d %s", rec.Code, rec.Body)
	}
	if out.Total < 2 || out.New7 < 2 || len(out.Days) != 90 || len(out.Recent) < 2 {
		t.Fatalf("installs: %+v", out)
	}
	for _, r := range out.Recent {
		if r["platform"] != "android" || r["device_id"] != nil {
			t.Fatalf("row %+v", r)
		}
	}
	if strings.Contains(rec.Body.String(), stamp) {
		t.Fatal("device id leaked")
	}
}

// Phones from before 2.4.0: counted from devices, else a stand-in per account that opened the
// app; the app's own report later lands on the same row or replaces the stand-in.
func TestBackfillInstalls(t *testing.T) {
	dsn := os.Getenv("KINJO_TEST_DSN")
	if dsn == "" {
		t.Skip("KINJO_TEST_DSN not set")
	}
	ctx := context.Background()
	s, err := openStore(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	a := &App{store: s, hub: newHub(s)}
	stamp := strconv.FormatInt(time.Now().UnixNano(), 10)
	mk := func(n string) string {
		uid, _, err := s.emailUser(ctx, n+"-"+stamp+"@example.com")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			_, _ = s.pool.Exec(context.Background(), `DELETE FROM installs WHERE uid = $1 OR device_id = $2`, uid, "u:"+uid)
			_ = s.deleteAccount(context.Background(), uid)
		})
		return uid
	}
	phone, opener, never := mk("phone"), mk("opener"), mk("never")
	dev := "dev-" + stamp
	t.Cleanup(func() { _, _ = s.pool.Exec(context.Background(), `DELETE FROM installs WHERE device_id = $1`, dev) })
	if _, err := s.pool.Exec(ctx, `INSERT INTO devices (push_token, uid, platform, device_id) VALUES ($1, $2, 'android', $3), ($4, $2, 'ioslocation', '')`, "tok-"+stamp, phone, dev, "loc-"+stamp); err != nil {
		t.Fatal(err)
	}
	for _, u := range []string{phone, opener} {
		if _, err := s.pool.Exec(ctx, `INSERT INTO app_opens (uid, day, platform) VALUES ($1, current_date, 'ios') ON CONFLICT DO NOTHING`, u); err != nil {
			t.Fatal(err)
		}
	}
	count := func(uid string) (n int) {
		_ = s.pool.QueryRow(ctx, `SELECT count(*) FROM installs WHERE uid = $1`, uid).Scan(&n)
		return
	}
	for i := 0; i < 2; i++ { // idempotent
		if err := a.backfillInstalls(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if count(phone) != 1 || count(opener) != 1 || count(never) != 0 {
		t.Fatalf("phone %d opener %d never %d", count(phone), count(opener), count(never))
	}
	// the opener's phone reports for real: the stand-in goes
	req := httptest.NewRequest("POST", "/api/install", strings.NewReader(`{"deviceId":"real-`+stamp+`","platform":"ios"}`))
	tok, _ := randToken(32)
	if err := s.createSession(ctx, tok, opener); err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	a.handleInstall(rec, req)
	t.Cleanup(func() {
		_, _ = s.pool.Exec(context.Background(), `DELETE FROM installs WHERE device_id = $1`, "real-"+stamp)
	})
	if rec.Code != 204 || count(opener) != 1 {
		t.Fatalf("after real install: %d, %d rows", rec.Code, count(opener))
	}
	_ = a.backfillInstalls(ctx)
	if count(opener) != 1 {
		t.Fatalf("backfill re-added the stand-in: %d", count(opener))
	}
}
