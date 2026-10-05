# Kinjo — LinkedIn + Go Rebuild & Production Architecture

A high-performance, Firebase-free rewrite of Kinjo. It connects professionals and innovators within a **30-meter radius** using real-time WebSockets, LinkedIn OpenID Connect authentication, and high-priority push notifications.

---

## 1. Production Infrastructure on Utho Cloud

The server is hosted on a **Utho Cloud VPS** (`103.127.28.201`). All legacy directories (such as `silver-sniffle` and old test backends) have been removed, leaving a clean production tree directly under `/var/www/kinjo-app`.

### Server Filesystem Layout

```text
/var/www/
├── html/                           # Default Nginx fallback
└── kinjo-app/
    ├── backend/
    │   ├── kinjo-api               # Standalone, statically compiled Linux x86_64 Go binary
    │   ├── .env                    # Production environment variables (chmod 600)
    │   └── firebase-service-account.json  # FCM service account key for push notifications
    └── site/                       # Built React marketing website + Admin Panel
        ├── index.html
        ├── assets/                 # Vite-compiled JS, CSS, media assets
        ├── privacy-policy.html     # Working legal redirects
        └── terms-conditions.html   # Working legal redirects
```

### Systemd Service (`kinjo.service`)
- **Service file:** `/etc/systemd/system/kinjo.service`
- **Working Directory:** `/var/www/kinjo-app/backend`
- **Executable:** `/var/www/kinjo-app/backend/kinjo-api`
- **Port:** Listens internally on `127.0.0.1:8080`
- **Commands:**
  ```bash
  systemctl restart kinjo     # Restart service (~25ms restart time)
  systemctl status kinjo      # Check live status
  journalctl -u kinjo -f      # Live server logs
  ```

### Nginx Reverse Proxy (`/etc/nginx/sites-available/kinjo`)
Nginx routes all HTTPS traffic on `https://kinjo.world`:
- `location /` → Serves the static site and SPA from `/var/www/kinjo-app/site/`
- `location /api/` → Proxies REST API calls to `http://127.0.0.1:8080/api/`
- `location /auth/` → Proxies LinkedIn OAuth endpoints to `http://127.0.0.1:8080/auth/`
- `location /ws` → Upgrades and proxies WebSocket connections to `http://127.0.0.1:8080/ws`
- `location ~ ^/admin/(metrics|users|email|notify)` → Proxies Admin API requests to `http://127.0.0.1:8080`
- `location /admin` and `/admin/` → Load the web Admin UI via `index.html`

---

## 2. PostgreSQL Database Architecture

- **Host & Port:** `localhost:5432`
- **Database:** `kinjo`
- **User:** `kinjo_user`
- **Credentials:** Documented in `CREDENTIALS_AND_OPS.md`

### Consolidated 7-Table Schema
1. **`users`**: `id` (PK, e.g. `li_...`), `linkedin_sub` (unique), `email`, `created_at`, `email_verified`, `email_verified_at`.
2. **`profiles`**: `uid` (PK, FK `users.id`), `name`, `role`, `look`, `photo`, `updated_at`.
3. **`presence`**: `uid` (PK, FK `users.id`), `cell`, `lat`, `lng`, `acc`, `t` (spatial grid for 30m radius).
4. **`sessions`**: `token` (PK), `uid` (FK `users.id`), `created_at`.
5. **`devices`**: `push_token` (PK), `uid` (FK `users.id`), `platform`, `updated_at`.
6. **`notifications`**: `id` (PK), `uid` (FK `users.id`), `title`, `body`, `data`, `created_at`, `sent_at`, `read_at`.
7. **`email_verifications`**: `token` (PK), `uid` (FK `users.id`), `email`, `created_at`, `expires_at`, `consumed_at`.

---

## 3. Authentication & Proximity Engine

- **Authentication:** Pure LinkedIn OpenID Connect (`openid`, `profile`, `email`). Completely replaces Firebase Auth.
  - The native app opens LinkedIn in the system browser (`Browser.open`).
  - Upon callback, the backend signs the user in and redirects to the deep link: `kinjo://auth#token=<session_token>`.
  - The native Capacitor app intercepts `kinjo://auth` via `appUrlOpen` and saves the token to local storage.
