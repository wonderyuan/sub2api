package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRecordConcurrencyTrendSampleKeepsPerUserAndSystemPeaks(t *testing.T) {
	pending := make(map[time.Time]map[int64]ConcurrencyPeak)
	system := make(map[time.Time]ConcurrencyPeak)
	bucket := time.Date(2026, 7, 19, 12, 34, 0, 0, time.UTC)
	live := map[int64]userConcurrencyLiveState{
		1: {active: 3, waiting: 2},
		2: {active: 4, waiting: 0},
	}

	recordConcurrencyTrendSample(pending, system, bucket.Add(10*time.Second), live, 0, 7, 2)
	live[1] = userConcurrencyLiveState{active: 2, waiting: 4}
	recordConcurrencyTrendSample(pending, system, bucket.Add(20*time.Second), live, 1, 6, 4)

	require.Equal(t, ConcurrencyPeak{PeakInUse: 3, PeakWaiting: 4, PeakDemand: 6}, pending[bucket][1])
	require.Equal(t, ConcurrencyPeak{PeakInUse: 4, PeakWaiting: 0, PeakDemand: 4}, pending[bucket][2])
	require.Equal(t, ConcurrencyPeak{PeakInUse: 7, PeakWaiting: 4, PeakDemand: 10}, system[bucket])
}

func TestConcurrencyServiceReturnsSixtyEmptyBucketsWithoutTrendCache(t *testing.T) {
	svc := NewConcurrencyService(nil)
	now := time.Date(2026, 7, 19, 12, 34, 45, 0, time.UTC)

	trend, err := svc.GetUserConcurrencyTrend(t.Context(), now)
	require.NoError(t, err)
	require.Equal(t, "minute", trend.Bucket)
	require.Len(t, trend.Points, 60)
	require.Equal(t, now.Truncate(time.Minute).Add(-59*time.Minute), trend.Points[0].BucketStart)
	require.Equal(t, now.Truncate(time.Minute), trend.Points[59].BucketStart)
}
