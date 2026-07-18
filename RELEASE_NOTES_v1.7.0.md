# Sub2API v1.7.0

## Claude Web conversation reuse

- Added cross-request Claude Web conversation reuse for `web_session` accounts.
- Follow-up requests reuse the upstream conversation and send only the newly appended user turn instead of rebuilding the complete history.
- Conversation state is isolated by API key, group, account, and session hash in Redis; Cookie and `sessionKey` values are never stored in conversation state.
- Added distributed serialization for concurrent turns to prevent a single conversation from forking.
- Added strict history digest validation, assistant-response digest tracking, model checks, and automatic rebuilds for expired, diverged, missing, or invalid upstream conversations.
- Added 5-minute and 1-hour state TTL handling based on request cache-control settings.
- Preserved the existing one-request behavior as a fail-open fallback when Redis conversation state is unavailable.
- Added cumulative runtime metrics, including `eligible_reuse_rate`, to the admin realtime metrics endpoint. The 95% value is an operational SLO for eligible follow-ups, not fabricated Anthropic prompt-cache usage.

## Web Session usage and capacity controls

- Added 5-hour and 7-day usage-window rows for Claude Web Session accounts using locally enforced account-window and weekly quota data.
- Fixed the account capacity column so Web Session accounts display configured 5-hour cost/utilization, active sessions, RPM, TPM, and daily/weekly/total quota limits.
- Unified frontend visibility, admin DTO mapping, account-list aggregation, scheduler safety checks, and post-request RPM accounting through the shared subscription-safety capability.
- Kept OAuth and Setup Token usage and capacity behavior unchanged.

## Reliability and safety

- Conversation reuse remains account-sticky; switching accounts always starts a new upstream conversation.
- Failed or interrupted streams invalidate conversation state to prevent partially advanced conversations from being reused.
- Upstream 404/410 conversation failures are rebuilt once with the complete request history.
- Reused historical context is tracked as a local estimate and is not reported as official `cache_read_input_tokens` or `cache_creation_input_tokens`.

## Build verification

- Backend production build completed successfully with `go build ./cmd/server`.
- Frontend production build completed successfully with `npm run build`.
- Frontend type checking completed successfully with `npm run typecheck`.
- No live Claude account or upstream validation was performed for this release.
