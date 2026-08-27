import { useState, useEffect, useCallback } from 'react';
import { calculateDistanceMeters, getOffsetCoordinates } from '../utils/geo';
import { fetchAreaAndCity, type GeoAddress } from '../utils/reverseGeocode';
import type { UserProfile } from '../types';


export interface GPSState {
  latitude: number | null;
  longitude: number | null;
  accuracy: number | null;
  areaName: string;
  cityName: string;
  formattedLocation: string;
  error: string | null;
  loading: boolean;
  permissionGranted: boolean;
  isCustomOverride: boolean;
}

export function useGPSLocation(initialProfiles: UserProfile[]) {
  const [gps, setGps] = useState<GPSState>(() => {
    // Check localStorage for saved location override
    const saved = localStorage.getItem('kinjo_user_location');
    if (saved) {
      try {
        const parsed: GeoAddress = JSON.parse(saved);
        return {
          latitude: parsed.latitude,
          longitude: parsed.longitude,
          accuracy: 5,
          areaName: parsed.area,
          cityName: parsed.city,
          formattedLocation: parsed.formatted,
          error: null,
          loading: false,
          permissionGranted: true,
          isCustomOverride: true,
        };
      } catch (e) {}
    }

    return {
      latitude: null,
      longitude: null,
      accuracy: null,
      areaName: 'Locating...',
      cityName: '',
      formattedLocation: 'Locating area...',
      error: null,
      loading: true,
      permissionGranted: false,
      isCustomOverride: false,
    };
  });

  const [profiles, setProfiles] = useState<UserProfile[]>([]);

  // Function to manually set location (e.g. Jaipur, Malviya Nagar)
  const setCustomLocation = useCallback((location: GeoAddress) => {
    localStorage.setItem('kinjo_user_location', JSON.stringify(location));
    setGps({
      latitude: location.latitude,
      longitude: location.longitude,
      accuracy: 5,
      areaName: location.area,
      cityName: location.city,
      formattedLocation: location.formatted,
      error: null,
      loading: false,
      permissionGranted: true,
      isCustomOverride: true,
    });
  }, []);

  // Reset custom location and auto-detect GPS / IP location
  const resetToAutoGPS = useCallback(async () => {
    localStorage.removeItem('kinjo_user_location');
    setGps((prev) => ({ ...prev, loading: true }));

    if (navigator.geolocation) {
      navigator.geolocation.getCurrentPosition(
        async (position) => {
          const { latitude, longitude, accuracy } = position.coords;
          const geoResult = await fetchAreaAndCity(latitude, longitude);
          setGps({
            latitude,
            longitude,
            accuracy: Math.round(accuracy),
            areaName: geoResult.area,
            cityName: geoResult.city,
            formattedLocation: geoResult.formatted,
            error: null,
            loading: false,
            permissionGranted: true,
            isCustomOverride: false,
          });
        },
        async () => {
          const fallbackGeo = await fetchAreaAndCity();
          setGps({
            latitude: fallbackGeo.latitude,
            longitude: fallbackGeo.longitude,
            accuracy: 5,
            areaName: fallbackGeo.area,
            cityName: fallbackGeo.city,
            formattedLocation: fallbackGeo.formatted,
            error: 'GPS error',
            loading: false,
            permissionGranted: false,
            isCustomOverride: false,
          });
        },
        { enableHighAccuracy: true, timeout: 10000, maximumAge: 0 }
      );
    } else {
      const fallbackGeo = await fetchAreaAndCity();
      setGps({
        latitude: fallbackGeo.latitude,
        longitude: fallbackGeo.longitude,
        accuracy: 5,
        areaName: fallbackGeo.area,
        cityName: fallbackGeo.city,
        formattedLocation: fallbackGeo.formatted,
        error: null,
        loading: false,
        permissionGranted: false,
        isCustomOverride: false,
      });
    }
  }, []);

  // Initial Geolocation check on mount if no custom location is saved
  useEffect(() => {
    if (gps.isCustomOverride) return;

    (async () => {
      const initialGeo = await fetchAreaAndCity();
      setGps((prev) => {
        if (prev.isCustomOverride) return prev;
        return {
          ...prev,
          latitude: prev.latitude || initialGeo.latitude || 26.9124,
          longitude: prev.longitude || initialGeo.longitude || 75.7873,
          areaName: initialGeo.area,
          cityName: initialGeo.city,
          formattedLocation: initialGeo.formatted,
          loading: false,
        };
      });
    })();
  }, [gps.isCustomOverride]);

  // Update mock profiles around user's coordinates strictly within 30m radius
  const updateProfilesAroundUser = useCallback(() => {
    const baseLat = gps.latitude || 26.9124; // Jaipur coordinates
    const baseLng = gps.longitude || 75.7873;

    const angles = [35, 110, 195, 270, 320];
    const distances = [4, 11, 17, 23, 29]; // Strictly <= 30m

    const updated = initialProfiles.map((prof, i) => {
      const targetDist = distances[i % distances.length];
      const angle = angles[i % angles.length];

      const coords = getOffsetCoordinates(baseLat, baseLng, targetDist, angle);
      const actualDist = calculateDistanceMeters(
        baseLat,
        baseLng,
        coords.latitude,
        coords.longitude
      );

      const clampedDist = Math.min(30, Math.max(1, actualDist));

      return {
        ...prof,
        latitude: coords.latitude,
        longitude: coords.longitude,
        distanceMeters: clampedDist,
        locationName: `${clampedDist}m away • ${gps.areaName || 'Nearby'}`,
      };
    });

    setProfiles(updated);
  }, [gps.latitude, gps.longitude, gps.areaName, initialProfiles]);

  useEffect(() => {
    updateProfilesAroundUser();
  }, [updateProfilesAroundUser]);

  // Subtle real-time pulse interval
  useEffect(() => {
    const interval = setInterval(() => {
      setProfiles((prevProfiles) =>
        prevProfiles.map((p) => {
          const delta = (Math.random() > 0.5 ? 1 : -1) * (Math.random() > 0.7 ? 1 : 0);
          const newDist = Math.min(30, Math.max(2, p.distanceMeters + delta));
          return {
            ...p,
            distanceMeters: newDist,
            locationName: `${newDist}m away • Live in 30m radius`,
          };
        })
      );
    }, 4000);

    return () => clearInterval(interval);
  }, []);

  return { gps, profiles, setProfiles, setCustomLocation, resetToAutoGPS };
}
