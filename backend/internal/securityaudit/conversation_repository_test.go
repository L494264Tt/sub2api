package securityaudit

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestRenewConversationLeaseFencesRunAndTurns(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WithArgs(conversationReviewAdvisoryLockKey).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE conversation_review_runs SET updated_at=NOW\\(\\) WHERE id=\\$1 AND status='processing'").WithArgs(int64(7)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE conversation_review_turns.*jsonb_to_recordset.*t.review_claim_version=claim.version AND t.review_status='processing'").WithArgs([]byte(`[{"id":8,"version":3}]`)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	require.NoError(t, NewPostgreSQLRepository(db).RenewConversationReviewLease(context.Background(), 7, []*ConversationTurn{{ID: 8, ReviewClaimVersion: 3}}))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRenewConversationLeaseRejectsReclaimedRun(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE conversation_review_runs").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()
	require.ErrorIs(t, NewPostgreSQLRepository(db).RenewConversationReviewLease(context.Background(), 7, nil), ErrLeaseLost)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestFinishConversationRunCannotOverwriteReclaimedRun(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	mock.ExpectExec("UPDATE conversation_review_runs.*WHERE id=\\$1 AND status='processing'").WillReturnResult(sqlmock.NewResult(0, 0))
	require.ErrorIs(t, NewPostgreSQLRepository(db).FinishConversationReviewRun(context.Background(), 7, 0, 0, 0, nil), ErrLeaseLost)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestReclaimConversationLeasesUsesHeartbeatNotStartTime(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	before := time.Now().Add(-conversationReviewLeaseDuration)
	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("UPDATE conversation_review_turns.*WHERE review_status='processing' AND updated_at < \\$1 RETURNING session_id").WithArgs(before.UTC()).WillReturnRows(sqlmock.NewRows([]string{"session_id"}))
	mock.ExpectExec("UPDATE conversation_review_runs.*WHERE status='processing' AND updated_at < \\$1").WithArgs(before.UTC()).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()
	require.NoError(t, NewPostgreSQLRepository(db).ReclaimStaleConversationReviews(context.Background(), before))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestConversationRetentionDeletesExpiredTurnsInActiveSessions(t *testing.T) {
	for _, failSummary := range []bool{false, true} {
		t.Run(map[bool]string{false: "commit", true: "rollback"}[failSummary], func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()
			before := time.Now().AddDate(0, 0, -30).UTC()
			mock.ExpectBegin()
			mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectQuery("SELECT s.id FROM conversation_review_sessions s WHERE EXISTS .*t.captured_at < \\$1.*FOR UPDATE OF s SKIP LOCKED").WithArgs(before, conversationCleanupBatchSize).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(7))
			mock.ExpectExec("DELETE FROM conversation_review_turns WHERE session_id=ANY\\(\\$1::bigint\\[\\]\\) AND captured_at < \\$2").WithArgs("{7}", before).WillReturnResult(sqlmock.NewResult(0, 2))
			summary := mock.ExpectExec("UPDATE conversation_review_sessions s SET turn_count=.*pending_review_count=.*flagged_turn_count=.*latest_review_decision=.*started_at=.*last_turn_at=.*WHERE s.id=ANY").WithArgs("{7}")
			if failSummary {
				summary.WillReturnError(errors.New("summary failed"))
				mock.ExpectRollback()
			} else {
				summary.WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectExec("DELETE FROM conversation_review_sessions s WHERE s.id=ANY.*AND NOT EXISTS").WithArgs("{7}").WillReturnResult(sqlmock.NewResult(0, 0))
				mock.ExpectCommit()
			}
			count, err := NewPostgreSQLRepository(db).cleanupConversationReviewBatch(context.Background(), before)
			if failSummary {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, 1, count)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestTruncatedResponseIsArchivedEvenWithoutRecoverableText(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	service := &PromptService{repo: NewPostgreSQLRepository(db), clock: realClock{}, config: &fakeConfigStore{active: true, cfg: ActiveConfig{
		ConversationRecordingEnabled: true, AllGroups: true, ConversationRequestMaxRunes: 128, ConversationResponseMaxRunes: 128,
	}}}
	mock.ExpectBegin()
	mock.ExpectQuery("INSERT INTO conversation_review_sessions").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(7))
	mock.ExpectQuery("INSERT INTO conversation_review_turns").WithArgs(int64(7), "request-1", "", "", "openai_chat", "", "[user]\nhello", "", 12, 0, false, true, 200, sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(8))
	mock.ExpectExec("UPDATE conversation_review_sessions SET").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	err = service.RecordConversationTurn(context.Background(), Request{RequestID: "request-1", Protocol: "openai_chat", Body: []byte(`{"messages":[{"role":"user","content":"hello"}]}`)}, 200, "application/json", []byte(`{"id":`), true)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}
