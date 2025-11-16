# Unified Engineering Backlog (Complexity, Quality, and Operations)

## TL;DR
- Collapse every outstanding TODO/review into a single backlog so architectural refactors, security/remediation work, and productization tasks share one prioritized queue.
- Core blockers remain the `AIAgentSession`/tool orchestration monoliths, lack of transport-level safety nets (timeouts/retries/queues), missing auth & file-include guardrails, and noisy deterministic harness runs that mask regressions.
- Secondary tracks cover deterministic sub-agent chaining, reasoning/context guard polish, structured telemetry/log schemas, shared MCP lifecycle + tool queues, OpenAI pricing schema updates, dependency sweeps, prompt/docs alignment (code-review agents), session-visualization UI, and external benchmarking (ai-agent vs swarms).
- Completed items (provider hook rewrite, streaming fix, silent-core cleanup) stay documented here for traceability; all future updates to architecture, docs, or pricing must edit this single backlog.

## Analysis

### Architecture & Complexity
- `src/ai-agent.ts` still houses >4 k LOC with 249 complexity in `executeAgentLoop` (per `reviews/architecture.md` & `reviews/complexity.md`); responsibilities span retry orchestration, telemetry, tool management, persistence, and final-report synthesis. Extract `TurnLoopRunner`, `ToolExecutionManager`, `SessionStateManager`, and `FinalReportManager` so `AIAgentSession` becomes a coordinator.
- `src/tools/tools.ts` (`executeWithManagementInternal`, complexity 69) mixes queueing, telemetry, and provider dispatch; needs a dedicated tool-execution facade plus shared queue manager (see “Shared Tool Queue Manager” section below).
- `src/agent-loader.ts`, `src/cli.ts`, `src/headends/rest-headend.ts`, and `BaseLLMProvider` remain monolithic; align with the decomposition roadmap: prompt repository + config service + session factory, application host abstraction, provider strategy split.
- CLI/bootstrap should become a thin shell that instantiates a reusable service container so other headends/API surfaces stop depending on CLI internals (per `reviews/architecture.md`).

### Execution Flow & Agent Features
- **Deterministic sub-agent chaining** (`TODO-CHAINS*`): `src/subagent-registry.ts` lacks `next` metadata, cycle detection, and chain-runner logic; frontmatter parser disallows a `next` key. Need a design decision on single-step vs array semantics, injected `reason` strings, downstream schema enforcement, and failure surfacing.
- **Code-review agent prompts** (`TODO-code-review-agents.md`): ensure every prompt honors `${FORMAT}`/`${MAX_TURNS}` as mandated in `docs/AI-AGENT-GUIDE.md`; reconcile README claims (“specialists reuse discovery output only”) with actual tool access. Deliver an audit + remediation plan.
- **Reasoning defaults/overrides** (`TODO-reasoning-defaults.md`): clarify semantics for `unset` vs missing keys, wire `--default-reasoning` to only fill absent prompts, and expand deterministic coverage across frontmatter/default/override combinations.
- **Context guard counters** (`TODO-context-guard.md`): replace snapshot math with live counters (`currentCtxTokens`, `pendingCtxTokens`, `newCtxTokens`), trim tool outputs iteratively before forcing final turns, align warnings/accounting/logs, and update Phase 1 coverage for warning logs and `[tokens: ctx …]` metrics.
- **Tool parameter parsing** (`TODO-llm-tool-parameters.md`): unify JSON-string parsing via shared helper, preserve lossy repair heuristics, and emit `ERR` logs whenever malformed payloads are tolerated (providers, sanitizer, nested final reports, `agent__batch`). Phase 1 scenarios (`run-test-122/124/131`) cover regressions.

