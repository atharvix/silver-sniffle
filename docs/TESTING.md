# Every Kinjo test

Four kinds of tests. The first three run on a computer; the last needs real phones and real people.

| Kind | What it checks | How to run | Time |
|---|---|---|---|
| 1. Server unit tests | Each rule of the server on its own | `cd server && go test ./...` | 6 s |
| 2. Real-life scenarios | 13 things real people do, against the running server | `cd server && go run ./cmd/scenarios -api http://localhost:8001` | 3 min |
| 3. Crowd test | Hundreds or thousands of phones in one room at once | `cd server && go run ./cmd/crowd -n 1000 -for 50s` | 1 min |
| 4. Real phones | Background, lock screen, Bluetooth, battery, walking | by hand, checklist below | hours |

Kinds 2 and 3 need the backend running: `powershell -File scripts\dev-up.ps1` (or `-Lan`).
Their pretend phones use the real API and socket exactly like the app (`server/internal/sim`).

## 1. Server unit tests

`go test ./...` in `server/`. Add `KINJO_TEST_DSN=postgres://kinjo@localhost:5433/kinjo_test` to also run
the database test. Also run `go vet ./...` and `staticcheck ./...`; all three must be clean before a push.

| Test | What it proves |
|---|---|
| TestDistanceKnownPair | Distance maths is right on a known pair of places |
| TestNeighbourhoodCoversMatchRadius | The map squares searched always cover 30 m + GPS error (20,000 random points, all latitudes) |
| TestMatching | 20 m apart: both see each other. 43 m with GPS error: card stays (no flicker). 47 m: gone at once |
| TestHoldExpiresAndStaleFixesGo | A card leaves 30 s after the last "near"; old positions are forgotten |
| TestRefusedFixes | Impossible places, bad accuracy and too-fast reports are ignored |
| TestBluetoothHearingMatchesIndoors | Hearing another phone's Bluetooth code counts as near |
| TestGoodOutdoorFixesVetoBluetooth | Outdoors, two good GPS fixes far apart overrule Bluetooth |
| TestHiddenUserIsGoneEverywhere | Hiding removes you from every screen and refuses your positions |
| TestStillPhoneKeepsItsSpot | A resting phone keeps its spot longer than a moving one, then it expires |
| TestStillPhoneThatAnswersKeepsItsSpot | A resting phone that answers pings never disappears while connected |
| TestDisconnectedPhoneLeaves | A short disconnect keeps the card up; gone for good means off every screen within 20 s |
| TestPersonWithoutCardIsInvisible | Nobody is shown before they've made a card |
| TestWakeQuietPhonesNearby | Only quiet phones nearby are woken, at most 1 per 20 s and 6 per 10 min |
| TestDeletedAccountSocketSeesNobody | A deleted account is gone and its open connection sees nobody |
| TestTwoServersShareState | People connected to different servers still see each other |
| TestBlockHidesBothWays | A block hides both people from each other, by GPS and by Bluetooth |
| TestNewPairsAreCountedOnce | A meeting counts as one match in the admin numbers, not one per update |
| TestStablePairsRebuildNothing | People standing together cost no card rebuilds |
| TestCrowdedRoomReportStaysCheap | In a room of 500, one report costs at most 40 Redis commands, and the deck holds the nearest 100 |
| TestProfileValidate | Card rules: name up to 50 letters, "what you do" up to 7 words, "looking for" up to 20 |
| TestCleanPhoto | Only real JPEGs are accepted, and they are re-encoded (GPS data inside photos removed) |
| TestAdminStatsAgainstPostgres | Admin numbers query the real database correctly |

