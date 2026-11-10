# CLAUDE.md — Kinjo workspace

Guidance for working in this repo. Keep it accurate; update it when the shape changes.

## What this is

Kinjo is a "find your people within 30 m" app. The **current, active** build is a
Firebase-free rewrite:

- The product lives at the repo root:
  - `backend/` — **Go** API (LinkedIn OIDC auth, Postgres, WebSocket presence/matching,
    FCM push, SMTP email, admin API). This is the whole server.
  - `frontend/web/` — the app UI (`src/app.fb.html` + `src/app.fb.js`, a single-file SPA).
  - `frontend/native/` — Capacitor wrapper; **bundles** `frontend/web` into the APK / iPhone app.
    Native presence lives in `frontend/native/plugins/kinjo-presence` (`android/` Java,
    `ios/` Swift, same JS API). iPhone build + notes: `frontend/native/ios-setup/README.md`.
- `kinjo-site-source/` — the marketing website **and the admin panel** (Vite + React +
  wouter + shadcn). Admin lives at route `/admin` (`src/pages/Admin.tsx`).

The old Firebase build and the legacy React app were removed in the repo restructure.

## Hard invariants (don't break these)

- **No Firebase** for auth or data. Sign-in, all via our backend: LinkedIn OIDC, Google
  OIDC when `GOOGLE_CLIENT_ID/SECRET` are set (browser flow, not the native Google SDK, so
  no SHA-1 setup), Sign in with Apple on iPhone only (`apple.go`: Apple's native sheet → `POST
  /auth/apple` with the identity token; we verify Apple's RS256 signature, issuer, audience =
  bundle id or `APPLE_BUNDLE_ID`, expiry and the app's nonce; uid prefix `ap_`, column
  `apple_sub`; with `APPLE_SIGNIN_KEY_*` set, the sign-in's authorization code becomes a refresh
  token in `users.apple_refresh`, revoked by `removeAccount`), and an emailed 6-digit code (`email_login.go`, no passwords). On iPhone,
  Google/LinkedIn open in `ASWebAuthenticationSession` (plugin `webAuth`), which shares Safari's
  sign-ins; the app passes the last Google address as `hint` (Google's `login_hint`). Data is
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

`main.go` routes · `config.go` env · `store.go` Postgres (users has `linkedin_sub`, `google_sub` and/or `apple_sub`; tables: users, profiles,
sessions, devices, notifications, email_verifications, ble_tokens, app_opens) · `auth.go` LinkedIn OAuth
+ sessions + CORS · `prox.go` proximity (matching, decks, wakes, housekeeping) · `tokens.go` Bluetooth
tokens · `geohash.go` · `realtime.go` WebSocket · `cards.go` card cache ·
`notify.go` + `fcm.go` + `mailer.go` notifications/push/email · `admin.go` admin API.

- The app returns from LinkedIn via the **`kinjo://auth`** deep link (native) or the URL
  hash (web). Backend `FRONTEND_URL` must be `kinjo://auth` for the app.
- **Sign-in is PKCE-bound (RFC 7636/8252):** the app sends a SHA-256 `challenge` to
  `/auth/linkedin`; the callback returns a one-time `#code=` (never the session token),
  redeemed at `POST /auth/exchange` with the verifier. Requests without a challenge (old
  app builds) still get the legacy `#token=` — remove that path once they're gone.
- **No sign-in method ever fetches a photo**, and only LinkedIn's name is used (`signIn`):
  Google and Apple names are dropped, people type their own (Apple's sheet asks for the email
  only). Everyone uploads their own photo (a card can't be saved without one). LinkedIn seeds a
  *new* profile only; an existing profile is never touched on login. The sign-in screen shows
  no remembered account; the device keeps only the last address per method (`account`, for
  Google's `login_hint`).
- **Hide guard:** after hide/logout/delete (`Hub.leave`), HTTP fixes taken before that moment
  are dropped and the account's tokens are revoked (reports in flight are refused). A socket
  `pos` lifts the guard, since a socket is ordered.
- **Schema comments must not contain `;` mid-line** — the schema is split on `;`.
- **Notification delivery is deduped by `notifications.sent_at`** — each notification is
  pushed to a device at most once. Senders *claim* the row first (`claimNotification` /
  `claimUndelivered`, an atomic `UPDATE … WHERE sent_at IS NULL`) and unclaim if no device
  took it; never go back to read-then-send (`deliver` and a device registration raced and
  every new user got the welcome push twice). Don't reintroduce "re-push all unread on every device
  register"; that spams users on every app open. A device row carries the session that
  registered it (`devices.session`, hashed): logout deletes that session's devices (and the
  app unregisters its token), so a signed-out phone gets nothing; the account's other
  phones keep theirs. Visible pushes go only to devices with `notify` (the app posts its
  notification permission with the token, and again when it changes): Android drops a push the
  user hasn't allowed while FCM still counts it sent, so the welcome was marked delivered and
  never shown. Silent `wake` pushes still go to every device.
- **One account, many devices:** the hub keeps every socket per account
  (`Hub.clients` uid → set). Never go back to "the newer socket closes the older": two
  phones on one account then kicked each other off forever (each reconnect closed the other).
- **Proximity v2 — the contract is `docs/proximity.md`; read it before touching proximity.**
  Change the doc, `prox.go` and both plugins together. In short: Bluetooth decides (phones
  advertise a server token, Herald-style connections + Android relays for iPhones), GPS only
  while the app is open. Outdoors (both fixes ≤ 15 m) fixes > 35 m + mean error apart veto
  Bluetooth (it carries 80 m+ outdoors); indoor fixes never veto. No outdoor RSSI floor (it made
  walk-pasts take 17 s). Target: every findable pair within 5 s; measure changes with
  `loadtest/sim/run.sh` (`VARIANTS=D`, results table in `docs/proximity.md`). Location match (v3, moving to location first; see the doc's v3 section): ≤ 35 m + the fixes' combined error (cap +40 m, fixes ≤ 100 m only), a shown card goes at once past that +5/15 m, `still` fixes stay 10 min while re-sent, `/api/presence` and the phone service's **reporter socket** (`["kinjo", token, "reporter"]`: no decks, no server pings, the open socket keeps a still fix placed (no wake-ups in Doze), nginx TCP keepalive finds dead phones, closed = gone in 30 s) answer `mode`/`next` (with/near/around/idle: GPS chip only in with/near), Android pairs also match on hashed Wi-Fi overlap; socket fixes go through a latest-wins queue (`ingest.go`), apps that say `hello {delta:true}` get delta decks, a restart hands positions to the next process through `/dev/shm` sealed with `DATA_KEY` (`handover.go`), new sockets are rate-limited, app opens are batched (`batch.go`), and `near` mode tells the phone how long GPS may rest (`sleep`, rounded) (geohash precision 7 index). A direct hearing
  ≥ `minRSSI` (−100 dBm, the radios' floor) counts `nearFor` (45 s; phones refresh every 20 s closed / 10 s open, someone new at once); a card is held `holdFor` (60 s) after the last signal
  (timed by the signal); a relay alone never shows a card, it sends both phones `scan`/`wake`.
  **Nothing about positions or who met whom is stored** (the old `presence` table is dropped;
  never bring back location at rest). No background location on either platform.
- **Phone side = `frontend/native/plugins/kinjo-presence`** (our own Capacitor plugin,
  tracked in git — `frontend/native/android/` is generated and gitignored, never put native
  code there; `android-build.sh` strips background-location permissions from its manifest).
  Android: `PresenceService` (foreground, type `connectedDevice|location` (location only when the open app starts it), `stopWithTask=false`; v3: `Gps` + `Live` socket + `Wifi` are the main signal, Bluetooth optional) runs
  `Ble` (BALANCED scan in the background, LOW_LATENCY open; up to 3 iPhone connections
  at once, failed ones retried after 2, 5, 10, 20, 40, 60 s) and, while the app is on screen,
  `Gps` (one fix per 10 s). `BLUETOOTH_SCAN` is
  `neverForLocation` — never drop it (Android then hides results from anyone without precise
  location; AOSP's denylist filters only iBeacon/Eddystone frames). Restarts: `START_STICKY`,
  `Check` (boot + 15-min WorkManager) and `WakeService` (server's `wake` push: any priority
  bursts a live service, high priority may start it). **`WakeService` replaces Capacitor's
  `MessagingService` in the manifest: keep the pair or Android pushes stop.** `Net` sends 3
  requests at once, a wake lock each; 409 on sightings = fetch a token now. Force-stop ends
  everything until the app is opened, by Android design.
- **Scan screen = loading step:** presence starts during the splash; the scan ends the moment
  a non-empty list is in (deck already built hidden, photos loading; 600 ms minimum so it
  doesn't flash; no waiting on photos). Card photos load only near the front (`photosNear`: 2
  behind, 4 ahead, front card first), so a crowd of 50 doesn't queue 50 downloads ahead of
  the card on screen, or after 8 s with nobody (later arrivals slide in). App open goes straight to the deck if the
  list is already there. **The deck is never blanked on resume or socket loss** (that made
  cards blink and reset to the first card): it stays until the fresh list arrives and
  `setPeople` diffs it. Back after longer than `STALE_MS` (2 min, the server's expiry), the
  old deck is never shown (people who left would flash and vanish): the app goes back to
  the scan screen and rebuilds from the fresh list. While open, `tick` clears it after
  `STALE_MS` with no word from the server.
- **Socket liveness:** the app sends `ping` every 25 s and reconnects after ~55 s without a
  message; on resume it always opens a fresh socket. The server pushes the nearby list on
  connect (`Hub.attach`) and on `list`. Deploy the backend before an app build that pings.
- **Realtime pushes are coalesced and photo-free:** changes only mark decks dirty;
  `Hub.run` flushes at most every 500 ms, max 50 cards, except that a new pair or a first fix
  flushes at once (`Hub.arrived`, one pending signal, so a crowd arriving is still one flush).
  A socket's first deck goes out on attach only if the server can place the user (`knows`);
  never send an empty list that only means "we don't know where you are". A send that hits a
  full buffer re-marks the deck dirty. Phones that reported and went quiet get a silent FCM
  `wake` at 75 s (20 s after a reporter socket closes: `wakeLost`), then one per 8 min for 12 h (`Hub.quiet`; an iPhone with a location-push token also gets an APNs location push: its `KinjoLocationPush` extension sends a `still` fix even when Kinjo was swiped away); fetching a token counts as a report. Cards carry `img` as an absolute
  URL (`<origin of LINKEDIN_REDIRECT_URL>/api/photo/{uid}?v=<updated unix>`, public,
  immutable-cached; absolute so every app build loads it) — never inline photos.
- Photos may only be `data:image/{jpeg,png,webp};base64,…`. No
  external URLs (viewer IP tracking) and no SVG (script on our origin).
- **Sessions:** tokens are stored as sha256 hex (never plaintext) and slide 90 days from
  last use. A checked session is trusted from memory for `sessionCacheTTL` (10 min:
  phones call the API every 5–10 s; at 1 min 100k phones were ~1,700 queries/s). `deleteSession` and `deleteAccount` drop cache
  entries first, so logout and deletion take effect at once; DB errors are never cached. The socket token travels as a subprotocol (`["kinjo", token]`), not `?token=`
  (old builds still use the query param — remove that fallback once they're gone).
- Logs are JSON (`log/slog`; `log.Printf` routes through it) with one access line per
  request (`id`, path without query, status, ms), except successful fast phone reports
  (`quietPath`: sightings, presence; they're in telemetry): logging each took most of the CPU
  under load. Errors and requests over 500 ms still log. Never log query strings or tokens.
  `first_match` (`via` gps|ble, `after_ms` since the later of the pair started reporting)
  logs each card arriving on a deck: the discovery-latency number.
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
- **"Hide my profile" is per account, enforced by the server** (`users.hidden`, `Hub.hidden`,
  `PUT /api/visibility`): a hidden account's fixes and Bluetooth reports are refused (another
  phone's service may still be running), nobody sees it and its deck is empty, on every device
  and old build. Open apps get `{type:"vis"}` and stop their service; `/api/me` carries
  `visible`. A hide the server didn't get (offline, or kept on the phone by an older build) is
  sent on the next `loadMe` (`visSync:<uid>`). The socket `hide` message (logout) does not set it.
- **App Review demo account** (`demo.go`): `DEMO_EMAIL` signs in with the fixed 6-digit
  `DEMO_CODE` (no email; 20 wrong codes/hour closes it) and its deck always carries the
  `demo_*` sample cards (`backend/demo/*.jpg`, seeded on boot). They have no position, so
  nobody else sees them; hiding the demo account hides them.
- **DB errors are 503, never 401** — the app deletes its token on 401. Keep that split.
- Never `close()` a client's `send` channel (others may still send → panic); close the conn.
- **Encryption at rest:** stored credentials (`users.apple_refresh`) are sealed with AES-256-GCM
  under `DATA_KEY` (`crypt.go`, `seal`/`unseal`, prefix `v1:`; older plaintext rows still read and
  are sealed on their next write). Losing the key makes them unreadable. Backups are encrypted
  with `age` to `BACKUP_AGE_RECIPIENT` (public key only on the server) and `backup.sh` refuses to
  upload a plaintext dump.
- Distance never leaves the server: cards are ordered by it, never carry it.
- **Admin telemetry** (`telemetry.go`, global `tel`): in-memory only, counts and timings, never
  uids/places. 60 one-minute buckets (requests/4xx/5xx/slow, fixes, matches by GPS/BLE, sightings,
  wakes, push/mail ok/fail) + latency and first-match percentiles + per-route stats keyed on the mux
  pattern. The hub's per-minute proximity counters feed it in `sweep` (still swapped once, there).
  `/admin/metrics` returns it with DB pool/size and host (load, mem, disk); `/admin/growth`
  (signups by method from the uid prefix li_/g_/em_, DAU/WAU/MAU from `app_opens`, funnel,
  D1/week-2 retention); `/admin/user?uid=` (detail, never push tokens or positions);
  `/admin/user/signout` and `/admin/user/delete` (shares `removeAccount` with the app's delete).
  UI: `kinjo-site-source/src/pages/AdminOps.tsx` (views, drawer) + `Admin.tsx` (shell, forms, ads).
- Admin API is guarded by `ADMIN_TOKEN` (header `X-Admin-Token`); off if unset. The
  `/admin` UI in `kinjo-site-source` sends that token and reads `VITE_API_BASE`. Every
  `/admin/<name>` API path must also be listed in the nginx `location ~ ^/admin/(…)` regex,
  or production serves the website for it.
- **Usage:** the app posts `POST /api/open {platform}` on start and on every resume;
  `app_opens` keeps one row per user per day (India time, `usageTZ`), repeats within 30 s
  merge. `/admin/usage` and the admin "Usage" tab show it. Devices carry `platform`
  (android/ios/web, from `Capacitor.getPlatform()`) and the app's own random install id
  (`device_id`). iOS push needs the Firebase iOS SDK first (a raw APNs token is not an FCM
  token, FCM rejects it): `ios-setup/AppDelegate.swift` hands Capacitor the FCM token instead.
- **iPhone** (`com.kinjo.app`, built by `ios-setup/ios-build.sh` on macOS / the manual
  "Build iOS" workflow): the webview origin is `capacitor://localhost`, so it must be in
  `ALLOW_ORIGINS`. Background modes: bluetooth-central, bluetooth-peripheral,
  remote-notification, location (v3: "While Using" started from the open app keeps running, blue indicator; `Presence.swift` + `Live.swift` reporter socket; still = coarse updates + accelerometer wake). Bluetooth (`Ble.swift`, Herald-style): advertises `KINJO`
  (token as local name while open), GATT `TOKEN` + `SIGNAL`, keeps up to 8 connections and
  writes keep-alives on `didReadRSSI` callbacks (background timers don't fire), reports `0x02`
  relays. State restoration ids are fixed. Location: "While Using"; v3 keeps it running in the background once started from the app (modes as on Android, no Wi-Fi on iPhone). The location-push extension (`ios-setup/KinjoLocationPush`, built with `LOCATION_PUSH=1` / the
  workflow's `location_push`, on by default: Apple granted the entitlement and `group.com.kinjo.app`; needs "Always" on the phone; server key `APNS_KEY_ID=NRDAWC5YX6`, team from `APPLE_TEAM_ID`; `kinjo-api apns-check` asks Apple if the key works) and `apns.go` are back (v3): a swiped-away
  iPhone answers the server's location pushes with a `still` fix; without them it is not found until reopened (iOS rule). Plugin `KinjoPresence` also carries Sign in with Apple and
  `webAuth` (`SignIn.swift`) — keep those methods. The plugin reports battery as unrestricted,
  so that card never shows. Sign in with Apple is still needed for App Store review.

- **App is dark only** (no theme setting). **Permissions:** Android's notification prompt shows as soon as a new account's profile
  screen appears (`notifOnce`, once per install; older installs get it after their cards). Then,
  when the profile is saved, the screen pauses, dimmed (no blur in the phone app, Android and
  iPhone alike; the website blurs lightly), under centred cards (`permSheet`,
  `#pm`, copy is the founder's, use it word for word): one card, location (no background-location
  card since proximity v2). Location can't be skipped (Not now nudges the card and stays; "While
  using" or "All the time" both count; in the phone app its Allow also asks for Bluetooth, and
  either one passes it, `canFind`). Skips are tracked per account (`pmDue`, `pmSkip:<uid>`) for
  cards that allow skipping. The battery card (`battery({ask:true})` shows
  Android's own "always run in background?" dialog, `REQUEST_IGNORE_BATTERY_OPTIMIZATIONS`) is in
  `STEPS` but due only on makers that stop background apps (`KILLERS`), skippable. Bluetooth's prompt follows location's; then a Bluetooth card (`bt`, placeholder copy until
  the founder writes it) while it's off or not allowed: Allow switches it on (`enableBluetooth`:
  Android's dialog, iOS's power alert), Not now moves on, back on every app start. Without location
  (and, in the phone app, without Bluetooth) later, its card stays over the scan screen (`needLoc`).
  The scan screen says when Bluetooth is off or not allowed. Native permission checks time out after
  3 s (`soon`) so a silent plugin can't leave the screen stuck. No Settings row for permissions.

## Conventions

- Match the surrounding code's terse, comment-light style in `app.fb.js`.
- Verify DB/schema changes against a real throwaway Postgres before claiming they work.
- Nothing deploys automatically from here — make changes locally; the user ships them.
