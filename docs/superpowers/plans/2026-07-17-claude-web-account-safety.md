# Claude Web Account Safety Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make Claude Web session accounts automatically synchronize email/subscription metadata and enforce the same configurable safety limits across create, import, edit, scheduling, and billing.

**Architecture:** Add a bounded Claude profile client and a pure backend safety-policy normalizer, then make all account write paths invoke that contract. Extend only the scheduler/billing capability predicates needed by Web Session accounts, while keeping Claude Code transport-specific behavior unchanged. The Vue create and edit modals share a typed serializer and a reusable safety panel.

**Tech Stack:** Go 1.24, Gin, Testify, Vue 3 Composition API, TypeScript, Vitest, Vue Test Utils.

---

## File Structure

- Create `backend/internal/service/claude_web_profile.go`: profile DTO decoding and tier normalization.
- Create `backend/internal/service/claude_web_profile_test.go`: profile schema and alias tests.
- Create `backend/internal/service/claude_web_safety.go`: presets, validation, missing-only application, capability predicates.
- Create `backend/internal/service/claude_web_safety_test.go`: pure safety-policy tests.
- Modify `backend/internal/service/claude_web_client.go`: authenticated bounded `FetchProfile` request.
- Modify `backend/internal/service/account_test_service.go`: profile synchronization during asynchronous and manual probes.
- Modify `backend/internal/service/account.go`: explicit subscription-safety and local-quota capabilities.
- Modify `backend/internal/service/gateway_service.go`: Web Session quota, RPM, session, window, billing, and notification enforcement.
- Modify `backend/internal/service/ops_account_quota.go`: operational schedulability parity.
- Modify `backend/internal/handler/admin/account_handler.go`: create/update normalization.
- Modify `backend/internal/handler/admin/account_data.go`: import normalization.
- Create `frontend/src/components/account/claudeWebSafety.ts`: shared form state, preset application, hydration, and serialization.
- Create `frontend/src/components/account/ClaudeWebSafetyPanel.vue`: shared Web Session safety UI.
- Create `frontend/src/components/account/__tests__/claudeWebSafety.spec.ts`: serializer/preset tests.
- Create `frontend/src/components/account/__tests__/ClaudeWebSafetyPanel.spec.ts`: component behavior tests.
- Modify `frontend/src/components/account/CreateAccountModal.vue`: shared panel and payload integration.
- Modify `frontend/src/components/account/EditAccountModal.vue`: shared panel, hydration, and payload integration.
- Modify `frontend/src/components/account/__tests__/EditAccountModal.spec.ts`: edit parity regression tests.
- Create `frontend/src/components/account/__tests__/CreateAccountModal.claudeWebSafety.spec.ts`: create parity regression tests.
- Modify `frontend/src/i18n/locales/en.ts` and `frontend/src/i18n/locales/zh.ts`: profile source, presets, and local quota copy.

### Task 1: Claude Web Profile Client

**Files:**
- Create: `backend/internal/service/claude_web_profile.go`
- Create: `backend/internal/service/claude_web_profile_test.go`
- Modify: `backend/internal/service/claude_web_client.go`

- [ ] **Step 1: Write failing profile normalization tests**

Add table tests that decode the live-compatible schema and verify no raw identifiers are required:

```go
func TestNormalizeClaudeWebProfile(t *testing.T) {
	profile := claudeWebAccountProfile{
		EmailAddress: "user@example.com",
		Memberships: []claudeWebMembership{{
			SeatTier: "max_5x",
			Organization: claudeWebOrganizationProfile{
				BillingType:  "stripe_subscription",
				RateLimitTier: "default_claude_ai",
			},
		}},
	}
	got := normalizeClaudeWebProfile(profile)
	require.Equal(t, "user@example.com", got.EmailAddress)
	require.Equal(t, ClaudeTierMax5x, got.Tier)
	require.Equal(t, ClaudeTierSourceProfile, got.TierSource)
	require.True(t, got.SubscriptionActive)
}

func TestNormalizeClaudeWebProfile_AmbiguousPaidSubscriptionDefaultsToPro(t *testing.T) {
	profile := claudeWebAccountProfile{Memberships: []claudeWebMembership{{
		Organization: claudeWebOrganizationProfile{BillingType: "stripe_subscription"},
	}}}
	got := normalizeClaudeWebProfile(profile)
	require.Equal(t, ClaudeTierPro, got.Tier)
	require.Equal(t, ClaudeTierSourceProfileDefault, got.TierSource)
}
```

