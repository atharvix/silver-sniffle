#!/usr/bin/env bash
# Builds the Linux server and installs it on the VPS. Run from server/:  bash deploy/deploy.sh root@kinjo.world
# First time only, on the VPS (see deploy/README.md): the kinjo user, the kinjo_v2 database and /etc/kinjo/server.env.
set -euo pipefail

host="${1:?usage: deploy.sh user@host}"

GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o build/kinjo-server .

scp build/kinjo-server deploy/kinjo.service "$host:/tmp/"
ssh "$host" 'set -e
  install -d -o root -g root /opt/kinjo
  install -m 0755 /tmp/kinjo-server /opt/kinjo/kinjo-server
  install -m 0644 /tmp/kinjo.service /etc/systemd/system/kinjo-v2.service
  systemctl daemon-reload
  systemctl enable --now kinjo-v2
  systemctl restart kinjo-v2
  sleep 2
  curl -fsS -o /dev/null -w "health %{http_code}\n" http://127.0.0.1:8001/healthz'
