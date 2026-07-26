import { afterEach, describe, expect, it, vi } from 'vitest'

import { monitorCenterAPI } from '../monitorCenter'
import { opsAPI } from '../ops'

describe('monitorCenterAPI.getRangeData', () => {
  afterEach(() => vi.restoreAllMocks())

  it('returns successful modules when one ops request fails', async () => {
    vi.spyOn(opsAPI, 'getDashboardOverview').mockResolvedValue({ health_score: 92 } as never)
    vi.spyOn(opsAPI, 'getLatencyTrend').mockRejectedValue(new Error('latency unavailable'))
    vi.spyOn(opsAPI, 'getUserConcurrencyTrend').mockResolvedValue({ points: [] } as never)
    vi.spyOn(opsAPI, 'getPerformanceDiagnostics').mockResolvedValue({ trend: [] } as never)
    vi.spyOn(opsAPI, 'getErrorTrend').mockResolvedValue({ points: [] } as never)
    vi.spyOn(opsAPI, 'getThroughputTrend').mockResolvedValue({ points: [] } as never)

    const result = await monitorCenterAPI.getRangeData({ time_range: '1h' })

    expect(result.success_count).toBe(5)
    expect(result.failure_count).toBe(1)
    expect(result.data.overview).toEqual({ health_score: 92 })
    expect(result.data.latency).toBeUndefined()
    expect(result.data.concurrency).toEqual({ points: [] })
  })
})
