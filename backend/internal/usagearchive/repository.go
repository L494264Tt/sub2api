package usagearchive

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

type PostgreSQLRepository struct {
	db *sql.DB
}

func NewPostgresRepository(db *sql.DB) *PostgreSQLRepository {
	return &PostgreSQLRepository{db: db}
}

func (r *PostgreSQLRepository) Record(ctx context.Context, capture UsageCapture, continuity ContinuityResult, policy ArchiveRetentionPolicy) error {
	if r == nil || r.db == nil {
		return errors.New("usage archive database unavailable")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	contentBytes := capture.RequestBytes + capture.ResponseBytes
	var currentBytes int64
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(content_bytes),0) FROM usage_archive_messages WHERE created_at >= date_trunc('day', NOW())`).Scan(&currentBytes); err != nil {
		return err
	}
	storeContent := ShouldStoreBody(int64(contentBytes), policy.DailyContentBudgetBytes, currentBytes)
	sessionID, err := r.upsertSession(ctx, tx, capture, continuity)
	if err != nil {
		return err
	}
	parentID, err := r.findParentTurn(ctx, tx, capture, sessionID)
	if err != nil {
		return err
	}
	sequence, err := r.nextSequence(ctx, tx, sessionID)
	if err != nil {
		return err
	}
	systemHashes, developerHashes, err := r.upsertPromptSnapshots(ctx, tx, capture, storeContent)
	if err != nil {
		return err
	}
	turnID, inserted, err := r.insertTurn(ctx, tx, capture, continuity, sessionID, parentID, sequence, systemHashes, developerHashes)
	if err != nil {
		return err
	}
	if !inserted {
		return tx.Commit()
	}
	for _, message := range append(capture.RequestMessages, capture.ResponseMessages...) {
		if !storeContent {
			message.Content = ""
			message.Truncated = true
		}
		if err := insertMessage(ctx, tx, turnID, message); err != nil {
			return err
		}
	}
	_, err = tx.ExecContext(ctx, `UPDATE usage_archive_sessions SET turn_count=turn_count+1,content_bytes=content_bytes+$2,last_seen_at=$3,updated_at=NOW(),continuity_confidence=$4 WHERE id=$1`, sessionID, contentBytes, capture.CapturedAt.UTC(), string(continuity.Confidence))
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (r *PostgreSQLRepository) upsertSession(ctx context.Context, tx *sql.Tx, capture UsageCapture, continuity ContinuityResult) (int64, error) {
	var id int64
	err := tx.QueryRowContext(ctx, `
        INSERT INTO usage_archive_sessions (session_key,user_id,api_key_id,group_id,provider,model,first_seen_at,last_seen_at,content_bytes,continuity_confidence)
        VALUES ($1,$2,$3,$4,$5,$6,$7,$7,$8,$9)
        ON CONFLICT (session_key) DO UPDATE SET
            user_id=COALESCE(EXCLUDED.user_id,usage_archive_sessions.user_id),
            api_key_id=COALESCE(EXCLUDED.api_key_id,usage_archive_sessions.api_key_id),
            group_id=COALESCE(EXCLUDED.group_id,usage_archive_sessions.group_id),
            provider=EXCLUDED.provider,model=EXCLUDED.model,last_seen_at=GREATEST(usage_archive_sessions.last_seen_at,EXCLUDED.last_seen_at),
            updated_at=NOW(),continuity_confidence=EXCLUDED.continuity_confidence
        RETURNING id`, continuity.SessionKey, nullableID(capture.UserID), nullableID(capture.APIKeyID), capture.GroupID,
		capture.Provider, capture.Model, capture.CapturedAt.UTC(), 0, string(continuity.Confidence)).Scan(&id)
	return id, err
}

func (r *PostgreSQLRepository) findParentTurn(ctx context.Context, tx *sql.Tx, capture UsageCapture, sessionID int64) (*int64, error) {
	identity := strings.TrimSpace(capture.PreviousResponseID)
	if identity == "" {
		identity = strings.TrimSpace(capture.UpstreamResponseID)
	}
	if identity == "" {
		return nil, nil
	}
	var id int64
	err := tx.QueryRowContext(ctx, `SELECT id FROM usage_archive_turns WHERE session_id=$1 AND (upstream_response_id=$2 OR request_id=$2) ORDER BY id DESC LIMIT 1`, sessionID, identity).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &id, err
}

func (r *PostgreSQLRepository) nextSequence(ctx context.Context, tx *sql.Tx, sessionID int64) (int, error) {
	var count int
	err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM usage_archive_turns WHERE session_id=$1`, sessionID).Scan(&count)
	return count + 1, err
}

