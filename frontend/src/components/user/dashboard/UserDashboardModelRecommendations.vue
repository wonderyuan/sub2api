<template>
  <section class="card overflow-hidden" aria-labelledby="model-recommendations-title">
    <header class="flex min-h-16 items-center justify-between gap-4 border-b border-gray-100 px-4 py-3 dark:border-dark-700 sm:px-5">
      <div class="flex min-w-0 items-center gap-3">
        <div class="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-primary-100 dark:bg-primary-900/30">
          <Icon name="sparkles" size="sm" class="text-primary-600 dark:text-primary-400" />
        </div>
        <div class="min-w-0">
          <h2 id="model-recommendations-title" class="text-base font-semibold text-gray-900 dark:text-white">
            {{ t('dashboard.modelRecommendations.title') }}
          </h2>
          <p v-if="formattedUpdatedAt" class="truncate text-xs text-gray-500 dark:text-gray-400">
            {{ t('dashboard.modelRecommendations.updatedAt', { time: formattedUpdatedAt }) }}
          </p>
        </div>
      </div>
      <button
        type="button"
        class="flex h-8 w-8 shrink-0 items-center justify-center rounded-md text-gray-500 transition-colors hover:bg-gray-100 hover:text-primary-600 disabled:cursor-not-allowed disabled:opacity-50 dark:text-gray-400 dark:hover:bg-dark-700 dark:hover:text-primary-400"
        :aria-label="t('dashboard.modelRecommendations.refresh')"
        :title="t('dashboard.modelRecommendations.refresh')"
        :disabled="loading"
        data-model-recommendations-refresh
        @click="$emit('refresh')"
      >
        <Icon name="refresh" size="sm" :class="{ 'animate-spin': loading }" />
      </button>
    </header>

    <div v-if="loading && !hasData" class="flex items-center justify-center py-10">
      <LoadingSpinner size="md" />
    </div>
    <div v-else-if="!hasData" class="px-5 py-7 text-sm text-gray-500 dark:text-gray-400">
      {{ t('dashboard.modelRecommendations.unavailable') }}
    </div>
    <div v-else>
      <section v-if="stationGroups.length" class="border-b border-gray-100 px-4 py-4 dark:border-dark-700 sm:px-5">
        <h3 class="mb-3 text-sm font-semibold text-gray-900 dark:text-white">
          {{ t('dashboard.modelRecommendations.station') }}
        </h3>
        <div class="grid grid-cols-1 gap-x-6 gap-y-4 sm:grid-cols-2 xl:grid-cols-4">
          <article
            v-for="group in stationGroups"
            :key="group.key"
            class="min-w-0 border-b border-gray-100 pb-4 last:border-b-0 last:pb-0 xl:border-b-0 xl:pb-0"
          >
            <h4 class="mb-2 truncate text-xs font-semibold text-gray-700 dark:text-gray-200" :title="stationTitle(group)">
              {{ stationTitle(group) }}
            </h4>
            <div class="space-y-2">
              <div v-for="item in group.items" :key="stationItemKey(group.key, item)" class="min-w-0">
                <div class="flex items-center justify-between gap-2">
                  <span class="truncate text-sm font-medium text-gray-900 dark:text-white" :title="item.model">{{ item.model }}</span>
                  <span class="shrink-0 rounded bg-gray-100 px-1.5 py-0.5 font-mono text-[10px] font-medium text-gray-600 dark:bg-dark-700 dark:text-gray-300">
                    {{ effortLabel(item.effort) }}
                  </span>
                </div>
                <div class="mt-1 grid grid-cols-3 gap-1 text-[11px] text-gray-500 dark:text-gray-400">
                  <span><b class="font-medium text-gray-700 dark:text-gray-200">IQ</b> {{ formatIQ(item.iq) }}</span>
                  <span><b class="font-medium text-gray-700 dark:text-gray-200">$</b> {{ formatPrice(item.average_cost_usd) }}</span>
                  <span><b class="font-medium text-gray-700 dark:text-gray-200">{{ t('dashboard.modelRecommendations.time') }}</b> {{ formatDuration(item.average_duration_minutes) }}</span>
                </div>
              </div>
            </div>
          </article>
        </div>
      </section>

      <section v-if="intelligenceGroups.length" class="px-4 py-4 sm:px-5">
        <h3 class="mb-3 text-sm font-semibold text-gray-900 dark:text-white">
          {{ t('dashboard.modelRecommendations.intelligence') }}
        </h3>
        <div class="space-y-5">
          <section v-for="group in intelligenceGroups" :key="group.model" class="border-t border-gray-100 pt-3 first:border-t-0 first:pt-0 dark:border-dark-700">
            <h4 class="mb-2 font-mono text-sm font-semibold text-gray-800 dark:text-gray-100">{{ group.model }}</h4>
            <div class="grid grid-cols-1 gap-x-5 gap-y-2 md:grid-cols-2 xl:grid-cols-3">
              <article
                v-for="item in group.items"
                :key="intelligenceItemKey(item)"
                class="min-w-0 border-b border-gray-100 py-2 last:border-b-0 xl:border-b-0 dark:border-dark-700"
                :data-effort="item.effort"
              >
                <div class="flex items-center justify-between gap-2">
                  <div class="flex min-w-0 items-center gap-1.5">
                    <span class="rounded bg-primary-50 px-1.5 py-0.5 font-mono text-[10px] font-medium text-primary-700 dark:bg-primary-900/25 dark:text-primary-300">
                      {{ effortLabel(item.effort) }}
                    </span>
                    <span
                      v-if="isBest(group, item)"
                      class="flex h-5 w-5 shrink-0 items-center justify-center rounded text-amber-500 dark:text-amber-300"
                      :title="t('dashboard.modelRecommendations.best')"
                      :aria-label="t('dashboard.modelRecommendations.best')"
                      :data-best-combination="intelligenceItemKey(item)"
                    >
                      <Icon name="star" size="xs" :stroke-width="2" />
                    </span>
                  </div>
                  <span class="shrink-0 font-mono text-xs font-semibold text-gray-700 dark:text-gray-200">IQ {{ formatIQ(item.iq) }}</span>
                </div>
                <div class="mt-2 h-1.5 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700" role="progressbar" :aria-valuenow="scoreBarPercent(item.iq)" aria-valuemin="0" aria-valuemax="100">
                  <div class="h-full rounded-full bg-primary-500 transition-[width] duration-300 dark:bg-primary-400" :style="{ width: `${scoreBarPercent(item.iq)}%` }" />
                </div>
                <div class="mt-2 grid grid-cols-3 gap-2 text-[11px] text-gray-500 dark:text-gray-400">
                  <span class="truncate" :title="t('dashboard.modelRecommendations.samples')">{{ t('dashboard.modelRecommendations.samples') }} {{ item.samples }}</span>
                  <span class="truncate" :title="t('dashboard.modelRecommendations.price')">{{ t('dashboard.modelRecommendations.price') }} {{ formatPrice(item.average_cost_usd) }}</span>
                  <span class="truncate" :title="t('dashboard.modelRecommendations.time')">{{ t('dashboard.modelRecommendations.time') }} {{ formatDuration(item.average_duration_minutes) }}</span>
                </div>
              </article>
            </div>
          </section>
        </div>
      </section>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import Icon from '@/components/icons/Icon.vue'
