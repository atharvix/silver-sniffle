# Kinjo — Complete Cloud, Production & App Store Deployment Requirements

This document outlines everything required to deploy, host, secure, and publish **Kinjo** in production, from cloud infrastructure and database requirements to Google Play Store and Apple App Store submission guides.

---

## 1. Cloud Infrastructure & Hosting Needs

| Component | Minimum Specs | Recommended Providers | Purpose | Cost Estimate |
|---|---|---|---|---|
| **API Server (Compute)** | 1 vCPU, 1GB-2GB RAM, Linux OS | Railway, Render, DigitalOcean Droplet, AWS EC2, GCP Cloud Run | Hosts the Go REST API server (`/cmd/api`) | \$5 - \$12 / month |
| **PostgreSQL Database** | PostgreSQL 14+ (PostGIS extension enabled) | Supabase, Neon.tech, AWS RDS PostgreSQL, Aiven | Persists user profiles, auth hashes, location logs | Free tier - \$25 / month |
| **File Storage** | Local Disk or S3 Bucket | Local Server Disk (Small Dev) OR Supabase Storage / AWS S3 / Cloudflare R2 | Stores compressed profile avatars and uploaded images | Free tier - \$5 / month |
| **Domain & SSL/TLS** | Custom Domain + Wildcard SSL | Cloudflare, Let's Encrypt, Namecheap | Provides HTTPS endpoint (`https://api.yourdomain.com`) | \$10 / year |

> [!TIP]
> **For Small Developers / Bootstrap Budget:**
> You can host the Go API on **Railway.app** or **Render.com** (Free/Starter \$5 plan) connected to a **Free Supabase PostgreSQL** database and use the built-in **Local Storage driver** (`STORAGE_DRIVER=local`) on server disk. Total cost: **\$0 - \$5/month**.

---

## 2. API Keys & External Service Credentials

