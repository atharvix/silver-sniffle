# Kinjo for iPhone — what's left

The iPhone app uses the same project as Android (Capacitor 7, `app.kinjo`). It needs an Apple Developer account,
because iPhones only install signed apps.

## You (once, ~20 minutes + Apple's approval)
1. Enrol at https://developer.apple.com/programs/enroll/ ($99/year, individual is fine).
2. When approved, in App Store Connect → Users and Access → Integrations → App Store Connect API,
   create a key with "App Manager" access and share: Issuer ID, Key ID and the .p8 file.

## Claude (after that)
1. Register bundle id `app.kinjo`, create the Kinjo app in App Store Connect.
2. Add an iOS app in Firebase (bundle `app.kinjo`) and download GoogleService-Info.plist.
3. Build on a macOS runner (GitHub Actions or Codemagic) with `ios-build.sh`:
   `npx cap add ios`, Info.plist location texts + background mode, Google URL scheme, signing via the API key.
4. Upload to TestFlight → you install Kinjo from the TestFlight app on your iPhone.

## iPhone specifics already decided
- Location: "While Using" first, then iOS offers "Always" so Kinjo keeps you visible in the background.
- Info.plist:
  - NSLocationWhenInUseUsageDescription: "Kinjo shows you to people within 30 metres while you're visible."
  - NSLocationAlwaysAndWhenInUseUsageDescription: "Allow Always so people nearby can still see you when Kinjo is in the background. You can hide your profile any time."
  - UIBackgroundModes: location
- Google sign-in: REVERSED_CLIENT_ID from GoogleService-Info.plist added as a URL scheme.
