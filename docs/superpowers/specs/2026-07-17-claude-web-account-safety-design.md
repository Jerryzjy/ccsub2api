# Claude Web Account Safety and Metadata Design

## Goal

Make Anthropic `web_session` accounts first-class managed subscription accounts:
new, imported, and edited accounts share one contract; authenticated profile
metadata is synchronized; and configured quota, RPM, TPM, session, and 5-hour
window guards are enforced by the scheduler and billing pipeline.

## Scope

This change covers Claude Web session accounts only. It reuses existing account
guardrails and storage keys instead of introducing a second limiter system.
OAuth and Setup Token behavior remains unchanged except where shared UI or
helpers are extracted without changing their persisted values.

## Current Gaps

- The create form sends only Claude Web credentials. Its quota-control section
  is restricted to Anthropic OAuth/Setup Token accounts.
- The edit form has the same restriction and therefore cannot load or save
  Claude Web tier, window, session, RPM, or TPM settings.
- Total/daily/weekly local account quota is only enforced for API Key and
  Bedrock accounts.
- RPM and session/window scheduling gates explicitly exclude `web_session`.
- Imported Cookie files do not contain an email address. The authenticated
  Claude profile must be queried after account creation.
- New, import, and edit paths currently serialize account safety settings in
  different places.

## Upstream Profile Contract

The existing Claude Web browser transport will add a read-only `GET
/api/account` operation. A live schema check with the supplied Cookie and proxy
returned HTTP 200 and confirmed these fields:

- `email_address`
- `memberships[]`
- `memberships[].seat_tier`
- `memberships[].organization.billing_type`
- `memberships[].organization.rate_limit_tier`

No Cookie, session key, proxy credential, organization UUID, or raw profile
body may be logged. Profile decoding is bounded to 1 MiB.

The normalized profile contains:

```text
EmailAddress
Tier
TierSource
SubscriptionActive
```

Tier normalization accepts known Pro, Max 5x, and Max 20x enum variants. If
the account is a paid subscription but the upstream response does not
distinguish the tier, it falls back to `Pro`. Unknown or free membership data
does not upgrade the tier.

## Persisted Metadata

The synchronization step writes only normalized values:

- `credentials.email_address`: authenticated profile email.
- `extra.claude_tier`: `Pro`, `Max_5x`, or `Max_20x`.
- `extra.claude_tier_source`: `manual`, `profile`, `inferred`, or
  `profile_default`.
- `extra.claude_safety_preset_version`: current preset version when defaults
  were applied.

An existing non-empty email is replaced only by a successful authenticated
profile response. A tier whose source is `manual` is never overwritten by
profile or inferred data. Existing non-empty limiter fields are never
overwritten during metadata synchronization.

## Safety Presets

Preset version 1 supplies conservative defaults:

| Tier | Base RPM | Base TPM | Max sessions | 5h cost limit | Sticky reserve |
| --- | ---: | ---: | ---: | ---: | ---: |
| Pro | 6 | 80,000 | 2 | $8 | $1 |
| Max 5x | 12 | 160,000 | 4 | $40 | $5 |
| Max 20x | 20 | 240,000 | 6 | $80 | $10 |

Preset defaults also use `tiered` RPM/TPM strategies and a five-minute session
idle timeout. They are applied only to missing fields when a Claude Web account
is created, imported, or first synchronized. Changing a tier manually in the
UI offers to apply that tier's preset, but only an explicit user action replaces
existing limiter values.

Total, daily, and weekly local cost quotas remain opt-in and have no automatic
dollar defaults. This avoids presenting API-equivalent cost as Anthropic's
actual subscription quota. Administrators can configure these values manually.

## Backend Capability Boundaries

Two explicit capabilities prevent unrelated Anthropic transport behavior from
leaking into Claude Web:

1. `SupportsSubscriptionSafetyLimits` includes Anthropic OAuth, Setup Token,
   and Web Session accounts. It controls 5-hour cost, downstream session count,
   RPM, and TPM scheduling gates.
2. `SupportsLocalQuotaControl` includes API Key, Bedrock, and Web Session
   accounts. It controls total/daily/weekly quota schedulability, post-usage
   quota increments, reset, and notifications.

The existing `IsAnthropicOAuthOrSetupToken` helper remains unchanged for
transport-specific behavior such as Claude Code TLS fingerprinting, cache TTL
injection, session ID masking, and custom relay handling.

## Unified Write Contract

Backend normalization is authoritative and runs for:

- single account creation;
- JSON data import;
- account updates containing Claude Web credentials or safety metadata.

It validates supported tier names and positive finite limiter values, applies
missing preset fields, preserves usage counters, and never accepts client-side
usage counters as authoritative input.

The Vue application uses one Claude subscription safety component and one
serializer for both create and edit forms. The component exposes tier, window
cost, session, RPM, TPM, and local total/daily/weekly quota controls. Importing
a Cookie into the create form uses the same serializer as manual Claude Web
creation.

## Metadata Synchronization Flow

After create, JSON import, or credential update, the existing asynchronous
Claude Web probe performs:

1. Resolve the account proxy.
2. Fetch `/api/account` and normalize email/subscription metadata.
3. Merge metadata and missing preset fields into credentials/extra.
4. Resolve the organization.
5. Create and delete a temporary conversation to validate the session.
6. Store organization ID and mark the account schedulable.

Manual account testing runs the same metadata synchronization before the test
conversation so existing accounts are repaired without re-importing them.
Profile decoding failure does not prevent a valid conversation probe, but an
authentication, Cloudflare, or region error follows the existing account error
and unschedulable behavior.

## Error Handling and Security

- Profile errors use the existing Claude Web response classifier.
- Unsupported or missing profile fields degrade to existing metadata or the
  conservative Pro preset; raw upstream bodies are not surfaced.
- Cookie and proxy values remain credentials and are excluded from logs,
  exported diagnostics, and tests.
- Limits fail open only when the backing RPM/TPM counter service is unavailable,
  matching existing behavior. Configured local cost quota remains enforced from
  persisted usage state.

## Testing

Backend tests cover:

- profile response normalization and tier aliases;
- authenticated email synchronization;
- manual tier precedence;
- preset application to missing fields without overwriting custom values;
- create/import/update normalization equivalence;
- Web Session RPM, TPM, session, window-cost, and local quota schedulability;
- quota increments for Web Session usage;
- error classification and profile failure fallback.

Frontend tests cover:

- the shared safety component rendering for create and edit;
- identical serialization of the same form state;
- tier preset application and custom-value preservation;
- loading existing Claude Web extra fields;
- Cookie creation payload containing the normalized safety metadata.

The final verification includes targeted Go and Vue tests plus one live,
credential-safe Claude Web probe using the ignored local Cookie and proxy files.