- [ ] **Step 2: Run the tests and verify RED**

Run:

```powershell
cd backend
go test -count=1 ./internal/service -run 'TestNormalizeClaudeWebProfile'
```

Expected: FAIL because the profile types and normalizer do not exist.

- [ ] **Step 3: Implement bounded profile DTO normalization**

Define normalized constants and types:

```go
const (
	ClaudeTierPro    = "Pro"
	ClaudeTierMax5x  = "Max_5x"
	ClaudeTierMax20x = "Max_20x"

	ClaudeTierSourceProfile        = "profile"
	ClaudeTierSourceProfileDefault = "profile_default"
)

type ClaudeWebProfile struct {
	EmailAddress      string
	Tier              string
	TierSource        string
	SubscriptionActive bool
}
```

Normalize case-insensitive aliases including `pro`, `claude_pro`, `max_5x`,
`max5x`, `max_20x`, and `max20x`. Treat a non-empty paid billing type such as
`stripe_subscription` with no distinct tier as conservative Pro.

- [ ] **Step 4: Add and test `FetchProfile`**

Implement on `ClaudeWebClient`:

```go
func (c *ClaudeWebClient) FetchProfile(ctx context.Context, account *Account, proxyURL string) (ClaudeWebProfile, error) {
	req, err := c.newRequest(ctx, account, http.MethodGet, "/api/account", nil, "/new")
	if err != nil { return ClaudeWebProfile{}, err }
	resp, err := c.transport.Do(ctx, req, proxyURL, account.ID)
	if err != nil { return ClaudeWebProfile{}, fmt.Errorf("fetch Claude Web profile: %w", err) }
	defer resp.Body.Close()
	if err := claudeWebRequireStatus(resp, http.StatusOK); err != nil { return ClaudeWebProfile{}, err }
	var raw claudeWebAccountProfile
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&raw); err != nil {
		return ClaudeWebProfile{}, errors.New("decode Claude Web profile response")
	}
	return normalizeClaudeWebProfile(raw), nil
}
```

Add a transport-stub test asserting `GET /api/account`, proxy forwarding, email,
and tier. Verify response bodies and credentials never appear in errors.

- [ ] **Step 5: Run focused tests and commit**

```powershell
cd backend
go test -count=1 ./internal/service -run 'ClaudeWebProfile|FetchProfile'
git add internal/service/claude_web_client.go internal/service/claude_web_profile.go internal/service/claude_web_profile_test.go
git commit -m "feat: read Claude Web account profiles"
```

### Task 2: Authoritative Safety Presets and Validation

**Files:**
- Create: `backend/internal/service/claude_web_safety.go`
- Create: `backend/internal/service/claude_web_safety_test.go`

- [ ] **Step 1: Write failing preset tests**

Cover all three presets and missing-only behavior:

```go
func TestApplyClaudeWebSafetyPreset_MissingOnly(t *testing.T) {
	extra := map[string]any{"claude_tier": ClaudeTierPro, "base_rpm": 3}
	got, err := NormalizeClaudeWebSafetyExtra(extra, true)
	require.NoError(t, err)
	require.Equal(t, 3, got["base_rpm"])
	require.Equal(t, 80000, got["base_tpm"])
	require.Equal(t, 2, got["max_sessions"])
	require.Equal(t, 8.0, got["window_cost_limit"])
	require.Equal(t, ClaudeWebSafetyPresetVersion, got["claude_safety_preset_version"])
}

func TestNormalizeClaudeWebSafetyExtra_RejectsInvalidTier(t *testing.T) {
	_, err := NormalizeClaudeWebSafetyExtra(map[string]any{"claude_tier": "Enterprise"}, true)
	require.ErrorContains(t, err, "invalid Claude tier")
}
```

- [ ] **Step 2: Run tests and verify RED**

```powershell
cd backend
go test -count=1 ./internal/service -run 'ClaudeWebSafety|NormalizeClaudeWebSafety'
```

Expected: FAIL because the normalizer does not exist.

- [ ] **Step 3: Implement pure preset policy**

Use one immutable preset table:

