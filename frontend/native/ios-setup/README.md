# Kinjo for iPhone

Same project as Android: Capacitor 7, the bundled web UI, the Go backend, app id
`com.kinjo.app`. iOS-only pieces live here and in `plugins/kinjo-presence/ios/`
(`ios/` itself is generated and gitignored, like `android/`).

| File | What it does |
|---|---|
| `ios-build.sh` | `app`: generates `ios/` (permission texts, background location + silent push, `kinjo://` deep link, icon, dark launch screen, Firebase). `upload`: also signs and uploads to TestFlight. |
| `AppDelegate.swift` | Capacitor's hooks, plus FCM tokens (the server only speaks FCM) and the server's silent `wake` push. |
| `App.entitlements` | Push notifications. |
| `../plugins/kinjo-presence/ios/` | Swift twin of the Android presence service (same JS API): `Presence.swift` (location, relaunch triggers, location-push token), `Ble.swift` (Bluetooth). |
| `KinjoLocationPush/` | The Location Push Service Extension (built only with `LOCATION_PUSH=1`). |

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

## How iPhone keeps up with Android
- **Location**: "While Using" on the location card, then "Always" on the next card (iOS offers that
  choice once; after that the card opens Kinjo's Settings).
- **App running in the background**: location updates never stop, so iOS keeps Kinjo running. They
  are coarse (GPS off) between GPS bursts every 45 s, plus a burst whenever the phone moves ~50 m.
- **Ended by iOS (memory)**: significant-change, visits and a 100 m geofence that follows you
  relaunch Kinjo when you move; the server's silent wake push relaunches a still phone.
- **Swiped away**: iOS restarts nothing for an app the person closed. Kinjo then:
  - shows "Kinjo is closed — open it to stay visible", and
  - with the **Location Push Service Extension** (below), answers the server's location pushes
    with one fix each (at 75 s, then every 10 min for 12 h: inside Apple's ~360/day budget).
- **Bluetooth**: iOS can't put our token in an advert, so iPhones serve it over a short Bluetooth
  connection instead; Android phones add a connectable advert that iPhones can read in the
  background. Two backgrounded iPhones find each other least reliably (Apple doesn't document it);
  everything else works with the screen off. No backend change.
- **No battery card**: iOS has no per-app restriction to lift.

## Location Push Service Extension (closest to Android's always-on service)
Needs Apple's approval, so it's **off** until then:
1. Request `com.apple.developer.location.push`:
   https://developer.apple.com/contact/request/location-push-service-extension/ — describe it as:
   people opt in to share their presence with others nearby, can hide at any time, and only a coarse
   "within 30 m" ever leaves the server. Apple may refuse; everything else works without it.
2. Once granted: in Identifiers, give `com.kinjo.app` and `com.kinjo.app.LocationPush` the
   Location Push and App Groups (`group.com.kinjo.app`) capabilities.
3. Build with `LOCATION_PUSH=1` (add it to the workflow's env).
4. Backend `.env`: `APNS_KEY_ID`, `APNS_TEAM_ID`, `APNS_KEY_FILE` (the same .p8 APNs key as in
   Firebase). Deploy the backend **before** that app build.

## Before App Store review (TestFlight works without these)
- **Sign in with Apple**: guideline 4.8 requires it when Google/LinkedIn sign-in is offered. Needs a
  backend provider like `auth.go`'s Google one.
- App Store listing: screenshots, privacy policy URL (https://kinjo.world/privacy-policy), and the
  App Privacy answers (precise location, name, email, photos; used for app functionality, not tracking).
- Explain "Always" location in the review notes: people nearby see your card while Kinjo is closed.
