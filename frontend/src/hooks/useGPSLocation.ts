import { useState, useEffect, useCallback } from 'react';
import { fetchAreaAndCity, type GeoAddress } from '../utils/reverseGeocode';
import type { UserProfile } from '../types';
import { getNearbyProfiles, saveProfile, updateLocation } from '../utils/api';
import { Geolocation } from '@capacitor/geolocation';
import { Capacitor } from '@capacitor/core';
import { TEST_PROFILES } from '../data/testProfiles';


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

export function useGPSLocation(token?: string, userProfile?: UserProfile) {
  const testMode = import.meta.env.VITE_ENABLE_TEST_MODE === 'true';
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

  // Use the native provider in the APK and browser GPS on the web. Never use IP location.
  const resetToAutoGPS = useCallback(async () => {
    localStorage.removeItem('kinjo_user_location');
    setGps((prev) => ({ ...prev, loading: true }));

    const setDeviceLocation = async (latitude: number, longitude: number, accuracy: number) => {
      const geoResult = await fetchAreaAndCity(latitude, longitude);
      setGps({ latitude, longitude, accuracy: Math.round(accuracy), areaName: geoResult.area, cityName: geoResult.city, formattedLocation: geoResult.formatted, error: null, loading: false, permissionGranted: true, isCustomOverride: false });
    };

    try {
      if (Capacitor.isNativePlatform()) {
        const permission = await Geolocation.requestPermissions();
        if (permission.location === 'denied') throw new Error('Location permission denied');
        const position = await Geolocation.getCurrentPosition({ enableHighAccuracy: true, timeout: 10000 });
        await setDeviceLocation(position.coords.latitude, position.coords.longitude, position.coords.accuracy);
        return;
      }

      if (navigator.geolocation) {
        let browserLocationResolved = false;
        await new Promise<void>((resolve) => {
          navigator.geolocation.getCurrentPosition(
            async (position) => {
              const { latitude, longitude, accuracy } = position.coords;
              await setDeviceLocation(latitude, longitude, accuracy);
              browserLocationResolved = true;
              resolve();
            },
            () => resolve(),
            { enableHighAccuracy: true, timeout: 10000, maximumAge: 0 }
          );
        });
        if (browserLocationResolved) return;
      }
      throw new Error('Device location is unavailable');
    } catch (error) {
      setGps((prev) => ({ ...prev, latitude: null, longitude: null, accuracy: null, areaName: 'Location unavailable', cityName: '', formattedLocation: 'Enable device location', error: error instanceof Error ? error.message : 'Device location unavailable', loading: false, permissionGranted: false, isCustomOverride: false }));
    }
  }, []);

  // Initial Geolocation check on mount if no custom location is saved
  useEffect(() => {
    if (gps.isCustomOverride) return;

    void resetToAutoGPS();
  }, [gps.isCustomOverride]);

  useEffect(() => {
    if (!token || !userProfile || gps.latitude === null || gps.longitude === null) {
      setProfiles(testMode ? TEST_PROFILES : []);
      return;
    }

    let cancelled = false;
    const profileRequest = userProfile ? saveProfile(userProfile, token) : Promise.resolve();
    void profileRequest
      .then(() => updateLocation(gps.latitude!, gps.longitude!, token))
      .then(() => getNearbyProfiles(token))
      .then((nearby) => {
        if (!cancelled) setProfiles(nearby);
      })
      .catch(() => {
        if (!cancelled && testMode) setProfiles(TEST_PROFILES);
      });

    return () => {
      cancelled = true;
    };
  }, [gps.latitude, gps.longitude, token, userProfile, testMode]);

  return { gps, profiles, setProfiles, setCustomLocation, resetToAutoGPS };
}
