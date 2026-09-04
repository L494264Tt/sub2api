package securityaudit

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

func (r *PostgreSQLRepository) RecordConversationCapture(ctx context.Context, capture conversationCapture) error {
	if r == nil || r.db == nil {
		return errors.New("conversation review database unavailable")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	sessionID := int64(0)
	if capture.PreviousResponseID != "" {
		err = tx.QueryRowContext(ctx, `
			SELECT id FROM conversation_review_sessions
			WHERE last_response_id=$1 AND ($2::BIGINT IS NULL OR user_id=$2) AND ($3::BIGINT IS NULL OR api_key_id=$3)
			ORDER BY last_turn_at DESC LIMIT 1 FOR UPDATE`, capture.PreviousResponseID,
			nullableID(capture.Request.UserID), nullableID(capture.Request.APIKeyID)).Scan(&sessionID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
	}
	if sessionID == 0 {
		err = tx.QueryRowContext(ctx, `
			INSERT INTO conversation_review_sessions (
				conversation_key,external_conversation_id,last_response_id,user_id,username_snapshot,user_email_snapshot,
				api_key_id,api_key_name_snapshot,group_id,group_name,provider,protocol,model,started_at,last_turn_at
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$14)
			ON CONFLICT (conversation_key) DO UPDATE SET
				external_conversation_id=CASE WHEN EXCLUDED.external_conversation_id<>'' THEN EXCLUDED.external_conversation_id ELSE conversation_review_sessions.external_conversation_id END,
				last_response_id=CASE WHEN EXCLUDED.last_response_id<>'' THEN EXCLUDED.last_response_id ELSE conversation_review_sessions.last_response_id END,
				username_snapshot=EXCLUDED.username_snapshot,user_email_snapshot=EXCLUDED.user_email_snapshot,
				api_key_name_snapshot=EXCLUDED.api_key_name_snapshot,group_name=EXCLUDED.group_name,
				provider=EXCLUDED.provider,protocol=EXCLUDED.protocol,model=EXCLUDED.model,updated_at=NOW()
			RETURNING id`, capture.ConversationKey, capture.ExternalConversationID, capture.UpstreamResponseID,
			nullableID(capture.Request.UserID), capture.Request.Username, capture.Request.UserEmail,
			nullableID(capture.Request.APIKeyID), capture.Request.APIKeyName, capture.Request.GroupID, capture.Request.GroupName,
			capture.Request.Provider, capture.Request.Protocol, capture.Request.Model, capture.CapturedAt.UTC()).Scan(&sessionID)
		if err != nil {
			return err
		}
	}

	var turnID int64
	err = tx.QueryRowContext(ctx, `
		INSERT INTO conversation_review_turns (
			session_id,request_id,upstream_response_id,endpoint,protocol,model,request_transcript,model_response,
			request_chars,response_chars,request_truncated,response_truncated,status_code,captured_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
		ON CONFLICT (request_id) WHERE request_id<>'' DO NOTHING
		RETURNING id`, sessionID, capture.Request.RequestID, capture.UpstreamResponseID, capture.Request.Endpoint,
		capture.Request.Protocol, capture.Request.Model, capture.RequestTranscript, capture.ModelResponse,
		capture.RequestChars, capture.ResponseChars, capture.RequestTruncated, capture.ResponseTruncated,
		capture.StatusCode, capture.CapturedAt.UTC()).Scan(&turnID)
	if errors.Is(err, sql.ErrNoRows) {
		return tx.Commit()
	}
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
		UPDATE conversation_review_sessions SET
			last_response_id=CASE WHEN $2<>'' THEN $2 ELSE last_response_id END,
			turn_count=turn_count+1,pending_review_count=pending_review_count+1,
			last_turn_at=$3,updated_at=NOW()
		WHERE id=$1`, sessionID, capture.UpstreamResponseID, capture.CapturedAt.UTC())
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (r *PostgreSQLRepository) QueueConversationReviewRun(ctx context.Context, trigger string, requestedBy int64, cfg ActiveConfig) (*ConversationRun, error) {
	if trigger != "manual" {
		trigger = "scheduled"
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, conversationReviewAdvisoryLockKey); err != nil {
		return nil, err
	}
	run, err := scanConversationRun(tx.QueryRowContext(ctx, `SELECT `+conversationRunColumns("r")+` FROM conversation_review_runs r WHERE status IN ('queued','processing') ORDER BY created_at,id LIMIT 1`))
	if err == nil {
		if commitErr := tx.Commit(); commitErr != nil {
			return nil, commitErr
		}
		return run, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	run, err = scanConversationRun(tx.QueryRowContext(ctx, `
		INSERT INTO conversation_review_runs (trigger_type,status,requested_by,config_version,batch_size)
		VALUES ($1,'queued',$2,$3,$4)
		RETURNING `+conversationRunColumns("conversation_review_runs"), trigger, nullableID(requestedBy), cfg.ConfigVersion, cfg.ConversationReviewBatchSize))
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return run, nil
}

func (r *PostgreSQLRepository) ScheduleConversationReviewIfDue(ctx context.Context, cfg ActiveConfig, now time.Time) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var locked bool
	if err := tx.QueryRowContext(ctx, `SELECT pg_try_advisory_xact_lock($1)`, conversationReviewAdvisoryLockKey).Scan(&locked); err != nil || !locked {
		return err
	}
	interval := time.Duration(cfg.ConversationReviewIntervalMinutes) * time.Minute
	var due bool
	if err := tx.QueryRowContext(ctx, `
		SELECT NOT EXISTS (SELECT 1 FROM conversation_review_runs WHERE status IN ('queued','processing'))
		AND NOT EXISTS (SELECT 1 FROM conversation_review_runs WHERE created_at > $1)`, now.UTC().Add(-interval)).Scan(&due); err != nil {
		return err
	}
	if due {
		if _, err := tx.ExecContext(ctx, `INSERT INTO conversation_review_runs (trigger_type,status,config_version,batch_size) VALUES ('scheduled','queued',$1,$2)`, cfg.ConfigVersion, cfg.ConversationReviewBatchSize); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *PostgreSQLRepository) ClaimConversationReviewRun(ctx context.Context) (*ConversationRun, bool, error) {
	run, err := scanConversationRun(r.db.QueryRowContext(ctx, `
		WITH candidate AS (
			SELECT id FROM conversation_review_runs WHERE status='queued' ORDER BY created_at,id FOR UPDATE SKIP LOCKED LIMIT 1
		)
		UPDATE conversation_review_runs r SET status='processing',started_at=NOW(),updated_at=NOW()
		FROM candidate WHERE r.id=candidate.id RETURNING `+conversationRunColumns("r")))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	return run, err == nil, err
}

func (r *PostgreSQLRepository) ClaimConversationReviewTurns(ctx context.Context, limit int) ([]*ConversationTurn, error) {
	if limit < 1 || limit > MaxConversationReviewBatchSize {
		limit = DefaultConversationReviewBatchSize
	}
	rows, err := r.db.QueryContext(ctx, `
		WITH candidates AS (
			SELECT id FROM conversation_review_turns
			WHERE review_status='pending' OR (review_status='failed' AND review_attempts<3)
			ORDER BY captured_at,id FOR UPDATE SKIP LOCKED LIMIT $1
		)
		UPDATE conversation_review_turns t SET review_status='processing',review_attempts=t.review_attempts+1,
			review_claim_version=t.review_claim_version+1,review_started_at=NOW(),updated_at=NOW()
		FROM candidates WHERE t.id=candidates.id RETURNING `+conversationTurnColumns("t"), limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	turns := make([]*ConversationTurn, 0, limit)
	for rows.Next() {
		turn, err := scanConversationTurn(rows)
		if err != nil {
			return nil, err
		}
		turns = append(turns, turn)
	}
	return turns, rows.Err()
}

func (r *PostgreSQLRepository) CompleteConversationReviewTurn(ctx context.Context, turn *ConversationTurn, result *NormalizedResult) error {
	if turn == nil || result == nil {
		return errors.New("conversation review result required")
	}
	categories, _ := json.Marshal(result.Categories)
	matched, _ := json.Marshal(result.MatchedScanners)
	scores, _ := json.Marshal(result.ScannerScores)
	evidence, _ := json.Marshal(result.ScannerEvidence)
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	updated, err := tx.ExecContext(ctx, `
		UPDATE conversation_review_turns SET review_status='reviewed',reviewed_at=NOW(),review_started_at=NULL,
			review_decision=$3,risk_level=$4,action=$5,categories=$6::jsonb,matched_scanners=$7::jsonb,
			scanner_scores=$8::jsonb,scanner_evidence=$9::jsonb,scanner_backend=$10,scanner_version=$11,
			guard_endpoint_id=$12,review_error_code='',review_error_message='',updated_at=NOW()
		WHERE id=$1 AND review_claim_version=$2 AND review_status='processing'`, turn.ID, turn.ReviewClaimVersion,
		string(result.Decision), string(result.RiskLevel), string(result.Action), categories, matched, scores, evidence,
		result.ScannerBackend, result.ScannerVersion, result.GuardEndpointID)
	if err := requireOneRow(updated, err, ErrLeaseLost); err != nil {
		return err
	}
	if err := refreshConversationSessionSummary(ctx, tx, turn.SessionID); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *PostgreSQLRepository) FailConversationReviewTurn(ctx context.Context, turn *ConversationTurn, code string) error {
	code, message := sanitizeStoredError(code)
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	updated, err := tx.ExecContext(ctx, `
		UPDATE conversation_review_turns SET review_status='failed',review_started_at=NULL,
			review_error_code=$3,review_error_message=$4,updated_at=NOW()
		WHERE id=$1 AND review_claim_version=$2 AND review_status='processing'`, turn.ID, turn.ReviewClaimVersion, code, message)
	if err := requireOneRow(updated, err, ErrLeaseLost); err != nil {
		return err
	}
	if err := refreshConversationSessionSummary(ctx, tx, turn.SessionID); err != nil {
		return err
	}
	return tx.Commit()
}

func refreshConversationSessionSummary(ctx context.Context, tx *sql.Tx, sessionID int64) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE conversation_review_sessions s SET
			pending_review_count=(SELECT COUNT(*) FROM conversation_review_turns t WHERE t.session_id=s.id AND t.review_status IN ('pending','processing','failed')),
			flagged_turn_count=(SELECT COUNT(*) FROM conversation_review_turns t WHERE t.session_id=s.id AND t.review_decision IN ('flag','critical')),
			latest_review_decision=COALESCE((SELECT t.review_decision FROM conversation_review_turns t WHERE t.session_id=s.id AND t.review_status='reviewed' ORDER BY t.reviewed_at DESC,t.id DESC LIMIT 1),''),
			updated_at=NOW() WHERE s.id=$1`, sessionID)
	return err
}

func (r *PostgreSQLRepository) FinishConversationReviewRun(ctx context.Context, runID int64, processed, flagged, failed int, runErr error) error {
	status, code, message := "completed", "", ""
	if runErr != nil {
		status, code = "failed", guardErrorCode(runErr)
		_, message = sanitizeStoredError(code)
	}
	_, err := r.db.ExecContext(ctx, `UPDATE conversation_review_runs SET status=$2,processed_count=$3,flagged_count=$4,
		failed_count=$5,completed_at=NOW(),last_error_code=$6,last_error_message=$7,updated_at=NOW() WHERE id=$1`,
		runID, status, processed, flagged, failed, code, message)
	return err
}

func (r *PostgreSQLRepository) CleanupConversationReview(ctx context.Context, before time.Time) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM conversation_review_sessions WHERE last_turn_at < $1`, before.UTC())
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `DELETE FROM conversation_review_runs WHERE created_at < $1 AND status IN ('completed','failed')`, before.UTC())
	return err
}

func (r *PostgreSQLRepository) ReclaimStaleConversationReviews(ctx context.Context, before time.Time) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.QueryContext(ctx, `
		UPDATE conversation_review_turns SET review_status='failed',review_started_at=NULL,
			review_error_code='review_lease_expired',review_error_message='',updated_at=NOW()
		WHERE review_status='processing' AND review_started_at < $1 RETURNING session_id`, before.UTC())
	if err != nil {
		return err
	}
	sessionIDs := make(map[int64]struct{})
	for rows.Next() {
		var sessionID int64
		if err := rows.Scan(&sessionID); err != nil {
			_ = rows.Close()
			return err
		}
		sessionIDs[sessionID] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for sessionID := range sessionIDs {
		if err := refreshConversationSessionSummary(ctx, tx, sessionID); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE conversation_review_runs SET status='failed',completed_at=NOW(),
			last_error_code='review_lease_expired',last_error_message='',updated_at=NOW()
		WHERE status='processing' AND started_at < $1`, before.UTC()); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *PostgreSQLRepository) ListConversationSessions(ctx context.Context, filter ConversationFilter, page, pageSize int) (*ConversationPage, error) {
	page, pageSize = normalizeConversationPage(page, pageSize)
	where, args := buildConversationWhere(filter)
	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM conversation_review_sessions s`+where, args...).Scan(&total); err != nil {
		return nil, err
	}
	limitIndex := len(args) + 1
	queryArgs := append(append([]any(nil), args...), pageSize, (page-1)*pageSize)
	rows, err := r.db.QueryContext(ctx, `SELECT `+conversationSessionColumns("s")+` FROM conversation_review_sessions s`+where+
		fmt.Sprintf(` ORDER BY s.last_turn_at DESC,s.id DESC LIMIT $%d OFFSET $%d`, limitIndex, limitIndex+1), queryArgs...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	items := make([]*ConversationSession, 0, pageSize)
	for rows.Next() {
		session, err := scanConversationSession(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, session)
	}
	pages := 0
	if total > 0 {
		pages = int((total + int64(pageSize) - 1) / int64(pageSize))
	}
	return &ConversationPage{Items: items, Total: total, Page: page, PageSize: pageSize, Pages: pages}, rows.Err()
}

func (r *PostgreSQLRepository) GetConversationSession(ctx context.Context, id int64) (*ConversationSession, error) {
	session, err := scanConversationSession(r.db.QueryRowContext(ctx, `SELECT `+conversationSessionColumns("s")+` FROM conversation_review_sessions s WHERE s.id=$1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrConversationNotFound
	}
	if err != nil {
		return nil, err
	}
	rows, err := r.db.QueryContext(ctx, `SELECT `+conversationTurnColumns("t")+` FROM conversation_review_turns t WHERE t.session_id=$1 ORDER BY t.captured_at,t.id`, id)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		turn, err := scanConversationTurn(rows)
		if err != nil {
			return nil, err
		}
		session.Turns = append(session.Turns, turn)
	}
	return session, rows.Err()
}

