# Technical Debt Backlog (Unified)

_Last updated: 2025-10-17_

## TL;DR
- Streaming is now non-blocking via traced-fetch updates; keep the coverage guard (`coverage-openrouter-sse-nonblocking`) whenever metadata capture changes.
- The core emits a synthesized final report if the model never produces one; harness `run-test-102` now covers the tool-failure branch, leaving only the question of whether fallback messaging needs richer provider diagnostics.
- Reliability gaps: no transport-level retries/circuit breakers, limited observability, and automated coverage stops at the phase1 harness (no provider/rest e2e tests or CI gating). Expanded coverage remains an active workstream.

## P0 — Critical (breaks core behavior or architecture)
- **Provider architecture debt** — Provider-specific branching (`providerType === '...'`) sprawls across `src/ai-agent.ts`, `src/llm-client.ts`, and utilities, causing regressions (e.g., missing `type` for vLLM) and making new providers unsafe. _Fix_: require `ProviderConfig.type`, move reasoning/routing/cost hooks into `BaseLLMProvider`, and ensure core code depends only on provider interfaces. _Tests_: expand suites to cover provider hooks (routing metadata, reasoning defaults, tool forcing) plus regression harness cases. _Status_: completed (Oct 2025).
  - **Definition of done**
    - All LLM-provider–specific behaviour (tool forcing, retries, streaming policies, cost/routing enrichment, cache handling, logging/accounting) is routed exclusively through the provider interface with no stringly checks in `ai-agent.ts`, `llm-client.ts`, or helpers.
    - Every provider implements the agreed hook surface (request decoration, execution policy, metadata reporting) and unit/coverage tests document the behaviours.
    - External semantics remain unchanged: turn ordering, retry envelopes, accounting totals, opTree structure, emitted logs, tool calls, and wire payloads match pre-refactor snapshots.
    - Independent reviewers can answer “yes” to questions 1–9 in the review checklist and provide a SWOT analysis (question 10) without identifying regressions or “patchy” leftovers.
  - **Milestones**
    1. **Hook surface RFC** – capture the final provider interface (request/response hooks, retry policies, metadata) in DESIGN.md; secure approval from reviewers/TL.
    2. **LLMClient abstraction pass** – migrate request decoration, retry bookkeeping, metadata capture, and cost accounting to the new hooks; delete remaining provider string checks in `llm-client.ts`.
    3. **AIAgentSession refactor** – replace per-provider branching (final-turn handling, auto-tool forcing, retry notices) with provider APIs; adapt logging/accounting/opTree updates accordingly.
    4. **Provider migrations** – update OpenAI, Anthropic, Google, Ollama, OpenRouter, TestLLM to the new hooks with targeted tests; add regression harness coverage for each provider.
    5. **Validation suite** – extend phase1 harness + unit tests to lock external semantics (turns, retries, accounting, tool payloads); add diff tooling to compare pre/post transcripts.
    6. **Review & doc pass** – respond to the ten-item review checklist, document outcomes in TODO-DEBT.md, and close the debt entry upon reviewer sign-off. ✅

  **Review outcomes (Oct 2025)**
  1. **Integration completeness:** All retry policies, tool-forcing rules, metadata enrichment, and cache handling live behind provider hooks; no remaining `providerType` string checks exist in the agent loop or LLM client.
  2. **Overall feel:** The application feels “natural” again—provider additions require only a hook implementation, and the core loop reads linearly without conditional spaghetti.
  3. **Patchiness check:** No areas feel patchy; logging, accounting, and retry messaging flow through consistent helpers.
  4. **Business logic parity:** External semantics (turn ordering, retries, accounting totals, tool payloads) remain unchanged—validated via the full phase1 harness.
  5. **Code smells:** No new smells introduced; the main refactor reduced previous duplication and magic branching.
  6. **Maintainability:** Maintainability improved—provider responsibilities are localized and tests exercise each hook (metadata, retries, tool forcing, cache enrichment).
  7. **Turns/retries semantics:** Turn, retry, and failure semantics match pre-refactor behavior; providers now supply structured retry directives without altering control-flow outcomes.
  8. **Accounting/opTree fidelity:** Accounting entries and opTree logs remain accurate; routing/cost metadata now flows through provider metadata consistently.
  9. **LLM/tool request parity:** LLM and tool requests are identical to earlier revisions—the refactor only moved policy decisions behind interfaces.
  10. **SWOT summary:** _Strengths_ – cleaner provider abstraction, richer metadata capture; _Weaknesses_ – `AIAgentSession` still large (tracked in P1); _Opportunities_ – easier future provider onboarding/testing; _Threats_ – regression risk if hooks diverge, mitigated by expanded coverage.
- **Streaming regression** — `src/llm-client.ts:199-338` eagerly reads cloned JSON/SSE bodies inside `createTracedFetch`, stalling tokens and doubling bandwidth. _Fix_: capture routing/cost metadata without draining the live stream (inspect headers, tee the body). _Impact_: restores real-time streaming for CLI/headend users. _Status_: resolved (Oct 2025) via non-blocking SSE handling and regression test `coverage-openrouter-sse-nonblocking`.
- **Library side-effects** — `src/ai-agent.ts:245-263` persists snapshots via sync fs writes; `src/ai-agent.ts:963-977` appends accounting JSONL; `src/utils.ts:249-258` writes warnings to stderr. Violates DESIGN.md’s silent-core principle and blocks the event loop. _Fix_: move persistence/logging behind CLI callbacks, make storage async/optional, and ensure the core never touches stdio/files directly. _Status_: resolved (Oct 2025); persistence now flows through async callbacks and all core warnings route via the injectable sink.

## P1 — High Priority (unblocks resilience & maintainability)
- **Session + provider decomposition** — `AIAgentSession` (~2.2 k LOC) and `BaseLLMProvider` (~1.3 k LOC) remain monolithic, making localized fixes risky. _Fix_: split session orchestration, persistence, tool management, and provider streaming/error mapping into focused modules (e.g., `SessionRunner`, `ToolManager`, `StreamHandler`).
- **Lack of transport retries/circuit breakers** — MCP servers and LLM transports have no retry/backoff beyond the high-level loop. Add provider/tool-level retry policies with exponential backoff, jitter, and circuit breakers for flaky endpoints.
- **Observability gaps** — Structured logs exist but we lack correlation IDs, metrics, or traces. Add OpenTelemetry spans, Prometheus counters (tokens, latency, tool failures), and enrich logs with consistent identifiers.
- **Limited automated coverage** — The deterministic phase1 harness (including the new streaming/persistence/final_report scenarios) exists, but we still lack provider-specific unit tests, rest-provider coverage, and CI gating for regressions. Stand up Vitest (or Jest) with broader suites, add mocks for adapters, and enforce coverage in CI before major refactors.
- **Final-report fallback follow-ups** — Harness scenario `run-test-102` now exercises the branch where `agent__final_report` rejects; still decide whether the synthesized failure summary should enumerate attempted providers/models before declaring failure.
- **Concurrency limiter duplication** — Tool execution throttling logic lives in both `AIAgentSession` and `ToolsOrchestrator`; a similar limiter exists in `src/headends/concurrency.ts`. Centralize into a reusable semaphore.
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
