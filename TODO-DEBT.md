# Technical Debt Backlog (Unified)

_Last updated: 2025-10-17_

## TL;DR
- Streaming is now non-blocking via traced-fetch updates; keep the coverage guard (`coverage-openrouter-sse-nonblocking`) whenever metadata capture changes.
- The core emits a synthesized final report if the model never produces one; harness `run-test-102` now covers the tool-failure branch, leaving only the question of whether fallback messaging needs richer provider diagnostics.
- Reliability gaps: no transport-level retries/circuit breakers, limited observability, and automated coverage stops at the phase1 harness (no provider/rest e2e tests or CI gating). Expanded coverage remains an active workstream.

## P0 — Critical (breaks core behavior or architecture)
- **Streaming regression** — `src/llm-client.ts:199-338` eagerly reads cloned JSON/SSE bodies inside `createTracedFetch`, stalling tokens and doubling bandwidth. _Fix_: capture routing/cost metadata without draining the live stream (inspect headers, tee the body). _Impact_: restores real-time streaming for CLI/headend users. _Status_: resolved (Oct 2025) via non-blocking SSE handling and regression test `coverage-openrouter-sse-nonblocking`.
- **Library side-effects** — `src/ai-agent.ts:245-263` persists snapshots via sync fs writes; `src/ai-agent.ts:963-977` appends accounting JSONL; `src/utils.ts:249-258` writes warnings to stderr. Violates DESIGN.md’s silent-core principle and blocks the event loop. _Fix_: move persistence/logging behind CLI callbacks, make storage async/optional, and ensure the core never touches stdio/files directly. _Status_: resolved (Oct 2025); persistence now flows through async callbacks and all core warnings route via the injectable sink.

## P1 — High Priority (unblocks resilience & maintainability)
- **Session + provider decomposition** — `AIAgentSession` (~2.2 k LOC) and `BaseLLMProvider` (~1.3 k LOC) remain monolithic, making localized fixes risky. _Fix_: split session orchestration, persistence, tool management, and provider streaming/error mapping into focused modules (e.g., `SessionRunner`, `ToolManager`, `StreamHandler`).
- **Lack of transport retries/circuit breakers** — MCP servers and LLM transports have no retry/backoff beyond the high-level loop. Add provider/tool-level retry policies with exponential backoff, jitter, and circuit breakers for flaky endpoints.
- **Observability gaps** — Structured logs exist but we lack correlation IDs, metrics, or traces. Add OpenTelemetry spans, Prometheus counters (tokens, latency, tool failures), and enrich logs with consistent identifiers.
- **Limited automated coverage** — The deterministic phase1 harness (including the new streaming/persistence/final_report scenarios) exists, but we still lack provider-specific unit tests, rest-provider coverage, and CI gating for regressions. Stand up Vitest (or Jest) with broader suites, add mocks for adapters, and enforce coverage in CI before major refactors.
- **Final-report fallback follow-ups** — Harness scenario `run-test-102` now exercises the branch where `agent__final_report` rejects; still decide whether the synthesized failure summary should enumerate attempted providers/models before declaring failure.
- **Concurrency limiter duplication** — Tool execution throttling logic lives in both `AIAgentSession` and `ToolsOrchestrator`; a similar limiter exists in `src/headends/concurrency.ts`. Centralize into a reusable semaphore.
- **Loader duplication** — `loadAgent` vs `loadAgentFromContent` in `src/agent-loader.ts` repeat the same resolution/validation logic. Extract shared helpers to reduce churn and bug risk.
## P2 — Medium Priority (nice-to-have but non-blocking)
- **Token accounting scatter** — Accounting is emitted from multiple layers (`ai-agent`, tool orchestrator, providers). Consolidate via a dedicated accounting service to avoid drift.
- **Magic strings / enums** — Exit codes and log identifiers are free-form strings. Promote to enums/const maps to avoid typos and ease refactors.
- **Capabilities discovery for tools** — Expose provider capabilities (`supportsBatch`, `maxConcurrency`, `defaultTimeout`) so planners can make informed choices.
- **Memory management** — Guard against unbounded conversation/tool payload growth (LRU trimming, response caps, MCP cleanup hooks).
- **Security hardening** — Centralize secret redaction, tighten input validation (tool params, REST payloads), and validate env overrides at startup.
- **Documentation gaps** — Add JSDoc/TypeDoc for public surfaces, contributor runbook, and migration guides for prompt/frontmatter changes.

## Suggested Execution Order
1. **Restore core expectations**: streaming fix, silent-core compliance, relax final-report requirement, split AIAgentSession hotspots (~1 sprint).
2. **Stabilize infrastructure**: introduce retries/circuit breakers, structured metrics/traces, shared concurrency/token services, automate CI with initial tests (~1 sprint).
3. **Refactor providers/tools**: decompose `BaseLLMProvider`, DRY loader + error handling, add tool capability metadata (~1–2 sprints depending on scope).
4. **Polish**: memory/security hardening, doc expansion, dependency hygiene (Dependabot, audits), bundle trimming.

## Cross-References
- Historical audits: see `DEBT-GEMINI.md` (design/maintainability focus) and `DEBT-CLAUDE.md` (provider split, resilience, observability). Their open items are folded above; retire those files once this backlog is the source of truth.
