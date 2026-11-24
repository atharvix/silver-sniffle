# Kinjo backend (Go)

The whole server. Owns:

- **Sign-in**: LinkedIn and Google (OIDC, browser flow), Sign in with Apple (iPhone), and an emailed
  6-digit code. No passwords, no Firebase for auth or data.
- **The database**: Postgres. Tables are created on boot (`store.go` and the `*Schema` constants).
- **Live matching**: an in-memory hub that decides who is near whom and pushes decks over WebSocket.
  Positions and encounters are never stored. The contract with the phones is `../docs/proximity.md`.
- **Push** (FCM as a transport, APNs location pushes for swiped-away iPhones) and **email** (SMTP).
- **Admin API** for Kinjo Control (campaigns, places, events, installs, exports, telemetry).

## Layout

| File | What it does |
|---|---|
| `main.go` | wiring, routes, access log, graceful shutdown |
| `config.go` | env-var config |
| `store.go` | Postgres schema (users, profiles, sessions, devices, notifications, ...) and queries |
| `auth.go` | LinkedIn / Google OAuth, PKCE hand-off, sessions, CORS, auth middleware |
| `apple.go`, `applejwt.go` | Sign in with Apple (token check, refresh-token revoke) |
| `email_login.go`, `mailer.go` | emailed sign-in codes, welcome / verification email |
| `demo.go` | the App Review demo account |
| `api.go` | `/api/me`, profile, visibility, logout, delete, device registration |
| `prox.go` | proximity: fixes, matching, decks, modes, wake pushes, housekeeping |
| `realtime.go` | WebSocket clients, delta decks, accept rate limit |
| `ingest.go`, `handover.go`, `batch.go` | latest-wins fix queue, restart hand-over via `/dev/shm`, batched app opens |
| `tokens.go`, `geohash.go` | Bluetooth tokens (older builds), geohash index |
| `cards.go` | profile card cache |
| `notify.go`, `fcm.go`, `apns.go` | notifications, FCM, iPhone location pushes |
| `control.go`, `places.go`, `events.go`, `leads.go`, `ads.go`, `cities.go`, `export.go` | Kinjo Control data and features |
| `admin.go`, `admin_auth.go`, `admin_store.go`, `telemetry.go` | admin API, emailed-code admin sign-in, metrics |
| `crypt.go` | AES-GCM sealing of stored credentials under `DATA_KEY` |
| `cmd/smoke`, `cmd/loadgen`, `cmd/loadv3` | post-deploy smoke test and load tools |
| `deploy/` | production systemd unit, reference nginx config, encrypted backup script + timer |

## Run locally

```bash
cp .env.example .env    # fill in DATABASE_URL and the LinkedIn credentials at least
set -a; . ./.env; set +a
go run .
```

Needs a reachable Postgres. `go test ./...` runs the matching tests without a database; the tests
that need one read `KINJO_TEST_DSN` (a throwaway database) and skip without it.

## Configuration

Everything comes from environment variables; `.env.example` lists them with comments. Required:
`DATABASE_URL`, `LINKEDIN_CLIENT_ID`, `LINKEDIN_CLIENT_SECRET`, `LINKEDIN_REDIRECT_URL`. Common:
`FRONTEND_URL` (`kinjo://auth` for the phone app), `ALLOW_ORIGINS`
(`https://localhost` for Android, `capacitor://localhost` for iPhone, plus the admin site),
`ADMIN_EMAILS`, `DATA_KEY`, `SMTP_*`, `FCM_*`, `GOOGLE_*`, `APNS_*`, `APPLE_*`, `DEMO_*`.
Never put a credential on a command line; write it to `.env`.

## Main endpoints

| Method | Path | Auth | Purpose |
|---|---|---|---|
| GET | `/auth/linkedin`, `/auth/google` (+ `/callback`) | none | provider sign-in (PKCE `challenge` from the app) |
| POST | `/auth/exchange` | none | redeem the one-time `#code=` with the PKCE verifier |
| POST | `/auth/email/start`, `/auth/email/verify` | none | emailed sign-in code |
| POST | `/auth/apple` | none | Sign in with Apple identity token |
| GET / PUT | `/api/me`, `/api/profile` | Bearer | account and card |
| PUT | `/api/visibility` | Bearer | hide / show my profile on every device |
| POST | `/api/presence` | Bearer | a fix (HTTP fallback) -> `{mode, next, sleep}` |
| POST | `/api/open`, `/api/install` | Bearer / none | app opens, installs |
| POST / DELETE | `/api/device` | Bearer | push token |
| GET | `/api/events`, `/api/events/{id}` | none | events (RSVP and attendees need a session) |
| GET | `/api/photo/{uid}` | none | card photo (404 while hidden), immutable per `?v=` |
| GET | `/ws` | token as subprotocol | realtime: `["kinjo", token]`, or `["kinjo", token, "reporter"]` for a phone's service |
| GET | `/healthz` | none | 200 only if the database answers |
| * | `/admin/...` | admin session or `X-Admin-Token` | Kinjo Control (every path must also be in nginx's `/admin/` regex) |

WebSocket messages and the phone protocol are in `../docs/proximity.md`.

## Deploy

See `../DEPLOYMENT_GUIDE.md`. In short: build `kinjo-api`, put it and `.env` in
`/var/www/kinjo-app/backend`, install `deploy/kinjo.service` and the nginx config, `systemctl
restart kinjo`, then run `cmd/smoke`. Diff the files in `deploy/` against the server before
replacing them.
