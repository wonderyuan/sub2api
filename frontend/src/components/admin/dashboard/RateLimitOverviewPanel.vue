<template>
  <section class="card overflow-hidden" data-testid="rate-limit-overview">
    <div class="border-b border-gray-100 p-4 dark:border-dark-700/70">
      <div class="flex flex-col gap-4 xl:flex-row xl:items-center xl:justify-between">
        <div class="min-w-0">
          <div class="flex items-center gap-2">
            <span class="flex h-8 w-8 items-center justify-center rounded-lg bg-teal-100 text-teal-600 dark:bg-teal-900/30 dark:text-teal-400">
              <Icon name="chartBar" size="sm" :stroke-width="2" />
            </span>
            <div>
              <h2 class="text-sm font-semibold text-gray-900 dark:text-white">
                {{ t('admin.dashboard.rateLimits.title') }}
              </h2>
              <p class="text-xs text-gray-500 dark:text-gray-400">
                {{ t('admin.dashboard.rateLimits.total', { count: activeTotal }) }}
              </p>
            </div>
          </div>
        </div>

        <div class="flex flex-col gap-3 sm:flex-row sm:items-center">
          <div class="inline-flex h-9 rounded-lg bg-gray-100 p-1 dark:bg-dark-700" role="tablist">
            <button
              type="button"
              role="tab"
              data-testid="accounts-tab"
              class="min-w-[112px] rounded-md px-3 text-xs font-medium transition-colors"
              :class="activeTab === 'accounts' ? activeTabClass : inactiveTabClass"
              :aria-selected="activeTab === 'accounts'"
              @click="selectTab('accounts')"
            >
              {{ t('admin.dashboard.rateLimits.aiAccounts') }}
            </button>
            <button
              type="button"
              role="tab"
              data-testid="keys-tab"
              class="min-w-[96px] rounded-md px-3 text-xs font-medium transition-colors"
              :class="activeTab === 'keys' ? activeTabClass : inactiveTabClass"
              :aria-selected="activeTab === 'keys'"
              @click="selectTab('keys')"
            >
              {{ t('admin.dashboard.rateLimits.apiKeys') }}
            </button>
          </div>

          <div class="flex min-w-0 items-center gap-2">
            <div class="min-w-0 flex-1 sm:w-56 sm:flex-none">
              <SearchInput
                v-model="activeSearch"
                :placeholder="t('admin.dashboard.rateLimits.searchPlaceholder')"
                @search="handleSearch"
              />
            </div>
            <button
              type="button"
              class="btn btn-secondary h-9 w-9 flex-shrink-0 p-0"
              :title="t('common.refresh')"
              :aria-label="t('common.refresh')"
              :disabled="activeLoading"
              @click="loadActiveTab"
            >
              <Icon name="refresh" size="sm" :class="activeLoading ? 'animate-spin' : ''" />
            </button>
            <button
              v-if="activeTab === 'accounts'"
              type="button"
              data-testid="live-refresh"
              class="btn btn-secondary h-9 flex-shrink-0 rounded-lg px-3 py-0 text-xs"
              :title="t('admin.dashboard.rateLimits.liveRefreshHint')"
              :disabled="liveRefreshing || activeLoading || refreshableAccountIds.length === 0"
              @click="refreshUpstream"
            >
              <Icon
                :name="liveRefreshing ? 'refresh' : 'cloud'"
                size="sm"
                :class="liveRefreshing ? 'animate-spin' : ''"
              />
              <span class="hidden sm:inline">{{ t('admin.dashboard.rateLimits.liveRefresh') }}</span>
            </button>
          </div>
        </div>
      </div>

      <div
        v-if="liveMessage"
        class="mt-3 flex items-center gap-2 rounded-lg bg-gray-50 px-3 py-2 text-xs text-gray-600 dark:bg-dark-700/60 dark:text-gray-300"
        role="status"
      >
        <Icon name="check" size="xs" class="text-emerald-500" />
        {{ liveMessage }}
      </div>
    </div>

    <div class="grid hidden-cols border-b border-gray-100 bg-gray-50/70 px-4 py-2.5 text-[11px] font-semibold uppercase text-gray-500 dark:border-dark-700/70 dark:bg-dark-800/70 dark:text-gray-400 md:grid">
      <span>{{ activeTab === 'accounts' ? t('admin.dashboard.rateLimits.account') : t('admin.dashboard.rateLimits.apiKey') }}</span>
      <span>{{ t('admin.dashboard.rateLimits.status') }}</span>
      <span>{{ t('admin.dashboard.rateLimits.fiveHour') }}</span>
      <span>{{ t('admin.dashboard.rateLimits.sevenDay') }}</span>
    </div>

    <div v-if="activeLoading && activeItems.length === 0" class="flex h-48 items-center justify-center">
      <LoadingSpinner size="md" />
    </div>
    <div v-else-if="activeError" class="flex h-48 flex-col items-center justify-center gap-3 px-4 text-center">
      <p class="text-sm text-red-600 dark:text-red-400">{{ activeError }}</p>
      <button type="button" class="btn btn-secondary py-2 text-xs" @click="loadActiveTab">
        <Icon name="refresh" size="sm" />
        {{ t('admin.dashboard.rateLimits.retry') }}
      </button>
    </div>
    <div v-else-if="activeItems.length === 0" class="flex h-48 flex-col items-center justify-center gap-2 px-4 text-center">
      <Icon :name="activeTab === 'accounts' ? 'server' : 'key'" size="lg" class="text-gray-300 dark:text-gray-600" />
      <p class="text-sm text-gray-500 dark:text-gray-400">
        {{ t('admin.dashboard.rateLimits.empty') }}
      </p>
    </div>

    <div v-else class="divide-y divide-gray-100 dark:divide-dark-700/70" :aria-busy="activeLoading">
      <template v-if="activeTab === 'accounts'">
      <div
        v-for="item in accountItems"
        :key="item.id"
        class="grid data-cols gap-3 px-4 py-3.5 transition-colors hover:bg-gray-50/70 dark:hover:bg-dark-700/30"
        data-testid="account-row"
      >
        <div class="min-w-0">
          <p class="truncate text-sm font-semibold text-gray-900 dark:text-white" :title="item.name">
            {{ item.name }}
          </p>
          <div class="mt-1 flex min-w-0 items-center gap-1.5 text-xs text-gray-500 dark:text-gray-400">
            <span class="capitalize">{{ item.platform }}</span>
            <span class="text-gray-300 dark:text-gray-600">/</span>
            <span class="truncate">{{ item.type }}</span>
            <span v-if="item.updated_at" class="text-gray-300 dark:text-gray-600">/</span>
            <span v-if="item.updated_at" class="truncate" :title="formatDateTime(item.updated_at)">
              {{ formatRelativeTime(item.updated_at) }}
            </span>
          </div>
          <p v-if="item.refresh_error" class="mt-1 truncate text-[11px] text-red-500">
            {{ refreshErrorLabel(item.refresh_error) }}
          </p>
        </div>
        <div class="flex items-start md:items-center">
          <span class="rounded-md px-2 py-1 text-[11px] font-medium" :class="statusClass(item.status)">
            {{ statusLabel(item.status) }}
          </span>
        </div>
        <div>
          <p class="mb-1 text-[11px] font-medium text-gray-400 md:hidden">{{ t('admin.dashboard.rateLimits.fiveHour') }}</p>
          <RateLimitGauge
            :utilization="item.five_hour?.utilization"
            :resets-at="item.five_hour?.resets_at"
          />
        </div>
        <div>
          <p class="mb-1 text-[11px] font-medium text-gray-400 md:hidden">{{ t('admin.dashboard.rateLimits.sevenDay') }}</p>
          <RateLimitGauge
            :utilization="item.seven_day?.utilization"
            :resets-at="item.seven_day?.resets_at"
          />
        </div>
      </div>
      </template>

      <template v-else>
      <section
        v-for="group in keyGroups"
        :key="group.userId"
        data-testid="key-group"
        class="divide-y divide-gray-100 dark:divide-dark-700/70"
      >
        <div class="flex min-w-0 flex-col gap-2 bg-gray-50/80 px-4 py-2.5 dark:bg-dark-800/70 sm:flex-row sm:items-center sm:justify-between sm:gap-3">
          <div class="flex min-w-0 items-center gap-2">
            <span class="flex h-7 w-7 flex-shrink-0 items-center justify-center rounded-md bg-white text-gray-500 shadow-sm dark:bg-dark-700 dark:text-gray-300">
              <Icon name="user" size="xs" />
            </span>
            <div class="min-w-0">
              <p
                data-testid="key-group-owner"
                class="truncate text-xs font-semibold text-gray-800 dark:text-gray-100"
                :title="group.ownerTitle"
              >
                {{ group.owner }}
              </p>
              <p class="text-[11px] text-gray-400 dark:text-gray-500">
                {{ t('admin.dashboard.rateLimits.keyCount', { count: group.items.length }) }}
              </p>
            </div>
          </div>
          <span
            data-testid="key-group-usage"
            class="text-xs font-semibold text-gray-600 dark:text-gray-300 sm:flex-shrink-0"
          >
            {{ t('admin.dashboard.rateLimits.groupUsage', { amount: formatUsd(group.usage7d) }) }}
          </span>
        </div>
        <div
          v-for="item in group.items"
          :key="item.id"
          class="grid data-cols gap-3 px-4 py-3.5 transition-colors hover:bg-gray-50/70 dark:hover:bg-dark-700/30"
          data-testid="key-row"
        >
          <div class="min-w-0">
            <p class="truncate text-sm font-semibold text-gray-900 dark:text-white" :title="item.name">
              {{ item.name }}
            </p>
          </div>
          <div class="flex items-start md:items-center">
            <span class="rounded-md px-2 py-1 text-[11px] font-medium" :class="statusClass(item.status)">
              {{ statusLabel(item.status) }}
            </span>
          </div>
          <div>
            <p class="mb-1 text-[11px] font-medium text-gray-400 md:hidden">{{ t('admin.dashboard.rateLimits.fiveHour') }}</p>
            <RateLimitGauge
              :utilization="keyUtilization(item.usage_5h, item.rate_limit_5h)"
              :summary="formatKeyUsage(item.usage_5h, item.rate_limit_5h)"
              :resets-at="item.reset_5h_at"
              :unlimited="item.rate_limit_5h <= 0"
            />
          </div>
          <div>
            <p class="mb-1 text-[11px] font-medium text-gray-400 md:hidden">{{ t('admin.dashboard.rateLimits.sevenDay') }}</p>
            <RateLimitGauge
              :utilization="keyUtilization(item.usage_7d, item.rate_limit_7d)"
              :summary="formatKeyUsage(item.usage_7d, item.rate_limit_7d)"
              :resets-at="item.reset_7d_at"
              :unlimited="item.rate_limit_7d <= 0"
            />
          </div>
        </div>
      </section>
      </template>
    </div>

    <Pagination
      v-if="activeTotal > pageSize"
      :total="activeTotal"
      :page="activePage"
      :page-size="pageSize"
      :show-page-size-selector="false"
      @update:page="changePage"
    />
  </section>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import keysAPI from '@/api/keys'
