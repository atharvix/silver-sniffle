-- 000009_fix_auth_constraints.up.sql
-- Fix legacy plaintext NOT NULL constraints on auth tables

-- 1. verification_tokens: allow inserting without legacy plaintext email and store email_enc
ALTER TABLE verification_tokens ADD COLUMN IF NOT EXISTS email_enc TEXT;
ALTER TABLE verification_tokens ALTER COLUMN email DROP NOT NULL;
ALTER TABLE verification_tokens ALTER COLUMN email SET DEFAULT '';

-- 2. otp_codes: allow inserting without legacy plaintext email
ALTER TABLE otp_codes ALTER COLUMN email DROP NOT NULL;
ALTER TABLE otp_codes ALTER COLUMN email SET DEFAULT '';

-- 3. profiles: allow inserting with encrypted/hashed fields only
ALTER TABLE profiles ALTER COLUMN email DROP NOT NULL;
ALTER TABLE profiles ALTER COLUMN email SET DEFAULT '';
ALTER TABLE profiles ALTER COLUMN name DROP NOT NULL;
ALTER TABLE profiles ALTER COLUMN name SET DEFAULT '';

-- 4. profiles identity: drop old plaintext email primary key, enforce unique email_hash
ALTER TABLE profiles DROP CONSTRAINT IF EXISTS profiles_pkey;
DROP INDEX IF EXISTS idx_profiles_email_hash;
ALTER TABLE profiles DROP CONSTRAINT IF EXISTS profiles_email_hash_key;
ALTER TABLE profiles ADD CONSTRAINT profiles_email_hash_key UNIQUE (email_hash);