func (r *PostgreSQLRepository) DeleteConversationSession(ctx context.Context, id int64) (*ConversationDeleteResult, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	var turns int64
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM conversation_review_turns WHERE session_id=$1`, id).Scan(&turns); err != nil {
		return nil, err
	}
	result, err := tx.ExecContext(ctx, `DELETE FROM conversation_review_sessions WHERE id=$1`, id)
	if err != nil {
		return nil, err
	}
	sessions, _ := result.RowsAffected()
	if sessions == 0 {
		return nil, ErrConversationNotFound
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &ConversationDeleteResult{DeletedSessions: sessions, DeletedTurns: turns}, nil
}

func (r *PostgreSQLRepository) ListConversationReviewRuns(ctx context.Context, page, pageSize int) (*ConversationRunPage, error) {
	page, pageSize = normalizeConversationPage(page, pageSize)
	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM conversation_review_runs`).Scan(&total); err != nil {
		return nil, err
	}
	rows, err := r.db.QueryContext(ctx, `SELECT `+conversationRunColumns("r")+` FROM conversation_review_runs r ORDER BY r.created_at DESC,r.id DESC LIMIT $1 OFFSET $2`, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	items := make([]*ConversationRun, 0, pageSize)
	for rows.Next() {
		run, err := scanConversationRun(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, run)
	}
	pages := 0
	if total > 0 {
		pages = int((total + int64(pageSize) - 1) / int64(pageSize))
	}
	return &ConversationRunPage{Items: items, Total: total, Page: page, PageSize: pageSize, Pages: pages}, rows.Err()
}

