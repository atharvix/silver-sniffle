# Deploying the new server to kinjo.world

The new server runs beside the old backend until you switch nginx. Switching nginx is the moment old app builds stop working.

## One time, on the VPS (as root)

```bash
useradd --system --no-create-home kinjo
# A NEW database: the old backend's database has a different `users` table and must not be shared.
sudo -u postgres createuser kinjo_v2
sudo -u postgres createdb -O kinjo_v2 kinjo_v2
install -d -m 0750 -o root -g kinjo /etc/kinjo
cat > /etc/kinjo/server.env <<'EOF'
DATABASE_URL=postgres://kinjo_v2@/kinjo_v2?host=/var/run/postgresql
ADDR=127.0.0.1:8080
PUBLIC_URL=https://kinjo.world
EOF
chmod 0640 /etc/kinjo/server.env
```

Check port 8080 is free first (`ss -ltnp | grep 8080`). If the old backend uses it, set `ADDR` to another port and use that port in `nginx-kinjo.conf`.
The database role needs local peer/trust auth for the `kinjo` system user, or add a password to `DATABASE_URL` in `server.env` (never on a command line).

## Every release (from this PC, in `server/`)

```bash
bash deploy/deploy.sh root@kinjo.world
```

It builds a static Linux binary, copies it, installs `kinjo-v2.service`, restarts it and prints `health 204`.

## Switch traffic (once, when you're ready)

Add `deploy/nginx-kinjo.conf` to the kinjo.world `server` block, then `nginx -t && systemctl reload nginx`.
Check from anywhere: `curl -i https://kinjo.world/healthz` → `204`.

Roll back: remove those locations and reload nginx; the old backend is untouched.
