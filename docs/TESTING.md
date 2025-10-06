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

## Coverage And Debugging
- Capture V8 coverage by running `NODE_V8_COVERAGE=coverage npm run test:phase1` or `npx c8 npm run test:phase1` for summarized reports.
- Harness logs, accounting entries, and persistence outputs land under temporary directories prefixed `ai-agent-phase1-*`; inspect these paths to diagnose failures.
- When extending scenarios, add new fixtures alongside the existing files and update the expectations in `src/tests/phase1-harness.ts` to keep assertions deterministic.

## Test MCP Notes
- The test MCP server exposes `test` and `test-summary` tools; sanitized tool names appear as `test__test` in accounting/logs.
- Tool behaviors include success, explicit error responses, long-payload truncation, and simulated timeouts; reuse these payloads before introducing new helpers.
- Update scenario metadata and harness expectations whenever new MCP behaviors are added to keep the suite coherent.
