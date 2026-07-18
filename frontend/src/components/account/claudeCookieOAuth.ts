export interface ClaudeCookieOAuthInput {
  cookie: string
  session_key: string
}

export function buildClaudeCookieOAuthInput(rawValue: string): ClaudeCookieOAuthInput {
  const value = rawValue.trim()
  const isCookie = value.includes('\t') || /(^|;|\s)sessionKey=/.test(value)

  return isCookie
    ? { cookie: value, session_key: '' }
    : { cookie: '', session_key: value }
}