import type {
  CodexRadarDashboardRecommendations,
  CodexRadarIntelligenceMetric,
  CodexRadarStationRecommendation,
  CodexRadarStationRecommendationSet
} from '@/api/usage'
import { formatDateTimeToMinute } from '@/utils/format'

interface IntelligenceGroup {
  model: string
  items: CodexRadarIntelligenceMetric[]
  bestKey: string | null
}

const props = withDefaults(defineProps<{
  data: CodexRadarDashboardRecommendations | null
  loading?: boolean
}>(), {
  loading: false
})

defineEmits<{
  refresh: []
}>()

const { t } = useI18n()

const effortOrder: Record<string, number> = {
  ultra: 0,
  max: 1,
  xhigh: 2,
  high: 3,
  medium: 4,
  low: 5
}

const modelOrder: Record<string, number> = {
  'gpt-5.6-sol': 0,
  'gpt-5.6-terra': 1,
  'gpt-5.6-luna': 2,
  'gpt-5.5': 3,
  'deepseek-v4-flash': 4
}

const stationCategoryKeys: Record<string, string> = {
  daily_development: 'dailyDevelopment',
  hard_problems: 'hardProblems',
  background_automation: 'backgroundAutomation',
  lobster_tasks: 'lobsterTasks'
}

const stationGroups = computed(() => props.data?.station_recommendations ?? [])

