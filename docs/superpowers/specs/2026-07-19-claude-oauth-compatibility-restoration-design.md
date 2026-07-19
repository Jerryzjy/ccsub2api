# Claude OAuth Compatibility Restoration Design

## Goal

Restore the pre-v1.7.4 Claude OAuth `sessionKey` authorization entry while keeping the new `credentials.json` import, and preserve the imported file's initial 5h/7d usage snapshot without changing any post-creation cache, refresh, scheduling, or gateway behavior.

## Confirmed regressions

1. v1.7.4 removed the OAuth `/cookie-auth` handler/route and hid the `sessionKey` option while removing the misleading v1.7.2/v1.7.3 Cookie-to-OAuth conversion feature. The underlying `OAuthService.CookieAuth` full-scope flow still exists.
2. `parseClaudeCredentialsJSON` imports OAuth tokens and account metadata but ignores the optional root `usage` object. Newly imported accounts therefore have no passive 7d snapshot even when the file contains `weekly_used_percent` and `weekly_resets_at`.

## User-visible behavior

Claude OAuth account creation offers three independent methods:

- Manual OAuth authorization
- `sessionKey` automatic authorization using the existing full-scope OAuth flow
- Import an already-generated `credentials.json`

Claude Setup Token creation continues to offer manual authorization and `sessionKey` authorization. The UI must describe `sessionKey` authorization as dependent on Anthropic accepting the supplied session; it must not claim that old Cookies can be converted.

When `credentials.json` contains a `usage` object, import maps the snapshot to the existing passive-cache fields:

- `session_used_percent` -> `session_window_utilization` (percent converted to a 0..1 ratio)
- `weekly_used_percent` -> `passive_usage_7d_utilization` (percent converted to a 0..1 ratio)
- `weekly_resets_at` -> `passive_usage_7d_reset` (Unix seconds)
- a valid usage snapshot sets `passive_usage_sampled_at`

Invalid optional usage fields are ignored without rejecting otherwise valid OAuth credentials. Token, scope, account, and subscription fields retain the current validation rules.

## Architecture and boundaries

The frontend parser remains responsible for translating the external file schema into the existing Sub2API account schema. No new account type, cache implementation, database column, scheduler path, or background job is introduced.

The backend restores only the thin `/cookie-auth` HTTP adapter that calls the already-existing `OAuthService.CookieAuth` with `Scope: "full"`. Setup Token continues to call the same service with `Scope: "inference"`.

After account creation, imported accounts use the same token refresher, active/passive usage endpoints, cache keys, rate-limit sampling, and scheduler code as every other Anthropic OAuth account. Imported usage values are only an initial snapshot and are overwritten by the existing active/passive synchronization flow.

## Error handling and security

- Never log or echo the supplied `sessionKey`, access token, or refresh token.
- Upstream rejection is returned through the existing OAuth error path.
- A malformed `credentials.json` OAuth section still blocks creation.
- A malformed optional usage snapshot does not block creation and is not persisted.

## Verification

- Frontend component test proves OAuth renders manual, `sessionKey`, and credentials import methods together.
- Parser tests prove the supplied usage schema maps to existing passive fields and malformed optional usage is ignored.
- Backend handler test proves `/cookie-auth` invokes full-scope OAuth and returns tokens without exposing the session key.
- Run focused frontend/backend tests, frontend typecheck/build, and the relevant backend package tests.
- Render the account creation flow and verify the three Claude OAuth methods visually with no console errors.
