#!/usr/bin/env bash
# Runs discovery.mjs against a backend built from a git ref (or the working tree),
# on its own throwaway Postgres and port. Nothing touches production.
#   bash loadtest/run-discovery.sh HEAD 8101      # committed code
#   bash loadtest/run-discovery.sh WORKTREE 8102  # uncommitted working tree
set -euo pipefail
REF=${1:-WORKTREE} PORT=${2:-8101} PGPORT=$((PORT + 1000))
ROOT=$(cd "$(dirname "$0")/.." && pwd)
W=$(mktemp -d /tmp/kd.XXXX)
trap 'kill $API_PID 2>/dev/null || true; pg_ctl -D "$W/pg" stop -m fast >/dev/null 2>&1 || true; rm -rf "$W"' EXIT
if [ "$REF" = WORKTREE ]; then SRC=$ROOT/backend; else git -C "$ROOT" archive "$REF" backend | tar -x -C "$W"; SRC=$W/backend; fi
(cd "$SRC" && go build -o "$W/api" .)
initdb -D "$W/pg" -U kinjo -A trust >/dev/null
pg_ctl -D "$W/pg" -o "-p $PGPORT -k '' -c listen_addresses=127.0.0.1" -l "$W/pg.log" start >/dev/null
sleep 2; createdb -h 127.0.0.1 -p $PGPORT -U kinjo kd
DB="postgres://kinjo@127.0.0.1:$PGPORT/kd?sslmode=disable"
start_api() {
  env -i PATH="$PATH" HOME="$HOME" PORT=$PORT DATABASE_URL="$DB" LINKEDIN_CLIENT_ID=x LINKEDIN_CLIENT_SECRET=x \
    LINKEDIN_REDIRECT_URL="http://127.0.0.1:$PORT/auth/linkedin/callback" "$W/api" >>"$W/api.log" 2>&1 &
  API_PID=$!
}
start_api
for i in $(seq 50); do curl -sf "http://127.0.0.1:$PORT/healthz" >/dev/null && break; sleep 0.2; done
psql -q -h 127.0.0.1 -p $PGPORT -U kinjo kd -f "$ROOT/loadtest/discovery-seed.sql"
set +e
if [ -n "${RESTART_AT_S:-}" ]; then   # a deploy mid-run: stop the backend (SIGTERM, saves state) and start it again
  ( sleep "$RESTART_AT_S"; kill -TERM $API_PID; wait $API_PID 2>/dev/null; sleep 1; start_api; echo "$API_PID" > "$W/api.pid" ) &
fi
API="http://127.0.0.1:$PORT" node "$ROOT/loadtest/discovery.mjs"; rc=$?
[ -f "$W/api.pid" ] && API_PID=$(cat "$W/api.pid")
echo "--- first_match log lines ($REF):"; grep -o '"msg":"first_match"[^}]*' "$W/api.log" | head -40 || true
exit $rc
