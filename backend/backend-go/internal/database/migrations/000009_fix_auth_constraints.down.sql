-- 000009_fix_auth_constraints.down.sql
ALTER TABLE verification_tokens ALTER COLUMN email SET NOT NULL;
ALTER TABLE otp_codes ALTER COLUMN email SET NOT NULL;
ALTER TABLE profiles ALTER COLUMN email SET NOT NULL;
ALTER TABLE profiles ALTER COLUMN name SET NOT NULL;
