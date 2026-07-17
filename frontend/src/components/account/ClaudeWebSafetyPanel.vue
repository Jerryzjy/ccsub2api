<template>
  <section class="space-y-4 rounded-lg border border-gray-200 p-4 dark:border-dark-600" data-testid="claude-web-safety-panel">
    <div class="flex flex-wrap items-end gap-3">
      <div class="min-w-48 flex-1">
        <label class="input-label">{{ t('admin.accounts.quotaControl.claudeTier.label') }}</label>
        <select :value="modelValue.tier" class="input" data-testid="claude-web-tier" @change="setTier">
          <option value="">{{ t('admin.accounts.claudeWeb.autoDetectTier') }}</option>
          <option value="Pro">Pro</option>
          <option value="Max_5x">Max 5x</option>
          <option value="Max_20x">Max 20x</option>
        </select>
      </div>
      <button type="button" class="btn btn-secondary" data-testid="claude-web-apply-preset" @click="applyPreset">
        {{ t('admin.accounts.claudeWeb.applySafetyPreset') }}
      </button>
    </div>
    <p class="input-hint">{{ t('admin.accounts.claudeWeb.tierPresetHint') }}</p>

    <div class="grid gap-4 md:grid-cols-2">
      <LimitField
        :enabled="modelValue.windowCostEnabled"
        :label="t('admin.accounts.quotaControl.windowCost.label')"
        data-testid="claude-web-window-cost"
        @update:enabled="patch('windowCostEnabled', $event)"
      >
        <NumberInput :model-value="modelValue.windowCostLimit" prefix="$" @update:model-value="patch('windowCostLimit', $event)" />
        <NumberInput :model-value="modelValue.windowCostStickyReserve" prefix="$+" @update:model-value="patch('windowCostStickyReserve', $event)" />
      </LimitField>

      <LimitField
        :enabled="modelValue.sessionLimitEnabled"
        :label="t('admin.accounts.quotaControl.sessionLimit.label')"
        data-testid="claude-web-session-limit"
        @update:enabled="patch('sessionLimitEnabled', $event)"
      >
        <NumberInput :model-value="modelValue.maxSessions" :placeholder="t('admin.accounts.quotaControl.sessionLimit.maxSessions')" @update:model-value="patch('maxSessions', $event)" />
        <NumberInput :model-value="modelValue.sessionIdleTimeoutMinutes" :placeholder="t('admin.accounts.quotaControl.sessionLimit.idleTimeout')" @update:model-value="patch('sessionIdleTimeoutMinutes', $event)" />
      </LimitField>

      <LimitField
        :enabled="modelValue.rpmLimitEnabled"
        :label="t('admin.accounts.quotaControl.rpmLimit.label')"
        data-testid="claude-web-rpm-limit"
        @update:enabled="patch('rpmLimitEnabled', $event)"
      >
        <NumberInput :model-value="modelValue.baseRPM" placeholder="RPM" @update:model-value="patch('baseRPM', $event)" />
        <StrategySelect :model-value="modelValue.rpmStrategy" @update:model-value="patch('rpmStrategy', $event)" />
      </LimitField>

      <LimitField
        :enabled="modelValue.tpmLimitEnabled"
        :label="t('admin.accounts.quotaControl.tpmLimit.label')"
        data-testid="claude-web-tpm-limit"
        @update:enabled="patch('tpmLimitEnabled', $event)"
      >
        <NumberInput :model-value="modelValue.baseTPM" placeholder="TPM" @update:model-value="patch('baseTPM', $event)" />
        <StrategySelect :model-value="modelValue.tpmStrategy" @update:model-value="patch('tpmStrategy', $event)" />
      </LimitField>
    </div>

    <div class="rounded-lg bg-gray-50 p-3 dark:bg-dark-700">
      <label class="input-label">{{ t('admin.accounts.claudeWeb.localQuotaLabel') }}</label>
      <div class="grid gap-3 sm:grid-cols-3">
        <NumberInput :model-value="modelValue.quotaLimit" :placeholder="t('admin.accounts.claudeWeb.totalQuota')" @update:model-value="patch('quotaLimit', $event)" />
        <NumberInput :model-value="modelValue.quotaDailyLimit" :placeholder="t('admin.accounts.claudeWeb.dailyQuota')" @update:model-value="patch('quotaDailyLimit', $event)" />
        <NumberInput :model-value="modelValue.quotaWeeklyLimit" :placeholder="t('admin.accounts.claudeWeb.weeklyQuota')" @update:model-value="patch('quotaWeeklyLimit', $event)" />
      </div>
      <p class="input-hint mt-2">{{ t('admin.accounts.claudeWeb.localQuotaHint') }}</p>
    </div>
  </section>
