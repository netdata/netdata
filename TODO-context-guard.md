## TL;DR
- Replace snapshot-based budget math with rolling counters (`currentCtxTokens`, `pendingCtxTokens`, `newCtxTokens`) so logs and guards reflect the same numbers.
- Trim overflowing tool responses before final turns and always let the LLM conclude once budget is safe; only synthesize failure reports if trimming cannot restore headroom.
- Add deterministic Phase 1 harness coverage for the new context guard telemetry (tool trim warning, LLM context metrics, tokenized tool success logs).

- `src/ai-agent.ts:111-152` already defines the three counters, but the runtime still relies on `buildContextSnapshot`, `evaluateContextWithExtras`, and `pendingContextSnapshot`, so the new fields never change and logs stay inconsistent with real usage.
- LLM request logging in `src/llm-client.ts:548-612` depends on the new counter-backed `contextMetrics`; this now emits `[tokens: ctx …, new …, schema …, expected …, pct]` and stores `ctx_tokens`, `new_tokens`, `schema_tokens`, `context_pct` in `details`.
- Tool handling (`src/ai-agent.ts:3791-3954`) estimates token cost for each response, logs a success `VRB` entry with `details.tokens`, and on overflow emits a `WRN` log plus an accounting row containing `original_tokens`/`replacement_tokens` before forcing the final turn.
- Current Phase 1 harness coverage (`src/tests/phase1-harness.ts:888-1180`) exercises context guard flows but never asserts the new warning log fields nor the accounting metadata for trimmed tools.
- No deterministic scenario today checks the `[tokens: …]` message or `details.ctx_tokens` etc in LLM request/response logs; existing log assertions (e.g., `RUN_TEST_11`/`RUN_TEST_12`) predate the counter rewrite.
- Successful tool execution cases (e.g., `run-test-13` at `src/tests/phase1-harness.ts:840-865`) verify conversation/accounting only and miss the new `details.tokens` log output, leaving the happy-path telemetry unprotected.

## Decisions (needs Costa’s confirmation)
- **Per-tool failure copy**: When trimming a tool due to overflow, adopt message `(tool failed: context window budget exceeded)` as today, or switch to a more explicit variant (e.g., include estimated tokens and remaining budget). Current implementation already uses that text in `src/ai-agent.ts:3495-3497`; propose keeping it unless you prefer richer wording.
- **Log fields**: Confirm that request log percent should be computed against the configured model limit (after subtracting `maxOutputTokens` + buffer). We can show `WW% = Math.round(expectedTokens / contextWindow * 100)`—is that acceptable?
- **Tool token estimator**: Plan to reuse each target’s configured tokenizer ID; fallback remains approximate if no tokenizer configured. Okay to keep this behaviour?
- **Snapshot usage**: Snapshots (`buildContextSnapshot`, `evaluateContextWithExtras`, `pendingContextSnapshot`) must be removed entirely; operational state must come from the live counters with no rebuilds.
- **New harness scenarios** *(Costa approved 2025-11-03)*: introduce dedicated tests for (1) tool-trim warning log + accounting, (2) LLM request metrics, and (3) successful tool log `details.tokens` instead of extending existing multi-branch cases.

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
11. Introduce `run-test-context-trim-log` covering the warning log + accounting metadata, using deterministic tool overflow.
12. Introduce `run-test-llm-context-metrics` verifying `[tokens: …]` request message and `details` fields on both request and response logs.
13. Introduce `run-test-tool-log-tokens` asserting the happy-path tool completion log includes `details.tokens` alongside accounting consistency.
14. Re-run `npm run test:phase1`, `npm run lint`, and `npm run build`; update TODO Status once all new harness cases pass.

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
- [x] Per-tool overflow replacements log a `WRN` entry with matching turn/subturn metadata — harness coverage added via `run-test-context-trim-log`.
- [x] Forced-final guard continues into concluding LLM call — covered by `run-test-context-forced-final` harness scenario.
