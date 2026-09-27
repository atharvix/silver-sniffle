import { Capacitor, registerPlugin } from '@capacitor/core';
import { API_BASE } from './api';

interface BackgroundLocationPluginInterface {
  start(options: { apiBase: string; token: string }): Promise<void>;
  stop(): Promise<void>;
}

const BackgroundLocation = registerPlugin<BackgroundLocationPluginInterface>('BackgroundLocation');

/**
 * Starts the Android foreground service that keeps location reaching the
 * backend while the app is backgrounded or has been swiped away. No-op on web,
 * where the platform simply does not allow this.
 */
export async function startBackgroundLocation(token: string): Promise<boolean> {
  if (!Capacitor.isNativePlatform() || !token) {
    return false;
  }

  try {
    await BackgroundLocation.start({ apiBase: API_BASE, token });
    return true;
  } catch (err) {
    console.warn('[BackgroundLocation] could not start presence service:', err);
    return false;
  }
}

/** Stops background presence — used on sign-out so we do not linger online. */
export async function stopBackgroundLocation(): Promise<void> {
  if (!Capacitor.isNativePlatform()) {
    return;
  }

  try {
    await BackgroundLocation.stop();
  } catch (err) {
    console.warn('[BackgroundLocation] could not stop presence service:', err);
  }
}
