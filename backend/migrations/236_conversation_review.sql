-- Optional conversation archive and periodic review for Prompt Audit.
-- Recording and review are disabled by default in prompt_audit_config.

CREATE TABLE IF NOT EXISTS conversation_review_sessions (
    id                       BIGSERIAL PRIMARY KEY,
    conversation_key         VARCHAR(64) NOT NULL UNIQUE,
    external_conversation_id VARCHAR(255) NOT NULL DEFAULT '',
    last_response_id         VARCHAR(255) NOT NULL DEFAULT '',
    user_id                  BIGINT REFERENCES users(id) ON DELETE SET NULL,
    username_snapshot        VARCHAR(255) NOT NULL DEFAULT '',
    user_email_snapshot      VARCHAR(320) NOT NULL DEFAULT '',
    api_key_id               BIGINT REFERENCES api_keys(id) ON DELETE SET NULL,
    api_key_name_snapshot    VARCHAR(255) NOT NULL DEFAULT '',
    group_id                 BIGINT REFERENCES groups(id) ON DELETE SET NULL,
    group_name               VARCHAR(255) NOT NULL DEFAULT '',
    provider                 VARCHAR(64) NOT NULL DEFAULT '',
    protocol                 VARCHAR(64) NOT NULL DEFAULT '',
    model                    VARCHAR(255) NOT NULL DEFAULT '',
    turn_count               INT NOT NULL DEFAULT 0,
    pending_review_count     INT NOT NULL DEFAULT 0,
    flagged_turn_count       INT NOT NULL DEFAULT 0,
    latest_review_decision   VARCHAR(32) NOT NULL DEFAULT '',
    started_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_turn_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_conversation_review_sessions_nonnegative
        CHECK (turn_count >= 0 AND pending_review_count >= 0 AND flagged_turn_count >= 0),
    CONSTRAINT chk_conversation_review_sessions_decision
        CHECK (latest_review_decision IN ('', 'pass', 'flag', 'critical'))
);

CREATE TABLE IF NOT EXISTS conversation_review_turns (
    id                       BIGSERIAL PRIMARY KEY,
    session_id               BIGINT NOT NULL REFERENCES conversation_review_sessions(id) ON DELETE CASCADE,
    request_id               VARCHAR(128) NOT NULL DEFAULT '',
    upstream_response_id     VARCHAR(255) NOT NULL DEFAULT '',
    endpoint                 VARCHAR(128) NOT NULL DEFAULT '',
    protocol                 VARCHAR(64) NOT NULL DEFAULT '',
    model                    VARCHAR(255) NOT NULL DEFAULT '',
    request_transcript       TEXT NOT NULL DEFAULT '',
    model_response           TEXT NOT NULL DEFAULT '',
    request_chars            INT NOT NULL DEFAULT 0,
    response_chars           INT NOT NULL DEFAULT 0,
    request_truncated        BOOLEAN NOT NULL DEFAULT FALSE,
    response_truncated       BOOLEAN NOT NULL DEFAULT FALSE,
    status_code              INT NOT NULL DEFAULT 0,
    review_status            VARCHAR(32) NOT NULL DEFAULT 'pending',
    review_attempts          INT NOT NULL DEFAULT 0,
    review_claim_version     BIGINT NOT NULL DEFAULT 0,
    review_started_at        TIMESTAMPTZ,
    reviewed_at              TIMESTAMPTZ,
    review_decision          VARCHAR(32) NOT NULL DEFAULT '',
    risk_level               VARCHAR(32) NOT NULL DEFAULT '',
    action                   VARCHAR(32) NOT NULL DEFAULT '',
    categories               JSONB NOT NULL DEFAULT '[]'::jsonb,
    matched_scanners         JSONB NOT NULL DEFAULT '[]'::jsonb,
    scanner_scores           JSONB NOT NULL DEFAULT '{}'::jsonb,
    scanner_evidence         JSONB NOT NULL DEFAULT '{}'::jsonb,
    scanner_backend          VARCHAR(64) NOT NULL DEFAULT '',
    scanner_version          VARCHAR(128) NOT NULL DEFAULT '',
    guard_endpoint_id        VARCHAR(128) NOT NULL DEFAULT '',
    review_error_code        VARCHAR(64) NOT NULL DEFAULT '',
    review_error_message     VARCHAR(512) NOT NULL DEFAULT '',
    captured_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_conversation_review_turns_status
        CHECK (review_status IN ('pending', 'processing', 'reviewed', 'failed')),
    CONSTRAINT chk_conversation_review_turns_decision
        CHECK (review_decision IN ('', 'pass', 'flag', 'critical')),
    CONSTRAINT chk_conversation_review_turns_risk
        CHECK (risk_level IN ('', 'low', 'medium', 'high', 'critical')),
    CONSTRAINT chk_conversation_review_turns_action
        CHECK (action IN ('', 'Allow', 'Warn', 'Block')),
    CONSTRAINT chk_conversation_review_turns_nonnegative
        CHECK (request_chars >= 0 AND response_chars >= 0 AND review_attempts >= 0 AND review_claim_version >= 0),
    CONSTRAINT chk_conversation_review_turns_json
        CHECK (jsonb_typeof(categories) = 'array' AND jsonb_typeof(matched_scanners) = 'array'
            AND jsonb_typeof(scanner_scores) = 'object' AND jsonb_typeof(scanner_evidence) = 'object')
);

