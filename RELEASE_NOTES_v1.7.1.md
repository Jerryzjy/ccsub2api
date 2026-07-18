# Sub2API v1.7.1

## Claude Web cache usage reporting

- Fixed `web_session` usage logs showing only input and output tokens even when the upstream Claude Web conversation was retained and reused.
- Newly retained Web conversation context is now recorded as cache creation, split into the existing 5-minute or 1-hour cache buckets according to the request TTL.
- Previously retained conversation context is now recorded as cache-read tokens on eligible follow-up requests.
- When Redis conversation state is unavailable, requests continue to use ordinary input-token accounting and do not claim a cache hit.
- Streaming `message_start`, non-streaming Anthropic responses, billing records, usage logs, totals, costs, and the existing Token Detail tooltip now receive the same cache usage fields.
- Avoided double counting: context recorded as cache creation is not also counted as ordinary input.

## Accuracy note

Claude Web does not expose the official Anthropic API Prompt Cache counters. These cache values are Sub2API's local token estimates for successfully retained and reused Claude Web conversation context. OAuth and Setup Token accounts continue to use the official upstream cache usage fields unchanged.

## Verification

- Added regression coverage for 5-minute cache creation, 1-hour cache creation, cache reads on follow-up turns, Redis-unavailable fallback behavior, streaming usage fields, and two-turn Web conversation reuse.
