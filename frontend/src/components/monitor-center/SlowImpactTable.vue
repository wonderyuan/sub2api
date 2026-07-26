<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import type { OpsPerformanceImpact } from '@/api/admin/ops'
import { formatMs, formatPercent } from './monitorCenterUtils'

type Dimension = 'user' | 'account' | 'model'
const props = defineProps<{ impacts: OpsPerformanceImpact[] }>()
const { t } = useI18n()
const dimension = ref<Dimension>('user')
const rows = computed(() => props.impacts.filter((item) => item.dimension === dimension.value))
function causeLabel(cause: string): string {
  const key = `admin.ops.performance.causes.${cause}`
  const translated = t(key)
  return translated === key ? cause : translated
}
</script>

<template>
  <div class="mc-impact">
    <div class="mc-impact-head">
      <div><h4>{{ t('admin.monitorCenter.slow.impact') }}</h4><p>{{ t('admin.monitorCenter.slow.impactHint') }}</p></div>
      <div class="mc-segmented" role="tablist" :aria-label="t('admin.monitorCenter.slow.impact')">
        <button
          v-for="item in (['user', 'account', 'model'] as Dimension[])"
          :key="item"
          type="button"
          role="tab"
          :aria-selected="dimension === item"
          :tabindex="dimension === item ? 0 : -1"
          :class="{ active: dimension === item }"
          @click="dimension = item"
        >
          {{ t(`admin.monitorCenter.slow.dimensions.${item}`) }}
        </button>
      </div>
    </div>
    <div class="mc-table-wrap">
      <table class="mc-table">
        <thead><tr>
          <th>{{ t(`admin.monitorCenter.slow.dimensions.${dimension}`) }}</th>
          <th class="numeric">{{ t('admin.monitorCenter.slow.requests') }}</th>
          <th class="numeric">{{ t('admin.monitorCenter.slow.slowRate') }}</th>
          <th class="numeric">E2E P95</th>
          <th class="numeric">TTFT P95</th>
          <th class="numeric">{{ t('admin.monitorCenter.slow.queueP95') }}</th>
          <th>{{ t('admin.monitorCenter.slow.mainCause') }}</th>
        </tr></thead>
        <tbody>
          <tr v-for="item in rows" :key="`${item.dimension}:${item.id}`">
            <td><strong :title="item.name || item.id">{{ item.name || `#${item.id}` }}</strong></td>
            <td class="numeric">{{ item.request_count }}</td>
            <td class="numeric" :class="item.slow_rate >= 30 ? 'mc-bad' : item.slow_rate >= 10 ? 'mc-warn' : ''">{{ formatPercent(item.slow_rate, 1) }}</td>
            <td class="numeric">{{ formatMs(item.e2e_p95_ms) }}</td>
            <td class="numeric">{{ formatMs(item.ttft_p95_ms) }}</td>
            <td class="numeric">{{ formatMs(item.queue_p95_ms) }}</td>
            <td>{{ causeLabel(item.main_cause) }}</td>
          </tr>
          <tr v-if="!rows.length"><td colspan="7"><div class="mc-empty">{{ t('common.noData') }}</div></td></tr>
        </tbody>
      </table>
    </div>
  </div>
</template>

<style scoped>
.mc-impact { min-width: 0; border-top: 1px solid var(--mc-line); padding-top: 12px; }
.mc-impact-head { display: flex; align-items: flex-start; justify-content: space-between; gap: 10px; margin-bottom: 9px; }
.mc-impact h4 { margin: 0; font-size: 12px; font-weight: 700; }
.mc-impact p { margin: 3px 0 0; color: var(--mc-subtle); font-size: 9px; }
.mc-table td:first-child { max-width: 190px; overflow: hidden; color: var(--mc-text); text-overflow: ellipsis; white-space: nowrap; }
.mc-table td:last-child { color: var(--mc-text); }
.mc-table .mc-empty { min-height: 98px; }
@media (max-width: 760px) {
  .mc-impact-head { flex-direction: column; }
}
</style>
