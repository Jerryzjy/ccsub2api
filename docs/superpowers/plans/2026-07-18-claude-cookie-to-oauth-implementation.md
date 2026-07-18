# Claude Cookie / sessionKey Automatic OAuth Account Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Allow an administrator to upload a Claude Cookie file or paste a raw sessionKey and have Sub2API exchange it server-side into OAuth credentials and create an Anthropic OAuth account without exposing `credentials.json` or tokens to the browser.

**Architecture:** Reuse the existing Cookie parser and Claude OAuth client, add a dedicated narrow-scope cookie conversion mode to `OAuthService`, then expose one atomic admin endpoint that exchanges credentials and calls the existing account creation service. The frontend reads Cookie files as text and sends either Cookie content or a raw sessionKey directly to the atomic endpoint.

**Tech Stack:** Go 1.x, Gin, req/v3, testify, Vue 3, TypeScript, Vitest, Axios.

---

## File map

- Modify `backend/internal/pkg/oauth/oauth.go`: define the Claude.ai-compatible minimal scope.
- Modify `backend/internal/service/oauth_service.go`: route cookie conversion requests to the minimal scope while preserving existing OAuth/setup-token behavior.
- Modify `backend/internal/service/oauth_service_test.go`: prove scope selection and token response validation.
- Create `backend/internal/handler/admin/account_claude_cookie_oauth.go`: parse Cookie/sessionKey, exchange token, and create the account atomically from the browser's perspective.
- Create `backend/internal/handler/admin/account_claude_cookie_oauth_test.go`: handler contract, secret non-disclosure, and account creation tests.
- Modify `backend/internal/server/routes/admin.go`: register the atomic endpoint.
- Modify `frontend/src/api/admin/accounts.ts`: add typed API request/response helper.
- Modify `frontend/src/components/account/OAuthAuthorizationFlow.vue`: accept Cookie files in addition to pasted sessionKey.
- Modify `frontend/src/components/account/CreateAccountModal.vue`: call the atomic endpoint instead of returning OAuth tokens to the browser for Anthropic cookie conversion.
- Modify `frontend/src/i18n/locales/zh.ts` and `frontend/src/i18n/locales/en.ts`: describe the combined Cookie/sessionKey input.
- Create `frontend/src/components/account/__tests__/claudeCookieOAuth.spec.ts`: input classification and API payload tests.

The existing dirty files under `backend/internal/service/claude_web_*` are not modified. `NormalizeClaudeWebCookie` is reused as an existing dependency.

### Task 1: Add a dedicated Claude.ai cookie OAuth scope

**Files:**
- Modify: `backend/internal/pkg/oauth/oauth.go:26-31`
- Modify: `backend/internal/service/oauth_service.go:161-224`
- Test: `backend/internal/service/oauth_service_test.go`

- [ ] **Step 1: Write failing scope-selection tests**

Add a test whose mock `GetAuthorizationCode` captures `scope` and whose mock exchange returns both token types:

```go
func TestOAuthServiceCookieAuthClaudeAIScope(t *testing.T) {
    client := &mockClaudeOAuthClient{
        getOrgUUIDFunc: func(context.Context, string, string) (string, error) {
            return "org-1", nil
        },
        getAuthCodeFunc: func(_ context.Context, _, _, scope, _, _, _ string) (string, error) {
            require.Equal(t, oauth.ScopeClaudeAI, scope)
            return "code#state", nil
        },
        exchangeCodeFunc: func(context.Context, string, string, string, string, bool) (*oauth.TokenResponse, error) {
            return &oauth.TokenResponse{
                AccessToken: "access",
                RefreshToken: "refresh",
                ExpiresIn: 28800,
                Scope: oauth.ScopeClaudeAI,
            }, nil
        },
    }
    svc := NewOAuthService(&mockProxyRepoForOAuth{}, client)
    got, err := svc.CookieAuth(context.Background(), &CookieAuthInput{
        SessionKey: "session",
        Scope: CookieAuthScopeClaudeAI,
    })
    require.NoError(t, err)
    require.Equal(t, "refresh", got.RefreshToken)
}
```

- [ ] **Step 2: Run the test and verify failure**

Run:

```powershell
cd backend
go test ./internal/service -run TestOAuthServiceCookieAuthClaudeAIScope -count=1
```

Expected: compilation failure because `ScopeClaudeAI` and `CookieAuthScopeClaudeAI` do not exist.

- [ ] **Step 3: Add the minimal constants and selection branch**

Add:

