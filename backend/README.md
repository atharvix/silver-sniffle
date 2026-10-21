# Kinjo backend (Go)

Replaces Firebase entirely. Owns:

- **LinkedIn sign-in** ("Sign In with LinkedIn using OpenID Connect") — the only
  auth method. Name, email and photo come back from LinkedIn's `userinfo`.
- **The database** — Postgres (`users`, `profiles`, `presence`, `sessions`),
  created automatically on first boot.
- **Live matching** — a WebSocket hub that pushes the "people within 30 m" deck,
  replacing Firestore's realtime listeners.

## Layout

```
main.go     server wiring, routes, graceful shutdown
config.go   env-var config (fails fast if anything required is missing)
store.go    Postgres schema + all queries
auth.go     LinkedIn OAuth flow, sessions, CORS, auth middleware
hub.go      in-memory presence + cell index + matching + profile cache
client.go   one WebSocket connection (read/write pumps, pos/hide messages)
api.go      REST: /api/me, /api/profile, /api/logout, /api/account
geo.go      haversine, grid cells, 30 m inside test (ported from the old client)
```

## Run locally

```bash
cp .env.example .env    # fill in DATABASE_URL + LinkedIn creds
set -a; . ./.env; set +a
go run .
```

Needs a reachable Postgres. `go test ./...` covers the geo/matching logic and
needs no database.

## Endpoints

| Method | Path                       | Auth        | Purpose                          |
|--------|----------------------------|-------------|----------------------------------|
| GET    | `/auth/linkedin`           | none        | redirect to LinkedIn consent     |
| GET    | `/auth/linkedin/callback`  | none        | finish OAuth, hand SPA a token   |
| GET    | `/api/me`                  | Bearer      | account + profile (or null)      |
| PUT    | `/api/profile`             | Bearer      | save name/role/look/photo        |
| POST   | `/api/logout`              | Bearer      | end session, leave the map       |
| DELETE | `/api/account`             | Bearer      | delete account + all data        |
| GET    | `/ws?token=...`            | token query | realtime nearby (`pos`/`hide`)   |
| GET    | `/healthz`                 | none        | liveness                         |

The session token is returned to the SPA in the redirect fragment
(`FRONTEND_URL/#token=...`). The SPA stores it and sends it as
`Authorization: Bearer <token>` and as the `?token=` on the WebSocket.

WebSocket messages (client → server):
`{"type":"pos","lat":..,"lng":..,"acc":..}` and `{"type":"hide"}`.
Server → client: `{"type":"nearby","people":[{uid,name,role,desc,img,d}]}`.

## Deploy on Utho (systemd)

```bash
# on the VPS
sudo apt install -y postgresql
sudo -u postgres createuser kinjo --pwprompt
sudo -u postgres createdb kinjo -O kinjo

# build (on the VPS or cross-compile and scp the binary)
CGO_ENABLED=0 go build -o kinjo-backend .

sudo cp kinjo-backend /usr/local/bin/
sudo cp deploy/kinjo-backend.service /etc/systemd/system/
sudo cp .env /etc/kinjo-backend.env      # 0600, owned by the service user
sudo systemctl daemon-reload
sudo systemctl enable --now kinjo-backend
```

Put nginx/Caddy in front for TLS and proxy `/auth`, `/api` and `/ws`
(WebSocket upgrade headers) to `127.0.0.1:8080`.

## LinkedIn app setup

1. https://www.linkedin.com/developers/apps → create an app.
2. Add the product **Sign In with LinkedIn using OpenID Connect**.
3. Under **Auth**, add the exact `LINKEDIN_REDIRECT_URL` as an authorized
   redirect URL, and copy the Client ID / Client Secret into `.env`.
