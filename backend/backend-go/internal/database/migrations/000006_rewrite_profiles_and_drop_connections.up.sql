-- 000006_rewrite_profiles_and_drop_connections.up.sql

ALTER TABLE profiles ADD COLUMN IF NOT EXISTS bio TEXT NOT NULL DEFAULT '';
ALTER TABLE profiles DROP COLUMN IF EXISTS about;
ALTER TABLE profiles DROP COLUMN IF EXISTS ai_summary_about;
ALTER TABLE profiles DROP COLUMN IF EXISTS headline_about;

DROP TABLE IF EXISTS connections;
