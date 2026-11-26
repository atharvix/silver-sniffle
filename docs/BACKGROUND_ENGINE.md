# Keeping phones findable in the background

A phone is findable when the server has its fresh position or Bluetooth code.
The card reaches the person by the open app, or later by a normal notification (not built yet).

## Android (built)

| Layer | What it does | Status |
|---|---|---|
| Background service | Shows "Kinjo is finding people nearby", keeps GPS + Bluetooth + the socket running. Survives a swipe on stock Android (tested on a Nothing phone, Android 16) | built |
| Moving / still | Moving: GPS every 2 s. Still 2 min: one low-power fix every 20 s; the motion sensor or Android's "started walking" signal switches back | built |
| Bluetooth | Sends a random server code, listens for others' codes, reports them every 2 s (20 s when still) | built, not yet tested between two phones |
| Restart after reboot or update | Starts the service again | built |
| Restart by the system | If Android kills the service, it restarts it | built |
| Wake-up push | The server's silent push restarts the service after a swipe | built, untested (needs the Firebase key on the server) |
| Back online | Reconnects as soon as the internet returns; clears cards after 2 min without the server | built |
| Restart when moving | Android wakes Kinjo when the person starts walking, running, cycling or driving (an official exception) | built |
| Restart when leaving | When the phone goes still, a 150 m area is set around it; leaving it wakes Kinjo (an official exception) | built |
| Phone health report | Each phone reports, at most hourly, the settings that keep Kinjo alive and why Android last stopped it; the admin numbers show it per maker | built |
| Cleaner apps | Checks a fixed list of 6 cleaner/booster apps (never all installed apps) and reports which are present | built |
| "Keep Kinjo on" card | Stays on the Nearby screen until the phone is set up, one step at a time: location "Allow all the time", battery "Unrestricted" (Android's own one-tap dialog), then on Xiaomi, Oppo, Realme, OnePlus, Vivo, Huawei and Honor their "Auto-start" screen | built |
| Watchdog | Every 5 min, even in deep sleep, an Android alarm checks the service runs and restarts it if it was killed. The alarm lives in Android, so it fires after Kinjo is killed; battery "Unrestricted" is what allows the restart | built |
| Bluetooth scan that wakes a closed app | Android can deliver scan results to a closed app | not built (needs testing first) |

Permissions asked: location (precise), "allow all the time", Bluetooth (nearby devices), physical activity, notifications,
battery "Unrestricted". Worst case after Android kills Kinjo: back within 5 min (watchdog; up to ~10 min in deep sleep), sooner when the person
moves, leaves the spot, or (with the Firebase key) someone nearby reports.
Force-stop from Android settings stops everything until the app is opened. That's Android's rule.

## iPhone (not started)

Plan: background location while the app is open or in the background ("Always" permission), Apple's
location push to wake a swiped-away iPhone (needs Apple's approval), and a region around the last spot as backup.
iOS does not let any app run after the user swipes it away, so a swiped iPhone is best effort.
Building it needs a Mac, an iPhone and an Apple developer account.
