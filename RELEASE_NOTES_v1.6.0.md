# Sub2API v1.6.0

## Claude Web account management

- Added authenticated Claude Web profile synchronization through `/api/account`.
- Imported Cookie/sessionKey accounts now synchronize their account email after a successful connection probe.
- Claude subscription levels are normalized as Pro, Max 5x, or Max 20x. Ambiguous paid subscriptions conservatively use the Pro preset.
- Manual subscription-level selections are preserved during later profile synchronization.

## Account safety controls

- Added conservative per-tier defaults for RPM, TPM, active sessions, and the five-hour local cost window.
- Claude Web Session accounts now participate in RPM, TPM, session-count, window-cost, total-quota, daily-quota, and weekly-quota scheduling checks.
- Post-request API-equivalent costs now update configured Claude Web local quota counters and notifications.
- Local cost quotas remain optional and are explicitly separate from Anthropic's displayed subscription quota.

## Create, import, and edit parity

- Added one shared Claude Web safety panel and serializer for create and edit flows.
- Cookie creation, JSON import, and account updates use the same backend validation and missing-only preset policy.
- Runtime usage counters cannot be supplied by untrusted create/import payloads and are preserved during edits.
- Account lists and edit forms display the synchronized `credentials.email_address` value.

## Security

- Profile response decoding is bounded to 1 MiB.
- Cookie, sessionKey, proxy credentials, organization identifiers, and raw profile responses are not added to logs or public errors.

## Verification note

Automated and live credential tests were intentionally not run for this development pass at the operator's request. Static formatting and diff hygiene were checked.
