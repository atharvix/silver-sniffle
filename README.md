# 📍 Kinjo — Location-Based Social Discovery Platform

Kinjo is a real-time **location-based social discovery platform** designed to connect people who are physically nearby (within a **30-meter radius**). Users can discover local creators, professionals, and innovators around them through a high-performance, 60fps swipeable card deck interface.

---

## 🚀 Quick Reference & Production Infrastructure

| Component | Production Configuration |
| :--- | :--- |
| **Server Host** | Utho Cloud VPS — address and access in `CREDENTIALS_AND_OPS.md` (never in this repo) |
| **SSH Access** | `ssh -i <your-key> <user>@<server-host>` — see `CREDENTIALS_AND_OPS.md` |
| **Production Domain** | `https://kinjo.world` |
| **Production API** | `https://kinjo.world/api` (reverse-proxied via Nginx to `:8080`) |
| **Service Manager** | Systemd (`kinjo.service`) |
| **Database** | PostgreSQL 16 on `localhost:5432` (`kinjo`, user: `kinjo_user`) |
| **Backend Path** | `/var/www/silver-sniffle/backend/backend-go` |
| **Android Package** | `com.kinjo.app` (must match the Android OAuth client in Google Cloud Console) |

---

## 🗄️ Clean Database Schema & Architecture

The database uses a clean, consolidated 5-table schema with native spatial bounding box indexing on `(latitude, longitude)`. All user profile data is stored in human-readable plaintext columns for direct inspection, with hashing applied only where strictly necessary:
- **Passwords**: `bcrypt` (`password_hash`)
- **Session Tokens**: `SHA-256` (`token_hash`)
- **OTP Codes**: `SHA-256` (`otp_hash`)

### Tables Overview
1. **`profiles`**: `email` (PK), `name`, `bio`, `photo_url`, `latitude`, `longitude`, `last_seen_at`, `password_hash`, `email_verified`, `face_verified_at`, `face_scan_photo_url`, `created_at`, `updated_at`.
2. **`verification_tokens`**: `token_hash` (PK), `email`, `expires_at`, `created_at`.
3. **`otp_codes`**: `email` (PK), `otp_hash`, `expires_at`, `attempts`.
4. **`verified_emails`**: `email` (PK), `expires_at`.
5. **`device_tokens`**: `device_token` (PK), `email`, `platform`, `updated_at`.

### Direct PostgreSQL Queries
```bash
# Connect to PostgreSQL on server (password in CREDENTIALS_AND_OPS.md)
PGPASSWORD="<db_password>" psql -h localhost -U kinjo_user -d kinjo

# View all user profiles in plaintext
SELECT email, name, email_verified, (face_verified_at IS NOT NULL) AS face_verified, created_at FROM profiles;

# View active session tokens
SELECT token_hash, email, expires_at FROM verification_tokens;
```

---

## 🛠️ Admin CLI (`kinjo-admin`)

A dedicated CLI tool is available on the server and locally to inspect and query user records in plaintext:

```bash
cd /var/www/silver-sniffle/backend/backend-go

# List all registered users in a clean table format
./kinjo-admin list

# Find full profile details for a specific user
./kinjo-admin find user@example.com

# List recent active session tokens
./kinjo-admin tokens
```

---

## 📱 Android Keystore & Build Information

The production Android release keystore is stored at `kinjo-release-key.jks` (gitignored) and configured in `frontend/android/app/build.gradle`, which reads the passwords from the environment or `local.properties` — they are deliberately not in this file.

The fingerprints below are public information; register them against the Android OAuth client for `com.kinjo.app` in Google Cloud Console.

| Property | Value |
|---|---|
| **Keystore File** | `kinjo-release-key.jks` (also in `frontend/android/app/kinjo-release-key.jks`) |
| **Key Alias** | `kinjo-release-key` |
| **Keystore Password** | GitHub secret `ANDROID_KEYSTORE_PASSWORD`, or `KINJO_KEYSTORE_PASSWORD` in the gitignored `frontend/android/local.properties` |
| **Key Password** | GitHub secret `ANDROID_KEY_PASSWORD`, or `KINJO_KEY_PASSWORD` in the gitignored `frontend/android/local.properties` |
| **Package Name** | `com.kinjo.app` |
| **SHA-1 Fingerprint** | `9E:4C:52:A7:21:DE:2D:CA:53:F0:09:E3:A9:77:F8:DB:21:4D:34:A8` |
| **SHA-256 Fingerprint** | `00:2C:B3:88:4C:5F:4F:66:2B:94:D9:CD:AC:09:2D:60:67:FB:6F:59:11:3A:4C:D1:F9:8B:3F:E6:49:79:A3:83` |
| **Valid Until** | 2054-02-02 |

