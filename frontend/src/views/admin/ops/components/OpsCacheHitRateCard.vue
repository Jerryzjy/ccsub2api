<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { opsAPI, type OpsCacheClientType, type OpsCacheHitRateReport } from '@/api/admin/ops'

interface Props {
  platformFilter?: string
  groupIdFilter?: number | null
  refreshToken: number
}

const props = withDefaults(defineProps<Props>(), {
  platformFilter: '',
  groupIdFilter: null
})

const { t } = useI18n()

type CacheTimeRange = '1h' | '6h' | '24h'

const loading = ref(false)
const errorMessage = ref('')
const report = ref<OpsCacheHitRateReport | null>(null)
const timeRange = ref<CacheTimeRange>('24h')

const timeRangeOptions: { value: CacheTimeRange; label: string }[] = [
  { value: '1h', label: t('admin.ops.timeRange.1h') },
  { value: '6h', label: t('admin.ops.timeRange.6h') },
  { value: '24h', label: t('admin.ops.timeRange.24h') }
]

// Stable display order + human labels for the three client buckets. Labels are
// inline (not i18n keys) so a missing translation can't render as a raw key on
// what is an internal ops-only card.
const clientTypeMeta: Record<OpsCacheClientType, { label: string; order: number }> = {
  claude_code: { label: 'Claude Code', order: 0 },
  third_party: { label: '第三方客户端', order: 1 },
  unknown: { label: '未识别', order: 2 }
}

function pct(rate: number): string {
  return `${(rate * 100).toFixed(1)}%`
}

function hitRateClass(rate: number): string {
  if (rate >= 0.9) return 'text-green-600 dark:text-green-400'
  if (rate >= 0.6) return 'text-yellow-600 dark:text-yellow-400'
  return 'text-red-600 dark:text-red-400'
}

function barClass(rate: number): string {
  if (rate >= 0.9) return 'bg-green-500 dark:bg-green-600'
  if (rate >= 0.6) return 'bg-yellow-500 dark:bg-yellow-600'
  return 'bg-red-500 dark:bg-red-600'
}

function barStyle(rate: number): string {
  return `width: ${Math.min(100, Math.max(0, rate * 100))}%`
}

// Headline rows: one per client type, summed across accounts.
const summaryRows = computed(() => {
  const rows = report.value?.by_client_type ?? []
  return [...rows].sort(
    (a, b) => (clientTypeMeta[a.client_type]?.order ?? 99) - (clientTypeMeta[b.client_type]?.order ?? 99)
  )
})

// Per-account detail, worst hit rate first so low-cache accounts surface.
const accountRows = computed(() => {
  const rows = report.value?.rows ?? []
  return [...rows].sort((a, b) => a.hit_rate - b.hit_rate)
})

function clientLabel(ct: OpsCacheClientType): string {
  return clientTypeMeta[ct]?.label ?? ct
}

async function loadData() {
  loading.value = true
  errorMessage.value = ''
  try {
    report.value = await opsAPI.getCacheHitRate({
      time_range: timeRange.value,
      platform: props.platformFilter || undefined,
      group_id: props.groupIdFilter ?? undefined
    })
  } catch (err: any) {
    console.error('[OpsCacheHitRateCard] Failed to load data', err)
    errorMessage.value = err?.response?.data?.detail || t('admin.ops.concurrency.loadFailed')
    report.value = null
  } finally {
    loading.value = false
  }
}

watch(
  () => [props.refreshToken, props.platformFilter, props.groupIdFilter, timeRange.value],
  () => {
    loadData()
  },
  { immediate: true }
)
</script>