</template>

<script setup lang="ts">
import { defineComponent, h, type PropType } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  applyClaudeWebPreset,
  type ClaudeLimitStrategy,
  type ClaudeTier,
  type ClaudeWebSafetyState
} from './claudeWebSafety'

const props = defineProps<{ modelValue: ClaudeWebSafetyState }>()
const emit = defineEmits<{ 'update:modelValue': [value: ClaudeWebSafetyState] }>()
const { t } = useI18n()

function patch<K extends keyof ClaudeWebSafetyState>(key: K, value: ClaudeWebSafetyState[K]) {
  emit('update:modelValue', { ...props.modelValue, [key]: value })
}

function setTier(event: Event) {
  emit('update:modelValue', {
    ...props.modelValue,
    tier: (event.target as HTMLSelectElement).value as ClaudeTier,
    tierSource: 'manual'
  })
}

function applyPreset() {
  const next = { ...props.modelValue }
  applyClaudeWebPreset(next, (next.tier || 'Pro') as Exclude<ClaudeTier, ''>, { overwrite: false })
  emit('update:modelValue', next)
}

const LimitField = defineComponent({
  props: { enabled: Boolean, label: { type: String, required: true } },
  emits: ['update:enabled'],
  setup(fieldProps, { emit: fieldEmit, slots, attrs }) {
    return () => h('div', { ...attrs, class: 'rounded-lg border border-gray-200 p-3 dark:border-dark-600' }, [
      h('label', { class: 'mb-3 flex items-center gap-2 text-sm font-medium text-gray-700 dark:text-gray-200' }, [
        h('input', { type: 'checkbox', checked: fieldProps.enabled, onChange: (event: Event) => fieldEmit('update:enabled', (event.target as HTMLInputElement).checked) }),
        fieldProps.label
      ]),
      fieldProps.enabled ? h('div', { class: 'grid gap-2 sm:grid-cols-2' }, slots.default?.()) : null
    ])
  }
})

const NumberInput = defineComponent({
  props: {
    modelValue: { type: Number as PropType<number | null>, default: null },
    placeholder: String,
    prefix: String
  },
  emits: ['update:modelValue'],
  setup(inputProps, { emit: inputEmit }) {
    return () => h('label', { class: 'flex items-center gap-2' }, [
      inputProps.prefix ? h('span', { class: 'text-xs text-gray-500' }, inputProps.prefix) : null,
      h('input', {
        type: 'number', min: '0', step: '1', class: 'input', value: inputProps.modelValue ?? '', placeholder: inputProps.placeholder,
        onInput: (event: Event) => {
          const raw = (event.target as HTMLInputElement).value
          inputEmit('update:modelValue', raw === '' ? null : Number(raw))
        }
      })
    ])
  }
})

const StrategySelect = defineComponent({
  props: { modelValue: { type: String as PropType<ClaudeLimitStrategy>, required: true } },
  emits: ['update:modelValue'],
  setup(selectProps, { emit: selectEmit }) {
    return () => h('select', {
      class: 'input', value: selectProps.modelValue,
      onChange: (event: Event) => selectEmit('update:modelValue', (event.target as HTMLSelectElement).value)
    }, [h('option', { value: 'tiered' }, 'Tiered'), h('option', { value: 'sticky_exempt' }, 'Sticky exempt')])
  }
})
</script>
