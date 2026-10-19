-- Independent full usage archive. This stores content beside usage_logs and is
-- intentionally independent from Prompt Audit / security audit decisions.

CREATE TABLE IF NOT EXISTS usage_archive_sessions (
    id BIGSERIAL PRIMARY KEY,
    session_key VARCHAR(128) NOT NULL UNIQUE,
    user_id BIGINT,
    api_key_id BIGINT,
    group_id BIGINT,
    provider VARCHAR(64) NOT NULL DEFAULT '',
    model VARCHAR(255) NOT NULL DEFAULT '',
    first_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    turn_count INT NOT NULL DEFAULT 0,
    content_bytes BIGINT NOT NULL DEFAULT 0,
    continuity_confidence VARCHAR(16) NOT NULL DEFAULT 'none',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS usage_archive_turns (
    id BIGSERIAL PRIMARY KEY,
    session_id BIGINT NOT NULL REFERENCES usage_archive_sessions(id) ON DELETE CASCADE,
    usage_log_request_id VARCHAR(128) NOT NULL DEFAULT '',
    request_id VARCHAR(128) NOT NULL DEFAULT '',
    parent_turn_id BIGINT REFERENCES usage_archive_turns(id) ON DELETE SET NULL,
    sequence_no INT NOT NULL DEFAULT 0,
    conversation_id VARCHAR(255) NOT NULL DEFAULT '',
    previous_response_id VARCHAR(255) NOT NULL DEFAULT '',
    upstream_response_id VARCHAR(255) NOT NULL DEFAULT '',
    history_key VARCHAR(128) NOT NULL DEFAULT '',
    endpoint VARCHAR(256) NOT NULL DEFAULT '',
    protocol VARCHAR(64) NOT NULL DEFAULT '',
    provider VARCHAR(64) NOT NULL DEFAULT '',
    model VARCHAR(255) NOT NULL DEFAULT '',
    status_code INT NOT NULL DEFAULT 0,
    duration_ms INT,
    first_token_ms INT,
    request_bytes INT NOT NULL DEFAULT 0,
    response_bytes INT NOT NULL DEFAULT 0,
    request_truncated BOOLEAN NOT NULL DEFAULT FALSE,
    response_truncated BOOLEAN NOT NULL DEFAULT FALSE,
    system_prompt_hashes JSONB NOT NULL DEFAULT '[]'::jsonb,
    developer_prompt_hashes JSONB NOT NULL DEFAULT '[]'::jsonb,
    continuity_source VARCHAR(32) NOT NULL DEFAULT 'independent',
    continuity_confidence VARCHAR(16) NOT NULL DEFAULT 'none',
    captured_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS usage_archive_messages (
    id BIGSERIAL PRIMARY KEY,
    turn_id BIGINT NOT NULL REFERENCES usage_archive_turns(id) ON DELETE CASCADE,
    role VARCHAR(16) NOT NULL DEFAULT 'unknown',
    source VARCHAR(32) NOT NULL DEFAULT 'client_request',
    ordinal INT NOT NULL DEFAULT 0,
    content TEXT NOT NULL DEFAULT '',
    content_hash VARCHAR(64) NOT NULL DEFAULT '',
    content_bytes INT NOT NULL DEFAULT 0,
    content_runes INT NOT NULL DEFAULT 0,
    truncated BOOLEAN NOT NULL DEFAULT FALSE,
    content_expired BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS usage_archive_prompt_snapshots (
    id BIGSERIAL PRIMARY KEY,
    role VARCHAR(16) NOT NULL,
    content_hash VARCHAR(64) NOT NULL,
    content TEXT NOT NULL DEFAULT '',
    content_bytes INT NOT NULL DEFAULT 0,
    content_runes INT NOT NULL DEFAULT 0,
    truncated BOOLEAN NOT NULL DEFAULT FALSE,
    content_expired BOOLEAN NOT NULL DEFAULT FALSE,
    first_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (role, content_hash)
);

CREATE INDEX IF NOT EXISTS idx_usage_archive_sessions_last_seen ON usage_archive_sessions(last_seen_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_usage_archive_sessions_user_time ON usage_archive_sessions(user_id, last_seen_at DESC);
CREATE INDEX IF NOT EXISTS idx_usage_archive_sessions_api_key_time ON usage_archive_sessions(api_key_id, last_seen_at DESC);
CREATE INDEX IF NOT EXISTS idx_usage_archive_turns_session_sequence ON usage_archive_turns(session_id, sequence_no, id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_usage_archive_turns_usage_log_request ON usage_archive_turns(usage_log_request_id) WHERE usage_log_request_id <> '';
CREATE INDEX IF NOT EXISTS idx_usage_archive_turns_captured_at ON usage_archive_turns(captured_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_usage_archive_turns_model_time ON usage_archive_turns(model, captured_at DESC);
CREATE INDEX IF NOT EXISTS idx_usage_archive_messages_turn_ordinal ON usage_archive_messages(turn_id, ordinal, id);
CREATE INDEX IF NOT EXISTS idx_usage_archive_messages_role ON usage_archive_messages(role, created_at DESC);
