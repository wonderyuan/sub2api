import { flushPromises, mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import OpsUserConcurrencyTrendChart from '../OpsUserConcurrencyTrendChart.vue'

const { getUserConcurrencyTrend } = vi.hoisted(() => ({
  getUserConcurrencyTrend: vi.fn()
}))

vi.mock('@/api/admin/ops', () => ({
  opsAPI: { getUserConcurrencyTrend }
}))

vi.mock('chart.js', () => ({
  Chart: { register: vi.fn() },
  CategoryScale: {},
  Legend: {},
  LineElement: {},
  LinearScale: {},
  PointElement: {},
  Tooltip: {}
}))

vi.mock('vue-chartjs', () => ({
  Line: defineComponent({
    name: 'LineChartStub',
    props: { data: { type: Object, required: true }, options: { type: Object, required: true } },
    template: '<div class="line-chart-stub" />'
  })
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, params?: Record<string, unknown>) => params ? `${key}:${JSON.stringify(params)}` : key
  })
}))

describe('OpsUserConcurrencyTrendChart', () => {
  it('shows system and ranked user demand series and can pin a selected user', async () => {
    getUserConcurrencyTrend.mockResolvedValue({
      enabled: true,
      bucket: 'minute',
      current: { in_use: 7, waiting: 2, demand: 9 },
      points: [
        {
          bucket_start: '2026-07-19T12:00:00Z',
          system: { peak_in_use: 7, peak_waiting: 2, peak_demand: 9 },
          users: {
            '1': { peak_in_use: 4, peak_waiting: 1, peak_demand: 5 },
            '2': { peak_in_use: 2, peak_waiting: 0, peak_demand: 2 },
            '6': { peak_in_use: 1, peak_waiting: 1, peak_demand: 2 }
          }
        }
      ],
      users: {
        '1': { user_id: 1, username: 'alpha', user_email: 'a@example.com', max_capacity: 5 },
        '2': { user_id: 2, username: 'beta', user_email: 'b@example.com', max_capacity: 3 },
        '6': { user_id: 6, username: 'zeta', user_email: 'z@example.com', max_capacity: 2 }
      }
    })

    const wrapper = mount(OpsUserConcurrencyTrendChart)
    await flushPromises()

    const chart = wrapper.findComponent({ name: 'LineChartStub' })
    expect(chart.exists()).toBe(true)
    expect(chart.props('data').datasets.map((dataset: any) => dataset.label)).toEqual([
      'admin.ops.concurrencyTrend.systemDemand',
      'admin.ops.concurrencyTrend.systemWaiting',
      'alpha',
      'beta',
      'zeta'
    ])

    await wrapper.find('select').setValue('6')
    expect(wrapper.text()).toContain('zeta')
  })
})
