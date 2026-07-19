# Sub2API v1.7.4

## Corrected Claude OAuth account import

- Removed the misleading Claude Cookie/sessionKey-to-OAuth account conversion introduced in v1.7.2 and v1.7.3.
- Removed the corresponding frontend Cookie file upload, automatic OAuth creation action, and backend conversion endpoints.
- Added direct import for an existing Claude `credentials.json` containing `claudeAiOauth` OAuth tokens.
- Normalizes the imported millisecond `expiresAt` value to the Unix-second format used by Sub2API and preserves OAuth scopes and account metadata.
- Cookie/sessionKey authorization remains available only for Setup Token accounts and is now labeled accordingly.

This release does not convert Claude Cookies or sessionKey values into OAuth credentials.