func buildConversationWhere(filter ConversationFilter) (string, []any) {
	clauses := []string{" WHERE 1=1"}
	args := make([]any, 0, 8)
	add := func(format string, value any) {
		args = append(args, value)
		clauses = append(clauses, fmt.Sprintf(format, len(args)))
	}
	if filter.GroupID != nil {
		add(" AND s.group_id=$%d", *filter.GroupID)
	}
	if filter.UserID != nil {
		add(" AND s.user_id=$%d", *filter.UserID)
	}
	if filter.APIKeyID != nil {
		add(" AND s.api_key_id=$%d", *filter.APIKeyID)
	}
	if value := strings.TrimSpace(filter.ConversationID); value != "" {
		add(" AND s.external_conversation_id ILIKE '%%'||$%d||'%%'", value)
	}
	if value := strings.TrimSpace(filter.Decision); value != "" {
		add(" AND s.latest_review_decision=$%d", strings.ToLower(value))
	}
	if value := strings.TrimSpace(filter.ReviewStatus); value != "" {
		add(" AND EXISTS (SELECT 1 FROM conversation_review_turns t WHERE t.session_id=s.id AND t.review_status=$%d)", strings.ToLower(value))
	}
	if value := strings.TrimSpace(filter.RequestID); value != "" {
		add(" AND EXISTS (SELECT 1 FROM conversation_review_turns t WHERE t.session_id=s.id AND t.request_id=$%d)", value)
	}
	if value := strings.TrimSpace(filter.Keyword); value != "" {
		args = append(args, value)
		index := len(args)
		clauses = append(clauses, fmt.Sprintf(" AND EXISTS (SELECT 1 FROM conversation_review_turns t WHERE t.session_id=s.id AND (t.request_transcript ILIKE '%%'||$%d||'%%' OR t.model_response ILIKE '%%'||$%d||'%%'))", index, index))
	}
	if filter.StartAt != nil {
		add(" AND s.last_turn_at >= $%d", filter.StartAt.UTC())
	}
	if filter.EndAt != nil {
		add(" AND s.last_turn_at <= $%d", filter.EndAt.UTC())
	}
	return strings.Join(clauses, ""), args
}

