package main

import (
	"encoding/json"
	"testing"
	"time"
)

// metresNorth moves a latitude by m metres.
func metresNorth(lat, m float64) float64 { return lat + m/111_195 }

func joined(h *Hub, uid string) *client {
	c := &client{uid: uid, send: make(chan []byte, 32)}
	h.attach(c, &Card{UID: uid, Name: uid, Img: "img/" + uid}, false)
	return c
}

// lastDeck drains the socket and returns the uids in the newest deck message.
func lastDeck(t *testing.T, c *client) []string {
	t.Helper()
	var uids []string
	for {
		select {
		case msg := <-c.send:
			var m struct {
				Type  string
				Cards []Card
			}
			if err := json.Unmarshal(msg, &m); err != nil {
				t.Fatal(err)
			}
			if m.Type != "deck" {
				continue
			}
			uids = uids[:0]
			for _, card := range m.Cards {
				uids = append(uids, card.UID)
			}
		default:
			return uids
		}
	}
}

func TestMatching(t *testing.T) {
	h := newHub()
	a, b := joined(h, "a"), joined(h, "b")
	t0 := time.Now()
	const lat, lng = 28.6139, 77.2090

	h.report("a", fix{lat, lng, 5, t0, false})
	h.report("b", fix{metresNorth(lat, 60), lng, 5, t0, false})
	if got := lastDeck(t, a); len(got) != 0 {
		t.Fatalf("60 m apart should not match, a sees %v", got)
	}

	h.report("b", fix{metresNorth(lat, 20), lng, 5, t0.Add(time.Second), false})
	if got := lastDeck(t, a); len(got) != 1 || got[0] != "b" {
		t.Fatalf("20 m apart: a sees %v", got)
	}
	if got := lastDeck(t, b); len(got) != 1 || got[0] != "a" {
		t.Fatalf("20 m apart: b sees %v", got)
	}

	// Between near and far the card stays (no flicker at the edge).
	h.report("b", fix{metresNorth(lat, 48), lng, 5, t0.Add(2 * time.Second), false})
	if got := h.deck("a", t0.Add(2*time.Second)); len(got) != 1 {
		t.Fatalf("48 m with 10 m error should hold, got %v", got)
	}

	// Clearly far: dropped at once.
	h.report("b", fix{metresNorth(lat, 80), lng, 5, t0.Add(3 * time.Second), false})
	if got := lastDeck(t, a); len(got) != 0 {
		t.Fatalf("80 m apart should drop, a sees %v", got)
	}
}

func TestHoldExpiresAndStaleFixesGo(t *testing.T) {
	h := newHub()
	a := joined(h, "a")
	joined(h, "b")
	t0 := time.Now()
	h.report("a", fix{10, 10, 5, t0, false})
	h.report("b", fix{10, 10, 5, t0, false})
	if got := lastDeck(t, a); len(got) != 1 {
		t.Fatalf("same spot should match, got %v", got)
	}
	h.sweep(t0.Add(holdFor + time.Second))
	if got := lastDeck(t, a); len(got) != 0 {
		t.Fatalf("held card should expire, got %v", got)
	}
	if len(h.fixes) != 0 || len(h.cells) != 0 {
		t.Fatalf("stale positions must be forgotten: %d fixes, %d cells", len(h.fixes), len(h.cells))
	}
}

func TestRefusedFixes(t *testing.T) {
	h := newHub()
	t0 := time.Now()
	for name, f := range map[string]fix{
		"inaccurate":  {10, 10, maxAccM + 1, t0, false},
		"no accuracy": {10, 10, 0, t0, false},
		"bad lat":     {95, 10, 5, t0, false},
	} {
		if h.report("a", f) {
			t.Errorf("%s fix accepted", name)
		}
	}
	if !h.report("a", fix{10, 10, 5, t0, false}) || h.report("a", fix{10, 10, 5, t0.Add(100 * time.Millisecond), false}) {
		t.Error("a second fix within minFixGap must be dropped")
	}
}

func tokenOf(h *Hub, uid string) string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.tokenOf[uid]
}

func TestBluetoothHearingMatchesIndoors(t *testing.T) {
	h := newHub()
	a := joined(h, "a")
	joined(h, "b")
	t0 := time.Now()
	// Poor indoor fixes 120 m apart: no GPS match and no veto, so Bluetooth decides.
	h.report("a", fix{10, 10, 40, t0, false})
	h.report("b", fix{metresNorth(10, 120), 10, 40, t0, false})
	h.sighted("a", []Sighting{{tokenOf(h, "b"), -95}}, t0)
	if got := lastDeck(t, a); len(got) != 0 {
		t.Fatalf("a hearing below minRSSI must not match, got %v", got)
	}
	h.sighted("a", []Sighting{{tokenOf(h, "b"), -70}}, t0)
	if got := lastDeck(t, a); len(got) != 1 {
		t.Fatalf("a direct hearing should match, got %v", got)
	}
	h.sighted("a", []Sighting{{"unknown", -50}}, t0) // a token nobody owns is ignored
}

