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
- We currently drop reasoning segments that arrive without signatures. This keeps the session healthy but sacrifices visibility into those thoughts.
- Redacted thinking (`redactedData`) is preserved, yet we have not implemented decryption or special handling for that payload.
- Reasoning telemetry is limited to text; token budgets and cost attribution are not exposed in headend summaries.

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
