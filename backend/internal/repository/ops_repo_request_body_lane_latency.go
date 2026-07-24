package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func (r *opsRepository) GetRequestBodyLaneLatencySummaries(
	ctx context.Context,
	start, end time.Time,
) (service.RequestBodyLaneLatencySummaries, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT
  request_body_lane,
  percentile_cont(0.50) WITHIN GROUP (ORDER BY duration_ms),
  percentile_cont(0.90) WITHIN GROUP (ORDER BY duration_ms),
  percentile_cont(0.95) WITHIN GROUP (ORDER BY duration_ms),
  AVG(duration_ms),
  MAX(duration_ms)
FROM usage_logs
WHERE created_at >= $1
  AND created_at <= $2
  AND duration_ms IS NOT NULL
  AND request_body_lane IN ('normal', 'heavy', 'recovery')
GROUP BY request_body_lane
`, start, end)
	if err != nil {
		return service.RequestBodyLaneLatencySummaries{}, err
	}
	defer func() { _ = rows.Close() }()

	result := service.RequestBodyLaneLatencySummaries{}
	for rows.Next() {
		var lane string
		var p50, p90, p95, avg sql.NullFloat64
		var max sql.NullInt64
		if err := rows.Scan(&lane, &p50, &p90, &p95, &avg, &max); err != nil {
			return service.RequestBodyLaneLatencySummaries{}, err
		}
		summary := service.OpsPercentiles{
			P50: floatToIntPtr(p50),
			P90: floatToIntPtr(p90),
			P95: floatToIntPtr(p95),
			Avg: floatToIntPtr(avg),
		}
		if max.Valid {
			value := int(max.Int64)
			summary.Max = &value
		}
		switch service.RequestBodyLane(lane) {
		case service.RequestBodyLaneNormal:
			result.Normal = summary
		case service.RequestBodyLaneHeavy:
			result.Heavy = summary
		case service.RequestBodyLaneRecovery:
			result.Recovery = summary
		}
	}
	if err := rows.Err(); err != nil {
		return service.RequestBodyLaneLatencySummaries{}, err
	}
	return result, nil
}
