TL;DR
- HubSpot headend session aborted with EXIT-INVALID-RESPONSES-EXHAUSTED even though only the first two LLM replies were invalid; later replies had tool calls.
- TurnRunner keeps the previous invalid_response flag across attempts, so a later valid attempt still inherits the stale error and the turn is marked as failed once maxRetries is hit.

Analysis
- Repro log (Dec 04 08:41 UTC): Turn 2 shows two “Empty response without tools; retrying…” warnings, then several tool calls (hubspot-search-objects) and LLM replies. The session ends with “Turn 2 exhausted 5 attempts with invalid responses; aborting session” even though only attempts 1–2 were actually empty.
- Code path: src/session-turn-runner.ts
  - `lastError` / `lastErrorType` are declared per turn (line ~271) and set to `invalid_response` for empty responses (lines 1136, 1163).
  - These variables are **not reset at the start of each attempt**; if an early attempt sets `lastErrorType` to `invalid_response`, later attempts that succeed do not overwrite it.
  - Later, when processing a successful-but-nonfinal attempt, we hit:
    `if (!finalReportReady && lastErrorType === 'invalid_response' && attempts < maxRetries) continue;`
    (line ~1183), so the stale flag forces another retry.
  - After maxRetries, the turn-level check sees `lastErrorType === 'invalid_response'` and emits EXIT-INVALID-RESPONSES-EXHAUSTED (line ~1442).
- Result: A single early invalid response poisons the whole turn, causing false-positive exhaustion and aborting otherwise progressing sessions.
- Scope: TurnRunner only; no MCP/tool changes required. Applies to all agents/headends.

Decisions (Costa)
1. Clear invalid_response state only **after a successful attempt** that returned tools or final_report (choice 2). Rationale: retries must keep the flag until a good response proves recovery; resetting before the attempt would hide repeated invalid replies.

Plan
- Confirm current maxRetries behavior and attempt counting in TurnRunner against logs.
- Implement chosen reset strategy for `lastError`/`lastErrorType` so invalid flags do not leak across attempts.
- Add targeted log or comment clarifying that exhaustion is based on per-attempt error state.
- Run lint+build; if feasible, add a deterministic Phase 1 harness case reproducing early-invalid → later-valid flow.
- Added orchestrator-level logging for no-tool/no-report cases, guarded text-fallback acceptance to occur only on final turns, and ensured text extraction doesn’t short-circuit subsequent retries. Implemented new Phase 1 scenario `run-test-invalid-response-clears-after-success` and re-ran full `npm run test:phase1` (pass), `npm run lint`, and `npm run build`.

Implied decisions
- No API/schema changes expected; documentation update limited to describing retry semantics if we touch docs.
- No config changes needed in hubspot.ai; fix is core logic.

Testing requirements
- `npm run lint`
- `npm run build`
- Add/execute Phase 1 harness scenario that simulates: attempt1 empty, attempt2 empty, attempt3 valid with tool call but no final report, ensure session does not prematurely EXIT-INVALID-RESPONSES-EXHAUSTED.

Documentation updates required
- If behavior change is user-visible (retry semantics), add a short note to docs/TESTING.md or AI-AGENT-GUIDE.md describing that invalid_response flags are scoped per attempt.

---

New issue (Dec 04 2025): Streaming tool calls dropped
- TL;DR: AI SDK 5.x streams tool calls in `result.toolCalls` but leaves `resp.messages` empty; `BaseLLMProvider.executeStreamingTurn` only reads `resp.messages`, so streamed tool calls are lost and the agent logs “empty response” despite calls being made.

Analysis
- Evidence: Logs show SSE chunk with tool_calls (e.g., `hubspot-get-user-details`) but `[DEBUG] resp.messages structure: []` and `tool-parity(stream): {toolCalls:0,...}` because only `resp.messages` is inspected.
- AI SDK `StreamTextResult` fields: `toolCalls: Promise<TypedToolCall[]>` accumulates streamed calls; `response.messages` can be empty for streaming.
- Code path: `executeStreamingTurn` awaits `result.response` but never awaits `result.toolCalls`; when `resp.messages` is empty, `normalizeResponseMessages` returns [], so tool calls vanish and orchestrator retries.
- Scope: All streaming providers using AI SDK 5.x; not a HubSpot-specific issue.

Decisions (Costa)
1. Handle streamed tool calls: A) Await `result.toolCalls` and synthesize assistant/tool messages when `resp.messages` is empty (recommended); B) Investigate regression first; C) Leave as-is.
2. Fallback behavior: A) Only synthesize when `resp.messages` is empty; B) Always merge `resp.messages` + `toolCalls`; C) Gate behind feature flag/env.
3. Testing: A) Add Phase 1 harness case with streamed tool call and empty resp.messages; B) Rely on existing tests.

Recommendation: 1A + 2A + 3A to restore tool-call delivery with minimal surface area.

Plan (pending decisions)
- Buffer `toolCalls` from `result.toolCalls` during streaming; if `resp.messages` is empty, build a synthetic assistant message containing those tool calls before `convertResponseMessages`.
- Ensure `tool-parity` debug logging counts buffered calls.
- Add Phase 1 harness reproducer for “tool-call streamed, resp.messages empty”.
- Run lint + build.

Implied decisions
- No provider-specific changes expected; applies generically via BaseLLMProvider.
- No docs change unless user-facing behavior noted; optional short note in AI-AGENT-GUIDE about streaming tool-call handling.
