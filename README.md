# 📍 Kinjo — Location-Based Social Discovery Platform

Kinjo is a real-time **location-based social discovery platform** designed to connect people who are physically nearby (within a **30-meter radius**). Users can discover local creators, professionals, and innovators around them through a high-performance, 60fps swipeable card deck interface.

---

## ⚠️ Master Pre-Production Checklist (Read Before Production Launch)

Before deploying to production or publishing to the Google Play Store / Apple App Store, every developer and ops engineer **must** complete these steps:

### 1. 🔑 Critical Key Permanence: `AES_ENCRYPTION_KEY`
> [!CAUTION]
> **PERMANENT SECRET — DO NOT ROTATE AFTER USERS ENCRYPT DATA:**
> All user Personal Identifiable Information (PII) — including email addresses, names, bios, and precise GPS coordinates — is encrypted at rest using **AES-256-GCM** with deterministic HMAC-SHA256 search indexes (`email_hash`).
>
> - **Generation Command**:
>   ```bash
>   openssl rand -base64 48
>   ```
> - **Permanence Rule**: Once users sign up or encrypt data with this key, **it can NEVER be changed**. Changing `AES_ENCRYPTION_KEY` will render all existing user profiles, identities, and coordinates permanently unreadable.
> - **Backup**: Store this key securely in your organization's secret vault (e.g. AWS Secrets Manager, Doppler, 1Password Secrets).

### 2. 📱 Android Release Signing Keystore & Certificates
The production Android release keystore is stored at `kinjo-release-key.jks` and configured in `frontend/android/app/build.gradle`:

| Property | Value |
|---|---|
| **Keystore File** | `kinjo-release-key.jks` (also in `frontend/android/app/kinjo-release-key.jks`) |
| **Key Alias** | `kinjo-release-key` |
| **Keystore Password** | `0e84c9c9d585d7c4512983e4582cf6c3` |
| **Key Password** | `0e84c9c9d585d7c4512983e4582cf6c3` |
| **Validity** | 10,000 days (Valid until Feb 2054) |
| **SHA-1 Fingerprint** | `9E:4C:52:A7:21:DE:2D:CA:53:F0:09:E3:A9:77:F8:DB:21:4D:34:A8` |
| **SHA-256 Fingerprint** | `00:2C:B3:88:4C:5F:4F:66:2B:94:D9:CD:AC:09:2D:60:67:FB:6F:59:11:3A:4C:D1:F9:8B:3F:E6:49:79:A3:83` |

> [!IMPORTANT]
> When configuring **Google Sign-In (OAuth 2.0)** in Google Cloud Console:
> 1. Create an **Android Client ID** with package name `com.kinjo.app`.
> 2. Paste the **SHA-1 Fingerprint** (`9E:4C:52:A7:21:DE:2D:CA:53:F0:09:E3:A9:77:F8:DB:21:4D:34:A8`) into the Android credentials configuration.

### 3. 🤳 One-Time Face Scan Re-Verification for Existing Users
- Under database migration `000008_encrypt_pii_and_face_verification`, the `face_verified_at` column tracks live facial motion verification.
- **Why Re-Verification Happens**: Existing users from legacy database states have `face_verified_at = NULL`. The discovery engine (`AND face_verified_at IS NOT NULL`) strictly enforces that unverified accounts cannot be discovered and cannot save profile changes.
- **Application Behavior**: When an existing user logs in or restores an existing session, `GET /api/profiles/me` returns `face_verified: false`. The frontend automatically launches `OnboardingModal` directly at the `face_verification` step. Once the user completes the quick live scan, their profile is marked `face_verified_at = NOW()`, unlocking discovery immediately.

### 4. 🗄️ PostgreSQL Database & Migrations
- **PostgreSQL Version**: 14+ with the `postgis` extension enabled (Supabase, Neon, AWS RDS).
- **Automated Migration**: The Go backend runs all SQL migrations (`000001` through `000008`) automatically upon startup via `db.Migrate(ctx)`.
- **Encryption Backfill**: Startup includes an idempotent backfill routine (`profileRepoForBackfill.BackfillEncryption`) that encrypts any remaining legacy plaintext rows.

### 5. 🌐 Domain, SSL/TLS, & CORS
- Web and mobile devices require HTTPS endpoints for camera media access, geolocation permissions, and Google Identity Services.
- Configure `ALLOWED_ORIGINS` in `.env` to include your production frontend domain (e.g. `https://kinjo.world`, `https://www.kinjo.world`, `capacitor://localhost`).

---

## ⚙️ Environment Variables Reference Guide

### Backend Configuration (`/backend/backend-go/.env`)

