# 全量使用归档 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task with verification checkpoints.

**Goal:** Add an independent all-usage archive that separates content roles, preserves conversation continuity, supports admin inspection, and controls PostgreSQL growth.

**Architecture:** Keep `usage_logs` as the compact billing fact table. Add an independent `usagearchive` package with sidecar session, turn, message, and prompt snapshot tables; capture metadata asynchronously at the gateway boundary and expose read-only admin APIs/UI. The existing Prompt Audit conversation tables remain historical and are not the new write path.

**Tech Stack:** Go, Gin, PostgreSQL migrations, Vue 3, TypeScript, Vitest, Go tests.

**Spec:** `docs/superpowers/specs/2026-10-19-all-usage-archive-design.md`

## Global Constraints

- Do not add large request/response columns to `usage_logs`.
- Do not make archive capture part of safety blocking or prompt audit decisions.
- Preserve `usage_logs` billing and analytics behavior.
- Default full-text retention is 7 days; metadata retention is 90 days.
- Binary content is metadata-only; no Base64 payloads in PostgreSQL.
- Capture failures must not fail API requests.
- No full-text search index is added in the first implementation.

---

### Task 1: Archive schema and compact domain types

**Files:**
- Create: `backend/migrations/246_usage_archive.sql`
- Create: `backend/internal/usagearchive/types.go`
- Create: `backend/internal/usagearchive/retention.go`
- Test: `backend/internal/usagearchive/retention_test.go`

**Interfaces:**
- Produces `SessionKey`, `ArchiveTurn`, `ArchiveMessage`, `PromptSnapshot`, `ArchiveRetentionPolicy`, and `ArchiveRecorder` types for later tasks.

- [ ] Write tests for retention cutoff calculation, role validation, and raw-content budget decisions.
- [ ] Run `go test ./internal/usagearchive -run 'Retention|Role'` and confirm the new package fails because the types do not exist.
- [ ] Add tables `usage_archive_sessions`, `usage_archive_turns`, `usage_archive_messages`, and `usage_archive_prompt_snapshots` with time/user/API key/session indexes and a unique `(usage_log_request_id)` turn link.
- [ ] Add retention policy defaults and a deterministic `ShouldStoreBody(size, dailyBudget, currentBytes)` helper.
- [ ] Run the package tests and a migration SQL syntax check against the existing migration test harness.
- [ ] Commit `feat: add usage archive schema and types`.

### Task 2: Protocol role extraction and continuity resolver

**Files:**
- Create: `backend/internal/usagearchive/extractor.go`
- Create: `backend/internal/usagearchive/continuity.go`
- Test: `backend/internal/usagearchive/extractor_test.go`
- Test: `backend/internal/usagearchive/continuity_test.go`

**Interfaces:**
- `ExtractMessages(protocol string, body []byte) ([]ArchiveMessage, error)`
- `ResolveContinuity(input ContinuityInput) ContinuityResult`

- [ ] Add failing cases for OpenAI messages/responses, Anthropic messages, Gemini contents/systemInstruction, tool calls, developer messages, and invalid JSON.
- [ ] Add failing continuity cases for explicit conversation ID, previous response ID, history key, and ambiguous fallback.
- [ ] Run the two test files and verify failures are caused by missing extractors/resolver.
- [ ] Implement role-aware extraction with source, ordinal, content hash, rune/byte length, and truncation metadata.
- [ ] Implement continuity precedence and confidence values; ambiguous inputs create a new session decision.
- [ ] Run the tests and add edge cases for repeated full-history requests so duplicate messages are marked as context rather than new user turns.
- [ ] Commit `feat: classify usage archive messages and continuity`.

### Task 3: PostgreSQL repository and async recorder

**Files:**
- Create: `backend/internal/usagearchive/repository.go`
- Create: `backend/internal/usagearchive/recorder.go`
- Create: `backend/internal/usagearchive/repository_test.go`
- Modify: `backend/internal/service/wire.go`
- Modify: `backend/internal/server/routes/gateway.go`

**Interfaces:**
- `NewPostgresRepository(db *sql.DB) *Repository`
- `NewRecorder(repo Repository, policy Policy) *Recorder`
- `Recorder.Enqueue(UsageCapture)`
- `Recorder.Stop(context.Context) error`

- [ ] Add repository tests for upsert session, append turn, prompt snapshot deduplication, message insertion, and idempotent request IDs.
- [ ] Add recorder tests proving repository errors are swallowed and queue shutdown drains bounded work.
- [ ] Implement inserts in one transaction per capture, with request body storage gated by retention budget.
- [ ] Link a turn to `usage_logs` by request ID when available and preserve metadata even when body storage is denied.
- [ ] Add a bounded async queue and worker count from config; no capture operation may block the API handler beyond enqueue.
- [ ] Wire the recorder at the gateway boundary for HTTP/SSE/WebSocket-compatible request metadata, using the existing response capture interface only as a transport hook.
- [ ] Run repository and recorder tests plus gateway route tests.
- [ ] Commit `feat: record all usage content asynchronously`.

