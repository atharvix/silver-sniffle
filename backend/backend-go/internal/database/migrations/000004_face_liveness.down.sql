-- 000004_face_liveness.down.sql
DROP INDEX IF EXISTS idx_profiles_face_scan_hash;
ALTER TABLE profiles DROP COLUMN IF EXISTS face_scan_hash;
DROP TABLE IF EXISTS face_challenges;
