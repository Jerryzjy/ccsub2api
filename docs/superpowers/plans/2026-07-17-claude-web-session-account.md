# Claude Web Session Account Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a native Anthropic `web_session` account type that imports Claude Web cookies, participates in the existing account pool, and translates Claude Web completions to Anthropic-compatible responses.

**Architecture:** Add an isolated Claude Web protocol adapter beside the existing Anthropic OAuth/APIKey transports. The adapter uses a dedicated Chrome 146 browser transport backed by the licensed `bogdanfinn/tls-client` library, while reusing account-bound proxy settings, scheduler state, concurrency accounting, and response billing. Cookie credentials never enter the OAuth token provider, and the admin UI creates `web_session` accounts from a Cookie header, Netscape cookie file, or sessionKey fallback.

**Tech Stack:** Go 1.26, Gin, `gjson`/`sjson`, `github.com/bogdanfinn/tls-client` Chrome 146 profile, Vue 3, TypeScript, Vitest.

---

### Task 1: Account Type And Credential Validation

**Files:**
- Modify: `backend/internal/domain/constants.go`
- Modify: `backend/internal/service/domain_constants.go`
- Modify: `backend/internal/handler/admin/account_handler.go`
- Modify: `backend/internal/handler/admin/account_data.go`
- Modify: `backend/internal/service/account.go`
- Test: `backend/internal/handler/admin/account_data_handler_test.go`
- Create: `backend/internal/service/claude_web_credentials_test.go`
- Create: `backend/internal/service/claude_web_credentials.go`

- [ ] **Step 1: Write failing validation tests**

```go
func TestValidateClaudeWebSessionCredentials(t *testing.T) {
    tests := []struct {
        name string
        platform string
        credentials map[string]any
        wantErr string
    }{
        {"cookie", PlatformAnthropic, map[string]any{"cookie": "sessionKey=abc"}, ""},
        {"session fallback", PlatformAnthropic, map[string]any{"session_key": "sk-ant-sid01-test"}, ""},
        {"wrong platform", PlatformOpenAI, map[string]any{"cookie": "sessionKey=abc"}, "web_session is only supported for anthropic"},
        {"missing credential", PlatformAnthropic, map[string]any{}, "cookie or session_key is required"},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := ValidateClaudeWebSessionCredentials(tt.platform, tt.credentials)
            if tt.wantErr == "" { require.NoError(t, err); return }
            require.EqualError(t, err, tt.wantErr)
        })
    }
}
```

- [ ] **Step 2: Run the tests and verify RED**

Run: `go test ./internal/service ./internal/handler/admin -run 'TestValidateClaudeWebSessionCredentials|TestAccountDataImport'`

Expected: FAIL because `AccountTypeWebSession` and `ValidateClaudeWebSessionCredentials` do not exist.

- [ ] **Step 3: Add the account constant and validator**

```go
const AccountTypeWebSession = "web_session"

func ValidateClaudeWebSessionCredentials(platform string, credentials map[string]any) error {
    if platform != PlatformAnthropic {
        return errors.New("web_session is only supported for anthropic")
    }
    if strings.TrimSpace(credentialString(credentials, "cookie")) == "" &&
        strings.TrimSpace(credentialString(credentials, "session_key")) == "" {
        return errors.New("cookie or session_key is required")
    }
    return nil
}
```

Extend the create binding `oneof`, data import type switch, and create/import validators. Add `Account.IsClaudeWebSession()` for platform/type checks.

- [ ] **Step 4: Run the targeted tests and verify GREEN**

Run: `go test ./internal/service ./internal/handler/admin -run 'TestValidateClaudeWebSessionCredentials|TestAccountDataImport'`

Expected: PASS.

### Task 2: Cookie Header And Netscape Parser

**Files:**
- Modify: `backend/internal/service/claude_web_credentials.go`
- Modify: `backend/internal/service/claude_web_credentials_test.go`

- [ ] **Step 1: Write failing parser tests**