const intelligenceGroups = computed<IntelligenceGroup[]>(() => {
  const groups = new Map<string, CodexRadarIntelligenceMetric[]>()
  for (const metric of props.data?.intelligence_recommendations ?? []) {
    const model = metric.model.trim()
    if (!model) continue
    const items = groups.get(model) ?? []
    items.push(metric)
    groups.set(model, items)
  }

  return [...groups.entries()]
    .map(([model, items]) => {
      const sortedItems = [...items].sort((left, right) => {
        const effortDelta = effortRank(left.effort) - effortRank(right.effort)
        return effortDelta || left.effort.localeCompare(right.effort)
      })
      return { model, items: sortedItems, bestKey: bestCombinationKey(sortedItems) }
    })
    .sort((left, right) => {
      const leftOrder = modelOrder[left.model] ?? Number.MAX_SAFE_INTEGER
      const rightOrder = modelOrder[right.model] ?? Number.MAX_SAFE_INTEGER
      return leftOrder - rightOrder || left.model.localeCompare(right.model)
    })
})

const hasData = computed(() => stationGroups.value.length > 0 || intelligenceGroups.value.length > 0)
const formattedUpdatedAt = computed(() => formatDateTimeToMinute(props.data?.source_updated_at ?? null))

function effortRank(effort: string): number {
  return effortOrder[effort.trim().toLowerCase()] ?? Number.MAX_SAFE_INTEGER
}

function effortLabel(effort: string): string {
  const normalized = effort.trim().toLowerCase()
  if (Object.prototype.hasOwnProperty.call(effortOrder, normalized)) {
    return String(t(`dashboard.modelRecommendations.efforts.${normalized}`))
  }
  return effort || '-'
}

function stationTitle(group: CodexRadarStationRecommendationSet): string {
  const key = stationCategoryKeys[group.key]
  return key ? String(t(`dashboard.modelRecommendations.stationCategories.${key}`)) : group.title || group.key
}

function stationItemKey(groupKey: string, item: CodexRadarStationRecommendation): string {
  return `${groupKey}|${item.model}|${item.effort}`
}

function intelligenceItemKey(item: CodexRadarIntelligenceMetric): string {
  return `${item.model}|${item.effort}`
}

function scoreBarPercent(iq: number | null | undefined): number {
  if (typeof iq !== 'number' || !Number.isFinite(iq)) return 0
  return Math.max(0, Math.min(100, iq / 150 * 100))
}

function formatIQ(value: number | null | undefined): string {
  return typeof value === 'number' && Number.isFinite(value) ? value.toFixed(1) : '-'
}

function formatPrice(value: number | null | undefined): string {
  if (typeof value !== 'number' || !Number.isFinite(value)) return '-'
  return value < 0.01 ? value.toFixed(4) : value.toFixed(2)
}

function formatDuration(value: number | null | undefined): string {
  if (typeof value !== 'number' || !Number.isFinite(value)) return '-'
  const formatted = value < 10 ? value.toFixed(1) : String(Math.round(value))
  return `${formatted}${t('dashboard.modelRecommendations.minutes')}`
}

function bestCombinationKey(items: CodexRadarIntelligenceMetric[]): string | null {
  if (items.length === 0) return null

  const intelligenceValues = items.map((item) => item.iq)
  const priceValues = items.map((item) => item.average_cost_usd)
  const durationValues = items.map((item) => item.average_duration_minutes)
  let bestItem = items[0]
  let bestScore = Number.NEGATIVE_INFINITY

  for (const item of items) {
    const score =
      normalizedScore(item.iq, intelligenceValues) * 0.5 +
      normalizedScore(item.average_cost_usd, priceValues, true) * 0.3 +
      normalizedScore(item.average_duration_minutes, durationValues, true) * 0.2
    if (score > bestScore) {
      bestScore = score
      bestItem = item
    }
  }

  return intelligenceItemKey(bestItem)
}

function normalizedScore(value: number | null, values: Array<number | null>, inverse = false): number {
  if (typeof value !== 'number' || !Number.isFinite(value)) return 0
  const validValues = values.filter((candidate): candidate is number => typeof candidate === 'number' && Number.isFinite(candidate))
  if (validValues.length === 0) return 0

  const min = Math.min(...validValues)
  const max = Math.max(...validValues)
  if (min === max) return 1

  const score = (value - min) / (max - min)
  return inverse ? 1 - score : score
}

function isBest(group: IntelligenceGroup, item: CodexRadarIntelligenceMetric): boolean {
  return group.bestKey === intelligenceItemKey(item)
}
</script>