<template>
  <div class="flex h-full flex-col rounded-3xl bg-white p-6 shadow-sm ring-1 ring-gray-900/5 dark:bg-dark-800 dark:ring-dark-700">
    <!-- Header -->
    <div class="mb-4 flex shrink-0 items-center justify-between gap-3">
      <h3 class="flex items-center gap-2 text-sm font-bold text-gray-900 dark:text-white">
        <svg class="h-4 w-4 text-blue-500" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 8h14M5 8a2 2 0 110-4h14a2 2 0 110 4M5 8v10a2 2 0 002 2h10a2 2 0 002-2V8m-9 4h4" />
        </svg>
        缓存命中率（按客户端）
      </h3>
      <div class="flex items-center gap-2">
        <select
          v-model="timeRange"
          class="rounded-lg border border-gray-200 bg-white px-2 py-1 text-[11px] font-semibold text-gray-700 dark:border-dark-600 dark:bg-dark-700 dark:text-gray-300"
        >
          <option v-for="opt in timeRangeOptions" :key="opt.value" :value="opt.value">{{ opt.label }}</option>
        </select>
        <button
          class="flex items-center gap-1 rounded-lg bg-gray-100 px-2 py-1 text-[11px] font-semibold text-gray-700 transition-colors hover:bg-gray-200 disabled:cursor-not-allowed disabled:opacity-50 dark:bg-dark-700 dark:text-gray-300 dark:hover:bg-dark-600"
          :disabled="loading"
          :title="t('common.refresh')"
          @click="loadData"
        >
          <svg class="h-3 w-3" :class="{ 'animate-spin': loading }" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
          </svg>
        </button>
      </div>
    </div>

    <!-- Error -->
    <div v-if="errorMessage" class="mb-3 shrink-0 rounded-xl bg-red-50 p-2.5 text-xs text-red-600 dark:bg-red-900/20 dark:text-red-400">
      {{ errorMessage }}
    </div>

    <!-- Headline: hit rate per client type -->
    <div class="mb-4 grid shrink-0 grid-cols-1 gap-3 sm:grid-cols-3">
      <div
        v-for="row in summaryRows"
        :key="row.client_type"
        class="rounded-2xl bg-gray-50 p-3 dark:bg-dark-900"
      >
        <div class="mb-1 text-[11px] font-semibold text-gray-500 dark:text-gray-400">{{ clientLabel(row.client_type) }}</div>
        <div class="mb-1.5 text-xl font-bold" :class="hitRateClass(row.hit_rate)">{{ pct(row.hit_rate) }}</div>
        <div class="h-1.5 w-full overflow-hidden rounded-full bg-gray-200 dark:bg-dark-700">
          <div class="h-full rounded-full transition-all duration-300" :class="barClass(row.hit_rate)" :style="barStyle(row.hit_rate)"></div>
        </div>
        <div class="mt-1.5 text-[10px] text-gray-400 dark:text-gray-500">
          {{ row.request_count }} 次请求 · 读 {{ row.cache_read_tokens.toLocaleString() }}
        </div>
      </div>
      <div v-if="summaryRows.length === 0" class="col-span-full rounded-2xl border border-dashed border-gray-200 p-4 text-center text-sm text-gray-400 dark:border-dark-700 dark:text-gray-500">
        {{ loading ? t('common.loading') : t('admin.ops.concurrency.empty') }}
      </div>
    </div>

    <!-- Per-account detail (worst hit rate first) -->
    <div v-if="accountRows.length > 0" class="flex min-h-0 flex-1 flex-col overflow-hidden rounded-xl border border-gray-200 dark:border-dark-700">
      <div class="flex shrink-0 items-center justify-between border-b border-gray-200 bg-gray-50 px-3 py-2 dark:border-dark-700 dark:bg-dark-900">
        <span class="text-[10px] font-bold uppercase tracking-wider text-gray-500 dark:text-gray-400">按账号</span>
        <span class="text-[10px] text-gray-500 dark:text-gray-400">{{ accountRows.length }}</span>
      </div>
      <div class="custom-scrollbar max-h-[300px] flex-1 space-y-2 overflow-y-auto p-3">
        <div v-for="row in accountRows" :key="`${row.account_id}-${row.client_type}`" class="rounded-lg bg-gray-50 p-2.5 dark:bg-dark-900">
          <div class="mb-1.5 flex items-center justify-between gap-2">
            <div class="flex min-w-0 items-center gap-2">
              <span class="truncate text-[11px] font-bold text-gray-900 dark:text-white">#{{ row.account_id }}</span>
              <span class="shrink-0 rounded-full bg-gray-200 px-1.5 py-0.5 text-[9px] font-semibold text-gray-600 dark:bg-dark-700 dark:text-gray-300">
                {{ clientLabel(row.client_type) }}
              </span>
            </div>
            <span class="shrink-0 text-[11px] font-bold" :class="hitRateClass(row.hit_rate)">{{ pct(row.hit_rate) }}</span>
          </div>
          <div class="h-1.5 w-full overflow-hidden rounded-full bg-gray-200 dark:bg-dark-700">
            <div class="h-full rounded-full transition-all duration-300" :class="barClass(row.hit_rate)" :style="barStyle(row.hit_rate)"></div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.custom-scrollbar {
  scrollbar-width: thin;
  scrollbar-color: rgba(156, 163, 175, 0.3) transparent;
}

.custom-scrollbar::-webkit-scrollbar {
  width: 6px;
}

.custom-scrollbar::-webkit-scrollbar-track {
  background: transparent;
}

.custom-scrollbar::-webkit-scrollbar-thumb {
  background-color: rgba(156, 163, 175, 0.3);
  border-radius: 3px;
}

.custom-scrollbar::-webkit-scrollbar-thumb:hover {
  background-color: rgba(156, 163, 175, 0.5);
}
</style>
