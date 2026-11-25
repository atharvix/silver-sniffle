# Test plan

Every claim gets tested on real phones before we rely on it. Status as of 25 Nov 2026.

## Server

| Test | Status |
|---|---|
| Matching: 30 m rule, no flicker at the edge, cards expire | passing (automated) |
| Map squares always cover 30 m + GPS error | passing (automated, 20,000 random points) |
| Bluetooth match indoors; good GPS overrules it outdoors | passing (automated) |
| Hide: gone for everyone, positions refused | passing (automated) |
| Deleted account: refused at once on every server | passing (automated) |
| Wake-ups: only quiet phones nearby, at most 1 per 20 s and 6 per 10 min | passing (automated + real run) |
| Two servers sharing Redis: people on different servers see each other | passing (automated) |
| Real run: Postgres + Redis + NATS + photo storage, two test people 10 m apart see each other instantly | passing (on this PC) |
| Many people at once (load test) | not done |

## Android (on a real phone)

| Test | Status |
|---|---|
| Service survives screen off, lock and swipe | passed for 2 min on Nothing (Android 16); longer run not done |
| Card shows up while the app is open | passed (phone + test person on this PC) |
| Wake-up push restarts a swiped app within 7 s | not done (needs the Firebase key on the server) |
| Bluetooth between two real phones | not done |
| Battery use over a day, moving and still | not done |
| Phones from makers that kill apps (Xiaomi, Oppo, Vivo, Samsung) | not done |
| Real 30 m walking tests with two phones | not done (needs the server on the VPS) |

## iPhone

Not started (needs a Mac, an iPhone and an Apple developer account).
