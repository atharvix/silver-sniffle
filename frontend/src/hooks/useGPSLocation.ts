import { useState, useEffect, useCallback, useRef } from 'react';
import type { UserProfile } from '../types';
import { syncProfiles } from '../utils/api';
import { Geolocation } from '@capacitor/geolocation';
import { App } from '@capacitor/app';
import { Capacitor } from '@capacitor/core';

export interface GPSState {
  latitude: number | null;
  longitude: number | null;
  /** Reported horizontal accuracy in metres; null until a fix arrives. */
  accuracy: number | null;
  error: string | null;
  loading: boolean;
  permissionGranted: boolean;
  /** True once the held fix is tight enough to publish to a 30m geofence. */
  hasPreciseFix: boolean;
}

/**
 * A fix tighter than this is good enough for a 30-metre geofence.
 * Anything looser must not be published: a 500m network fix puts the user in a
 * random nearby neighbourhood and makes "within 30m" meaningless.
 */
const PRECISE_ACCURACY_M = 50;

/**
 * Beyond this the fix is cell-tower guesswork and is never published or shown as
 * a position, only used to keep the "searching" state honest.
 */
const MAX_TRUSTED_ACCURACY_M = 500;

/**
 * How long the best fix is preferred over newer, looser ones. Without this a
 * temporarily worse signal would drag the reported position around.
 */
const BEST_FIX_TTL_MS = 30_000;

const BLANK_GPS: GPSState = {
  latitude: null,
  longitude: null,
  accuracy: null,
  error: null,
  loading: true,
  permissionGranted: false,
  hasPreciseFix: false,
};

