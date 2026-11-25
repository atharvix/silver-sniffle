# Kinjo v2: proof plan

Nothing in `ARCHITECTURE.md` counts until it passes here, on real phones. Every test has a number to hit.
Status words: **Verified** (seen in code or docs), **Research** (sourced, not tested by us), **Unproven**.

## Requirement (the only thing that matters)

Two people with Kinjo within 30 m see each other's profile in <= 5 s (cold start after swipe <= 7 s), on Android and iPhone, in every state: app open, backgrounded, screen off and locked, **swiped away**, and while moving.

**Assumption (founder, 2026-11-25):** Apple's location-push entitlement and Google Play's background-location approval are granted. Gates B and C below become "prove it works", not blockers. Kernel-level approaches are out: neither OS lets a shipped app run kernel code.

"Seeing" splits in two, and each half has a different mechanism:
1. **Findable:** the server knows the phone's fresh position. This is the hard part when the app is swiped away.
2. **Shown:** the person gets the card. Open app = card on screen. Locked or swiped = a **normal visible push notification carrying the card** (name + photo). A visible push is delivered to a swiped app on both OSes (not after an Android force-stop), so the "shown" half does not need our code to run.

| State | Android | iPhone |
|---|---|---|
| Open | OK | OK |
| Background, screen off, locked | OK (foreground service) | OK ("Always" background location) |
| Moving | OK (service; activity transitions restart it) | OK while not swiped |
| **Swiped away** | Likely OK: service often survives, backed by BLE-scan wake, FCM wake, activity/geofence, boot. **Unproven until A3, A11, A4** | **Not guaranteed.** Best chance: location push (Apple entitlement) + beacon wake from Android phones. Backstops are minutes late |

## 0. What the research changed

| Claim in the design | Finding | Status |
|---|---|---|
| Android geofence ring wakes a swiped phone fast | Android docs: geofence responsiveness is "every couple of minutes" on average, and Doze adds more. **Too slow for 7 s.** Geofence only keeps the last position fresh and acts as a backstop. | Research |
| Android wake = high-priority FCM | Allowed to start a foreground service from the background, but a `location` service needs a location permission, and while-in-use rules block it from the background unless the app holds `ACCESS_BACKGROUND_LOCATION`. Android 14+ throws `SecurityException` otherwise. | Research |
| Play will accept background location | Needs a declaration form, an in-app prominent disclosure and a demo video. "Core functionality" is judged by Google. No guarantee. | Research |
| iOS significant-change relaunches a swiped app | Apple forum: inconsistent after force-quit; one report says it **stopped relaunching on iOS 26**. Region monitoring is the dependable relaunch, but with delay and thresholds. | Research |
| iOS location push wakes a swiped app in seconds | Works only with Apple's entitlement, given for specific uses (location sharing). Requests often wait long; denial is possible. "Always" is mandatory. | Research |
| Uber/Rapido run a permanent location service | No. Uber: location foreground job via WorkManager + geofence receivers, **no** `ACCESS_BACKGROUND_LOCATION` declared. Rapido: no location service. Rapido's CleverTap boot receiver re-arms geofences only if `ACCESS_BACKGROUND_LOCATION` is granted (decompiled: `CTGeofenceBootReceiver.onReceive`). | **Verified** |

| Android official exemptions that let a swiped app start a foreground service (fetched from developer.android.com, quoted) | high-priority FCM; **geofence or activity-recognition transition event**; boot / package-replaced; Companion Device Manager permission; **user turns off battery optimization for the app**; exact alarm for a user-requested action. A BLE-scan PendingIntent is **not** on the list. | **Verified (docs)** |
| Android location service from the background | Page: a service needing a while-in-use permission can't be created in the background even under an exemption; a `location`-type service started by an app holding `ACCESS_BACKGROUND_LOCATION` can read location all the time. Whether FCM + background permission works on Android 16 is the thing A4 proves. | Research, A4 |
| Android BLE scan with PendingIntent survives process death | Google's own sample and Nordic's library use it; sources say the system keeps scanning and delivers to a manifest receiver. **Swipe-away not confirmed by any source.** | Research, A11 |
| iOS iBeacon region monitoring relaunches a force-quit app | Apple DTS says region detection works when force-closed if beacons follow the iBeacon spec; one vendor claims ~1 s. Needs "Always" and Background App Refresh on; 20-region cap; relaunches throttled to about one per 3-5 min (forum). **No source on an Android phone advertising iBeacon to wake an iPhone.** | Research, I9 |
| iOS 26 Wi-Fi Aware talking to Android | Exists, but forum reports show discovery/pairing failures and nothing about background use. | Parked (R&D only) |
| Life360 / Happn / Zenly background tricks | Searches found no reliable detail. Not relied on. | Unknown |

