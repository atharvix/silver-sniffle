import type { CapacitorConfig } from '@capacitor/cli';

const config: CapacitorConfig = {
  appId: 'com.kinjo.app',
  appName: 'Kinjo',
  webDir: 'dist',
  plugins: {
    // Applied natively before the WebView loads, so the bar does not flash the
    // wrong colour on a cold start; App.tsx keeps it in sync with the app theme.
    StatusBar: {
      style: 'DARK',
    },
    GoogleAuth: {
      scopes: ['profile', 'email'],
      serverClientId: '469545347988-vsu4c3rvqh6tcelvm8c1sce13ea5dopc.apps.googleusercontent.com',
      forceCodeForRefreshToken: true,
    },
  },
};

export default config;
