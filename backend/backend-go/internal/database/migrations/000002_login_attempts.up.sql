-- 000002_login_attempts.up.sql
-- Persistent record of failed password sign-in attempts, used to lock an
-- account for a cooldown period. Keeping this in Postgres (instead of process
-- memory) means the lockout survives restarts and is shared across replicas.
CREATE TABLE IF NOT EXISTS login_attempts (
    email TEXT PRIMARY KEY,
    failed_count INTEGER NOT NULL DEFAULT 0,
    locked_until TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_login_attempts_locked_until
ON login_attempts (locked_until)
WHERE locked_until IS NOT NULL;
