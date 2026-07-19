export interface ClaudeCredentialsImportResult {
  credentials: Record<string, string>
  extra: Record<string, unknown>
}

function asRecord(value: unknown): Record<string, unknown> | null {
  return value !== null && typeof value === 'object' && !Array.isArray(value)
    ? value as Record<string, unknown>
    : null
}

function requiredString(value: unknown, field: string): string {
  if (typeof value !== 'string' || !value.trim()) {
    throw new Error(`credentials.json is missing ${field}`)
  }
  return value.trim()
}

function normalizeExpiresAt(value: unknown): string {
  const parsed = typeof value === 'number'
    ? value
    : typeof value === 'string' && value.trim()
      ? Number(value)
      : Number.NaN

  if (!Number.isFinite(parsed) || parsed <= 0) {
    throw new Error('credentials.json has an invalid claudeAiOauth.expiresAt')
  }

  const seconds = parsed >= 100_000_000_000 ? Math.floor(parsed / 1000) : Math.floor(parsed)
  return String(seconds)
}

function optionalPercentRatio(value: unknown): number | null {
  const parsed = typeof value === 'number'
    ? value
    : typeof value === 'string' && value.trim()
      ? Number(value)
      : Number.NaN

  if (!Number.isFinite(parsed) || parsed < 0 || parsed > 100) return null
  return parsed / 100
}

function optionalUnixSeconds(value: unknown): number | null {
  if (typeof value !== 'string' || !value.trim()) return null
  const parsed = Date.parse(value)
  if (!Number.isFinite(parsed)) return null
  return Math.floor(parsed / 1000)
}

export function parseClaudeCredentialsJSON(
  content: string,
  importedAt = new Date()
): ClaudeCredentialsImportResult {
  let root: Record<string, unknown>
  try {
    root = asRecord(JSON.parse(content)) || {}
  } catch {
    throw new Error('credentials.json is not valid JSON')
  }

  const oauth = asRecord(root.claudeAiOauth)
  if (!oauth) {
    throw new Error('credentials.json is missing claudeAiOauth')
  }

  const credentials: Record<string, string> = {
    access_token: requiredString(oauth.accessToken, 'claudeAiOauth.accessToken'),
    refresh_token: requiredString(oauth.refreshToken, 'claudeAiOauth.refreshToken'),
    expires_at: normalizeExpiresAt(oauth.expiresAt),
    token_type: 'Bearer'
  }

  if (Array.isArray(oauth.scopes)) {
    const scopes = oauth.scopes
      .filter((scope): scope is string => typeof scope === 'string')
      .map((scope) => scope.trim())
      .filter(Boolean)
    if (scopes.length > 0) credentials.scope = scopes.join(' ')
  }

  const extra: Record<string, unknown> = {}
  if (typeof root.email === 'string' && root.email.trim()) {
    extra.email_address = root.email.trim()
  }
  if (typeof root.accountType === 'string' && root.accountType.trim()) {
    extra.account_type = root.accountType.trim()
  }
  if (typeof oauth.subscriptionType === 'string' && oauth.subscriptionType.trim()) {
    extra.subscription_type = oauth.subscriptionType.trim()
  }

  const usage = asRecord(root.usage)
  if (usage) {
    const weeklyUtilization = optionalPercentRatio(usage.weekly_used_percent)
    const weeklyReset = optionalUnixSeconds(usage.weekly_resets_at)
    if (weeklyUtilization !== null) {
      extra.passive_usage_7d_utilization = weeklyUtilization
    }
    if (weeklyReset !== null) {
      extra.passive_usage_7d_reset = weeklyReset
    }
    if (weeklyUtilization !== null || weeklyReset !== null) {
      extra.passive_usage_sampled_at = importedAt.toISOString()
    }
  }

  return { credentials, extra }
}