func normalizeConversationPage(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize
}

func conversationSessionColumns(alias string) string {
	return fmt.Sprintf(`%[1]s.id,%[1]s.external_conversation_id,%[1]s.user_id,%[1]s.username_snapshot,%[1]s.user_email_snapshot,
		%[1]s.api_key_id,%[1]s.api_key_name_snapshot,%[1]s.group_id,%[1]s.group_name,%[1]s.provider,%[1]s.protocol,%[1]s.model,
		%[1]s.turn_count,%[1]s.pending_review_count,%[1]s.flagged_turn_count,%[1]s.latest_review_decision,
		%[1]s.started_at,%[1]s.last_turn_at,%[1]s.created_at,%[1]s.updated_at`, alias)
}

func conversationTurnColumns(alias string) string {
	return fmt.Sprintf(`%[1]s.id,%[1]s.session_id,%[1]s.request_id,%[1]s.upstream_response_id,%[1]s.endpoint,%[1]s.protocol,%[1]s.model,
		%[1]s.request_transcript,%[1]s.model_response,%[1]s.request_chars,%[1]s.response_chars,%[1]s.request_truncated,%[1]s.response_truncated,
		%[1]s.status_code,%[1]s.review_status,%[1]s.review_attempts,%[1]s.review_claim_version,%[1]s.review_started_at,%[1]s.reviewed_at,
		%[1]s.review_decision,%[1]s.risk_level,%[1]s.action,%[1]s.categories,%[1]s.matched_scanners,%[1]s.scanner_scores,%[1]s.scanner_evidence,
		%[1]s.scanner_backend,%[1]s.scanner_version,%[1]s.guard_endpoint_id,%[1]s.review_error_code,%[1]s.review_error_message,
		%[1]s.captured_at,%[1]s.created_at,%[1]s.updated_at`, alias)
}

