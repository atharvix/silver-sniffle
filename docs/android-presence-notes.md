# What Uber and Rapido do (Android)

Looked at with REA (`inspect-android-package`) on their APKs from our test phone (Nothing A001T, Android 16), 25 Nov 2026.
This reads their manifests and two decompiled methods only, not how they behave at runtime.

| | Uber | Rapido (rider app) |
|---|---|---|
| Always-on location service | no | no |
| Background location service | yes, through Android's WorkManager | no |
| Geofences (wake when leaving an area) | yes | yes (CleverTap, only with "allow all the time") |
| Restart after reboot | yes | yes |
| Bluetooth | declared | no |

Takeaway: neither keeps tracking when idle. They get woken by the system or a push, then run for a short time.
Our app goes further: a background service that stays on, plus wake-up pushes when it is killed.
