<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { opsAPI, type OpsAccountQuotaReport, type OpsAccountPoolHealth } from '@/api/admin/ops'

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

type PlatformTab = 'claude' | 'openai'
const activeTab = ref<PlatformTab>('claude')

const loading = ref(false)
const errorMessage = ref('')
const report = ref<OpsAccountQuotaReport | null>(null)
const lastUpdated = ref<Date | null>(null)

const healthMeta: Record<OpsAccountPoolHealth, { label: string; class: string }> = {
  sustainable: { label: '可持续', class: 'text-green-600 dark:text-green-400' },
  tight: { label: '紧张', class: 'text-yellow-600 dark:text-yellow-400' },
  depleted: { label: '枯竭', class: 'text-red-600 dark:text-red-400' }
}

function pct(ratio: number): string {
  return `${(ratio * 100).toFixed(1)}%`
}

function ratioBarClass(ratio: number): string {
  if (ratio >= 0.6) return 'bg-green-500 dark:bg-green-600'
  if (ratio >= 0.3) return 'bg-yellow-500 dark:bg-yellow-600'
  return 'bg-red-500 dark:bg-red-600'
}

function loadBarClass(ratio: number): string {
  if (ratio >= 0.8) return 'bg-red-500 dark:bg-red-600'
  if (ratio >= 0.5) return 'bg-yellow-500 dark:bg-yellow-600'
  return 'bg-green-500 dark:bg-green-600'
}

function barStyle(ratio: number): string {
  return `width: ${Math.min(100, Math.max(0, ratio * 100))}%`
}

const healthLabel = computed(() => (report.value ? healthMeta[report.value.health].label : '—'))
const healthClass = computed(() => (report.value ? healthMeta[report.value.health].class : ''))

const saturationLabel = computed(() => {
  const m = report.value?.saturation_multiple
  if (m == null || !Number.isFinite(m)) return ''
  return `当前负载约 ${m.toFixed(1)} 倍才会跑满`
})

const updatedLabel = computed(() => {
  if (!lastUpdated.value) return '—'
  const diffSec = Math.floor((Date.now() - lastUpdated.value.getTime()) / 1000)
  if (diffSec < 5) return '刚刚'
  if (diffSec < 60) return `${diffSec} 秒前`
  const min = Math.floor(diffSec / 60)
  return `${min} 分钟前`
})

// Recovery list: humanize "after_seconds" into "Xm" / "Xh Ym后".
function recoverLabel(afterSeconds: number): string {
  const totalMin = Math.round(afterSeconds / 60)
  if (totalMin < 60) return `${totalMin}m 后`
  const h = Math.floor(totalMin / 60)
  const m = totalMin % 60
  return m > 0 ? `${h}h${m}m 后` : `${h}h 后`
}

async function loadData() {
  if (activeTab.value !== 'claude') return
  loading.value = true
  errorMessage.value = ''
  try {
    report.value = await opsAPI.getAccountQuotaMonitor({
      platform: props.platformFilter || undefined,
      group_id: props.groupIdFilter ?? undefined
    })
    lastUpdated.value = new Date()
  } catch (err: any) {
    console.error('[OpsAccountQuotaCard] Failed to load data', err)
    errorMessage.value = err?.response?.data?.detail || t('admin.ops.concurrency.loadFailed')
    report.value = null
  } finally {
    loading.value = false
  }
}

watch(
  () => [props.refreshToken, props.platformFilter, props.groupIdFilter],
  () => {
    loadData()
  },
  { immediate: true }
)
</script>

