ALTER TABLE profiles ADD COLUMN IF NOT EXISTS social_links JSONB NOT NULL DEFAULT '{}'::jsonb;

CREATE TABLE IF NOT EXISTS connections (
    requester_email TEXT NOT NULL REFERENCES profiles(email) ON DELETE CASCADE,
    recipient_email TEXT NOT NULL REFERENCES profiles(email) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (requester_email, recipient_email),
    CHECK (requester_email <> recipient_email)
);

CREATE INDEX IF NOT EXISTS idx_connections_recipient ON connections (recipient_email, created_at DESC);