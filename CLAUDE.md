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
- `kinjo-site-source/` — the marketing website **and the admin panel** (Vite + React +
  wouter + shadcn). Admin lives at route `/admin` (`src/pages/Admin.tsx`).

The old Firebase build and the legacy React app were removed in the repo restructure.

## Hard invariants (don't break these)

- **No Firebase** for auth or data. Auth is LinkedIn OIDC only; data is Postgres. FCM is
  used *only* as a push transport.
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

`main.go` routes · `config.go` env · `store.go` Postgres (7 tables: users, profiles,
presence, sessions, devices, notifications, email_verifications) · `auth.go` LinkedIn OAuth
+ sessions + CORS · `hub.go`/`client.go` WebSocket presence & matching · `geo.go` 30 m math ·
`notify.go` + `fcm.go` + `mailer.go` notifications/push/email · `admin.go` admin API.

- The app returns from LinkedIn via the **`kinjo://auth`** deep link (native) or the URL
  hash (web). Backend `FRONTEND_URL` must be `kinjo://auth` for the app.
- **Sign-in is PKCE-bound (RFC 7636/8252):** the app sends a SHA-256 `challenge` to
  `/auth/linkedin`; the callback returns a one-time `#code=` (never the session token),
  redeemed at `POST /auth/exchange` with the verifier. Requests without a challenge (old
  app builds) still get the legacy `#token=` — remove that path once they're gone.
- LinkedIn's OIDC `picture` is a signed, expiring **100×100** URL. `fetchPicture` stores
  the bytes as a data URL (unchanged); later logins refresh it only while the stored photo
  is still a raw `media.licdn…` link. Sharper photos can only come from user uploads.
- **Notification delivery is deduped by `notifications.sent_at`** — each notification is
  pushed to a device at most once. Don't reintroduce "re-push all unread on every device
  register"; that spams users on every app open.
- **Positions go through one path: `applyPos` (`client.go`)**, shared by the WebSocket
  `pos` message and `POST /api/presence` (the phone sends via native `CapacitorHttp`,
  because the WebView throttles sockets in the background). The server — not the client —
  decides fix quality against the user's *stored* presence: a coarse fix never replaces a
  fresh good one, it only refreshes the timestamp. Keep `android.useLegacyBridge: true` in
  `capacitor.config.json`, or background location stops after ~5 min.
- **Realtime pushes are coalesced and photo-free:** `notify` only marks watchers dirty;
  `Hub.run` flushes at most every 500 ms, max 50 cards. Cards carry `img` as an absolute
  URL (`<origin of LINKEDIN_REDIRECT_URL>/api/photo/{uid}?v=<updated unix>`, public,
  immutable-cached; absolute so every app build loads it) — never inline photos.
- Photos may only be `data:image/{jpeg,png,webp};base64,…` (or legacy LinkedIn CDN). No
  external URLs (viewer IP tracking) and no SVG (script on our origin).
- **Sessions:** tokens are stored as sha256 hex (never plaintext) and slide 90 days from
  last use. The socket token travels as a subprotocol (`["kinjo", token]`), not `?token=`
  (old builds still use the query param — remove that fallback once they're gone).
- Logs are JSON (`log/slog`; `log.Printf` routes through it) with one access line per
  request (`id`, path without query, status, ms). Never log query strings or tokens.
- Admin broadcasts insert every row in one statement first (durable), then deliver with
  8 workers. Daily retention (`applyRetention`) prunes sessions, notifications, ad events.
- `backend/deploy/`: production systemd unit, reference nginx (rate limits, log format),
  `backup.sh` + timer (off-box destination still to decide). `loadtest/` has the load test.
- **Email verification:** the welcome email carries the link (one email). Links are built
  from `App.publicURL` (origin of `LINKEDIN_REDIRECT_URL`), never the request Host header.
  Consuming a link is **idempotent until expiry** on purpose — corporate mail scanners open
  links before the person does. Resends: max 3/hour per user (`recentVerifications`).
- **DB errors are 503, never 401** — the app deletes its token on 401. Keep that split.
- Never `close()` a client's `send` channel (others may still send → panic); close the conn.
- Exact distance never leaves the server (`nearbyPerson.D` is `json:"-"`, ordering only).
- Admin API is guarded by `ADMIN_TOKEN` (header `X-Admin-Token`); off if unset. The
  `/admin` UI in `kinjo-site-source` sends that token and reads `VITE_API_BASE`.

## Conventions

- Match the surrounding code's terse, comment-light style in `app.fb.js`.
- Verify DB/schema changes against a real throwaway Postgres before claiming they work.
- Nothing deploys automatically from here — make changes locally; the user ships them.
