package main

import (
	"slices"
	"testing"
	"time"
)

// "Hide my profile" is per account: once hidden, the server takes no fix and no
// Bluetooth report from any of their phones (another device's service may still be
// running), nobody sees them, and they see nobody.
func TestHiddenAccountSeesNobodyAndIsSeenByNobody(t *testing.T) {
	w := newWorld()
	now := time.Now()
	w.put(fix("a", 0, 8, now))
	w.put(fix("b", 10, 8, now))
	if !sees(w.h, w.a, "b", now) {
		t.Fatal("setup: A should see B")
	}
	w.h.setHidden("b", true)
	w.h.dropPresence("b")
	if w.put(fix("b", 10, 8, now.Add(time.Second))) != nil {
		t.Fatal("a hidden account's fix must be refused")
	}
	if sees(w.h, w.a, "b", now) || sees(w.h, w.b, "a", now) {
		t.Fatal("hidden: nobody sees B and B sees nobody")
	}
	ta, _ := w.h.bleToken("a", now)
	tb, _ := w.h.bleToken("b", now)
	if w.h.addSightings("a", []sighting{{Tok: tb, RSSI: -60}}, now) != 0 ||
		w.h.addSightings("b", []sighting{{Tok: ta, RSSI: -60}}, now) != 0 {
		t.Fatal("Bluetooth to or from a hidden account must not count")
	}
	w.h.setHidden("b", false)
	if w.put(fix("b", 10, 8, now.Add(2*time.Second))) == nil || !sees(w.h, w.a, "b", now) {
		t.Fatal("shown again: B's next fix puts them back")
	}
}

// App Review's account sees the demo cards with no fix at all, nobody else sees them,
// and hiding the demo account hides them too.
func TestDemoCards(t *testing.T) {
	w := newWorld()
	now := time.Now()
	for _, d := range demoCards {
		w.h.prof.items[d.uid] = profEntry{prof: Profile{Name: d.name, Role: d.role, Look: d.look, Photo: "p"}, ok: true, at: now}
	}
	w.h.setDemo("a")
	if !w.h.knows("a", now) {
		t.Fatal("the demo account's list goes out on connect, located or not")
	}
	w.h.recompute(w.a)
	got := lastList(w.a)
	if len(got) != len(demoCards) || got[0] != "demo_tara" {
		t.Fatalf("demo deck = %v", got)
	}
	w.put(fix("b", 0, 8, now))
	w.h.recompute(w.b)
	if got := lastList(w.b); slices.ContainsFunc(got, func(u string) bool { return u[:5] == demoPrefix }) {
		t.Fatalf("someone else saw demo cards: %v", got)
	}
	w.h.setHidden("a", true)
	w.h.recompute(w.a)
	if got := lastList(w.a); len(got) != 0 {
		t.Fatalf("hidden demo account still sees %v", got)
	}
}
