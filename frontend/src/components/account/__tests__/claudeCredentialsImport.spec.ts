import { describe, expect, it } from 'vitest'
import { parseClaudeCredentialsJSON } from '../claudeCredentialsImport'

describe('parseClaudeCredentialsJSON', () => {
  it('maps Claude credentials.json into Sub2API OAuth credentials', () => {
    const result = parseClaudeCredentialsJSON(JSON.stringify({
      claudeAiOauth: {
        accessToken: 'access-token',
        refreshToken: 'refresh-token',
        expiresAt: 1784362707000,
        scopes: ['user:chat', 'user:profile'],
        subscriptionType: 'claude_pro'
      },
      email: 'owner@example.com',
      accountType: 'apple_subscription',
      scope: 'full',
      usage: {
        session_used_percent: 25,
        session_resets_at: '2026-07-19T12:00:00Z',
        weekly_used_percent: 10,
        weekly_resets_at: '2026-07-19T18:00:00.141661+00:00'
      }
    }), new Date('2026-07-19T08:00:00Z'))

    expect(result).toEqual({
      credentials: {
        access_token: 'access-token',
        refresh_token: 'refresh-token',
        expires_at: '1784362707',
        scope: 'user:chat user:profile',
        token_type: 'Bearer'
      },
      extra: {
        email_address: 'owner@example.com',
        account_type: 'apple_subscription',
        subscription_type: 'claude_pro',
        passive_usage_7d_utilization: 0.1,
        passive_usage_7d_reset: 1784484000,
        passive_usage_sampled_at: '2026-07-19T08:00:00.000Z'
      }
    })
    expect(result.extra).not.toHaveProperty('session_window_utilization')
  })

  it('accepts an already-second-based expiry timestamp', () => {
    const result = parseClaudeCredentialsJSON(JSON.stringify({
      claudeAiOauth: {
        accessToken: 'access-token',
        refreshToken: 'refresh-token',
        expiresAt: 1784362707
      }
    }))

    expect(result.credentials.expires_at).toBe('1784362707')
  })

  it('rejects files without both OAuth tokens', () => {
    expect(() => parseClaudeCredentialsJSON(JSON.stringify({
      claudeAiOauth: { accessToken: 'access-token', expiresAt: 1784362707000 }
    }))).toThrow('refreshToken')
  })

  it('ignores malformed optional usage fields without rejecting valid OAuth credentials', () => {
    const result = parseClaudeCredentialsJSON(JSON.stringify({
      claudeAiOauth: {
        accessToken: 'access-token',
        refreshToken: 'refresh-token',
        expiresAt: 1784362707000
      },
      usage: {
        session_used_percent: 101,
        session_resets_at: 'not-a-date',
        weekly_used_percent: 'unknown',
        weekly_resets_at: 'not-a-date'
      }
    }))

    expect(result.extra).toEqual({})
  })
})