import type { AccountUsageWindowItem } from '@/api/admin/accounts'
import type { ApiKey } from '@/types'
import { formatDateTime, formatRelativeTime } from '@/utils/format'
import Icon from '@/components/icons/Icon.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import Pagination from '@/components/common/Pagination.vue'
import SearchInput from '@/components/common/SearchInput.vue'
import RateLimitGauge from './RateLimitGauge.vue'

type PanelTab = 'accounts' | 'keys'

interface ApiKeyOwnerGroup {
  userId: number
  owner: string
  ownerTitle: string
  usage5h: number
  usage7d: number
  items: ApiKey[]
}

const { t } = useI18n()
const pageSize = 10
const activeTab = ref<PanelTab>('accounts')
const accountItems = ref<AccountUsageWindowItem[]>([])
const keyItems = ref<ApiKey[]>([])
const accountTotal = ref(0)
const keyTotal = ref(0)
const accountPage = ref(1)
const keyPage = ref(1)
const accountSearch = ref('')
const keySearch = ref('')
const accountLoading = ref(false)
const keyLoading = ref(false)
const accountLoaded = ref(false)
const keyLoaded = ref(false)
const accountError = ref('')
const keyError = ref('')
const liveRefreshing = ref(false)
const liveMessage = ref('')
let accountController: AbortController | null = null
let keyController: AbortController | null = null

