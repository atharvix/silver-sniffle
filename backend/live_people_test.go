package main

import (
	"context"
	"encoding/json"
	"math"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestSnapArea(t *testing.T) {
	lat, lng := snapArea(26.912345, 75.787654)
	if math.Abs(lat-26.9125) > 1e-9 || math.Abs(lng-75.7875) > 1e-9 {
		t.Fatalf("snap %v %v", lat, lng)
	}
	a1, b1 := snapArea(26.9101, 75.7801)
	a2, b2 := snapArea(26.9149, 75.7849) // same ~550 m square
	if a1 != a2 || b1 != b2 {
		t.Fatal("one square, two centres")
	}
}

// The live people list carries the square's centre, city and state, never the fix.
func TestAdminLivePeople(t *testing.T) {
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
	uid, _, err := s.emailUser(ctx, "live-"+strconv.FormatInt(time.Now().UnixNano(), 10)+"@example.com")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.deleteAccount(context.Background(), uid) })
	_ = s.saveProfile(ctx, uid, Profile{Name: "Live Person", Role: "Builder"})
	a.hub.addFix(uid, 26.912345, 75.787654, 10, 0, true, time.Now())
	rec := httptest.NewRecorder()
	a.handleAdminLivePeople(rec, httptest.NewRequest("GET", "/admin/live/people", nil))
	if strings.Contains(rec.Body.String(), "26.912345") || strings.Contains(rec.Body.String(), "75.787654") {
		t.Fatal("exact fix leaked")
	}
	var out struct {
		People []struct {
			UID, Name, City, State string
			Lat, Lng               float64
		}
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	var me *struct {
		UID, Name, City, State string
		Lat, Lng               float64
	}
	for i := range out.People {
		if out.People[i].UID == uid {
			me = &out.People[i]
		}
	}
	if me == nil || me.Name != "Live Person" || me.City != "Jaipur" || me.State != "Rajasthan" || me.Lat != 26.9125 {
		t.Fatalf("got %+v", me)
	}
	live := a.hub.live(uid)
	if live["areaLat"] != 26.9125 || live["areaState"] != "Rajasthan" {
		t.Fatalf("profile live: %v", live)
	}
}
