package main

import (
	"testing"
	"time"
)

func TestDistM(t *testing.T) {
	// ~111.3 m for 0.001 deg of latitude.
	d := distM(12.9716, 77.5946, 12.9725, 77.5946)
	if d < 95 || d > 105 {
		t.Fatalf("expected ~100 m, got %.1f", d)
	}
}

func TestInsideHysteresis(t *testing.T) {
	// With 20 m accuracy each, slack maxes at 10 -> base boundary 40 m, and the
	// 7 m exit gap widens it to 47 m once someone is already on the deck.
	if inside(44, 20, 20, false) {
		t.Fatal("44 m should be outside when not already shown")
	}
	if !inside(44, 20, 20, true) {
		t.Fatal("44 m should stay inside once shown (exit gap)")
	}
	if inside(48, 20, 20, true) {
		t.Fatal("48 m is past the exit gap and should drop")
	}
}

func TestCellRoundTrip(t *testing.T) {
	c := cellOf(12.97163, 77.59461)
	around := cellsAround(12.97163, 77.59461)
	found := false
	for _, x := range around {
		if x == c {
			found = true
		}
	}
	if !found {
		t.Fatalf("own cell %s not in cellsAround %v", c, around)
	}
}

func TestItoa(t *testing.T) {
	for _, tc := range []struct {
		in   int
		want string
	}{{0, "0"}, {7, "7"}, {-3, "-3"}, {25910, "25910"}, {-155189, "-155189"}} {
		if got := itoa(tc.in); got != tc.want {
			t.Errorf("itoa(%d) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestWorse(t *testing.T) {
	now := time.Now()
	fresh := &Pres{Acc: 8, T: now.Add(-10 * time.Second)}
	old := &Pres{Acc: 8, T: now.Add(-2 * time.Minute)}
	for _, tc := range []struct {
		acc  int
		prev *Pres
		want bool
	}{
		{12, fresh, false}, // similar accuracy wins
		{90, fresh, true},  // screen-off cell fix loses to a fresh GPS fix
		{90, old, false},   // ...but beats a stale one (they may have moved)
		{200, old, true},   // unusable is always ignored
	} {
		if got := worse(tc.acc, tc.prev, now); got != tc.want {
			t.Errorf("worse(%d, acc %d age %v) = %v, want %v", tc.acc, tc.prev.Acc, now.Sub(tc.prev.T), got, tc.want)
		}
	}
}
