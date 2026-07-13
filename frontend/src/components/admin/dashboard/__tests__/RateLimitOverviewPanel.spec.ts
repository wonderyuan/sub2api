import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import type { ApiKey } from '@/types'
import RateLimitOverviewPanel from '../RateLimitOverviewPanel.vue'

const { listUsageWindows, refreshUsageWindows, listKeys } = vi.hoisted(() => ({
  listUsageWindows: vi.fn(),
  refreshUsageWindows: vi.fn(),
  listKeys: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      listUsageWindows,
      refreshUsageWindows
    }
  }
}))

vi.mock('@/api/keys', () => ({
  default: { list: listKeys }
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) => {
        if (!params) return key
        return `${key} ${Object.values(params).join(' ')}`
      }
    })
  }
})

const accountItem = {
  id: 11,
  name: 'Codex Primary',
  platform: 'openai',
  type: 'oauth',
  status: 'active',
  five_hour: { utilization: 35, resets_at: null, remaining_seconds: 0 },
  seven_day: { utilization: 68, resets_at: null, remaining_seconds: 0 },
  updated_at: null,
  supports_live_refresh: true
}

const apiKey = {
  id: 21,
  user_id: 7,
  name: 'Production Key',
  status: 'active',
  rate_limit_5h: 25,
  rate_limit_7d: 100,
  usage_5h: 5,
  usage_7d: 40,
  reset_5h_at: null,
  reset_7d_at: null,
  user: { id: 7, username: 'yuan', email: 'yuan@example.com', role: 'user', status: 'active' }
} as ApiKey

describe('RateLimitOverviewPanel', () => {
  beforeEach(() => {
    listUsageWindows.mockReset()
    refreshUsageWindows.mockReset()
    listKeys.mockReset()
    listUsageWindows.mockResolvedValue({ items: [accountItem], total: 1, page: 1, page_size: 10, pages: 1 })
    listKeys.mockResolvedValue({ items: [apiKey], total: 1, page: 1, page_size: 10, pages: 1 })
    refreshUsageWindows.mockResolvedValue([
      {
        ...accountItem,
        five_hour: { utilization: 91, resets_at: null, remaining_seconds: 0 }
      }
    ])
  })

  it('shows account windows and refreshes supported accounts from upstream', async () => {
    const wrapper = mount(RateLimitOverviewPanel)
    await flushPromises()

    expect(wrapper.text()).toContain('Codex Primary')
    expect(wrapper.text()).toContain('35%')
    expect(wrapper.text()).toContain('68%')

    await wrapper.get('[data-testid="live-refresh"]').trigger('click')
    await flushPromises()

    expect(refreshUsageWindows).toHaveBeenCalledWith([11])
    expect(wrapper.text()).toContain('91%')
  })

  it('loads all-system API key limits when the API Key tab is selected', async () => {
    const wrapper = mount(RateLimitOverviewPanel)
    await flushPromises()

    await wrapper.get('[data-testid="keys-tab"]').trigger('click')
    await flushPromises()

    expect(listKeys).toHaveBeenCalledWith(
      1,
      10,
      expect.objectContaining({ sort_by: 'name', sort_order: 'asc' }),
      expect.objectContaining({ signal: expect.any(AbortSignal) })
    )
    expect(wrapper.text()).toContain('Production Key')
    expect(wrapper.text()).toContain('yuan')
    expect(wrapper.text()).toContain('$5.00 / $25.00')
    expect(wrapper.text()).toContain('$40.00 / $100.00')
  })
})
