# 📍 Kinjo — Location-Based Social Discovery Platform

Kinjo is a real-time **location-based social discovery platform** designed to connect people who are physically nearby (within a **30-meter radius**). Users can discover local creators, professionals, and innovators around them through a high-performance, 60fps swipeable card deck interface.

---

## 🚀 Production Infrastructure on Utho Cloud

| Component | Production Configuration |
| :--- | :--- |
| **Server Host** | Utho Cloud VPS (`103.127.28.201`) — credentials in `CREDENTIALS_AND_OPS.md` |
| **SSH Command** | `ssh -i ~/.ssh/myserver root@103.127.28.201` |
| **Production Domain** | `https://kinjo.world` |
| **Server Root Directory** | `/var/www/kinjo-app` (clean production tree) |
| **Backend Path** | `/var/www/kinjo-app/backend/kinjo-api` (Go binary) |
| **Website & Admin Path** | `/var/www/kinjo-app/site` (React production build) |
| **Admin Panel URL** | `https://kinjo.world/admin` |
| **Service Manager** | Systemd (`kinjo.service`) on internal port `8080` |
| **Database** | PostgreSQL 16 on `localhost:5432` (`kinjo`, user: `kinjo_user`) |
| **Reverse Proxy** | Nginx (`/etc/nginx/sites-available/kinjo`) routing `/`, `/api/`, `/auth/`, `/ws`, `/admin/` |
| **Android Package** | `com.kinjo.app` |
| **Release Version** | `versionCode 21`, `versionName "2.2.5"` |
| **Release Keystore** | `kinjo-release-key.jks` (`SHA-1: 9E:4C:52:A7:21:DE:2D:CA:53:F0:09:E3:A9:77:F8:DB:21:4D:34:A8`) |

---

## 🗄️ Database Schema & Architecture

The database runs on PostgreSQL 16 with a clean, consolidated 7-table schema:

### Tables Overview
1. **`users`**: `id` (PK, e.g. `li_...`), `linkedin_sub` (unique), `email`, `created_at`, `email_verified`, `email_verified_at`.
2. **`profiles`**: `uid` (PK, FK `users.id`), `name`, `role`, `look`, `photo`, `updated_at`.
3. **`presence`**: `uid` (PK, FK `users.id`), `cell`, `lat`, `lng`, `acc`, `t` (spatial grid for 30m radius).
4. **`sessions`**: `token` (PK), `uid` (FK `users.id`), `created_at`.
5. **`devices`**: `push_token` (PK), `uid` (FK `users.id`), `platform`, `updated_at`.
6. **`notifications`**: `id` (PK), `uid` (FK `users.id`), `title`, `body`, `data`, `created_at`, `sent_at`, `read_at`.
7. **`email_verifications`**: `token` (PK), `uid` (FK `users.id`), `email`, `created_at`, `expires_at`, `consumed_at`.

### Direct PostgreSQL Queries
```bash
# Connect to PostgreSQL on server (password in CREDENTIALS_AND_OPS.md)
psql -h localhost -U kinjo_user -d kinjo

# View registered users
SELECT id, email, created_at FROM users;

# View user profiles
SELECT uid, name, role FROM profiles;

# View push notification device tokens
SELECT uid, platform, updated_at FROM devices;
```

---

## 🔐 Authentication & Real-Time Proximity

1. **LinkedIn OpenID Connect:** Replaces Firebase completely.
   - The native app opens LinkedIn in the system browser (`Browser.open`).
   - Upon successful sign-in, the backend redirects to `kinjo://auth#token=<session_token>`.
   - The Capacitor app intercepts the URL via `appUrlOpen` and loads the profile.
2. **Proximity Engine:** Users within a **30-meter radius** discover each other in real-time.
   - GPS coordinates are reported via native background geolocation.
   - A persistent WebSocket connection (`wss://kinjo.world/ws?token=...`) handles real-time presence exchange and notifications.

---

## 🔔 Push Notifications & Email

- **Firebase Cloud Messaging (FCM):**
  - High-priority system channel (`fcm_default_channel`) with sound and vibration.
  - Capacitor `"presentationOptions": ["badge", "sound", "alert"]` ensures heads-up notifications appear both in foreground and background.
  - Android 13+ runtime permissions explicitly handled (`PushNotifications.requestPermissions()`).
  - Pending notifications (e.g. welcome message) are delivered immediately upon device registration.
  - Alternative push architectures documented in [`PUSH_NOTIFICATION_ALTERNATIVES.md`](file:///home/yaxh/Desktop/silver-sniffle/PUSH_NOTIFICATION_ALTERNATIVES.md).
- **GoDaddy SMTP Email:**
  - Configured with `smtpout.secureserver.net` on **port 465 (Direct SSL/TLS)**.
  - Port 587/25 are blocked by cloud firewalls, so direct TLS dial via port 465 is utilized.

---

## 📊 Web Admin Panel

- **URL:** [https://kinjo.world/admin](https://kinjo.world/admin)
- **Authentication:** Protected by `X-Admin-Token` header.
- **Features:** Live server telemetry (memory, goroutines, active WebSockets), database metrics, user inspection, and notification/email broadcasting.

---

## 📱 Google Play Console & Builds

- **Closed Testing:** Currently enrolled in the 14-day closed testing period with 12+ opted-in testers.
- **Lineage:** Bumped from `versionCode 15` (`v2.1.0`) to `versionCode 17` (`v2.2.0`).
- **Play Store Bundle (`.aab`):** Signed with production keystore `kinjo-release-key.jks`. Ready to upload to Google Play Console.
