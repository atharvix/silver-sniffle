# Setting up "Continue with LinkedIn"

Kinjo uses LinkedIn's official **Sign In with LinkedIn using OpenID Connect**.
It returns the user's name, profile photo and email, which seed the Kinjo
profile (all editable afterwards).

## 1. Create the LinkedIn app

1. Go to https://www.linkedin.com/developers/apps → **Create app**.
2. LinkedIn requires the app to be linked to a **Company Page**. If you don't
   have one, create a LinkedIn Company Page first (free), then select it.
3. Fill in app name, logo, privacy policy URL. Submit.

## 2. Add the OpenID Connect product

1. Open your app → **Products** tab.
2. Request **"Sign In with LinkedIn using OpenID Connect"**. It's granted
   automatically (no manual review).
3. After it's added, the app has the scopes `openid`, `profile`, `email`.

## 3. Configure Auth

1. App → **Auth** tab.
2. Copy **Client ID** and **Client Secret**.
3. Under **Authorized redirect URLs for your app**, add your backend callback —
   it must be **https** and match `LINKEDIN_REDIRECT_URL` exactly:

   ```
   https://YOUR-UTHO-BACKEND/auth/linkedin/callback
   ```

   (LinkedIn does not allow custom schemes like `kinjo://` here — that's why the
   backend owns the OAuth redirect and then bounces to the app's deep link.)

## 4. Put the values in the backend .env

```
LINKEDIN_CLIENT_ID=<from Auth tab>
LINKEDIN_CLIENT_SECRET=<from Auth tab>
LINKEDIN_REDIRECT_URL=https://YOUR-UTHO-BACKEND/auth/linkedin/callback
FRONTEND_URL=kinjo://auth
ALLOW_ORIGINS=https://localhost
```

## How the flow works (bundled Android app)

```
[App] tap "Continue with LinkedIn"
   -> opens system browser at  https://backend/auth/linkedin
      -> redirects to LinkedIn consent (openid profile email)
      -> LinkedIn redirects to  https://backend/auth/linkedin/callback?code=...
         -> backend exchanges code, reads /v2/userinfo (name, email, picture),
            creates the account + session token
         -> backend redirects to  kinjo://auth#token=<session>
            -> Android reopens the App; it reads the token from the deep link,
               calls /api/me, and you're in.
```

The access token and client secret never leave the backend; the app only ever
holds its own opaque session token.