```go
type ClaudeWebSafetyPreset struct {
	BaseRPM, BaseTPM, MaxSessions int
	WindowCostLimit, WindowCostStickyReserve float64
}

var claudeWebSafetyPresets = map[string]ClaudeWebSafetyPreset{
	ClaudeTierPro:    {BaseRPM: 6, BaseTPM: 80000, MaxSessions: 2, WindowCostLimit: 8, WindowCostStickyReserve: 1},
	ClaudeTierMax5x:  {BaseRPM: 12, BaseTPM: 160000, MaxSessions: 4, WindowCostLimit: 40, WindowCostStickyReserve: 5},
	ClaudeTierMax20x: {BaseRPM: 20, BaseTPM: 240000, MaxSessions: 6, WindowCostLimit: 80, WindowCostStickyReserve: 10},
}
```

The normalizer clones the input, validates finite positive limiter values,
normalizes the tier/source enums, and applies only missing preset fields. It
must never copy `quota_used`, `quota_daily_used`, `quota_weekly_used`, or their
period starts from untrusted create/import input.

- [ ] **Step 4: Verify GREEN and commit**

```powershell
cd backend
go test -count=1 ./internal/service -run 'ClaudeWebSafety|NormalizeClaudeWebSafety'
git add internal/service/claude_web_safety.go internal/service/claude_web_safety_test.go
git commit -m "feat: define Claude Web safety presets"
```

### Task 3: Normalize Create, Import, and Update Equally

**Files:**
- Modify: `backend/internal/handler/admin/account_handler.go`
- Modify: `backend/internal/handler/admin/account_data.go`
- Modify: `backend/internal/handler/admin/account_data_web_session_test.go`
- Create: `backend/internal/handler/admin/account_web_session_safety_test.go`

- [ ] **Step 1: Write failing handler parity tests**

Add table-driven tests that pass the same Web Session credentials and Pro tier
through create-request normalization, data-import normalization, and update
normalization. Assert all paths produce identical `base_rpm`, `base_tpm`,
`max_sessions`, `window_cost_limit`, strategies, and preset version.

Also assert:

```go
require.NotContains(t, normalized, "quota_used")
require.NotContains(t, normalized, "quota_daily_used")
require.Equal(t, "manual", normalized["claude_tier_source"])
```

- [ ] **Step 2: Run tests and verify RED**

```powershell
cd backend
go test -count=1 ./internal/handler/admin -run 'WebSessionSafety|DataAccount_ClaudeWebSession'
```

Expected: FAIL because Web Session extras are not normalized.

- [ ] **Step 3: Add one handler helper and call it from all paths**

Create an unexported helper in `account_handler.go`:

```go
func normalizeClaudeWebAccountExtra(platform, accountType string, extra map[string]any, applyDefaults bool) (map[string]any, error) {
	if platform != service.PlatformAnthropic || accountType != service.AccountTypeWebSession {
		return extra, nil
	}
	return service.NormalizeClaudeWebSafetyExtra(extra, applyDefaults)
}
```

Call it before `CreateAccount`, before `UpdateAccount` when the stored or
requested type is Web Session, and inside `importData` after credential
validation. Create/import use `applyDefaults=true`; update applies defaults only
when the tier is newly set or preset metadata is absent, and otherwise validates
without overwriting existing fields.

- [ ] **Step 4: Verify GREEN and commit**

```powershell
cd backend
go test -count=1 ./internal/handler/admin -run 'WebSessionSafety|DataAccount_ClaudeWebSession'
git add internal/handler/admin/account_handler.go internal/handler/admin/account_data.go internal/handler/admin/account_data_web_session_test.go internal/handler/admin/account_web_session_safety_test.go
git commit -m "feat: normalize Claude Web safety settings"
```

### Task 4: Synchronize Email and Tier During Probes

**Files:**
- Modify: `backend/internal/service/account_test_service.go`
- Modify: `backend/internal/service/gateway_claude_web_test.go`

- [ ] **Step 1: Write failing synchronization tests**

Use the existing transport stub with `/api/account`, create, and delete
responses. Assert the repository receives merged credentials and extra:

```go
require.Equal(t, "user@example.com", updatedCredentials[ClaudeWebCredentialEmailAddress])
require.Equal(t, ClaudeTierPro, updatedExtra["claude_tier"])
require.Equal(t, ClaudeTierSourceProfileDefault, updatedExtra["claude_tier_source"])
require.Equal(t, 6, updatedExtra["base_rpm"])
```

