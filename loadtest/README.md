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