<template>
  <div class="flex h-full flex-col rounded-3xl bg-white p-6 shadow-sm ring-1 ring-gray-900/5 dark:bg-dark-800 dark:ring-dark-700">
    <!-- Header -->
    <div class="mb-4 flex shrink-0 flex-wrap items-center justify-between gap-3">
      <div class="flex items-center gap-3">
        <h3 class="flex items-center gap-2 text-sm font-bold text-gray-900 dark:text-white">
          <svg class="h-4 w-4 text-emerald-500" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 13.125C3 12.504 3.504 12 4.125 12h2.25c.621 0 1.125.504 1.125 1.125v6.75C7.5 20.496 6.996 21 6.375 21h-2.25A1.125 1.125 0 013 19.875v-6.75zM9.75 8.625c0-.621.504-1.125 1.125-1.125h2.25c.621 0 1.125.504 1.125 1.125v11.25c0 .621-.504 1.125-1.125 1.125h-2.25a1.125 1.125 0 01-1.125-1.125V8.625zM16.5 4.125c0-.621.504-1.125 1.125-1.125h2.25C20.496 3 21 3.504 21 4.125v15.75c0 .621-.504 1.125-1.125 1.125h-2.25a1.125 1.125 0 01-1.125-1.125V4.125z" />
          </svg>
          可用额度监控
        </h3>
        <!-- Platform tabs -->
        <div class="flex items-center gap-1 rounded-lg bg-gray-100 p-0.5 dark:bg-dark-700">
          <button
            class="rounded-md px-2 py-0.5 text-[11px] font-semibold transition-colors"
            :class="activeTab === 'claude' ? 'bg-white text-gray-900 shadow-sm dark:bg-dark-800 dark:text-white' : 'text-gray-500 dark:text-gray-400'"
            @click="activeTab = 'claude'"
          >
            Claude
          </button>
          <button
            class="cursor-not-allowed rounded-md px-2 py-0.5 text-[11px] font-semibold text-gray-300 dark:text-gray-600"
            disabled
            title="敬请期待"
          >
            OpenAI
          </button>
        </div>
      </div>
      <div class="flex items-center gap-2">
        <span class="text-[10px] text-gray-400 dark:text-gray-500">更新于 {{ updatedLabel }}</span>
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

    <!-- OpenAI placeholder -->
    <div
      v-if="activeTab === 'openai'"
      class="flex min-h-0 flex-1 items-center justify-center rounded-xl border border-dashed border-gray-200 p-4 text-center text-sm text-gray-400 dark:border-dark-700 dark:text-gray-500"
    >
      OpenAI 号池监控敬请期待
    </div>

    <template v-else-if="report">
      <!-- Headline metrics -->
      <div class="mb-4 grid shrink-0 grid-cols-2 gap-3 lg:grid-cols-4">
        <!-- Account count -->
        <div class="rounded-2xl bg-gray-50 p-3 dark:bg-dark-900">
          <div class="mb-1 text-[11px] font-semibold text-gray-500 dark:text-gray-400">账号数（可用/总）</div>
          <div class="text-xl font-bold text-gray-900 dark:text-white">
            {{ report.available_accounts }} <span class="text-sm font-medium text-gray-400">/ {{ report.total_accounts }}</span>
          </div>
          <div class="mt-1.5 text-[10px] text-gray-400 dark:text-gray-500">
            使用中 {{ report.in_use_accounts }} · 闲置 {{ report.idle_accounts }} · 打满 {{ report.exhausted_accounts }}
          </div>
        </div>

        <!-- Remaining capacity -->
        <div class="rounded-2xl bg-gray-50 p-3 dark:bg-dark-900">
          <div class="mb-1 text-[11px] font-semibold text-gray-500 dark:text-gray-400">剩余容量</div>
          <div class="mb-1.5 text-xl font-bold text-gray-900 dark:text-white">{{ pct(report.remaining_capacity_ratio) }}</div>
          <div class="h-1.5 w-full overflow-hidden rounded-full bg-gray-200 dark:bg-dark-700">
            <div class="h-full rounded-full transition-all duration-300" :class="ratioBarClass(report.remaining_capacity_ratio)" :style="barStyle(report.remaining_capacity_ratio)"></div>
          </div>
        </div>

        <!-- Pool load -->
        <div class="rounded-2xl bg-gray-50 p-3 dark:bg-dark-900">
          <div class="mb-1 text-[11px] font-semibold text-gray-500 dark:text-gray-400">池子负载</div>
          <div class="mb-1.5 text-xl font-bold text-gray-900 dark:text-white">{{ pct(report.pool_load_ratio) }}</div>
          <div class="h-1.5 w-full overflow-hidden rounded-full bg-gray-200 dark:bg-dark-700">
            <div class="h-full rounded-full transition-all duration-300" :class="loadBarClass(report.pool_load_ratio)" :style="barStyle(report.pool_load_ratio)"></div>
          </div>
          <div v-if="saturationLabel" class="mt-1.5 text-[10px] text-gray-400 dark:text-gray-500">{{ saturationLabel }}</div>
        </div>

        <!-- Pool health -->
        <div class="rounded-2xl bg-gray-50 p-3 dark:bg-dark-900">
          <div class="mb-1 text-[11px] font-semibold text-gray-500 dark:text-gray-400">池子健康度</div>
          <div class="mb-1.5 text-xl font-bold" :class="healthClass">{{ healthLabel }}</div>
          <div class="text-[10px] text-gray-400 dark:text-gray-500">
            满血 {{ report.full_idle_accounts }} · 重新授权 {{ report.reauth_accounts }} · 异常 {{ report.error_accounts }}
          </div>
        </div>
      </div>

      <!-- Recovery forecast -->
      <div class="flex min-h-0 flex-1 flex-col overflow-hidden rounded-xl border border-gray-200 dark:border-dark-700">
        <div class="flex shrink-0 items-center justify-between border-b border-gray-200 bg-gray-50 px-3 py-2 dark:border-dark-700 dark:bg-dark-900">
          <span class="text-[10px] font-bold uppercase tracking-wider text-gray-500 dark:text-gray-400">4h 内恢复</span>
          <span class="text-[10px] text-gray-400 dark:text-gray-500">{{ report.recovery_buckets.length }} 个时段</span>
        </div>
        <div class="custom-scrollbar max-h-[260px] flex-1 space-y-1.5 overflow-y-auto p-3">
          <div
            v-for="bucket in report.recovery_buckets"
            :key="bucket.after_seconds"
            class="flex items-center justify-between rounded-lg bg-gray-50 px-3 py-2 text-[11px] dark:bg-dark-900"
          >
            <span class="font-semibold text-gray-700 dark:text-gray-300">{{ recoverLabel(bucket.after_seconds) }}</span>
            <span class="text-gray-500 dark:text-gray-400">
              +{{ bucket.account_count }} 账号 · 恢复并发 {{ bucket.restored_concurrency }}
            </span>
          </div>
          <div v-if="report.recovery_buckets.length === 0" class="py-6 text-center text-xs text-gray-400 dark:text-gray-500">
            4 小时内无账号恢复 🎉
          </div>
        </div>
      </div>
    </template>

    <!-- Whole-card empty state -->
    <div
      v-else
      class="flex min-h-0 flex-1 items-center justify-center rounded-xl border border-dashed border-gray-200 p-4 text-center text-sm text-gray-400 dark:border-dark-700 dark:text-gray-500"
    >
      {{ loading ? t('common.loading') : t('admin.ops.concurrency.empty') }}
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
