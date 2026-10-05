# Kinjo — frontend

The web app (SPA) and the Capacitor phone shell. No Firebase — it talks to the
Go backend (`../backend`) for LinkedIn sign-in, the profile API, and the live
"within 30 m" WebSocket.

```
web/
  src/
    app.fb.html   all screens + CSS
    app.fb.js     app logic: LinkedIn sign-in, profile, location, 30 m matching,
                  card deck, settings. Talks to the backend over REST + WebSocket.
    build_live.py builds deploy/public/index.html from the two files above
    build_preview.py  iPhone-frame preview (see note below)
  deploy/
    build.sh      API_BASE=https://backend bash build.sh  -> public/ ready to host
    public/       the built static site (index.html, sw.js, manifest, icons, img)
  preview/, tests/  dev tools written for the old Firebase build — NOT updated for
                  the LinkedIn/WebSocket backend yet, so they won't run as-is.
native/           Capacitor 7 shell. Loads the live site (server.url). Ships as
                  com.kinjo.app, release-signed with the existing Kinjo key.
                  See android-build.sh and the top-level README.
```

## Build the web app

```bash
API_BASE=https://your-backend-host bash web/deploy/build.sh
# then host the contents of web/deploy/public (nginx on Utho, any static host)
```

`API` defaults to `http://localhost:8080` for local development (edit the
`/*API_BASE*/` line in `web/src/app.fb.js`, or let `build.sh` inject it).

## Build the phone app

Set `server.url` in `native/capacitor.config.json` to your web host, then see
the top-level README for the signed-release APK steps.

## Data (now in Postgres, owned by the backend)

- `users` — LinkedIn account (sub, email).
- `profiles/{uid}` — name, role, look, photo, updated_at (name+photo seeded from
  LinkedIn on first sign-in, all editable).
- `presence/{uid}` — cell, lat, lng, acc, t. Removed on hide / log out / delete,
  and swept after an hour of staleness.
