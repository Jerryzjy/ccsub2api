export function applyInterceptWarmup(
  credentials: Record<string, unknown>,
  enabled: boolean,
  mode: 'create' | 'edit'
): void {
  if (enabled) {
    credentials.intercept_warmup_requests = true
  } else if (mode === 'edit') {
    delete credentials.intercept_warmup_requests
  }
}

export interface ClaudeWebCredentialInput {
  cookieFile?: string
  cookieHeader?: string
  sessionKey?: string
}

export function buildClaudeWebCredentials(
  input: ClaudeWebCredentialInput
): Record<string, string> {
  const cookie = input.cookieFile?.trim() || input.cookieHeader?.trim() || ''
  const sessionKey = input.sessionKey?.trim() || ''

  if (!cookie && !sessionKey) {
    throw new Error('claude_web_credentials_required')
  }

  const credentials: Record<string, string> = {}
  if (cookie) credentials.cookie = cookie
  if (sessionKey) credentials.session_key = sessionKey
  return credentials
}
