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
- Local filtering: set `PHASE1_ONLY_SCENARIO=run-test-1,run-test-2` to run a subset while debugging. The harness automatically ignores this variable when `CI` is set (`CI=1|true|yes`), ensuring automation always runs the full suite even if the environment leaks the filter.

### Restart Fixture Controls
The deterministic stdio MCP server exposes several environment knobs so restart scenarios stay reproducible:

| Variable | Effect |
| --- | --- |
| `MCP_FIXTURE_MODE=restart` | Enables the restart fixture behavior (hang, exit, recover). |
| `MCP_FIXTURE_STATE_FILE=/tmp/.../state.json` | Shared state file that tracks phases between process restarts. The harness auto-creates this under `os.tmpdir()`. |
| `MCP_FIXTURE_HANG_MS` / `MCP_FIXTURE_EXIT_DELAY_MS` | Milliseconds to busy-wait/sleep before exiting the child process and how long to wait before actually calling `process.exit`. Defaults: 4000 ms hang, 2000 ms exit delay. |
| `MCP_FIXTURE_BLOCK_EVENT_LOOP=1` | Forces a CPU-bound busy wait instead of `setTimeout`, simulating an event-loop stall. |
| `MCP_FIXTURE_SKIP_EXIT=1` | Prevents the fixture from calling `process.exit`, letting tests simulate a hung process that never dies. |
| `MCP_FIXTURE_FAIL_INIT_ATTEMPTS=N` | Causes the fixture to terminate during startup `N` times before allowing a successful boot (used to test initialization retries). |
| `MCP_FIXTURE_FAIL_RESTART_ATTEMPTS=N` | After the fixture has exited once, it will crash `N` additional times during restart before allowing a successful recovery. |

Use these knobs in new scenarios whenever you need deterministic combinations of hangs, exits, and restart delays; see `run-test-71`, `run-test-72`, and `run-test-73` for examples.

## Phase 2 Integration Runs (Live Only)
- Phase 2 uses the runner in `src/tests/phase2-runner.ts` to execute real-provider scenarios (`basic-llm`, `multi-turn`) in both streaming and non-streaming modes. There is no fixture replay path.
- Model ordering and cost tiers live in `src/tests/phase2-models.ts`. Tier 1 currently targets the self-hosted `vllm/default-model` (OpenAI-compatible) and `ollama/gpt-oss:20b`; higher tiers cover Anthropic, OpenRouter, OpenAI, and Google models.
- Test agents for the scenarios are located under `src/tests/phase2/test-agents/` (`test-master.ai`, `test-agent1.ai`, `test-agent2.ai`).
- Commands:
  - `npm run test:phase2` — build + run every configured model/tier/stream combination.
  - `npm run test:phase2:tier1` — run only Tier 1 (self-hosted) models.
  - `npm run test:phase2:tier2` — run Tiers 1 and 2 (adds mid-priced cloud models).
  - `npm run test:all` — executes Phase 1, then Phase 2 Tier 1.
- Safeguards:
  - The runner stops after the first failure when `PHASE2_STOP_ON_FAILURE=1` (default). Set `PHASE2_STOP_ON_FAILURE=0` to force the full matrix while debugging.
  - Optional tracing/logging toggles: `PHASE2_TRACE_LLM`, `PHASE2_TRACE_SDK`, `PHASE2_TRACE_MCP`, `PHASE2_VERBOSE`.
  - All runs are live. Verify credentials before invoking Tier 2/3 and budget for provider costs.


## Coverage And Debugging
- Capture V8 coverage by running `NODE_V8_COVERAGE=coverage npm run test:phase1` or `npx c8 npm run test:phase1` for summarized reports.
- Harness logs, accounting entries, and persistence outputs land under temporary directories prefixed `ai-agent-phase1-*`; inspect these paths to diagnose failures.
- When extending scenarios, update `src/tests/phase1-harness.ts` expectations and ensure new live runs are scoped (e.g., `--tier` / `--model`) before landing changes.

## Test MCP Notes
- The test MCP server exposes `test` and `test-summary` tools; sanitized tool names appear as `test__test` in accounting/logs.
- Tool behaviors include success, explicit error responses, long-payload truncation, and simulated timeouts; reuse these payloads before introducing new helpers.
- Update scenario metadata and harness expectations whenever new MCP behaviors are added to keep the suite coherent.
