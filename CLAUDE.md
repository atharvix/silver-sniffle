# CLAUDE.md — Kinjo workspace

Guidance for working in this repo. Keep it accurate; update it when the shape changes.

## What this is

Kinjo is a "find your people within 30 m" app. The **current, active** build is a
Firebase-free rewrite:

- The product lives at the repo root:
  - `backend/` — **Go** API (LinkedIn OIDC auth, Postgres, WebSocket presence/matching,
    FCM push, SMTP email, admin API). This is the whole server.
  - `frontend/web/` — the app UI (`src/app.fb.html` + `src/app.fb.js`, a single-file SPA).
  - `frontend/native/` — Capacitor wrapper; **bundles** `frontend/web` into the APK.
    Native presence service lives in `frontend/native/plugins/kinjo-presence`.
- `kinjo-site-source/` — the marketing website **and the admin panel** (Vite + React +
  wouter + shadcn). Admin lives at route `/admin` (`src/pages/Admin.tsx`).

The old Firebase build and the legacy React app were removed in the repo restructure.

## Hard invariants (don't break these)

- **No Firebase** for auth or data. Sign-in, all via our backend: LinkedIn OIDC, Google
  OIDC when `GOOGLE_CLIENT_ID/SECRET` are set (browser flow, not the native Google SDK, so
  no SHA-1 setup), and an emailed 6-digit code (`email_login.go`, no passwords). Data is
  Postgres. FCM is used *only* as a push transport.
- **Account linking:** a provider login joins an existing account only if the provider says
  the email is verified *and* that account's email is verified (`upsertUser`). Never link on
  an unverified address — that's an account takeover.
- **Android package = `com.kinjo.app`**, release-signed with `kinjo-release-key.jks`
  (alias `kinjo-release-key`, SHA-1 `9E:4C:52:A7:21:DE:2D:CA:53:F0:09:E3:A9:77:F8:DB:21:4D:34:A8`).
  Changing either makes Play Console treat it as a new app — never change them.
