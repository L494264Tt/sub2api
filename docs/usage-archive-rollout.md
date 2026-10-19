# Usage Archive Rollout

The usage archive is independent from Prompt Audit and safety decisions. `usage_logs` remains the billing and usage fact table; the archive sidecar stores optional text and role-separated messages.

## Defaults

- Full request/response content: 7 days.
- Session and turn metadata: 90 days.
- Daily content budget: 2 GiB.
- Capture queue: 2 workers and 2048 pending items.
- Capture failures are best effort and never fail an API request.

## Data handling

System/developer prompts, user messages, assistant responses, and tool messages are stored as separate roles. Repeated prompt snapshots are deduplicated by SHA-256. Binary payloads are represented by request metadata and are not stored as Base64 in PostgreSQL.

Continuity prefers explicit conversation IDs, previous response IDs, upstream response IDs, and provider history keys. Requests without a reliable identity are stored as independent sessions.

## Operations

Migration `246_usage_archive.sql` creates the sidecar tables. The cleanup loop runs every six hours, expires message bodies after the raw retention window, and removes old sessions after the metadata retention window. The admin page is available at `/admin/usage-archive`.

The existing `conversation_review_*` tables remain historical compatibility data. No automatic backfill or deletion is performed by the usage archive rollout.

## Rollback

Set `USAGE_ARCHIVE_ENABLED=false` and restart the service to stop new body capture while preserving existing `usage_logs` and archive rows. The admin API remains read-only for existing archive data. Database migration rollback is intentionally not destructive; cleanup or table removal requires an explicit data-management operation after a separate backup.
