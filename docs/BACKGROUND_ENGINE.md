# Background engine (software only)

Goal: keep every phone **findable** (fresh position or BLE token known to the server) and get the card **shown** (open app, or visible push), in every state, on both OSes.
Everything below uses public OS APIs only. Items marked *verify* are not confirmed by us yet; `PROOF_PLAN.md` has the test for each.

## 1. Shared state machine (both phones)

| State | Meaning | Position source | Report | Radio |
|---|---|---|---|---|
| MOVING | user moving (activity or speed > 0.7 m/s) | GPS best, 1-2 s | every fix, socket | BLE adv fast + scan fast |
| STILL | stationary >= 2 min | coarse/network, or none | one `still` fix, then heartbeat 5 min | BLE adv slow + scan low-power |
| ARMED | app swiped / service gone | none | woken by a trigger, 1 fix | none (OS-held triggers only) |
| HIDDEN | user hid profile | none | none | none |

Transitions: MOVING -> STILL after 2 min without motion; STILL -> MOVING on motion event (accelerometer/activity); any -> ARMED when the process dies; ARMED -> MOVING/STILL on any trigger below.
Every start reports `why_started` (`user`, `boot`, `sticky`, `fcm`, `geofence`, `activity`, `ble`, `location_push`, `region`) so we measure which layer really works.

## 2. Android

**Permissions:** `ACCESS_FINE_LOCATION`, `ACCESS_COARSE_LOCATION`, `ACCESS_BACKGROUND_LOCATION`, `FOREGROUND_SERVICE`, `FOREGROUND_SERVICE_LOCATION`, `FOREGROUND_SERVICE_CONNECTED_DEVICE`, `BLUETOOTH_SCAN`, `BLUETOOTH_ADVERTISE`, `BLUETOOTH_CONNECT`, `ACTIVITY_RECOGNITION`, `POST_NOTIFICATIONS`, `RECEIVE_BOOT_COMPLETED`, `WAKE_LOCK`, `INTERNET`. Ask in this order: notifications -> location while-in-use -> background ("all the time", second prompt with a plain explanation screen first, needed for Play) -> Bluetooth -> activity -> battery "Unrestricted" (settings screen, not the Play-restricted exemption dialog).

**Service:** one foreground service, `foregroundServiceType="location|connectedDevice"`, `stopWithTask="false"`, `START_STICKY`, ongoing low-importance notification ("Kinjo is finding people nearby", action: Hide me). Partial wake lock only during a report burst, never held.

**Location (Fused provider):**
- MOVING: `PRIORITY_HIGH_ACCURACY`, interval 2000 ms, min interval 1000 ms, min distance 3 m.
- STILL: stop GPS. Register `Sensor.TYPE_SIGNIFICANT_MOTION` trigger (hardware, near-zero power) + Activity Recognition transitions (STILL/WALKING/IN_VEHICLE). Either one -> MOVING.
- Reject mock fixes (`isMock`) flag, send flag to server.

**Socket:** one WebSocket, ping 25 s, reconnect with jitter. Foreground service keeps the network alive in Doze.

**BLE:**
- Advertise: service UUID = Kinjo UUID + service data = 8-byte rotating token (server issues, 15 min). MOVING `ADVERTISE_MODE_LOW_LATENCY`, STILL `LOW_POWER`, TX power medium.
- Scan (service running): `ScanFilter` on the Kinjo UUID, `SCAN_MODE_LOW_LATENCY` moving / `LOW_POWER` still. Report `{token, rssi}` batches every 2 s.
- Scan (armed): `startScan(filters, settings, PendingIntent)` to a **manifest-declared** receiver, `PendingIntent.FLAG_MUTABLE`, explicit intent, `CALLBACK_TYPE_ALL_MATCHES`, `MATCH_MODE_AGGRESSIVE`. The receiver uses `goAsync()`, POSTs the heard tokens to `/ble`, and finishes. *verify (A11): survives a swipe; Android 15+ force-stop cancels it.*
- Android cannot read an iPhone's backgrounded advert (Apple puts the UUID in an overflow area). *verify (A9/I6).* So Android <-> iPhone is confirmed by BLE only when the iPhone app is in the foreground; otherwise GPS decides.

