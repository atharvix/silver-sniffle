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
  const [gps, setGps] = useState<GPSState>(() => {
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
      latitude: 26.9124,
      longitude: 75.7873,
      accuracy: 5,
      areaName: 'Jaipur',
      cityName: 'Malviya Nagar',
      formattedLocation: 'Jaipur, Malviya Nagar',
      error: null,
      loading: false,
      permissionGranted: true,
      isCustomOverride: false,
    };
  });

  // Default to all 10 mock profiles so web and mobile always show full card deck
  const [profiles, setProfiles] = useState<UserProfile[]>(TEST_PROFILES);

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

  const resetToAutoGPS = useCallback(async () => {
    localStorage.removeItem('kinjo_user_location');
    setGps((prev) => ({ ...prev, loading: true }));

    const setDeviceLocation = async (latitude: number, longitude: number, accuracy: number) => {
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
    } catch (error) {
      setGps((prev) => ({
        ...prev,
        loading: false,
      }));
    }
  }, []);

  useEffect(() => {
    if (gps.isCustomOverride) return;
    void resetToAutoGPS();

    // Continuous watchPosition as the user moves
    let watchId: any = null;

    const startWatching = async () => {
      try {
        if (Capacitor.isNativePlatform()) {
          watchId = await Geolocation.watchPosition(
            { enableHighAccuracy: true, timeout: 10000, maximumAge: 3000 },
            async (position, err) => {
              if (position && !err) {
                const { latitude, longitude, accuracy } = position.coords;
                const geoResult = await fetchAreaAndCity(latitude, longitude);
                setGps((prev) => ({
                  ...prev,
                  latitude,
                  longitude,
                  accuracy: Math.round(accuracy),
                  areaName: geoResult.area,
                  cityName: geoResult.city,
                  formattedLocation: geoResult.formatted,
                }));
              }
            }
          );
        } else if (navigator.geolocation) {
          watchId = navigator.geolocation.watchPosition(
            async (position) => {
              const { latitude, longitude, accuracy } = position.coords;
              const geoResult = await fetchAreaAndCity(latitude, longitude);
              setGps((prev) => ({
                ...prev,
                latitude,
                longitude,
                accuracy: Math.round(accuracy),
                areaName: geoResult.area,
                cityName: geoResult.city,
                formattedLocation: geoResult.formatted,
              }));
            },
            () => {},
            { enableHighAccuracy: true, timeout: 10000, maximumAge: 3000 }
          );
        }
      } catch (e) {}
    };

    void startWatching();

    return () => {
      if (watchId !== null) {
        if (Capacitor.isNativePlatform()) {
          Geolocation.clearWatch({ id: watchId });
        } else if (navigator.geolocation) {
          navigator.geolocation.clearWatch(watchId);
        }
      }
    };
  }, [gps.isCustomOverride]);

  useEffect(() => {
    if (!token || !userProfile || gps.latitude === null || gps.longitude === null) {
      setProfiles(TEST_PROFILES);
      return;
    }

    let cancelled = false;
    const profileRequest = userProfile ? saveProfile(userProfile, token) : Promise.resolve();
    void profileRequest
      .then(() => updateLocation(gps.latitude!, gps.longitude!, token))
      .then(() => getNearbyProfiles(token))
      .then((nearby) => {
        if (!cancelled) {
          setProfiles(nearby && nearby.length > 0 ? nearby : TEST_PROFILES);
        }
      })
      .catch(() => {
        if (!cancelled) setProfiles(TEST_PROFILES);
      });

    return () => {
      cancelled = true;
    };
  }, [gps.latitude, gps.longitude, token, userProfile]);

  return { gps, profiles, setProfiles, setCustomLocation, resetToAutoGPS };
}
