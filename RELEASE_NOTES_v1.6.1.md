# Sub2API v1.6.1

## Release hotfix

- Fixed the missing `log/slog` import that prevented the v1.6.0 backend, GoReleaser artifacts, and container images from compiling.
- Reissued the Claude Web account safety release as v1.6.1 so the admin updater can discover a complete GitHub Release with downloadable artifacts.
- Renewed the two existing `xlsx` audit exceptions through 2026-08-18. The mitigations remain limited to administrator-only, dynamically loaded export functionality.

## Included Claude Web improvements

- Authenticated profile synchronization for account email and Pro/Max subscription tier.
- Conservative tier presets for RPM, TPM, active sessions, and five-hour local cost windows.
- Total, daily, and weekly local quota enforcement for Web Session accounts.
- Unified Cookie create, JSON import, and account edit behavior.
- Shared create/edit safety controls with synchronized email display.
