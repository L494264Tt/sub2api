//go:build unit

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

type batchLimitsUserRepoStub struct {
	*userRepoStub
	calls           int
	userIDs         []int64
	concurrency     *int
	rpmLimit        *int
	tokenLimit1d    *int64
	tokenLimit7d    *int64
	tokenLimit30d   *int64
	resetTokenQuota bool
	affected        int
	err             error
}

func (s *batchLimitsUserRepoStub) BatchUpdateLimits(_ context.Context, userIDs []int64, concurrency, rpmLimit *int, tokenLimit1d, tokenLimit7d, tokenLimit30d *int64, resetTokenQuota bool) (int, error) {
	s.calls++
	s.userIDs = append([]int64(nil), userIDs...)
	s.concurrency = cloneBatchLimitValue(concurrency)
	s.rpmLimit = cloneBatchLimitValue(rpmLimit)
	s.tokenLimit1d = cloneBatchTokenLimitValue(tokenLimit1d)
	s.tokenLimit7d = cloneBatchTokenLimitValue(tokenLimit7d)
	s.tokenLimit30d = cloneBatchTokenLimitValue(tokenLimit30d)
	s.resetTokenQuota = resetTokenQuota
	return s.affected, s.err
}

func cloneBatchTokenLimitValue(value *int64) *int64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneBatchLimitValue(value *int) *int {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func TestAdminServiceBatchUpdateLimitsPassesOnlyProvidedFields(t *testing.T) {
	concurrency := 0
	repo := &batchLimitsUserRepoStub{
		userRepoStub: &userRepoStub{},
		affected:     2,
	}
	invalidator := &authCacheInvalidatorStub{}
	service := &adminServiceImpl{userRepo: repo, authCacheInvalidator: invalidator}

	affected, err := service.BatchUpdateLimits(
		context.Background(),
		[]int64{3, 0, 3, 7, -1},
		&concurrency,
		nil,
		nil,
		nil,
		nil,
		false,
	)

	require.NoError(t, err)
	require.Equal(t, 2, affected)
	require.Equal(t, []int64{3, 7}, repo.userIDs)
	require.Equal(t, pointerToInt(0), repo.concurrency)
	require.Nil(t, repo.rpmLimit)
	require.Equal(t, []int64{3, 7}, invalidator.userIDs)
}

func TestAdminServiceBatchUpdateLimitsDoesNotInvalidateCacheOnRepositoryError(t *testing.T) {
	rpmLimit := 60
	repo := &batchLimitsUserRepoStub{
		userRepoStub: &userRepoStub{},
		err:          errors.New("database unavailable"),
	}
	invalidator := &authCacheInvalidatorStub{}
	service := &adminServiceImpl{userRepo: repo, authCacheInvalidator: invalidator}

	affected, err := service.BatchUpdateLimits(context.Background(), []int64{1, 2}, nil, &rpmLimit, nil, nil, nil, false)

	require.EqualError(t, err, "database unavailable")
	require.Zero(t, affected)
	require.Empty(t, invalidator.userIDs)
}

func TestAdminServiceBatchUpdateLimitsRequiresAField(t *testing.T) {
	repo := &batchLimitsUserRepoStub{userRepoStub: &userRepoStub{}}
	service := &adminServiceImpl{userRepo: repo, authCacheInvalidator: &authCacheInvalidatorStub{}}

	affected, err := service.BatchUpdateLimits(context.Background(), []int64{1}, nil, nil, nil, nil, nil, false)

	require.Error(t, err)
	require.Zero(t, affected)
	require.Zero(t, repo.calls)
}

func TestAdminServiceBatchUpdateLimitsPassesGPTTokenLimits(t *testing.T) {
	daily := int64(100_000_000)
	weekly := int64(400_000_000)
	unlimited := int64(0)
	repo := &batchLimitsUserRepoStub{userRepoStub: &userRepoStub{}, affected: 2}
	service := &adminServiceImpl{userRepo: repo, authCacheInvalidator: &authCacheInvalidatorStub{}}

	affected, err := service.BatchUpdateLimits(
		context.Background(),
		[]int64{3, 7},
		nil,
		nil,
		&daily,
		&weekly,
		&unlimited,
		false,
	)

	require.NoError(t, err)
	require.Equal(t, 2, affected)
	require.Equal(t, &daily, repo.tokenLimit1d)
	require.Equal(t, &weekly, repo.tokenLimit7d)
	require.Equal(t, &unlimited, repo.tokenLimit30d)
}

func TestAdminServiceBatchUpdateLimitsResetsGPTTokenQuotaAndInvalidatesCache(t *testing.T) {
	repo := &batchLimitsUserRepoStub{userRepoStub: &userRepoStub{}, affected: 2}
	invalidator := &authCacheInvalidatorStub{}
	service := &adminServiceImpl{userRepo: repo, authCacheInvalidator: invalidator}

	affected, err := service.BatchUpdateLimits(
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
	require.True(t, repo.resetTokenQuota)
	require.Equal(t, []int64{3, 7}, invalidator.userIDs)
}

func pointerToInt(value int) *int {
	return &value
}
