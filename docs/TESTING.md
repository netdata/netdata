# Testing

## Deterministic Harness
- Harness runs scripted scenarios against the core agent loop without touching production providers.
- Scripted provider lives at `src/llm-providers/test-llm.ts`; deterministic MCP server lives at `src/tests/mcp/test-stdio-server.ts`.
- Scenario definitions reside in `src/tests/fixtures/test-llm-scenarios.ts` and are executed via `src/tests/phase1-harness.ts`.

## Running The Suite
- Build + lint remain required before invoking tests (`npm run build`, `npm run lint`).
- Execute all deterministic scenarios with `npm run test:phase1` (the script rebuilds automatically).
- Expect three informational warnings when scenario `run-test-20` simulates persistence write failures; no manual cleanup is needed.
- Scenario coverage currently spans fallback routing, MCP timeouts, retry exhaustion, persistence artifacts, pricing/sub-agent flows, and rate-limit backoff.

## Phase 2 Integration Runs
- Phase 2 exercises live provider integrations using the runner at `src/tests/phase2-runner.ts`.
- Edit the ordered model list in `src/tests/phase2-models.ts` to adjust coverage or tier ordering.
- Test agents live under `fixtures/test-agents/` (`test-master.ai`, `test-agent1.ai`, `test-agent2.ai`); prompts are documented in `TODO-TESTING2.md`.
- Commands:
  - `npm run test:phase2` — build + run all tiers and streaming variants.
  - `npm run test:phase2:tier1` — build + run Tier 1 (self-hosted) models only.
  - `npm run test:phase2:tier2` — build + run Tier 1 and Tier 2 models.
  - `npm run test:all` — executes Phase 1 deterministic harness, then reruns the Phase 2 Tier 1 subset.
- Temporary safeguard: the runner currently stops after the first failure to limit spend; export `PHASE2_STOP_ON_FAILURE=0` to force full execution when debugging.
- Runner enforces zero-tolerance: every combination must finish with `status=success`, emit a final report, record accounting entries, execute agent1→agent2 in order, and (for Anthropic high-reasoning runs) preserve reasoning signatures across turns. Streaming and non-streaming variants run for each scenario.

## Coverage And Debugging
- Capture V8 coverage by running `NODE_V8_COVERAGE=coverage npm run test:phase1` or `npx c8 npm run test:phase1` for summarized reports.
- Harness logs, accounting entries, and persistence outputs land under temporary directories prefixed `ai-agent-phase1-*`; inspect these paths to diagnose failures.
- When extending scenarios, add new fixtures alongside the existing files and update the expectations in `src/tests/phase1-harness.ts` to keep assertions deterministic.

## Test MCP Notes
- The test MCP server exposes `test` and `test-summary` tools; sanitized tool names appear as `test__test` in accounting/logs.
- Tool behaviors include success, explicit error responses, long-payload truncation, and simulated timeouts; reuse these payloads before introducing new helpers.
- Update scenario metadata and harness expectations whenever new MCP behaviors are added to keep the suite coherent.
