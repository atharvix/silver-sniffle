# What you need to run and publish Kinjo

Accounts, keys and settings required to host the backend and ship the apps. For the steps see
`DEPLOYMENT_GUIDE.md`. Real values are only in `CREDENTIALS_AND_OPS.md` and `backend/.env`, never
in git.

## Hosting

| Need | Today |
|---|---|
| A Linux server (Go binary, nginx, Postgres) | Utho VPS, 2 vCPU / 4 GB. Holds about 1,000 position reports/s. |
| PostgreSQL 14+ | PostgreSQL 16 on the same server |
| A domain with TLS | `kinjo.world` (Let's Encrypt). `www` and `kinjoworld.com` redirect to it. |
| Outbound SMTP | GoDaddy SMTP on port 465 (direct TLS: 587 and 25 are blocked by the cloud firewall) |
| An off-box place for encrypted backups | still to be decided |

No object storage is used: profile photos and event covers are stored in Postgres as `data:` URLs
and served (and nginx-cached) by the API.

## Sign-in providers

| Provider | What to create | Backend variables |
|---|---|---|
| LinkedIn (required) | An app with "Sign In with LinkedIn using OpenID Connect"; the redirect URL is the **backend** callback (`https://<host>/auth/linkedin/callback`). See `LINKEDIN_SETUP.md`. | `LINKEDIN_CLIENT_ID`, `LINKEDIN_CLIENT_SECRET`, `LINKEDIN_REDIRECT_URL` |
| Google (optional) | A **Web application** OAuth client; redirect `https://<host>/auth/google/callback`. Browser flow, so no Android SHA-1 setup. | `GOOGLE_CLIENT_ID`, `GOOGLE_CLIENT_SECRET` |
| Sign in with Apple (iPhone) | Needs nothing to sign in. To revoke the grant when someone deletes their account, a key with "Sign in with Apple" enabled. | `APPLE_SIGNIN_KEY_ID`, `APPLE_SIGNIN_KEY_FILE`, `APPLE_TEAM_ID`, `APPLE_BUNDLE_ID` |
| Emailed code | Just working SMTP. | `SMTP_HOST`, `SMTP_PORT`, `SMTP_USER`, `SMTP_PASS`, `SMTP_FROM` |

## Push

- **FCM** (transport only, not Firebase Auth or Firestore): a Firebase project with Cloud Messaging,
  a service-account key (`FCM_CREDENTIALS_FILE` or `FCM_SERVICE_ACCOUNT_JSON`), and
  `google-services.json` (Android) / `GoogleService-Info.plist` (iPhone) in `frontend/native/`.
- **APNs location pushes** (a swiped-away iPhone stays findable): a `.p8` key with the push service,
  `APNS_KEY_ID`, `APNS_TEAM_ID` (or `APPLE_TEAM_ID`), `APNS_KEY_FILE`, and Apple's
  `com.apple.developer.location.push` entitlement (granted) plus the App Group `group.com.kinjo.app`.
  `kinjo-api apns-check` asks Apple whether the key works.

## Other backend settings

`DATABASE_URL`, `PORT`, `FRONTEND_URL` (`kinjo://auth`), `ALLOW_ORIGINS`, `ADMIN_EMAILS`
(who may sign in to Kinjo Control), optional `ADMIN_TOKEN` (scripts only), `DATA_KEY`
(`openssl rand -base64 32`), `DEMO_EMAIL` / `DEMO_CODE` (App Review account), `BACKUP_AGE_RECIPIENT`
(public key only). `backend/.env.example` is the reference.

## Android

- Google Play Console, package `com.kinjo.app`, release key `kinjo-release-key.jks` (alias
  `kinjo-release-key`). Never change either.
- Permissions the app asks for: location while using the app, notifications, and on some phone makers
  the battery / autostart setting. No Bluetooth prompt and no background location.
- Play listing needs a privacy policy URL (`https://kinjo.world/privacy-policy`) and a child-safety
  page (`/child-safety`); the website has both, plus `/delete-account`.
- Camera: "Take photo" on the profile screen uses the phone's camera app; Kinjo does not hold the
  camera permission.

## iPhone

- Apple Developer Program, bundle id `com.kinjo.app` with Push Notifications, the location-push
  entitlement and the App Group.
- An App Store Connect API key (Admin), and the GitHub secrets `APPLE_TEAM_ID`, `ASC_KEY_ID`,
  `ASC_ISSUER_ID`, `ASC_KEY_P8`, `GOOGLE_SERVICE_INFO_PLIST`.
- Info.plist texts are written by `ios-build.sh` (location, Always, Bluetooth, camera). Review notes
  should explain that location runs while the app is open and in the background, and that the demo
  account (`DEMO_EMAIL`) sees sample cards.
- Sign in with Apple is offered, as guideline 4.8 requires.

## Website and admin

`VITE_API_BASE` (the backend URL) and `VITE_ADMIN_PATH` (a private, unguessable path; unset = no
admin panel in that build). The backend must list the admin's email in `ADMIN_EMAILS` and allow the
site's origin in `ALLOW_ORIGINS`.