- **Proximity:** Proximity calculations use Haversine distance within a **30-meter radius**. The client connects to `wss://kinjo.world/ws?token=<session_token>` for real-time presence exchange and instant in-app notification toasts.

---

## 4. Push Notifications & Permissions

- **FCM v1 HTTP API:** Configured via service account `kinjo-3fd56`.
- **System Notification Channel:** High-priority channel `fcm_default_channel` with sound and vibration enabled.
- **In-App & Background Display:** Configured in `capacitor.config.json` with `"presentationOptions": ["badge", "sound", "alert"]` so notifications appear as typical Android system alerts in both foreground and background.
- **Android 13+ Runtime Permissions:** Prompts `PushNotifications.requestPermissions()` upon sign-in. Listeners (`registration`, `pushNotificationReceived`) are attached before calling `register()` to eliminate race conditions.
- **Immediate Unread Delivery:** When `POST /api/device` registers a push token, any pending unread notifications (e.g. welcome notification) are immediately pushed to the device via FCM.
- **Alternatives Guide:** Documented in [`PUSH_NOTIFICATION_ALTERNATIVES.md`](./PUSH_NOTIFICATION_ALTERNATIVES.md) (OneSignal, WebSocket + Local Notifications, UnifiedPush/ntfy, Pusher Beams).

---

## 5. Transactional Email Service

- **Provider:** GoDaddy SMTP (`smtpout.secureserver.net`)
- **Port:** **465 (Direct SSL/TLS)**.
  > [!IMPORTANT]
  > Cloud VPS networks (including Utho) block outbound ports `587` and `25` by default. The Go backend uses `crypto/tls.Dial` directly on port `465` to ensure reliable email delivery.
- **Sender:** `hello@kinjo.world`
- **Notice on Gmail:** Emails sent from newly registered domains may initially land in the **Spam** or **Promotions** folder until SPF/DKIM DNS records are established.

---

## 6. Web Admin Panel

- **URL:** [https://kinjo.world/admin](https://kinjo.world/admin)
- **Authentication:** Protected by `X-Admin-Token` header. Token password is configured in `/var/www/kinjo-app/backend/.env` (and documented in `CREDENTIALS_AND_OPS.md`).
- **Capabilities:**
  - Real-time telemetry (active WebSockets, memory allocation, goroutines, FCM & SMTP status).
  - Database metric rollups (registered users, complete cards, presence count, devices).
  - Full user list inspection.
  - Broadcast and targeted notification & email dispatch.

---

## 7. Android Builds & Google Play Console

### Release Version Lineage
- Previous Release: `versionCode 15`, `versionName "2.1.0"`
- Current Production Release: **`versionCode 17`**, **`versionName "2.2.0"`**
- Package: `com.kinjo.app`
- Keystore: `kinjo-release-key.jks` (`SHA-1: 9E:4C:52:A7:21:DE:2D:CA:53:F0:09:E3:A9:77:F8:DB:21:4D:34:A8`)

### Play Console Closed Testing
- The app is currently completing Google's 14-day closed testing requirement (12+ testers opted in).
- **Backend updates do not affect or reset the 14-day streak:** Google tracks tester account opt-ins on its own servers, independent of your backend uptime.
- Pushing updates to the Closed Testing track is encouraged by Google and does not pause the 14-day clock.

### Build Commands

```bash
# 1. Build the production Android App Bundle (.aab) for Google Play:
cd frontend/native/android
ANDROID_HOME=$HOME/android-sdk ./gradlew bundleRelease

# 2. Build the standalone release APK (.apk) for manual device installation:
ANDROID_HOME=$HOME/android-sdk ./gradlew assembleRelease
```

Generated release artifacts:
- **Play Store Bundle (`.aab`):** `frontend/native/android/app/build/outputs/bundle/release/app-release.aab`
- **Standalone APK (`.apk`):** `frontend/native/android/app/build/outputs/apk/release/app-release.apk`
