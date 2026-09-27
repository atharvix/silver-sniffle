-- 000002_prod_spatial_index.up.sql
-- Production spatial indexing and extensions for high-performance discovery

CREATE EXTENSION IF NOT EXISTS cube;
CREATE EXTENSION IF NOT EXISTS earthdistance;

-- Production GiST index for Earth great-circle distance queries
CREATE INDEX IF NOT EXISTS idx_profiles_earth 
ON profiles USING gist (ll_to_earth(latitude, longitude)) 
WHERE face_verified_at IS NOT NULL AND latitude IS NOT NULL AND longitude IS NOT NULL;

-- High-performance composite B-tree index for coordinate bounding-box scans
CREATE INDEX IF NOT EXISTS idx_profiles_discovery 
ON profiles (latitude, longitude) 
WHERE face_verified_at IS NOT NULL AND latitude IS NOT NULL AND longitude IS NOT NULL;
