-- Throwaway test DB only. 200 accounts with sessions tok_1..tok_200 and no profile:
-- each simulated phone saves its card (PUT /api/profile) when it starts, like onboarding.
INSERT INTO users (id, linkedin_sub) SELECT 't_'||i, 'ts'||i FROM generate_series(1, 200) i ON CONFLICT DO NOTHING;
INSERT INTO sessions (token, uid) SELECT encode(sha256(convert_to('tok_'||i, 'UTF8')), 'hex'), 't_'||i FROM generate_series(1, 200) i ON CONFLICT DO NOTHING;