### 1. Google OAuth 2.0 Credentials (Google Sign-In)
- **Where to Get**: [Google Cloud Console](https://console.cloud.google.com/) -> APIs & Services -> Credentials.
- **Required Types**:
  - **Web Client ID**: Used for web browser authentication and backend ID token verification.
  - **Android Client ID**: Package name `com.kinjo.app`, SHA-1 fingerprint of your Android signing keystore.
- **Required Scopes**: `openid`, `email`, `profile`.
- **Environment Variables**: `GOOGLE_CLIENT_ID`, `GOOGLE_CLIENT_SECRET`, `VITE_GOOGLE_CLIENT_ID`.

### 2. Transactional Email Provider (OTP Email Verification)
- **Where to Get**: [Resend.com](https://resend.com) OR [Brevo.com](https://www.brevo.com/) (formerly Sendinblue).
- **Purpose**: Delivers 4-digit OTP login/signup verification codes to users' inbox.
- **Free Tier**: 300 emails/day (Brevo) or 3,000 emails/month (Resend).
- **Environment Variables**: `BREVO_API_KEY`, `BREVO_SENDER_EMAIL`.

---

## 3. Full Environment Variable Reference Guide

### Backend Environment Variables (`/backend/backend-go/.env`)

```env
# ─── Server Environment ────────────────────────────────────────────────────────
ENVIRONMENT=production                 # "development" or "production"
PORT=8080                              # HTTP Server Port

# ─── Database ──────────────────────────────────────────────────────────────────
# PostgreSQL connection string (Supabase / Neon / Local)
DATABASE_URL=postgresql://postgres:YOUR_PASSWORD@your-db-host.com:5432/postgres?sslmode=require

# ─── Security & CORS ──────────────────────────────────────────────────────────
# Comma-separated list of allowed frontend origins
ALLOWED_ORIGINS=capacitor://localhost,http://localhost:5173,https://yourdomain.com
BASE_URL=https://api.yourdomain.com

# ─── File Storage Driver ──────────────────────────────────────────────────────
STORAGE_DRIVER=local                   # "local" (server disk) OR "supabase" (cloud bucket)
MAX_PHOTO_BYTES=10485760               # Maximum uploaded image size (10MB default)

# ─── Supabase (Required if STORAGE_DRIVER=supabase) ───────────────────────────
SUPABASE_URL=https://your-project.supabase.co
SUPABASE_ANON_KEY=your_supabase_anon_key
SUPABASE_SERVICE_ROLE_KEY=your_supabase_service_role_key

# ─── Google Sign-In Verification ──────────────────────────────────────────────
GOOGLE_CLIENT_ID=your_google_client_id.apps.googleusercontent.com
GOOGLE_CLIENT_SECRET=your_google_client_secret
GOOGLE_REDIRECT_URI=https://api.yourdomain.com/api/v1/auth/google/callback

# ─── OTP Email Provider ───────────────────────────────────────────────────────
BREVO_API_KEY=your_brevo_api_key
BREVO_SENDER_EMAIL=noreply@yourdomain.com
```

### Frontend Environment Variables (`/frontend/.env.local`)

```env
# ─── Kinjo API Endpoint ────────────────────────────────────────────────────────
VITE_API_URL=https://api.yourdomain.com/api

# ─── Google OAuth Client ID ────────────────────────────────────────────────────
VITE_GOOGLE_CLIENT_ID=your_google_client_id.apps.googleusercontent.com

# ─── Supabase Public Credentials (Optional) ──────────────────────────────────
VITE_SUPABASE_URL=https://your-project.supabase.co
VITE_SUPABASE_ANON_KEY=your_supabase_anon_key
```

---

## 4. Google Play Store Submission Guide (Android)

### Prerequisites:
1. **Google Play Console Account**: \$25 one-time registration fee at [play.google.com/console](https://play.google.com/console).
2. **Release Keystore**: Created for digitally signing your APK/AAB release.

### Step-by-Step Submission Process:

#### Step 1: Generate Android Release Keystore
Run keytool in terminal to generate a secure release signing key:
```bash
keytool -genkey -v -keystore kinjo-release-key.jks -keyalg RSA -keysize 2048 -validity 10000 -alias kinjo-key
```

#### Step 2: Configure Release Signing in `android/app/build.gradle`
Add your release keystore configuration:
```groovy
android {
    signingConfigs {
        release {
            storeFile file("kinjo-release-key.jks")
            storePassword "YOUR_KEYSTORE_PASSWORD"
            keyAlias "kinjo-key"
            keyPassword "YOUR_KEY_PASSWORD"
        }
    }
    buildTypes {
        release {
            signingConfig signingConfigs.release
            minifyEnabled true
            proguardFiles getDefaultProguardFile('proguard-android-optimize.txt'), 'proguard-rules.pro'
        }
    }
}
```

#### Step 3: Build Production Android App Bundle (.aab)
```bash
cd frontend
npm run build
npx cap sync android
cd android
./gradlew bundleRelease
```
The signed AAB file will be generated at:
`frontend/android/app/build/outputs/bundle/release/app-release.aab`

#### Step 4: Google Play Console Release Setup
1. Log in to [Google Play Console](https://play.google.com/console) -> **Create App** (`Kinjo`).
2. **Data Safety Form**: Declare that the app accesses fine GPS location strictly to calculate nearby user proximity (30m discovery), and photo uploads for user avatars.
3. **App Access**: Provide demo credentials (email + password) for Google reviewers to log in.
4. **Upload AAB**: Drag and drop `app-release.aab` under **Production -> Create New Release**.
5. **Store Listing Assets**: Upload 512x512 App Icon, 1024x500 Feature Graphic, and 4+ phone screenshots.
6. Submit for review (Approval takes 24-48 hours).

---

## 5. Apple App Store Submission Guide (iOS)

### Prerequisites:
1. **Apple Developer Account**: \$99/year membership at [developer.apple.com](https://developer.apple.com/).
2. **macOS Machine + Xcode 15+**: Required for building and signing iOS apps.

### Step-by-Step Submission Process:

#### Step 1: Sync iOS Project with Capacitor
```bash
cd frontend
npm run build
npx cap add ios
npx cap sync ios
npx cap open ios
```

#### Step 2: Configure App Capabilities & Signing in Xcode
1. Open the project in Xcode (`npx cap open ios`).
2. Under **Signing & Capabilities**, select your **Apple Developer Team** and enable **Automatic Signing**.
3. Under `Info.plist`, ensure privacy location keys are declared:
   - `NSLocationWhenInUseUsageDescription`: "Kinjo uses your location to discover other profiles within 30 meters."
   - `NSCameraUsageDescription`: "Kinjo requires camera access for live face verification during setup."

#### Step 3: Archive & Submit via Xcode
1. Select target **Any iOS Device (arm64)**.
2. In the top menu, select **Product -> Archive**.
3. Once the archive completes, click **Distribute App** -> **App Store Connect** -> **Upload**.

#### Step 4: App Store Connect Listing Setup
1. Log in to [App Store Connect](https://appstoreconnect.apple.com).
2. Create **New App** -> Select Bundle ID (`com.kinjo.app`).
3. Upload 6.5" and 5.5" iPhone Screenshots.
4. Fill in Category (`Social Networking`), Age Rating, and Privacy Policy URL.
5. Select the uploaded Xcode build under **Build**, and click **Submit for Review**.

---

## 6. Production Launch & Security Checklist

- [x] **Enforce Email Verification**: All user accounts require verified OTP or Google OAuth parity (`email_verified = TRUE`).
- [x] **Account Lockout Enabled**: 15-minute lock triggered automatically after 5 consecutive failed password attempts.
- [x] **Input Sanitization**: HTML escaping applied on all user `name` and `bio` inputs to eliminate XSS risks.
- [x] **Spatial Bounding Box Optimization**: Bounding box pre-filters enabled on 30m distance queries.
- [x] **API Versioning**: Route aliases supported for `/api/v1/...`.
- [x] **Universal Back Button**: Full step history stack handling back buttons across mobile and web.
- [ ] **HTTPS Enforcement**: Ensure your production domain serves traffic over TLS/HTTPS with a valid SSL certificate.
- [ ] **CORS Configuration**: Update `ALLOWED_ORIGINS` to contain only your official app origins.
- [ ] **Google Play / App Store Key**: Build signed APK/AAB with your release keystore before Play Store upload.
- [ ] **Privacy Policy & Terms**: Publish privacy policy covering location data retention (required by Google Play Store & Apple App Store).
