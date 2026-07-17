# Sub2API v1.5.1

Hotfix for Claude Web session accounts introduced in v1.5.0.

## Fixed

- Parse Netscape `#HttpOnly_` cookie records as cookie data instead of comments.
  This prevents HttpOnly `sessionKey` values from being silently discarded.
- Validate imported Cookie content before creating an account. Imports now fail
  immediately when the Cookie is malformed or contains no usable sessionKey.
- Read `lastActiveOrg` directly from the imported Cookie before calling the
  organizations endpoint, matching the browser flow and avoiding an unnecessary
  reauthentication or Cloudflare-sensitive request.
- Translate Anthropic API model names such as
  `claude-sonnet-4-5-20250929` to Claude Web model IDs such as
  `claude-sonnet-5` before account tests and gateway forwarding.
- Apply account model mappings before resolving the Claude Web model ID.

## Upgrade Notes

- No database migration is required.
- Upgrade from v1.5.0 to v1.5.1 before using Claude Web session accounts.
- Accounts imported by v1.5.0 should be edited with the Cookie again or deleted
  and re-imported so the corrected parser and validation run on the source file.
- A Cloudflare clearance Cookie can still be bound to the browser egress IP and
  User-Agent. Assign the account a matching proxy when its browser Cookie was
  captured through a different network than the Sub2API server.
