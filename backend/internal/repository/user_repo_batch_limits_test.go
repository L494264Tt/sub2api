package repository

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestBatchUpdateLimitsUpdatesRequestAndGPTTokenLimitsTogether(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &userRepository{sql: db}
	concurrency := 8
	rpmLimit := 60
	tokenLimit1d := int64(100_000_000)
	tokenLimit7d := int64(400_000_000)
	tokenLimit30d := int64(0)

	mock.ExpectExec(`UPDATE users SET concurrency = \$1, rpm_limit = \$2, token_limit_1d = \$3, token_limit_7d = \$4, token_limit_30d = \$5, updated_at = NOW\(\) WHERE id = ANY\(\$6\) AND deleted_at IS NULL`).
		WithArgs(concurrency, rpmLimit, tokenLimit1d, tokenLimit7d, tokenLimit30d, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 2))

	affected, err := repo.BatchUpdateLimits(
		context.Background(),
		[]int64{3, 7},
		&concurrency,
		&rpmLimit,
		&tokenLimit1d,
		&tokenLimit7d,
		&tokenLimit30d,
		false,
	)

	require.NoError(t, err)
	require.Equal(t, 2, affected)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestBatchUpdateLimitsResetsGPTTokenQuotaStartAtDatabaseNow(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &userRepository{sql: db}

	mock.ExpectExec(`UPDATE users SET token_quota_started_at = NOW\(\), updated_at = NOW\(\) WHERE id = ANY\(\$1\) AND deleted_at IS NULL`).
		WithArgs(sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 2))

	affected, err := repo.BatchUpdateLimits(
		context.Background(),
		[]int64{3, 7},
		nil,
		nil,
		nil,
		nil,
		nil,
		true,
	)

	require.NoError(t, err)
	require.Equal(t, 2, affected)
	require.NoError(t, mock.ExpectationsWereMet())
}
