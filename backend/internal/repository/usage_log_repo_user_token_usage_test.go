//go:build unit

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/stretchr/testify/require"
)

func TestGetUserTokenUsageReturnsDailyAndRollingWindows(t *testing.T) {
	require.NoError(t, timezone.Init("Asia/Shanghai"))
	t.Cleanup(func() { _ = timezone.Init("UTC") })
	db, mock := newSQLMock(t)
	repo := &usageLogRepository{sql: db}
	now := time.Date(2026, 7, 23, 8, 0, 0, 0, time.UTC)
	quotaStartedAt := now.Add(-6 * time.Hour)
	quotaDayStartedAt := time.Date(2026, 7, 23, 7, 0, 0, 0, timezone.Location())

	mock.ExpectQuery(`(?s)FROM usage_logs.*requested_model.*LIKE 'gpt-%'`).
		WithArgs(int64(42), now, quotaStartedAt, quotaDayStartedAt).
		WillReturnRows(sqlmock.NewRows([]string{"usage_1d", "usage_7d", "usage_30d"}).AddRow(100, 700, 3000))

	usage, err := repo.GetUserTokenUsage(context.Background(), 42, now, quotaStartedAt)
	require.NoError(t, err)
	require.EqualValues(t, 100, usage.Usage1d)
	require.EqualValues(t, 700, usage.Usage7d)
	require.EqualValues(t, 3000, usage.Usage30d)
	require.NoError(t, mock.ExpectationsWereMet())
}
