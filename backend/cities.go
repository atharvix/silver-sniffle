package main

import "math"

// A small offline gazetteer: the city a person joined from is the nearest centre
// (within its radius) to their first fix. Only the city name is stored, never the fix,
// and no geocoding service is called.
type cityRef struct {
	Name, Country string
	Lat, Lng      float64
	RadiusKm      float64
}

var cityRefs = []cityRef{
	{"Jaipur", "India", 26.9124, 75.7873, 35}, {"Delhi", "India", 28.6139, 77.2090, 40}, {"Gurugram", "India", 28.4595, 77.0266, 18},
	{"Noida", "India", 28.5355, 77.3910, 18}, {"Ghaziabad", "India", 28.6692, 77.4538, 14}, {"Faridabad", "India", 28.4089, 77.3178, 14},
	{"Mumbai", "India", 19.0760, 72.8777, 40}, {"Navi Mumbai", "India", 19.0330, 73.0297, 15}, {"Thane", "India", 19.2183, 72.9781, 12},
	{"Bengaluru", "India", 12.9716, 77.5946, 35}, {"Hyderabad", "India", 17.3850, 78.4867, 35}, {"Chennai", "India", 13.0827, 80.2707, 35},
	{"Pune", "India", 18.5204, 73.8567, 30}, {"Kolkata", "India", 22.5726, 88.3639, 30}, {"Ahmedabad", "India", 23.0225, 72.5714, 30},
	{"Gandhinagar", "India", 23.2156, 72.6369, 12}, {"Vadodara", "India", 22.3072, 73.1812, 18}, {"Rajkot", "India", 22.3039, 70.8022, 15},
	{"Udaipur", "India", 24.5854, 73.7125, 20}, {"Jodhpur", "India", 26.2389, 73.0243, 20}, {"Kota", "India", 25.2138, 75.8648, 18},
	{"Ajmer", "India", 26.4499, 74.6399, 15}, {"Bikaner", "India", 28.0229, 73.3119, 15}, {"Chandigarh", "India", 30.7333, 76.7794, 20},
	{"Ludhiana", "India", 30.9010, 75.8573, 18}, {"Amritsar", "India", 31.6340, 74.8723, 15}, {"Lucknow", "India", 26.8467, 80.9462, 25},
	{"Kanpur", "India", 26.4499, 80.3319, 20}, {"Varanasi", "India", 25.3176, 82.9739, 15}, {"Agra", "India", 27.1767, 78.0081, 15},
	{"Indore", "India", 22.7196, 75.8577, 22}, {"Bhopal", "India", 23.2599, 77.4126, 22}, {"Surat", "India", 21.1702, 72.8311, 22},
	{"Kochi", "India", 9.9312, 76.2673, 20}, {"Goa", "India", 15.4909, 73.8278, 30}, {"Nagpur", "India", 21.1458, 79.0882, 22},
	{"Dehradun", "India", 30.3165, 78.0322, 15}, {"Bhubaneswar", "India", 20.2961, 85.8245, 18}, {"Coimbatore", "India", 11.0168, 76.9558, 20},
	{"Visakhapatnam", "India", 17.6868, 83.2185, 20}, {"Thiruvananthapuram", "India", 8.5241, 76.9366, 18}, {"Guwahati", "India", 26.1445, 91.7362, 18},
	{"Patna", "India", 25.5941, 85.1376, 18}, {"Ranchi", "India", 23.3441, 85.3096, 15}, {"Raipur", "India", 21.2514, 81.6296, 15},
	{"Mysuru", "India", 12.2958, 76.6394, 15}, {"Mangaluru", "India", 12.9141, 74.8560, 15}, {"Madurai", "India", 9.9252, 78.1198, 15},
	{"Vijayawada", "India", 16.5062, 80.6480, 15}, {"Nashik", "India", 19.9975, 73.7898, 15}, {"Aurangabad", "India", 19.8762, 75.3433, 15},
	{"Srinagar", "India", 34.0837, 74.7973, 15}, {"Jammu", "India", 32.7266, 74.8570, 15}, {"Shimla", "India", 31.1048, 77.1734, 12},
	{"Dubai", "United Arab Emirates", 25.2048, 55.2708, 40}, {"Abu Dhabi", "United Arab Emirates", 24.4539, 54.3773, 35},
	{"Singapore", "Singapore", 1.3521, 103.8198, 30}, {"London", "United Kingdom", 51.5072, -0.1276, 40},
	{"New York", "United States", 40.7128, -74.0060, 40}, {"San Francisco", "United States", 37.7749, -122.4194, 35},
	{"Toronto", "Canada", 43.6532, -79.3832, 35}, {"Sydney", "Australia", -33.8688, 151.2093, 40},
	{"Berlin", "Germany", 52.5200, 13.4050, 30}, {"Paris", "France", 48.8566, 2.3522, 30},
}