> [!IMPORTANT]
> Both fingerprints above were read directly from the keystore in this repository
> (`keytool -list -v -keystore kinjo-release-key.jks`).
>
> A previously documented pair did **not** match it:
> `8A:6B:A0:6B:...` / `37:DF:A6:49:...`. If the Google Cloud Console Android OAuth
> client is registered against that old SHA-1, Google Sign-In fails with
> `DEVELOPER_ERROR` (10) even when the package name is correct. Register the
> **current** SHA-1 for package `com.kinjo.app`, and add the debug keystore's
> SHA-1 as a second client for local testing.

---

## 🤖 Automated CI/CD (GitHub Actions)

The repository is equipped with an automated GitHub Actions workflow (`.github/workflows/build-apk.yml`) that builds both the **Signed Production Release APK** and **Android App Bundle (.aab)** on every push to `slave`/`main` or manual workflow dispatch.

### Required GitHub Repository Secrets
Under **Settings** ➔ **Secrets and variables** ➔ **Actions**:
- `ANDROID_KEYSTORE_BASE64`: Base64 string of `kinjo-release-key.jks` (see `keystore_base64.txt`, local only)
- `ANDROID_KEYSTORE_PASSWORD`: the release keystore password
- `ANDROID_KEY_ALIAS`: `kinjo-release-key`
- `ANDROID_KEY_PASSWORD`: the release key password

> Release builds now **fail loudly** if the password secrets are missing, instead of falling back to a value committed to this repository.

Generated artifacts (`app-release.apk` and `app-release.aab`) are automatically uploaded as downloadable workflow artifacts.

---

## 🌐 External Services & Integrations

> [!NOTE]
> All production secrets and credentials are stored securely in `CREDENTIALS_AND_OPS.md` (local only, git-ignored) and `/var/www/silver-sniffle/backend/backend-go/.env`.

### 1. Google OAuth 2.0
* **Google Web Client ID**: `Refer to CREDENTIALS_AND_OPS.md or frontend/.env.production`
* **Google Web Client Secret**: `Refer to CREDENTIALS_AND_OPS.md`
* **Google Android Client ID**: `Refer to CREDENTIALS_AND_OPS.md`

### 2. Transactional Email (GoDaddy SMTP)
Used for delivering 6-digit OTP codes and welcome emails:
* **SMTP Host**: `smtpout.secureserver.net`
* **SMTP Port**: `465` (SSL)
* **SMTP Username / Sender**: `hello@kinjo.world`
* **SMTP Password**: `Refer to CREDENTIALS_AND_OPS.md`
* **Encryption**: `ssl`

### 3. Push Notifications (Firebase Cloud Messaging)
* **FCM Project ID**: `kinjo-3fd56`
* **Service Account Key**: `/var/www/silver-sniffle/backend/backend-go/firebase-service-account.json`

---

## 💻 Local Development & Build Commands

### 1. Start Go Backend Server
```bash
cd backend/backend-go
go run ./cmd/api
```
- Listens on `http://localhost:8080`
- Health check: `curl http://localhost:8080/api/healthz` (`{"status":"ok"}`)

### 2. Start Frontend Web App
```bash
cd frontend
npm run dev
```
- Access the web interface at `http://localhost:5173`

### 3. Generate Android Release APK Locally
```bash
cd frontend
npm run build
npx cap sync android
cd android
./gradlew assembleRelease
```
- **Release APK Output**: `frontend/android/app/build/outputs/apk/release/app-release.apk`

### 4. Deploy Updates to Production Server
Access details live in `CREDENTIALS_AND_OPS.md`, not here.
```bash
ssh -i <your-key> <user>@<server-host>

cd /var/www/silver-sniffle
git pull origin slave

cd backend/backend-go
go build -o kinjo-api ./cmd/api
go build -o kinjo-admin ./cmd/admin
systemctl restart kinjo

# Check service status & logs
systemctl status kinjo
journalctl -u kinjo -f
```

---

## 🧪 Testing & Verification

```bash
# Run backend test suite (unit and integration)
cd backend/backend-go
go test ./...

# Run frontend build & TypeScript validation
cd frontend
npm run build
```