Add a second test where `claude_tier_source=manual` and custom `base_rpm=2`;
profile synchronization must preserve both.

- [ ] **Step 2: Run tests and verify RED**

```powershell
cd backend
go test -count=1 ./internal/service -run 'ProbeClaudeWebSession.*Profile|ClaudeWebProfileSync'
```

- [ ] **Step 3: Implement merge and probe ordering**

Add `ClaudeWebCredentialEmailAddress = "email_address"`. Fetch the profile
before organization resolution. Merge successful email into cloned credentials.
Merge tier only when the existing source is not manual, then run missing-only
preset normalization. Persist credentials with `UpdateCredentials` and extra
with `UpdateExtra` before clearing the account error.

Profile decoding errors are logged only as stage and error class and do not
stop conversation validation. HTTP authentication, Cloudflare, or region errors
still fail the probe through the existing classifier.

- [ ] **Step 4: Make manual test invoke the same sync**

Refactor the profile merge into one method used by both
`ProbeClaudeWebSession` and `testClaudeAccountConnection`. The manual test must
repair existing account metadata without exposing the profile body in SSE.

- [ ] **Step 5: Verify GREEN and commit**

```powershell
cd backend
go test -count=1 ./internal/service -run 'ClaudeWeb|ProbeClaudeWebSession'
git add internal/service/account_test_service.go internal/service/claude_web_credentials.go internal/service/gateway_claude_web_test.go
git commit -m "feat: sync Claude Web account metadata"
```

### Task 5: Enforce Web Session Safety Limits

**Files:**
- Modify: `backend/internal/service/account.go`
- Modify: `backend/internal/service/gateway_service.go`
- Modify: `backend/internal/service/ops_account_quota.go`
- Modify: `backend/internal/service/account_quota_schedulable_test.go`
- Create: `backend/internal/service/claude_web_scheduling_limits_test.go`
- Create: `backend/internal/service/claude_web_quota_billing_test.go`

- [ ] **Step 1: Write failing capability and quota tests**

Add assertions:

```go
web := &Account{Platform: PlatformAnthropic, Type: AccountTypeWebSession}
require.True(t, web.SupportsSubscriptionSafetyLimits())
require.True(t, web.SupportsLocalQuotaControl())
require.False(t, web.IsAnthropicOAuthOrSetupToken())
```

Extend `TestAccountIsSchedulable_QuotaExceeded` with a Web Session whose daily
quota is exhausted and expect false. Add billing tests asserting a Web Session
with a configured quota triggers exactly one `IncrementQuotaUsed` call.

- [ ] **Step 2: Run tests and verify RED**

```powershell
cd backend
go test -count=1 -tags=unit ./internal/service -run 'WebSession.*Quota|SupportsSubscriptionSafety|AccountIsSchedulable_QuotaExceeded'
```

- [ ] **Step 3: Implement explicit capability helpers**

```go
func (a *Account) SupportsSubscriptionSafetyLimits() bool {
	return a != nil && a.Platform == PlatformAnthropic &&
		(a.Type == AccountTypeOAuth || a.Type == AccountTypeSetupToken || a.Type == AccountTypeWebSession)
}

func (a *Account) SupportsLocalQuotaControl() bool {
	return a != nil && (a.Type == AccountTypeAPIKey || a.Type == AccountTypeBedrock || a.IsClaudeWebSession())
}
```

Keep `IsAPIKeyOrBedrock` and `IsAnthropicOAuthOrSetupToken` unchanged for pool
mode and transport-specific behavior.

- [ ] **Step 4: Replace only safety/quota gates**

Use `SupportsSubscriptionSafetyLimits` in window-cost, RPM, downstream session,
and batch counter selection. TPM already applies to any account with
`base_tpm`; retain that behavior.

Use `SupportsLocalQuotaControl` in:

- `Account.IsSchedulable`;
- `GatewayService.isAccountSchedulableForQuota`;
- unified and legacy post-usage account quota guards;
- account quota notification guard;
- Ops schedulability.

Do not replace helpers used by TLS fingerprint, cache injection, session ID
masking, custom relay, or API Key pool mode.

- [ ] **Step 5: Verify GREEN and commit**

