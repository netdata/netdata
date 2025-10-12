# Testing Roadmap

## Status Overview
- ✅ Deterministic harness, scripted provider, and test MCP server remain the single source of truth for automated testing (`src/llm-providers/test-llm.ts`, `src/tests/mcp/test-stdio-server.ts`).
- ✅ Eighty-two deterministic scenarios (`run-test-1` … `run-test-82`) execute via `src/tests/phase1-harness.ts`, covering retries, provider failover, persistence, concurrency, schema enforcement, sub-agents (success/failure), Slack/REST integrations, and recent schema preload work.
- ✅ `npm run test:phase1`, `npm run build`, and `npm run lint` are green; the persistence-error scenario still emits intentional warnings when session storage is blocked.
- ✅ README → `docs/TESTING.md` flow documents how to operate the harness and collect coverage.
- ✅ Coverage guidance (including `c8` instructions) lives in `docs/TESTING.md`; latest schema scenarios push coverage further across `agent-loader`, `subagent-registry`, and `ai-agent` branches.
- ✅ LLM client error matrix scenarios (`run-test-83-*`) exercise `LLMClient.mapError` for auth, quota, rate limit, timeout, network, and model errors.

## Phase 1 — Deterministic Core Harness

**Goal**: stand up a fully deterministic end-to-end harness that exercises the agent loop (LLM turn orchestration + MCP tool execution + final report) without touching production code paths.

### Scope (Status)
- [x] Implemented the scripted provider (`src/llm-providers/test-llm.ts`) with deterministic failure matrix, temperature/topP assertions, and tool availability enforcement.
- [x] Wired the provider through the standard LLM client path.
- [x] Built the test MCP stdio server with success/failure/timeout/large-payload behaviours (`src/tests/mcp/test-stdio-server.ts`).
- [x] Authored baseline + extended scenarios in `src/tests/fixtures/test-llm-scenarios.ts` (now 82 total, including schema-preload, concurrency, accounting, rate-limit, and sub-agent recursion/validation flows).
- [x] Ship deterministic harness runner (`src/tests/phase1-harness.ts`) with `npm run test:phase1` hook.
- [x] Published coverage how-to in `docs/TESTING.md` (c8 usage, coverage debugging steps, CLI shortcuts).

### Deliverables
- [x] `src/llm-providers/test-llm.ts` with enhanced failure-mode controls.
- [x] Test MCP server (`src/tests/mcp/test-stdio-server.ts`) wired through the standard discovery path.
- [x] Scenario fixtures (`src/tests/fixtures/test-llm-scenarios.ts`) plus sub-agent prompt assets under `src/tests/fixtures/subagents*/`.
- [x] Harness CLI (`src/tests/phase1-harness.ts`) + NPM script (`npm run test:phase1`).
- [x] README entry links to `docs/TESTING.md` and notes the deterministic harness command.

### Running Phase 1
- Build (`npm run build`).
- Execute the deterministic harness: `npm run test:phase1` (includes build) or `node dist/tests/phase1-harness.js`.
- Scenario buckets presently cover:
  - Baseline MCP/LLM happy paths and retry matrix (`run-test-1` … `run-test-16`).
  - Provider overrides, abort/stop, sub-agent failure/success, persistence error handling (`run-test-17` … `run-test-24`).
  - Concurrency limits, batch validation/errors, streaming reasoning, provider throw/retry, rate-limit backoff (`run-test-25` … `run-test-58`).
  - Pricing, Slack/REST integrations, traced fetch flows, env fallback, prompt flattening (`run-test-45`, `run-test-46`, `run-test-75` … `run-test-78`).
  - Schema preload / validation coverage (`run-test-79` … `run-test-82`).
- Persistence-error scenario (`run-test-20`) intentionally emits warnings when session storage is blocked; nothing to fix.
- Coverage: `npx c8 npm run test:phase1` or `NODE_V8_COVERAGE=coverage npm run test:phase1`.

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
- Expand provider-specific adapters (Anthropic/OpenAI/Google) with deterministic fixtures that cover provider-normalised error mapping and streaming deltas.
- Cover REST/MCP edge cases not yet scripted (e.g., mixed success/failure batches with partial accounting, REST fallback headers).
- Add scenario asserting AI-agent final report status propagation (success/partial/failure) through `agent__final_report` to improve `internal-provider.ts` coverage.
- Follow up with coverage snapshot + gating (document thresholds and wire into CI) once the above scenarios land.

### Phase 2 Coverage Objectives (Updated)
- Drive `src/ai-agent.ts`, `src/llm-client.ts`, and `src/tools/internal-provider.ts` toward 100% statements/branches/functions/lines using deterministic scenarios only.
- Stretch goals: surface provider-specific adapters (`src/llm-providers/openai.ts`, `anthropic.ts`, `google.ts`, etc.) via scripted harness flows where feasible.

### Next Additions (Planned)
- **AI Agent orchestration** (`ai-agent.ts`): add deterministic cases for final-turn tool filtering, thinking stream toggles, and persistence fallback logging.
- **Internal provider advanced paths** (`internal-provider.ts`): cover Slack metadata merge, REST fallback accounting, and JSON schema validation failures.
- Keep running `npx c8 npm run test:phase1` after each scenario batch to verify incremental coverage.

### Implementation Notes
- Scenario definitions live in `src/tests/fixtures/test-llm-scenarios.ts`; harness configuration hooks in `src/tests/phase1-harness.ts` set per-scenario overrides.
- Test MCP server already supports timeout, long payload, and failure payloads needed for future scenarios; extend as new cases arise.
- Continue running `npx c8 npm run test:phase1` to monitor coverage as new scenarios land.
