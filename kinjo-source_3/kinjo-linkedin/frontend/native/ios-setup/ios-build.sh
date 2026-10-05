#!/bin/bash
# Runs on macOS (CI runner). Needs: GoogleService-Info.plist in ios-setup/, Xcode, CocoaPods.
set -e
cd "$(dirname "$0")/.."
npm install --no-audit --no-fund --loglevel=error
[ -d ios ] || npx cap add ios
cp ios-setup/GoogleService-Info.plist ios/App/App/GoogleService-Info.plist
P=ios/App/App/Info.plist
/usr/libexec/PlistBuddy -c "Add :NSLocationWhenInUseUsageDescription string Kinjo shows you to people within 30 metres while you're visible." $P || true
/usr/libexec/PlistBuddy -c "Add :NSLocationAlwaysAndWhenInUseUsageDescription string Allow Always so people nearby can still see you when Kinjo is in the background. You can hide your profile any time." $P || true
/usr/libexec/PlistBuddy -c "Add :UIBackgroundModes array" $P || true
/usr/libexec/PlistBuddy -c "Add :UIBackgroundModes:0 string location" $P || true
REV=$(/usr/libexec/PlistBuddy -c "Print :REVERSED_CLIENT_ID" ios/App/App/GoogleService-Info.plist)
/usr/libexec/PlistBuddy -c "Add :CFBundleURLTypes array" $P || true
/usr/libexec/PlistBuddy -c "Add :CFBundleURLTypes:0 dict" $P || true
/usr/libexec/PlistBuddy -c "Add :CFBundleURLTypes:0:CFBundleURLSchemes array" $P || true
/usr/libexec/PlistBuddy -c "Add :CFBundleURLTypes:0:CFBundleURLSchemes:0 string $REV" $P || true
npx cap sync ios
echo "iOS project ready: archive + upload with xcodebuild / fastlane using the App Store Connect API key"
