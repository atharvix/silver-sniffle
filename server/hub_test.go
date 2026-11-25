package main

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

var ctx = context.Background()

func testHub(t *testing.T) (*Hub, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	return hubOn(t, mr), mr
}

func hubOn(t *testing.T, mr *miniredis.Miniredis) *Hub {
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })
	return newHub(rdb)
}

// metresNorth moves a latitude by m metres.
func metresNorth(lat, m float64) float64 { return lat + m/111_195 }

func joined(t *testing.T, h *Hub, uid string) *client {
	t.Helper()
	c := &client{uid: uid, send: make(chan []byte, 64)}
	if err := h.attach(ctx, c, &Card{UID: uid, Name: uid, Img: "img/" + uid}, false, nil); err != nil {
		t.Fatal(err)
	}
	return c
}

func report(t *testing.T, h *Hub, uid string, f fix) bool {
	t.Helper()
	ok, err := h.report(ctx, uid, f)
	if err != nil {
		t.Fatal(err)
	}
	return ok
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

func tokenOf(t *testing.T, h *Hub, uid string) string {
	t.Helper()
	tok, err := h.rdb.Get(ctx, kTokOf(uid)).Result()
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

func TestMatching(t *testing.T) {
	h, _ := testHub(t)
	a, b := joined(t, h, "a"), joined(t, h, "b")
	t0 := time.Now()
	const lat, lng = 28.6139, 77.2090

	report(t, h, "a", fix{lat, lng, 5, t0, false})
	report(t, h, "b", fix{metresNorth(lat, 60), lng, 5, t0, false})
	if got := lastDeck(t, a); len(got) != 0 {
		t.Fatalf("60 m apart should not match, a sees %v", got)
	}

	report(t, h, "b", fix{metresNorth(lat, 20), lng, 5, t0.Add(time.Second), false})
	if got := lastDeck(t, a); len(got) != 1 || got[0] != "b" {
		t.Fatalf("20 m apart: a sees %v", got)
	}
	if got := lastDeck(t, b); len(got) != 1 || got[0] != "a" {
		t.Fatalf("20 m apart: b sees %v", got)
	}

	// Between near and far the card stays (no flicker at the edge).
	report(t, h, "b", fix{metresNorth(lat, 48), lng, 5, t0.Add(2 * time.Second), false})
	if got, _ := h.deck(ctx, "a", t0.Add(2*time.Second)); len(got) != 1 {
		t.Fatalf("48 m with 10 m error should hold, got %v", got)
	}

	// Clearly far: dropped at once.
	report(t, h, "b", fix{metresNorth(lat, 80), lng, 5, t0.Add(3 * time.Second), false})
	if got := lastDeck(t, a); len(got) != 0 {
		t.Fatalf("80 m apart should drop, a sees %v", got)
	}
}

func TestHoldExpiresAndStaleFixesGo(t *testing.T) {
	h, mr := testHub(t)
	a := joined(t, h, "a")
	joined(t, h, "b")
	t0 := time.Now()
	report(t, h, "a", fix{10, 10, 5, t0, false})
	report(t, h, "b", fix{10, 10, 5, t0, false})
	if got := lastDeck(t, a); len(got) != 1 {
		t.Fatalf("same spot should match, got %v", got)
	}
	if got, _ := h.deck(ctx, "a", t0.Add(holdFor+time.Second)); len(got) != 0 {
		t.Fatalf("held card should expire, got %v", got)
	}
	mr.FastForward(fixTTL + time.Second)
	if mr.Exists(kFix("a")) {
		t.Fatal("a moving phone's position must be forgotten after fixTTL")
	}
}

func TestRefusedFixes(t *testing.T) {
	h, _ := testHub(t)
	t0 := time.Now()
	for name, f := range map[string]fix{
		"inaccurate":  {10, 10, maxAccM + 1, t0, false},
		"no accuracy": {10, 10, 0, t0, false},
		"bad lat":     {95, 10, 5, t0, false},
	} {
		if report(t, h, "a", f) {
			t.Errorf("%s fix accepted", name)
		}
	}
	if !report(t, h, "a", fix{10, 10, 5, t0, false}) || report(t, h, "a", fix{10, 10, 5, t0.Add(100 * time.Millisecond), false}) {
		t.Error("a second fix within minFixGap must be dropped")
	}
}

func TestBluetoothHearingMatchesIndoors(t *testing.T) {
	h, _ := testHub(t)
	a := joined(t, h, "a")
	joined(t, h, "b")
	t0 := time.Now()
	// Poor indoor fixes 120 m apart: no GPS match and no veto, so Bluetooth decides.
	report(t, h, "a", fix{10, 10, 40, t0, false})
	report(t, h, "b", fix{metresNorth(10, 120), 10, 40, t0, false})
	if err := h.sighted(ctx, "a", []Sighting{{tokenOf(t, h, "b"), -95}}, t0); err != nil {
		t.Fatal(err)
	}
	if got := lastDeck(t, a); len(got) != 0 {
		t.Fatalf("a hearing below minRSSI must not match, got %v", got)
	}
	if err := h.sighted(ctx, "a", []Sighting{{tokenOf(t, h, "b"), -70}, {"unknown", -50}}, t0); err != nil {
		t.Fatal(err)
	}
	if got := lastDeck(t, a); len(got) != 1 {
		t.Fatalf("a direct hearing should match (an unknown token is ignored), got %v", got)
	}
}

func TestGoodOutdoorFixesVetoBluetooth(t *testing.T) {
	h, _ := testHub(t)
	a := joined(t, h, "a")
	joined(t, h, "b")
	t0 := time.Now()
	report(t, h, "a", fix{10, 10, 5, t0, false})
	report(t, h, "b", fix{metresNorth(10, 90), 10, 5, t0, false})
	if err := h.sighted(ctx, "a", []Sighting{{tokenOf(t, h, "b"), -70}}, t0); err != nil {
		t.Fatal(err)
	}
	if got := lastDeck(t, a); len(got) != 0 {
		t.Fatalf("two good fixes 90 m apart must overrule Bluetooth, got %v", got)
	}
}

func TestHiddenUserIsGoneEverywhere(t *testing.T) {
	h, _ := testHub(t)
	a, b := joined(t, h, "a"), joined(t, h, "b")
	t0 := time.Now()
	report(t, h, "a", fix{10, 10, 5, t0, false})
	report(t, h, "b", fix{10, 10, 5, t0, false})
	bToken := tokenOf(t, h, "b")
	if err := h.setHidden(ctx, "b", true, nil); err != nil {
		t.Fatal(err)
	}
	if got := lastDeck(t, a); len(got) != 0 {
		t.Fatalf("hidden b must vanish from a's deck, got %v", got)
	}
	if got := lastDeck(t, b); len(got) != 0 {
		t.Fatalf("hidden b must see nobody, got %v", got)
	}
	if report(t, h, "b", fix{10, 10, 5, t0.Add(time.Second), false}) {
		t.Fatal("a hidden user's fix must be refused")
	}
	if err := h.sighted(ctx, "a", []Sighting{{bToken, -60}}, t0.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if got := lastDeck(t, a); len(got) != 0 {
		t.Fatalf("hidden b's old Bluetooth token must not match, got %v", got)
	}
}

func TestStillPhoneKeepsItsSpot(t *testing.T) {
	h, mr := testHub(t)
	joined(t, h, "a")
	report(t, h, "a", fix{10, 10, 5, time.Now(), true})
	mr.FastForward(5 * time.Minute)
	if !mr.Exists(kFix("a")) {
		t.Fatal("a still fix should outlive fixTTL")
	}
	mr.FastForward(stillTTL)
	if mr.Exists(kFix("a")) {
		t.Fatal("a still fix must still expire after stillTTL")
	}
}

func TestPersonWithoutCardIsInvisible(t *testing.T) {
	h, _ := testHub(t)
	a := joined(t, h, "a")
	if err := h.attach(ctx, &client{uid: "nocard", send: make(chan []byte, 8)}, nil, false, nil); err != nil {
		t.Fatal(err)
	}
	t0 := time.Now()
	report(t, h, "a", fix{10, 10, 5, t0, false})
	report(t, h, "nocard", fix{10, 10, 5, t0, false})
	if got := lastDeck(t, a); len(got) != 0 {
		t.Fatalf("a user without a card must not be shown, got %v", got)
	}
}

func TestWakeQuietPhonesNearby(t *testing.T) {
	h, mr := testHub(t)
	var woken []string
	h.wake = func(uid string) { woken = append(woken, uid) }
	t0 := time.Now()

	// "b" reported here, then went quiet (app swiped): no socket, and its fix expires.
	report(t, h, "b", fix{10, 10, 5, t0, false})
	// "far" was last seen ~3 km away.
	report(t, h, "far", fix{metresNorth(10, 3000), 10, 5, t0, false})
	mr.FastForward(fixTTL + time.Second)

	joined(t, h, "a")
	at := t0.Add(2 * time.Minute)
	report(t, h, "a", fix{10, 10, 5, at, false})
	if len(woken) != 1 || woken[0] != "b" {
		t.Fatalf("only the quiet nearby phone should be woken, got %v", woken)
	}
	report(t, h, "a", fix{10, 10, 5, at.Add(5 * time.Second), false})
	if len(woken) != 1 {
		t.Fatalf("a second wake within wakeGap must wait, got %v", woken)
	}
	for i := 1; i <= 10; i++ {
		report(t, h, "a", fix{10, 10, 5, at.Add(time.Duration(i) * wakeGap), false})
	}
	if len(woken) != wakeBudget {
		t.Fatalf("wakes per window must stop at %d, got %d", wakeBudget, len(woken))
	}
}

func TestDeletedAccountSocketSeesNobody(t *testing.T) {
	h, _ := testHub(t)
	a := joined(t, h, "a")
	joined(t, h, "b")
	t0 := time.Now()
	report(t, h, "a", fix{10, 10, 5, t0, false})
	report(t, h, "b", fix{10, 10, 5, t0, false})
	var revoked string
	h.revoked = func(uid string) { revoked = uid }
	if err := h.remove(ctx, "a"); err != nil {
		t.Fatal(err)
	}
	if got := lastDeck(t, a); len(got) != 0 {
		t.Fatalf("a deleted account's open socket must get an empty deck, got %v", got)
	}
	if report(t, h, "a", fix{10, 10, 5, t0.Add(time.Second), false}) {
		t.Fatal("a deleted account's fixes must be refused")
	}
	if revoked != "a" {
		t.Fatal("a deleted account's cached sessions must be dropped")
	}
}

// Two servers share one Redis: people connected to different servers still see each other.
func TestTwoServersShareState(t *testing.T) {
	mr := miniredis.RunT(t)
	one, two := hubOn(t, mr), hubOn(t, mr)
	lctx, stop := context.WithCancel(ctx)
	defer stop()
	go one.listen(lctx)
	go two.listen(lctx)
	time.Sleep(100 * time.Millisecond) // let both subscriptions start

	a := joined(t, one, "a")
	b := joined(t, two, "b")
	t0 := time.Now()
	report(t, one, "a", fix{10, 10, 5, t0, false})
	report(t, two, "b", fix{10, 10, 5, t0, false}) // "a" is on the other server: it learns through pub/sub

	deadline := time.Now().Add(2 * time.Second)
	for {
		if got := lastDeck(t, a); len(got) == 1 && got[0] == "b" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("a, on server one, never saw b, on server two")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if got := lastDeck(t, b); len(got) != 1 || got[0] != "a" {
		t.Fatalf("b sees %v", got)
	}
}

func TestBlockHidesBothWays(t *testing.T) {
	h, _ := testHub(t)
	a, b := joined(t, h, "a"), joined(t, h, "b")
	t0 := time.Now()
	report(t, h, "a", fix{10, 10, 5, t0, false})
	report(t, h, "b", fix{10, 10, 5, t0, false})
	if got := lastDeck(t, a); len(got) != 1 {
		t.Fatalf("same spot should match before the block, got %v", got)
	}
	if err := h.block(ctx, "a", "b"); err != nil {
		t.Fatal(err)
	}
	if got := lastDeck(t, a); len(got) != 0 {
		t.Fatalf("a blocked b: a must not see b, got %v", got)
	}
	if got := lastDeck(t, b); len(got) != 0 {
		t.Fatalf("a blocked b: b must not see a either, got %v", got)
	}
	report(t, h, "b", fix{10, 10, 5, t0.Add(time.Second), false})
	if err := h.sighted(ctx, "a", []Sighting{{tokenOf(t, h, "b"), -60}}, t0.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if got, _ := h.deck(ctx, "a", t0.Add(time.Second)); len(got) != 0 {
		t.Fatalf("neither GPS nor Bluetooth may bring a blocked pair back, got %v", got)
	}
}

func TestNewPairsAreCountedOnce(t *testing.T) {
	h, mr := testHub(t)
	joined(t, h, "a")
	joined(t, h, "b")
	t0 := time.Now()
	report(t, h, "a", fix{10, 10, 5, t0, false})
	report(t, h, "b", fix{10, 10, 5, t0, false})
	report(t, h, "b", fix{10, 10, 5, t0.Add(time.Second), false}) // same pair again: not a new match
	if got, _ := mr.Get(kCount(countMatches)); got != "1" {
		t.Fatalf("one pair should count as one match, got %q", got)
	}
}
