package main

import "math"

// Geo + matching constants, mirrored from the original client logic so the
// "who is within 30 m" behaviour is identical after the move off Firestore.
const (
	radiusM  = 30.0   // metres shown to people
	slackMin = 3.0    // GPS tolerance floor
	slackMax = 10.0   // GPS tolerance ceiling (grows with both phones' accuracy)
	exitGap  = 7.0    // hysteresis: someone shown stays until this much further out
	badAccM  = 150.0  // ignore GPS fixes worse than this (metres)
	cellSize = 0.0005 // grid used to query the neighbourhood (~55 m north-south)
	staleMS  = int64(60 * 60 * 1000)
)

// cellOf returns the grid-cell id for a coordinate. Presence is indexed by cell
// so a neighbourhood lookup only scans nearby cells, not every user.
func cellOf(lat, lng float64) string {
	return itoa(int(math.Floor(lat/cellSize))) + "_" + itoa(int(math.Floor(lng/cellSize)))
}

// cellsAround returns the cells covering RADIUS + slack + one cell of movement
// in every direction, at the given latitude.
func cellsAround(lat, lng float64) []string {
	reach := radiusM + slackMax + exitGap
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

// inside reports whether a person at distance d is inside the circle right now.
// wasShown widens the boundary by exitGap so cards don't flicker at the edge.
func inside(d float64, accMe, accThem int, wasShown bool) bool {
	me := float64(accMe)
	if accMe <= 0 {
		me = 20
	}
	them := float64(accThem)
	if accThem <= 0 {
		them = 20
	}
	slack := clampF((me+them)/4, slackMin, slackMax)
	extra := 0.0
	if wasShown {
		extra = exitGap
	}
	return d <= radiusM+slack+extra
}

func clampF(v, lo, hi float64) float64 {
	return math.Max(lo, math.Min(hi, v))
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
