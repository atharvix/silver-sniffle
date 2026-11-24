# Kinjo for iPhone

Same project as Android: Capacitor 7, the bundled web UI, the Go backend, app id
`com.kinjo.app`. iOS-only pieces live here and in `plugins/kinjo-presence/ios/`
(`ios/` itself is generated and gitignored, like `android/`).

| File | What it does |
|---|---|
| `ios-build.sh` | `app`: generates `ios/` (permission texts for location, Bluetooth and camera, background modes, silent push, `kinjo://` deep link, icon, dark launch screen, Firebase, and the location-push extension when `LOCATION_PUSH=1`). `upload`: also signs and uploads to TestFlight. |
| `AppDelegate.swift` | Capacitor's hooks, plus FCM tokens (the server only speaks FCM) and the server's silent `wake` push. |
| `App.entitlements` | Push, the location-push entitlement and the App Group `group.com.kinjo.app`. |
| `KinjoLocationPush/` | The Location Push Service Extension: answers the server's location pushes with one `still` fix, so a swiped-away iPhone stays findable. |
| `../plugins/kinjo-presence/ios/` | Swift twin of the Android presence service (same JS API, protocol in `docs/proximity.md`): `Presence.swift` (session, location, the signal), `Live.swift` (the reporter socket), `Ble.swift` (Bluetooth, for older builds), `SignIn.swift` (Apple / web sign-in). |

## One-time setup (you)
1. **Apple Developer Program** ($99/year): https://developer.apple.com/programs/enroll/
2. **App Store Connect** → Apps → New app: iOS, bundle id `com.kinjo.app` (register it first under
   Certificates, Identifiers & Profiles → Identifiers, with **Push Notifications** ticked).
3. **API key**: App Store Connect → Users and Access → Integrations → App Store Connect API →
   new key, access **Admin** (App Manager can't use cloud signing: export fails with "Cloud signing permission error"). Keep the Issuer ID, Key ID and the `.p8` file (downloadable once).
4. **APNs key** for push: developer.apple.com → Keys → new key with "Apple Push Notifications service".
   Upload it in Firebase console → Project settings → Cloud Messaging → Apple app configuration.
   The same key also serves the location pushes (`APNS_KEY_ID`, `APNS_TEAM_ID`, `APNS_KEY_FILE` on the
   server); `kinjo-api apns-check` asks Apple whether it works.
5. **Firebase**: Project settings → Add app → iOS, bundle id `com.kinjo.app` → download
   `GoogleService-Info.plist`.
6. **GitHub secrets**: `APPLE_TEAM_ID`, `ASC_KEY_ID`, `ASC_ISSUER_ID`, `ASC_KEY_P8` (the .p8 text),
   `GOOGLE_SERVICE_INFO_PLIST` (the plist text).
7. **Backend**: if production sets `ALLOW_ORIGINS`, add `capacitor://localhost` (the iPhone app's
   origin), then restart. Without it every API call from the iPhone fails.

## Build
- GitHub → Actions → **Build iOS (TestFlight)** → Run workflow. About 15 minutes later the build is
  in App Store Connect → TestFlight; add yourself as a tester and install it from the TestFlight app.
  The `app_version` input must be higher than the last approved version; `location_push` (on by
  default) builds the location-push extension.
- On a Mac instead: put the plist in `ios-setup/`, then
  `API_BASE=https://kinjo.world bash ios-setup/ios-build.sh app` and open `ios/App/App.xcworkspace`.

## How iPhone finds people (docs/proximity.md)
- **Location** does it (v3). One "While Using" permission, asked from the open app. Started there, location
  keeps running in the background (blue indicator) while the profile is visible. The phone reports over its
  own reporter socket (`Live.swift`, HTTP when it is down), and the server answers each report with a
  `mode` (with / near / around / idle) that says how hard to work: GPS only when someone could be in range,
  Wi-Fi / cell otherwise. After 2 minutes within about 25 m it rests: coarse updates, the last fix re-sent
  as `still` every 5 minutes if the socket is down (an open socket holds it with no reports), and an
  accelerometer check for 2 s of every 10 to notice walking.
- **Swiped away**: iOS ends the app, so only the location-push extension can still locate it. It needs
  "Always" (asked once) and Apple's entitlement. The server sends one location push 20 s after the phone's
  socket closes, then one every 8 minutes while it stays quiet, for up to 12 hours. Without the extension,
  Kinjo shows "Kinjo is closed" and the phone is not found until it is reopened.
- **Bluetooth** (`Ble.swift`) only serves older app builds on the other phone. It runs while the app is on
  screen and rests in the background whenever location is allowed (`restInBackground`); on a phone that
  has Bluetooth but no precise location it keeps running in the background as before.
- **No battery card**: iOS has no per-app restriction to lift.
- **Photos**: "Take photo" on the profile screen opens the camera, which is why the generated Info.plist
  has `NSCameraUsageDescription`. Without that key iOS ends the app when the camera opens.

## Before App Store review (TestFlight works without these)
- **Sign in with Apple** is built in (`apple.go`, `SignIn.swift`); guideline 4.8 needs it because Google and
  LinkedIn sign-in are offered. To revoke the grant when an account is deleted, set `APPLE_SIGNIN_KEY_*`
  on the server.
- **Demo account** for the reviewer: set `DEMO_EMAIL` and `DEMO_CODE` on the server. It signs in with the
  fixed code and always sees sample cards.
- App Store listing: screenshots, privacy policy URL (https://kinjo.world/privacy-policy), and the App
  Privacy answers (precise location, name, email, photos; used for app functionality, not tracking).
- Explain location in the review notes: people nearby see your card while Kinjo is open and while it runs
  in the background; nothing is stored, and the person can hide their profile at any time.
