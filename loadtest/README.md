# Load test (staging only)

Seed a staging database with N users before starting the backend. The plaintext
tokens below are hashed in place by the backend's boot migration.

```sql
INSERT INTO users (id, linkedin_sub) SELECT 'li_'||i, 's'||i FROM generate_series(1, 3000) i;
INSERT INTO profiles (uid, name, role, look, photo)
  SELECT 'li_'||i, 'User '||i, 'Engineer', 'co-founders', 'data:image/jpeg;base64,AAAA' FROM generate_series(1, 3000) i;
INSERT INTO sessions (token, uid) SELECT 'tok_'||i, 'li_'||i FROM generate_series(1, 3000) i;
```

Then: `API=https://staging-host USERS=3000 DENSE=200 node loadtest/presence.mjs`
while watching `top`, `pg_stat_activity`, and the backend's JSON logs.

Targets for 50k registered (~5k online): REST p95 < 200 ms, p99 < 500 ms, error
rate < 0.1%, CPU < 60%, DB connections < 80% of the pool, RSS flat over an hour.

# Discovery test (local, throwaway DB)

`bash loadtest/run-discovery.sh <git-ref|WORKTREE> <port>` builds that backend, starts it
on a throwaway Postgres, and runs `discovery.mjs`: simulated phones sending what the
APK's native service sends (fixes, Bluetooth token, sightings) plus the app socket.
Scenarios run at once in separate spots: outdoor and indoor discovery of a new user
next to idle ones, GPS-only indoors (informational), a phone 80 m away heard through a
wall (must never match), walk out and back, 40 s signal loss, and 3 min of idle phones
(no card may drop). Compare builds by running two refs on two ports.