```powershell
cd backend
go test -count=1 -tags=unit ./internal/service -run 'Quota|RPM|TPM|Session|WindowCost|SupportsSubscriptionSafety'
git add internal/service/account.go internal/service/gateway_service.go internal/service/ops_account_quota.go internal/service/account_quota_schedulable_test.go internal/service/claude_web_scheduling_limits_test.go internal/service/claude_web_quota_billing_test.go
git commit -m "feat: enforce Claude Web account safety limits"
```

### Task 6: Shared Frontend Safety State and Panel

**Files:**
- Create: `frontend/src/components/account/claudeWebSafety.ts`
- Create: `frontend/src/components/account/ClaudeWebSafetyPanel.vue`
- Create: `frontend/src/components/account/__tests__/claudeWebSafety.spec.ts`
- Create: `frontend/src/components/account/__tests__/ClaudeWebSafetyPanel.spec.ts`

- [ ] **Step 1: Write failing serializer and preset tests**

Define the wished-for API in tests:

```ts
const state = createClaudeWebSafetyState()
applyClaudeWebPreset(state, 'Pro', { overwrite: false })
expect(serializeClaudeWebSafety(state)).toMatchObject({
  claude_tier: 'Pro',
  claude_tier_source: 'manual',
  base_rpm: 6,
  base_tpm: 80000,
  max_sessions: 2,
  window_cost_limit: 8,
  window_cost_sticky_reserve: 1
})
```

Hydrate custom `base_rpm=2`, apply a missing-only preset, and assert 2 is
preserved. Test quota reset fields and deletion of disabled optional values.

- [ ] **Step 2: Run tests and verify RED**

```powershell
cd frontend
pnpm test:run src/components/account/__tests__/claudeWebSafety.spec.ts
```

- [ ] **Step 3: Implement typed shared state**

Export:

```ts
export type ClaudeTier = '' | 'Pro' | 'Max_5x' | 'Max_20x'
export interface ClaudeWebSafetyState {
  tier: ClaudeTier
  tierSource: '' | 'manual' | 'profile' | 'inferred' | 'profile_default'
  windowCostEnabled: boolean
  windowCostLimit: number | null
  windowCostStickyReserve: number | null
  sessionLimitEnabled: boolean
  maxSessions: number | null
  sessionIdleTimeoutMinutes: number | null
  rpmLimitEnabled: boolean
  baseRPM: number | null
  rpmStrategy: 'tiered' | 'sticky_exempt'
  tpmLimitEnabled: boolean
  baseTPM: number | null
  tpmStrategy: 'tiered' | 'sticky_exempt'
  quotaLimit: number | null
  quotaDailyLimit: number | null
  quotaWeeklyLimit: number | null
}
```

Keep preset numbers identical to the backend. Serialization returns only
persistent configuration keys and never returns usage counters.

- [ ] **Step 4: Implement and test the shared panel**

The panel uses `v-model` for the whole state, renders tier/preset actions,
window cost, sessions, RPM, TPM, and `QuotaLimitCard`, and emits a cloned state
on every change. Add stable `data-testid` attributes for tier, apply-preset,
RPM, TPM, session, and quota controls.

- [ ] **Step 5: Verify GREEN and commit**

```powershell
cd frontend
pnpm test:run src/components/account/__tests__/claudeWebSafety.spec.ts src/components/account/__tests__/ClaudeWebSafetyPanel.spec.ts
git add src/components/account/claudeWebSafety.ts src/components/account/ClaudeWebSafetyPanel.vue src/components/account/__tests__/claudeWebSafety.spec.ts src/components/account/__tests__/ClaudeWebSafetyPanel.spec.ts
git commit -m "feat: add shared Claude Web safety controls"
```

### Task 7: Create/Edit UI Parity and Email Display

**Files:**
- Modify: `frontend/src/components/account/CreateAccountModal.vue`
- Modify: `frontend/src/components/account/EditAccountModal.vue`
- Modify: `frontend/src/components/account/__tests__/EditAccountModal.spec.ts`
- Create: `frontend/src/components/account/__tests__/CreateAccountModal.claudeWebSafety.spec.ts`
- Modify: `frontend/src/i18n/locales/en.ts`
- Modify: `frontend/src/i18n/locales/zh.ts`

- [ ] **Step 1: Write failing modal parity tests**

Create a Claude Web account fixture with profile email and custom safety extra.
Assert edit hydration renders those values and update payload preserves them.
Mount create, select Web Session, choose Pro, and assert create payload extra is
identical to `serializeClaudeWebSafety` for the same state.

