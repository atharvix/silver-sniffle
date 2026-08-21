package unit

import (
	"math"
	"testing"
)

// Haversine formula reference implementation in Go for testing accuracy
func haversineMetres(lat1, lon1, lat2, lon2 float64) float64 {
	const earthRadius = 6371000.0 // meters
	toRad := func(deg float64) float64 { return deg * math.Pi / 180.0 }

	dLat := toRad(lat2 - lat1)
	dLon := toRad(lon2 - lon1)

	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(toRad(lat1))*math.Cos(toRad(lat2))*math.Sin(dLon/2)*math.Sin(dLon/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	return earthRadius * c
}

func TestHaversineAccuracy(t *testing.T) {
	// Points ~15 meters apart
	lat1, lon1 := 37.774929, -122.419416
	lat2, lon2 := 37.775050, -122.419416 // ~13.4 meters north

	dist := haversineMetres(lat1, lon1, lat2, lon2)
	if dist < 12.0 || dist > 15.0 {
		t.Errorf("haversine distance = %f, expected ~13.4m", dist)
	}

	// Same point should be 0 distance
	if d := haversineMetres(lat1, lon1, lat1, lon1); d != 0.0 {
		t.Errorf("distance to same point = %f, want 0", d)
	}

	// Points 30m away boundary
	lat3, lon3 := 37.774929, -122.419075 // ~30 meters east
	distEast := haversineMetres(lat1, lon1, lat3, lon3)
	if distEast < 28.0 || distEast > 32.0 {
		t.Errorf("haversine distance east = %f, expected ~30m", distEast)
	}
}
