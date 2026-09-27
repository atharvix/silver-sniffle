-- 000004_face_liveness.up.sql
-- Server-side face-scan hardening.
--
-- The client runs the liveness challenge (motion/centering) with the camera, and
-- the server cannot re-run that without a face model. What the server *can*
-- enforce, and previously did not, is:
--   1. freshness — the scan must be bound to a one-shot challenge it issued, so
--      a scan captured in an earlier session cannot be replayed;
--   2. uniqueness — one captured selfie can verify exactly one account, ever.

CREATE TABLE IF NOT EXISTS face_challenges (
    email          TEXT PRIMARY KEY,
    challenge_hash TEXT NOT NULL,
    expires_at     TIMESTAMPTZ NOT NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Expired challenges are dead weight; the reader treats them as absent anyway.
CREATE INDEX IF NOT EXISTS idx_face_challenges_expires_at
ON face_challenges (expires_at);

ALTER TABLE profiles ADD COLUMN IF NOT EXISTS face_scan_hash TEXT;

CREATE UNIQUE INDEX IF NOT EXISTS idx_profiles_face_scan_hash
ON profiles (face_scan_hash)
WHERE face_scan_hash IS NOT NULL;