func (r *PostgreSQLRepository) upsertPromptSnapshots(ctx context.Context, tx *sql.Tx, capture UsageCapture, storeContent bool) ([]string, []string, error) {
	insert := func(role MessageRole, snapshots []PromptSnapshot) ([]string, error) {
		hashes := make([]string, 0, len(snapshots))
		for _, snapshot := range snapshots {
			content := snapshot.Content
			if !storeContent {
				content = ""
			}
			_, err := tx.ExecContext(ctx, `
                INSERT INTO usage_archive_prompt_snapshots (role,content_hash,content,content_bytes,content_runes,truncated)
                VALUES ($1,$2,$3,$4,$5,$6)
                ON CONFLICT (role,content_hash) DO UPDATE SET last_seen_at=NOW()`, role, snapshot.ContentHash, content, snapshot.ContentBytes, snapshot.ContentRunes, snapshot.Truncated || !storeContent)
			if err != nil {
				return nil, err
			}
			hashes = append(hashes, snapshot.ContentHash)
		}
		return hashes, nil
	}
	system, err := insert(RoleSystem, capture.SystemPrompts)
	if err != nil {
		return nil, nil, err
	}
	developer, err := insert(RoleDeveloper, capture.DeveloperPrompts)
	return system, developer, err
}

func (r *PostgreSQLRepository) insertTurn(ctx context.Context, tx *sql.Tx, capture UsageCapture, continuity ContinuityResult, sessionID int64, parentID *int64, sequence int, systemHashes, developerHashes []string) (int64, bool, error) {
	systemJSON, _ := json.Marshal(systemHashes)
	developerJSON, _ := json.Marshal(developerHashes)
	var id int64
	err := tx.QueryRowContext(ctx, `
        INSERT INTO usage_archive_turns (session_id,usage_log_request_id,request_id,parent_turn_id,sequence_no,conversation_id,previous_response_id,upstream_response_id,history_key,endpoint,protocol,provider,model,status_code,duration_ms,first_token_ms,request_bytes,response_bytes,request_truncated,response_truncated,system_prompt_hashes,developer_prompt_hashes,continuity_source,continuity_confidence,captured_at)
        VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21::jsonb,$22::jsonb,$23,$24,$25)
        ON CONFLICT (usage_log_request_id) WHERE usage_log_request_id<>'' DO NOTHING
        RETURNING id`, sessionID, capture.UsageLogRequestID, capture.RequestID, parentID, sequence, capture.ConversationID,
		capture.PreviousResponseID, capture.UpstreamResponseID, capture.HistoryKey, capture.Endpoint, capture.Protocol, capture.Provider,
		capture.Model, capture.StatusCode, capture.DurationMS, capture.FirstTokenMS, capture.RequestBytes, capture.ResponseBytes,
		capture.RequestTruncated, capture.ResponseTruncated, systemJSON, developerJSON, string(continuity.Source), string(continuity.Confidence), capture.CapturedAt.UTC()).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	return id, err == nil, err
}

func insertMessage(ctx context.Context, tx *sql.Tx, turnID int64, message ArchiveMessage) error {
	if err := ValidateMessageRole(message.Role); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO usage_archive_messages (turn_id,role,source,ordinal,content,content_hash,content_bytes,content_runes,truncated) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`, turnID, string(message.Role), string(message.Source), message.Ordinal, message.Content, message.ContentHash, message.ContentBytes, message.ContentRunes, message.Truncated)
	return err
}

func nullableID(value int64) any {
	if value <= 0 {
		return nil
	}
	return value
}

func (r *PostgreSQLRepository) Cleanup(ctx context.Context, rawBefore, metadataBefore time.Time, batchSize int) (CleanupResult, error) {
	if r == nil || r.db == nil {
		return CleanupResult{}, errors.New("usage archive database unavailable")
	}
	if batchSize <= 0 {
		batchSize = 100
	}
	result := CleanupResult{}
	res, err := r.db.ExecContext(ctx, `WITH target AS (
        SELECT id FROM usage_archive_messages WHERE content_expired=FALSE AND created_at<$1 ORDER BY id LIMIT $2
    ) UPDATE usage_archive_messages m SET content='',content_expired=TRUE FROM target WHERE m.id=target.id`, rawBefore.UTC(), batchSize)
	if err != nil {
		return result, err
	}
	if count, err := res.RowsAffected(); err == nil {
		result.DeletedContentRows = count
	}
	_, err = r.db.ExecContext(ctx, `DELETE FROM usage_archive_sessions WHERE last_seen_at<$1`, metadataBefore.UTC())
	return result, err
}

func (r *PostgreSQLRepository) StorageStats(ctx context.Context) (StorageStats, error) {
	if r == nil || r.db == nil {
		return StorageStats{}, errors.New("usage archive database unavailable")
	}
	var result StorageStats
	err := r.db.QueryRowContext(ctx, `
        SELECT pg_total_relation_size('usage_archive_sessions'),pg_total_relation_size('usage_archive_turns'),
               pg_total_relation_size('usage_archive_messages'),pg_total_relation_size('usage_archive_prompt_snapshots'),
               (SELECT COUNT(*) FROM usage_archive_sessions),(SELECT COUNT(*) FROM usage_archive_turns),(SELECT COUNT(*) FROM usage_archive_messages),
               (SELECT COALESCE(SUM(content_bytes),0) FROM usage_archive_messages)`).Scan(&result.SessionBytes, &result.TurnBytes, &result.MessageBytes, &result.PromptSnapshotBytes, &result.SessionCount, &result.TurnCount, &result.MessageCount, &result.RawContentBytes)
	return result, err
}