- **Secrets live in `.env` / env vars only**, never committed. `backend/.env`,
  `kinjo-site-source/.env`, keystores and `*service-account*.json` are gitignored.
  Never put a credential on a shell command line (it's blocked) — write it to `.env`.
- Full credentials/ops are in `CREDENTIALS_AND_OPS.md` (confidential, not committed).

## Build & run

```bash
# Backend (needs a reachable Postgres; tables auto-create on boot)
cd backend
cp .env.example .env    # fill it in; go build ./... ; go test ./... ; go run .

# App web build (bundled into the APK)
API_BASE=https://<backend> bash frontend/web/deploy/build.sh

# Signed release APK (creds via env; see CREDENTIALS_AND_OPS.md)
cd frontend/native
API_BASE=https://<backend> ANDROID_KEYSTORE_PASSWORD=… ANDROID_KEY_PASSWORD=… \
  ANDROID_KEY_ALIAS=kinjo-release-key bash android-build.sh app

# Site + admin panel
cd kinjo-site-source
cp .env.example .env    # set VITE_API_BASE to the backend URL
npm run build           # or: npm run dev   (typecheck: npx tsc --noEmit)
```

Production runs on a Utho VPS behind nginx at `https://kinjo.world` (systemd `kinjo.service`).
During dev the backend is commonly exposed via ngrok. See `README.md` in
the repo root for the full server/nginx/systemd layout.

## Backend map (`backend/`)

`main.go` routes · `config.go` env · `store.go` Postgres (users has `linkedin_sub` and/or `google_sub`; tables: users, profiles,
presence, sessions, devices, notifications, email_verifications) · `auth.go` LinkedIn OAuth
+ sessions + CORS · `hub.go`/`client.go` WebSocket presence & matching · `geo.go` 30 m math ·
`notify.go` + `fcm.go` + `mailer.go` notifications/push/email · `admin.go` admin API.

- The app returns from LinkedIn via the **`kinjo://auth`** deep link (native) or the URL
  hash (web). Backend `FRONTEND_URL` must be `kinjo://auth` for the app.
- **Sign-in is PKCE-bound (RFC 7636/8252):** the app sends a SHA-256 `challenge` to
  `/auth/linkedin`; the callback returns a one-time `#code=` (never the session token),
  redeemed at `POST /auth/exchange` with the verifier. Requests without a challenge (old
  app builds) still get the legacy `#token=` — remove that path once they're gone.
- **Providers give the name only — no sign-in method ever fetches a photo.** Everyone
  uploads their own (a card can't be saved without one). A provider seeds a *new* profile
  only; an existing profile is never touched on login.
- **Hide guard:** after hide/logout/delete, fixes taken before that moment are dropped
  (the phone service's HTTP post can still be in flight). A socket `pos` lifts the guard,
  since a socket is ordered.
- **Schema comments must not contain `;` mid-line** — the schema is split on `;`.
- **Notification delivery is deduped by `notifications.sent_at`** — each notification is
  pushed to a device at most once. Don't reintroduce "re-push all unread on every device
  register"; that spams users on every app open.
- **Positions go through one path: `applyPos` (`client.go`)**, shared by the WebSocket
  `pos` message (browser) and `POST /api/presence` (phone). The latest fix always wins;
  its time is the phone's fix time (`age` field), never arrival time, and an older fix
  can't overwrite a newer one (memory and SQL).
- **Phone location = `frontend/native/plugins/kinjo-presence`** (our own Capacitor plugin,
  tracked in git — `frontend/native/android/` is generated and gitignored, never put native
  code there). `PresenceService` (foreground, type location, `stopWithTask=false`) runs
  `Gps` (last fix if <60 s old, then `getCurrentLocation`, then 10 s on screen / 30 s off /
  5 s moving; a still phone gets no new fixes, so a 20 s Handler plus a 60 s
  `setAndAllowWhileIdle` alarm (`PresenceService.Beat`) ask for one when none went out for
  40 s) and `Ble` (advertises a server token at low latency, filtered scan: full speed while
  the app is open, low power otherwise; reports every 5 s open / 10 s closed),
  independent of the WebView. Advertising keeps going while a phone sleeps and one phone
  hearing the other counts for both, so the open phone does the listening. Restarts: `START_STICKY`, `BootReceiver`, `Revive` (150 m
  geofence exit + 15-min WorkManager check) and `WakeService` (server's silent `wake` push;
  it extends Capacitor's push service and replaces it in the manifest). All background
  restarts need "Allow all the time". Stops on `KinjoPresence.stop()` or a 401. Force-stop
  ends everything until the app is opened, by Android design.
- **Scan screen = loading step:** presence starts during the splash; the scan ends as soon
  as a non-empty list is in and the first 3 photos loaded (deck already built), or after
  8 s with nobody (later arrivals slide in). App open goes straight to the deck if the
  list is already there. **The deck is never blanked on resume or socket loss** (that made
  cards blink and reset to the first card): it stays until the fresh list arrives and
  `setPeople` diffs it. It's cleared only after `STALE_MS` (2 min) with no word from the
  server, or 15 s after a resume that brings no list.
- **Socket liveness:** the app sends `ping` every 25 s and reconnects after ~55 s without a
  message; on resume it always opens a fresh socket. The server pushes the nearby list on
  connect (`Hub.attach`) and on `list`. Deploy the backend before an app build that pings.
- **Proximity rule, server-side only: GPS or Bluetooth.** GPS = `eligible` (`geo.go`):
  both fixes ≤ `freshFor` (2 min) old, both accuracies known and ≤ `maxAccM` (50 m: indoor
  phones report 20–50 m), centres ≤ 30 m. Bluetooth (`ble.go`): phones advertise a random
  8-byte token from `GET /api/ble-token` (rotates every 15 min; the latest one stays valid until
  replaced, up to 12 h, because a sleeping phone may not wake to fetch the next; a replaced
  one stays valid 15 min; memory only, revoked on hide) and report what they hear to `POST /api/sightings`
  (≥ `minRSSI` −90 dBm); a pair heard within `freshFor` is near. No slack, no hysteresis,
  and never refresh a timestamp without a new fix. `Hub.expire` (10 s) pushes removals for
  fixes and Bluetooth pairs that went silent. A socket's first list is sent immediately, not on
  the next flush: on connect only if the server can place the user (fresh fix or Bluetooth
  pair, `Hub.knows`), else on their next fix. Never send an empty list that only means
  "we don't know where you are". Phones that reported and
  went quiet for 3–10 min get one silent FCM `wake` push per 10 min (`Hub.quiet`).
- **Realtime pushes are coalesced and photo-free:** `notify` only marks watchers dirty;
  `Hub.run` flushes at most every 500 ms, max 50 cards. Cards carry `img` as an absolute
  URL (`<origin of LINKEDIN_REDIRECT_URL>/api/photo/{uid}?v=<updated unix>`, public,
  immutable-cached; absolute so every app build loads it) — never inline photos.
- Photos may only be `data:image/{jpeg,png,webp};base64,…`. No
  external URLs (viewer IP tracking) and no SVG (script on our origin).
- **Sessions:** tokens are stored as sha256 hex (never plaintext) and slide 90 days from
  last use. The socket token travels as a subprotocol (`["kinjo", token]`), not `?token=`
  (old builds still use the query param — remove that fallback once they're gone).
- Logs are JSON (`log/slog`; `log.Printf` routes through it) with one access line per
  request (`id`, path without query, status, ms). Never log query strings or tokens.
- Admin broadcasts insert every row in one statement first (durable), then deliver with
  8 workers. Daily retention (`applyRetention`) prunes sessions, notifications, ad events.
- **nginx rate limits key on the session, not the IP** for `/api/` (venue Wi-Fi and carrier
  CGNAT put many phones behind one IP); photos are cached by nginx. Never go back to
  per-IP-only limits — a room full of people would get 429s.
- `backend/deploy/`: production systemd unit, reference nginx (rate limits, log format),
  `backup.sh` + timer (off-box destination still to decide). `loadtest/` has the load test.
- **Email verification:** the welcome email carries the link (one email). Links are built
  from `App.publicURL` (origin of `LINKEDIN_REDIRECT_URL`), never the request Host header.
  Consuming a link is **idempotent until expiry** on purpose — corporate mail scanners open
  links before the person does. Resends: 1/minute (repeat taps are no-ops) and 3/hour,
  enforced atomically by `reserveVerification` (per-user advisory lock); the email itself
  is sent in the background so the API answers in milliseconds.
- **DB errors are 503, never 401** — the app deletes its token on 401. Keep that split.
- Never `close()` a client's `send` channel (others may still send → panic); close the conn.
- Exact distance never leaves the server (`nearbyPerson.D` is `json:"-"`, ordering only).
- Admin API is guarded by `ADMIN_TOKEN` (header `X-Admin-Token`); off if unset. The
  `/admin` UI in `kinjo-site-source` sends that token and reads `VITE_API_BASE`.

## Conventions

- Match the surrounding code's terse, comment-light style in `app.fb.js`.
- Verify DB/schema changes against a real throwaway Postgres before claiming they work.
- Nothing deploys automatically from here — make changes locally; the user ships them.