const activeTabClass = 'bg-white text-gray-900 shadow-sm dark:bg-dark-600 dark:text-white'
const inactiveTabClass = 'text-gray-500 hover:text-gray-800 dark:text-gray-400 dark:hover:text-gray-200'
const activeItems = computed(() => activeTab.value === 'accounts' ? accountItems.value : keyItems.value)
const activeTotal = computed(() => activeTab.value === 'accounts' ? accountTotal.value : keyTotal.value)
const activePage = computed(() => activeTab.value === 'accounts' ? accountPage.value : keyPage.value)
const activeLoading = computed(() => activeTab.value === 'accounts' ? accountLoading.value : keyLoading.value)
const activeError = computed(() => activeTab.value === 'accounts' ? accountError.value : keyError.value)
const keyGroups = computed<ApiKeyOwnerGroup[]>(() => {
  const groups = new Map<number, ApiKeyOwnerGroup>()

  for (const key of keyItems.value) {
    let group = groups.get(key.user_id)
    if (!group) {
      group = {
        userId: key.user_id,
        owner: ownerLabel(key),
        ownerTitle: ownerTitle(key),
        usage5h: 0,
        usage7d: 0,
        items: []
      }
      groups.set(key.user_id, group)
    }
    group.usage5h += key.usage_5h
    group.usage7d += key.usage_7d
    group.items.push(key)
  }

  return Array.from(groups.values())
    .map((group) => ({
      ...group,
      items: [...group.items].sort(compareKeyUsage)
    }))
    .sort((a, b) =>
      b.usage7d - a.usage7d
      || b.usage5h - a.usage5h
      || a.owner.localeCompare(b.owner)
      || a.userId - b.userId
    )
})
const activeSearch = computed({
  get: () => activeTab.value === 'accounts' ? accountSearch.value : keySearch.value,
  set: (value: string) => {
    if (activeTab.value === 'accounts') accountSearch.value = value
    else keySearch.value = value
  }
})
const refreshableAccountIds = computed(() => accountItems.value.filter((item) => item.supports_live_refresh).map((item) => item.id))

