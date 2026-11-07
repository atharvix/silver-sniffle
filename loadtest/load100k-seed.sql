-- 100,000 load-test accounts for backend/cmd/loadgen: users lt_1..lt_100000, session tokens
-- "lt_<i>" (stored hashed, like real ones), complete profiles. STAGING DATABASE ONLY.
INSERT INTO users (id, linkedin_sub) SELECT 'lt_'||i, 'lts'||i FROM generate_series(1, 100000) i ON CONFLICT DO NOTHING;
INSERT INTO sessions (token, uid) SELECT encode(sha256(convert_to('lt_'||i, 'UTF8')), 'hex'), 'lt_'||i FROM generate_series(1, 100000) i ON CONFLICT DO NOTHING;
INSERT INTO profiles (uid, name, role, look, photo) SELECT 'lt_'||i, 'Load '||i, 'Tester', 'load test', 'data:image/jpeg;base64,/9j/4AAQ' FROM generate_series(1, 100000) i ON CONFLICT DO NOTHING;