App unit tests: `cd android && gradlew testDebugUnitTest lintDebug` (word counting and the card rules, the same as the server's).

## 2. Real-life scenarios

`go run ./cmd/scenarios -api http://localhost:8001`. Each scenario happens at its own place, ~1 km from the
others, and prints PASS or FAIL with the measured time. Last run: 13 of 13 passed.

| Scenario | What happens | Passes when | Last result |
|---|---|---|---|
| Two friends meet | Two people stand 20 m apart | Both see each other within 5 s | 0.1–0.3 s |
| Strangers 80 m apart | Two people 80 m apart | Neither sees the other | nobody shown |
| Someone walks past | One walks from 150 m away, past the other, to 150 m away, at walking speed | Appears within 45 m, disappears after | appears at 40 m, gone 50 m after |
| Busy café | 20 people arrive at the same moment | Everyone sees all 19 others within 10 s | within 0.2 s |
| Hide, then show | One person turns "Show my profile" off, then on | Gone from everyone within 3 s, back within 5 s | 0.25 s each way |
| Edit a card | One person changes their card | The others see the new card within 3 s | 0.23 s |
| Block | One person blocks another, in a group of three | Those two vanish for each other and stay gone; the third still sees both | as expected |
| Two people open Kinjo at the same second | Two first-time sign-ups at the same instant | Two separate accounts that see each other | as expected |
| Resting phone: signal blip, then gone | A phone on a table loses signal for 3 s, then for good | No blink during the blip; gone within 35 s after | gone in 17–21 s |
| Signal lost and back | A phone reconnects | Its cards are back within 3 s | instantly |
| Delete account | One person deletes their account | Gone from everyone within 3 s; can't connect again | 0.2 s |
| Garbage data | Broken messages, 500 m accuracy, an impossible place | Ignored; the phone stays connected and in place | as expected |
| Phone killed while walking | A walking phone dies | Gone from the other screen within 35 s | 24 s |

## 3. Crowd test

`go run ./cmd/crowd -n <phones> -for 50s`. All phones sign up, make a card, connect and report at the same
moment inside a 15 m room, then keep reporting every 2 s. It prints how long until each phone sees the
full deck (everyone, up to the nearest 100), failures, drops and data sent. Watch the server and Redis CPU
while it runs. Options: `-spread` (room size, m), `-report` (interval), `-lat`/`-lng` (place).

Last results on this PC (the phones, server and Redis all share one machine, so a real server does better):

| Phones in one room | Everyone sees the full deck | Dropped | Redis busy | Server memory |
|---|---|---|---|---|
| 100 | within 1 s | 0 | low | 53 MB |
| 300 | within 2.9 s | 0 | ~40% | 97 MB |
| 1000 | within 4 s | 0 | 43% | 185 MB |

A few sign-ups or card saves can fail at 500+ because Windows refuses that many new connections in the same
instant on one PC. The app retries sign-up and reconnects by itself.

## 4. Real phones (by hand)

Install the test APK (`dist\kinjo-test-<address>.apk`, same office Wi-Fi) or the live one once the VPS is up.
Note the phone make, model and Android version for each run; the admin page shows the phone health report.

| # | Do this | Expect | Status |
|---|---|---|---|
| 1 | Two phones, both apps open, 10 m apart | Both cards within 5 s | passed |
| 2 | Screen off and locked on one phone for 10 min | Its card stays on the other phone | 2 min passed; 10 min not done |
| 3 | Swipe the app away on one phone | Card stays (the background service keeps running) | 2 min passed on Nothing (Android 16) |
| 4 | Walk one phone 100 m away and back | Card goes after ~35–45 m, comes back within 5 s of returning | not done |
| 5 | Leave one phone on a table for 5 min, then walk off with it | Card goes within ~30 s of walking off | not done |
| 6 | Turn on airplane mode on one phone | Card gone from the other within ~25 s; back within a few seconds of turning it off | not done |
| 7 | Restart one phone and don't open the app | It comes back by itself after start-up | not done |
| 8 | Turn Bluetooth off | The app turns it back on (Android 12 and older) or asks (13+) | not done |
| 9 | Two phones indoors with poor GPS, 5 m apart | Matched by Bluetooth | not done |
| 10 | App killed by the phone overnight (needs the Firebase key on the server) | A nearby report wakes it within 7 s | not done |
| 11 | A day of normal use | Battery use acceptable; note % per hour | not done |
| 12 | Xiaomi, Oppo, Vivo, Samsung phones | Same results as on Nothing | not done |
| 13 | Edit the card, hide, block, delete on a real phone | Same as the scenarios above | not done |
