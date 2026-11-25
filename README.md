# Kinjo

See the people around you (within 30 m) as cards, even when the app is in the background.

| Folder | What |
|---|---|
| `android/` | The Android app (Kotlin, Jetpack Compose) |
| `server/` | The backend (Go) and its deploy kit (`server/deploy/`) |
| `docs/` | How it works, the background plan, the test plan |
| `scripts/dev-up.ps1` | Runs the whole backend on this PC for phone testing |

The previous project, including the old web app whose design the Android app copies, is saved at git tag `archive/pre-rebuild`.

## Run it on this PC and test with phones

Tools are installed per user in `%USERPROFILE%\dev` (JDK 17, Android SDK, Go, PostgreSQL, Redis, NATS, SeaweedFS).

1. Plug the phones in by USB with USB debugging on.
2. `powershell -File scripts\dev-up.ps1` starts Postgres, Redis, NATS, SeaweedFS (stands in for R2) and the server on port 8001, and connects every plugged-in phone.
3. Build and install the app:
   ```
   cd android
   gradlew assembleDebug
   adb install -r app\build\outputs\apk\debug\app-debug.apk
   ```

Phones on USB are next to the PC, so this tests "near", not real distances.

**Phones without a cable (same office network):** `powershell -File scripts\dev-up.ps1 -Lan` runs the server
on this PC's network address and builds `dist\kinjo-test-<address>.apk`. Send that APK to anyone on the same
network; it works while this PC and the script's windows stay on. Only the server (8001) and photos (8333)
listen on the network; photo uploads need the server's key. Phones elsewhere need the VPS.

The app needs `android/app/google-services.json` (Firebase, not in git: copy it from the old project's `frontend/native/`).

## Build a shareable APK

`gradlew assembleDebug -PkinjoApi=https://kinjo.world` points the app at the live server.

## Tests

- Server: `cd server && go vet ./... && go test ./...` (add `KINJO_TEST_DSN=postgres://kinjo@localhost:5433/kinjo_test` to also run the database tests)
- App: `cd android && gradlew testDebugUnitTest lintDebug`
