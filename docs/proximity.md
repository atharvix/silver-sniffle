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
| `POST /api/presence` | `{lat,lng,acc,age}` — **only while the app is open** (no background GPS). Old builds still send it in the background; the server takes it the same way. |
| WebSocket `/ws` (subprotocol `["kinjo", token]`) | server → `nearby {people}`, `vis`, `notif`, `pong`, `scan`; app → `ping`, `list`, `hide`, `pos` (browser) |

Reporting: every 5 s while open, 10 s while closed; a token not heard for 8 s goes out after
300 ms (that is what puts a new card up in about a second). Wake: an FCM data push `kind=wake`
(or socket `scan`) makes the phone scan at full speed for 30 s and report.

## Server rules (`backend/prox.go`)

All in memory; nothing about who was near whom is stored.

- **Pair** (two accounts): last direct hearing (time, RSSI), last relay.
- **Direct and close** = heard directly within 30 s with RSSI ≥ −100 dBm, the radios' floor
  (or over a connection), either side hearing counting for both. Pocketed phones 20–30 m apart
  or across a wall read −95…−100; cutting them off at −95 cost 15–25 s on those pairs.
- **GPS match** (browser users, outdoors) = both fixes ≤ 2 min old, both accuracies ≤ 20 m,
  centres ≤ 30 m apart. Candidates come from a geohash index (precision 7, the cell plus its 8
  neighbours).
- **Outdoor veto** — when both have fixes ≤ 60 s old with accuracy ≤ 15 m (open sky), fixes
  further apart than 35 m + their mean error overrule Bluetooth (it carries 80 m+ outdoors).
  Indoor fixes are vaguer (and sometimes 40 m off while claiming 10 m) and never veto, so
  same-room pairs never blink because of GPS. (A −85 dBm outdoor floor was tried: it made
  walking past someone take 17 s. Removed.)
- **Card shown** while direct-and-close or GPS match, not vetoed; **held** until 60 s pass
  with neither (stops blinking when a few adverts are missed).
- **Relay alone never shows a card** (the relaying phone could sit between two people 60 m
  apart): it sends both phones a `scan` (socket) / `wake` (push) so they check directly.
- **Delivery**: a deck change marks the viewer dirty; decks flush every 500 ms, a new pair at
  once. Cards are `{uid,name,role,desc,img}`, max 50, closest first (RSSI, then GPS distance),
  photos by URL only.
- **Wake pushes**: a phone that reported and went quiet gets a silent push at 75 s, then every
  10 min for 12 h.
- **Hide / logout / delete** (`Hub.leave`): drops the account's pairs, fix and tokens at once
  and refreshes everyone's deck; reports in flight from before are dropped (hide guard).

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
| Crowd of 50 within 20 m (realistic mix) | 74% | 0.6 s | 15 s |
| Pairs 25–30 m outdoors, one pocketed | 100% | 0.4 s | 28 s |
| Two backgrounded iPhones | 0% | – | – |
| 80 m apart outdoors (must not show) | 0 of 50 | | |

Over 5 s is always a background iPhone (iOS slows its scanning and hides its advert from
other phones) or a pocketed phone at the edge of range (−100 dBm). Never found: killed or
swiped apps, Bluetooth off, two locked iPhones with no Android near. Known wrong: pairs 30–50 m
apart outdoors are shown (GPS error 5–12 m can't separate them; Bluetooth carries further).

## Notifications to people near a place — postponed

Planned design (not built): phones that allow background location send a coarse fix on
significant movement; the server keeps `geohash(precision 8) → users`; "notify everyone near
café X" looks up X's cell and its neighbours and filters by distance ≤ 30 m + fix accuracy.
Limits: only reaches people who allowed background location, and indoors means "near", not an
exact 30 m.