```go
const ScopeClaudeAI = "user:chat user:inference user:profile"
```

and:

```go
const CookieAuthScopeClaudeAI = "claude_ai"

switch input.Scope {
case "inference":
    scope = oauth.ScopeInference
    isSetupToken = true
case CookieAuthScopeClaudeAI:
    scope = oauth.ScopeClaudeAI
}
```

Do not change `ScopeAPI`, `ScopeOAuth`, or the setup-token path.

- [ ] **Step 4: Run targeted service tests**

Run:

```powershell
cd backend
go test ./internal/service -run 'TestOAuthService.*CookieAuth' -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit Task 1 files only**

```powershell
git add backend/internal/pkg/oauth/oauth.go backend/internal/service/oauth_service.go backend/internal/service/oauth_service_test.go
git commit -m "feat: add Claude cookie OAuth scope"
```

### Task 2: Add the atomic Cookie/sessionKey account endpoint

**Files:**
- Create: `backend/internal/handler/admin/account_claude_cookie_oauth.go`
- Create: `backend/internal/handler/admin/account_claude_cookie_oauth_test.go`
- Modify: `backend/internal/server/routes/admin.go:298-343`

- [ ] **Step 1: Write failing handler contract tests**

Cover raw sessionKey, Netscape Cookie input, missing input, missing refresh token, and secret-free responses. The success fixture must assert the created account input:

```go
require.Len(t, adminSvc.createdAccounts, 1)
created := adminSvc.createdAccounts[0]
require.Equal(t, service.PlatformAnthropic, created.Platform)
require.Equal(t, service.AccountTypeOAuth, created.Type)
require.Equal(t, "access", created.Credentials["access_token"])
require.Equal(t, "refresh", created.Credentials["refresh_token"])
require.NotContains(t, created.Credentials, "session_key")
require.NotContains(t, recorder.Body.String(), "session-secret")
require.NotContains(t, recorder.Body.String(), "refresh")
```

Use `newStubAdminService()` and a mocked `ClaudeOAuthClient` so no real credential leaves the test process.

- [ ] **Step 2: Run the handler tests and verify failure**

Run:

```powershell
cd backend
go test ./internal/handler/admin -run TestAccountHandlerCreateClaudeCookieOAuth -count=1
```

Expected: compilation failure because the handler does not exist.

- [ ] **Step 3: Implement request parsing and account creation**

Define a focused request DTO:

```go
type CreateClaudeCookieOAuthRequest struct {
    Name                    string         `json:"name" binding:"required"`
    Cookie                  string         `json:"cookie"`
    SessionKey              string         `json:"session_key"`
    Notes                   *string        `json:"notes"`
    Extra                   map[string]any `json:"extra"`
    ProxyID                 *int64         `json:"proxy_id"`
    Concurrency             int            `json:"concurrency"`
    Priority                int            `json:"priority"`
    RateMultiplier          *float64       `json:"rate_multiplier"`
    LoadFactor              *int           `json:"load_factor"`
    GroupIDs                []int64        `json:"group_ids"`
    ConfirmMixedChannelRisk *bool          `json:"confirm_mixed_channel_risk"`
}
```

Normalize input without storing Cookie:

```go
sessionKey := strings.TrimSpace(req.SessionKey)
if strings.TrimSpace(req.Cookie) != "" {
    normalized, err := service.NormalizeClaudeWebCookie(req.Cookie, time.Now())
    if err != nil {
        response.BadRequest(c, "invalid Claude Cookie")
        return
    }
    if normalized.SessionKey != "" {
        sessionKey = normalized.SessionKey
    }
}
if sessionKey == "" {
    response.BadRequest(c, "Cookie or sessionKey is required")
    return
}
```

Exchange using `CookieAuthScopeClaudeAI`, require non-empty access and refresh tokens, then call `adminService.CreateAccount` with flat credentials:

```go
credentials := map[string]any{
    "access_token": tokenInfo.AccessToken,
    "refresh_token": tokenInfo.RefreshToken,
    "expires_at": strconv.FormatInt(tokenInfo.ExpiresAt, 10),
    "scope": tokenInfo.Scope,
    "token_type": tokenInfo.TokenType,
}
```

Return `buildAccountResponseWithRuntime`, never `TokenInfo`.

- [ ] **Step 4: Register the endpoint**

Add:

```go
accounts.POST("/claude-cookie-oauth", h.Admin.Account.CreateClaudeCookieOAuth)
```

next to the standard account create route.

- [ ] **Step 5: Run handler and route tests**

Run:

```powershell
cd backend
go test ./internal/handler/admin -run 'TestAccountHandlerCreateClaudeCookieOAuth|Test.*APIContract' -count=1
go test ./internal/server -run TestAPIContract -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit Task 2 files only**