### Task 4: Retention, storage metrics, and cleanup

**Files:**
- Create: `backend/migrations/247_usage_archive_cleanup.sql`
- Create: `backend/internal/usagearchive/cleanup.go`
- Create: `backend/internal/usagearchive/cleanup_test.go`
- Modify: `backend/internal/config/config.go`
- Modify: `backend/internal/service/wire.go`

**Interfaces:**
- `Cleanup(ctx context.Context, before time.Time, batchSize int) (CleanupResult, error)`
- `StorageStats(ctx context.Context) (StorageStats, error)`

- [ ] Test raw-body deletion vs metadata retention and batched cleanup limits.
- [ ] Add config fields for full-text retention days, metadata retention days, daily byte budget, queue capacity, and batch size with defaults 7/90 days.
- [ ] Add indexes/partition-safe cleanup queries and a storage stats query for table bytes, content bytes, message counts, and last cleanup time.
- [ ] Schedule cleanup in the existing service lifecycle without touching billing logs.
- [ ] Run cleanup tests and migration checks.
- [ ] Commit `feat: add usage archive retention controls`.

### Task 5: Admin query API

**Files:**
- Create: `backend/internal/handler/admin/usage_archive_handler.go`
- Create: `backend/internal/usagearchive/query.go`
- Create: `backend/internal/usagearchive/query_test.go`
- Modify: `backend/internal/server/routes/admin.go`

**Interfaces:**
- `GET /api/v1/admin/usage-archive/sessions`
- `GET /api/v1/admin/usage-archive/sessions/:id`
- `GET /api/v1/admin/usage-archive/stats`
- `DELETE /api/v1/admin/usage-archive/sessions/:id/content`

- [ ] Test filters for user, API key, group, provider, model, request ID, session ID, time range, status, and role/content availability.
- [ ] Test detail ordering by sequence and message ordinal, including continuity confidence.
- [ ] Implement paginated session listing with bounded page size and no body text in list responses.
- [ ] Implement detail responses with separate arrays for user content, prompt snapshots, assistant/tool messages, and usage metadata.
- [ ] Implement aggregate stats using metadata only; never scan all content for dashboard counts.
- [ ] Implement explicit per-session content deletion that leaves usage metadata and billing records intact.
- [ ] Run handler/query tests and API contract tests.
- [ ] Commit `feat: expose usage archive admin APIs`.

### Task 6: Admin UI for full usage records

**Files:**
- Create: `frontend/src/features/usage-archive/api.ts`
- Create: `frontend/src/features/usage-archive/types.ts`
- Create: `frontend/src/features/usage-archive/UsageArchiveView.vue`
- Create: `frontend/src/features/usage-archive/components/UsageArchiveFilters.vue`
- Create: `frontend/src/features/usage-archive/components/UsageSessionDetail.vue`
- Create: `frontend/src/features/usage-archive/__tests__/UsageArchiveView.spec.ts`
- Modify: `frontend/src/router/index.ts`
- Modify: `frontend/src/i18n/locales/zh/admin/overview.ts`
- Modify: `frontend/src/i18n/locales/en/admin/overview.ts`

- [ ] Test session filters, pagination, role-separated panels, chronological turn rendering, and content-delete confirmation.
- [ ] Add the route under admin navigation as a separate “Usage Archive” page, not under Prompt Audit.
- [ ] Build a metadata-first table; load full content only after selecting a session/turn.
- [ ] Render user messages, system prompts, developer prompts, assistant replies, and tool calls in separate visual sections with continuity markers.
- [ ] Add stats for requests, tokens, cost, latency, error rate, content bytes, and retained sessions.
- [ ] Add clear truncation/content-expired states.
- [ ] Run focused Vitest tests, `pnpm --dir frontend typecheck`, and production build.
- [ ] Commit `feat: add usage archive admin UI`.

### Task 7: Historical compatibility and rollout controls

**Files:**
- Create: `backend/internal/usagearchive/backfill.go`
- Create: `backend/internal/usagearchive/backfill_test.go`
- Modify: `backend/internal/securityaudit/conversation_repository.go`
- Modify: `backend/internal/handler/admin/prompt_audit_handler.go`
- Create: `docs/usage-archive-rollout.md`

- [ ] Add a dry-run backfill report from `conversation_review_*` into the new model without writing data.
- [ ] Add an explicit idempotent backfill command that only runs after dry-run review and never deletes source rows.
- [ ] Keep old Prompt Audit conversation endpoints working as historical compatibility views.
- [ ] Document enablement, storage budget, retention, rollback, and how to disable body capture while retaining metadata.
- [ ] Run all focused backend/frontend tests and verify no write path remains dependent on safety audit decisions.
- [ ] Commit `docs: document usage archive rollout and compatibility`.

