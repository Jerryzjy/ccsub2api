# Sub2API v1.5.2

Claude Web session account compatibility hotfix.

## Fixed

- Send the supported `en-US` locale in Claude Web completion requests. Claude
  Web now rejects the previous `zh-CN` value with HTTP 400, which surfaced in
  the account test dialog as `Claude Web upstream request failed`.
- Detect Anthropic's `app-unavailable-in-region` redirect and report a clear
  account proxy-region error instead of the generic upstream failure.
- Mark region-blocked Claude Web accounts as unschedulable so the account pool
  does not repeatedly select an account whose configured egress cannot reach
  Claude Web.

## Validation

- Verified the full Claude Web flow with an imported browser Cookie and the
  account's SOCKS5 proxy: organization resolution, conversation creation,
  streamed completion, and conversation deletion all succeeded.
- Cookie and proxy source files remain ignored and are not included in this
  release.

## Upgrade Notes

- No database migration is required.
- Existing Claude Web accounts do not need to be re-imported for this fix.
- Configure a supported-region proxy on each Claude Web account when the
  Sub2API server's own egress is redirected to Anthropic's unavailable-region
  page.