### Reliability, Security & Production Readiness
- **HTTP/auth hardening** (from `reviews/security.md` & `reviews/production.md`): REST headend currently unauthenticated; prompt include resolver allows arbitrary file disclosure; TRACE logging leaks non-Authorization secrets. Need API key/mTLS enforcement, include sandboxing (block `..`, absolute paths), and broader header redaction.
- **Transport safety nets**: enforce bounded timeouts in `src/setup-undici.ts`, add provider/tool-level retries & circuit breakers (`src/llm-client.ts`, `src/tools/rest-provider.ts`, `src/tools/mcp-provider.ts`), and avoid synchronous REST blocking by introducing async run IDs or streaming.
- **Shared tool queues** (`TODO-queue-concurrency.md`): replace per-session `maxConcurrentTools` with process-wide queues configured in `.ai-agent.json` (`queues.{name}.concurrent` + mandatory `default`). Only MCP/REST tools join queues; emit telemetry for depth/wait time and log when calls actually waited.
- **Shared MCP lifecycle** (`TODO-shared-mcp.md`): generalize pooling across transports, keep relentless restart loop with exponential backoff, probe health before restarting, detect idle exits via transport `onclose`, and ensure restart severity/log policy matches Costa’s directives.
- **Graceful shutdown** (`TODO-graceful-shutdown.md`): wire `ShutdownController` → headend manager → MCP registry; propagate `stopRef` to sessions; add watchdog + double-signal behavior.
- **Harness stability** (`TODO-harness-stability.md`): deterministic suite currently emits lingering-handle/persistence warnings post-shutdown refactor. Need cleanup of temp dirs, env layers, MCP fixtures, plus harness assertions that fail on regressions.

### Observability, Instrumentation, and UX
- **Structured logs inventory** (`TODO-structured-logs.md`): document every structured field, register missing journald `MESSAGE_ID`s, ensure severity policy (LLM/tool/session failures) matches Costa’s November directives, and confine “LLM response received” context to actual LLM events.
- **Instrumentation roadmap** (`TODO-INSTRUMENTATION.md`): converge logs/metrics/traces on shared resource attributes with opt-in OTLP export, journald detection heuristics, and consistent `log_uid`/label behavior across sinks. Ensure sinks degrade gracefully (systemd-cat helper, logfmt fallback) and telemetry stays opt-in.
- **Session visualization UI** (`TODO-UI.md`): design backend snapshot indexer + REST API + frontend (summary, cost, graph/timeline tabs) to explore `sessions/<txn>.json.gz`. Requires stack decision (reuse Express vs standalone), auth plan, and perf considerations (large trees, truncated payloads).

### Pricing, Dependencies, and External Research
- **OpenAI pricing table** (`TODO-openai-pricing.md`): confirm schema changes needed to encode multiple tiers (Batch/Flex/Standard/Priority) and non-token services in `neda/.ai-agent.json`; adjust runtime cost lookup accordingly.
- **Dependency refresh** (`TODO-update-dependencies.md`): upgrade AI SDK, MCP SDK, OTEL, ESLint, knip, ollama provider, etc., capturing release-note-driven code/doc updates and re-running lint/build/Phase 1.
- **Swarms comparison** (`TODO-swarms-comparison.md`): compile fact-based comparison (architecture, workflows, tooling, ops) between ai-agent and `kyegomez/swarms`; clarify deliverable format and evaluation weighting with Costa before drafting.

### Testing Strategy & Coverage
- **Phase 1 harness strategy** (`TODO-TEST-STRATEGY.md`): map each scenario to the eleven user-facing guarantees (LLM retries, tool usage, limits, context guard, accounting, etc.), identify redundant cases, and propose a behavior-driven suite while retaining determinism codes.
- **Phase 1 warnings cleanup** ties back to harness stability; once clean, consider gating CI on warning-free runs.

### Completed Items (for traceability)
- Provider architecture debt closed (Oct 2025): all provider-specific logic moved into hooks; no stringly `providerType` branches remain (see `TODO-DEBT.md`).
- Streaming regression fixed: SSE metadata capture no longer drains bodies prematurely (`coverage-openrouter-sse-nonblocking`).
- Silent-core compliance enforced: persistence/logging now flow through injected callbacks; core avoids direct stdio/fs writes.
- Final-report fallback scenario (`run-test-102`) validated; pending decision on richer diagnostics remains listed below.

## Decisions Needed (Awaiting Costa)
1. Deterministic chaining semantics: single `next` vs arrays, injected `reason` wording, downstream schema enforcement, and failure propagation strategy.
2. Code-review agents: should specialists lose filesystem/GitHub tools entirely or retain them with stricter guidance? Confirm required prompt placeholders (`${FORMAT}`, `${MAX_TURNS}`) scope.
3. Context guard messaging + estimator: keep current failure copy or adopt richer logs; confirm per-tool estimator behavior and removal of snapshot APIs.
4. Reasoning keyword semantics: does `unset` mean “disable” everywhere or only in frontmatter/config? Confirm override/default precedence matrix.
5. REST auth + include sandbox approach: API key vs mTLS vs proxying? Acceptable include root(s) and blocklist behavior.
6. Shared queue schema: confirm `.ai-agent.json` structure, default capacity formula, and migration messaging for removed `maxConcurrentTools`/`parallelToolCalls` knobs.
7. OpenAI pricing schema: multi-tier encoding + runtime selection + handling of non-token services.
8. Session UI scope: stack decision (reuse Express vs new service), auth expectations, initial feature set.
9. Swarms comparison deliverable format and evaluation weighting (architecture vs DX vs ops).
10. CI gating on warning-free Phase 1 harness: should harness noise become a failure once cleanup lands?

