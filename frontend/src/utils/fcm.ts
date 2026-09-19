import { Capacitor, registerPlugin } from '@capacitor/core';
import { registerDeviceToken } from './api';

interface FCMNativePluginInterface {
  getFCMToken(): Promise<{ token: string }>;
  checkNotificationPermission(): Promise<{ granted: boolean }>;
}

const FCMNative = registerPlugin<FCMNativePluginInterface>('FCMNative');

/**
 * Initializes FCM push notifications on native devices.
 * Retrieves the device registration token from the native Android Firebase SDK
 * and registers it with the Kinjo Go backend.
 */
export async function initializeFCM(authToken?: string): Promise<string | null> {
  if (!Capacitor.isNativePlatform()) {
    return null;
  }

  try {
    const result = await FCMNative.getFCMToken();
    const fcmToken = result?.token;

    if (!fcmToken) {
      console.warn('[FCM] No registration token received from native plugin.');
      return null;
    }

    console.info('[FCM] Native registration token acquired:', fcmToken.substring(0, 10) + '...');

    // If an auth token is provided, sync with backend
    if (authToken) {
      const lastSynced = localStorage.getItem('kinjo_fcm_registered_token');
      if (lastSynced !== fcmToken) {
        try {
          const res = await registerDeviceToken(authToken, fcmToken, 'android');
          if (res.success) {
            localStorage.setItem('kinjo_fcm_registered_token', fcmToken);
            console.info('[FCM] Device token successfully registered with Kinjo backend.');
          }
        } catch (err) {
          console.error('[FCM] Failed to register device token with backend:', err);
        }
      }
    }

    return fcmToken;
  } catch (err) {
    console.error('[FCM] Error initializing FCM:', err);
    return null;
  }
}
