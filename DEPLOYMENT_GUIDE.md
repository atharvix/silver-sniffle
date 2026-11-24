# Deployment guide

How Kinjo runs in production, and how to ship each piece. Credentials, IPs and passwords are not
here: they are in `CREDENTIALS_AND_OPS.md` (confidential, not committed). Nothing deploys
automatically from this repo; changes are built locally (or by the workflows) and shipped by hand.

| Piece | Where it runs |
|---|---|
| Go API (`backend/`) | Utho VPS, systemd `kinjo.service`, `127.0.0.1:8080` |
| Website + admin panel (`kinjo-site-source/`) | static files from nginx at `https://kinjo.world` |
| App web UI (`frontend/web/`) | bundled into the Android APK and the iPhone app (not hosted) |
| Database | PostgreSQL 16 on the same VPS |
| Push | FCM (transport only) and APNs location pushes (iPhone) |

Server layout: `/var/www/kinjo-app/backend/` (`kinjo-api` binary + `.env`) and
`/var/www/kinjo-app/site/` (the website build). Production is a 2 vCPU / 4 GB box; it holds about
1,000 position reports per second (`docs/proximity.md`, "Load test on the production VPS"). More
than that needs a bigger machine, then sharding the hub by region.

## 1. Database

PostgreSQL 14+ (production runs 16). Create a database and a user, and put the URL in `.env`:

```sql
CREATE DATABASE kinjo;
CREATE USER kinjo_user WITH ENCRYPTED PASSWORD '…';
GRANT ALL PRIVILEGES ON DATABASE kinjo TO kinjo_user;
```

```
DATABASE_URL=postgres://kinjo_user:…@127.0.0.1:5432/kinjo?sslmode=disable
```

There are no migration files: the backend creates every table on boot (`CREATE TABLE IF NOT EXISTS`
and `ALTER … ADD COLUMN IF NOT EXISTS`), so deploying a new version is also the migration. Test
schema changes against a throwaway Postgres first. Add `pool_max_conns=…` to the URL to change the
default pool of 20.

## 2. Backend

1. Copy `backend/.env.example` to `/var/www/kinjo-app/backend/.env` and fill it in. The file lists
   every variable with comments. The ones that matter first: `DATABASE_URL`, `LINKEDIN_CLIENT_ID`,
   `LINKEDIN_CLIENT_SECRET`, `LINKEDIN_REDIRECT_URL`, `FRONTEND_URL=kinjo://auth`, `ALLOW_ORIGINS`
   (`https://localhost` for Android, `capacitor://localhost` for iPhone, plus the website origin),
   `ADMIN_EMAILS`, `DATA_KEY` (`openssl rand -base64 32`, losing it makes stored Apple refresh
   tokens unreadable), `SMTP_*`, `FCM_*`. Optional: `GOOGLE_*`, `APNS_*`, `APPLE_*`, `DEMO_*`.
2. Build on a machine with the Go version in `backend/go.mod` (currently 1.27) and copy the binary over:

   ```bash
   cd backend && GOOS=linux GOARCH=amd64 go build -o kinjo-api .
   scp kinjo-api <server>:/var/www/kinjo-app/backend/kinjo-api.new
   ```