func conversationRunColumns(alias string) string {
	return fmt.Sprintf(`%[1]s.id,%[1]s.trigger_type,%[1]s.status,%[1]s.requested_by,%[1]s.config_version,%[1]s.batch_size,
		%[1]s.processed_count,%[1]s.flagged_count,%[1]s.failed_count,%[1]s.started_at,%[1]s.completed_at,
		%[1]s.last_error_code,%[1]s.last_error_message,%[1]s.created_at,%[1]s.updated_at`, alias)
}

func scanConversationSession(row rowScanner) (*ConversationSession, error) {
	session := &ConversationSession{}
	var userID, apiKeyID, groupID sql.NullInt64
	err := row.Scan(&session.ID, &session.ExternalConversationID, &userID, &session.UsernameSnapshot, &session.UserEmailSnapshot,
		&apiKeyID, &session.APIKeyNameSnapshot, &groupID, &session.GroupName, &session.Provider, &session.Protocol, &session.Model,
		&session.TurnCount, &session.PendingReviewCount, &session.FlaggedTurnCount, &session.LatestReviewDecision,
		&session.StartedAt, &session.LastTurnAt, &session.CreatedAt, &session.UpdatedAt)
	if err != nil {
		return nil, err
	}
	session.UserID, session.APIKeyID, session.GroupID = nullableInt64Value(userID), nullableInt64Value(apiKeyID), nullableInt64Ptr(groupID)
	return session, nil
}

