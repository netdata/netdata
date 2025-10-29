# TL;DR
- Updated auto-streaming rules: Anthropic now enables streaming only when reasoning is active, no stream override is present, and `maxOutputTokens >= 21,333`.
- Need diagnostics and a root-cause fix for the empty streaming responses that still occur under those conditions.

# Analysis
- Reproduced log snippet shows `LLM response failed [MODEL_ERROR]: No output generated` whenever `max_output_tokens` crosses 21,333 without reasoning metadata.
- Code path: `AIAgentSession` asks `llmClient.shouldAutoEnableReasoningStream`; Anthropic override now checks reasoning + threshold + absence of explicit streaming.
- Once streaming is forced, `BaseLLMProvider.executeStreamingTurn` may receive no `text-delta` events, resulting in an empty `response` string and eventual failure.

# Decisions
1. Auto-streaming only fires when all three prerequisites (reasoning, ≥21 333 tokens, no explicit stream) are satisfied.
2. Next steps focus on instrumentation and root-cause analysis rather than mitigation.

# Plan
1. Instrument streaming requests to log Anthropic error payloads when the response body is empty.
2. Reproduce large-token streaming failures to gather the new diagnostics.
3. Fix the underlying issue (likely in our stream reader or SDK usage) and add regression coverage.

# Implied Decisions
- Existing anthropic requests that explicitly set `stream=true` must retain streaming even without reasoning.
- Any change must avoid regressing reasoning scenarios where streaming is required (cache streaming handshake).

# Testing Requirements
- `npm run lint`
- `npm run build`
- Phase1 harness scenario covering large-token Anthropic call (to be added)
- Manual verification with real Anthropic call across 20k+ max tokens

# Documentation Updates
- Update docs/LOGS.md (or relevant release notes) once behaviour is confirmed.
