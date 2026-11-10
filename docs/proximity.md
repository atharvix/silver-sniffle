# Kinjo proximity (v2)

Who is within ~30 m of whom, on Android, iPhone and the web. Rebuilt Nov 2026 to replace
the v1 system (background GPS + Bluetooth, `hub.go`/`geo.go`/`ble.go`). This file is the
contract between the phone plugins and the server; change both sides together.

## Why this design

- Every system that worked at scale for phone-to-phone proximity (Herald / Australia's
  COVIDSafe, Singapore's BlueTrace, Apple+Google GAEN) uses the same shape: phones advertise a
  short random token issued by a server, hear each other over Bluetooth Low Energy, and the
  server decides who is near. v1 had that shape; it failed on the details, mostly iPhones.
- iPhones can't be heard in the background by plain scanning (iOS moves a background app's
  advert to the "overflow area"). Herald's answer, ported here: **connect and stay connected**,
  each side **writes** a small frame to the other's signal characteristic (a write wakes the
  receiving app), and **Android phones relay** to iPhones the tokens they hear.
- GPS is indoors 20–60 m off, and Bluetooth signal strength is not a precise distance (Leith &
  Farrell 2020: little correlation in trams; outdoors phones 40 m apart still hear each other).
  So Bluetooth decides, with a signal-strength floor, and good outdoor GPS can only *veto*.
- No background location. Bluetooth needs only "Nearby devices" (Android 12+) / Bluetooth
  (iPhone). Location is optional, used while the app is open.

What it cannot do (OS limits, not bugs): find a force-quit app or a phone with Bluetooth off;
reliably find two locked iPhones with no Android phone near them; draw an exact 30 m line.

## Radio protocol

| Item | Value |
|---|---|
| Service UUID `KINJO` | `6b1a0c30-4f9e-4b8e-9c55-2d3c7e1f0a30` |
| `TOKEN` characteristic (read) | `6b1a0c31-4f9e-4b8e-9c55-2d3c7e1f0a30`, value = the 8-byte token |
| `SIGNAL` characteristic (write; also notify on iPhone) | `6b1a0c32-4f9e-4b8e-9c55-2d3c7e1f0a30` |
| Token | 8 random bytes from `GET /api/ble-token`, lower-case hex on the wire to the server |

SIGNAL frames, one per write (9 bytes: fits the default 20-byte ATT payload):

| Frame | Meaning |
|---|---|
| `0x01` + 8-byte token | "I am this token" (keep-alive; wakes the receiver) |
| `0x02` + 8-byte token | relay: "I heard this token directly in the last 30 s" |

Unknown frame types are ignored (room for later versions).

**Android** advertises two things: (1) non-connectable, service data `KINJO` = token (Android
↔ Android, and what open iPhones read); (2) connectable, service UUID `KINJO`, backed by a GATT
server with `TOKEN` and `SIGNAL` (for iPhones). In the background it scans BALANCED (LOW_POWER left pocketed
phones 10–20 s apart), with full speed while open or bursting. It scans with three filters: service data
`KINJO`, service UUID `KINJO` (open iPhones put the token in their local name), and Apple's
overflow record with the iOS bit (background iPhones). For an iPhone without a name it connects (up to 3 at once),
reads `TOKEN`, writes `0x01` (own token) and `0x02` frames for iPhone tokens it heard directly in
the last 30 s (at most 10), and disconnects. Failed connects back off 2, 5, 10, 20, 40 s, then 60 s (both platforms).

