# Reasoning / Thinking Blocks – Status Report

## Goal
Document how Vercel AI SDK (v5.0.76) handles Anthropic "reasoning" blocks so we stop losing the required `signature` metadata when replaying conversation history. This description remains the source-of-truth for understanding the upstream contract.

## Verified Facts (from upstream source)
- Outgoing Anthropic requests are assembled in `@ai-sdk/anthropic/dist/index.js` (`convertToAnthropicMessagesPrompt`). When it sees an assistant `reasoning` part it **expects** `providerOptions` to include either `signature` or `redactedData` (lines ~1414-1443). Without those keys, the SDK drops the block and records an "unsupported reasoning metadata" warning.
- The SDK only emits a valid `thinking` block when it can forward both the text and the signature: `anthropicContent.push({ type: "thinking", thinking: part.text, signature })`. Missing signature → no `thinking` entry → another assistant part becomes the first content item.
- Streaming responses capture reasoning metadata via `reasoning-delta` / `signature_delta` events (same file, lines ~2280-2590). The SDK merges the streamed signature into `providerMetadata.anthropic.signature` so consumers can persist it.
- Our adapter (`src/llm-providers/base.ts`) collects the signature into `reasoningSegments.push({ text, providerMetadata })`. The in-memory conversation therefore holds `{ providerMetadata: { anthropic: { signature } } }` for every valid reasoning segment.
- When the conversation is replayed, we rely on those segments to survive. If they are stripped, the SDK serializes the assistant message starting with `tool_use`, which triggers Anthropic's `messages.1.content.0.type` error.

## Current Implementation Status (2025-10-22)

### Persistence & Schema
- `ConversationMessage.reasoning` now mirrors the AI SDK `ReasoningOutput[]` type so provider metadata is preserved end-to-end. Legacy string entries were removed.
- `normalizeReasoningSegments` runs before every Anthropic turn. Segments lacking signatures/redacted data are dropped and the turn is flagged to run without reasoning (prevents malformed replays).
- Session snapshots and REST persistence inherit this richer structure; reasoning metadata is written verbatim and survives compression.

### Runtime Safeguards
- A turn is only marked “reasoning disabled” if reasoning was actually active (Level or budget was set). Otherwise, no warning is logged.
- Master-only thinking updates: sub-agent thoughts are still recorded in the op-tree but do not emit status logs or headend updates. This prevents mixed progress streams across agents.
- CLI/REST logs now include `reasoning=unset|disabled|minimal|low|medium|high` for every LLM request/response, making it obvious when reasoning is enabled and whether it was suppressed.

### Headend Coverage
- **OpenAI-compatible headend** keeps thinking deltas in the SSE stream; only the master agent’s reasoning is exposed to consumers.
- **Anthropic-compatible headend** now emits proper `thinking` blocks in SSE mode and includes aggregated reasoning alongside final text in non-streaming mode, satisfying Claude’s new protocol expectations.
- **CLI output** shows thinking streams (prefixed with `THK`) and respects the master-only guard. REST responses also provide a `reasoning` string when available.
- **Slack headend** includes the master’s latest thinking in the status block, but not the raw reasoning text yet (Block Kit formatting remains TODO).

### Sub-agent Inheritance
- Front-matter reasoning settings are no longer copied to sub-agents. Only CLI/global overrides (`defaultsForUndefined`) propagate. This stops master prompts such as `neda/web-research.ai` from forcing reasoning onto `neda/web-search.ai`.

## Trade-offs & Known Limitations
- We currently drop reasoning segments that arrive without signatures. This keeps the session healthy but sacrifices visibility into those thoughts. Anthropic’s extended-thinking contract requires replaying the exact thinking block (with its signature) on subsequent turns; altering or omitting it yields validation errors such as `messages.1.content.0.type`.citeturn0search0turn0search1turn0search6turn0search7
- Redacted thinking (`redactedData`) is preserved, yet we have not implemented decryption or special handling for that payload.
- Reasoning telemetry is limited to text; token budgets and cost attribution are not exposed in headend summaries.

## Latest Findings (2025-10-22)
- Anthropic’s Messages API requires that the final assistant message start with a `thinking` or `redacted_thinking` block whenever reasoning is enabled; tool-only turns are fine, but the previous signed thinking must remain intact or Claude rejects the replay.citeturn0search0turn0search1turn0search2turn0search6
- When we intentionally disable reasoning, we must also stop forwarding stored thinking blocks (e.g., set `providerOptions.anthropic.sendReasoning=false` and/or strip the reasoning array) or we re-trigger the same validation failure.citeturn0search0turn0search1
- Empty `reasoning` arrays emitted alongside tool calls should not trigger a session-wide disable; Anthropic notes that some turns may omit thinking as long as signed segments are preserved across turns.citeturn0search2turn0search6
- Signatures are cryptographic proofs; dropping them prevents Claude from authenticating our replayed thinking and leads to rejection.citeturn0search6turn0search7

## Immediate Action Items
1. **Refine suppression logic** – only mark `disableReasoningForTurn` when we discard a reasoning segment with invalid metadata; tolerate empty arrays from tool-only turns.
2. **Align provider options** – when disabling reasoning, set `providerOptions.anthropic.sendReasoning=false` (and strip reasoning parts) so Anthropic does not receive thinking blocks alongside a disabled flag.
3. **Regression coverage** – add a deterministic harness case that reproduces a “tool-only turn with valid signatures on prior turn” and asserts that the next request keeps reasoning enabled unless signatures were dropped.

We must update this document again after applying the fixes and validating real Anthropic sessions (CLI + headends). If future testing shows additional requirements (e.g., budget adjustments or Bedrock-specific behavior), append them here with dates and citations.

## Headend Snapshot
- **CLI / REST / API** – show thinking tags and include `reasoning=` in logs/responses.
- **OpenAI Chat Compatibility** – forwards thinking as SSE reasoning deltas; runs master-only.
- **Anthropic Chat Compatibility** – streams `thinking_delta` blocks to Claude clients and appends aggregated reasoning to the final message payload.
- **Slack** – Displays the master agent’s latest reasoning text in progress updates, but does not yet render the full reasoning stream as Block Kit.

## Open TODOs
- Audit every mutation path (`sanitizeTurnMessages`, persistence reload, retry) to ensure we never drop `providerMetadata` in reasoning segments.
- Add regression coverage that streams an Anthropic turn, persists it, reloads the session, and verifies the signature survives replay.
- Extend deterministic harness scenarios to cover redacted thinking, budget exhaustion, and final-report retries with reasoning enabled.
- Evaluate Block Kit-friendly rendering of reasoning streams in Slack headend.
- Continue monitoring AI SDK releases for schema changes to reasoning/metadata.

## MANDATORY RULES

1. The agent should set a reasoning effort on the requests if unset
2. Configuration option `reasoning = none` should unset reasoning (even if set in front matter)
3. Valid reasoning options are:
   - `none`: to unset reasoning
   - `minimal`: to set it minimal (or 1024 tokens)
   - `low`: low or 30% of max output tokens
   - `medium`: medium or 60% of max output tokens
   - `high`: high or 80% to max output tokens
4. Specific to `anthropic` provider:
   - Anthropic ALWAYS sends reasoning with signatures on the first LLM turn, so: reasoning MUST be turned off if the first request does not return signatures
   - Anthropic demands `stream` to be `true` when max output tokens is >= 21333 tokens, so: `stream` must be switched automatically to `true` when sending requests with max output tokens >= 21333 tokens to anthropic