```powershell
git add backend/internal/handler/admin/account_claude_cookie_oauth.go backend/internal/handler/admin/account_claude_cookie_oauth_test.go backend/internal/server/routes/admin.go
git commit -m "feat: create OAuth accounts from Claude cookies"
```

### Task 3: Add the frontend API and input classifier

**Files:**
- Modify: `frontend/src/api/admin/accounts.ts`
- Create: `frontend/src/components/account/claudeCookieOAuth.ts`
- Create: `frontend/src/components/account/__tests__/claudeCookieOAuth.spec.ts`

- [ ] **Step 1: Write failing input-classification tests**

```ts
import { buildClaudeCookieOAuthInput } from '../claudeCookieOAuth'

it('treats a Netscape export as cookie content', () => {
  expect(buildClaudeCookieOAuthInput('.claude.ai\tTRUE\t/\tTRUE\t0\tsessionKey\tvalue')).toEqual({
    cookie: '.claude.ai\tTRUE\t/\tTRUE\t0\tsessionKey\tvalue',
    session_key: ''
  })
})

it('treats a raw sessionKey as session_key', () => {
  expect(buildClaudeCookieOAuthInput('sk-ant-sid-test')).toEqual({
    cookie: '',
    session_key: 'sk-ant-sid-test'
  })
})
```

- [ ] **Step 2: Run the test and verify failure**

Run:

```powershell
cd frontend
pnpm test -- src/components/account/__tests__/claudeCookieOAuth.spec.ts
```

Expected: FAIL because the utility does not exist.

- [ ] **Step 3: Implement the classifier**

```ts
export function buildClaudeCookieOAuthInput(rawValue: string) {
  const value = rawValue.trim()
  const isCookie = value.includes('\t') || /(^|;|\s)sessionKey=/.test(value)
  return isCookie
    ? { cookie: value, session_key: '' }
    : { cookie: '', session_key: value }
}
```

- [ ] **Step 4: Add the typed API helper**

```ts
export interface CreateClaudeCookieOAuthRequest extends Omit<CreateAccountRequest, 'platform' | 'type' | 'credentials'> {
  cookie: string
  session_key: string
}

export async function createClaudeCookieOAuth(
  payload: CreateClaudeCookieOAuthRequest
): Promise<Account> {
  const { data } = await apiClient.post<Account>('/admin/accounts/claude-cookie-oauth', payload)
  return data
}
```

Also add `createClaudeCookieOAuth` to the exported `accountsAPI` object so `adminAPI.accounts.createClaudeCookieOAuth` is available to the modal.

- [ ] **Step 5: Run the frontend unit test**

Run:

```powershell
cd frontend
pnpm test -- src/components/account/__tests__/claudeCookieOAuth.spec.ts
```

Expected: PASS.

- [ ] **Step 6: Commit Task 3 files only**

```powershell
git add frontend/src/api/admin/accounts.ts frontend/src/components/account/claudeCookieOAuth.ts frontend/src/components/account/__tests__/claudeCookieOAuth.spec.ts
git commit -m "feat: add Claude cookie OAuth client"
```

### Task 4: Wire Cookie file upload into account creation

**Files:**
- Modify: `frontend/src/components/account/OAuthAuthorizationFlow.vue:261-361, 681-855`
- Modify: `frontend/src/components/account/CreateAccountModal.vue:3211-3237, 5835-6015`
- Modify: `frontend/src/i18n/locales/zh.ts:3926-3940`
- Modify: `frontend/src/i18n/locales/en.ts:3778-3792`

- [ ] **Step 1: Add a failing component test for file input and atomic API use**

Extend the existing account modal test setup so a synthetic Cookie file produces one call to `createClaudeCookieOAuth` and no call to `/cookie-auth`:

```ts
expect(accountsAPI.createClaudeCookieOAuth).toHaveBeenCalledWith(
  expect.objectContaining({
    cookie: expect.stringContaining('sessionKey'),
    session_key: ''
  })
)
expect(accountsAPI.exchangeCode).not.toHaveBeenCalled()
```

- [ ] **Step 2: Run the component test and verify failure**

Run:

```powershell
cd frontend
pnpm test -- src/components/account/__tests__/CreateAccountModal.spec.ts
```

