# Kinjo v2: full architecture

Everyone with the app sees each other's profile within 30 m: app open, backgrounded, locked,
swiped away, or while moving.
Targets: warm path <= 5 s, cold start (swiped away) <= 7 s **best effort**, 1M users in year one.

## 1. Platform reality (decides everything)

| State | Android 16 | iOS |
|---|---|---|
| Open / backgrounded / locked | foreground service (`location`) | background location ("Always") |
| Swiped away | service dies. OS geofence, FCM high-priority, boot, WorkManager can restart it | app terminated. Only significant-change, region exit and **location push** relaunch it. Silent push and BLE restoration do **not** |
| Cold-start guarantee | none | none |

Consequences:
- No tech guarantees 7 s after a swipe. We get close with **wake on demand** (section 6) and measure.
- Geofence rings (Android: ~2 min average per Android docs) and iOS region/significant-change (minutes, unreliable after a swipe, reportedly broken on iOS 26) are **backstops that keep the last position fresh, not the fast path**. The fast path is a server-sent high-priority FCM / APNs location push. The iOS one depends on an Apple entitlement that can be denied.
- Test plan and pass/fail numbers: `docs/PROOF_PLAN.md`.
- Android needs "Allow all the time" (`ACCESS_BACKGROUND_LOCATION`): a service started from the background can't read location without it. Needs Play Console background-location approval.
- iOS needs "Always" + Apple's location-push entitlement (request it early) + a Mac to build.
- BLE is a 30 m *confirmation*, not the wake signal. iOS background BLE is weak.

## 2. System diagram

```
Android (Kotlin)            iPhone (Swift)
 GPS, BLE, geofence          GPS, BLE, region ring, location-push ext
        │  WSS (protobuf)  /  HTTPS (REST)
        ▼
  L4 LB (TLS) ─▶ Gateway (Go, stateless, ~20k sockets each)
                    │  auth cache -> write fix -> match -> push delta
                    ├─▶ Redis Cluster  (cells, positions, routing, rate limits, pub/sub)
                    ├─▶ Postgres       (users, profiles, devices, blocks, reports)
                    ├─▶ NATS           (async: wake, push, moderation, analytics)
                    └─▶ S3 + CDN       (photos; presigned upload)
  NATS ─▶ Wake worker ─▶ APNs (location push) / FCM (high-priority)
       ─▶ Moderation worker (photo classifier)   ─▶ Metrics/analytics
```

