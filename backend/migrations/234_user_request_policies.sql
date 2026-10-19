-- Add per-user model restrictions and rolling token limits.
ALTER TABLE users
    ADD COLUMN IF NOT EXISTS token_limit_1d BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS token_limit_7d BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS token_limit_30d BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS model_restrictions JSONB NOT NULL DEFAULT '[]'::jsonb;

ALTER TABLE users
    DROP CONSTRAINT IF EXISTS users_token_limit_1d_nonnegative,
    DROP CONSTRAINT IF EXISTS users_token_limit_7d_nonnegative,
    DROP CONSTRAINT IF EXISTS users_token_limit_30d_nonnegative;

ALTER TABLE users
    ADD CONSTRAINT users_token_limit_1d_nonnegative CHECK (token_limit_1d >= 0),
    ADD CONSTRAINT users_token_limit_7d_nonnegative CHECK (token_limit_7d >= 0),
    ADD CONSTRAINT users_token_limit_30d_nonnegative CHECK (token_limit_30d >= 0);