3. Install `backend/deploy/kinjo.service` as `/etc/systemd/system/kinjo.service` (diff against the
   server's first), then `systemctl daemon-reload && systemctl enable --now kinjo`.
4. Replace the binary with `mv kinjo-api.new kinjo-api && systemctl restart kinjo`. On shutdown the
   server hands fresh positions to the next process through `/dev/shm`, so people stay visible
   across a restart.
5. Check `curl https://kinjo.world/healthz` (200 only if the database answers), then run the smoke
   test (below).

**Deploy the backend before an app build that depends on it.** Old app builds keep working: the
server still accepts the legacy `#token=` hand-off and the `?token=` WebSocket parameter, which
will go once those builds are gone.

## 3. nginx and TLS

`backend/deploy/nginx-kinjo.conf` is the reference config for `kinjo.world`. Diff it against
`/etc/nginx/sites-available/kinjo` before applying, then `nginx -t && systemctl reload nginx`.
What it provides, and why you shouldn't loosen it:

- **Rate limits keyed on the session** for `/api/`, not the IP: venue Wi-Fi and carrier CGNAT put
  many phones behind one IP.
- **A photo cache** for `/api/photo/` and `/api/event-cover/` (immutable per `?v=`).
- **`/ws`** with a 3600 s read timeout (reporter sockets may be silent while a phone sleeps) and
  `so_keepalive=240:30:4` on the listener, so a switched-off phone's socket closes in about 6 minutes.
- **Every `/admin/<name>` API path must be listed** in the `location ~ ^/admin/(…)` regex, or
  production serves the website for it. Add the name there whenever you add an admin route.
- A log format that records `$uri` without the query string, so tokens never reach access logs.

TLS comes from Let's Encrypt: `certbot --nginx -d kinjo.world` (add names with `--expand`).

## 4. Website and admin panel

```bash
cd kinjo-site-source
cp .env.example .env     # VITE_API_BASE=https://kinjo.world, VITE_ADMIN_PATH=/<private path>
npm install && npm run build
```

Upload `dist/` to `/var/www/kinjo-app/site/` (nginx serves it; the repo's `kinjo-site-source/nginx.conf`
and `vercel.json` are the website-only variants). The admin panel (Kinjo Control) lives at the private
path in `VITE_ADMIN_PATH`, never `/admin`; unset, the build has no panel. Sign-in is an emailed code
sent only to `ADMIN_EMAILS`.

## 5. Android app

```bash
cd frontend/native
API_BASE=https://kinjo.world ANDROID_KEYSTORE_PASSWORD=… ANDROID_KEY_PASSWORD=… \
  ANDROID_KEY_ALIAS=kinjo-release-key bash android-build.sh app
```

`android-build.sh sdk` installs the Android SDK once. The script builds the web UI, bundles it into
`www/`, generates `android/` (gitignored, never edit it; native code lives in
`plugins/kinjo-presence/android/`), strips the background-location permissions, injects the release
signing config and builds a signed APK. The workflow `build-apk.yml` does the same on GitHub (and an
`.aab` for Play) from the secrets `ANDROID_KEYSTORE_BASE64`, `ANDROID_KEYSTORE_PASSWORD`,
`ANDROID_KEY_PASSWORD`, `ANDROID_KEY_ALIAS` and `GOOGLE_SERVICES_JSON`.

Package `com.kinjo.app` and the key `kinjo-release-key.jks` (SHA-1
`9E:4C:52:A7:21:DE:2D:CA:53:F0:09:E3:A9:77:F8:DB:21:4D:34:A8`) must never change, or Play treats the
build as a new app. Bump `versionCode` / `versionName` for every release.

## 6. iPhone app

Built on macOS, by `frontend/native/ios-setup/ios-build.sh` or the manual **Build iOS (TestFlight)**
workflow. One-time setup, secrets and the review checklist are in
`frontend/native/ios-setup/README.md`. `capacitor://localhost` must be in `ALLOW_ORIGINS`.

## 7. Backups

`backend/deploy/backup.sh` runs daily from `kinjo-backup.timer` (03:30, 14 days kept). With
`BACKUP_AGE_RECIPIENT` set to an `age` public key it encrypts every dump before it touches the disk
and refuses to write a plaintext one; the private key stays offline. Restore a dump with
`age -d -i key.txt <file>.dump.age > f.dump` then `pg_restore --clean --if-exists --no-owner -d "$DATABASE_URL" f.dump`.
Restore one into a scratch database monthly. The off-box copy destination is still to be decided.

## 8. After every deploy: smoke test

Create two temporary accounts, put their session tokens in `SMOKE_A` and `SMOKE_B` (environment, never
the command line), and run `backend/cmd/smoke`: both cards must reach the other's open socket within
5 s, and disappear at once when one hides. Delete the accounts (`DELETE /api/account`) afterwards.

## 9. Logs and monitoring

Logs are JSON on stderr (journald): `journalctl -u kinjo -f`. One access line per request (id, path
without query, status, ms) except fast successful phone reports; errors and requests over 500 ms
always log. `GET /healthz` is for monitors. Kinjo Control shows the in-memory telemetry
(`/admin/metrics`: requests, errors, matches, push and mail results, latency percentiles, database
pool, host load and disk) on its overview and system pages.
