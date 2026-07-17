import { describe, it, expect } from 'vitest'
import { applyInterceptWarmup, buildClaudeWebCredentials } from '../credentialsBuilder'

describe('applyInterceptWarmup', () => {
  it('create + enabled=true: should set intercept_warmup_requests to true', () => {
    const creds: Record<string, unknown> = { access_token: 'tok' }
    applyInterceptWarmup(creds, true, 'create')
    expect(creds.intercept_warmup_requests).toBe(true)
  })

  it('create + enabled=false: should not add the field', () => {
    const creds: Record<string, unknown> = { access_token: 'tok' }
    applyInterceptWarmup(creds, false, 'create')
    expect('intercept_warmup_requests' in creds).toBe(false)
  })

  it('edit + enabled=true: should set intercept_warmup_requests to true', () => {
    const creds: Record<string, unknown> = { api_key: 'sk' }
    applyInterceptWarmup(creds, true, 'edit')
    expect(creds.intercept_warmup_requests).toBe(true)
  })

  it('edit + enabled=false + field exists: should delete the field', () => {
    const creds: Record<string, unknown> = { api_key: 'sk', intercept_warmup_requests: true }
    applyInterceptWarmup(creds, false, 'edit')
    expect('intercept_warmup_requests' in creds).toBe(false)
  })

  it('edit + enabled=false + field absent: should not throw', () => {
    const creds: Record<string, unknown> = { api_key: 'sk' }
    applyInterceptWarmup(creds, false, 'edit')
    expect('intercept_warmup_requests' in creds).toBe(false)
  })

  it('should not affect other fields', () => {
    const creds: Record<string, unknown> = {
      api_key: 'sk',
      base_url: 'url',
      intercept_warmup_requests: true
    }
    applyInterceptWarmup(creds, false, 'edit')
    expect(creds.api_key).toBe('sk')
    expect(creds.base_url).toBe('url')
    expect('intercept_warmup_requests' in creds).toBe(false)
  })
})

describe('buildClaudeWebCredentials', () => {
  it('prefers an exported cookie file over a pasted cookie header', () => {
    expect(buildClaudeWebCredentials({
      cookieFile: '.claude.ai\tTRUE\t/\tTRUE\t0\tsessionKey\tfile-value',
      cookieHeader: 'sessionKey=header-value',
      sessionKey: 'fallback-value'
    })).toEqual({
      cookie: '.claude.ai\tTRUE\t/\tTRUE\t0\tsessionKey\tfile-value',
      session_key: 'fallback-value'
    })
  })

  it('accepts a cookie header without duplicating it as a session key', () => {
    expect(buildClaudeWebCredentials({
      cookieHeader: 'sessionKey=test-value; lastActiveOrg=org-test'
    })).toEqual({
      cookie: 'sessionKey=test-value; lastActiveOrg=org-test'
    })
  })

  it('accepts sessionKey as a fallback when no full cookie is available', () => {
    expect(buildClaudeWebCredentials({ sessionKey: 'sk-test' })).toEqual({
      session_key: 'sk-test'
    })
  })

  it('rejects empty credentials without including credential values in the error', () => {
    expect(() => buildClaudeWebCredentials({
      cookieFile: '  ',
      cookieHeader: '\n',
      sessionKey: '\t'
    })).toThrowError('claude_web_credentials_required')
  })
})