Expected: FAIL because the modal still calls `/cookie-auth` and has no OAuth Cookie file input.

- [ ] **Step 3: Add Cookie file reading to OAuthAuthorizationFlow**

Add a file input visible only for Anthropic cookie auth. Read it as text and place the content into the same sensitive in-memory ref used by the textarea:

```ts
const handleClaudeCookieFile = async (event: Event) => {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  if (!file) return
  sessionKeyInput.value = await file.text()
}
```

Reset the native file input and `sessionKeyInput` when the flow closes. Do not write the content to localStorage, Pinia, route state, or analytics.

- [ ] **Step 4: Replace the create-modal cookie exchange with the atomic endpoint**

For `addMethod === 'oauth'`, classify the emitted text and call `createClaudeCookieOAuth` with the existing account form settings. Preserve the setup-token flow on its existing endpoint.

```ts
const credentialInput = buildClaudeCookieOAuthInput(rawCredential)
await adminAPI.accounts.createClaudeCookieOAuth({
  name: form.name,
  ...credentialInput,
  proxy_id: form.proxy_id || undefined,
  group_ids: form.group_ids,
  concurrency: form.concurrency,
  priority: form.priority,
  rate_multiplier: form.rate_multiplier,
  extra: buildAccountExtra()
})
```

Do not receive or construct OAuth tokens in the browser on this path.

- [ ] **Step 5: Update Chinese and English copy**

Describe the accepted inputs as “Cookie 文件 / Cookie Header / sessionKey” and the button as “自动转换并创建 OAuth 账号”. State that conversion is performed server-side and no `credentials.json` is required.

- [ ] **Step 6: Run component and localization tests**

Run:

```powershell
cd frontend
pnpm test -- src/components/account/__tests__/CreateAccountModal.spec.ts src/components/account/__tests__/claudeCookieOAuth.spec.ts
pnpm typecheck
```

Expected: PASS.

- [ ] **Step 7: Commit Task 4 files only**

```powershell
git add frontend/src/components/account/OAuthAuthorizationFlow.vue frontend/src/components/account/CreateAccountModal.vue frontend/src/i18n/locales/zh.ts frontend/src/i18n/locales/en.ts frontend/src/components/account/__tests__/CreateAccountModal.spec.ts
git commit -m "feat: upload Claude cookies for OAuth accounts"
```

### Task 5: Verification and safe manual acceptance

**Files:**
- Modify only if verification reveals a defect in files already listed above.

- [ ] **Step 1: Run backend targeted tests**

```powershell
cd backend
go test ./internal/service ./internal/handler/admin ./internal/server -count=1
```

Expected: PASS.

- [ ] **Step 2: Run backend static checks**

```powershell
cd backend
go test -tags unit ./... -run '^$'
go vet ./...
```

Expected: PASS.

- [ ] **Step 3: Run frontend tests and type checking**

```powershell
cd frontend
pnpm test -- src/components/account/__tests__/CreateAccountModal.spec.ts src/components/account/__tests__/claudeCookieOAuth.spec.ts
pnpm typecheck
```

Expected: PASS.

- [ ] **Step 4: Check whitespace and scope**

```powershell
git diff --check
git status --short
```

Expected: no whitespace errors; pre-existing dirty Claude Web files remain untouched unless explicitly required and reviewed.

- [ ] **Step 5: Perform one credential-safe manual acceptance**

Using an administrator-owned Cookie and configured proxy, submit the new endpoint once. Record only stage, HTTP status, created account ID, account type, and refresh success. Never print or commit Cookie/sessionKey/token values. Immediately rotate the test credential after acceptance.

- [ ] **Step 6: Final commit if verification required fixes**

```powershell
git add backend/internal/pkg/oauth/oauth.go backend/internal/service/oauth_service.go backend/internal/service/oauth_service_test.go backend/internal/handler/admin/account_claude_cookie_oauth.go backend/internal/handler/admin/account_claude_cookie_oauth_test.go backend/internal/server/routes/admin.go frontend/src/api/admin/accounts.ts frontend/src/components/account/claudeCookieOAuth.ts frontend/src/components/account/__tests__/claudeCookieOAuth.spec.ts frontend/src/components/account/OAuthAuthorizationFlow.vue frontend/src/components/account/CreateAccountModal.vue frontend/src/components/account/__tests__/CreateAccountModal.spec.ts frontend/src/i18n/locales/zh.ts frontend/src/i18n/locales/en.ts
git commit -m "fix: verify Claude cookie OAuth creation"
```