export function useGPSLocation(token?: string) {
  const [gps, setGps] = useState<GPSState>(BLANK_GPS);
  const [profiles, setProfiles] = useState<UserProfile[]>(() => {
    try {
      const cached = localStorage.getItem('kinjo_cached_nearby');
      return cached ? JSON.parse(cached) : [];
    } catch {
      return [];
    }
  });
  const [isLoadingProfiles, setIsLoadingProfiles] = useState(false);
  const [refreshVersion, setRefreshVersion] = useState(0);

  // Best fix held so far, and the last coordinates actually published.
  const bestFixRef = useRef<{ latitude: number; longitude: number; accuracy: number; at: number } | null>(null);
  const publishedRef = useRef<{ latitude: number; longitude: number } | null>(null);

  const [isAppInForeground, setIsAppInForeground] = useState(() => {
    if (typeof document !== 'undefined') {
      return document.visibilityState === 'visible';
    }
    return true;
  });

  // Track foreground/background lifecycle.
  useEffect(() => {
    const handleVisibility = () => {
      setIsAppInForeground(document.visibilityState === 'visible');
    };
    document.addEventListener('visibilitychange', handleVisibility);

    let appStateHandle: any = null;
    if (Capacitor.isNativePlatform()) {
      void App.addListener('appStateChange', (state) => {
        setIsAppInForeground(state.isActive && document.visibilityState === 'visible');
      }).then((h) => {
        appStateHandle = h;
      });
    }

    return () => {
      document.removeEventListener('visibilitychange', handleVisibility);
      if (appStateHandle) void appStateHandle.remove();
    };
  }, []);

  /**
   * Keeps the tightest fix seen, and refuses to be dragged around by looser ones
   * until the held fix goes stale.
   */
  const applyFix = useCallback((latitude: number, longitude: number, accuracy: number) => {
    const now = Date.now();
    const current = bestFixRef.current;
    const withinTrust = accuracy <= MAX_TRUSTED_ACCURACY_M;

    if (withinTrust && current) {
      const isBetter = accuracy < current.accuracy;
      const heldFixIsStale = now - current.at > BEST_FIX_TTL_MS;
      if (!isBetter && !heldFixIsStale) return;
    }

    bestFixRef.current = { latitude, longitude, accuracy, at: now };

    setGps((prev) => ({
      ...prev,
      latitude,
      longitude,
      accuracy: Math.round(accuracy),
      error: null,
      loading: false,
      permissionGranted: true,
      hasPreciseFix: accuracy <= PRECISE_ACCURACY_M,
    }));
  }, []);

  const applyError = useCallback((message: string, permissionDenied: boolean) => {
    setGps((prev) => ({
      ...prev,
      error: message,
      loading: false,
      permissionGranted: permissionDenied ? false : prev.permissionGranted,
    }));
  }, []);

  /**
   * Watches position for as long as the app process lives — foreground OR
   * background. It is deliberately not torn down when the app is backgrounded:
   * that was what made presence look like it "stopped".
   */
  useEffect(() => {
    let cancelled = false;
    let watchId: any = null;

    const startWatching = async () => {
      try {
        if (Capacitor.isNativePlatform()) {
          const permission = await Geolocation.requestPermissions().catch(() => null);
          if (permission && permission.location === 'denied') {
            applyError('Location permission denied. Please allow location access to discover nearby people.', true);
            return;
          }

          // One immediate high-accuracy fix so the deck is not waiting on the
          // watcher's first callback.
          try {
            const first = await Geolocation.getCurrentPosition({
              enableHighAccuracy: true,
              timeout: 15000,
              maximumAge: 0,
            });
            if (!cancelled) {
              applyFix(first.coords.latitude, first.coords.longitude, first.coords.accuracy);
            }
          } catch {
            // The watcher below may still land a fix.
          }

          watchId = await Geolocation.watchPosition(
            { enableHighAccuracy: true, timeout: 15000, maximumAge: 0 },
            (position, err) => {
              if (err || !position) return;
              const { latitude, longitude, accuracy } = position.coords;
              applyFix(latitude, longitude, accuracy);
            }
          );
          return;
        }

        if (navigator.geolocation) {
          watchId = navigator.geolocation.watchPosition(
            (position) => {
              const { latitude, longitude, accuracy } = position.coords;
              applyFix(latitude, longitude, accuracy);
            },
            (error) => {
              const denied = error.code === error.PERMISSION_DENIED;
              applyError(
                denied
                  ? 'Location permission is required to discover nearby people.'
                  : 'Unable to determine your location precisely. Move somewhere with a clearer view of the sky.',
                denied
              );
            },
            { enableHighAccuracy: true, timeout: 15000, maximumAge: 0 }
          );
        }
      } catch (error) {
        applyError(error instanceof Error ? error.message : 'Unable to determine your location.', false);
      }
    };

    void startWatching();

    return () => {
      cancelled = true;
      if (watchId !== null) {
        if (Capacitor.isNativePlatform()) {
          void Geolocation.clearWatch({ id: watchId });
        } else if (navigator.geolocation) {
          navigator.geolocation.clearWatch(watchId);
        }
      }
    };
  }, [applyFix, applyError]);

  const refreshProfiles = useCallback(() => {
    setIsLoadingProfiles(true);
    setRefreshVersion((version) => version + 1);
  }, []);

  /**
   * Polls while the app is in the foreground. In the background the Android
   * foreground service keeps presence fresh natively, so this stops (which also
   * stops the 8-second request train). Returning to the foreground re-runs this
   * effect and syncs immediately.
   */
  useEffect(() => {
    if (!token || !isAppInForeground) {
      // Any refresh already requested must clear its spinner here, or the
      // loading overlay would stay up forever.
      setIsLoadingProfiles(false);
      return;
    }

    let cancelled = false;

    const performSync = async () => {
      if (cancelled || !isAppInForeground) return;
      try {
        const { latitude, longitude, accuracy } = gps;

        // Only publish coordinates the geofence can trust. Without a usable fix
        // the request still refreshes presence and reads the deck from the
        // location the server already holds.
        const publishable =
          latitude !== null &&
          longitude !== null &&
          accuracy !== null &&
          (accuracy <= PRECISE_ACCURACY_M || publishedRef.current === null);
        const moved =
          publishable &&
          (publishedRef.current?.latitude !== latitude ||
            publishedRef.current?.longitude !== longitude);

        const nearby = await syncProfiles(token, moved ? latitude : null, moved ? longitude : null);
        if (moved) publishedRef.current = { latitude: latitude as number, longitude: longitude as number };

        if (!cancelled && Array.isArray(nearby)) {
          setProfiles(nearby);
          try {
            localStorage.setItem('kinjo_cached_nearby', JSON.stringify(nearby));
          } catch { /* quota — the deck still renders from memory */ }
        }
      } catch (err: any) {
        if (err?.status === 403 || err?.message?.toLowerCase().includes('face verification')) {
          window.dispatchEvent(new CustomEvent('kinjo:face_verification_required'));
        }
      } finally {
        if (!cancelled) setIsLoadingProfiles(false);
      }
    };

    void performSync();

    const intervalId = setInterval(() => {
      void performSync();
    }, 8000);

    return () => {
      cancelled = true;
      clearInterval(intervalId);
    };
  }, [gps.latitude, gps.longitude, gps.accuracy, token, refreshVersion, isAppInForeground]);

  return { gps, profiles, isLoadingProfiles, refreshProfiles };
}
