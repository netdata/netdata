# Testing Roadmap

## Status Overview
- ✅ Deterministic harness, scripted provider, and test MCP server are implemented (`src/llm-providers/test-llm.ts`, `src/tests/mcp/test-stdio-server.ts`).
- ✅ Twenty-four deterministic scenarios (`run-test-1` … `run-test-24`) execute via `src/tests/phase1-harness.ts`, covering fallback logic, retries, tool failures, persistence, pricing, and sub-agent flows.
- ✅ `npm run test:phase1` passes (warnings expected for the persistence-error scenario); `npm run build` and `npm run lint` are clean.
- ✅ README now links to `docs/TESTING.md` with a brief harness overview.
- ✅ Coverage guidance now lives in `docs/TESTING.md` (includes `c8` usage and debugging tips).

## Phase 1 — Deterministic Core Harness

**Goal**: stand up a fully deterministic end-to-end harness that exercises the agent loop (LLM turn orchestration + MCP tool execution + final report) without touching production code paths.

### Scope (Status)
- [x] Implement `test-llm` scripted provider with deterministic failures, temperature/topP assertions, and tool availability checks (`src/llm-providers/test-llm.ts`).
  - [x] Turn counting, scenario-driven tool/script outputs, and accounting data are enforced per `ScenarioTurn`.
- [x] Wire provider support into the agent via existing LLM client plumbing.
- [x] Build test MCP stdio server with success/failure/timeout/large-payload behaviours (`src/tests/mcp/test-stdio-server.ts`).
- [x] Author baseline and extended scenarios in `src/tests/fixtures/test-llm-scenarios.ts` (now 24 total, including retry, timeout, persistence, pricing, and sub-agent branches).
- [x] Provide deterministic harness runner (`src/tests/phase1-harness.ts`) callable with `npm run test:phase1`.
- [x] Publish coverage how-to beyond this TODO (dedicated doc or README snippet with `c8` usage) — see `docs/TESTING.md`.

### Deliverables
- [x] `src/llm-providers/test-llm.ts` with enhanced failure-mode controls.
- [x] Test MCP server (`src/tests/mcp/test-stdio-server.ts`) wired through the standard discovery path.
- [x] Scenario fixtures (`src/tests/fixtures/test-llm-scenarios.ts`) plus sub-agent prompt assets under `src/tests/fixtures/subagents*/`.
- [x] Harness CLI (`src/tests/phase1-harness.ts`) + NPM script (`npm run test:phase1`).
- [x] README entry links to `docs/TESTING.md` and notes the deterministic harness command.

### Running Phase 1
- Build the project (`npm run build`).
- Execute the deterministic harness: `node dist/tests/phase1-harness.js` or `npm run test:phase1` (build step is included in the script).
- The harness now executes 42 scripted scenarios:
  - Baseline + MCP failure/retry (`run-test-1` … `run-test-3`).
  - Provider fallback, timeouts, retry exhaustion (`run-test-4` … `run-test-7`).
  - Tool truncation, unknown tool handling, progress reports, final-report schema guards (`run-test-8` … `run-test-12`).
  - Batch tooling paths, pricing accounting, persistence snapshots (`run-test-13` … `run-test-16`).
  - Model overrides, abort/stop flows, sub-agent failure/success, persistence error handling (`run-test-17` … `run-test-24`).
  - Tool concurrency, batch validation/errors, streaming reasoning, provider throw/retry, LLM error matrix (`run-test-25` … `run-test-42`).
- Persistence error scenario (`run-test-20`) intentionally emits warnings when directories are blocked; no action required.
- Enable coverage when needed: `NODE_V8_COVERAGE=coverage node dist/tests/phase1-harness.js` or wrap with `npx c8`.

### Acceptance Criteria
- Running the Phase 1 scenario exercises two turns (tool call turn + final report turn) and succeeds without touching production agent code.
- Deterministic assertions confirm system prompt enforcement, tool availability, tool execution order, final report emission, and accounting entries.
- Coverage tooling instructions exist (e.g., `c8 node --test` or `NODE_V8_COVERAGE=...`) so we can quantify future test reach.

### After Phase 1
Once Phase 1 is delivered, we will enumerate additional scenarios to drive coverage toward 100% across LLM error handling, tool failures, sub-agent recursion, accounting edge cases, etc.

## Phase 2 — Single-Agent Coverage Expansion

**Goal**: exercise every branch of `ai-agent.ts`, the LLM client, and tool orchestration using scripted single-agent scenarios (no sub-agents yet).

**Testing doctrine**: continue expanding only the deterministic harness built on the scripted test LLM provider and test MCP server. Every new case must be modelled as its own scenario that isolates a single behaviour. No alternative unit test harnesses should be introduced unless they wrap the same deterministic provider pair end-to-end.

### Delivered Scenarios (single-agent variants)
- `run-test-17`: Validates provider-level overrides for temperature/topP.
- `run-test-18`: Covers abort-signal cancellation before any LLM call.
- `run-test-19`: Exercises failing sub-agent delegation and accounting propagation.
- `run-test-20`: Forces persistence write failures and warning surfacing.
- `run-test-21`: Confirms rate-limit handling and exponential backoff logging.
- `run-test-22`: Verifies provider validation failures fail fast.
- `run-test-23`: Ensures graceful stop short-circuits conversation/final report.
- `run-test-24`: Runs successful sub-agent execution with accounting + logs.

### Remaining Backlog / Next Scenarios
- Tool concurrency saturation scenario (queueing under low `maxConcurrentTools`) – not yet scripted.
- Additional accounting edge cases (e.g., partial final report status, mixed success/failure batches).
- Coverage reporting playbook (documented steps + thresholds) once concurrency + recursion scenarios are in place.

### Phase 2 Coverage Objectives (Updated)
- Drive `src/ai-agent.ts`, `src/llm-client.ts`, and `src/tools/internal-provider.ts` toward 100% statements/branches/functions/lines using deterministic scenarios only.
- Stretch goals: surface provider-specific adapters (`src/llm-providers/openai.ts`, `anthropic.ts`, `google.ts`, etc.) via scripted harness flows where feasible.

### Next Additions (In Progress)
- **LLM client error matrix** (`llm-client.ts`): deterministic scenarios that script rate-limit, auth, quota, timeout, network, and raw throws to exercise every branch of `mapError` and traced-fetch enrichment.
- **AI Agent orchestration branches** (`ai-agent.ts`): scenarios covering concurrency limiter logs, persistence fallback, final-turn tool filtering/max-turn adjustments, and thinking-stream toggles.
- **Internal provider advanced paths** (`internal-provider.ts`): scenarios hitting Slack metadata merge, JSON schema validation failures, trace logging, and REST fallback/child tool accounting.
- Continue running `npx c8 npm run test:phase1` after each batch of scenarios to verify coverage progress.

### Implementation Notes
- Scenario definitions live in `src/tests/fixtures/test-llm-scenarios.ts`; harness configuration hooks in `src/tests/phase1-harness.ts` set per-scenario overrides.
- Test MCP server already supports timeout, long payload, and failure payloads needed for future scenarios; extend as new cases arise.
- Continue running `npx c8 npm run test:phase1` to monitor coverage as new scenarios land.
