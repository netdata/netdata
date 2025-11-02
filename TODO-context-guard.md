## TL;DR
- Replace snapshot-based budget math with rolling counters (`currentCtxTokens`, `pendingCtxTokens`, `newCtxTokens`) so logs and guards reflect the same numbers.
- Trim overflowing tool responses before final turns and always let the LLM conclude once budget is safe; only synthesize failure reports if trimming cannot restore headroom.

## Analysis
- `src/ai-agent.ts:111-152` already defines the three counters, but the runtime still relies on `buildContextSnapshot`, `evaluateContextWithExtras`, and `pendingContextSnapshot`, so the new fields never change and logs stay inconsistent with real usage.
- LLM request logging in `src/llm-client.ts:548-604` depends on `TurnRequest.contextSnapshot`, which today derives from token re-estimation of the entire conversation; this is why request logs show mismatched `ctx/tools/expected` values versus the preceding response log.
- Tool processing updates only bytes/latency in accounting; there is no token estimator tied to `newCtxTokens`, so we cannot log per-tool token costs or trim before the next request.
- Guard enforcement (`src/ai-agent.ts:2995-3050`) projects full conversations, marks providers as blocked, and immediately forces a synthesized final report when all models overflow. This pathway never trims tool output and never gives the model a final turn, contradicting the desired behaviour recorded in the verbose run above.
- Phase 1 harness scenarios (`src/tests/phase1-harness.ts:870-1010`) still expect the preflight guard to abort without issuing an LLM call; these assertions must change once trimming keeps the guard below the limit.
- Logging expectations from Costa require request logs to emit `[tokens: ctx XXXX, tools YYYY, expected ZZZZ, WW%]`, where `tools` counts only the newly added tool tokens for that request; current formatting cannot satisfy this without the counter rewrite.

## Decisions (needs Costa’s confirmation)
- **Per-tool failure copy**: When trimming a tool due to overflow, adopt message `(tool failed: context window budget exceeded)` as today, or switch to a more explicit variant (e.g., include estimated tokens and remaining budget). Current implementation already uses that text in `src/ai-agent.ts:3495-3497`; propose keeping it unless you prefer richer wording.
- **Log fields**: Confirm that request log percent should be computed against the configured model limit (after subtracting `maxOutputTokens` + buffer). We can show `WW% = Math.round(expectedTokens / contextWindow * 100)`—is that acceptable?
- **Tool token estimator**: Plan to reuse each target’s configured tokenizer ID; fallback remains approximate if no tokenizer configured. Okay to keep this behaviour?
- **Snapshot usage**: Snapshots (`buildContextSnapshot`, `evaluateContextWithExtras`, `pendingContextSnapshot`) must be removed entirely; operational state must come from the live counters with no rebuilds.

## Plan
1. Remove snapshot/pending-context helpers and wire the three counters into tool callbacks, LLM request preparation, and LLM success handling.
2. Emit per-tool token estimates, update `LLMClient.logRequest`/`logResponse` to use the counters (`ctx=current`, `tools=pending`, `expected=current+pending+maxOutput+buffer`), and add `ctx …` to response logs.
3. Rework the guard: if `current + pending + new + maxOutput >= contextWindow`, iteratively trim newest tool results (updating counters) until the inequality is false or no trim is possible; if trimming succeeds enqueue final-turn instruction, else synthesize failure report. Document in code that tool payloads are byte-clamped (via `ToolsOrchestrator`) before token measurements. Ensure we still issue at least one final-turn LLM call; do not break the turn loop when `evaluateContextForProvider` or the per-attempt guard exceed the limit.
4. Update metrics/logging helpers and remove obsolete fields (`pendingContextSnapshot`, `evaluateContextWithExtras`, etc.). Update telemetry labels to include new counters.
5. Adjust Phase 1 harness cases and add a deterministic scenario where trimming succeeds before overflow. Ensure new fake-overflow case captures retry path.
6. Run `npm run lint`, `npm run build`, and `npm run test:phase1`; perform live smoke test (`neda/web-search.ai`) to verify logs/behaviour.
7. Update docs (`LOGGING-IMPROVEMENT.md`, specs/implementation notes) and then close or prune this TODO file.
8. When the guard replaces a tool response with `(tool failed: context window budget exceeded)`, emit a `WRN` log on the same turn/subturn highlighting the tool name and original token estimate.
9. Add a deterministic harness case later that proves oversized tool payloads are truncated before token accounting and that the new warning log appears, once core behaviour stabilises.
10. Validate forced-final flow end-to-end: when the guard triggers, ensure schema shrink occurs and the next loop iteration issues the concluding LLM request instead of synthesizing a report; capture this with harness coverage.

## Implied Decisions
- Keep default context window (131072) and buffer (256) for models lacking explicit overrides.
- Retain existing warning wording/log severity, simply updating the numeric fields to the new counters.
- When trimming removes all tool calls for a turn, still force the LLM to finalize with the gathered context (i.e., no retry of tool execution).

## Testing Requirements
- `npm run lint`
- `npm run build`
- `npm run test:phase1` (ensure new scenarios pass)
- Manual run: `./neda/web-search.ai --verbose "observability pain points 2025" --override models=vllm/default-model`

## Documentation Updates Required
- `docs/LOGS.md` (describe new ctx/tools/expected fields and response `ctx` addition).
- `docs/IMPLEMENTATION.md` (detail counter-based guard and trimming workflow).
- `README.md` or `docs/SPECS.md` section on context management (if present) to reflect new behaviour.

## Status
- [x] Counter wiring replaces snapshot math for requests, tools, and responses.
- [x] Guard trimming converts overflowing tool outputs and enforces final turns without synthetic fallback unless trimming fails.
- [x] Logging/docs updated (`docs/LOGS.md`) to cover new `ctx/tool/expected` fields.
- [x] Harness expectations adjusted; `npm run lint`, `npm run build`, and `npm run test:phase1` pass locally (Nov 02 2025).
- [x] Per-tool overflow replacements log a `WRN` entry with matching turn/subturn metadata.
- [ ] Forced-final guard continues into concluding LLM call (needs harness coverage/doc update).