```env
# ─── Server Environment ────────────────────────────────────────────────────────
ENVIRONMENT=production                 # "development" or "production"
PORT=8080                              # Server listening port
LOG_LEVEL=info                         # "debug", "info", "warn", "error"

# ─── At-Rest Encryption (CRITICAL) ──────────────────────────────────────────
# 48-byte base64 string generated via openssl rand -base64 48
AES_ENCRYPTION_KEY=nJA4/NasAcoBMuaRqiRdMVKiY8SJsngWbD9ortYdK6B5Crio4XLuoHCXXOGjZQEE

# ─── PostgreSQL Database ──────────────────────────────────────────────────────
DATABASE_URL=postgresql://user:password@host:5432/postgres?sslmode=require
DB_MAX_CONNS=25
DB_MIN_CONNS=5

# ─── Security & CORS ──────────────────────────────────────────────────────────
ALLOWED_ORIGINS=capacitor://localhost,http://localhost:5173,https://yourdomain.com
BASE_URL=https://api.yourdomain.com

# ─── File Storage ─────────────────────────────────────────────────────────────
STORAGE_DRIVER=local                   # "local" (server disk) OR "supabase" (cloud bucket)
STORAGE_DIR=./uploads                  # Local uploads directory
PHOTO_STORAGE=db                       # "db" (inline base64), "local", or "supabase"

# ─── Supabase (Required if STORAGE_DRIVER=supabase) ───────────────────────────
SUPABASE_URL=https://your-project.supabase.co
SUPABASE_ANON_KEY=your_supabase_anon_key
SUPABASE_SERVICE_ROLE_KEY=your_supabase_service_role_key
SUPABASE_BUCKET=profiles

# ─── Google OAuth 2.0 ─────────────────────────────────────────────────────────
GOOGLE_CLIENT_ID=599627705479-os5q2be0jnrjcbftfkatv75nd5idmhsk.apps.googleusercontent.com
GOOGLE_CLIENT_SECRET=your_google_client_secret

# ─── Transactional Email (SMTP for OTP & Welcome Emails) ──────────────────────
SMTP_HOST=smtp.gmail.com
SMTP_PORT=587
SMTP_USERNAME=your_smtp_username
SMTP_PASSWORD=your_smtp_password
SMTP_SENDER_EMAIL=hello@kinjo.world
SMTP_SENDER_NAME=Kinjo
SMTP_ENCRYPTION=tls

# ─── Push Notifications (Firebase Cloud Messaging) ───────────────────────────
FCM_PROJECT_ID=your_firebase_project_id
FCM_SERVICE_ACCOUNT_KEY=./firebase-service-account.json

# ─── Android Release Signing (Optional override; defaults to kinjo-release-key.jks) ──
KINJO_KEYSTORE_FILE=kinjo-release-key.jks
KINJO_KEYSTORE_PASSWORD=0e84c9c9d585d7c4512983e4582cf6c3
KINJO_KEY_ALIAS=kinjo-release-key
KINJO_KEY_PASSWORD=0e84c9c9d585d7c4512983e4582cf6c3
```

### Frontend Configuration (`/frontend/.env.local`)

```env
# Kinjo Backend API Base URL
VITE_API_URL=https://api.yourdomain.com/api

# Google OAuth Web Client ID (Must match Google Cloud Console)
VITE_GOOGLE_CLIENT_ID=599627705479-os5q2be0jnrjcbftfkatv75nd5idmhsk.apps.googleusercontent.com
```

---

## 📱 Mobile Build & Android APK / AAB Generation

The Android mobile app is configured with the official Kinjo branding, splash screens, and launcher icons.

### 1. Build Web Assets & Sync with Capacitor
```bash
cd frontend
npm run build
npx cap sync android
```

### 2. Generate Debug APK (Testing on Device / Emulator)
```bash
cd frontend/android
./gradlew assembleDebug
```
- **Output Path**: `frontend/android/app/build/outputs/apk/debug/app-debug.apk`

### 3. Generate Signed Production Release APK (Direct Device Distribution)
```bash
cd frontend/android
./gradlew assembleRelease
```
- **Output Path**: `frontend/android/app/build/outputs/apk/release/app-release.apk`
- Automatically signed using `kinjo-release-key.jks`.

### 4. Generate Production Android App Bundle (.aab) (Google Play Store Submission)
```bash
cd frontend/android
./gradlew bundleRelease
```
- **Output Path**: `frontend/android/app/build/outputs/bundle/release/app-release.aab`
- Upload this `.aab` directly to Google Play Console under **Production / Internal Testing**.

---

## 🚀 Running the Project Locally

### 1. Start the Go Backend Server
```bash
cd backend/backend-go
go run ./cmd/api
```
- The backend listens on `http://localhost:8080`.
- Health check: `curl http://localhost:8080/api/healthz` (`{"status":"ok"}`)
- Readiness check: `curl http://localhost:8080/api/readyz` (`{"status":"ready"}`)

### 2. Start the Frontend Web App
```bash
cd frontend
npm run dev
```
- Access the web interface at `http://localhost:5173`.

### 3. Ngrok HTTPS Tunnel for Mobile Development
Because camera capture and Google OAuth require HTTPS, run an Ngrok tunnel when testing native devices:
```bash
ngrok http 8080
```
Update `VITE_API_URL` in `frontend/.env.local` with your HTTPS URL (e.g. `https://your-tunnel.ngrok-free.app/api`).

---

## 🏗️ Technology Stack Architecture

```mermaid
graph TD
  subgraph "Frontend — Capacitor / React 19 / Vite"
    A["App.tsx — State & Session Restore"]
    B["OnboardingModal — Auth & Face Scan"]
    C["CardDeck — 60fps Gesture Stack"]
    D["useGPSLocation — Proximity Engine"]
    E["api.ts — Capacitor HTTP Client"]
  end

  subgraph "Backend — Go 1.22+ / Chi Router"
    F["Auth Service — Google & Email OTP"]
    G["Profile Service — AES-256-GCM Encryption"]
    H["Discovery Service — 30m Spherical Query"]
    I["Security Layer — HMAC email_hash & PII Envelopes"]
  end

  subgraph "Data Layer — PostgreSQL (Supabase/Neon)"
    K[(PostgreSQL 14+ with PostGIS)]
    L["Migrations 000001 - 000008"]
  end

  A --> B & C
  B & C & D --> E
  E --> F & G & H
  G --> I
  F & G & H & I --> K
  K --> L
```

---

## 🧪 Testing & Verification

```bash
# Run backend test suite
cd backend/backend-go
go test ./...

# Run frontend build & TypeScript check
cd frontend
npm run build
```