## Plan (Single Backlog)
1. **Architectural Core (Incremental Refactor Plan)**
   - **Seams First (Weeks 1–2)**
     - Introduce `TurnContext` struct to bundle turn parameters while still delegating to current logic.
     - Extract log-merging helper (`agent-log-utils.ts`) and tool-budget callback interface with no behavioral changes.
     - Add unit coverage for helpers and keep Phase 1 harness as guardrail.
   - **Split `executeAgentLoop` (Weeks 3–4)**
     - Move pre-turn setup into `prepareTurn`, provider/model retries into `runProviderCycle`, and final-report handling into `completeSession` (all private helpers inside `src/ai-agent.ts`).
     - Ensure instrumentation order stays identical; extend harness assertions if log ordering is touched.
   - **Tool Execution Manager (Weeks 5–6)**
     - Create shim class that wraps `ToolsOrchestrator.executeWithManagementInternal`, then migrate logging/telemetry scaffolding and per-session slot tracking into it.
     - Keep existing callbacks wired; add tests mocking the orchestrator.
   - **Loader/CLI Adapters (Weeks 7–8)**
     - Add `PromptRepository`, `ConfigService`, and `ApplicationHost` adapters that currently just forward to existing helpers, giving us seams for future splits.
     - Update DESIGN/IMPLEMENTATION docs to reflect the new adapters; run Phase 1 harness after each move.
2. **Reliability & Security** – Implement REST auth + include sandbox + timeout/retry/circuit breaker policies; wire graceful shutdown + shared queue manager + MCP lifecycle watchers; document new configs and add regression coverage (Phase 1 + targeted smoke tests).
3. **Execution Enhancements** – Deliver deterministic chaining MVP, context guard counter rewrite, reasoning semantics, and tool parameter normalization; expand harness scenarios for each.
4. **Observability & Instrumentation** – Finalize structured log schema inventory, register `MESSAGE_ID`s, extend telemetry (OTLP opt-in, queue metrics), and scope the session visualization UI (backend indexer + frontend prototype).
5. **Product & Docs Alignment** – Audit code-review prompts, update OpenAI pricing schema + config, execute dependency sweep, and produce the ai-agent vs swarms comparison once format is approved.
6. **Testing Infrastructure** – Clean harness shutdown warnings, publish the behavior-to-scenario matrix, and decide on CI gating for warning-free deterministic runs.

## Implied Decisions / Guardrails
- Provider hooks, streaming fix, and silent-core compliance are now immutable constraints; future refactors must preserve their interfaces and coverage.
- Shared queue manager + MCP registry become the authoritative concurrency controls; no reintroduction of per-session knobs.
- Structured logging severity policy (LLM/tool/session failures) is binding; deviations require explicit sign-off and harness updates.
- Deterministic harness remains the safety net; any feature touching orchestration/tooling/logging must add or update Phase 1 coverage.

## Testing Requirements
- Every architectural or tooling change: `npm run lint`, `npm run build`, `npm run test:phase1`; defer `npm run test:phase2 -- --tier=1` until post-refactor integration, but schedule before merging multi-stage work.
- Feature-specific additions (chaining, context guard, reasoning, queues, MCP lifecycle, pricing) need new deterministic scenarios and, when applicable, unit tests for extracted helpers.
- Harness stability work must assert zero lingering handles/temp-dir warnings; treat regressions as failures once cleanup lands.

## Documentation Updates Required
- `docs/AI-AGENT-GUIDE.md`, `docs/DESIGN.md`, `docs/IMPLEMENTATION.md`, `docs/TESTING.md`, and `README.md` must be edited whenever runtime behavior, defaults, schemas, or tooling change (per existing policy).
- Document new queue schema + reasoning semantics + context guard counters + structured log fields + REST auth requirements + pricing tiers + dependency versions.
- Session UI + swarms comparison deliverables should capture findings in repo docs if Costa wants them persisted (otherwise keep external but note in this backlog when delivered).
