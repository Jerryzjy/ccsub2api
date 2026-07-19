# Claude OAuth Compatibility Restoration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Restore Claude OAuth sessionKey authorization alongside credentials import and preserve imported initial usage windows without changing post-creation account behavior.

**Architecture:** Restore the deleted HTTP/UI adapter around the existing full-scope `OAuthService.CookieAuth`. Extend the existing frontend credentials parser to translate optional usage metadata into the passive-cache fields already consumed by `AccountUsageService`.

**Tech Stack:** Vue 3, TypeScript, Vitest, Go, Gin, Go testing.

---

### Task 1: Restore the OAuth sessionKey entry

**Files:**
- Modify: `frontend/src/components/account/__tests__/OAuthAuthorizationFlow.spec.ts`
- Modify: `frontend/src/components/account/CreateAccountModal.vue`
- Modify: `frontend/src/i18n/locales/zh.ts`
- Modify: `frontend/src/i18n/locales/en.ts`

- [ ] **Step 1: Write the failing component test**

Change the OAuth test to mount with both options enabled and require both labels:

```ts
expect(wrapper.text()).toContain('admin.accounts.oauth.cookieAutoAuth')
expect(wrapper.text()).toContain('admin.accounts.oauth.credentialsImport')
```

- [ ] **Step 2: Verify the component test fails**

Run: `pnpm vitest run src/components/account/__tests__/OAuthAuthorizationFlow.spec.ts`

Expected: FAIL because the OAuth parent currently passes `showCookieOption=false`.

- [ ] **Step 3: Restore the parent wiring and endpoint selection**

Set `show-cookie-option` for all Anthropic OAuth/Setup Token flows and remove the guard that rejects OAuth in `handleCookieAuth`. Select `/admin/accounts/cookie-auth` for OAuth and `/admin/accounts/setup-token-cookie-auth` for Setup Token.

- [ ] **Step 4: Correct the bilingual copy**

Label the option as sessionKey authorization and describe that it succeeds only when Anthropic accepts the supplied session. Keep credentials import copy separate and do not claim Cookie conversion.

- [ ] **Step 5: Verify the component test passes**

Run: `pnpm vitest run src/components/account/__tests__/OAuthAuthorizationFlow.spec.ts`

Expected: PASS.

### Task 2: Restore the backend OAuth adapter

**Files:**
- Create: `backend/internal/handler/admin/account_cookie_auth_test.go`
- Modify: `backend/internal/handler/admin/account_handler.go`
- Modify: `backend/internal/server/routes/admin.go`

- [ ] **Step 1: Write the failing handler test**

Create a fake `service.ClaudeOAuthClient`, invoke `OAuthHandler.CookieAuth` with a redacted sessionKey, and assert the captured authorization scope equals `oauth.ScopeAPI`, the exchange is not a Setup Token exchange, and the response is HTTP 200.

- [ ] **Step 2: Verify the handler test fails**

Run: `go test -tags=unit ./internal/handler/admin -run TestOAuthHandlerCookieAuthUsesFullScope -count=1`

Expected: FAIL to compile because `OAuthHandler.CookieAuth` is absent.

- [ ] **Step 3: Restore the minimal handler and route**

Add `CookieAuth` using the existing `CookieAuthRequest` and:

```go
tokenInfo, err := h.oauthService.CookieAuth(c.Request.Context(), &service.CookieAuthInput{
    SessionKey: req.SessionKey,
    ProxyID: req.ProxyID,
    Scope: "full",
})
```

Register `accounts.POST("/cookie-auth", h.Admin.OAuth.CookieAuth)` beside the Setup Token route.

- [ ] **Step 4: Verify the handler test passes**

Run: `go test -tags=unit ./internal/handler/admin -run TestOAuthHandlerCookieAuthUsesFullScope -count=1`

Expected: PASS.

### Task 3: Preserve imported usage windows

**Files:**
- Modify: `frontend/src/components/account/__tests__/claudeCredentialsImport.spec.ts`
- Modify: `frontend/src/components/account/claudeCredentialsImport.ts`

- [ ] **Step 1: Write failing parser tests**

Add a file fixture with:

```ts
usage: {
  session_used_percent: 25,
  session_resets_at: '2026-07-19T12:00:00Z',
  weekly_used_percent: 10,
  weekly_resets_at: '2026-07-26T12:00:00Z'
}
```

Require `passive_usage_7d_utilization=0.1`, a Unix-second 7d reset, and `passive_usage_sampled_at`. Explicitly require that the 5h utilization is not imported without the dedicated session-window end column. Add a second test proving malformed optional values are ignored.

- [ ] **Step 2: Verify parser tests fail**

Run: `pnpm vitest run src/components/account/__tests__/claudeCredentialsImport.spec.ts`

Expected: FAIL because the parser currently ignores `usage`.

- [ ] **Step 3: Implement optional usage normalization**

Change `extra` to `Record<string, unknown>`, parse the finite weekly percentage in the 0..100 range into a ratio, parse the weekly reset timestamp with `Date.parse`, write only valid passive 7d fields, and set `passive_usage_sampled_at` when at least one 7d field is accepted. Do not import the 5h utilization without a safe session-window end writer.

- [ ] **Step 4: Verify parser tests pass**

Run: `pnpm vitest run src/components/account/__tests__/claudeCredentialsImport.spec.ts`

Expected: PASS.

### Task 4: Regression verification and release

**Files:**
- Modify: `backend/cmd/server/VERSION`
- Create: `RELEASE_NOTES_v1.7.6.md`

- [ ] **Step 1: Run focused frontend tests**

Run: `pnpm vitest run src/components/account/__tests__/OAuthAuthorizationFlow.spec.ts src/components/account/__tests__/claudeCredentialsImport.spec.ts`

Expected: all tests PASS.

- [ ] **Step 2: Run frontend static verification**

Run: `pnpm typecheck`

Run: `pnpm build`

Expected: both exit 0.

- [ ] **Step 3: Run backend verification**

Run: `go test -tags=unit ./internal/handler/admin ./internal/service -count=1`

Expected: all tests PASS.

- [ ] **Step 4: Perform rendered UI verification**

Open the Claude OAuth account creation flow and verify manual authorization, sessionKey authorization, and credentials import are simultaneously visible; verify no framework overlay or relevant console error.

- [ ] **Step 5: Prepare v1.7.6**

Set `backend/cmd/server/VERSION` to `1.7.6` and document the compatibility restoration and usage snapshot fix in `RELEASE_NOTES_v1.7.6.md`.

- [ ] **Step 6: Commit, tag, and push after fresh verification**

Commit the tested changes, create annotated tag `v1.7.6`, and push `master` plus the tag to `origin`.
