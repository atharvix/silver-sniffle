# Setting up "Continue with LinkedIn"

Kinjo uses LinkedIn's official **Sign In with LinkedIn using OpenID Connect**. LinkedIn returns the
person's name and email. Only the **name** seeds a brand-new profile (it stays editable); Kinjo never
fetches a LinkedIn photo, and everyone uploads their own. An existing profile is never touched on
login. Google and Apple sign-in are separate (`backend/.env.example`).

## 1. Create the LinkedIn app

1. Go to https://www.linkedin.com/developers/apps → **Create app**.
2. LinkedIn requires the app to be linked to a **Company Page**. If you don't have one, create a
   LinkedIn Company Page first (free), then select it.
3. Fill in app name, logo and the privacy policy URL (`https://kinjo.world/privacy-policy`). Submit.

## 2. Add the OpenID Connect product

1. Open your app → **Products** tab.
2. Request **"Sign In with LinkedIn using OpenID Connect"**. It is granted automatically.
3. The app then has the scopes `openid`, `profile`, `email`.

## 3. Configure Auth

1. App → **Auth** tab. Copy the **Client ID** and **Client Secret**.
2. Under **Authorized redirect URLs for your app**, add the backend callback. It must be **https**
   and match `LINKEDIN_REDIRECT_URL` exactly:

   ```
   https://kinjo.world/auth/linkedin/callback
   ```

   LinkedIn does not allow custom schemes like `kinjo://` here. That is why the backend owns the
   OAuth redirect and then sends the person back to the app's deep link.

## 4. Put the values in the backend `.env`

```
LINKEDIN_CLIENT_ID=<from the Auth tab>
LINKEDIN_CLIENT_SECRET=<from the Auth tab>
LINKEDIN_REDIRECT_URL=https://kinjo.world/auth/linkedin/callback
FRONTEND_URL=kinjo://auth
ALLOW_ORIGINS=https://localhost,capacitor://localhost
```

Never commit `.env` and never put the secret on a command line. `LINKEDIN_REDIRECT_URL`'s origin is
also what the backend uses for absolute photo URLs and email links.

## How the flow works (the phone app, PKCE)

```
[App] tap "Continue with LinkedIn"; the app makes a random verifier and sends its SHA-256 as `challenge`
   -> opens the system browser (iPhone: ASWebAuthenticationSession) at  https://backend/auth/linkedin?challenge=…
      -> redirects to LinkedIn consent (openid profile email)
      -> LinkedIn redirects to  https://backend/auth/linkedin/callback?code=…
         -> backend exchanges the code, reads /v2/userinfo (name, email), creates or finds the account
         -> backend stores a one-time code bound to the challenge and redirects to  kinjo://auth#code=<one-time code>
            -> the app reopens, POSTs {code, verifier} to /auth/exchange, receives its session token,
               calls /api/me, and you're in.
```

Only the app that started the sign-in can redeem the code (RFC 7636 / 8252), so a deep link
intercepted by another app is useless. The LinkedIn access token and the client secret never leave the
backend; the app only ever holds its own opaque session token. A request with no `challenge` (a very
old app build) still gets the legacy `#token=` redirect; that path goes away once those builds do.

On the web, the redirect lands on `FRONTEND_URL` and the app reads the result from the URL hash.

## Account linking

A LinkedIn (or Google) login joins an existing Kinjo account only if the provider says the email is
verified **and** that account's email is verified. An unverified address is never linked: that would
be an account takeover.
