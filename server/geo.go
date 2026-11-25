package main

import "math"

const earthRadiusM = 6_371_000.0

// distanceM is the great-circle (haversine) distance in metres.
func distanceM(lat1, lng1, lat2, lng2 float64) float64 {
	p1, p2 := lat1*math.Pi/180, lat2*math.Pi/180
	dp, dl := p2-p1, (lng2-lng1)*math.Pi/180
	a := math.Sin(dp/2)*math.Sin(dp/2) + math.Cos(p1)*math.Cos(p2)*math.Sin(dl/2)*math.Sin(dl/2)
	return 2 * earthRadiusM * math.Asin(math.Sqrt(a))
}

// cell is a 0.001° grid square: ~111 m tall, narrower east-west away from the equator.
type cell struct{ lat, lng int32 }

func cellOf(lat, lng float64) cell {
	return cell{int32(math.Floor(lat * 1000)), int32(math.Floor(lng * 1000))}
}

// neighbourhood returns the cells that cover at least ~111 m around the point in every
// direction: one row above and below, and enough columns to make up for narrower cells.
// shortcut: no wrap at the ±180° meridian, nobody meets there; and |lat| must be < 89°.
func neighbourhood(lat, lng float64) []cell {
	c := cellOf(lat, lng)
	k := int32(math.Ceil(1 / math.Cos(lat*math.Pi/180)))
	out := make([]cell, 0, 3*(2*k+1))
	for dy := int32(-1); dy <= 1; dy++ {
		for dx := -k; dx <= k; dx++ {
			out = append(out, cell{c.lat + dy, c.lng + dx})
		}
	}
	return out
}
