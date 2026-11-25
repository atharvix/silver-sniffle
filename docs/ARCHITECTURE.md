# How Kinjo works

## The goal

Two people with Kinjo within 30 m see each other's card within about 5 seconds: app open, in the background,
screen locked, swiped away (best effort), or moving.

## The pieces

```
Android app â”€â”€ WebSocket + HTTPS â”€â”€> nginx (kinjo.world) â”€â”€> Kinjo server (Go, port 8001)
  GPS, Bluetooth,                                              â”‚
  background service                                           â”œâ”€ PostgreSQL: accounts, cards, push tokens
                                                               â”œâ”€ Redis: live positions and who is near whom
                                                               â”œâ”€ NATS JetStream: queue of wake-ups
                                                               â””â”€ Cloudflare R2: photos
Kinjo server â”€â”€ wake-up push â”€â”€> Firebase Cloud Messaging â”€â”€> Android app
```

| Piece | What we use | Why |
|---|---|---|
| Phone app | Kotlin + Jetpack Compose (Android). iPhone: Swift, not started | Background location and Bluetooth need native code |
| Server | Go, one binary | Fast with many open sockets, simple to deploy |
| Accounts and cards | PostgreSQL | The only data kept long term |
| Live state | Redis, nothing saved to disk | Positions change every few seconds and must never be stored |
| Wake-up queue | NATS JetStream | Wake-ups survive a restart and are retried |
| Photos | Cloudflare R2 (S3 API) | Cheap storage, free downloads, cached worldwide |
| Wake-up pushes | Firebase Cloud Messaging | Only Android's push service can wake a closed app |

More servers can run side by side behind nginx when one isn't enough: they share Redis and NATS.

## How two people find each other

1. Each phone sends its position every 2 s while moving, every 20 s while resting (a cheap fix, plus a ping every 30 s).
2. The server puts it in a small map square (about 111 m) and looks at the squares around it.
3. If two people are within 30 m (plus up to 40 m for GPS error), both get each other's card.
4. Indoors GPS is poor, so phones also send a random Bluetooth code that changes every 15 minutes.
   Hearing another phone's code counts as "near". Outdoors, two good GPS fixes far apart overrule Bluetooth.
5. A card goes when the person is past 35 m (plus GPS error), or 30 s after the last "near" (so it doesn't flicker).
6. A phone that loses its connection is taken off the map 20 s later (a quick reconnect keeps the card up,
   and on reconnecting the phone resends its position at once).

## Crowds (hundreds or thousands in one room)

The first app crashed with 7 people. This one is built so that one report costs about the same in a full
stadium as in an empty cafÃ©:

| What | How | Why it keeps the load low |
|---|---|---|
| Positions | Each map square is one Redis record holding everyone in it | Reading a whole room is ~15 reads, not one per person |
| "Near" pairs | Saved only when new, or when 10 s old (cards stay 30 s) | People standing together cost almost nothing |
| Far-away people | Removed only if they were actually paired | Busy streets nearby cost nothing |
| Limit per person | Keeps the nearest 200 pairs fresh; the screen shows the nearest 100 | The work has a ceiling, however big the crowd |
| Card updates | Changed cards are collected and sent every 250 ms, 16 rebuilt at a time | 500 people arriving together = one update each, not 500 |
| Wake-ups | Only phones quiet for over a minute, each area checked at most once per 20 s, shared by everyone there | A room of 1000 checks as rarely as one person |
| Liveness | The server pings each phone every 30 s; a resting phone that answers keeps its spot | A phone on a table never disappears while connected |
| Redis connections | 16 kept open, at most 64 per server | A crowd opening the app at once waits a few ms instead of being refused |

Nothing is traded away: the 30 m rule, the 5 s promise, hiding, blocking and deleting all work the same
(see "Real-life scenarios" in `PROOF_PLAN.md`). An edited card reaches everyone near within about 0.3 s.

## Phones that stopped reporting (app swiped away)

The server remembers each phone's rough area (a 550 m square) for 12 hours, in Redis only.
When someone reports nearby, phones in that area that went quiet get a silent push.
The push restarts the app's background service, which sends one position. Limits: one per 20 s, six per 10 min.
Details: `BACKGROUND_ENGINE.md`.

## What is stored

| Kept | Where | How long |
|---|---|---|
| Random account id, card (name, what you do, what you're looking for), photo key | PostgreSQL | until deleted |
| Photo (re-encoded, so GPS inside the photo is removed) | R2 | until replaced or deleted |
| Session key (only its fingerprint), push token, hidden yes/no | PostgreSQL | until deleted |
| Exact position | Redis | 1 min (2 min if resting); removed 20 s after the phone disconnects |
| Who is near whom | Redis | 90 s |
| Rough area (550 m square) | Redis | 12 hours |
| Bluetooth code | Redis | 30 min |

| Blocks and reports | PostgreSQL | until the account is deleted |
| "I'm 18+ and accept the terms" time | PostgreSQL | until deleted |
| Phone health: maker, model, Android version, battery/background settings, why Android last stopped Kinjo, cleaner apps from a fixed list | PostgreSQL | latest per install |
| Days the app was opened | PostgreSQL | until deleted |
| Daily totals (sign-ups, matches, wake-ups, reportsâ€¦) | PostgreSQL | kept, counts only |

Never stored anywhere: where someone was, who met whom, distances. Accounts are anonymous for now (no sign-in yet).

## Admin numbers

`GET /admin/stats` (password `ADMIN_TOKEN`, not public yet) returns what the admin dashboard will show:
daily sign-ups, active people, new matches (GPS and Bluetooth), wake-ups, cards saved, blocks, reports, hides and
deletions; people active in 7 and 30 days; phones connected and findable right now; and phone health per maker
(% battery unrestricted, % restricted by Android, % location "all the time", % Bluetooth on, why Android stopped Kinjo,
cleaner apps found). Totals and percentages only, never a person or a place.

## Running costs

| Item | Cost |
|---|---|
| VPS (Utho, already paid) | what you pay now |
| PostgreSQL, Redis, NATS, nginx | free (on the VPS) |
| Cloudflare R2 photos | free up to 10 GB, 1 million uploads and 10 million views a month; then about $0.015 per GB a month (a photo is ~300 KB, so 1 million users â‰ˆ 300 GB â‰ˆ $4.50/month), views stay free |
| Firebase push | free |
| Google Play account | $25 once (already have one) |
| Apple developer account (iPhone app) | $99 a year |
| Domain kinjo.world | already have it |

Prices are the providers' published prices; check them before relying on them.

## Built differently from the first plan

| First plan | Built | Why |
|---|---|---|
| H3 map cells | simple 0.001Â° squares | H3 needs C code, which breaks the one-file server; the squares are tested to cover 30 m + error everywhere |
| NATS or Kafka | NATS only | Kafka adds nothing at this size |
| Load balancer with many servers | one server for now | the code already supports more; add them when needed |
