-- Start user token quotas at policy activation instead of charging historical usage.
ALTER TABLE users
    ADD COLUMN IF NOT EXISTS token_quota_started_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP;
