import { useState, useEffect, useCallback, useRef } from 'react';
import { fetchAreaAndCity, type GeoAddress } from '../utils/reverseGeocode';
import type { UserProfile } from '../types';
import { getNearbyProfiles, updateLocation } from '../utils/api';
import { Geolocation } from '@capacitor/geolocation';
import { App } from '@capacitor/app';
import { Capacitor } from '@capacitor/core';

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

export function useGPSLocation(token?: string) {
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
      } catch {}
    }

    return {
      latitude: null,
      longitude: null,
      accuracy: null,
      areaName: '',
      cityName: '',
      formattedLocation: '',
      error: null,
      loading: false,
      permissionGranted: false,
      isCustomOverride: false,
    };
  });

  const [profiles, setProfiles] = useState<UserProfile[]>([]);
  const [isLoadingProfiles, setIsLoadingProfiles] = useState(false);
  const [refreshVersion, setRefreshVersion] = useState(0);

  const refreshProfiles = useCallback(() => {
    setIsLoadingProfiles(true);
    setRefreshVersion((version) => version + 1);
  }, []);

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
    setRefreshVersion((v) => v + 1);
  }, []);

  const resetToAutoGPS = useCallback(async () => {
    localStorage.removeItem('kinjo_user_location');
    setGps((prev) => ({ ...prev, loading: true }));

    const setDeviceLocation = (latitude: number, longitude: number, accuracy: number) => {
      setGps((prev) => ({
        ...prev,
        latitude,
        longitude,
        accuracy: Math.round(accuracy),
        error: null,
        loading: false,
        permissionGranted: true,
        isCustomOverride: false,
      }));
      setRefreshVersion((v) => v + 1);

      // Asynchronously update area/city names without blocking numeric lat/lon coordinates
      void fetchAreaAndCity(latitude, longitude)
        .then((geoResult) => {
          setGps((prev) => ({
            ...prev,
            areaName: geoResult.area,
            cityName: geoResult.city,
            formattedLocation: geoResult.formatted,
          }));
        })
        .catch(() => {});
    };

    try {
      if (Capacitor.isNativePlatform()) {
        const permission = await Geolocation.requestPermissions();
        if (permission.location === 'denied') throw new Error('Location permission denied');
        
        let position;
        try {
          position = await Geolocation.getCurrentPosition({ enableHighAccuracy: true, timeout: 10000 });
        } catch {
          position = await Geolocation.getCurrentPosition({ enableHighAccuracy: false, timeout: 15000 });
        }
        setDeviceLocation(position.coords.latitude, position.coords.longitude, position.coords.accuracy);
        return;
      }

      if (navigator.geolocation) {
        let browserLocationResolved = false;
        await new Promise<void>((resolve) => {
          navigator.geolocation.getCurrentPosition(
            (position) => {
              const { latitude, longitude, accuracy } = position.coords;
              setDeviceLocation(latitude, longitude, accuracy);
              browserLocationResolved = true;
              resolve();
            },
            () => {
              setGps((prev) => ({
                ...prev,
                error: 'Location permission is required to discover nearby people.',
                permissionGranted: false,
              }));
              resolve();
            },
            { enableHighAccuracy: true, timeout: 10000, maximumAge: 0 }
          );
        });
        if (browserLocationResolved) return;
      }
    } catch (error) {
      setGps((prev) => ({
        ...prev,
        error: error instanceof Error ? error.message : 'Unable to determine your location.',
        permissionGranted: false,
      }));
    } finally {
      setGps((prev) => ({
        ...prev,
        loading: false,
      }));
    }
  }, []);

  useEffect(() => {
    if (gps.isCustomOverride) return;
    void resetToAutoGPS();

    let watchId: any = null;

    const startWatching = async () => {
      try {
        if (Capacitor.isNativePlatform()) {
          watchId = await Geolocation.watchPosition(
            { enableHighAccuracy: true, timeout: 8000, maximumAge: 0 },
            (position, err) => {
              if (position && !err) {
                const { latitude, longitude, accuracy } = position.coords;
                setGps((prev) => ({
                  ...prev,
                  latitude,
                  longitude,
                  accuracy: Math.round(accuracy),
                }));
              }
            }
          );
        } else if (navigator.geolocation) {
          watchId = navigator.geolocation.watchPosition(
            (position) => {
              const { latitude, longitude, accuracy } = position.coords;
              setGps((prev) => ({
                ...prev,
                latitude,
                longitude,
                accuracy: Math.round(accuracy),
              }));
            },
            () => {},
            { enableHighAccuracy: true, timeout: 8000, maximumAge: 0 }
          );
        }
      } catch (error) {
        console.warn('watchPosition warning:', error);
      }
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

  const lastSyncedLoc = useRef<{ lat: number; lon: number } | null>(null);

  useEffect(() => {
    if (!token) return;

    let cancelled = false;

    const performSync = async () => {
      if (cancelled) return;
      try {
        const lat = gps.latitude;
        const lon = gps.longitude;
        if (lat !== null && lon !== null) {
          await updateLocation(lat, lon, token).catch(() => {});
          lastSyncedLoc.current = { lat, lon };
        }
        const nearby = await getNearbyProfiles(token, lat, lon);
        if (!cancelled && Array.isArray(nearby)) {
          setProfiles(nearby);
        }
      } catch (err: any) {
        // Keep existing/cached profiles on transient error
      } finally {
        if (!cancelled) setIsLoadingProfiles(false);
      }
    };

    void performSync();

    const intervalId = setInterval(() => {
      if (gps.latitude !== null && gps.longitude !== null && lastSyncedLoc.current) {
        const dLat = (gps.latitude - lastSyncedLoc.current.lat) * 111000;
        const dLon = (gps.longitude - lastSyncedLoc.current.lon) * 111000 * Math.cos((gps.latitude * Math.PI) / 180);
        const dist = Math.sqrt(dLat * dLat + dLon * dLon);
        if (dist > 10) {
          void performSync();
          return;
        }
      }
    }, 30000);

    const handleVisibility = () => {
      if (document.visibilityState === 'visible') {
        void performSync();
      }
    };
    document.addEventListener('visibilitychange', handleVisibility);

    let appStateHandle: any = null;
    if (Capacitor.isNativePlatform()) {
      void App.addListener('appStateChange', () => {
        void performSync();
      }).then((h) => {
        appStateHandle = h;
      });
    }

    return () => {
      cancelled = true;
      clearInterval(intervalId);
      document.removeEventListener('visibilitychange', handleVisibility);
      if (appStateHandle) {
        void appStateHandle.remove();
      }
    };
  }, [gps.latitude, gps.longitude, token, refreshVersion]);

  return { gps, profiles, isLoadingProfiles, setProfiles, setCustomLocation, resetToAutoGPS, refreshProfiles };
}
