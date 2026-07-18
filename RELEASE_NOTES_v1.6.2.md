# Sub2API v1.6.2

## Claude Web Session gateway fix

- Fixed Claude Web region-block responses being collapsed into the generic `Upstream request failed` 502 error.
- Region failures now report that the current outbound region is unavailable and instruct administrators to bind or replace the account proxy.
- Claude Web SSE errors are classified as session expiry, browser verification, region blocking, rate limiting, or upstream failure without exposing upstream response bodies or Cookie data.
- Streaming responses are no longer committed before the first valid upstream event, allowing the account pool to fail over when Claude Web rejects a request before producing content.
- Account connection tests now surface the same classified Web Session errors as normal gateway requests.

## Root cause confirmed

- The supplied Claude Web Cookie completed profile, organization, conversation, Haiku 4.5, and Opus 4.8 requests when routed through the supplied account proxy.
- The same Cookie without the proxy was redirected to Anthropic's `app-unavailable-in-region` page. Previous versions converted that redirect into an unhelpful generic 502.
