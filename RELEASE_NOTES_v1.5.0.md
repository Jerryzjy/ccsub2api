# Sub2API v1.5.0

This release adds native Claude Web session accounts to Sub2API. A Claude Web
account can now be imported and scheduled by the existing account pool without
running a separate Claude2api service.

## Highlights

- Add the Anthropic `web_session` account type.
- Import Netscape-format Cookie `.txt` exports, a Cookie header, or a sessionKey.
- Prefer the full Cookie automatically and use sessionKey as a fallback.
- Resolve the Claude organization and probe account availability after import.
- Route Claude Web accounts through the existing proxy, concurrency, priority,
  group, sticky-session, and failover infrastructure.
- Translate Claude Web streaming responses to Anthropic Messages API events.
- Classify expired sessions, Cloudflare challenges, rate limits, and upstream
  failures without exposing upstream response bodies.
- Add create and edit controls for rotating Claude Web Cookie/sessionKey data.
- Accept `web_session` accounts in the existing JSON account import workflow.

## Security

- Cookie and sessionKey values are treated as sensitive account credentials.
- Sensitive values are redacted from admin API responses and are never included
  in public error messages.
- Editing an account with empty credential fields preserves the stored secrets.

## Upgrade Notes

- No database migration is required.
- Existing OAuth, Setup Token, API Key, Bedrock, and Vertex accounts are
  unchanged.
- After upgrading, open **Accounts -> Create Account -> Anthropic -> Claude Web**
  to add a Web session account.
- The account data import format uses `platform: "anthropic"` and
  `type: "web_session"`, with `credentials.cookie` and/or
  `credentials.session_key`.

## Current Limitations

- Claude Web session forwarding currently supports text message content only.
- Image content and native Anthropic tool-use blocks are rejected explicitly.
- Claude Web endpoints are private upstream interfaces and may require future
  compatibility updates when the upstream implementation changes.
