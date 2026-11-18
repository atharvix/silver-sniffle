# Kinjo for iPhone

Same project as Android: Capacitor 7, the bundled web UI, the Go backend, app id
`com.kinjo.app`. iOS-only pieces live here and in `plugins/kinjo-presence/ios/`
(`ios/` itself is generated and gitignored, like `android/`).

| File | What it does |
|---|---|
| `ios-build.sh` | `app`: generates `ios/` (permission texts, background Bluetooth + silent push, `kinjo://` deep link, icon, dark launch screen, Firebase). `upload`: also signs and uploads to TestFlight. |
| `AppDelegate.swift` | Capacitor's hooks, plus FCM tokens (the server only speaks FCM) and the server's silent `wake` push. |
| `App.entitlements` | Push notifications. |
| `../plugins/kinjo-presence/ios/` | Swift twin of the Android presence service (same JS API, protocol in `docs/proximity.md`): `Ble.swift` (Bluetooth, the signal), `Presence.swift` (session, optional fix while open), `SignIn.swift` (Apple / web sign-in). |

## One-time setup (you)
1. **Apple Developer Program** ($99/year): https://developer.apple.com/programs/enroll/
2. **App Store Connect** → Apps → New app: iOS, bundle id `com.kinjo.app` (register it first under
   Certificates, Identifiers & Profiles → Identifiers, with **Push Notifications** ticked).
3. **API key**: App Store Connect → Users and Access → Integrations → App Store Connect API →
   new key, access **Admin** (App Manager can't use cloud signing: export fails with "Cloud signing permission error"). Keep the Issuer ID, Key ID and the `.p8` file (downloadable once).
4. **APNs key** for push: developer.apple.com → Keys → new key with "Apple Push Notifications service".
   Upload it in Firebase console → Project settings → Cloud Messaging → Apple app configuration.
5. **Firebase**: Project settings → Add app → iOS, bundle id `com.kinjo.app` → download
   `GoogleService-Info.plist`.
6. **GitHub secrets**: `APPLE_TEAM_ID`, `ASC_KEY_ID`, `ASC_ISSUER_ID`, `ASC_KEY_P8` (the .p8 text),
   `GOOGLE_SERVICE_INFO_PLIST` (the plist text).
7. **Backend**: if production sets `ALLOW_ORIGINS`, add `capacitor://localhost` (the iPhone app's
   origin), then restart. Without it every API call from the iPhone fails.

## Build
- GitHub → Actions → **Build iOS (TestFlight)** → Run workflow. About 15 minutes later the build is
  in App Store Connect → TestFlight; add yourself as a tester and install it from the TestFlight app.
- On a Mac instead: put the plist in `ios-setup/`, then
  `API_BASE=https://kinjo.world bash ios-setup/ios-build.sh app` and open `ios/App/App.xcworkspace`.

## How iPhone finds people (docs/proximity.md)
- **Bluetooth** does it, with the app open or in the background (background modes
  bluetooth-central/peripheral, state restoration). iPhones connect to the Kinjo phones they find,
  stay connected to up to 8 iPhones and write each other every ~8 s: each write wakes the other app,
  so two backgrounded iPhones keep each other running. Android phones relay to iPhones the tokens
  they hear.
- **Location** is optional and only "While Using": one fix every 10 s while Kinjo is open, an outdoor
  check for the server. No "Always", no background location, no location pushes.
- **Swiped away**: iOS restarts nothing for an app the person closed; Kinjo shows "Kinjo is closed —
  open it to stay visible". A silent wake push reaches a suspended (not swiped) app.
- **No battery card**: iOS has no per-app restriction to lift.

## Before App Store review (TestFlight works without these)
- **Sign in with Apple**: guideline 4.8 requires it when Google/LinkedIn sign-in is offered. Needs a
  backend provider like `auth.go`'s Google one.
- App Store listing: screenshots, privacy policy URL (https://kinjo.world/privacy-policy), and the
  App Privacy answers (precise location while open, name, email, photos; used for app functionality, not tracking).
- Explain background Bluetooth in the review notes: people nearby see your card while Kinjo is closed.
