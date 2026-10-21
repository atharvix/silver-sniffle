#!/bin/bash
# Builds the Kinjo iPhone app: the same bundled web UI, backend and app id
# (com.kinjo.app, from capacitor.config.json) as Android. Needs macOS with Xcode and
# CocoaPods (a Mac, or the "Build iOS" GitHub workflow).
#
#   API_BASE=https://kinjo.world bash ios-setup/ios-build.sh app      Xcode project in ios/ (open ios/App/App.xcworkspace)
#   API_BASE=… APPLE_TEAM_ID=… ASC_KEY_ID=… ASC_ISSUER_ID=… ASC_KEY_PATH=… \
#     BUILD_NUMBER=7 bash ios-setup/ios-build.sh upload               signed build → TestFlight
#   LOCATION_PUSH=1 …   also builds the KinjoLocationPush extension (only once Apple granted
#                       com.apple.developer.location.push: signing fails without it)
#
# Needs ios-setup/GoogleService-Info.plist (Firebase console → the iOS app, bundle com.kinjo.app).
# ios/ is generated and gitignored like android/: native code lives in plugins/ and ios-setup/.
set -e
cd "$(dirname "$0")/.."
: "${API_BASE:?set API_BASE to the Go backend URL, e.g. https://kinjo.world}"
[ -f ios-setup/GoogleService-Info.plist ] || { echo "missing ios-setup/GoogleService-Info.plist (Firebase → iOS app)" >&2; exit 1; }

# 1. the web UI, exactly as Android bundles it
API_BASE="$API_BASE" bash ../web/deploy/build.sh
rm -rf www && mkdir -p www && cp -a ../web/deploy/public/. www/

npm install --no-audit --no-fund --loglevel=error
# cap add runs pod install while the project still targets iOS 14, below KinjoPresence's 15.0:
# that install fails, the project itself is already made. The target is raised below and
# cap sync installs the pods again.
[ -d ios ] || npx cap add ios --packagemanager CocoaPods || [ -f ios/App/Podfile ]

A=ios/App/App
P=$A/Info.plist
pb() { /usr/libexec/PlistBuddy -c "$1" "$P"; }
put() { pb "Delete :$1" 2>/dev/null || true; pb "Add :$1 $2 $3"; }

# 2. Info.plist: permission texts, background location + silent push, kinjo://auth, dark status bar
put NSLocationWhenInUseUsageDescription string "Kinjo uses your location to show you people within 30 metres who you could connect with."
put NSBluetoothAlwaysUsageDescription string "Kinjo uses Bluetooth to find people within a few metres of you indoors, where location is too vague. Only a random code that changes every 15 minutes is sent."
put NSLocationAlwaysAndWhenInUseUsageDescription string "Allow Always so people near you can still see your card when Kinjo is closed. Hide your profile any time to stop."
pb "Delete :UIBackgroundModes" 2>/dev/null || true
pb "Add :UIBackgroundModes array"
pb "Add :UIBackgroundModes:0 string location"
pb "Add :UIBackgroundModes:1 string remote-notification"
pb "Add :UIBackgroundModes:2 string bluetooth-central"
pb "Add :UIBackgroundModes:3 string bluetooth-peripheral"
pb "Delete :CFBundleURLTypes" 2>/dev/null || true
pb "Add :CFBundleURLTypes array"
pb "Add :CFBundleURLTypes:0 dict"
pb "Add :CFBundleURLTypes:0:CFBundleURLName string com.kinjo.app.auth"
pb "Add :CFBundleURLTypes:0:CFBundleURLSchemes array"
pb "Add :CFBundleURLTypes:0:CFBundleURLSchemes:0 string kinjo"
put UIStatusBarStyle string UIStatusBarStyleLightContent
put UIViewControllerBasedStatusBarAppearance bool false
put ITSAppUsesNonExemptEncryption bool false   # HTTPS only: no export-compliance question on every upload

# 3. our AppDelegate (FCM token, wake push), push entitlement, Firebase config
cp ios-setup/AppDelegate.swift $A/AppDelegate.swift
cp ios-setup/App.entitlements $A/App.entitlements
cp ios-setup/GoogleService-Info.plist $A/GoogleService-Info.plist
python3 - <<'PY'
import os, re
p = 'ios/App/App.xcodeproj/project.pbxproj'; s = open(p).read()
# App target only (on the xcodebuild line these would reach the Pods frameworks too)
assert s.count('PRODUCT_BUNDLE_IDENTIFIER = com.kinjo.app;') >= 2, 'bundle id is not com.kinjo.app: check capacitor.config.json'
if 'CODE_SIGN_ENTITLEMENTS' not in s:
    s = s.replace('PRODUCT_BUNDLE_IDENTIFIER = com.kinjo.app;', 'CODE_SIGN_ENTITLEMENTS = App/App.entitlements;\n\t\t\t\tPRODUCT_BUNDLE_IDENTIFIER = com.kinjo.app;')