**Restart chain (software-only watchdog), in order:**
1. `START_STICKY`: the OS restarts the service.
2. `onTaskRemoved()` (fires on swipe): schedule a one-shot WorkManager job + `AlarmManager.setAndAllowWhileIdle` restart 5 s later; log it.
3. Geofence ring (150 m, EXIT, Play Services) + Activity Recognition transitions: official exemptions to start the service.
4. BLE PendingIntent scan receiver.
5. Server FCM `wake` (`priority: HIGH`, `ttl: 30s`, data-only): official exemption.
6. Boot / `MY_PACKAGE_REPLACED` receiver; WorkManager 15-min periodic check.
7. On every start read `ApplicationExitInfo` (why we died: user-requested, low memory, OEM kill) and send it to the server.

**OEM killers:** detect maker (Xiaomi, Oppo, Realme, Vivo, OnePlus, Huawei/Honor, Samsung). Show one card with the exact switch: Autostart, "No restrictions" battery, "Lock in recents", Samsung "Never sleeping apps". Open that maker's settings page by intent; fall back to the app info page. Mark done only after the user returns.

## 3. iPhone

**Info.plist:** `UIBackgroundModes` = `location`, `bluetooth-central`, `bluetooth-peripheral`, `remote-notification`. Usage strings: `NSLocationWhenInUseUsageDescription`, `NSLocationAlwaysAndWhenInUseUsageDescription`, `NSBluetoothAlwaysUsageDescription`, `NSMotionUsageDescription`.
**Entitlements:** `com.apple.developer.location.push` (app + extension), App Group + Keychain access group shared by app and extension.
**Authorization flow:** When-In-Use -> (after first value) Always. Check `backgroundRefreshStatus`; if denied, show the exact Settings path (it blocks every relaunch).

**Location (Core Location, `CLServiceSession` / `CLLocationUpdate`):**
- MOVING: `desiredAccuracy = kCLLocationAccuracyBest`, `distanceFilter = 5`, `activityType = .other`.
- STILL: keep updates running (this keeps the process alive in the background) but drop to `kCLLocationAccuracyThreeKilometers`, `distanceFilter = 100`; Core Motion (`CMMotionActivityManager`) raises accuracy when it sees walking/automotive.
- Always: `allowsBackgroundLocationUpdates = true`, `pausesLocationUpdatesAutomatically = false`, `showsBackgroundLocationIndicator = true`.

**When the app is swiped (ARMED):** nothing runs. These triggers are armed beforehand:
1. **Location push extension** (main path): `startMonitoringLocationPushes` gives a token -> send to server. Server sends APNs: HTTP/2, **token-based auth only**, headers `apns-push-type: location`, `apns-topic: <bundle id>.location-query`, `apns-priority: 10` (forum reports: priority 5 breaks it). Extension class implements `CLLocationPushServiceExtension.didReceiveLocationPushPayload`: get one fix (`requestLocation`), read the session token from the **shared keychain** (`kSecAttrAccessibleAfterFirstUnlock`, because the extension runs while locked), POST `/fix`, call the completion block fast, and finish in `serviceExtensionWillTerminate`. *verify (I4): wake after a swipe, rate limits.*
2. Region ring (150 m, exit) around the last still fix + significant-change + `CLVisit`: backstops. *verify (I2/I3): conflicting reports about relaunch after a swipe.*
3. Normal **visible push** carries the card to the person (this needs no code from us).

**BLE (CoreBluetooth):** `CBCentralManager` and `CBPeripheralManager` with fixed restore identifiers, scan for the Kinjo UUID, advertise the UUID; when found, connect and read the token characteristic (iPhone <-> iPhone). Background adverts are UUID-only. Restoration works after a system kill, not after a swipe.

## 4. Server side of the engine

- `/fix` and `/ble` accept the `why_started` reason, activity, accuracy, `still`/`mock` flags.
- Wake payloads: FCM data message `{type:"wake", id}` priority HIGH ttl 30 s; APNs location push (no payload needed). One lock per target (`wl:<uid>` 20 s), budget 6 per 10 min, heartbeat every 5-10 min per ARMED phone.
- Server infers a swipe/kill from a socket closing without a `bye`, and uses the phone's last state to size the wake radius (`30 m + v * age`).

## 5. What the engine measures about itself

Per phone and per day: `why_started` counts, time spent in each state, gaps between fixes, wake sent / answered / seconds to answer, `ApplicationExitInfo` reasons (Android), battery % per hour per state. These numbers decide what to tune, and prove or kill each layer.

## 6. Known limits (software only)

- A swiped iPhone that was **moving** stays unfindable until a trigger fires; wake radius + heartbeat shrink the gap but do not remove it.
- Android force-stop (Settings) ends every layer until the app is opened.
- Android <-> backgrounded iPhone BLE detection is weak; GPS carries those pairs.
- Apple's location-push rate limit and the extension's time limit are unmeasured.
