#!/bin/bash
# Builds the Kinjo Android app. No Firebase. The web UI is BUNDLED into the APK
# (loaded from local assets, not a remote URL), and it talks to the Go backend
# on Utho over HTTPS. Sign-in (LinkedIn/Google) runs in the system browser, returning to
# the app through the kinjo://auth deep link.
#
# The APK is RELEASE-signed with the existing Kinjo key (package com.kinjo.app)
# so Play Console treats it as an update, not a new app.
#
#   ./android-build.sh sdk                         one-time: Android SDK + licences
#   API_BASE=https://api.host ./android-build.sh app   build the signed release APK
#
# Required for `app`:
#   API_BASE                  the Go backend base URL (e.g. https://api.kinjo.example)
# Signing credentials (same names as the CI secrets; values in CREDENTIALS_AND_OPS.md):
#   ANDROID_KEYSTORE_FILE     (default: ./kinjo-release-key.jks)
#   ANDROID_KEYSTORE_PASSWORD
#   ANDROID_KEY_ALIAS         (default: kinjo-release-key)
#   ANDROID_KEY_PASSWORD
set -e
cd "$(dirname "$0")"
export ANDROID_HOME=$HOME/android-sdk ANDROID_SDK_ROOT=$HOME/android-sdk
export PATH=$ANDROID_HOME/cmdline-tools/latest/bin:$ANDROID_HOME/platform-tools:$PATH

if [ "$1" = "sdk" ]; then
  if [ ! -x $ANDROID_HOME/cmdline-tools/latest/bin/sdkmanager ]; then
    mkdir -p $ANDROID_HOME/cmdline-tools
    curl -sSLo /tmp/clt.zip https://dl.google.com/android/repository/commandlinetools-linux-11076708_latest.zip
    rm -rf /tmp/clt && unzip -q /tmp/clt.zip -d /tmp/clt && rm -rf $ANDROID_HOME/cmdline-tools/latest && mv /tmp/clt/cmdline-tools $ANDROID_HOME/cmdline-tools/latest
  fi
  yes | sdkmanager --licenses >/dev/null 2>&1 || true
  sdkmanager "platform-tools" "platforms;android-35" "build-tools;35.0.0" >/dev/null
  echo "SDK READY"; java -version 2>&1 | head -1; df -h $HOME | tail -1
  exit 0
fi

if [ "$1" = "app" ]; then
  : "${API_BASE:?set API_BASE to the Go backend URL, e.g. https://api.kinjo.example}"
  KS_FILE=${ANDROID_KEYSTORE_FILE:-kinjo-release-key.jks}
  KS_ALIAS=${ANDROID_KEY_ALIAS:-kinjo-release-key}
  if [ ! -f "$KS_FILE" ]; then echo "missing keystore: $KS_FILE" >&2; exit 1; fi
  if [ -z "$ANDROID_KEYSTORE_PASSWORD" ] || [ -z "$ANDROID_KEY_PASSWORD" ]; then
    echo "set ANDROID_KEYSTORE_PASSWORD and ANDROID_KEY_PASSWORD (see CREDENTIALS_AND_OPS.md)" >&2; exit 1
  fi

  # 1. build the web UI (inject the backend URL) and bundle it into www/
  API_BASE="$API_BASE" bash ../web/deploy/build.sh
  rm -rf www && mkdir -p www && cp -a ../web/deploy/public/. www/
  echo "web UI bundled into www/ (API_BASE=$API_BASE)"

  npm install --no-audit --no-fund --loglevel=error
  [ -d android ] || npx cap add android

  # dark system bars
  python3 - <<'PY'
import re
p='android/app/src/main/res/values/styles.xml'; s=open(p).read()
if 'statusBarColor' not in s:
    s=re.sub(r'(<style name="AppTheme.NoActionBar"[^>]*>)', r'\1\n        <item name="android:statusBarColor">#0a0a0a</item>\n        <item name="android:navigationBarColor">#0a0a0a</item>\n        <item name="android:windowLightStatusBar">false</item>\n        <item name="android:windowBackground">@android:color/black</item>', s, count=1)
    open(p,'w').write(s)
print('styles patched')
PY

  # deep link: kinjo://auth brings the sign-in result back into the app
  python3 - <<'PY'
p='android/app/src/main/AndroidManifest.xml'; s=open(p).read()
if 'android:scheme="kinjo"' not in s:
    deeplink=('\n            <intent-filter>\n'
              '                <action android:name="android.intent.action.VIEW" />\n'
              '                <category android:name="android.intent.category.DEFAULT" />\n'
              '                <category android:name="android.intent.category.BROWSABLE" />\n'
              '                <data android:scheme="kinjo" android:host="auth" />\n'
              '            </intent-filter>\n        ')
    # insert just before the MainActivity </activity>
    i=s.index('</activity>')
    s=s[:i]+deeplink+s[i:]
    open(p,'w').write(s)
    print('deep link intent-filter added')
