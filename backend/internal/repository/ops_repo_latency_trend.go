package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func (r *opsRepository) GetLatencyTrend(ctx context.Context, filter *service.OpsDashboardFilter, bucketSeconds int) (*service.OpsLatencyTrendResponse, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil ops repository")
	}
	if filter == nil {
		return nil, fmt.Errorf("nil filter")
	}
	if filter.StartTime.IsZero() || filter.EndTime.IsZero() {
		return nil, fmt.Errorf("start_time/end_time required")
	}

	if bucketSeconds != 60 && bucketSeconds != 300 && bucketSeconds != 3600 {
		bucketSeconds = 60
	}
	start := filter.StartTime.UTC()
	end := filter.EndTime.UTC()
	usageJoin, usageWhere, args, _ := buildUsageWhere(filter, start, end, 1)
	bucketExpr := opsBucketExprForUsage(bucketSeconds)

	rows, err := r.db.QueryContext(ctx, `
SELECT
  `+bucketExpr+` AS bucket,
  percentile_cont(0.50) WITHIN GROUP (ORDER BY duration_ms),
  percentile_cont(0.90) WITHIN GROUP (ORDER BY duration_ms),
  percentile_cont(0.95) WITHIN GROUP (ORDER BY duration_ms),
  AVG(duration_ms),
  MAX(duration_ms),
  COUNT(*)
FROM usage_logs ul
`+usageJoin+`
`+usageWhere+`
  AND duration_ms IS NOT NULL
GROUP BY 1
ORDER BY 1 ASC`, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	points := make([]*service.OpsLatencyTrendPoint, 0, 256)
	for rows.Next() {
		var bucket time.Time
		var p50, p90, p95, avg sql.NullFloat64
		var max sql.NullInt64
		var sampleCount int64
		if err := rows.Scan(&bucket, &p50, &p90, &p95, &avg, &max, &sampleCount); err != nil {
			return nil, err
		}
		point := &service.OpsLatencyTrendPoint{
			BucketStart: bucket.UTC(),
			P50:         floatToIntPtr(p50),
			P90:         floatToIntPtr(p90),
			P95:         floatToIntPtr(p95),
			Avg:         floatToIntPtr(avg),
			SampleCount: sampleCount,
		}
		if max.Valid {
			value := int(max.Int64)
			point.Max = &value
		}
		points = append(points, point)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return &service.OpsLatencyTrendResponse{
		Bucket: opsBucketLabel(bucketSeconds),
		Points: fillOpsLatencyBuckets(start, end, bucketSeconds, points),
	}, nil
}

func fillOpsLatencyBuckets(start, end time.Time, bucketSeconds int, points []*service.OpsLatencyTrendPoint) []*service.OpsLatencyTrendPoint {
	if bucketSeconds <= 0 {
		bucketSeconds = 60
	}
	if !start.Before(end) {
		return points
	}

	lastInstant := end.Add(-time.Nanosecond)
	if lastInstant.Before(start) {
		return points
	}
	first := opsFloorToBucketStart(start, bucketSeconds)
	last := opsFloorToBucketStart(lastInstant, bucketSeconds)
	step := time.Duration(bucketSeconds) * time.Second
	existing := make(map[int64]*service.OpsLatencyTrendPoint, len(points))
	for _, point := range points {
		if point != nil {
			existing[point.BucketStart.UTC().Unix()] = point
		}
	}

	out := make([]*service.OpsLatencyTrendPoint, 0, int(last.Sub(first)/step)+1)
	for cursor := first; !cursor.After(last); cursor = cursor.Add(step) {
		if point := existing[cursor.Unix()]; point != nil {
			out = append(out, point)
			continue
		}
		out = append(out, &service.OpsLatencyTrendPoint{BucketStart: cursor})
	}
	return out
}
