# Technical Debt Backlog (Unified)

_Last updated: 2025-10-14_

## TL;DR
- Streaming is broken by our traced fetch wrapper; fix it first to restore realtime output.
- Core library violates the “silent core” contract (sync FS + stderr writes) and the agent loop can no longer finish without calling `agent__final_report`.
- `AIAgentSession` and `BaseLLMProvider` are monolithic and need to be split; we also duplicate loader/concurrency logic.
- Reliability gaps: no transport-level retries/circuit breakers, limited observability, and automated coverage stops at the phase1 harness (no provider/rest e2e tests or CI gating). Expanded coverage remains an active workstream.

## P0 — Critical (breaks core behavior or architecture)
- **Streaming regression** — `src/llm-client.ts:199-338` eagerly reads cloned JSON/SSE bodies inside `createTracedFetch`. Streaming stalls until the full response downloads and doubles bandwidth. _Fix_: capture routing/cost metadata without consuming the live body (inspect headers, wrap the stream, or buffer _after_ the provider completes).
- **Library side-effects** — `src/ai-agent.ts:245-263` persists snapshots via sync fs writes; `src/ai-agent.ts:963-977` appends accounting JSONL; `src/utils.ts:249-258` writes warnings to stderr. Violates DESIGN.md’s silent-core principle and blocks the event loop. _Fix_: move persistence/logging behind CLI callbacks and make storage async/optional.
- **Mandatory `agent__final_report`** — `src/ai-agent.ts:1261-1288` treats plain assistant answers as invalid unless the internal tool fires. Vanilla prompts now fail. _Fix_: accept text completions unless prompts opt into the stricter contract.
- **`AIAgentSession` god object** — `src/ai-agent.ts` (~2.2 k LOC) mixes session orchestration, persistence, progress, logging, tool management. Break into dedicated modules (`SessionRunner`, `SessionStore`, `ToolManager`, `ProgressReporter`, etc.) per DESIGN.md.
- **`BaseLLMProvider` monolith** — `src/llm-providers/base.ts` (~1.3 k LOC) blends streaming, error mapping, format policy, token extraction. Split into focused components (stream handler, error mapper, message converter, token usage extractor) to reduce risk and enable targeted tests.

## P1 — High Priority (unblocks resilience & maintainability)
- **Lack of transport retries/circuit breakers** — MCP servers and LLM transports have no retry/backoff beyond the high-level loop. Add provider/tool-level retry policies with exponential backoff, jitter, and circuit breakers for flaky endpoints.
- **Observability gaps** — Structured logs exist but we lack correlation IDs, metrics, or traces. Add OpenTelemetry spans, Prometheus counters (tokens, latency, tool failures), and enrich logs with consistent identifiers.
- **Limited automated coverage** — The deterministic phase1 harness (including the new streaming/persistence/final_report scenarios) exists, but we still lack provider-specific unit tests, rest-provider coverage, and CI gating for regressions. Stand up Vitest (or Jest) with broader suites, add mocks for adapters, and enforce coverage in CI before major refactors.
- **Concurrency limiter duplication** — Tool execution throttling logic lives in both `AIAgentSession` and `ToolsOrchestrator`; a similar limiter exists in `src/headends/concurrency.ts`. Centralize into a reusable semaphore.
- **Loader duplication** — `loadAgent` vs `loadAgentFromContent` in `src/agent-loader.ts` repeat the same resolution/validation logic. Extract shared helpers to reduce churn and bug risk.
- **Noisy diagnostics** — `src/llm-client.ts:220-253` prints warnings via `console.error` even with tracing disabled. Route through structured logging and guard behind trace flags.
- **Product-specific alias** — `src/tools/tools.ts:12-15` hard-codes `netdata → rest__ask-netdata`. Move to configuration/frontmatter to keep the core generic.

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
