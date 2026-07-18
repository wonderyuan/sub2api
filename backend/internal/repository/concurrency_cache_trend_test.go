package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func newConcurrencyTrendTestCache(t *testing.T) (*concurrencyCache, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	return &concurrencyCache{rdb: rdb, slotTTLSeconds: 900, waitQueueTTLSeconds: 900}, mr
}

func TestConcurrencyTrendMergeKeepsMinuteMaxAndExpires(t *testing.T) {
	cache, _ := newConcurrencyTrendTestCache(t)
	ctx := context.Background()
	bucket := time.Date(2026, 7, 19, 12, 34, 0, 0, time.UTC)

	require.NoError(t, cache.MergeUserConcurrencyTrend(ctx, bucket,
		map[int64]service.ConcurrencyPeak{7: {PeakInUse: 3, PeakWaiting: 2, PeakDemand: 5}},
		service.ConcurrencyPeak{PeakInUse: 8, PeakWaiting: 3, PeakDemand: 11},
	))
	require.NoError(t, cache.MergeUserConcurrencyTrend(ctx, bucket,
		map[int64]service.ConcurrencyPeak{
			7: {PeakInUse: 2, PeakWaiting: 1, PeakDemand: 3},
			9: {PeakInUse: 4, PeakWaiting: 0, PeakDemand: 4},
		},
		service.ConcurrencyPeak{PeakInUse: 6, PeakWaiting: 1, PeakDemand: 7},
	))

	trend, err := cache.GetUserConcurrencyTrend(ctx, bucket.Add(-time.Minute), bucket.Add(time.Minute))
	require.NoError(t, err)
	require.Len(t, trend.Points, 3)
	require.Empty(t, trend.Points[0].Users)
	require.Equal(t, service.ConcurrencyPeak{PeakInUse: 8, PeakWaiting: 3, PeakDemand: 11}, trend.Points[1].System)
	require.Equal(t, service.ConcurrencyPeak{PeakInUse: 3, PeakWaiting: 2, PeakDemand: 5}, trend.Points[1].Users[7])
	require.Equal(t, service.ConcurrencyPeak{PeakInUse: 4, PeakWaiting: 0, PeakDemand: 4}, trend.Points[1].Users[9])
	require.Empty(t, trend.Points[2].Users)

	ttl, err := cache.rdb.TTL(ctx, userConcurrencyTrendKey(bucket)).Result()
	require.NoError(t, err)
	require.Positive(t, ttl)
	require.LessOrEqual(t, ttl, userConcurrencyTrendTTL)
}

func TestConcurrencyStateMethodsReturnObservedCounts(t *testing.T) {
	cache, _ := newConcurrencyTrendTestCache(t)
	ctx := context.Background()

	acquired, active, observedAt, err := cache.AcquireUserSlotWithState(ctx, 42, 2, "request-1")
	require.NoError(t, err)
	require.True(t, acquired)
	require.Equal(t, 1, active)
	require.False(t, observedAt.IsZero())

	acquired, active, _, err = cache.AcquireUserSlotWithState(ctx, 42, 2, "request-2")
	require.NoError(t, err)
	require.True(t, acquired)
	require.Equal(t, 2, active)

	acquired, active, _, err = cache.AcquireUserSlotWithState(ctx, 42, 2, "request-3")
	require.NoError(t, err)
	require.False(t, acquired)
	require.Equal(t, 2, active)

	incremented, waiting, _, err := cache.IncrementWaitCountWithState(ctx, 42, 22)
	require.NoError(t, err)
	require.True(t, incremented)
	require.Equal(t, 1, waiting)

	active, _, err = cache.ReleaseUserSlotWithState(ctx, 42, "request-1")
	require.NoError(t, err)
	require.Equal(t, 1, active)
	waiting, _, err = cache.DecrementWaitCountWithState(ctx, 42)
	require.NoError(t, err)
	require.Zero(t, waiting)

	unlimited, _, err := cache.TrackUserSlotWithState(ctx, 99, "unlimited-1")
	require.NoError(t, err)
	require.Equal(t, 1, unlimited)
	unlimited, _, err = cache.TrackUserSlotWithState(ctx, 99, "unlimited-2")
	require.NoError(t, err)
	require.Equal(t, 2, unlimited)
}

func TestReleaseUserSlotWithStateExcludesExpiredMembers(t *testing.T) {
	cache, _ := newConcurrencyTrendTestCache(t)
	ctx := context.Background()
	userID := int64(42)
	now, err := cache.rdb.Time(ctx).Result()
	require.NoError(t, err)

	require.NoError(t, cache.rdb.ZAdd(ctx, userSlotKey(userID),
		redis.Z{Score: float64(now.Unix() - int64(cache.slotTTLSeconds) - 1), Member: "expired-request"},
		redis.Z{Score: float64(now.Unix()), Member: "current-request"},
	).Err())

	remaining, _, err := cache.ReleaseUserSlotWithState(ctx, userID, "current-request")
	require.NoError(t, err)
	require.Zero(t, remaining)
	require.Zero(t, cache.rdb.Exists(ctx, userSlotKey(userID)).Val())
}