else:
    print('deep link already present')
PY

  # launch screen: just the app's background, no logo. The web app's animated logo is
  # the first logo anyone sees (a static one here showed the logo twice). Android 12+
  # always shows a system splash: same colour, transparent icon. White/dark follows the
  # phone's theme, like the web app, so there's no flash between the two.
  python3 - <<'PY'
import os, re
res = 'android/app/src/main/res'
for d, c in (('values', '#ffffff'), ('values-night', '#0a0a0a')):
    os.makedirs(f'{res}/{d}', exist_ok=True)
    open(f'{res}/{d}/kinjo_launch.xml', 'w').write(
        f'<?xml version="1.0" encoding="utf-8"?>\n<resources><color name="kinjo_launch">{c}</color></resources>\n')
p = f'{res}/values/styles.xml'; s = open(p).read()
launch = ('<style name="AppTheme.NoActionBarLaunch" parent="Theme.SplashScreen">\n'
          '        <item name="android:windowBackground">@color/kinjo_launch</item>\n'
          '        <item name="windowSplashScreenBackground">@color/kinjo_launch</item>\n'
          '        <item name="windowSplashScreenAnimatedIcon">@android:color/transparent</item>\n'
          '        <item name="postSplashScreenTheme">@style/AppTheme.NoActionBar</item>\n'
          '    </style>')
s = re.sub(r'<style name="AppTheme.NoActionBarLaunch".*?</style>', launch, s, flags=re.S)
s = s.replace('<item name="android:windowBackground">@android:color/black</item>', '<item name="android:windowBackground">@color/kinjo_launch</item>')
open(p, 'w').write(s)
print('launch screen: background only')
PY

  # icons
  cp -r res/mipmap-* android/app/src/main/res/ && cp res/values/ic_launcher_background.xml android/app/src/main/res/values/
  [ -f google-services.json ] && cp google-services.json android/app/
  echo "icons + google-services copied"

  # release signing with the existing Kinjo key (so Play sees an update)
  cp "$KS_FILE" android/app/kinjo-release-key.jks
  cat > android/key.properties <<EOF
storeFile=kinjo-release-key.jks
storePassword=$ANDROID_KEYSTORE_PASSWORD
keyAlias=$KS_ALIAS
keyPassword=$ANDROID_KEY_PASSWORD
EOF
  python3 - <<'PY'
p='android/app/build.gradle'; s=open(p).read()
if 'signingConfigs' not in s:
    load=('\ndef keystoreProperties = new Properties()\n'
          'def keystorePropertiesFile = rootProject.file("key.properties")\n'
          'if (keystorePropertiesFile.exists()) { keystoreProperties.load(new FileInputStream(keystorePropertiesFile)) }\n')
    s = s.replace('android {', load + '\nandroid {', 1)
    # point buildTypes.release at our signing config (anchor on minifyEnabled so
    # this targets buildTypes.release, not the signingConfigs.release block)
    s = s.replace('release {\n            minifyEnabled',
                  'release {\n            signingConfig signingConfigs.release\n            minifyEnabled', 1)
    sign=('    signingConfigs {\n'
          '        release {\n'
          '            if (keystorePropertiesFile.exists()) {\n'
          "                storeFile file(keystoreProperties['storeFile'])\n"
          "                storePassword keystoreProperties['storePassword']\n"
          "                keyAlias keystoreProperties['keyAlias']\n"
          "                keyPassword keystoreProperties['keyPassword']\n"
          '            }\n'
          '        }\n'
          '    }\n')
    s = s.replace('    buildTypes {', sign + '    buildTypes {', 1)
    open(p,'w').write(s)
    print('signing config injected')
else:
    print('signing config already present')
PY

  npx cap sync android >/dev/null
  cd android && ./gradlew assembleRelease --no-daemon -q --console=plain 2>&1 | grep -vE "^w: |warning:|Note:" | tail -25
  cd ..
  APK=android/app/build/outputs/apk/release/app-release.apk
  ls -la $APK && echo "APK READY: $APK"
  $ANDROID_HOME/build-tools/35.0.0/apksigner verify --print-certs "$APK" 2>/dev/null | grep -i "SHA-1" || true
  exit 0
fi
echo "usage: $0 sdk | API_BASE=https://... $0 app"
