# TL;DR
- Catalog every remaining context-window regression test, execute them in sequence, and conclude with a full end-to-end review so no coverage gap survives future refactors.

# Analysis
- Forced-final guard now relies on counter state, schema recomputation, and logging/telemetry; regressions in any layer can silently reintroduce overflow failures.
- Phase 1 harness contains core scenarios but still lacks stress cases (multiple tool bursts, tokenizer drift, cache token handling) and isolated unit tests for counter invariants, schema sizing, logging, and metrics.
- Without a disciplined checklist, future refactors may drop or alter tests, re-opening failure modes we just closed.

# Plan
1. **Harness: Bulk Tool Burst** — Add a deterministic case with several large tool outputs in one turn; assert newest outputs are trimmed first and pending counters shrink accordingly.
2. **Harness: Tokenizer Drift** — Mix approximate and canonical tokenizers to trigger post-shrink warnings while ensuring final turn succeeds.
3. **Harness: Cache Token Handling** — Simulate cache read/write tokens and confirm `currentCtxTokens` includes them, with guard limits honored.
4. **Harness: Tool Retry Replacement** — Force a retried tool to overflow, ensure failure stub replaces output, and verify accounting records the failure.
5. **Harness: Summarizer Placeholder** — (Future) when summarization arrives, add scenario validating guard can delegate to summarizer and continue.
6. **Unit: Counter Invariants** — Test `current`, `pending`, `new` transitions across success, retry, and failure paths.
7. **Unit: Schema Computation** — Validate `computeForcedFinalSchemaTokens` never increases schema size and handles missing final-report tool gracefully.
8. **Unit: Tool Guard Ordering** — Confirm byte clamp occurs before token estimation in `ToolsOrchestrator`.
9. **Unit: Logging Hooks** — Ensure `CONTEXT_POST_SHRINK_WARN` and `CONTEXT_POST_SHRINK_TURN_WARN` emit exactly once per forced-final sequence.
10. **Unit: Telemetry Metrics** — Verify `recordContextGuardMetrics` fires with correct labels for each guard trigger.
11. **Property/Fuzz Test** — Randomize conversation/tool/token mixes to confirm guard never queues a request beyond `contextWindow - buffer - maxOutput`.
12. **Telemetry Alert Validation** — Add automated check that forced-final counters increment during guard scenarios.
13. **Documentation Update** — Once suite is complete, document guard workflow/tests in `docs/IMPLEMENTATION.md`.
14. **End-to-End Review** — After all tests pass, perform comprehensive audit of guard code, logs, accounting, and telemetry; record findings.

# Implied Decisions
- Every item above is mandatory before closing the context guard stabilization effort.
- CONTEXT_DEBUG logging remains only for development; remove once tests cover behavior.
- End-to-end review is required; do not close this TODO until completed and documented.

# Testing Requirements
- Run `npm run lint`, `npm run build`, and `npm run test:phase1` after each new scenario.
- Introduce a `npm run test:unit` when unit suites land and include it in CI.

# Documentation Updates Required
- Update `docs/IMPLEMENTATION.md` post-completion per Plan item 13.