s = re.sub(r'MARKETING_VERSION = [^;]+;', 'MARKETING_VERSION = %s;' % os.environ.get('APP_VERSION', '1.0.0'), s)
s = re.sub(r'CURRENT_PROJECT_VERSION = [^;]+;', 'CURRENT_PROJECT_VERSION = %s;' % os.environ.get('BUILD_NUMBER', '1'), s)
# GoogleService-Info.plist must be in the app bundle: add it to the App target's resources once
if 'GoogleService-Info.plist' not in s:
    ref, bf = 'A1B2C3D4E5F60718293A4B5C', 'A1B2C3D4E5F60718293A4B5D'
    s = s.replace('/* Begin PBXBuildFile section */',
        '/* Begin PBXBuildFile section */\n\t\t%s /* GoogleService-Info.plist in Resources */ = {isa = PBXBuildFile; fileRef = %s /* GoogleService-Info.plist */; };' % (bf, ref), 1)
    s = s.replace('/* Begin PBXFileReference section */',
        '/* Begin PBXFileReference section */\n\t\t%s /* GoogleService-Info.plist */ = {isa = PBXFileReference; lastKnownFileType = text.plist.xml; path = "GoogleService-Info.plist"; sourceTree = "<group>"; };' % ref, 1)
    s = re.sub(r'(/\* App \*/ = \{\s*isa = PBXGroup;\s*children = \()', r'\1\n\t\t\t\t%s /* GoogleService-Info.plist */,' % ref, s, count=1)
    s = re.sub(r'(/\* Resources \*/ = \{\s*isa = PBXResourcesBuildPhase;\s*buildActionMask = \d+;\s*files = \()', r'\1\n\t\t\t\t%s /* GoogleService-Info.plist in Resources */,' % bf, s, count=1)
    assert s.count(ref) == 3 and s.count(bf) == 2, 'pbxproj layout changed: add GoogleService-Info.plist in Xcode'
# iOS 15+: current Firebase needs it (covers almost every iPhone in use)
s = re.sub(r'IPHONEOS_DEPLOYMENT_TARGET = 1[0-4]\.\d;', 'IPHONEOS_DEPLOYMENT_TARGET = 15.0;', s)
open(p, 'w').write(s)
p = 'ios/App/Podfile'; s = open(p).read()
s = re.sub(r"platform :ios, '1[0-4]\.\d'", "platform :ios, '15.0'", s)
if "FirebaseMessaging" not in s:
    s = s.replace("  # Add your Pods here", "  # Add your Pods here\n  pod 'FirebaseMessaging'", 1)
    assert "FirebaseMessaging" in s, 'Podfile layout changed: add pod FirebaseMessaging to target App'
open(p, 'w').write(s)
print('entitlements, version, Firebase, iOS 15 wired')
PY

# 4. icon (1024 px, no transparency) and a plain dark launch screen, like Android's
cp assets/icon-only.png $A/Assets.xcassets/AppIcon.appiconset/AppIcon-512@2x.png
python3 - <<'PY'
import glob, struct, zlib
def png(path, w, h, rgb):
    row = b'\x00' + bytes(rgb) * w
    chunk = lambda t, d: struct.pack('>I', len(d)) + t + d + struct.pack('>I', zlib.crc32(t + d) & 0xffffffff)
    open(path, 'wb').write(b'\x89PNG\r\n\x1a\n' + chunk(b'IHDR', struct.pack('>IIBBBBB', w, h, 8, 2, 0, 0, 0))
                           + chunk(b'IDAT', zlib.compress(row * h, 9)) + chunk(b'IEND', b''))
for f in glob.glob('ios/App/App/Assets.xcassets/Splash.imageset/*.png'):
    png(f, 2732, 2732, (10, 10, 10))
print('icon + launch screen')
PY

