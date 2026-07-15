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

const higherUsageApiKey = {
  ...apiKey,
  id: 23,
  name: 'High Usage Key',
  usage_5h: 10,
  usage_7d: 70
} as ApiKey

const otherOwnerApiKey = {
  ...apiKey,
  id: 22,
  user_id: 8,
  name: 'Team Key',
  usage_5h: 15,
  usage_7d: 60,
  user: { id: 8, username: 'alice', email: 'alice@example.com', role: 'user', status: 'active' }
} as ApiKey

describe('RateLimitOverviewPanel', () => {
  beforeEach(() => {
    listUsageWindows.mockReset()
    refreshUsageWindows.mockReset()
    listKeys.mockReset()
    listUsageWindows.mockResolvedValue({ items: [accountItem], total: 1, page: 1, page_size: 10, pages: 1 })
    listKeys.mockResolvedValue({
      items: [apiKey, otherOwnerApiKey, higherUsageApiKey],
      total: 3,
      page: 1,
      page_size: 10,
      pages: 1
    })
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

  it('groups API keys by owner and sorts groups and keys by usage', async () => {
    const wrapper = mount(RateLimitOverviewPanel)
    await flushPromises()

    await wrapper.get('[data-testid="keys-tab"]').trigger('click')
    await flushPromises()

    expect(listKeys).toHaveBeenCalledWith(
      1,
      10,
      expect.objectContaining({ sort_by: 'usage_7d', sort_order: 'desc' }),
      expect.objectContaining({ signal: expect.any(AbortSignal) })
    )

    const groups = wrapper.findAll('[data-testid="key-group"]')
    expect(groups).toHaveLength(2)
    expect(groups[0].get('[data-testid="key-group-owner"]').text()).toBe('yuan')
    expect(groups[0].get('[data-testid="key-group-usage"]').text()).toContain('$110.00')
    expect(groups[0].findAll('[data-testid="key-row"]').map((row) => row.text())).toEqual([
      expect.stringContaining('High Usage Key'),
      expect.stringContaining('Production Key')
    ])
    expect(groups[1].get('[data-testid="key-group-owner"]').text()).toBe('alice')
    expect(wrapper.text()).toContain('Production Key')
    expect(wrapper.text()).toContain('yuan')
    expect(wrapper.text()).toContain('$5.00 / $25.00')
    expect(wrapper.text()).toContain('$40.00 / $100.00')
  })
})