```go
func TestNormalizeClaudeWebCookie_Netscape(t *testing.T) {
    raw := "# Netscape HTTP Cookie File\n.claude.ai\tTRUE\t/\tTRUE\t4102444800\tsessionKey\tsk-test\nexample.com\tTRUE\t/\tTRUE\t4102444800\tignored\tx\n"
    got, err := NormalizeClaudeWebCookie(raw, time.Unix(1_800_000_000, 0))
    require.NoError(t, err)
    require.Equal(t, "sessionKey=sk-test", got.Header)
    require.Equal(t, "sk-test", got.SessionKey)
}

func TestNormalizeClaudeWebCookie_HeaderPrefersCookieSessionKey(t *testing.T) {
    got, err := NormalizeClaudeWebCookie("foo=bar; sessionKey=cookie-value", time.Now())
    require.NoError(t, err)
    require.Equal(t, "cookie-value", got.SessionKey)
}
```

- [ ] **Step 2: Run and verify RED**

Run: `go test ./internal/service -run TestNormalizeClaudeWebCookie`

Expected: FAIL because the parser is missing.

- [ ] **Step 3: Implement a deterministic parser**

```go
type ClaudeWebCookie struct {
    Header         string
    SessionKey     string
    OrganizationID string
}

func NormalizeClaudeWebCookie(raw string, now time.Time) (ClaudeWebCookie, error)
```

The implementation must parse Cookie Header and Netscape formats, ignore comments/blank lines/expired cookies/non-Claude domains, prefer root-path `.claude.ai` duplicates, sort output by cookie name, and never include raw values in errors.

- [ ] **Step 4: Run and verify GREEN**

Run: `go test ./internal/service -run TestNormalizeClaudeWebCookie`

Expected: PASS.

### Task 3: Claude Web Protocol Client

**Files:**
- Modify: `backend/go.mod`
- Modify: `backend/go.sum`
- Create: `backend/internal/service/claude_web_client.go`
- Create: `backend/internal/service/claude_web_client_test.go`
- Create: `backend/internal/service/claude_web_transport.go`
- Create: `backend/internal/service/claude_web_transport_test.go`
- Create: `backend/internal/service/claude_web_protocol.go`
- Create: `backend/internal/service/claude_web_protocol_test.go`

- [ ] **Step 1: Write failing request-contract tests**

```go
func TestClaudeWebClientResolveOrganization(t *testing.T) {
    upstream := &recordingClaudeWebTransport{responses: []*http.Response{
        jsonResponse(200, `[{"uuid":"org-1"}]`),
    }}
    client := NewClaudeWebClient(upstream)
    account := &Account{ID: 7, Type: AccountTypeWebSession, Platform: PlatformAnthropic,
        Credentials: map[string]any{"cookie": "sessionKey=test"}}
    got, err := client.ResolveOrganization(context.Background(), account, "")
    require.NoError(t, err)
    require.Equal(t, "org-1", got)
    require.Equal(t, "https://claude.ai/api/organizations", upstream.requests[0].URL.String())
    require.Equal(t, "sessionKey=test", upstream.requests[0].Header.Get("Cookie"))
}
```

Add tests for cached `organization_id`, create conversation, completion URL, browser header consistency, account proxy forwarding, and best-effort delete.

- [ ] **Step 2: Run and verify RED**

Run: `go test ./internal/service -run 'TestClaudeWebClient|TestBuildClaudeWeb'`

Expected: FAIL because the client and protocol types are missing.

- [ ] **Step 3: Implement the client and request models**

```go
type ClaudeWebClient struct {
    transport ClaudeWebTransport
    baseURL string
}

type ClaudeWebTransport interface {
    Do(ctx context.Context, req *http.Request, proxyURL string, accountID int64) (*http.Response, error)
}

func NewClaudeWebClient(transport ClaudeWebTransport) *ClaudeWebClient
func (c *ClaudeWebClient) ResolveOrganization(ctx context.Context, account *Account, proxyURL string) (string, error)
func (c *ClaudeWebClient) CreateConversation(ctx context.Context, account *Account, proxyURL, orgID string) (string, error)
func (c *ClaudeWebClient) Complete(ctx context.Context, account *Account, proxyURL, orgID, conversationID string, payload ClaudeWebCompletionRequest) (*http.Response, error)
func (c *ClaudeWebClient) DeleteConversation(ctx context.Context, account *Account, proxyURL, orgID, conversationID string) error
```

The production transport creates/reuses `tls-client` Chrome 146 clients keyed by account ID and proxy URL. Browser headers declare Chrome 146 consistently and preserve the complete normalized Cookie. Signed Cloudflare values are replayed only when supplied. The transport supports HTTP, HTTPS, SOCKS5 and SOCKS5H proxy URLs through the library's proxy option.

