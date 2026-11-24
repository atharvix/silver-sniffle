# Kinjo

Kinjo shows you a swipeable deck of the people within about **30 m** of you right now: a photo, a
name, what they do and what they're looking for. It runs on Android, iPhone and the web, against one
Go backend. Nothing about where anyone was, or who met whom, is stored.

Live at **https://kinjo.world**. Android and iPhone package / bundle id: `com.kinjo.app`.

> `CLAUDE.md` is the working guide for this repo (invariants, backend map, the reasons behind
> each rule). `docs/proximity.md` is the contract between the phones and the server. Read both
> before changing proximity, sign-in or notifications.

## What is in the repo

| Path | What it is |
|---|---|
| `backend/` | The whole server, in Go: sign-in, Postgres, WebSocket matching, push, email, admin API. |
| `frontend/web/` | The app UI, a single-file SPA (`src/app.fb.html` + `src/app.fb.js`). `deploy/build.sh` builds it. |
| `frontend/native/` | Capacitor 7 wrapper that **bundles** the web UI into the Android APK and the iPhone app. |
| `frontend/native/plugins/kinjo-presence/` | Our own native plugin: Android (Java) and iOS (Swift), same JS API. |
| `frontend/native/ios-setup/` | iPhone build script, AppDelegate, entitlements, the location-push extension. |
| `docs/proximity.md` | How people are found: the phone-to-server contract, the rules, measurements. |
| `loadtest/`, `backend/cmd/` | Load tests, a 650-phone simulation, and the post-deploy smoke test. |
| `kinjo-site-source/` | The marketing website and the admin panel (**Kinjo Control**), Vite + React. Its own git repository (`yashmish18/kinjo-website`, branch `main`), git-ignored by this one: commit and push it separately. |
| `.github/workflows/` | `build-apk.yml` (signed APK + AAB) and `build-ios.yml` (TestFlight, manual). |

## How it works, in short

- **Sign-in** is through our backend only (no Firebase for auth or data): LinkedIn, Google,
  Sign in with Apple (iPhone) or an emailed 6-digit code. The app returns through the
  `kinjo://auth` deep link with a one-time PKCE code, never the session token.
- **Finding people** is location first. A phone sends its position over a WebSocket; the server keeps
  all positions in memory and decides who is within range (35 m plus the two fixes' error). Wi-Fi
  overlap helps indoors. Bluetooth is kept only for older app builds and runs only while the app is
  open. See `docs/proximity.md`.
- **Cards** carry a photo URL, never the photo itself. Distance never leaves the server.
- **Push** uses FCM purely as a transport. A swiped-away iPhone is reached with an APNs location push.
- **Email** (welcome, verification, sign-in codes) goes out over SMTP.
- **Admin** is Kinjo Control, at a private path set at build time (never `/admin`).

## Production

| Item | Value |
|---|---|
| Host | Utho VPS (2 vCPU / 4 GB), nginx in front, TLS by Let's Encrypt |
| App directory | `/var/www/kinjo-app` (`backend/kinjo-api` binary, `site/` for the website) |
| Service | systemd `kinjo.service`, listening on `127.0.0.1:8080` |
| Database | PostgreSQL 16, tables are created on boot |
| Reverse proxy | `backend/deploy/nginx-kinjo.conf` (rate limits per session, photo cache, `/ws`) |
| Backups | `backend/deploy/backup.sh` + `kinjo-backup.timer`, encrypted with `age` |
| Android | `com.kinjo.app`, release-signed with `kinjo-release-key.jks`, **v2.4.0 (versionCode 49)** |

Credentials and server access are in `CREDENTIALS_AND_OPS.md` (confidential, never committed).
Nothing deploys automatically from this repo: changes are made locally and shipped by hand.

## Build and run

```bash
# Backend (needs a reachable Postgres; tables auto-create on boot)
cd backend && cp .env.example .env     # fill it in
go build ./... && go test ./... && go run .

# App web build (bundled into the APK / iPhone app)
API_BASE=https://<backend> bash frontend/web/deploy/build.sh

# Signed Android release APK (credentials via env, see CREDENTIALS_AND_OPS.md)
cd frontend/native
API_BASE=https://<backend> ANDROID_KEYSTORE_PASSWORD=… ANDROID_KEY_PASSWORD=… \
  ANDROID_KEY_ALIAS=kinjo-release-key bash android-build.sh app

# iPhone: see frontend/native/ios-setup/README.md

# Website + admin panel
cd kinjo-site-source && cp .env.example .env   # set VITE_API_BASE (and VITE_ADMIN_PATH)
npm run build                                  # or npm run dev; typecheck: npx tsc --noEmit
```

More detail: `DEPLOYMENT_GUIDE.md` (server, nginx, systemd), `DEPLOYMENT_REQUIREMENTS.md`
(accounts and credentials you need), `LINKEDIN_SETUP.md`, `backend/README.md`.

## After every backend deploy

Run `backend/cmd/smoke` with two temporary accounts (session tokens in `SMOKE_A` / `SMOKE_B`, never
on the command line): both cards must reach the other's open socket within 5 s and disappear at
once on hide. Delete the accounts afterwards. See `docs/proximity.md`.

## Play Console

Releases are built to `release-builds/app-release-v<versionName>-b<versionCode>.{apk,aab}` (local,
git-ignored). Package name and signing key must never change: Play would treat it as a new app.
