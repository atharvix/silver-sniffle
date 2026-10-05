# Push Notification Alternatives to FCM for Kinjo

This document outlines viable alternatives to **Firebase Cloud Messaging (FCM)** for Kinjo's mobile and web architecture (Capacitor Android/iOS + Go backend).

---

## 1. Executive Summary & Comparison Matrix

| Provider / Strategy | Self-Hosted? | Free Tier | Google Services Required on Android? | Capacitor Support | Best Fit For |
|---|---|---|---|---|---|
| **OneSignal** | No (SaaS) | Up to 10,000 subscribers | Uses FCM gateway on Play Android, or HMS/direct | Official Capacitor Plugin (`@onesignal/onesignal-capacitor`) | Full marketing suite, analytics, segmentation, effortless setup |
| **Pusher Beams** | No (SaaS) | 1,000 monthly active devices | Uses FCM gateway for Android | Capacitor Community / Cordova plugin | Clean developer REST API, simple pub-sub |
| **UnifiedPush / ntfy** | Yes (Self-hostable) | 100% Free / Open-source | **No** (Zero Google dependency) | Available via Cordova/Capacitor plugins & Web Push | De-Googled Android, privacy-centric apps, F-Droid distribution |
| **WebSocket + Local Notifications** | Yes (Built-in) | Free (Runs on Go backend) | **No** | `@capacitor/local-notifications` | Instant delivery while app is active or in background; no third party |
| **Amazon SNS** | No (AWS Cloud) | 1,000,000 publishes/mo free | Uses FCM gateway for Android | Native bridge / REST | High-volume AWS deployments |

---

## 2. In-Depth Breakdown

### Option A: OneSignal (Most Popular Commercial Alternative)
- **How it works:** OneSignal manages device tokens, user aliases, retries, and cross-platform formatting. On standard Android devices it talks to Google's gateway, but handles token expiration, delivery tracking, and targeting automatically.
- **Pros:**
  - Official Capacitor plugin: `@onesignal/onesignal-capacitor`.
  - Rich web dashboard for viewing delivered notifications, click rates, and failed deliveries.
  - Multi-channel support (can trigger Push, In-App modal banners, and Emails in one flow).
  - Handles Android notification channels, badges, and priority out-of-the-box.
- **Cons:**
  - Proprietary third-party SaaS.
  - Requires account creation and integration with OneSignal API.

### Option B: UnifiedPush / ntfy.sh (100% Google-Free & Open Source)
- **How it works:** An open standard where push notifications do not rely on Google Play Services or Firebase. A self-hosted `ntfy` or `Gotify` server communicates directly with the phone over a long-lived HTTP/2 or WebSocket connection.
- **Pros:**
  - Complete data privacy: no payload metadata touches Google or third-party ad networks.
  - Works on de-Googled Android ROMs (GrapheneOS, CalyxOS, LineageOS) and Chinese devices without Google Play Services.
  - Entirely open source and self-hostable on your own VPS.
- **Cons:**
  - Android battery optimizations may sleep the persistent connection unless the user disables battery optimizations or runs a distributor app (like ntfy app or Nextcloud Talk).

### Option C: Kinjo Native WebSocket + `@capacitor/local-notifications`
- **How it works:** Kinjo already has an active WebSocket server (`hub.go`) that broadcasts presence updates. When a notification event occurs (e.g. welcome message, someone nearby), the server sends a JSON message over the WebSocket. The app intercepts it and calls `LocalNotifications.schedule(...)` to fire a native Android/iOS system banner.
- **Pros:**
  - Zero external dependencies or accounts.
  - 100% free, runs completely on the Kinjo Go backend.
  - Immediate real-time delivery with zero latency when the app is foregrounded or active in the background.
- **Cons:**
  - When the app is force-killed or deeply asleep, the OS terminates the WebSocket connection. System-level wakes require a platform push service.

### Option D: Pusher Beams
- **How it works:** Developer-first notification service by MessageBird/Pusher with clean REST APIs in Go (`github.com/pusher/push-notifications-go`).
- **Pros:**
  - Clean server SDK for Go.
  - Reliable token management.
- **Cons:**
  - Free tier is limited to 1,000 monthly active devices.

---

## 3. Recommendation for Kinjo
1. **For Production on Google Play / App Store:** Keep the native push plugin integrated. If you wish to replace direct FCM API maintenance in Go, **OneSignal** is the smoothest drop-in replacement with `@onesignal/onesignal-capacitor`.
2. **For Zero-Vendor Privacy:** Pair **WebSocket + `@capacitor/local-notifications`** for live in-app notifications, and use **UnifiedPush/ntfy** for background delivery without Google.
