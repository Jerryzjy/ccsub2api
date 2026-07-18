import { describe, expect, it } from 'vitest'
import { buildClaudeCookieOAuthInput, readClaudeCookieFile } from '../claudeCookieOAuth'

describe('buildClaudeCookieOAuthInput', () => {
  it('treats a Netscape export as cookie content', () => {
    const cookie = '.claude.ai\tTRUE\t/\tTRUE\t0\tsessionKey\tvalue'
    expect(buildClaudeCookieOAuthInput(cookie)).toEqual({
      cookie,
      session_key: ''
    })
  })

  it('treats a Cookie header as cookie content', () => {
    const cookie = 'sessionKey=value; lastActiveOrg=org-1'
    expect(buildClaudeCookieOAuthInput(cookie)).toEqual({
      cookie,
      session_key: ''
    })
  })

  it('treats a raw sessionKey as session_key', () => {
    expect(buildClaudeCookieOAuthInput('sk-ant-sid-test')).toEqual({
      cookie: '',
      session_key: 'sk-ant-sid-test'
    })
  })
})

describe('readClaudeCookieFile', () => {
  it('reads the Cookie export as text without transforming secrets', async () => {
    const content = '.claude.ai\tTRUE\t/\tTRUE\t0\tsessionKey\tsecret-value'
    const file = new File([content], 'cookies.txt', { type: 'text/plain' })
    await expect(readClaudeCookieFile(file)).resolves.toBe(content)
  })
})
