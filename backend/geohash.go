package main

import "math"

// Geohash (precision ghPrec, a cell ~153 x 153 m) indexes fixes so a GPS match only
// looks at the cell and its 8 neighbours: everyone within 30 m is in one of them.
const ghPrec = 7

const ghBase32 = "0123456789bcdefghjkmnpqrstuvwxyz"

func geohash(lat, lng float64) string {
	minLat, maxLat, minLng, maxLng := -90.0, 90.0, -180.0, 180.0
	out := make([]byte, 0, ghPrec)
	bit, ch, even := 0, 0, true
	for len(out) < ghPrec {
		if even {
			mid := (minLng + maxLng) / 2
			if lng >= mid {
				ch, minLng = ch<<1|1, mid
			} else {
				ch, maxLng = ch<<1, mid
			}
		} else {
			mid := (minLat + maxLat) / 2
			if lat >= mid {
				ch, minLat = ch<<1|1, mid
			} else {
				ch, maxLat = ch<<1, mid
			}
		}
		even = !even
		if bit++; bit == 5 {
			out = append(out, ghBase32[ch])
			bit, ch = 0, 0
		}
	}
	return string(out)
}

// geohashAround is the cell holding (lat, lng) and its 8 neighbours (deduplicated
// at the poles and the date line).
func geohashAround(lat, lng float64) []string {
	const bits = ghPrec * 5
	dLng := 360 / math.Pow(2, math.Ceil(bits/2.0))
	dLat := 180 / math.Pow(2, math.Floor(bits/2.0))
	seen := map[string]bool{}
	out := make([]string, 0, 9)
	for i := -1; i <= 1; i++ {
		for j := -1; j <= 1; j++ {
			la := max(-90, min(89.999999, lat+float64(i)*dLat))
			lo := math.Mod(lng+float64(j)*dLng+540, 360) - 180
			if g := geohash(la, lo); !seen[g] {
				seen[g] = true
				out = append(out, g)
			}
		}
	}
	return out
}

// distM is the great-circle distance in metres (haversine).
func distM(aLat, aLng, bLat, bLng float64) float64 {
	const r = 6371000.0
	rad := math.Pi / 180
	dLat, dLng := (bLat-aLat)*rad, (bLng-aLng)*rad
	x := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(aLat*rad)*math.Cos(bLat*rad)*math.Sin(dLng/2)*math.Sin(dLng/2)
	return 2 * r * math.Asin(math.Sqrt(x))
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
