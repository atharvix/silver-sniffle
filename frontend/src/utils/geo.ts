// Haversine formula to compute distance in meters between two lat/lng points
export function calculateDistanceMeters(
  lat1: number,
  lon1: number,
  lat2: number,
  lon2: number
): number {
  const R = 6371000; // Earth radius in meters
  const dLat = ((lat2 - lat1) * Math.PI) / 180;
  const dLon = ((lon2 - lon1) * Math.PI) / 180;
  const a =
    Math.sin(dLat / 2) * Math.sin(dLat / 2) +
    Math.cos((lat1 * Math.PI) / 180) *
      Math.cos((lat2 * Math.PI) / 180) *
      Math.sin(dLon / 2) *
      Math.sin(dLon / 2);
  const c = 2 * Math.atan2(Math.sqrt(a), Math.sqrt(1 - a));
  return Math.round(R * c);
}

// Generate offset lat/lng within a specific radius (meters) from center
export function getOffsetCoordinates(
  baseLat: number,
  baseLng: number,
  offsetMeters: number,
  angleDegrees: number
) {
  const earthRadius = 6371000;
  const angleRad = (angleDegrees * Math.PI) / 180;
  const dLat = (offsetMeters * Math.cos(angleRad)) / earthRadius;
  const dLng =
    (offsetMeters * Math.sin(angleRad)) /
    (earthRadius * Math.cos((baseLat * Math.PI) / 180));

  return {
    latitude: baseLat + (dLat * 180) / Math.PI,
    longitude: baseLng + (dLng * 180) / Math.PI,
  };
}
