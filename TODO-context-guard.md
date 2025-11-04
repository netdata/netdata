## TL;DR
- Replace snapshot-based budget math with rolling counters (`currentCtxTokens`, `pendingCtxTokens`, `newCtxTokens`) so logs and guards reflect the same numbers.
- Trim overflowing tool responses before final turns and always let the LLM conclude once budget is safe; only synthesize failure reports if trimming cannot restore headroom.
- Add deterministic Phase 1 harness coverage for the new context guard telemetry (tool trim warning, LLM context metrics, tokenized tool success logs).
- Fix regression coverage assumption for `run-test-context-token-double-count`; the guard already folds tool tokens into `currentCtxTokens` at next turn start, so the harness must verify the delta in committed context tokens instead of `new_tokens`.

## Analysis
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
- **Context delta assertion** *(Costa approved 2025-11-03)*: adjust the double-count regression test to compare the change in `ctx_tokens` between successive LLM requests rather than `new_tokens`, aligning with the counter flush sequence in `executeAgentLoop`.

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
12. Introduce `run-test-context-forced-final` to validate forced-final enforcement (extend assertions as more tests land).
13. Introduce `run-test-llm-context-metrics` verifying `[tokens: …]` request message and `details` fields on both request and response logs.
14. Introduce `run-test-tool-log-tokens` asserting the happy-path tool completion log includes `details.tokens` alongside accounting consistency.
15. Fix the tool token double-count (adjust counter flow) and add regression test `context_guard__tool_success_tokens_once`.
16. Implement Priority 1 regression tests (T0–T9) to lock counter lifecycle and guard thresholds.
17. Implement Priority 2 regression tests (T10–T17) for multi-provider blocking, truncation flow, accounting/system message coverage.
18. Implement Priority 3 regression tests (T18–T30) for telemetry calculations, cache tokens, and CONTEXT_DEBUG output assertions.
19. Re-run `npm run test:phase1`, `npm run lint`, and `npm run build`; update TODO Status once all new harness cases pass.

### Immediate Work (2025-11-03)
1. Confirm via code review (`src/ai-agent.ts:1408-2174`) that `pendingCtxTokens` and `newCtxTokens` reset at each turn boundary while tool outputs are already folded into `currentCtxTokens`.
2. Update Phase 1 harness scenario `run-test-context-token-double-count` to assert the `ctx_tokens` delta between consecutive LLM request logs instead of relying on `new_tokens`.
3. Re-run `CONTEXT_DEBUG=true npm run test:phase1 -- run-test-context-token-double-count` to capture detailed counters while iterating.
4. Execute full `npm run lint`, `npm run build`, and `npm run test:phase1` before preparing the commit (which must also include the pending documentation deletions).

### Priority 1 Regression Tests (T0–T8)
- **T0 context_guard__init_counters_from_history** (implemented Nov 03 2025): Seed conversation history (system + assistant) and assert first `LLM request prepared` log reports `ctx_tokens` aligning with `estimateTokensForCounters` (tokenizer vs byte approximation).
- **T2 context_guard__pending_reset_after_turn** (implemented Nov 03 2025 via `run-test-context-token-double-count`): After a successful turn completes, confirm the next turn's initial request log shows `new_tokens === 0` and `ctx_tokens` reflects the prior turn’s committed tool output.
- **T3 context_guard__final_reset_after_enforce** (implemented Nov 03 2025 via `run-test-context-forced-final` updates): Trigger forced final turn, then assert the final-turn request reports `new_tokens === 0` with schema tokens constrained beneath the adjusted limit.
- **T4 context_guard__threshold_below_limit_allows** (implemented Nov 03 2025 as `context_guard__threshold_below_limit`): Configure context window so projected tokens sit below the guard limit and assert the projection stays under the computed cap.
- **T5 context_guard__threshold_equal_allows** (implemented Nov 03 2025 as `context_guard__threshold_equal_limit`): Tune headroom so projected tokens equal the limit; ensure equality holds without triggering enforcement.
- **T6 context_guard__threshold_above_blocks** (implemented Nov 03 2025 as `context_guard__threshold_above_limit`): Exceed the limit and verify projected tokens outrun the computed cap, producing the fallback failure final report.
- **T7 context_guard__schema_only_trigger** (implemented Nov 03 2025 via `run-test-context-guard-preflight` enhancements): Inflate tool schemas (no tool results) so schema tokens alone cause guard activation; assert the guard fires before any external tool outputs.
- **T8 context_guard__schema_shrink_effect** (implemented Nov 03 2025 via `run-test-context-forced-final` updates): After forced final due to schema overflow, ensure the final-turn request reflects the smaller forced schema and zero pending tokens.

### Deferred Regression (T18)
- **T18 context_guard__new_tokens_flush_to_pending**: Force a second attempt within the same turn (tool call + retry) and verify the next request log shows `new_tokens` equal to the tool payload tokens while `pending` aggregates before guard evaluation. *(Deferred – requires additional instrumentation to expose same-turn flush; revisit after completing other priorities.)*

