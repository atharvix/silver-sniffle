INSERT INTO users (id, linkedin_sub) SELECT 't_'||i, 'ts'||i FROM generate_series(1, 800) i ON CONFLICT DO NOTHING;
INSERT INTO sessions (token, uid) SELECT encode(sha256(convert_to('tok_'||i, 'UTF8')), 'hex'), 't_'||i FROM generate_series(1, 800) i ON CONFLICT DO NOTHING;
