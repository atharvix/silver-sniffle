-- 000008_encrypt_pii_and_face_verification.up.sql
--
-- 1. PII encryption at rest (profiles):
--    - email_hash (HMAC-SHA256, deterministic) becomes the lookup/join key.
--    - email, name, bio stored as AES-256-GCM envelopes ("v1:" prefix).
--    - latitude/longitude moved to encrypted TEXT columns (lat_enc, lon_enc).
--    - geo cells (rounded to 3 decimals ~= 111m) kept as plaintext floats so
--      nearby discovery can use a B-tree index instead of a full scan.
--    - The Go application backfills email_hash (real HMAC) + encrypted values
--      from legacy plaintext columns at startup (idempotent).
-- 2. Face verification gate:
--    - face_verified_at records when the live liveness scan passed.
--    - face_scan_photo_url stores the reference selfie used for matching.
-- 3. One-email-one-account preserved via UNIQUE index on profiles.email_hash.
-- 4. Transient auth tables (otp_codes, verified_emails) switch their primary
--    key to email_hash. Legacy rows in these short-lived tables are wiped
--    (OTPs expire in minutes; verified_emails is only a welcome-email gate).
--    verification_tokens gains a nullable email_hash; legacy rows keep working
--    via their legacy email column until naturally expired.

-- ── Profiles: encrypted / hashed PII columns ─────────────────────────────
ALTER TABLE profiles ADD COLUMN IF NOT EXISTS email_hash TEXT;
ALTER TABLE profiles ADD COLUMN IF NOT EXISTS email_enc TEXT NOT NULL DEFAULT '';
ALTER TABLE profiles ADD COLUMN IF NOT EXISTS name_enc TEXT NOT NULL DEFAULT '';
ALTER TABLE profiles ADD COLUMN IF NOT EXISTS bio_enc TEXT NOT NULL DEFAULT '';
ALTER TABLE profiles ADD COLUMN IF NOT EXISTS lat_enc TEXT;
ALTER TABLE profiles ADD COLUMN IF NOT EXISTS lon_enc TEXT;

-- Coarse geo cells for indexed proximity search (encrypted coords can't be indexed)
ALTER TABLE profiles ADD COLUMN IF NOT EXISTS geo_lat_cell DOUBLE PRECISION;
ALTER TABLE profiles ADD COLUMN IF NOT EXISTS geo_lon_cell DOUBLE PRECISION;

-- Face verification state (server-enforced)
ALTER TABLE profiles ADD COLUMN IF NOT EXISTS face_verified_at TIMESTAMPTZ;
ALTER TABLE profiles ADD COLUMN IF NOT EXISTS face_scan_photo_url TEXT NOT NULL DEFAULT '';

-- Unique identity: one email = one account (partial: backfill fills it in).
CREATE UNIQUE INDEX IF NOT EXISTS idx_profiles_email_hash
ON profiles (email_hash) WHERE email_hash IS NOT NULL;

-- Fast nearby discovery over geo cells (30m radius only needs adjacent cells)
CREATE INDEX IF NOT EXISTS idx_profiles_geo_cells
ON profiles (geo_lat_cell, geo_lon_cell)
WHERE geo_lat_cell IS NOT NULL AND geo_lon_cell IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_profiles_face_verified
ON profiles (face_verified_at)
WHERE face_verified_at IS NOT NULL;

-- ── Transient auth tables: switch keys to email_hash ─────────────────────
ALTER TABLE otp_codes ADD COLUMN IF NOT EXISTS email_hash TEXT;
ALTER TABLE verified_emails ADD COLUMN IF NOT EXISTS email_hash TEXT;
ALTER TABLE verification_tokens ADD COLUMN IF NOT EXISTS email_hash TEXT;
ALTER TABLE device_tokens ADD COLUMN IF NOT EXISTS email_hash TEXT;

-- OTPs are ephemeral: wipe legacy plaintext-keyed rows (users just re-request).
DELETE FROM otp_codes WHERE email_hash IS NULL;

-- verified_emails is only a welcome-email gate; wiping forces nothing.
DELETE FROM verified_emails WHERE email_hash IS NULL;

-- Swap PKs to email_hash
ALTER TABLE otp_codes DROP CONSTRAINT IF EXISTS otp_codes_pkey;
ALTER TABLE otp_codes ADD PRIMARY KEY (email_hash);

ALTER TABLE verified_emails DROP CONSTRAINT IF EXISTS verified_emails_pkey;
ALTER TABLE verified_emails ADD PRIMARY KEY (email_hash);

CREATE INDEX IF NOT EXISTS idx_device_tokens_email_hash ON device_tokens (email_hash);
CREATE INDEX IF NOT EXISTS idx_verification_tokens_email_hash ON verification_tokens (email_hash);

-- Legacy plaintext email columns retained for rollback safety; the
-- application never reads/writes them anymore. A later cleanup migration
-- drops them once the encrypted backfill is confirmed.
COMMENT ON COLUMN profiles.email IS 'LEGACY: plaintext email retained for rollback. App uses email_enc/email_hash only.';
COMMENT ON COLUMN otp_codes.email IS 'LEGACY: retained for rollback. App uses email_hash only.';
