# Deploying the server to kinjo.world

## What runs where

| Piece | Where | Holds |
|---|---|---|
| nginx (existing) | VPS, ports 80/443 | HTTPS; forwards `/api/`, `/ws`, `/healthz` to the server |
| Kinjo server (one Go binary) | VPS, 127.0.0.1:8001 | nothing of its own: any number can run |
| PostgreSQL (existing) | VPS, local socket | accounts, cards (text + photo key), push tokens: database `kinjo_v2` |
| Redis | VPS, 127.0.0.1:6379, **no persistence** | live state only: positions (1-10 min), who is near whom, Bluetooth tokens, coarse areas (12 h), wake limits; pub/sub between servers |
| NATS + JetStream | VPS, 127.0.0.1:4222 | the wake queue (messages expire after 2 min) |
| Cloudflare R2 (S3-compatible) | Cloudflare | card photos, served through its public URL / CDN |
| Firebase Cloud Messaging | Google | delivers wake pushes to Android |

No Docker, no Go or Node on the VPS: the server is built on a dev machine and copied as one file.
Scaling later: more servers behind nginx `upstream`, Redis Cluster (`REDIS_CLUSTER=...`), a NATS cluster. No code change.

## One time, on the VPS (as root)

Database (already done): `kinjo_v2` owned by role `kinjo`, peer auth over the local socket.

First copy the config files up (from `server/` on the dev machine): `scp deploy/redis-kinjo.conf deploy/nats.conf deploy/nats.service root@kinjo.world:/tmp/`, then on the VPS `cd /tmp`.

```bash
ss -ltnp | grep -E ':(8001|6379|4222) '        # must print nothing

useradd --system --no-create-home kinjo         # skip if it exists

# Redis, live state only, never on disk
apt-get install -y redis-server
cat redis-kinjo.conf >> /etc/redis/redis.conf
systemctl restart redis-server && redis-cli ping           # PONG

# NATS with JetStream
curl -fsSL https://github.com/nats-io/nats-server/releases/download/v2.10.24/nats-server-v2.10.24-linux-amd64.tar.gz | tar xz -C /tmp
install -m 0755 /tmp/nats-server-v2.10.24-linux-amd64/nats-server /usr/local/bin/nats-server
useradd --system --no-create-home nats
install -d -o nats -g nats /var/lib/nats /etc/nats
install -m 0644 nats.conf /etc/nats/nats.conf
install -m 0644 nats.service /etc/systemd/system/nats.service
systemctl daemon-reload && systemctl enable --now nats

# Photos: in Cloudflare, create an R2 bucket (e.g. kinjo-photos), turn on its public URL (r2.dev or a
# custom domain such as photos.kinjo.world), and create an R2 API token with read+write on that bucket.

install -d -m 0750 -o root -g kinjo /etc/kinjo
cat > /etc/kinjo/server.env <<'EOF'
ADDR=127.0.0.1:8001
DATABASE_URL=postgres:///kinjo_v2?host=/var/run/postgresql
REDIS_URL=redis://127.0.0.1:6379/0
NATS_URL=nats://127.0.0.1:4222
S3_ENDPOINT=https://<account-id>.r2.cloudflarestorage.com
S3_REGION=auto
S3_BUCKET=kinjo-photos
S3_ACCESS_KEY=<R2 access key id>
S3_SECRET_KEY=<R2 secret>
PHOTO_BASE_URL=https://photos.kinjo.world
FCM_PROJECT_ID=kinjo-3fd56
FCM_CREDENTIALS_FILE=/path/to/firebase-service-account.json
EOF
chmod 0640 /etc/kinjo/server.env
chgrp kinjo /path/to/firebase-service-account.json && chmod 0640 /path/to/firebase-service-account.json
```

Without the two `FCM_` lines the server runs, but phones whose app was swiped away aren't woken (the log says "wake pushes off").

## Every release (from the dev machine, in `server/`)

```bash
bash deploy/deploy.sh root@kinjo.world
```

Builds a static Linux binary, copies it, installs `kinjo-v2.service`, restarts it and prints `health 204`.

## Switch traffic (once, when you're ready)

Add `deploy/nginx-kinjo.conf` to the kinjo.world `server` block, then `nginx -t && systemctl reload nginx`.
Check from anywhere: `curl -i https://kinjo.world/healthz` → `204`. Old app builds stop working at this moment.

Roll back: remove those locations and reload nginx; the old backend is untouched.

## Health

- `curl -s 127.0.0.1:8001/metrics | grep ^kinjo_` on the VPS: sockets, fixes, wakes (queued/sent/failed/gone), errors.
- `journalctl -u kinjo-v2 -f`: JSON logs, never positions or tokens.