| **Conflict:** does iOS relaunch a *user-swiped* app for region/beacon/CLMonitor events? | Older answers and a DTS quote say yes. Newer forum threads (CLMonitor, beacon regions, iOS 26) say no: "the normal behavior of iOS is that it does not relaunch applications that have been force quit". Only a device test settles it. | Conflicting, I2/I9 |
| Location-push entitlement process | Described by Apple as for "location sharing apps". An Apple DTS reply (Aug 2026) says the portal currently has **no option to submit the request**. Whether "discover nearby strangers" is an accepted use is **not stated anywhere I could read**. | Unknown, may be unobtainable |
| iOS Shortcuts personal automations (Arrive/Leave, Wi-Fi, Bluetooth) can run when locked | Apple docs list them as able to run automatically ("Allow running when locked"). Behaviour after a swipe and location access from the intent: not found. Needs each user to build the automation by hand. | Backstop idea only |
| Nearby Interaction (UWB) in background | Apple documents background ranging with UWB *accessories*; iPhone-to-iPhone and relaunch of a terminated app: nothing found. Several unresolved bugs. | Not usable |
| Android 15+ force-stop cancels PendingIntent scans; Android 17 changes BLE behaviour | One blog says so; AOSP source not confirmed. Our test phone is Android 16. | Check in A11 on 16, then on 17 |
| Companion Device Manager | Official wake path, but needs a user-approved association per BLE device. Not workable for strangers' phones. | Rejected |

Honest consequence: **"visible in <= 7 s after a swipe" is only realistic through server-driven wake on demand**
(FCM high-priority, APNs location push). Geofences and significant-change are backstops, not the fast path.
If Apple denies the entitlement, iPhone swipe-away becomes best-effort and the product promise for iPhone must change.

## 1. Day-one actions (long lead times)

1. Apple Developer account, then **request the location-push entitlement** with a truthful description ("show nearby users who opted in"). Start now: replies take weeks and may be a no.
2. Play Console account, internal testing track, prepare the background-location declaration + demo video script.
3. Get a Mac (or cloud Mac) and an iPhone. Windows cannot build iOS.
4. A second Android phone if possible. Without one, a server-side simulated peer covers most tests.

## 2. Measurement harness (built first, used by every test)

