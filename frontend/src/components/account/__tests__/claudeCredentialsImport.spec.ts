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
      scope: 'full'
    }))

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
        subscription_type: 'claude_pro'
      }
    })
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
})