- [ ] **Step 4: Implement Anthropic-to-Web prompt conversion**

```go
func BuildClaudeWebPrompt(body []byte) (string, error)
func BuildClaudeWebCompletion(body []byte, model, prompt string) (ClaudeWebCompletionRequest, error)
```

Support text system/user/assistant blocks. Return `ErrClaudeWebUnsupportedContent` for tool/image/document blocks in the first release instead of silently flattening them.

- [ ] **Step 5: Run and verify GREEN**

Run: `go test ./internal/service -run 'TestClaudeWebClient|TestBuildClaudeWeb'`

Expected: PASS.

### Task 4: Claude Web SSE Translation

**Files:**
- Modify: `backend/internal/service/claude_web_protocol.go`
- Modify: `backend/internal/service/claude_web_protocol_test.go`

- [ ] **Step 1: Write failing SSE translation tests**

```go
func TestTranslateClaudeWebSSE(t *testing.T) {
    input := "event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"hello\"}}\n\nevent: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"
    var out bytes.Buffer
    usage, err := TranslateClaudeWebSSE(strings.NewReader(input), &out, ClaudeWebStreamMeta{Model: "claude-sonnet-4-5", MessageID: "msg-test"})
    require.NoError(t, err)
    require.Contains(t, out.String(), `"type":"content_block_delta"`)
    require.Contains(t, out.String(), `"text":"hello"`)
    require.Contains(t, out.String(), `"type":"message_stop"`)
    require.Greater(t, usage.OutputTokens, 0)
}
```

- [ ] **Step 2: Run and verify RED**

Run: `go test ./internal/service -run TestTranslateClaudeWebSSE`

Expected: FAIL because translation is missing.

- [ ] **Step 3: Implement stream and non-stream adapters**

```go
func TranslateClaudeWebSSE(src io.Reader, dst io.Writer, meta ClaudeWebStreamMeta) (ClaudeUsage, error)
func AggregateClaudeWebSSE(src io.Reader, model, messageID string) ([]byte, ClaudeUsage, error)
```

Emit valid Anthropic `message_start`, `content_block_start`, deltas, `content_block_stop`, `message_delta`, and `message_stop`. Estimate tokens only when upstream usage is absent.

- [ ] **Step 4: Run and verify GREEN**

Run: `go test ./internal/service -run 'TestTranslateClaudeWebSSE|TestAggregateClaudeWebSSE'`

Expected: PASS.

### Task 5: Gateway Routing And Error Classification

**Files:**
- Create: `backend/internal/service/gateway_claude_web.go`
- Create: `backend/internal/service/gateway_claude_web_test.go`
- Modify: `backend/internal/service/gateway_service.go`
- Modify: `backend/internal/service/account_test_service.go`

- [ ] **Step 1: Write failing gateway routing tests**

```go
func TestGatewayForwardClaudeWebSessionUsesWebTransport(t *testing.T) {
    // Queue organization, create, completion and delete responses.
    // Assert Forward writes Anthropic SSE, uses the account proxy, and never asks ClaudeTokenProvider for access_token.
}

func TestClassifyClaudeWebResponse(t *testing.T) {
    require.Equal(t, ClaudeWebErrorExpired, ClassifyClaudeWebResponse(401, http.Header{}, []byte("")))
    require.Equal(t, ClaudeWebErrorCloudflare, ClassifyClaudeWebResponse(403, http.Header{"Cf-Mitigated": {"challenge"}}, []byte("<html>")))
    require.Equal(t, ClaudeWebErrorRateLimited, ClassifyClaudeWebResponse(429, nil, nil))
}
```

- [ ] **Step 2: Run and verify RED**

Run: `go test ./internal/service -run 'TestGatewayForwardClaudeWeb|TestClassifyClaudeWeb'`

Expected: FAIL because gateway routing is missing.

- [ ] **Step 3: Implement the isolated gateway path**

```go
if account != nil && account.IsClaudeWebSession() {
    return s.forwardClaudeWebSession(ctx, c, account, parsed, startTime)
}
```

`forwardClaudeWebSession` resolves organization, creates a temporary conversation, sends completion, maps errors to `UpstreamFailoverError`, streams/aggregates the response, and deletes the conversation with a detached timeout context.

- [ ] **Step 4: Add account connectivity testing and automatic probe scheduling**

