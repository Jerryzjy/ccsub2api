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

export async function readClaudeCookieFile(file: File): Promise<string> {
  if (typeof file.text === 'function') return file.text()

  return new Promise<string>((resolve, reject) => {
    const reader = new FileReader()
    reader.onerror = () => reject(reader.error || new Error('claude_cookie_file_read_failed'))
    reader.onload = () => resolve(typeof reader.result === 'string' ? reader.result : '')
    reader.readAsText(file)
  })
}

export function formatClaudeCookieOAuthError(error: any, fallback: string): string {
  return error?.response?.data?.detail
    || error?.response?.data?.message
    || error?.message
    || fallback
}
