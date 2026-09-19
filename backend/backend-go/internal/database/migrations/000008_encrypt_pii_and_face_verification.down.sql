-- 000008_encrypt_pii_and_face_verification.down.sql
-- Reverses the encrypted PII migration (data in new columns is lost;
-- legacy plaintext columns are restored to primary state).

DROP INDEX IF EXISTS idx_profiles_face_verified;
DROP INDEX IF EXISTS idx_profiles_geo_cells;
DROP INDEX IF EXISTS idx_profiles_email_hash;

ALTER TABLE profiles DROP COLUMN IF EXISTS face_scan_photo_url;
ALTER TABLE profiles DROP COLUMN IF EXISTS face_verified_at;
ALTER TABLE profiles DROP COLUMN IF EXISTS geo_lon_cell;
ALTER TABLE profiles DROP COLUMN IF EXISTS geo_lat_cell;
ALTER TABLE profiles DROP COLUMN IF EXISTS lon_enc;
ALTER TABLE profiles DROP COLUMN IF EXISTS lat_enc;
ALTER TABLE profiles DROP COLUMN IF EXISTS bio_enc;
ALTER TABLE profiles DROP COLUMN IF EXISTS name_enc;
ALTER TABLE profiles DROP COLUMN IF EXISTS email_enc;
ALTER TABLE profiles DROP COLUMN IF EXISTS email_hash;
