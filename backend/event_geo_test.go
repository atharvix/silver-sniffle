package main

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"
)

// An event's pinned venue is watched while the event is on: a registered person arriving
// gets one "you're here" message; someone not registered gets nothing; the admin sees the count.
func TestEventVenueArrival(t *testing.T) {
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
	a.hub.geo = newGeoWatch()
	stamp := strconv.FormatInt(time.Now().UnixNano(), 10)
	mk := func(n string) string {
		uid, _, err := s.emailUser(ctx, n+"-"+stamp+"@example.com")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = s.deleteAccount(context.Background(), uid) })
		return uid
	}
	going, other := mk("going"), mk("other")
	var ev, later int64
	if err := s.pool.QueryRow(ctx, `INSERT INTO events (title, city, lat, lng, starts_at, published) VALUES ('Geo night `+stamp+`', 'Jaipur', 26.85, 75.81, now() + interval '30 minutes', true) RETURNING id`).Scan(&ev); err != nil {
		t.Fatal(err)
	}
	if err := s.pool.QueryRow(ctx, `INSERT INTO events (title, city, lat, lng, starts_at, published) VALUES ('Later `+stamp+`', 'Jaipur', 26.95, 75.81, now() + interval '5 hours', true) RETURNING id`).Scan(&later); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = s.pool.Exec(context.Background(), `DELETE FROM events WHERE id = ANY($1)`, []int64{ev, later})
	})
	if _, err := s.pool.Exec(ctx, `INSERT INTO event_rsvps (event_id, uid) VALUES ($1, $2), ($3, $2)`, ev, going, later); err != nil {
		t.Fatal(err)
	}
	if err := a.reloadEventVenues(ctx); err != nil {
		t.Fatal(err)
	}
	drain := func() {
		for {
			select {
			case ar := <-a.hub.geo.arrive:
				if ar.event != 0 {
					a.sendEventArrival(ctx, ar)
				}
			default:
				return
			}
		}
	}
	inbox := func(uid string) (n int) {
		_ = s.pool.QueryRow(ctx, `SELECT count(*) FROM inbox WHERE uid = $1`, uid).Scan(&n)
		return
	}
	now := time.Now()
	a.hub.geo.observe(going, 26.8503, 75.81, 10, now) // ~33 m from the pin
	a.hub.geo.observe(other, 26.8502, 75.81, 10, now) // inside, not registered
	a.hub.geo.observe(going, 26.9503, 75.81, 10, now) // the later event: not open yet
	a.hub.geo.observe(going, 26.8503, 75.81, 10, now) // back at the first one
	drain()
	a.hub.geo.observe(going, 26.8501, 75.81, 10, now.Add(time.Minute))
	drain()
	if n := inbox(going); n != 1 {
		t.Fatalf("registered: %d messages, want 1", n)
	}
	if n := inbox(other); n != 0 {
		t.Fatalf("not registered: %d messages", n)
	}
	if here := a.hub.geo.eventsInside(); here[ev] != 2 || here[later] != 0 {
		t.Fatalf("here: %v", here)
	}
	a.hub.geo.observe(going, 27.5, 75.81, 10, now.Add(2*time.Minute)) // left
	if here := a.hub.geo.eventsInside(); here[ev] != 1 {
		t.Fatalf("after leaving: %v", here)
	}
	// a restart forgets the in-memory set: event_arrivals still stops a repeat
	a.hub.geo = newGeoWatch()
	_ = a.reloadEventVenues(ctx)
	a.hub.geo.observe(going, 26.8503, 75.81, 10, now.Add(3*time.Minute))
	drain()
	if n := inbox(going); n != 1 {
		t.Fatalf("after restart: %d messages", n)
	}
}