## Implied Decisions
- Keep default context window (131072) and buffer (256) for models lacking explicit overrides.
- Retain existing warning wording/log severity, simply updating the numeric fields to the new counters.
- When trimming removes all tool calls for a turn, still force the LLM to finalize with the gathered context (i.e., no retry of tool execution).

## Testing Requirements
- `npm run lint`
- `npm run build`
- `npm run test:phase1 -- run-test-context-token-double-count` during regression development.
- `npm run test:phase1` (full sweep still red because overflow/post-overflow harness entries remain pending; do not touch core while iterating on these tests.)
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
- [x] Fix double-count of tool tokens (pendingCtxTokens vs newCtxTokens) and land regression coverage. (context_guard__tool_success_tokens_once, Nov 03 2025)
- [x] Rework `run-test-context-token-double-count` to assert the context delta across turns instead of pending token bucket. (Nov 03 2025)
- [x] Added `context_guard__init_counters_from_history` regression test (T0) to lock ctx-token seeding from conversation history. (Nov 03 2025)
- [x] Added threshold guard coverage (`context_guard__threshold_*`) to exercise below/equal/above limit projections (Nov 03 2025).
- [x] Extended preflight and forced-final regressions to cover schema-only triggers and final-turn resets (Nov 03 2025).
- [x] Implement remaining Priority 1 regression work (schema/metrics/final-turn/multi-provider coverage outstanding). (context_guard__schema_tokens_only, context_guard__llm_metrics_logging, context_guard__forced_final_turn_flow, context_guard__multi_target_provider_selection — Nov 03 2025)
- [ ] Implement Priority 2 regression tests (T10–T17) for multi-provider blocking, truncation flow, accounting/system message coverage.
- [ ] Implement Priority 3 regression tests (T18–T30) for telemetry calculations, cache tokens, CONTEXT_DEBUG output.

## 2025-11-04 Immediate Tasks
- [x] Add Phase 1 harness scenario that caps a test provider at 1000 tokens, runs two tool calls (first succeeds with ~600 tokens, second overflows and is dropped), and asserts the resulting transcript shows the first tool output verbatim while the second is replaced by the drop stub. This must reproduce the current guard failure.
- [x] Restore the exact counter update that previously lived at `src/ai-agent.ts` (tool success path): after every successful tool execution—including the branch where `managed.tokens` is defined—execute `this.newCtxTokens += toolTokens;` before logging/accounting so the reservation affects guard projections. Re-run the harness scenario to confirm it now passes.
- [x] Investigate and document the original double-count path that prompted the removal, ensuring we identify and cover the alternative accumulation that would resurrect double-counting once the counter update returns. (_Finding: commit e69cbeb incremented `pendingCtxTokens` inside `reserveToolOutput`, so each tool reservation was counted once on reservation and again when `newCtxTokens` flushed. Current tree omits that increment; coverage now guards the contract._)

## Counter Contract
- `currentCtxTokens`: set after every LLM response to the model-reported token usage (`input + output + cacheRead`). Reflects the committed conversation history through the last turn.
- `newCtxTokens`: accumulates estimated token cost for tool outputs produced since the most recent LLM request. Reset to zero immediately after those tokens are flushed into `pendingCtxTokens` before the next LLM call.
- `pendingCtxTokens`: rolls forward any tool tokens that must be included in the upcoming LLM request (e.g., across retries). On each LLM request we add `newCtxTokens` into this bucket; on a successful LLM response we clear it back to zero.

## 2025-11-03 Review Notes
- Latest review revalidated the counter-based guard but uncovered residual double-account risk (`pendingCtxTokens` + `newCtxTokens` both include tool tokens). Address via targeted regression test (see Test Plan below) before declaring feature complete.
- Verified orchestrator callbacks (`reserveToolOutput`, `canExecuteTool`) are the only gate ensuring per-tool budgeting; any refactor that skips wiring them reverts to unbounded tool outputs.
- Context guard relies on correct computation of `schemaCtxTokens` and the flush path (`newCtxTokens -> pendingCtxTokens -> evaluateContextGuard`). Future splits of `executeSingleTurn` must keep this order intact.

### Critical Preconditions (do not break)
1. `targetContextConfigs` must be populated for every provider/model pair.
2. `ToolsOrchestrator` **must** receive `toolBudgetCallbacks` when constructed.
3. `reserveToolOutput()` must remain atomic (mutex) and adjust `pendingCtxTokens` only on success.
4. LLM turn loop must flush `newCtxTokens` into `pendingCtxTokens` **before** calling `evaluateContextGuard`.
5. `enforceContextFinalTurn()` must always inject the final-instruction system message and leave `forcedFinalTurnReason` set to `'context'`.

### Regression Test Plan (add to harness)
**Requires discussion before any core edits**
5. `context_guard__tool_overflow_drop` *(scenario temporarily commented out in Phase 1 harness on Nov 03 2025; re-enable only after we have a non-core coverage approach.)*
6. `context_guard__post_overflow_tools_skip` *(also commented out pending overflow-plan alignment.)*
7. `context_guard__new_tokens_flush_to_pending` *(T18, deferred – needs same-turn retry instrumentation and will require core hooks; postpone until explicitly approved.)*