Add an account-list regression assertion that `credentials.email_address` is
displayed using the existing email fallback expression.

- [ ] **Step 2: Run tests and verify RED**

```powershell
cd frontend
pnpm test:run src/components/account/__tests__/EditAccountModal.spec.ts src/components/account/__tests__/CreateAccountModal.claudeWebSafety.spec.ts
```

- [ ] **Step 3: Integrate create path**

Add one `claudeWebSafety` ref initialized by `createClaudeWebSafetyState()`. Show
`ClaudeWebSafetyPanel` only for Anthropic Web Session. Pass
`serializeClaudeWebSafety(claudeWebSafety.value)` as `extra` to
`createAccountAndFinish('anthropic', 'web_session', credentials, extra)`.

Reset the state when closing or switching away from Web Session.

- [ ] **Step 4: Integrate edit path**

Hydrate the same state from `account.extra`, render the same panel, and merge
serialized configuration into a clone of existing extra while preserving
server-managed usage counters. Remove the Web Session exclusion from tier and
local quota editing. Empty credential replacement fields must continue to
preserve redacted Cookie/session credentials.

- [ ] **Step 5: Add bilingual copy**

Add labels explaining:

- profile email is synchronized after connection testing;
- ambiguous paid subscriptions use the conservative Pro preset;
- preset application fills missing values;
- local dollar quota is an API-equivalent guardrail, not Anthropic's displayed
  subscription quota;
- manual values are not overwritten by future profile synchronization.

- [ ] **Step 6: Verify GREEN and commit**

```powershell
cd frontend
pnpm test:run src/components/account/__tests__/EditAccountModal.spec.ts src/components/account/__tests__/CreateAccountModal.claudeWebSafety.spec.ts src/components/account/__tests__/claudeWebSafety.spec.ts src/components/account/__tests__/ClaudeWebSafetyPanel.spec.ts
pnpm typecheck
git add src/components/account/CreateAccountModal.vue src/components/account/EditAccountModal.vue src/components/account/__tests__/EditAccountModal.spec.ts src/components/account/__tests__/CreateAccountModal.claudeWebSafety.spec.ts src/i18n/locales/en.ts src/i18n/locales/zh.ts
git commit -m "feat: align Claude Web create and edit forms"
```

### Task 8: Regression, Live Verification, and Release Preparation

**Files:**
- Modify: `backend/cmd/server/VERSION`
- Create: `RELEASE_NOTES_v1.6.0.md`

- [ ] **Step 1: Run backend regression tests**

```powershell
cd backend
go test -count=1 ./internal/service ./internal/handler/admin -run 'ClaudeWeb|WebSession|Quota|RPM|TPM|WindowCost|Session'
```

Expected: PASS.

- [ ] **Step 2: Run frontend regression and build checks**

```powershell
cd frontend
pnpm test:run src/components/account/__tests__
pnpm typecheck
```

Expected: PASS with no Vue or TypeScript errors.

- [ ] **Step 3: Run one credential-safe live probe**

Use the ignored Cookie and proxy files through an environment-gated diagnostic
test. Log only stage/status/boolean metadata. Verify profile email presence,
tier source, organization resolution, conversation create/delete, and one
streamed completion. Remove the diagnostic file immediately afterward.

- [ ] **Step 4: Check credential hygiene**

```powershell
git check-ignore -v docs\claude-pro-cookie-1.txt docs\claude-pro-proxy.txt.txt
rg -n --hidden --glob '!docs/**' --glob '!.git/**' 'sk-ant-sid|CLAUDE_WEB_COOKIE_FILE|CLAUDE_WEB_PROXY_FILE' .
git diff --check
git status --short
```

Expected: local credential files remain ignored; repository matches only test
placeholders/UI examples; no temporary diagnostic file remains.

- [ ] **Step 5: Prepare release metadata**

Set `backend/cmd/server/VERSION` to `1.6.0`. Release notes must list profile
email synchronization, conservative tier presets, true Web Session quota/RPM/
TPM/session enforcement, and create/edit/import parity. State that the live
probe succeeded only if Step 3 passed.

- [ ] **Step 6: Final verification commit**

```powershell
git add backend/cmd/server/VERSION RELEASE_NOTES_v1.6.0.md
git commit -m "chore: prepare v1.6.0"
git status --short
```

Expected: clean worktree after the release commit.