**iPhone** advertises service UUID `KINJO` (local name = token while open; iOS drops the name
and moves the UUID to overflow in the background), with a GATT server (`TOKEN`, `SIGNAL` with
notify). It scans for `KINJO`, connects to what it finds, reads `TOKEN`, subscribes to `SIGNAL`
on iPhones, and keeps up to 8 connections open. Its loop runs on Bluetooth callbacks (iOS
background timers don't fire): each `didReadRSSI` re-arms the next read after ~8 s, and on each
it writes `0x01` to the peer, which keeps both apps awake. A `0x02` frame received is reported
as a relay. Bluetooth state restoration brings the managers back after iOS ends the app.

## Phone ↔ server API (Bearer session token)

| Call | Body / answer |
|---|---|
| `GET /api/ble-token` | → `{token, every, iosBit}`; fetch again after `every − 30` s. Counts as "alive" for wake pushes. |
| `POST /api/sightings` | `{"seen":[{"tok":"16 hex","rssi":-70}, {"tok":"…","via":"relay"}]}`, ≤ 32 per request; rssi omitted = heard over a connection (no RSSI). 204; **409 = the server doesn't know your token: fetch one now.** |
| `POST /api/presence` | `{lat,lng,acc,age,still?}` → `{ok, mode, next}`. v2 phones send it only while open; v3 phones (below) also in the background. `mode`: `with` (someone in range or heard), `near` (anyone placed within ~1 km), `idle`; `next`: the longest the phone may wait before reporting again, moving or not (5 / 15 / 120 s; 120 s also bounds how long a still idle phone is missing after a server restart). Old builds ignore both. |
| WebSocket `/ws` (subprotocol `["kinjo", token]`) | server → `nearby {people}`, `vis`, `notif`, `pong`, `scan`; app → `ping`, `list`, `hide`, `pos` (browser) |

Reporting: a refresh every 10 s while open, 20 s while closed (halved from 5/10 s for server load); a token not heard for 8 s goes out after
300 ms (that is what puts a new card up in about a second). Wake: an FCM data push `kind=wake`
(or socket `scan`) makes the phone scan at full speed for 30 s and report.

## Server rules (`backend/prox.go`)

All in memory; nothing about who was near whom is stored.

- **Pair** (two accounts): last direct hearing (time, RSSI), last relay.
- **Direct and close** = heard directly within 45 s with RSSI ≥ −100 dBm, the radios' floor
  (or over a connection), either side hearing counting for both. Pocketed phones 20–30 m apart
  or across a wall read −95…−100; cutting them off at −95 cost 15–25 s on those pairs.
- **Location match** (v3, Nov 2026; the move to location first, see "v3" below) = both fixes
  fresh and both accuracies ≤ 100 m (Android's "approximate" never matches), centres ≤
  35 m + √(accA² + accB²) apart, the widening capped at 40 m (accuracy 0 = unknown counts as
  40). Indoor fixes therefore widen the line instead of missing anyone within 35 m. Once a
  card is shown it stays until the gap passes that line by 5 m (both fixes ≤ 15 m) or 15 m,
  and then it goes **at once**, with no hold: the margin is what stops noisy fixes from blinking it.
  Candidates come from a geohash index (precision 7, the cell plus its 8 neighbours).
- **Fresh** = taken ≤ 2 min ago, or, for a fix sent with `still: true`, re-sent ≤ 10 min ago (a
  phone that isn't moving re-sends its last fix instead of taking new ones; the server only
  refreshes it, no deck work).
- **Through a third phone** — if one phone hears both A and B at ≥ −85 dBm within 30 s (each
  within ~15 m of it, closer through pockets), A and B are near. This is how two locked iPhones,
  which can't hear each other, find each other in a crowd. Worked out per deck (open apps only),
  not per report: per report it capped the server near 4,500 reports/s.
- **Outdoor veto** — when both have fixes ≤ 60 s old with accuracy ≤ 15 m (open sky), the
  location match overrules Bluetooth (it carries 80 m+ outdoors).
  Indoor fixes are vaguer (and sometimes 40 m off while claiming 10 m) and never veto, so
  same-room pairs never blink because of GPS. (A −85 dBm outdoor floor was tried: it made
  walking past someone take 17 s. Removed.)
- **Card shown** while direct-and-close or location match, not vetoed; **held** until 60 s pass
  with neither (stops blinking when a few adverts are missed), except that two fresh fixes
  apart with no Bluetooth signal drop it at once (above).
- **Relay alone never shows a card** (the relaying phone could sit between two people 60 m
  apart): it sends both phones a `scan` (socket) / `wake` (push) so they check directly.
- **Delivery**: a deck change marks the viewer dirty; decks flush every 500 ms, and a fix or
  a new pair flushes at once, at most every 50 ms (a crowd reporting is ≤ 20 flushes/s). Cards are `{uid,name,role,desc,img}`, max 50, closest first (RSSI, then GPS distance),
  photos by URL only.
- **Wake pushes**: a phone that reported and went quiet gets a silent push at 75 s, then every
  10 min for 12 h.
- **Hide / logout / delete** (`Hub.leave`): drops the account's pairs, fix and tokens at once
  and refreshes everyone's deck; reports in flight from before are dropped (hide guard).

## v3: location first (in progress, Nov 2026)

Why: on a real Android 16 phone (Nothing, 10 Nov 2026) the OS paused Kinjo's background
Bluetooth scan 77% of the time with the screen off (111 s of 145 s), Bluetooth needs its own
permission and switch, and it can't find two locked iPhones. v3 makes location the signal and
keeps Bluetooth only for old builds until they are gone.

- **Phones** (to build): one location permission ("While using"; iPhone asks for "Always"
  later). Android: `PresenceService` becomes foreground type `location`, started from the
  app, so it keeps "while in use" access when locked or swiped. iPhone: background location
  updates (`allowsBackgroundLocationUpdates`, `pausesLocationUpdatesAutomatically = false`).
  Report on movement (5 m) in `with`/`near`, every `next` seconds otherwise, `still: true`
  while not moving (Android significant-motion sensor, iPhone `isStationary`: no permission).
  `idle` = no GPS chip (Wi-Fi/cell only). Runs while the profile is visible: no other switch.
- **Server** (done): location match + keep margin + instant leave, still fixes (a re-sent fix
  within 2 s of the stored one is the same fix: its `age` wobbles by network delay), `mode`/`next`,
  flush on every fix (≤ 20/s).
- **Android** (done, `Gps.java`, not yet on a device): the service runs on location alone
  (Bluetooth optional); it takes the location type when the open app starts it or comes to the
  screen, never from a background start (boot / Check / push bring back Bluetooth only until
  the app opens). Precise updates every 5 m in `with`/`near` or while open, Wi-Fi/cell every
  100 m in `idle`; still after 2 min within 25 m (one precise fix, then no updates; woken by the
  significant-motion sensor; phones without one never go still); never more than 1 report/s,
  never longer than `next` without one. An older server's `{ok:true}` keeps mode `near`.
- **Next**: native socket from the plugin (no HTTP per fix), delta messages (`+card`/`-uid`)
  instead of full lists, crossing prediction from speed/heading, geo-sharded hub with
  latest-fix-wins inboxes instead of the one `Hub.mu`, then Bluetooth removed.

## Measured (50-phone simulation, Nov 2026)

`loadtest/sim/run.sh` (needs Go, Node 22+, Postgres binaries; `VARIANTS=D` is the v2 phone
model) runs 650 modelled phones against the real server. Radio, OS limits and GPS error are a
model; the server is real. First card, typical / worst (p95):

| Scenario | Found | Typical | Worst |
|---|---|---|---|
| Both apps open, any platform pair | 100% | 0.6 s | 0.6 s |
| Approximate location on the Android | 100% | 0.6 s | 0.7 s |
| Through a wall or a floor | 100% | 0.6 s | 2.5 s |
| Walking past, outdoors | 100% | 0 s | 2.2 s |
| Stale token (server restarted / revoked) | 100% | 1.6 s | 3.3 s |
| One open, one pocketed | 100% | 0.6 s | 4.4 s |
| 50 people within 20 m, all phones on (⅓ open), open viewer | 100% | 0.4 s | 3.2 s |
| same, any viewer ("if they opened the app now") | 100% | 0.6 s | 5.6 s |
| Pairs 25–30 m outdoors, one pocketed | 100% | 0.4 s | 28 s |
| Two backgrounded iPhones | 0% | – | – |
| 80 m apart outdoors (must not show) | 0 of 50 | | |

Over 5 s is always a background iPhone (iOS slows its scanning and hides its advert from
other phones) or a pocketed phone at the edge of range (−100 dBm). Never found: killed or
swiped apps, Bluetooth off, two locked iPhones with no Android near. Known wrong: pairs 30–50 m
apart outdoors are shown (GPS error 5–12 m can't separate them; Bluetooth carries further).

## Server load (`go test -bench Sightings100k`, one core, Ryzen 7 7735HS)

All proximity state is in memory under one lock; no database write per report. A report of 20
sightings costs ~21 µs at steady state (~48,000 reports/s) and ~49 µs in the worst case of
100,000 people all standing in crowds of 50 (~20,000/s, ~600 MB of pairs). 100,000 phones
reporting every 10 s is ~10,000/s. Ceilings, in order: the single lock (shard by geohash
region when one process isn't enough), deck rebuilds in dense crowds (O(partners²) per open
app, ≤ 2/s each), then sockets (~20 KB each).

## Load test on the production VPS (Nov 2026)

`backend/cmd/loadgen` against a staging copy on the 2-core / 4 GB Utho VPS (own database, 100,000
seeded accounts from `loadtest/load100k-seed.sql`, 5,000 open sockets, generator on the same box):

| Reports/s | Server p50 / p95 | New pair → card on the open app, p50 / p95 / max | Never arrived |
|---|---|---|---|
| 1,000 | 2 ms / 88 ms | 5 ms / 0.5 s / 1.5 s | 0 of 1,199 |
| 2,500 | 218 ms / 1.7 s | 5.2 s / 27 s / 32 s | 181 of 1,080 |
| 5,000 | 690 ms / 1.6 s | 14 s / 28 s / 31 s | 446 of 1,013 |

So this box holds ~1,000 reports/s (~10,000 people actively near others; phones with nobody
around send nothing). 100,000 needs ~10,000/s: an 8-core box (the hub's single lock tops out
near 20,000/s in benchmarks), then sharding by geohash region. Two fixes came out of this test:
deck rebuilds only for cards not already on the deck (`markNew`), and no access log line for
successful phone reports.

## After every deploy: smoke test on production

`backend/cmd/smoke` with two temporary accounts (session tokens in `SMOKE_A`/`SMOKE_B` env, never
the command line): A reports hearing B → both cards must reach the other's open socket within 5 s,
then B hides → gone at once. Delete the accounts after (`DELETE /api/account`). First run on
production, 7 Nov 2026 (v2.3.1): 10 ms and 12 ms, hide 0 s, PASS.

## Notifications to people near a place — postponed

Planned design (not built): phones that allow background location send a coarse fix on
significant movement; the server keeps `geohash(precision 8) → users`; "notify everyone near
café X" looks up X's cell and its neighbours and filters by distance ≤ 30 m + fix accuracy.
Limits: only reaches people who allowed background location, and indoors means "near", not an
exact 30 m.
