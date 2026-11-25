# Android presence: what Uber and Rapido actually do

Source: REA `inspect-android-package` (static manifest only) on the APKs pulled from our own test phone
(Nothing A001T, Android 16): Uber `com.ubercab`, Rapido rider `com.rapido.passenger`.
Raw evidence: `%USERPROFILE%\dev\rea-out\*-manifest.json`. Code was NOT decompiled yet, so these are
declarations, not proof of behaviour.

## Findings

| | Uber | Rapido (rider) |
|---|---|---|
| Own always-on location service | none | none |
| Foreground service of type `location` | WorkManager's `SystemForegroundService` (`microphone\|location\|dataSync`) | none (only `dataSync`, `microphone`) |
| OS geofences (Play Services) | `GeofenceBroadcastReceiverV3/V4` + JobIntentService | CleverTap `CTGeofenceReceiver`, `CTLocationUpdateReceiver` |
| Boot restart | `RECEIVE_BOOT_COMPLETED`, NTP boot receiver | `CTGeofenceBootReceiver` re-arms geofences |
| Bluetooth | SCAN / CONNECT / ADVERTISE declared | none |
| Push wake | own `PushReceiver` (FCM) | `FirebaseInstanceIdReceiver`, CleverTap |
| Exact alarms | `SCHEDULE_EXACT_ALARM` | no |

Phone check (`adb dumpsys`, idle): neither app held a running foreground service.

## What this means for us

1. Neither app tracks you continuously while idle. They get woken by the OS or a push, then run a short foreground job. There is no Uber trick for "always on while swiped away".
2. Mechanisms that survive a swipe on Android: **Play Services geofences** (delivered by the OS, re-armed on boot), **high-priority FCM**, **WorkManager foreground work** (location type), boot receiver.
3. Our design for step 10: a `location`-type foreground service while the user is visible, plus a geofence ring around the last position that restarts it when they leave, FCM high-priority wake from the server, and a boot + 15-min WorkManager revive. Add Bluetooth advertise/scan inside the service for the 30 m signal.

## Not yet verified

- Runtime behaviour (what restarts what, timings): needs the decompiled code or a Frida trace.
- Whether Android 16 on this phone kills each of these after a swipe: must be tested with our own build.
- Native `.so` code was not inspected (would need Ghidra).
