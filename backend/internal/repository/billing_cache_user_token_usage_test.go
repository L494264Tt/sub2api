//go:build unit

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestUserTokenUsageCacheSetGetAndIncrement(t *testing.T) {
	cache, _ := newMiniRedisCache(t)
	ctx := context.Background()
	quotaStartedAt := time.Date(2026, 7, 23, 5, 0, 0, 0, time.UTC)
	quotaDayStartedAt := time.Date(2026, 7, 23, 7, 0, 0, 0, time.UTC)

	usage := &service.UserTokenUsage{Usage1d: 10, Usage7d: 20, Usage30d: 30}
	require.NoError(t, cache.SetUserTokenUsageCache(ctx, 42, quotaStartedAt, quotaDayStartedAt, usage, time.Minute))
	require.NoError(t, cache.IncrementUserTokenUsageCache(ctx, 42, quotaStartedAt, quotaDayStartedAt, 7))

	got, hit, err := cache.GetUserTokenUsageCache(ctx, 42, quotaStartedAt, quotaDayStartedAt)
	require.NoError(t, err)
	require.True(t, hit)
	require.Equal(t, &service.UserTokenUsage{Usage1d: 17, Usage7d: 27, Usage30d: 37}, got)
}

func TestUserTokenUsageCacheIncrementMissIsNoop(t *testing.T) {
	cache, _ := newMiniRedisCache(t)
	ctx := context.Background()
	quotaStartedAt := time.Date(2026, 7, 23, 5, 0, 0, 0, time.UTC)
	quotaDayStartedAt := time.Date(2026, 7, 23, 7, 0, 0, 0, time.UTC)

	require.NoError(t, cache.IncrementUserTokenUsageCache(ctx, 42, quotaStartedAt, quotaDayStartedAt, 7))
	got, hit, err := cache.GetUserTokenUsageCache(ctx, 42, quotaStartedAt, quotaDayStartedAt)
	require.NoError(t, err)
	require.False(t, hit)
	require.Nil(t, got)
}

func TestUserTokenUsageCacheIsolatedByQuotaStart(t *testing.T) {
	cache, _ := newMiniRedisCache(t)
	ctx := context.Background()
	oldStart := time.Date(2026, 7, 23, 5, 0, 0, 0, time.UTC)
	newStart := oldStart.Add(time.Minute)
	quotaDayStartedAt := time.Date(2026, 7, 23, 7, 0, 0, 0, time.UTC)

	require.NoError(t, cache.SetUserTokenUsageCache(
		ctx,
		42,
		oldStart,
		quotaDayStartedAt,
		&service.UserTokenUsage{Usage1d: 80_000_000},
		time.Minute,
	))

	got, hit, err := cache.GetUserTokenUsageCache(ctx, 42, newStart, quotaDayStartedAt)
	require.NoError(t, err)
	require.False(t, hit)
	require.Nil(t, got)
}

func TestUserTokenUsageCacheIsolatedByQuotaDay(t *testing.T) {
	cache, _ := newMiniRedisCache(t)
	ctx := context.Background()
	quotaStartedAt := time.Date(2026, 7, 23, 5, 0, 0, 0, time.UTC)
	oldQuotaDay := time.Date(2026, 7, 23, 7, 0, 0, 0, time.UTC)
	newQuotaDay := oldQuotaDay.Add(24 * time.Hour)

	require.NoError(t, cache.SetUserTokenUsageCache(
		ctx,
		42,
		quotaStartedAt,
		oldQuotaDay,
		&service.UserTokenUsage{Usage1d: 80_000_000},
		time.Minute,
	))

	got, hit, err := cache.GetUserTokenUsageCache(ctx, 42, quotaStartedAt, newQuotaDay)
	require.NoError(t, err)
	require.False(t, hit)
	require.Nil(t, got)
}

func TestUserTokenUsageCacheCorruptValueFallsBackToMiss(t *testing.T) {
	cache, _ := newMiniRedisCache(t)
	ctx := context.Background()
	quotaStartedAt := time.Date(2026, 7, 23, 5, 0, 0, 0, time.UTC)
	quotaDayStartedAt := time.Date(2026, 7, 23, 7, 0, 0, 0, time.UTC)
	require.NoError(t, cache.rdb.HSet(ctx, userTokenUsageCacheKey(42, quotaStartedAt, quotaDayStartedAt),
		"usage_1d", "invalid", "usage_7d", 20, "usage_30d", 30,
	).Err())

	got, hit, err := cache.GetUserTokenUsageCache(ctx, 42, quotaStartedAt, quotaDayStartedAt)
	require.Error(t, err)
	require.False(t, hit)
	require.Nil(t, got)
}
