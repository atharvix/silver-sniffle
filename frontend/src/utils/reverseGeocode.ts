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
    } catch (err) {
      console.warn('Nominatim reverse geocode warning:', err);
    }
  }

  if (lat !== undefined && lng !== undefined) {
    return {
      area: 'Current location',
      city: 'GPS',
      formatted: 'Current location',
      latitude: lat,
      longitude: lng,
    };
  }

  return {
    area: 'Location unavailable',
    city: '',
    formatted: 'Location unavailable',
    latitude: 0,
    longitude: 0,
  };
}