async function loadAccounts(): Promise<void> {
  accountController?.abort()
  const controller = new AbortController()
  accountController = controller
  accountLoading.value = true
  accountError.value = ''
  liveMessage.value = ''
  try {
    const response = await adminAPI.accounts.listUsageWindows(
      accountPage.value,
      pageSize,
      accountSearch.value.trim(),
      { signal: controller.signal }
    )
    if (accountController !== controller) return
    accountItems.value = response.items
    accountTotal.value = response.total
    accountLoaded.value = true
  } catch (error: any) {
    if (accountController !== controller) return
    if (error?.name === 'CanceledError' || error?.name === 'AbortError') return
    accountError.value = error?.message || t('admin.dashboard.rateLimits.loadFailed')
  } finally {
    if (accountController === controller) accountLoading.value = false
  }
}

async function loadKeys(): Promise<void> {
  keyController?.abort()
  const controller = new AbortController()
  keyController = controller
  keyLoading.value = true
  keyError.value = ''
  try {
    const response = await keysAPI.list(
      keyPage.value,
      pageSize,
      { search: keySearch.value.trim() || undefined, sort_by: 'usage_7d', sort_order: 'desc' },
      { signal: controller.signal }
    )
    if (keyController !== controller) return
    keyItems.value = response.items
    keyTotal.value = response.total
    keyLoaded.value = true
  } catch (error: any) {
    if (keyController !== controller) return
    if (error?.name === 'CanceledError' || error?.name === 'AbortError') return
    keyError.value = error?.message || t('admin.dashboard.rateLimits.loadFailed')
  } finally {
    if (keyController === controller) keyLoading.value = false
  }
}

