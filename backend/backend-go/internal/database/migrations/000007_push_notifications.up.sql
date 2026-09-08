-- 000007_push_notifications.up.sql

CREATE TABLE IF NOT EXISTS device_tokens (
    email TEXT NOT NULL,
    token TEXT NOT NULL,
    platform TEXT NOT NULL DEFAULT 'android',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (email, token)
);

CREATE INDEX IF NOT EXISTS idx_device_tokens_email ON device_tokens(LOWER(email));
