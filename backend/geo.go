package main

import (
	"math"
	"time"
)

// GPS proximity rule. A person is shown when the server has a fresh fix for both
// sides, each accurate to maxAccM, and their centres are within radiusM. Indoors
// phones often report 20-50 m, so maxAccM allows that; Bluetooth (ble.go) is the
// other way two people count as close.
const (
	radiusM  = 30.0            // metres; d <= radiusM is eligible, d > radiusM is not
	maxAccM  = 50              // a fix must report accuracy within this (metres); unknown (0) never counts
	freshFor = 2 * time.Minute // a fix older than this is stale: the person may have walked off
	cellSize = 0.0005          // grid used to query the neighbourhood (~55 m north-south)
)

// cellOf returns the grid-cell id for a coordinate. Presence is indexed by cell
// so a neighbourhood lookup only scans nearby cells, not every user.
func cellOf(lat, lng float64) string {
	return itoa(int(math.Floor(lat/cellSize))) + "_" + itoa(int(math.Floor(lng/cellSize)))
}

// cellsAround returns the cells covering RADIUS + slack + one cell of movement
// in every direction, at the given latitude.
func cellsAround(lat, lng float64) []string {
	reach := radiusM
	mLat := cellSize * 111320
	mLng := math.Max(1, cellSize*111320*math.Cos(lat*math.Pi/180))
	nA := int(math.Max(1, math.Ceil(reach/mLat)))
	nB := int(math.Min(4, math.Max(1, math.Ceil(reach/mLng))))
	a := int(math.Floor(lat / cellSize))
	b := int(math.Floor(lng / cellSize))
	out := make([]string, 0, (2*nA+1)*(2*nB+1))
	for i := -nA; i <= nA; i++ {
		for j := -nB; j <= nB; j++ {
			out = append(out, itoa(a+i)+"_"+itoa(b+j))
		}
	}
	return out
}

// distM is the haversine distance in metres between two points.
func distM(aLat, aLng, bLat, bLng float64) float64 {
	const R = 6371000.0
	t := math.Pi / 180
	dLa := (bLat - aLat) * t
	dLo := (bLng - aLng) * t
	x := math.Pow(math.Sin(dLa/2), 2) +
		math.Cos(aLat*t)*math.Cos(bLat*t)*math.Pow(math.Sin(dLo/2), 2)
	return 2 * R * math.Asin(math.Sqrt(x))
}

// eligible decides whether them may appear on me's deck at time now. It returns
// the distance (ordering only, never sent) and "" when eligible, or the reason
// they aren't: "stale", "accuracy" or "far".
func eligible(me, them *Pres, now time.Time) (float64, string) {
	if now.Sub(me.T) > freshFor || now.Sub(them.T) > freshFor {
		return 0, "stale"
	}
	if me.Acc <= 0 || me.Acc > maxAccM || them.Acc <= 0 || them.Acc > maxAccM {
		return 0, "accuracy"
	}
	d := distM(me.Lat, me.Lng, them.Lat, them.Lng)
	if d > radiusM+1e-6 { // a micrometre of float noise, so exactly 30 m stays in
		return d, "far"
	}
	return d, ""
}

// itoa is a tiny signed-int formatter (avoids importing strconv everywhere).
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