CREATE TABLE IF NOT EXISTS conversation_review_runs (
    id                   BIGSERIAL PRIMARY KEY,
    trigger_type         VARCHAR(32) NOT NULL DEFAULT 'scheduled',
    status               VARCHAR(32) NOT NULL DEFAULT 'queued',
    requested_by         BIGINT REFERENCES users(id) ON DELETE SET NULL,
    config_version       BIGINT NOT NULL DEFAULT 1,
    batch_size           INT NOT NULL DEFAULT 0,
    processed_count      INT NOT NULL DEFAULT 0,
    flagged_count        INT NOT NULL DEFAULT 0,
    failed_count         INT NOT NULL DEFAULT 0,
    started_at           TIMESTAMPTZ,
    completed_at         TIMESTAMPTZ,
    last_error_code      VARCHAR(64) NOT NULL DEFAULT '',
    last_error_message   VARCHAR(512) NOT NULL DEFAULT '',
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_conversation_review_runs_trigger
        CHECK (trigger_type IN ('scheduled', 'manual')),
    CONSTRAINT chk_conversation_review_runs_status
        CHECK (status IN ('queued', 'processing', 'completed', 'failed')),
    CONSTRAINT chk_conversation_review_runs_nonnegative
        CHECK (config_version >= 1 AND batch_size >= 0 AND processed_count >= 0
            AND flagged_count >= 0 AND failed_count >= 0)
);

CREATE INDEX IF NOT EXISTS idx_conversation_review_sessions_last_turn
    ON conversation_review_sessions(last_turn_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_conversation_review_sessions_user
    ON conversation_review_sessions(user_id, last_turn_at DESC);
CREATE INDEX IF NOT EXISTS idx_conversation_review_sessions_group
    ON conversation_review_sessions(group_id, last_turn_at DESC);
CREATE INDEX IF NOT EXISTS idx_conversation_review_sessions_external
    ON conversation_review_sessions(external_conversation_id);
CREATE INDEX IF NOT EXISTS idx_conversation_review_sessions_last_response
    ON conversation_review_sessions(last_response_id) WHERE last_response_id <> '';

CREATE UNIQUE INDEX IF NOT EXISTS idx_conversation_review_turns_request
    ON conversation_review_turns(request_id) WHERE request_id <> '';
CREATE INDEX IF NOT EXISTS idx_conversation_review_turns_session
    ON conversation_review_turns(session_id, captured_at, id);
CREATE INDEX IF NOT EXISTS idx_conversation_review_turns_review_queue
    ON conversation_review_turns(review_status, captured_at, id);
CREATE INDEX IF NOT EXISTS idx_conversation_review_turns_decision
    ON conversation_review_turns(review_decision, reviewed_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_conversation_review_runs_queue
    ON conversation_review_runs(status, created_at, id);
CREATE INDEX IF NOT EXISTS idx_conversation_review_runs_created
    ON conversation_review_runs(created_at DESC, id DESC);