// nearestCity: the closest known city whose radius covers the point ("" when none does).
func nearestCity(lat, lng float64) (string, string) {
	best, bestD := -1, math.MaxFloat64
	for i, c := range cityRefs {
		d := distM(lat, lng, c.Lat, c.Lng) / 1000
		if d <= c.RadiusKm && d < bestD {
			best, bestD = i, d
		}
	}
	if best < 0 {
		return "", ""
	}
	return cityRefs[best].Name, cityRefs[best].Country
}

// citiesWithin: known cities whose centre lies inside a circle (for "people from around here").
func citiesWithin(lat, lng, radiusM float64) []string {
	var out []string
	for _, c := range cityRefs {
		if distM(lat, lng, c.Lat, c.Lng) <= radiusM {
			out = append(out, c.Name)
		}
	}
	return out
}

// cityState: the state / region of each gazetteer city (for grouping in the admin).
var cityState = map[string]string{
	"Jaipur": "Rajasthan", "Udaipur": "Rajasthan", "Jodhpur": "Rajasthan", "Kota": "Rajasthan", "Ajmer": "Rajasthan", "Bikaner": "Rajasthan",
	"Delhi": "Delhi", "Gurugram": "Haryana", "Faridabad": "Haryana", "Noida": "Uttar Pradesh", "Ghaziabad": "Uttar Pradesh",
	"Lucknow": "Uttar Pradesh", "Kanpur": "Uttar Pradesh", "Varanasi": "Uttar Pradesh", "Agra": "Uttar Pradesh",
	"Mumbai": "Maharashtra", "Navi Mumbai": "Maharashtra", "Thane": "Maharashtra", "Pune": "Maharashtra", "Nagpur": "Maharashtra",
	"Nashik": "Maharashtra", "Aurangabad": "Maharashtra", "Bengaluru": "Karnataka", "Mysuru": "Karnataka", "Mangaluru": "Karnataka",
	"Hyderabad": "Telangana", "Chennai": "Tamil Nadu", "Coimbatore": "Tamil Nadu", "Madurai": "Tamil Nadu", "Kolkata": "West Bengal",
	"Ahmedabad": "Gujarat", "Gandhinagar": "Gujarat", "Vadodara": "Gujarat", "Rajkot": "Gujarat", "Surat": "Gujarat",
	"Chandigarh": "Chandigarh", "Ludhiana": "Punjab", "Amritsar": "Punjab", "Indore": "Madhya Pradesh", "Bhopal": "Madhya Pradesh",
	"Kochi": "Kerala", "Thiruvananthapuram": "Kerala", "Goa": "Goa", "Dehradun": "Uttarakhand", "Bhubaneswar": "Odisha",
	"Visakhapatnam": "Andhra Pradesh", "Vijayawada": "Andhra Pradesh", "Guwahati": "Assam", "Patna": "Bihar", "Ranchi": "Jharkhand",
	"Raipur": "Chhattisgarh", "Srinagar": "Jammu and Kashmir", "Jammu": "Jammu and Kashmir", "Shimla": "Himachal Pradesh",
	"Dubai": "Dubai", "Abu Dhabi": "Abu Dhabi", "Singapore": "Singapore", "London": "England", "New York": "New York",
	"San Francisco": "California", "Toronto": "Ontario", "Sydney": "New South Wales", "Berlin": "Berlin", "Paris": "Île-de-France",
}

// Neighbourhood precision for the admin (founder's call, 2026-11-18): a live position is
// shown only as the centre of its ~550 m grid square, never the fix, and never stored.
const areaCell = 0.005

func snapArea(lat, lng float64) (float64, float64) {
	r := func(v float64) float64 { return math.Round(((math.Floor(v/areaCell)+.5)*areaCell)*1e5) / 1e5 }
	return r(lat), r(lng)
}
