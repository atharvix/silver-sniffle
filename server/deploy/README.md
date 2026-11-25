# Deploying the new server to kinjo.world

Runs on the VPS: nginx (HTTPS, existing), PostgreSQL (existing), systemd, and one Go binary built on a dev machine.
No Go, Node, Docker or Redis on the VPS. The server listens on 127.0.0.1:8001; only nginx talks to it.

The new server runs beside the old backend until you switch nginx. Switching nginx is the moment old app builds stop working.

## One time, on the VPS (as root)

```bash
ss -ltnp | grep 8001                # must print nothing: the port is free
useradd --system --no-create-home kinjo
# A NEW database: the old backend's database has a different `users` table and must not be shared.
# The role has the system user's name, so it signs in over the local socket with no password.
sudo -u postgres createuser kinjo
sudo -u postgres createdb -O kinjo kinjo_v2
install -d -m 0750 -o root -g kinjo /etc/kinjo
cat > /etc/kinjo/server.env <<'EOF'
DATABASE_URL=postgres:///kinjo_v2?host=/var/run/postgresql
ADDR=127.0.0.1:8001
PUBLIC_URL=https://kinjo.world
FCM_PROJECT_ID=kinjo-3fd56
FCM_CREDENTIALS_FILE=/path/to/firebase-service-account.json
EOF
chmod 0640 /etc/kinjo/server.env
# The Firebase file the old backend uses; the kinjo user must be able to read it:
chgrp kinjo /path/to/firebase-service-account.json && chmod 0640 /path/to/firebase-service-account.json
```

Without the two `FCM_` lines the server runs, but phones whose app was swiped away aren't woken (the log says "wake pushes off").

## Every release (from the dev machine, in `server/`)

```bash
bash deploy/deploy.sh root@kinjo.world
```

It builds a static Linux binary, copies it, installs `kinjo-v2.service`, restarts it and prints `health 204`.

## Switch traffic (once, when you're ready)

Add `deploy/nginx-kinjo.conf` to the kinjo.world `server` block, then `nginx -t && systemctl reload nginx`.
Check from anywhere: `curl -i https://kinjo.world/healthz` → `204`.

Roll back: remove those locations and reload nginx; the old backend is untouched.