function loadActiveTab(): Promise<void> {
  return activeTab.value === 'accounts' ? loadAccounts() : loadKeys()
}

function selectTab(tab: PanelTab): void {
  activeTab.value = tab
  liveMessage.value = ''
  if (tab === 'accounts' && !accountLoaded.value) void loadAccounts()
  if (tab === 'keys' && !keyLoaded.value) void loadKeys()
}

function handleSearch(): void {
  if (activeTab.value === 'accounts') accountPage.value = 1
  else keyPage.value = 1
  void loadActiveTab()
}

function changePage(page: number): void {
  if (activeTab.value === 'accounts') accountPage.value = page
  else keyPage.value = page
  void loadActiveTab()
}

async function refreshUpstream(): Promise<void> {
  const ids = refreshableAccountIds.value
  if (ids.length === 0 || liveRefreshing.value) return
  liveRefreshing.value = true
  liveMessage.value = ''
  try {
    const refreshed = await adminAPI.accounts.refreshUsageWindows(ids)
    const byId = new Map(refreshed.map((item) => [item.id, item]))
    accountItems.value = accountItems.value.map((item) => byId.get(item.id) ?? item)
    const failures = refreshed.filter((item) => item.refresh_error).length
    const successes = refreshed.length - failures
    liveMessage.value = failures > 0
      ? t('admin.dashboard.rateLimits.livePartial', { success: successes, failed: failures })
      : t('admin.dashboard.rateLimits.liveSuccess', { count: successes })
  } catch (error: any) {
    liveMessage.value = error?.message || t('admin.dashboard.rateLimits.liveFailed')
  } finally {
    liveRefreshing.value = false
  }
}

function formatUsd(value: number): string {
  return `$${value.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`
}

function formatKeyUsage(used: number, limit: number): string {
  if (limit <= 0) return t('admin.dashboard.rateLimits.unlimited')
  return `${formatUsd(used)} / ${formatUsd(limit)}`
}

function keyUtilization(used: number, limit: number): number | null {
  if (limit <= 0) return null
  return (used / limit) * 100
}

function compareKeyUsage(a: ApiKey, b: ApiKey): number {
  return b.usage_7d - a.usage_7d
    || b.usage_5h - a.usage_5h
    || a.name.localeCompare(b.name)
    || a.id - b.id
}

function ownerLabel(key: ApiKey): string {
  return key.user?.username?.trim() || key.user?.email?.trim() || t('admin.dashboard.rateLimits.userId', { id: key.user_id })
}

function ownerTitle(key: ApiKey): string {
  if (!key.user) return ownerLabel(key)
  return [key.user.username, key.user.email].filter(Boolean).join(' / ')
}

function statusLabel(status: string): string {
  const key = `admin.dashboard.rateLimits.statuses.${status}`
  const translated = t(key)
  return translated === key ? status : translated
}

function statusClass(status: string): string {
  if (status === 'active') return 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300'
  if (status === 'error' || status === 'quota_exhausted' || status === 'expired') {
    return 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300'
  }
  return 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300'
}

function refreshErrorLabel(errorCode: string): string {
  const key = `admin.dashboard.rateLimits.refreshErrors.${errorCode}`
  const translated = t(key)
  return translated === key ? t('admin.dashboard.rateLimits.liveFailed') : translated
}

onMounted(() => void loadAccounts())
onBeforeUnmount(() => {
  accountController?.abort()
  keyController?.abort()
})
</script>

<style scoped>
.hidden-cols,
.data-cols {
  grid-template-columns: minmax(0, 1.25fr) 7rem minmax(10rem, 1fr) minmax(10rem, 1fr);
}

@media (max-width: 767px) {
  .data-cols {
    grid-template-columns: minmax(0, 1fr) auto;
  }

  .data-cols > :nth-child(3),
  .data-cols > :nth-child(4) {
    grid-column: 1 / -1;
  }
}
</style>
