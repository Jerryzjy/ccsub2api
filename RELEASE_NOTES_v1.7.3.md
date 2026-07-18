# Sub2API v1.7.3

## Claude Cookie OAuth correction

- Fixed uploaded Netscape Cookie files being reduced to only `sessionKey` before Claude organization and OAuth authorization requests.
- The normalized full Claude Cookie header now remains attached to both requests; directly pasted raw `sessionKey` values keep the existing compatibility path.
- Added safe failure stages for session validation, OAuth authorization, request preparation, and token exchange.
- Fixed the account form hiding the backend error message behind the generic `Cookie authorization failed` text.

## Verification boundary

This release fixes Sub2API's Cookie transport and diagnostics. Claude may still reject an OAuth request because of upstream session age, recent-sign-in, region, proxy, or account security policy; this release does not claim to bypass those checks.
