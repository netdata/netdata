# TL;DR
- Clarify how existing deterministic Phase 1 scenarios map to the eleven user-facing behaviors (LLM calls, tool usage, retries, fallbacks, limits, context guard, etc.).
- Identify where coverage is redundant or overly granular versus feature-level verification, based on `src/tests/phase1-harness.ts` and `src/tests/fixtures/test-llm-scenarios.ts`.
- Prepare options for a leaner, behavior-driven test suite while preserving current regression protection.

# Analysis
- Phase 1 harness (`src/tests/phase1-harness.ts`) drives 100+ scripted scenarios (`src/tests/fixtures/test-llm-scenarios.ts`) against the deterministic provider and MCP stub, already touching every surface area in docs/TESTING.md.
- Each core behavior listed by Costa has at least one scenario:
  - Facts: `run-test-1` exercises nominal LLM + tool success; `run-test-61` + harness assertions ensure tool-call caps (feature 7); `run-test-max-turn-limit` verifies forced-final-turn ephemerals stay transient (feature 6/8) via captured `TurnRequest`.
  - Facts: `run-test-3`, `run-test-6`, `run-test-62` cover retry loops, exhaustion, and abort interruption (feature 3); `run-test-4` plus harness sections around lines 1097–1122 and 6382–6507 confirm provider fallback usage and accounting (feature 4).
  - Facts: Tool failure reporting is validated by scenarios such as `run-test-7` (timeout), `run-test-8` (truncation), and `run-test-60`/`run-test-61` asserting failure-status final reports still render summaries (features 5 & 11).
  - Facts: Parameter propagation checks in harness block starting line 3633 ensure temperature/topP/maxOutputTokens/repeatPenalty (feature 9); multiple `run-test-context-*` scenarios (e.g., `run-test-context-forced-final`, `run-test-context-limit`) drive the context guard flow, including rejection + forced conclusion (feature 10).
  - Facts: Session completion guarantees appear where `result.success === true` even when `finalReport.status === 'failure'` (e.g., `run-test-61`, `run-test-context-forced-final`), satisfying feature 11 except intentional cancellations (e.g., `run-test-62`).
- Redundancy hotspots: retry/fallback behavior is revalidated across dozens of IDs (`run-test-3`, `run-test-4`, `run-test-37`, `run-test-83-*`, `run-test-context-retry`), and many scenarios assert minute logging/accounting details instead of outcome-focused behavior.
- Missing clarity: there is no consolidated matrix documenting which scenarios satisfy which user-facing guarantees; developers must cross-reference harness assertions manually, causing confusion.

# Decisions
- Need Costa’s direction on whether to collapse multiple microscopic scenarios into broader E2E cases per behavior group, or retain fine-grained assertions and simply document the mapping.
- Confirm if cancellations (e.g., abort-driven `run-test-62`) should be treated as exceptions to “sessions always produce output” or if additional fallback reporting is desired.

# Plan
- Catalogue every Phase 1 scenario by behavior tag (LLM retry, tool failure, context guard, limits, accounting) to build a reference matrix.
- Highlight redundant scenarios and propose consolidation strategies (e.g., single parametrized retry test instead of multiple variants).
- Draft recommendations for a behavior-driven Phase 1 suite (core path + fallbacks + limits), leaving specialized accounting/logging checks in unit or integration layers.
- Validate recommendations against Phase 2 live runs to ensure real-provider coverage is preserved.

# Implied decisions
- Transitioning to higher-level tests probably requires reworking `phase1-harness` helpers and may impact existing coverage thresholds; alignment with CI expectations is necessary.
- If we demote certain assertions to unit tests, we must decide where new unit coverage should live (e.g., `src/tests/unit` vs. new directories).

# Testing requirements
- Future restructuring must keep `npm run build`, `npm run lint`, and `npm run test:phase1` green; any scenario pruning mandates updating harness expectations and coverage baselines.

# Documentation updates required
- Update `docs/TESTING.md` (and possibly `README.md` or new testing overview) to reflect the behavior-centric test matrix and clarify how Phase 1/Phase 2 cover the eleven core guarantees.
