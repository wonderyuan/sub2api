package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestGetLatencyTrendReturnsPercentilesAndFillsMissingBuckets(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &opsRepository{db: db}
	start := time.Date(2026, 7, 25, 8, 0, 0, 0, time.UTC)
	end := start.Add(2 * time.Minute)
	groupID := int64(7)

	rows := sqlmock.NewRows([]string{"bucket", "p50", "p90", "p95", "avg", "max", "sample_count"}).
		AddRow(start, 100.4, 250.2, 400.8, 175.5, int64(900), int64(12))
	mock.ExpectQuery(`(?s)SELECT.*percentile_cont\(0\.50\).*FROM usage_logs ul.*duration_ms IS NOT NULL.*GROUP BY 1`).
		WithArgs(start, end, groupID).
		WillReturnRows(rows)

	result, err := repo.GetLatencyTrend(context.Background(), &service.OpsDashboardFilter{
		StartTime: start,
		EndTime:   end,
		GroupID:   &groupID,
	}, 60)
	require.NoError(t, err)
	require.Equal(t, "1m", result.Bucket)
	require.Len(t, result.Points, 2)
	require.Equal(t, 12, int(result.Points[0].SampleCount))
	require.Equal(t, 100, *result.Points[0].P50)
	require.Equal(t, 250, *result.Points[0].P90)
	require.Equal(t, 401, *result.Points[0].P95)
	require.Equal(t, 176, *result.Points[0].Avg)
	require.Equal(t, 900, *result.Points[0].Max)
	require.Zero(t, result.Points[1].SampleCount)
	require.Nil(t, result.Points[1].P95)
	require.NoError(t, mock.ExpectationsWereMet())
}
