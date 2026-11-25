package main

import (
	"math"
	"math/rand"
	"slices"
	"testing"
)

func TestDistanceKnownPair(t *testing.T) {
	// One thousandth of a degree of latitude is ~111.2 m everywhere.
	if d := distanceM(28.6, 77.2, 28.601, 77.2); math.Abs(d-111.2) > 0.5 {
		t.Fatalf("got %.1f m", d)
	}
}

// Anyone within the largest match radius (30 m + 40 m error) must be in the neighbourhood.
func TestNeighbourhoodCoversMatchRadius(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	for i := 0; i < 20_000; i++ {
		lat := r.Float64()*170 - 85
		lng := r.Float64()*358 - 179
		bearing := r.Float64() * 2 * math.Pi
		dist := r.Float64() * 70
		lat2 := lat + dist*math.Cos(bearing)/111_195
		lng2 := lng + dist*math.Sin(bearing)/(111_195*math.Cos(lat*math.Pi/180))
		if !slices.Contains(neighbourhood(lat, lng), cellOf(lat2, lng2)) {
			t.Fatalf("missed a point %.1f m away at lat %.4f", dist, lat)
		}
	}
}
