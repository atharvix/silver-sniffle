package main

import "testing"

func TestDistM(t *testing.T) {
	// ~111.3 m for 0.001 deg of latitude.
	d := distM(12.9716, 77.5946, 12.9725, 77.5946)
	if d < 95 || d > 105 {
		t.Fatalf("expected ~100 m, got %.1f", d)
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