# 3b. Location Push Service Extension (wakes a swiped-away Kinjo for one fix per server push)
if [ "${LOCATION_PUSH:-0}" = 1 ]; then
  E=$A/../KinjoLocationPush   # ios/App/KinjoLocationPush, next to ios/App/App
  mkdir -p "$E" && cp ios-setup/KinjoLocationPush/* "$E/"
  ent() { /usr/libexec/PlistBuddy -c "$1" $A/App.entitlements; }
  ent "Delete :com.apple.developer.location.push" 2>/dev/null || true
  ent "Add :com.apple.developer.location.push bool true"
  ent "Delete :com.apple.security.application-groups" 2>/dev/null || true
  ent "Add :com.apple.security.application-groups array"
  ent "Add :com.apple.security.application-groups:0 string group.com.kinjo.app"
  # the xcodeproj gem ships with CocoaPods; a Homebrew CocoaPods keeps it private, so install it if missing
  ruby -e 'require "xcodeproj"' 2>/dev/null || gem install --user-install --no-document xcodeproj
  APP_VERSION="${APP_VERSION:-1.0.0}" BUILD_NUMBER="${BUILD_NUMBER:-1}" ruby - <<'RB'
require 'xcodeproj'
name = 'KinjoLocationPush'
proj = Xcodeproj::Project.open('ios/App/App.xcodeproj')
if proj.targets.any? { |t| t.name == name }
  puts "#{name}: already in the project"
else
  app = proj.targets.find { |t| t.name == 'App' } or abort('no App target')
  ext = proj.new_target(:app_extension, name, :ios, '15.0', nil, :swift)
  grp = proj.main_group.new_group(name, name)
  ext.add_file_references([grp.new_reference('LocationPushService.swift')])
  grp.new_reference('Info.plist')
  grp.new_reference("#{name}.entitlements")
  ext.build_configurations.each do |c|
    s = c.build_settings
    s['PRODUCT_BUNDLE_IDENTIFIER'] = 'com.kinjo.app.LocationPush'   # must start with the app's id
    s['PRODUCT_NAME'] = '$(TARGET_NAME)'
    s['INFOPLIST_FILE'] = "#{name}/Info.plist"
    s['GENERATE_INFOPLIST_FILE'] = 'NO'
    s['CODE_SIGN_ENTITLEMENTS'] = "#{name}/#{name}.entitlements"
    s['CODE_SIGN_STYLE'] = 'Automatic'
    s['SWIFT_VERSION'] = '5.0'
    s['IPHONEOS_DEPLOYMENT_TARGET'] = '15.0'
    s['TARGETED_DEVICE_FAMILY'] = '1,2'
    s['APPLICATION_EXTENSION_API_ONLY'] = 'YES'
    s['SKIP_INSTALL'] = 'YES'
    s['MARKETING_VERSION'] = ENV['APP_VERSION']
    s['CURRENT_PROJECT_VERSION'] = ENV['BUILD_NUMBER']
    s['LD_RUNPATH_SEARCH_PATHS'] = ['$(inherited)', '@executable_path/Frameworks', '@executable_path/../../Frameworks']
  end
  embed = app.new_copy_files_build_phase('Embed Foundation Extensions')
  embed.dst_subfolder_spec = '13'   # PlugIns
  bf = embed.add_file_reference(ext.product_reference, true)
  bf.settings = { 'ATTRIBUTES' => ['RemoveHeadersOnCopy'] }
  # before the CocoaPods script phases: after them Xcode 15+ can report "Cycle inside App"
  app.build_phases.delete(embed)
  app.build_phases.insert(app.build_phases.index(app.resources_build_phase) + 1, embed)
  app.add_dependency(ext)
  proj.save
  puts "#{name}: target added and embedded in App"
end
RB
fi

npx cap sync ios   # copies www/, links the plugins (KinjoPresence) and runs pod install
echo "iOS project ready: open ios/App/App.xcworkspace in Xcode"

[ "$1" = "upload" ] || exit 0

# 5. archive, sign (automatic, via the App Store Connect API key) and upload to TestFlight
: "${APPLE_TEAM_ID:?}" "${ASC_KEY_ID:?}" "${ASC_ISSUER_ID:?}" "${ASC_KEY_PATH:?}"
AUTH=(-allowProvisioningUpdates -authenticationKeyPath "$ASC_KEY_PATH" -authenticationKeyID "$ASC_KEY_ID" -authenticationKeyIssuerID "$ASC_ISSUER_ID")
rm -rf build/ios && mkdir -p build/ios
xcodebuild -workspace ios/App/App.xcworkspace -scheme App -configuration Release \
  -destination 'generic/platform=iOS' -archivePath build/ios/Kinjo.xcarchive "${AUTH[@]}" \
  DEVELOPMENT_TEAM="$APPLE_TEAM_ID" CODE_SIGN_STYLE=Automatic archive
cat > build/ios/ExportOptions.plist <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
  <key>method</key><string>app-store-connect</string>
  <key>destination</key><string>upload</string>
  <key>teamID</key><string>$APPLE_TEAM_ID</string>
  <key>signingStyle</key><string>automatic</string>
  <key>manageAppVersionAndBuildNumber</key><false/>
</dict></plist>
EOF
xcodebuild -exportArchive -archivePath build/ios/Kinjo.xcarchive -exportOptionsPlist build/ios/ExportOptions.plist \
  -exportPath build/ios "${AUTH[@]}"
echo "uploaded build ${BUILD_NUMBER:-1}: it shows in App Store Connect → TestFlight after Apple's processing (~10–30 min)"