func TestGoodOutdoorFixesVetoBluetooth(t *testing.T) {
	h := newHub()
	a := joined(h, "a")
	joined(h, "b")
	t0 := time.Now()
	h.report("a", fix{10, 10, 5, t0, false})
	h.report("b", fix{metresNorth(10, 90), 10, 5, t0, false})
	h.sighted("a", []Sighting{{tokenOf(h, "b"), -70}}, t0)
	if got := lastDeck(t, a); len(got) != 0 {
		t.Fatalf("two good fixes 90 m apart must overrule Bluetooth, got %v", got)
	}
}

func TestHiddenUserIsGoneEverywhere(t *testing.T) {
	h := newHub()
	a, b := joined(h, "a"), joined(h, "b")
	t0 := time.Now()
	h.report("a", fix{10, 10, 5, t0, false})
	h.report("b", fix{10, 10, 5, t0, false})
	bToken := tokenOf(h, "b")
	h.setHidden("b", true, nil)
	if got := lastDeck(t, a); len(got) != 0 {
		t.Fatalf("hidden b must vanish from a's deck, got %v", got)
	}
	if got := lastDeck(t, b); len(got) != 0 {
		t.Fatalf("hidden b must see nobody, got %v", got)
	}
	if h.report("b", fix{10, 10, 5, t0.Add(time.Second), false}) {
		t.Fatal("a hidden user's fix must be refused")
	}
	h.sighted("a", []Sighting{{bToken, -60}}, t0.Add(time.Second))
	if got := lastDeck(t, a); len(got) != 0 {
		t.Fatalf("hidden b's old Bluetooth token must not match, got %v", got)
	}
}

func TestStillPhoneKeepsItsSpot(t *testing.T) {
	h := newHub()
	joined(h, "a")
	t0 := time.Now()
	h.report("a", fix{10, 10, 5, t0, true})
	h.sweep(t0.Add(5 * time.Minute))
	if _, ok := h.fixes["a"]; !ok {
		t.Fatal("a still fix should outlive fixTTL")
	}
	h.sweep(t0.Add(stillTTL + time.Second))
	if _, ok := h.fixes["a"]; ok {
		t.Fatal("a still fix must still expire after stillTTL")
	}
}

func TestPersonWithoutCardIsInvisible(t *testing.T) {
	h := newHub()
	a := joined(h, "a")
	h.attach(&client{uid: "nocard", send: make(chan []byte, 8)}, nil, false)
	t0 := time.Now()
	h.report("a", fix{10, 10, 5, t0, false})
	h.report("nocard", fix{10, 10, 5, t0, false})
	if got := lastDeck(t, a); len(got) != 0 {
		t.Fatalf("a user without a card must not be shown, got %v", got)
	}
}

func TestWakeQuietPhonesNearby(t *testing.T) {
	h := newHub()
	var woken []string
	h.wake = func(uid string) { woken = append(woken, uid) }
	t0 := time.Now()

	// "b" reported here, then went quiet (app swiped): no socket, fix expired.
	h.report("b", fix{10, 10, 5, t0, false})
	h.sweep(t0.Add(fixTTL + time.Second))
	// "far" was last seen ~3 km away.
	h.report("far", fix{metresNorth(10, 3000), 10, 5, t0, false})

	joined(h, "a")
	at := t0.Add(2 * time.Minute)
	h.report("a", fix{10, 10, 5, at, false})
	if len(woken) != 1 || woken[0] != "b" {
		t.Fatalf("only the quiet nearby phone should be woken, got %v", woken)
	}
	h.report("a", fix{10, 10, 5, at.Add(5 * time.Second), false})
	if len(woken) != 1 {
		t.Fatalf("a second wake within wakeGap must wait, got %v", woken)
	}
	for i := 1; i <= 10; i++ {
		h.report("a", fix{10, 10, 5, at.Add(time.Duration(i) * wakeGap), false})
	}
	if len(woken) != wakeBudget {
		t.Fatalf("wakes per window must stop at %d, got %d", wakeBudget, len(woken))
	}
}

func TestDeletedAccountSocketSeesNobody(t *testing.T) {
	h := newHub()
	a := joined(h, "a")
	joined(h, "b")
	t0 := time.Now()
	h.report("a", fix{10, 10, 5, t0, false})
	h.report("b", fix{10, 10, 5, t0, false})
	h.remove("a")
	if got := lastDeck(t, a); len(got) != 0 {
		t.Fatalf("a deleted account's open socket must get an empty deck, got %v", got)
	}
	if h.report("a", fix{10, 10, 5, t0.Add(time.Second), false}) {
		t.Fatal("a deleted account's fixes must be refused")
	}
}
