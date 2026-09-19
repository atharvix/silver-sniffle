# 📍 Kinjo — Location-Based Social Discovery Platform

Kinjo is a real-time **location-based social discovery platform** designed to connect people who are physically nearby (within a **30-meter radius**). Users can discover local creators, professionals, and innovators around them through a high-performance, 60fps swipeable card deck interface.

---

## 🚀 Quick Reference & Production Infrastructure

| Component | Production Configuration |
| :--- | :--- |
| **Server Host** | Utho Cloud VPS (`103.127.28.201`) |
| **SSH Command** | `ssh -i ~/.ssh/myserver root@103.127.28.201` |
| **Production Domain** | `https://kinjo.world` |
| **Production API** | `https://kinjo.world/api` (reverse-proxied via Nginx to `:8080`) |
| **Service Manager** | Systemd (`kinjo.service`) |
| **Database** | PostgreSQL 16 on `localhost:5432` (`kinjo`, user: `kinjo_user`) |
| **Backend Path** | `/var/www/silver-sniffle/backend/backend-go` |
| **Android Package** | `world.kinjo.app` |

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

The production Android release keystore is stored at `kinjo-release-key.jks` and configured in `frontend/android/app/build.gradle`:

| Property | Value |
|---|---|
| **Keystore File** | `kinjo-release-key.jks` (also in `frontend/android/app/kinjo-release-key.jks`) |
| **Key Alias** | `kinjo-release-key` |
| **Keystore Password** | `0e84c9c9d585d7c4512983e4582cf6c3` |
| **Key Password** | `0e84c9c9d585d7c4512983e4582cf6c3` |
| **Package Name** | `world.kinjo.app` |
| **SHA-1 Fingerprint** | `8A:6B:A0:6B:17:8F:D6:75:5D:80:C1:F6:DE:F7:56:5C:F8:78:E1:48` |
| **SHA-256 Fingerprint** | `37:DF:A6:49:15:37:37:31:3D:02:D5:AC:05:48:ED:9A:86:E0:52:13:B9:4C:E2:20:96:AC:61:9D:C2:59:75:A5` |

---

## 🤖 Automated CI/CD (GitHub Actions)

The repository is equipped with an automated GitHub Actions workflow (`.github/workflows/build-apk.yml`) that builds both the **Signed Production Release APK** and **Android App Bundle (.aab)** on every push to `slave`/`main` or manual workflow dispatch.

### Required GitHub Repository Secrets
Under **Settings** ➔ **Secrets and variables** ➔ **Actions**:
- `ANDROID_KEYSTORE_BASE64`: Base64 string of `kinjo-release-key.jks` (saved in `keystore_base64.txt`)
- `ANDROID_KEYSTORE_PASSWORD`: `0e84c9c9d585d7c4512983e4582cf6c3`
- `ANDROID_KEY_ALIAS`: `kinjo-release-key`
- `ANDROID_KEY_PASSWORD`: `0e84c9c9d585d7c4512983e4582cf6c3`

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
```bash
ssh -i ~/.ssh/myserver root@103.127.28.201

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
