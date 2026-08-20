-- 000001_init_schema.up.sql

CREATE TABLE IF NOT EXISTS profiles (
    email TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    about TEXT NOT NULL DEFAULT '',
    photo_url TEXT NOT NULL DEFAULT '',
    latitude DOUBLE PRECISION,
    longitude DOUBLE PRECISION,
    last_seen_at TIMESTAMPTZ,
    ai_summary TEXT,
    ai_summary_about TEXT,
    headline TEXT,
    headline_about TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Compound index for fast spatial bounding box + presence queries
CREATE INDEX IF NOT EXISTS idx_profiles_spatial_presence 
ON profiles (latitude, longitude, last_seen_at) 
WHERE latitude IS NOT NULL AND longitude IS NOT NULL AND last_seen_at IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_profiles_last_seen_at 
ON profiles (last_seen_at);

CREATE TABLE IF NOT EXISTS verification_tokens (
    token_hash TEXT PRIMARY KEY,
    email TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_verification_tokens_email 
ON verification_tokens (email);

CREATE INDEX IF NOT EXISTS idx_verification_tokens_expires_at 
ON verification_tokens (expires_at);

CREATE TABLE IF NOT EXISTS otp_codes (
    email TEXT PRIMARY KEY,
    otp_hash TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    attempts INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_otp_codes_expires_at 
ON otp_codes (expires_at);

CREATE TABLE IF NOT EXISTS verified_emails (
    email TEXT PRIMARY KEY,
    expires_at TIMESTAMPTZ NOT NULL
);