func scanConversationTurn(row rowScanner) (*ConversationTurn, error) {
	turn := &ConversationTurn{}
	var startedAt, reviewedAt sql.NullTime
	var categories, matched, scores, evidence []byte
	err := row.Scan(&turn.ID, &turn.SessionID, &turn.RequestID, &turn.UpstreamResponseID, &turn.Endpoint, &turn.Protocol, &turn.Model,
		&turn.RequestTranscript, &turn.ModelResponse, &turn.RequestChars, &turn.ResponseChars, &turn.RequestTruncated, &turn.ResponseTruncated,
		&turn.StatusCode, &turn.ReviewStatus, &turn.ReviewAttempts, &turn.ReviewClaimVersion, &startedAt, &reviewedAt,
		&turn.ReviewDecision, &turn.RiskLevel, &turn.Action, &categories, &matched, &scores, &evidence,
		&turn.ScannerBackend, &turn.ScannerVersion, &turn.GuardEndpointID, &turn.ReviewErrorCode, &turn.ReviewErrorMessage,
		&turn.CapturedAt, &turn.CreatedAt, &turn.UpdatedAt)
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal(categories, &turn.Categories)
	_ = json.Unmarshal(matched, &turn.MatchedScanners)
	_ = json.Unmarshal(scores, &turn.ScannerScores)
	_ = json.Unmarshal(evidence, &turn.ScannerEvidence)
	if turn.Categories == nil {
		turn.Categories = []string{}
	}
	if turn.MatchedScanners == nil {
		turn.MatchedScanners = []string{}
	}
	if turn.ScannerScores == nil {
		turn.ScannerScores = map[string]float64{}
	}
	if turn.ScannerEvidence == nil {
		turn.ScannerEvidence = map[string]string{}
	}
	if startedAt.Valid {
		value := startedAt.Time
		turn.ReviewStartedAt = &value
	}
	if reviewedAt.Valid {
		value := reviewedAt.Time
		turn.ReviewedAt = &value
	}
	return turn, nil
}

func scanConversationRun(row rowScanner) (*ConversationRun, error) {
	run := &ConversationRun{}
	var requestedBy sql.NullInt64
	var startedAt, completedAt sql.NullTime
	err := row.Scan(&run.ID, &run.TriggerType, &run.Status, &requestedBy, &run.ConfigVersion, &run.BatchSize,
		&run.ProcessedCount, &run.FlaggedCount, &run.FailedCount, &startedAt, &completedAt,
		&run.LastErrorCode, &run.LastErrorMessage, &run.CreatedAt, &run.UpdatedAt)
	if err != nil {
		return nil, err
	}
	run.RequestedBy = nullableInt64Value(requestedBy)
	if startedAt.Valid {
		value := startedAt.Time
		run.StartedAt = &value
	}
	if completedAt.Valid {
		value := completedAt.Time
		run.CompletedAt = &value
	}
	return run, nil
}