- Server `GET /time` so every phone logs events in server time (offset measured, skew < 50 ms).
- Every phone logs: `fix_sent`, `wake_received`, `service_started`, `card_shown`, with server time.
- Server logs `first_match{via, after_ms}` (latency from the later phone's first fix to the card).
- A **simulated peer** (a script holding a WebSocket) can report a fix next to a real phone on demand, so one real phone proves wake-on-demand alone.
- Android: `adb logcat` + `dumpsys` captured per run. iPhone: Xcode device logs / signposts.
- Every run records: phone, OS version, battery %, power mode, Wi-Fi/cell, screen state, minutes since last app use.

## 3. Server proofs (no phone needed)

| ID | Test | Pass |
|---|---|---|
| S1 | Matching unit tests: ring-1 coverage, distance + error rule, hysteresis, blocks, hidden | all pass, ring covers 30 m + 40 m error everywhere on a 10k-point grid |
| S2 | Simulator: 5k then 50k fake phones moving in a city | `first_match` p95 < 1 s server-side, Redis p99 < 5 ms |
| S3 | Wake-on-demand logic: stale target in ring gets exactly one wake, locks and budgets hold | no duplicate wakes within 20 s, <= 6 per 10 min |
| S4 | Gateway kill mid-run | all sockets back within 15 s, no thundering herd (reconnect spread over >= 10 s) |
| S5 | Redis shard failover | recovery < 30 s, no data left on disk afterwards (`redis` persistence off, scan dumps for coordinates) |
| S6 | Privacy audit: grep logs, DB, backups for lat/lng | zero hits |

## 4. Android proofs (Nothing A001T, Android 16, then a Samsung/Xiaomi/Oppo for OEM)

| ID | Test | Pass |
|---|---|---|
| A1 | Foreground service (`location`) survives screen off + lock for 1 h, 8 h overnight (Doze) | still reporting; max gap < 10 s awake, < 60 s in Doze |
| A2 | Same, while walking/driving | fixes every 2-4 s, no gap > 10 s |
| A3 | Swipe from recents: does the service live? | record yes/no per OEM (decides how much depends on wake) |
| A4 | **FCM high-priority wake -> service start -> fix reported**, app swiped away, screen off, idle 30 min | wake-to-fix p50 <= 4 s, p95 <= 7 s over 30 runs; zero `SecurityException` / start-denied |
| A5 | Same as A4 with only "While using" granted | expected to **fail**; proves we need `ACCESS_BACKGROUND_LOCATION` |
| A6 | Geofence ring exit while swiped | record real delay over 20 runs (expect minutes). Proves it is only a backstop |
| A7 | Reboot: boot receiver re-arms ring + WorkManager revive | service back <= 2 min after boot unlock |
| A8 | Battery: 24 h with the app visible-state vs armed-state | visible < 8 %/h worst case, armed < 1 %/h; if worse, lower fix rate |
| A9 | BLE adv + scan inside the service at 30 m, indoors and outdoors | detect within 10 s at <= 20 m; record false positives through a wall |
| A10 | Force-stop from Settings | confirm nothing runs until the app is opened (expected) |
| A11 | **BLE scan with PendingIntent** (filter = Kinjo service UUID), app swiped, screen off, a second device advertising the UUID | receiver fires <= 5 s after the advert is in range; receiver sends the heard token to the server over HTTPS with **no GPS**; repeat 30 runs and after 1 h idle. If the system keeps scanning, Android needs no server wake for pairs where one phone advertises |
| A12 | Can the BLE receiver start the foreground service (not on the exemption list)? | record yes/no. If no, the receiver just reports and finishes (that is enough) |
| A13 | Activity-recognition transition (STILL -> WALKING/IN_VEHICLE) while swiped | service starts within 10 s of the transition; this is an exempt start path |
| A14 | User sets battery to "Unrestricted" / ignores optimization: can WorkManager or an alarm start the service while swiped? | yes/no; if yes, onboarding asks for it and OEM killers are mostly defeated |
| A15 | Does the foreground service survive a swipe with `stopWithTask=false` (A3) on Nothing, Samsung, Xiaomi, Oppo? | per-OEM table. Where it survives, the other layers are only backups |

## 5. iPhone proofs (needs Mac + iPhone; do these **before any iOS screens**)

| ID | Test | Pass |
|---|---|---|
| I1 | Background location, "Always", locked, 1 h and overnight | fixes continue, gaps < 10 s moving, blue indicator shown |
| I2 | Swipe the app away; region ring (100/200 m) exit by walking | record relaunch delay over 20 runs; pass = relaunch happens at all, report distribution |
| I3 | Swipe away; significant-change | record whether it relaunches on the current iOS (known unreliable, possibly broken on iOS 26) |
| I4 | **Location push** (needs entitlement) with the app swiped away, 30 min idle | wake-to-fix p50 <= 5 s, p95 <= 8 s over 30 runs |
| I5 | Silent push to a swiped app | expected to **not** run; proves the claim |
| I6 | Background BLE: iPhone advertising to Android scanner and to iPhone | record real detection; expected weak, drives "BLE only when open" |
| I7 | Battery: 24 h in visible and armed states | visible < 6 %/h, armed < 1 %/h |
| I8 | Low Power Mode, Background App Refresh off, Location "While Using" only | record what breaks; drives onboarding copy |
| I9 | **iBeacon region wake:** iPhone monitors a Kinjo beacon UUID, app swiped; a visible Android phone advertises iBeacon format | relaunch within 10 s of entering range, 20 runs; then the app reports one fix. Also test a cheap venue beacon. If this works, Android -> swiped iPhone is solved without Apple's entitlement |
| I10 | Can an **iPhone** advertise as iBeacon in the background? (expected no) | record. Decides whether iPhone <-> iPhone swiped pairs need the location push |
| I11 | `CLVisit` + region ring with Background App Refresh on, then off | record delays; app must detect the setting off (`backgroundRefreshStatus`) and tell the user |
| I12 | Server-driven regions: server tells a phone "monitor a circle around where a peer was last seen" (within the 20-region cap) | relaunch on entering that circle; record delay |

If **I4 cannot be run** (entitlement denied or pending), iPhone is tested and shipped as: visible instantly while the app is open or backgrounded; swiped = relaunch by region exit only (minutes). Say so in the product copy.

## 6. End-to-end proofs

| ID | Scenario | Pass |
|---|---|---|
| E1 | Two phones, both open, 30 m apart | card on both in <= 5 s, p95 over 30 runs |
| E2 | One locked, one open | <= 5 s |
| E3 | One swiped away (idle 30 min), other opens near it | <= 7 s via wake on demand, report p50/p95 and **failure rate** |
| E4 | Both swiped away, then one opens | <= 7 s |
| E5 | One moving (walk, bike, car at 30 km/h), other still | card appears before the pair is > 60 m apart |
| E6 | Pair at 35 m and 45 m | no card (distance rule holds, no flicker over 5 min) |
| E7 | Hidden / blocked user | never shown, wake never sent |
| E8 | 100 simulated peers around one real phone | deck correct, no UI stall, battery impact recorded |

## 7. Gates and fallbacks

- **Gate A (after A4, A5):** high-priority FCM -> background-started location service works with `ACCESS_BACKGROUND_LOCATION`. If it fails on stock Android 16, stop and redesign Android before any server polish.
- **Gate B (after I2-I4):** if no iPhone path wakes a swiped app within 60 s, the iPhone promise is "instant while the app is open or backgrounded". Decide with the founder before building iOS screens.
- **Gate C (before launch):** Play background-location declaration and App Review pass. Without Play approval, Android falls back to foreground-only location (visible while the app is open).
- Any test that misses its number is recorded, the design is changed on paper first, then the code.

## 8. Order of work

1. Harness + S1-S3 (server side, no hardware).
2. Android project, then A1, A4, A5 (this decides Gate A). Then A2, A3, A6-A10.
3. In parallel: entitlement request, Mac setup.
4. iOS project, I1-I8.
5. E1-E8 with two real devices, then the OEM matrix.

## 9. Not yet verified by us

Everything marked Research or Unproven above. The REA findings on Uber/Rapido come from their manifests and two decompiled methods, not a runtime trace, and do not tell us how well their approaches perform.