- Hot path (fix -> peer's screen) never touches Postgres or NATS.
- Gateways are stateless; any gateway can serve any phone. Cross-gateway delivery uses Redis pub/sub.

### As built (server, Nov 2026)

| Planned | Built | Why |
|---|---|---|
| Redis Cluster: cells, positions, pub/sub | Redis (one node or Cluster via `REDIS_CLUSTER`), keys in `server/hub.go` | as planned |
| NATS/Kafka async | NATS JetStream work queue for wakes (`server/wake.go`) | Kafka adds nothing at this size; analytics can be a second stream |
| S3 + CDN photos | S3-compatible storage, Cloudflare R2 in production (`server/photos.go`) | as planned |
| H3 res-10 cells | 0.001° grid cells, enough columns to cover ~111 m everywhere (`server/geo.go`, tested) | H3's Go library needs C (cgo), which breaks the one static binary; the grid gives the same guarantee |
| Session cache | 5 min per server, dropped on account deletion on every server | as planned |
| Metrics | Prometheus `/metrics` (local port only) | as planned |
| Load balancer + many gateways | one server today; more behind nginx `upstream` with no code change | add when one isn't enough |

## 3. Phones

### Android
```
VISIBLE: foreground service (location|connectedDevice), GPS 1-4 s, BLE adv+scan, WebSocket
   │ swipe / kill
ARMED:   Play Services geofence ring (~100 m) + boot receiver + 15-min WorkManager + FCM `wake`
   │ ring exit | wake | boot | tick  ->  start service, one fix, report
```
- OEM killers (Xiaomi, Oppo, Vivo): per-maker exemption card. Force-stop blocks everything (Android rule).
- Mock locations are detected (`isMock`) and reported as a flag.

### iPhone
```
VISIBLE: background location updates (blue indicator) + CoreBluetooth + WebSocket
   │ swipe
ARMED:   significant-change + region ring + location-push extension
   │ ring exit | significant move | location push  ->  ~10-30 s of run time, one fix, report
```
- Ask "While Using" first, then "Always". Sign in with Apple required with any other social login.
- Extension is its own target; it sends one fix over HTTPS (no socket) and exits.

### Wake layers (every one runs; first to fire wins)

| # | Android | iPhone |
|---|---|---|
| 1 | Foreground service, `stopWithTask=false`, START_STICKY | Background location while visible (blue indicator) |
| 2 | **BLE scan with PendingIntent** on the Kinjo service UUID: survives a dead process (to be proven), manifest receiver reports the heard token over HTTPS, no GPS needed | **iBeacon region** on a Kinjo UUID (advertised by visible Android phones, venue beacons): relaunches even after a swipe |
| 3 | High-priority FCM wake from the server (wake on demand) | APNs **location push** (entitlement) |
| 4 | Activity-recognition and geofence transitions (both are official exemptions to start a service) | Region ring, `CLVisit`, significant-change (backstops) |
| 5 | Boot / package-replaced receiver, 15-min WorkManager | Server-driven regions around where peers were last seen |
| 6 | User sets battery to Unrestricted (an official exemption and the best OEM-killer defence) | Background App Refresh must be on (checked, user told) |

Matching accepts a BLE-only report (`POST /ble {token, rssi}`), so a swiped Android phone can be found by a peer without any location read.

### Shared phone rules
- Same wire protocol and same server calls on both OSes; platform code only produces fixes and BLE sightings.
- Fix report cadence: moving 2-4 s, still 60 s (+ heartbeat on the open socket), armed 1 fix per wake.
- Reconnect with exponential backoff + jitter (avoids a thundering herd after a gateway restart).
- Tokens live in Keychain / Android Keystore. No secrets in the app.

## 4. Data

### Redis (all ephemeral)
| Key | Value | TTL |
|---|---|---|
| `c:{p6}:<cell10>` ZSET | uid scored by fix time. `{p6}` = H3 res-6 parent hash tag, so a neighbourhood lives on one slot | trimmed to 60 s |
| `pos:<uid>` | lat, lng, acc, ts, src | 60 s |
| `last:<uid>` | **coarse** H3 res-8 cell + ts, for wake on demand | 12 h |
| `sock:<uid>` | gateway id (+ socket count) | 90 s, refreshed by ping |
| `tok:<ble token>` | uid | 30 min |
| `wl:<uid>` | wake lock (SET NX) | 20 s |
| `wc:<uid>` | wake counter | 10 min |
| `hide:<uid>`, `blk:<uid>` | visibility flag, blocked set (cache of Postgres) | 5 min |
| `rl:<uid>:<route>` | rate-limit counter | window |

Exact positions live 60 s in memory. Only a coarse cell (~460 m) lasts 12 h. Neither goes to disk, logs or backups.

### Postgres (durable, no locations)
```
users(id, apple_sub, google_sub, email, email_verified, hidden, created_at, deleted_at)
profiles(user_id, name, headline, photo_key, photo_ver, updated_at)
devices(id, user_id, platform, push_token_sealed, apns_loc_token_sealed, app_ver, os_ver, last_seen)
sessions(token_sha256, user_id, device_id, expires_at)            -- sliding 90 days
blocks(blocker, blocked, created_at)
reports(id, reporter, target, reason, created_at, status)
consents(user_id, policy_ver, accepted_at)
```
Stored credentials are AES-GCM sealed under a key from the secret manager. Backups are encrypted.

## 5. Protocol

- **WebSocket, protobuf frames.** Client: `hello{token,ver,delta}`, `fix{lat,lng,acc,ts,seq,src,mock}`, `ble{seen[{token,rssi}]}`, `ping`. Server: `deck{add[card],remove[uid]}`, `vis{hidden}`, `kick`.
- **Card** = `{uid, name, headline, photo_url, ver}`. No distance, no coordinates, ever.
- **REST:** `/auth/*`, `/me`, `/me/photo` (presigned S3 upload), `/block`, `/report`, `/devices`, `/fix` (one-shot, used by the iOS extension and Android wake).
- Auth: first WS message carries the token (never in the URL). Sessions are opaque tokens, stored as sha256, cached in gateway memory for 10 min.
- Errors: database failure = 503, never 401 (a 401 makes the app log out).

## 6. Matching and wake on demand

**On a fix from A** (gateway, one pass, < 10 ms):
1. Validate: acc <= 100 m, speed < 100 m/s, ts not older than 30 s, not hidden, rate limit.
2. Write `pos`, `c:*` (ZADD), `last`.
3. Read A's cell + ring 1 (7 cells; ring 1 always covers > 114 m, more than 30 m + error). Drop blocked and hidden.
4. Distance: show if `d <= 30 + min(accA + accB, 40)`. A card is held 60 s after the last positive signal; drops only on a clear miss (`d > 45 + err`) to stop flicker.
5. Send deltas to both users' sockets (coalesce: flush <= 500 ms, new pair flushes at once).

**Stationary keep-alive (what makes a swiped phone findable without waking it):** a phone's last fix is the truth until it moves. When a phone reports `still` (accelerometer + no GPS drift), the server keeps its `pos` for up to 10 min instead of 60 s, marked `still`. A phone that was moving when it vanished gets the normal 60 s. Walking away after a swipe leaves a ghost until the ring/relaunch clears it, so a still-only card is shown with lower priority and is dropped as soon as any signal contradicts it. Most swiped-away iPhones are sitting on a table, so this covers the common case.

**BLE:** phones advertise a server-issued rotating token (15 min). A heard token resolves through `tok:`. Hit with RSSI >= -90 = match. Outdoors a GPS gap > 35 m + error vetoes it.

**Wake on demand** (what makes a swiped-away phone visible in time):
1. On a fix from A, find swiped/stale phones that **could** be within 30 m: last-known position within `30 m + v * age`, where `v` = 0.5 m/s if the phone was `still`, 2 m/s if it was walking, 15 m/s if in a vehicle (from the last reported activity). Ages beyond 10 min are skipped (the slow heartbeat below refreshes those). Cap candidates per fix (e.g. 50, nearest first).
   A slow **heartbeat wake** (every 5-10 min per swiped phone, budgeted per day) refreshes `last` so the radius stays small.
2. `SET wl:<uid> NX EX 20`. Skip if locked or `wc:<uid>` over budget (6 per 10 min).
3. Publish to NATS `wake.apns` / `wake.fcm`. The worker sends the location push / high-priority message.
4. The phone reports one fix (`/fix`), step 1 above runs, A sees the new card.
Budget: push 1-3 s + fix 1-4 s + report/match < 1 s = **3-8 s. Unmeasured, test first.**

## 7. Security, abuse, privacy

- Sign-in: Apple, Google, emailed 6-digit code (no passwords). Link accounts only on verified email.
- Anti-spoof: mock flag, impossible-speed rejection, per-account fix rate limits, device attestation (Play Integrity, App Attest) before we trust a new device.
- Photos: re-encode server-side to JPEG, strip EXIF, size cap, async moderation, report flow, takedown.
- Block = both sides invisible. Hide = server-enforced, fixes refused. Report queue with an admin tool.
- Privacy: policy + consent screens, delete account removes everything, data inventory kept current. India DPDP and GDPR apply. Store listings need the "Always" purpose text, Play's background-location video, App Store privacy labels.
- Safety: meeting strangers has real risk. Minimum: block/report, no distance shown, profiles are opt-in, minors excluded (18+ gate).

## 7b. What is stored, what people see, what admins see

| Data | Stored where | Who sees it |
|---|---|---|
| Card: name, what you do, what you're looking for, photo | Postgres + photo store | people within 30 m; admins (moderation) |
| Account: id, sign-in method, email, verified, created, hidden, deleted | Postgres | the user; admins |
| Devices: platform, app/OS version, push token (sealed) | Postgres | admins see platform/versions only, never tokens |
| Sign-up city (nearest city to the first fix, set once) | Postgres | admins (growth by city) |
| Daily usage (one row per user per day) | Postgres | admins (DAU/WAU/retention) |
| Blocks, reports | Postgres | admins (report queue); blocks never shown to the blocked person |
| Live position, BLE tokens heard, who is near whom | memory only (TTL) | **nobody**; never stored, logged or exported |

Admin panel ("Kinjo Control") shows: totals and growth, user list and profile, report queue with hide / remove photo / delete account, platform and app-version mix, sign-up cities, and a **live heat map of online people snapped to ~500 m squares with at least 3 people per square** (no individual position, no square that singles one person out). Admin sign-in: emailed 6-digit code to allow-listed addresses only, sessions stored as sha256, every admin action written to an audit log (who opened which account, when).

## 8. Scale and failure

| Users | Online | Fix rate | Setup |
|---|---|---|---|
| 10k | 2k | ~0.4k/s | 1 VPS: gateway, Redis, Postgres, NATS |
| 100k | 20k | ~4k/s | 2 gateways, Redis primary+replica, managed Postgres |
| 1M | 200k | ~40k/s | ~10 gateways, Redis Cluster (3+ shards), PG primary+replicas, NATS cluster |

Redis memory is small (~200 B per online user). Shard geographically: nearby people share a region, so regions scale independently. Start with one region (India).

| Failure | Behaviour |
|---|---|
| Gateway dies | phones reconnect with jitter to another gateway, first deck on attach |
| Redis shard fails | replica promoted (~10 s), that area has no matches meanwhile |
| Postgres down | cached sessions and the CDN keep running, profile edits/sign-in fail with 503 |
| APNs/FCM down | wake retried with backoff, warm path unaffected |
| Bad deploy | rolling update, canary one gateway first |

## 9. Operations

- **Metrics (Prometheus + Grafana):** sockets, fix/s, match latency, `first_match` (time from the later phone's first fix to the card, split by path), wake sent/ok, wake-to-card time, 5xx, queue depth. Logs are JSON, never contain coordinates or tokens.
- **Alerts:** first_match p95 > 5 s, wake failure rate, socket drop spike, Redis memory/latency, 5xx.
- **Deploy:** start with docker compose on one VPS behind nginx/Caddy, GitHub Actions for test + build, then Kubernetes when one box isn't enough. Secrets in env / a secret manager, never in git.
- **Backups:** encrypted nightly Postgres dump to off-box storage, restore tested.

## 10. Testing

- Unit: matching maths, cell ring coverage, ranking, protocol.
- Simulation: thousands of fake phones walking around, measure `first_match` and Redis load.
- Real devices (the proof): Nothing A001T now, an iPhone later. Scenarios: open, locked, swiped, moving, in a pocket for an hour, Doze overnight. Two phones log timestamps; the harness reports seconds to first card.

## 11. Build order (risk first)

0. **Android spike:** project skeleton, foreground service + geofence + FCM wake, measure survival and wake time on the A001T.
1. Server: Go skeleton, schema, auth, sessions.
2. Cell index + matching (Redis) with tests and a simulator.
3. WebSocket hub, deck deltas, `/fix`.
4. Profile + photo API, block/report, moderation hook.
5. Wake worker (FCM first, APNs later).
6. Android app screens built to the old design (reference only).
7. iOS spike (needs a Mac): location push, ring relaunch, background BLE. **Before any iOS screens.**
8. iOS app screens, store submissions, load test, beta.

Exit rule for every step: it builds, its tests pass, and the one thing it adds is measured on a real phone.

## 12. Open risks

- iOS cold start after a swipe depends on Apple's entitlement and rate limits.
- Play background-location approval and App Review can each take days to weeks.
- OEM battery killers outside stock Android.
- Numbers in sections 6 and 8 are estimates until step 0 and the simulator measure them.
