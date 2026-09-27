import { Capacitor, registerPlugin } from '@capacitor/core';
import { StatusBar, Style } from '@capacitor/status-bar';

/**
 * Opens this app's OS settings screen. Android (and iOS) only ever show a
 * runtime permission dialog once: after a denial the prompt never appears
 * again, so the only way back for the user is this screen. The onboarding
 * steps offer it as the recovery action instead of leaving a dead end.
 */
interface AppSettingsPlugin {
  open(): Promise<void>;
}

const AppSettings = registerPlugin<AppSettingsPlugin>('AppSettings');

export async function openAppSettings(): Promise<void> {
  if (!Capacitor.isNativePlatform()) return;
  try {
    await AppSettings.open();
  } catch {
    // Older build without the plugin — nothing reachable from the WebView.
  }
}

/**
 * Mirrors the in-app theme onto the native status bar (battery, clock, signal).
 *
 * Without this the bar follows the *device* colour scheme rather than the app's,
 * so choosing the light theme on a dark-mode phone left light icons on a white
 * bar — unreadable. Note the enum reads backwards on purpose:
 * `Style.Dark` is light text (for dark backgrounds).
 */
export async function syncStatusBar(isDark: boolean): Promise<void> {
  if (!Capacitor.isNativePlatform()) return;
  try {
    await StatusBar.setStyle({ style: isDark ? Style.Dark : Style.Light });
  } catch {
    // Not fatal: the platform default is still readable enough to ship.
  }
}
