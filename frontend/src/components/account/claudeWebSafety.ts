export type ClaudeTier = '' | 'Pro' | 'Max_5x' | 'Max_20x'
export type ClaudeTierSource = '' | 'manual' | 'profile' | 'inferred' | 'profile_default'
export type ClaudeLimitStrategy = 'tiered' | 'sticky_exempt'

export interface ClaudeWebSafetyState {
  tier: ClaudeTier
  tierSource: ClaudeTierSource
  windowCostEnabled: boolean
  windowCostLimit: number | null
  windowCostStickyReserve: number | null
  sessionLimitEnabled: boolean
  maxSessions: number | null
  sessionIdleTimeoutMinutes: number | null
  rpmLimitEnabled: boolean
  baseRPM: number | null
  rpmStrategy: ClaudeLimitStrategy
  tpmLimitEnabled: boolean
  baseTPM: number | null
  tpmStrategy: ClaudeLimitStrategy
  quotaLimit: number | null
  quotaDailyLimit: number | null
  quotaWeeklyLimit: number | null
}

export const CLAUDE_WEB_SAFETY_PRESET_VERSION = 1
export const CLAUDE_WEB_SAFETY_CONFIG_KEYS = [
  'claude_tier', 'claude_tier_source', 'claude_safety_preset_version',
  'window_cost_limit', 'window_cost_sticky_reserve',
  'max_sessions', 'session_idle_timeout_minutes',
  'base_rpm', 'rpm_strategy', 'base_tpm', 'tpm_strategy',
  'quota_limit', 'quota_daily_limit', 'quota_weekly_limit'
] as const

export const CLAUDE_WEB_SAFETY_PRESETS = {
  Pro: { baseRPM: 6, baseTPM: 80000, maxSessions: 2, windowCostLimit: 8, windowCostStickyReserve: 1 },
  Max_5x: { baseRPM: 12, baseTPM: 160000, maxSessions: 4, windowCostLimit: 40, windowCostStickyReserve: 5 },
  Max_20x: { baseRPM: 20, baseTPM: 240000, maxSessions: 6, windowCostLimit: 80, windowCostStickyReserve: 10 }
} as const

export function createClaudeWebSafetyState(): ClaudeWebSafetyState {
  return {
    tier: '',
    tierSource: '',
    windowCostEnabled: false,
    windowCostLimit: null,
    windowCostStickyReserve: null,
    sessionLimitEnabled: false,
    maxSessions: null,
    sessionIdleTimeoutMinutes: null,
    rpmLimitEnabled: false,
    baseRPM: null,
    rpmStrategy: 'tiered',
    tpmLimitEnabled: false,
    baseTPM: null,
    tpmStrategy: 'tiered',
    quotaLimit: null,
    quotaDailyLimit: null,
    quotaWeeklyLimit: null
  }
}

export function hydrateClaudeWebSafety(extra?: Record<string, unknown>): ClaudeWebSafetyState {
  const state = createClaudeWebSafetyState()
  if (!extra) return state
  const tier = extra.claude_tier
  if (tier === 'Pro' || tier === 'Max_5x' || tier === 'Max_20x') state.tier = tier
  const source = extra.claude_tier_source
  if (source === 'manual' || source === 'profile' || source === 'inferred' || source === 'profile_default') state.tierSource = source
  state.windowCostLimit = positiveNumber(extra.window_cost_limit)
  state.windowCostStickyReserve = positiveNumber(extra.window_cost_sticky_reserve)
  state.windowCostEnabled = state.windowCostLimit != null
  state.maxSessions = positiveNumber(extra.max_sessions)
  state.sessionIdleTimeoutMinutes = positiveNumber(extra.session_idle_timeout_minutes)
  state.sessionLimitEnabled = state.maxSessions != null
  state.baseRPM = positiveNumber(extra.base_rpm)
  state.rpmLimitEnabled = state.baseRPM != null
  state.rpmStrategy = extra.rpm_strategy === 'sticky_exempt' ? 'sticky_exempt' : 'tiered'
  state.baseTPM = positiveNumber(extra.base_tpm)
  state.tpmLimitEnabled = state.baseTPM != null
  state.tpmStrategy = extra.tpm_strategy === 'sticky_exempt' ? 'sticky_exempt' : 'tiered'
  state.quotaLimit = positiveNumber(extra.quota_limit)
  state.quotaDailyLimit = positiveNumber(extra.quota_daily_limit)
  state.quotaWeeklyLimit = positiveNumber(extra.quota_weekly_limit)
  return state
}

export function applyClaudeWebPreset(state: ClaudeWebSafetyState, tier: Exclude<ClaudeTier, ''>, options: { overwrite?: boolean } = {}): void {
  const preset = CLAUDE_WEB_SAFETY_PRESETS[tier]
  const overwrite = options.overwrite === true
  state.tier = tier
  state.tierSource = 'manual'
  state.windowCostEnabled = true
  state.sessionLimitEnabled = true
  state.rpmLimitEnabled = true
  state.tpmLimitEnabled = true
  if (overwrite || state.windowCostLimit == null) state.windowCostLimit = preset.windowCostLimit
  if (overwrite || state.windowCostStickyReserve == null) state.windowCostStickyReserve = preset.windowCostStickyReserve
  if (overwrite || state.maxSessions == null) state.maxSessions = preset.maxSessions
  if (overwrite || state.sessionIdleTimeoutMinutes == null) state.sessionIdleTimeoutMinutes = 5
  if (overwrite || state.baseRPM == null) state.baseRPM = preset.baseRPM
  if (overwrite || state.baseTPM == null) state.baseTPM = preset.baseTPM
}

export function serializeClaudeWebSafety(state: ClaudeWebSafetyState): Record<string, unknown> {
  const extra: Record<string, unknown> = {}
  if (state.tier) {
    extra.claude_tier = state.tier
    extra.claude_tier_source = state.tierSource || 'manual'
    extra.claude_safety_preset_version = CLAUDE_WEB_SAFETY_PRESET_VERSION
  }
  if (state.windowCostEnabled && positiveNumber(state.windowCostLimit) != null) {
    extra.window_cost_limit = state.windowCostLimit
    if (positiveNumber(state.windowCostStickyReserve) != null) extra.window_cost_sticky_reserve = state.windowCostStickyReserve
  }
  if (state.sessionLimitEnabled && positiveNumber(state.maxSessions) != null) {
    extra.max_sessions = state.maxSessions
    extra.session_idle_timeout_minutes = positiveNumber(state.sessionIdleTimeoutMinutes) ?? 5
  }
  if (state.rpmLimitEnabled && positiveNumber(state.baseRPM) != null) {
    extra.base_rpm = state.baseRPM
    extra.rpm_strategy = state.rpmStrategy
  }
  if (state.tpmLimitEnabled && positiveNumber(state.baseTPM) != null) {
    extra.base_tpm = state.baseTPM
    extra.tpm_strategy = state.tpmStrategy
  }
  if (positiveNumber(state.quotaLimit) != null) extra.quota_limit = state.quotaLimit
  if (positiveNumber(state.quotaDailyLimit) != null) extra.quota_daily_limit = state.quotaDailyLimit
  if (positiveNumber(state.quotaWeeklyLimit) != null) extra.quota_weekly_limit = state.quotaWeeklyLimit
  return extra
}

function positiveNumber(value: unknown): number | null {
  const parsed = typeof value === 'number' ? value : Number(value)
  return Number.isFinite(parsed) && parsed > 0 ? parsed : null
}