Route `AccountTestService.testClaudeAccountConnection` to a Web Session probe before OAuth/APIKey authentication. The test resolves organization and sends a minimal text completion through the account proxy. Add `scheduleClaudeWebSessionProbe` beside the existing OpenAI probe and call it after create/update; success caches `organization_id`, while 401/403 marks the new account unschedulable with a sanitized error reason.

- [ ] **Step 5: Run and verify GREEN**

Run: `go test ./internal/service -run 'TestGatewayForwardClaudeWeb|TestClassifyClaudeWeb|TestClaudeWebAccountConnection'`

Expected: PASS.

### Task 6: Admin UI And Cookie File Import

**Files:**
- Modify: `frontend/src/types/index.ts`
- Modify: `frontend/src/constants/account.ts`
- Modify: `frontend/src/components/account/CreateAccountModal.vue`
- Modify: `frontend/src/components/account/EditAccountModal.vue`
- Modify: `frontend/src/components/account/credentialsBuilder.ts`
- Create: `frontend/src/components/account/__tests__/claudeWebCredentials.spec.ts`
- Modify: `frontend/src/i18n/locales/zh.ts`
- Modify: `frontend/src/i18n/locales/en.ts`

- [ ] **Step 1: Write failing frontend credential tests**

```ts
it('builds a web_session account from a Netscape cookie file', () => {
  expect(buildClaudeWebCredentials({ cookieFileText: netscape, cookieHeader: '', sessionKey: '' }))
    .toEqual({ cookie: netscape })
})

it('requires cookie or sessionKey', () => {
  expect(() => buildClaudeWebCredentials({ cookieFileText: '', cookieHeader: ' ', sessionKey: '' }))
    .toThrow('cookieOrSessionKeyRequired')
})
```

- [ ] **Step 2: Run and verify RED**

Run from `frontend`: `npm run test -- --run src/components/account/__tests__/claudeWebCredentials.spec.ts`

Expected: FAIL because the builder and account type are missing.

- [ ] **Step 3: Add the type, builder and create form**

```ts
export type AccountType = 'oauth' | 'setup-token' | 'apikey' | 'upstream' | 'bedrock' | 'service_account' | 'web_session'

export function buildClaudeWebCredentials(input: ClaudeWebCredentialInput): Record<string, string> {
  const cookie = input.cookieFileText.trim() || input.cookieHeader.trim()
  const sessionKey = input.sessionKey.trim()
  if (!cookie && !sessionKey) throw new Error('cookieOrSessionKeyRequired')
  return { ...(cookie ? { cookie } : {}), ...(sessionKey ? { session_key: sessionKey } : {}) }
}
```

Add a `Claude Web` account card only for Anthropic. Use a file input accepting `.txt`; read with `File.text()`. Do not display parsed secret values elsewhere. The create request uses `type: 'web_session'` and existing proxy/group fields.

- [ ] **Step 4: Add edit and list labels**

Editing preserves redacted credentials when fields are blank and replaces only explicitly supplied Cookie/sessionKey values. Add Chinese and English labels for the type and error categories.

- [ ] **Step 5: Run and verify GREEN**

Run from `frontend`: `npm run test -- --run src/components/account/__tests__/claudeWebCredentials.spec.ts`

Expected: PASS.

### Task 7: Regression And Release Verification

**Files:**
- Modify only if verification exposes a feature-related defect.

- [ ] **Step 1: Run backend focused tests**

Run: `go test ./internal/service ./internal/handler/admin -run 'ClaudeWeb|WebSession|AccountDataImport'`

Expected: PASS.

- [ ] **Step 2: Run backend compile suite**

Run: `go test -tags unit ./... -run '^$'`

Expected: PASS.

- [ ] **Step 3: Run frontend tests and typecheck**

Run from `frontend`: `npm run test -- --run src/components/account/__tests__/claudeWebCredentials.spec.ts`

Run: `npm run type-check`

Expected: PASS.

- [ ] **Step 4: Run static checks**

Run: `go vet ./...`

Run: `git diff --check`

Expected: PASS with no whitespace errors.

- [ ] **Step 5: Perform local manual smoke test without real credentials**

Start the application and verify the account modal exposes Claude Web only under Anthropic, file selection does not resize the dialog, validation blocks empty credentials, and existing OAuth/APIKey creation remains unchanged. Do not commit or log a real Cookie.
