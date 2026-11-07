#!/usr/bin/env bash
# Builds the working-tree backend, starts 3 instances (one per variant) on one throwaway Postgres, runs sim.mjs against each at once.
set -euo pipefail
S=$(cd "$(dirname "$0")" && pwd); ROOT=$(cd "$S/../.." && pwd)
W=${WORK:-$(mktemp -d)}; rm -rf "$W"; mkdir -p "$W"
PIDS=()
trap 'for p in "${PIDS[@]}"; do kill $p 2>/dev/null || true; done; pg_ctl -D "$W/pg" stop -m fast >/dev/null 2>&1 || true' EXIT
(cd $ROOT/backend && go build -o "$W/api" .)
PGPORT=9311
initdb -D "$W/pg" -U kinjo -A trust >/dev/null
pg_ctl -D "$W/pg" -o "-p $PGPORT -k '' -c listen_addresses=127.0.0.1 -c max_connections=200" -l "$W/pg.log" start >/dev/null
sleep 2
i=0
for V in ${VARIANTS:-A B C D}; do
  PORT=$((8311 + i)); i=$((i+1))
  createdb -h 127.0.0.1 -p $PGPORT -U kinjo k$V
  cat > "$W/$V.env" <<E
PORT=$PORT
DATABASE_URL=postgres://kinjo@127.0.0.1:$PGPORT/k$V?sslmode=disable
LINKEDIN_CLIENT_ID=x
LINKEDIN_CLIENT_SECRET=x
LINKEDIN_REDIRECT_URL=http://127.0.0.1:$PORT/auth/linkedin/callback
E
  (set -a; . "$W/$V.env"; set +a; exec env -i PATH="$PATH" HOME="$HOME" PORT=$PORT DATABASE_URL="$DATABASE_URL" LINKEDIN_CLIENT_ID=x LINKEDIN_CLIENT_SECRET="$LINKEDIN_CLIENT_SECRET" LINKEDIN_REDIRECT_URL="$LINKEDIN_REDIRECT_URL" "$W/api" >"$W/api-$V.log" 2>&1) &
  PIDS+=($!)
  for _ in $(seq 50); do curl -sf "http://127.0.0.1:$PORT/healthz" >/dev/null && break; sleep 0.2; done
  psql -q -h 127.0.0.1 -p $PGPORT -U kinjo k$V -f "$S/seed.sql"
done
sleep 16   # bootGrace
i=0; RUNS=()
for V in ${VARIANTS:-A B C D}; do
  PORT=$((8311 + i)); i=$((i+1))
  VARIANT=$V API=http://127.0.0.1:$PORT node "$S/sim.mjs" > "$W/res-$V.json" 2> "$W/sim-$V.err" &
  RUNS+=($!)
done
for r in "${RUNS[@]}"; do wait $r || true; done
for V in ${VARIANTS:-A B C D}; do echo "== $V: first_match lines: $(grep -c first_match "$W/api-$V.log" || true), 5xx: $(grep -c '"status":5' "$W/api-$V.log" || true)"; done
node "$S/table.mjs" $(for V in ${VARIANTS:-A B C D}; do echo "$W/res-$V.json"; done)
