ALTER TABLE profiles ADD COLUMN IF NOT EXISTS password_hash TEXT;
ALTER TABLE profiles ADD COLUMN IF NOT EXISTS email_verified BOOLEAN NOT NULL DEFAULT FALSE;

INSERT INTO profiles (email, name, password_hash, email_verified)
SELECT pa.email, split_part(pa.email, '@', 1), pa.password_hash, pa.email_verified
FROM password_accounts pa
ON CONFLICT (email) DO UPDATE SET
    password_hash = EXCLUDED.password_hash,
    email_verified = EXCLUDED.email_verified;

DROP TABLE IF EXISTS password_accounts;