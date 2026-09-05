-- Keep dialogue separate from system/tool context without rewriting legacy archives.
ALTER TABLE conversation_review_turns
    ADD COLUMN IF NOT EXISTS request_kind VARCHAR(32) NOT NULL DEFAULT 'unknown',
    ADD COLUMN IF NOT EXISTS request_details JSONB NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN IF NOT EXISTS history_key VARCHAR(64) NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_conversation_review_turns_kind_session
    ON conversation_review_turns(request_kind, session_id);

CREATE INDEX IF NOT EXISTS idx_conversation_review_turns_history
    ON conversation_review_turns(history_key, captured_at) WHERE history_key <> '';
CREATE INDEX IF NOT EXISTS idx_conversation_review_turns_response
    ON conversation_review_turns(upstream_response_id) WHERE upstream_response_id <> '';
