package securityaudit

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestConversationHeartbeatRenewsWhileScanIsBusy(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	var calls atomic.Int32
	renewed := make(chan struct{})
	err := withConversationReviewHeartbeat(ctx, time.Millisecond, func(context.Context) error {
		if calls.Add(1) == 3 {
			close(renewed)
		}
		return nil
	}, func(ctx context.Context) error {
		select {
		case <-renewed:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})
	require.NoError(t, err)
	require.GreaterOrEqual(t, calls.Load(), int32(3))
}

func TestConversationHeartbeatCancelsScanOnLeaseLoss(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	var calls atomic.Int32
	err := withConversationReviewHeartbeat(ctx, time.Millisecond, func(context.Context) error {
		if calls.Add(1) > 1 {
			return ErrLeaseLost
		}
		return nil
	}, func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() })
	require.ErrorIs(t, err, ErrLeaseLost)
	require.ErrorIs(t, err, context.Canceled)
}

func TestConversationHeartbeatDoesNotScanWithoutLease(t *testing.T) {
	err := withConversationReviewHeartbeat(context.Background(), time.Second,
		func(context.Context) error { return ErrLeaseLost },
		func(context.Context) error { t.Fatal("must not scan after failed renewal"); return nil })
	require.True(t, errors.Is(err, ErrLeaseLost))
}

func TestConversationCleanupRejectsInvalidDisabledConfigRetention(t *testing.T) {
	for _, days := range []int{0, -1, MaxConversationRetentionDays + 1} {
		service := &PromptService{
			config: &fakeConfigStore{active: true, cfg: ActiveConfig{ConversationRetentionDays: days}},
			repo:   NewPostgreSQLRepository(nil), clock: realClock{},
		}
		// A disabled config does not validate conversation limits. Never turn
		// an invalid value into a future cutoff that deletes all archives.
		service.runConversationReviewTick(context.Background())
		require.True(t, service.lastConversationCleanupAt.IsZero())
	}
}
