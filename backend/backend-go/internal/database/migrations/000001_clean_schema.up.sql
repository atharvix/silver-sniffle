-- 000001_clean_schema.up.sql
-- Unified Clean Database Schema for Kinjo

-- 1. Profiles (User accounts, bio, real-time location, and verification status)
CREATE TABLE IF NOT EXISTS profiles (
    email TEXT PRIMARY KEY,
    name TEXT NOT NULL DEFAULT '',
    bio TEXT NOT NULL DEFAULT '',
    photo_url TEXT NOT NULL DEFAULT '',
    latitude DOUBLE PRECISION,
    longitude DOUBLE PRECISION,
    last_seen_at TIMESTAMPTZ,
    password_hash TEXT,
    email_verified BOOLEAN NOT NULL DEFAULT FALSE,
    face_verified_at TIMESTAMPTZ,
    face_scan_photo_url TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Fast spatial bounding box & presence index
CREATE INDEX IF NOT EXISTS idx_profiles_spatial 
ON profiles (latitude, longitude, last_seen_at) 
WHERE latitude IS NOT NULL AND longitude IS NOT NULL AND last_seen_at IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_profiles_last_seen_at 
ON profiles (last_seen_at);

CREATE INDEX IF NOT EXISTS idx_profiles_face_verified 
ON profiles (face_verified_at) 
WHERE face_verified_at IS NOT NULL;

-- 2. Verification Tokens (Session tokens hashed with SHA-256)
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

-- 3. OTP Codes (Hashed with SHA-256)
CREATE TABLE IF NOT EXISTS otp_codes (
    email TEXT PRIMARY KEY,
    otp_hash TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    attempts INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_otp_codes_expires_at 
ON otp_codes (expires_at);

-- 4. Verified Emails (Welcome email tracking)
CREATE TABLE IF NOT EXISTS verified_emails (
    email TEXT PRIMARY KEY,
    expires_at TIMESTAMPTZ NOT NULL
);

-- 5. Device Tokens (Firebase Cloud Messaging push tokens)
CREATE TABLE IF NOT EXISTS device_tokens (
    device_token TEXT PRIMARY KEY,
    email TEXT NOT NULL,
    platform TEXT NOT NULL DEFAULT 'android',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_device_tokens_email 
ON device_tokens (email);
