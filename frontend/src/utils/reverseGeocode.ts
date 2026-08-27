export interface GeoAddress {
  area: string;
  city: string;
  formatted: string;
  latitude: number;
  longitude: number;
}

// Reverse Geocode latitude/longitude to Area and City using multi-provider fallback
export async function fetchAreaAndCity(
  lat?: number,
  lng?: number
): Promise<GeoAddress> {
  // If coordinates provided, reverse geocode via BigDataCloud & Nominatim
  if (lat !== undefined && lng !== undefined) {
    try {
      const response = await fetch(
        `https://api.bigdatacloud.net/data/reverse-geocode-client?latitude=${lat}&longitude=${lng}&localityLanguage=en`
      );
      if (response.ok) {
        const data = await response.json();
        const area =
          data.locality ||
          data.suburb ||
          data.city ||
          data.principalSubdivision ||
          'Local Area';

        const city =
          data.city ||
          data.locality ||
          data.principalSubdivision ||
          'City';

        const formatted = area !== city && area ? `${area}, ${city}` : city;

        if (formatted && formatted !== 'City') {
          return {
            area,
            city,
            formatted,
            latitude: lat,
            longitude: lng,
          };
        }
      }
    } catch (e) {
      console.warn('BigDataCloud reverse geocode warning:', e);
    }

    // Nominatim fallback
    try {
      const nomRes = await fetch(
        `https://nominatim.openstreetmap.org/reverse?format=jsonv2&lat=${lat}&lon=${lng}`
      );
      if (nomRes.ok) {
        const nomData = await nomRes.json();
        const addr = nomData.address || {};
        const area = addr.suburb || addr.neighbourhood || addr.quarter || addr.city || 'Nearby Area';
        const city = addr.city || addr.town || addr.state || 'City';
        return {
          area,
          city,
          formatted: `${area}, ${city}`,
          latitude: lat,
          longitude: lng,
        };
      }
    } catch (err) {}
  }

  // IP Location Lookup (Detects real city like Jaipur automatically from IP)
  try {
    const ipRes = await fetch('https://ipapi.co/json/');
    if (ipRes.ok) {
      const ipData = await ipRes.json();
      if (ipData.city) {
        const area = ipData.city;
        const city = ipData.region || ipData.country_name || 'Rajasthan';
        return {
          area,
          city,
          formatted: `${area}, ${city}`,
          latitude: ipData.latitude || 26.9124,
          longitude: ipData.longitude || 75.7873,
        };
      }
    }
  } catch (e) {}

  try {
    const ipRes2 = await fetch('https://ip-api.com/json/');
    if (ipRes2.ok) {
      const ipData2 = await ipRes2.json();
      if (ipData2.city) {
        return {
          area: ipData2.city,
          city: ipData2.regionName || 'Rajasthan',
          formatted: `${ipData2.city}, ${ipData2.regionName || 'Rajasthan'}`,
          latitude: ipData2.lat || 26.9124,
          longitude: ipData2.lon || 75.7873,
        };
      }
    }
  } catch (e) {}

  // Final default coordinates (Jaipur City Center)
  return {
    area: 'Jaipur',
    city: 'Rajasthan',
    formatted: 'Jaipur, Rajasthan',
    latitude: 26.9124,
    longitude: 75.7873,
  };
}

// Forward Geocoding Search (Allows searching any area/city like Jaipur, Malviya Nagar, etc.)
export async function searchLocationByName(query: string): Promise<GeoAddress[]> {
  if (!query || query.trim().length < 2) return [];

  try {
    const res = await fetch(
      `https://nominatim.openstreetmap.org/search?format=jsonv2&q=${encodeURIComponent(query)}&limit=5`
    );
    if (res.ok) {
      const results = await res.json();
      return results.map((item: any) => {
        const nameParts = item.display_name.split(',');
        const area = nameParts[0]?.trim() || query;
        const city = nameParts[1]?.trim() || nameParts[2]?.trim() || 'City';
        return {
          area,
          city,
          formatted: `${area}, ${city}`,
          latitude: parseFloat(item.lat),
          longitude: parseFloat(item.lon),
        };
      });
    }
  } catch (e) {
    console.warn('Location search error:', e);
  }

  return [];
}
