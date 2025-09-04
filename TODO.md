# Architecture and Implementation TODOs

This document collects findings from recent reviews (e.g., Kimi K2) and open questions for alignment before further changes. No code changes are made from this file.

## 1) Architecture Target (Decision Needed)
- Problem: Dual influence from original Claude design and MULTI-AGENT.md; current code implements multi‑agent via sub‑agent tools but retains hybrid aspects.
- Decision: Confirm the canonical target:
  - Option A: Claude session core + sub‑agent tool model (current path).
  - Option B: Full recursive multi‑agent per MULTI-AGENT.md.
- Why it matters: Dictates prompt/loop structure, error policy, extensibility hooks, and documentation.

## 2) Provider Abstraction & Accounting
- Finding: Provider‑specific usage/cost fields leak (e.g., OpenRouter specific cost fields). Unified `LLMAccountingEntry` exists but population varies by provider.
- Impact: Downstream consumers must branch per provider; inconsistent analytics.
- Actions:
  - Add a provider adapter layer in `llm-client` to normalize usage/costs into `LLMAccountingEntry` consistently.
  - Ensure optional fields are present (or null/undefined) uniformly; document per‑provider support.
  - Add tests/fixtures per provider for accounting mapping.

## 3) Tool Instructions & Independence
- Finding: MCP instructions are injected once per session; per‑tool context is not injected (beyond schema validation at execution).
- Impact: LLM lacks granular per‑tool guidance; less control/traceability.
- Actions:
  - Introduce PromptBuilder with strategies:
    - Global‑only (current),
    - Global + per‑tool stubs (name + 1‑line description),
    - Demand‑driven (inject per call; optional).
  - Start with “Global + per‑tool stubs” to avoid prompt bloat; feature‑flag the strategy.

## 4) Session Management & Boundaries
- Finding: Library‑first intent is correct in structure (`AIAgentSession`, `llm-client`, `mcp-client`) but boundaries and lifecycle are not documented.
- Impact: Unclear responsibilities between library and CLI; harder to embed programmatically.
- Actions:
  - Document a public library API (create → run → teardown), responsibilities, and examples (no CLI).
  - Clarify what the CLI adds (I/O, help, persistence, grouping output).

## 5) File Structure & Maintainability
- Finding: `ai-agent.ts` is monolithic (>1100 LOC) with multiple concerns (prompt shaping, loop control, batch tool, final report, logging/accounting).
- Impact: Hard to modify/extend; increases coupling.
- Actions (refactor plan): Extract modules
  - PromptBuilder (system + MCP + internal tools instructions)
  - LoopRunner (turn/retry orchestration)
  - ToolExecutor (includes batch tool)
  - FinalReportHandler
  - AccountingLogger (format and emit accounting/logs)
  - Keep existing public types/APIs stable.

## 6) Dependency Boundaries (AI SDK vs MCP)
- Finding: Separation exists (`llm-client` vs `mcp-client`) but session reaches into both; sub‑agent registry couples to session config.
- Impact: Risk of dependency creep; harder to reason about failures.
- Actions:
  - Define interface boundaries:
    - LLM: submitTurn, stream, standard error mapping.
    - MCP: discover tools, get schema, execute tool, standard error mapping.
  - Ensure `AIAgentSession` uses only these interfaces (no internals).

## 7) Extensibility Hooks
- Finding: Only `AIAgentCallbacks` exist (onLog/onOutput/onThinking/onAccounting). No lifecycle hooks or strategy injection beyond provider list.
- Impact: Hard to customize policies/telemetry.
- Actions:
  - Add non‑breaking hooks: beforeTurn/afterTurn, beforeTool/afterTool.
  - Pluggable strategies: provider/model selection, tool invocation policy.

## 8) Error Handling & Retry Policy
- Finding: Many exit codes defined, but error mapping + retry policy logic is spread across the loop and providers.
- Impact: Unclear recovery behavior; inconsistent messages; brittle string matching.
- Actions:
  - Introduce a unified `AgentError` type `{ code, message, retryable }`.
  - Centralize retry policy in a small module; map provider/MCP errors into `AgentError`.
  - Replace string checks with error codes; document defaults (per‑turn retry across pairs, caps).

## 9) MCP Compliance & Tool Limits
- Finding: Tool schemas validated; instruction injection is global; limits exist but need clearer surfacing.
- Impact: Ambiguity on compliance/mapping; limits enforcement may vary in UX.
- Actions:
  - Document MCP mapping: discovery, validation, execution, error mapping.
  - Confirm and document `toolResponseMaxBytes` enforcement and error surfaces (standardized error code/message).

## 10) Configuration Simplification & Naming
- Progress: Frontmatter now agent‑only; runtime controls are CLI/config; grouped help.
- Remaining Actions:
  - Document canonical mapping (kebab on CLI ↔ camelCase in configs/frontmatter).
  - Remove stale aliases in docs; keep only the canonical names.

## 11) Open Questions (for alignment)
- Architecture target: confirm “session core + sub‑agent tool model” as canonical (vs. alternate multi‑agent orchestration).
- Per‑tool instructions: inject lightweight per‑tool stubs in system prompt? Default on/off?
- Retry defaults: keep per‑turn across provider/model pairs? Any conditions to spill across turns?
- Library‑first API: formalize a minimal public API package (e.g., `@ai-agent/core`) and document usage.
- Save‑all semantics: current `--save-all` names files `<selfId>__<agent>.json` under `<originId>/`; any changes to structure or metadata?

## 12) Suggested Next Steps (no code yet)
- Decide architecture target (1).
- Approve error handling unification (8) and provider accounting normalization (2).
- Approve PromptBuilder strategy for per‑tool stubs (3).
- Approve initial refactor boundaries (5) and minimal lifecycle hooks (7).
- Then implement in small, reviewable PRs, updating docs after each step.


---

# Grok Review – Merged Additions

This section integrates Grok’s phased roadmap into the plan above. Items are de-duplicated where they overlap; otherwise they’re recorded verbatim for tracking.

## Phase 1: Critical Fixes (Immediate)

1) Session Isolation Violations
- Clarification: There is no global AgentRegistry singleton; isolation risk stems primarily from process.chdir() during sub-agent loading and shared process.env usage.
- Actions:
  - Remove all global process.chdir(); pass explicit baseDir everywhere (loader, resolver, MCP/FS access).
  - Implement per-session env overlays in config-resolver (do not rely on process.env for placeholders during a session run).
  - Add session-scoped caching for loaded agents (thread a registry through the session; no cross-session cache).
  - Add concurrent session tests to verify isolation.

2) Refactor turn execution (aka executeSingleTurn())
- Actions:
  - Extract tool execution path into a dedicated method/module.
  - Extract response processing and final answer detection.
  - Keep glue/controller method < 50 LOC; unit test the extracted pieces.

3) Standardize Error Handling
- Actions:
  - Define AgentError (code, message, retryable, optional cause/context).
  - Map provider/MCP errors to AgentError in adapters.
  - Standardize log error format and fields; propagate context.
  - Define recovery strategies (retry policy module) and apply consistently.

## Phase 2: Architecture Improvements

4) Eliminate Code Duplication
- Actions:
  - Extract common configuration/defaults/frontmatter merging logic from agent-loader.
  - Create unified prompt processing pipeline.
  - DRY validation logic and shared types.

5) Fix Type Safety Issues
- Actions:
  - Replace “as unknown as” with type guards.
  - Enforce strict null checks across boundaries.
  - Strengthen provider/MCP types; minimize assertions.

6) Improve Multi-Agent Architecture
- Current: recursion detection present in sub-agent registry; maintain it.
- Actions:
  - Add per-child resource budgeting (turns/tokens/time).
  - Improve structured error propagation from sub-agents (AgentError mapping + context).
  - Add optional sub-agent result validation hooks when contracts are known.

## Phase 3: Quality & Maintainability

7) Standardize Naming Conventions
- Actions:
  - kebab-case filenames; camelCase variables; PascalCase classes (lint rules to enforce).

8) Improve Configuration Management
- Actions:
  - Simplify precedence logic and document it (we already print resolution order in --help).
  - Add schemas for configuration validation and clearer error messages.

9) Enhance Logging & Observability
- Actions:
  - Standardize log envelope (fields and levels).
  - Add log-level filtering and performance metrics (per turn and per session).

## Phase 4: Testing & Documentation

10) Add Comprehensive Testing
- Actions:
  - Unit tests for resolver, loader, provider adapters, session loop (simple cases), sub-agent execution path.
  - Integration tests for multi-agent scenarios; add performance/chaos tests later.

11) Improve Documentation
- Actions:
  - Update architecture doc with canonical design (session + sub-agent tools), lifecycle, and boundaries.
  - Add public API usage for library-first consumers.
  - Troubleshooting and performance tuning guides.

## Success Metrics (from Grok)
- Zero linting errors under strict configuration.
- 100% type safety: no “as unknown as” patterns; guarded external inputs.
- Verified concurrent session isolation (no cwd/env bleed, deterministic overlays).
- Single responsibility: controller methods < 50 LOC with extracted units.
- DRY: duplicated logic removed via shared utilities.
- Consistent error handling with AgentError across all components.
- Test coverage on critical paths (unit + integration) with passing runs.

## Risk Mitigation (from Grok)
1) Incremental changes with small, testable PRs.
2) API stability during refactors (name any breaking changes explicitly).
3) Add/expand tests before refactoring core flows.
4) Mandatory reviews for architectural changes.
5) Keep docs updated alongside code changes.


---

# Claude Review – Merged Additions

This section integrates Claude’s findings. Items overlapping earlier sections are noted; otherwise added for tracking.

## Critical Issues (Immediate)

1) Complete Environment Isolation Migration
- Findings: Direct process.env access remains in multiple places (config.ts expand env, config-resolver placeholders, provider headers, scattered DEBUG checks).
- Impact: Violates session isolation; risk of cross-talk under parallel sub-agents.
- Actions:
  - Implement per-session env overlays in resolver and provider adapters; eliminate ambient process.env reads during a session.
  - Thread overlays via loader/session; document precedence.

2) Process Directory Mutation Risk
- Findings: subagent-registry uses process.chdir() during child load.
- Impact: Not thread-safe; breaks parallelism.
- Actions:
  - Pass explicit baseDir/context everywhere (resolver, loader, FS access);
  - Remove all global CWD mutations.

3) Recursion Depth Tracking
- Findings: Registry blocks declared sub-agents-of-sub-agents, but indirect deep recursion can still occur.
- Actions:
  - Add MAX_RECURSION_DEPTH guard (2–3) and include callPath depth checks during execution.

## Architecture Inconsistencies (Short Term)

1) Duplicate Frontmatter Interface
- Issue: FrontmatterOptions declared in cli.ts and frontmatter.ts.
- Actions: Single source of truth (frontmatter.ts), import type in CLI.

2) Tool Naming Prefixes
- Issue: Both internal tools and sub-agent tools use agent__ prefix.
- Actions: Reserve internal__ for built-ins; agent__/subagent__ for sub-agents; enforce in registry and internal tools.

3) Concurrency Control for Sub-Agents
- Issue: No bounded parallelism.
- Actions: Add concurrency gate (configurable, default 2–4), apply to sub-agent executions.

4) Error Handling Consistency
- Issue: Mixed patterns (generic rethrows vs raw propagation).
- Actions: Adopt AgentError with preserved context/cause across boundaries and standard logging.

## Code Quality (Mid Term)

1) Large Function Bodies
- Files: ai-agent.ts (turn execution), agent-loader.ts (loadAgent variants).
- Actions: Extract units (tool exec, response processing, final detection), keep controller methods small.

2) Magic Values and Defaults
- Issue: Prefixes and defaults scattered.
- Actions: Centralize constants and default knobs.

3) Budget Enforcement
- Issue: Token/time budgets not implemented (per-child/global).
- Actions: Add budgeting and guards; expose configurable caps.

4) Path Resolution Consistency
- Issue: Mixed cwd/relative/absolute; duplicated canonical helpers.
- Actions: Centralize path utilities; avoid cwd reliance.

## Minor (Ongoing)

1) Registry Pattern
- Note: Not a singleton; keep session-scoped cache, avoid cross-session caching.

2) Debug Logging Configuration
- Actions: Replace scattered env checks with centralized logger + config.

3) Validation Gaps
- Actions: Add optional sub-agent output validation; standardize tool result truncation.

4) Documentation Gaps
- Actions: Document internal tools (e.g., batch), finalization flow, MCP mapping.

## Priorities Recap
- P0: env overlays + remove chdir + depth cap.
- P1: refactor loop + AgentError + constants + path utils.
- P2: sub-agent concurrency + budgets + dedupe loader + unify frontmatter types.
- P3: logging schema + metrics + output validation + docs.


---

# Codex Review – Merged Additions

This section integrates Codex’s review with explicit acceptance criteria and priorities.

## Critical (High Priority)

1) Concurrency-safe sub-agent loading
- Findings: subagent-registry uses process.chdir(); parallelToolCalls may overlap.
- Acceptance Criteria:
  - No process.chdir() anywhere in sub-agent execution path.
  - Resolver/loader accept baseDir; all file accesses/path resolves are absolute.
  - parallelToolCalls enabled does not affect config discovery or relative paths.

2) Per-session environment overlays
- Findings: config-resolver falls back to process.env; openrouter reads process.env.* directly; DEBUG checks scattered.
- Acceptance Criteria:
  - Resolver accepts a session env map; no ambient process.env reads during a session run.
  - Providers (e.g., OpenRouter) read referer/title/headers from config/overlay, not process.env.
  - Centralized debug config replaces scattered env checks.

## Medium Priority

3) Loader deduplication and pruning legacy config
- Findings: loadAgent vs loadAgentFromContent duplicated logic; config.ts legacy loader unused.
- Acceptance Criteria:
  - Shared internal helper computes selected targets/tools/agents, resolved config, and effective runtime knobs.
  - loadAgent/loadAgentFromContent wrap the helper; remove or clearly deprecate legacy loader in config.ts.

4) Retry and error policy unification
- Findings: Mixed error propagation; string heuristics in loop.
- Acceptance Criteria:
  - Introduce AgentError { code, message, retryable, cause? }.
  - Map provider/MCP errors to AgentError; replace string checks with codes.
  - Standardize error logs with consistent fields.

5) Utilities consolidation
- Findings: utils.ts mixes concerns; duplicate provider helpers.
- Acceptance Criteria:
  - Split formatting/tool-format/guards modules.
  - Centralize isPlainObject/deepMerge helpers for providers.

## Low Priority

6) Config/schema and docs cleanup
- Findings: extra defaults (maxParallelTools, maxConcurrentTools) in sample configs; provider env fields in process.env.
- Acceptance Criteria:
  - Remove or document unsupported keys in samples.
  - Add provider headers (e.g., OpenRouter referer/title) to config; document overlay usage.

## Decisions Needed

- Final report strictness:
  - Option A: Warn and complete (current).
  - Option B: Hard fail on format/schema mismatch.
- Retry API exposure:
  - Option A: Keep AIAgentSession.retry() public.
  - Option B: Make internal-only (to avoid external misuse with shared clients).

## Notes
- These items align with existing P0–P3 in this TODO and with Kimi/Grok reviews.
- Implementation should be incremental with tests accompanying each refactor.


---

# Gemini Review – Merged Additions

This section integrates Gemini’s findings with acceptance criteria and priority tags.

## P0 – Correctness / Architecture Alignment

1) Session model vs docs (immutable sessions)
- Finding: Current `AIAgentSession` is stateful and long‑lived; docs suggest stateless factory creating fresh, immutable session objects per run.
- Decision/Acceptance:
  - Option A: Refactor to stateless factory returning a new stateful session per run; make `retry()` internal or return a fresh session.
  - Option B: Keep current stateful session but document lifecycle and discourage reuse; `retry()` remains internal‑only.
- Done when: Decision recorded; docs updated; API surface aligned; tests verify session isolation with concurrent runs.

2) Runtime configuration validation
- Finding: Active path `config-resolver.ts` isn’t validated against a schema; legacy `config.ts` has zod schema but is not used.
- Acceptance:
  - Validate the unified, resolved configuration with zod in one place (post‑resolver).
  - Errors reference file path, offending key, expected vs actual type.
  - Docs include a short “invalid config” troubleshooting section.

## P1 – Maintainability / Consistency

3) Mixed concerns in ai-agent.ts
- Finding: Prompt shaping, loop orchestration, tool execution, validation, accounting, and logging all interleaved.
- Acceptance:
  - Extract modules: PromptBuilder, ToolExecutor, ResponseProcessor/Finalization, ErrorPolicy.
  - Controller/glue methods kept < 50 LOC; unit tests added for extracted units.

4) Error handling standardization
- Finding: Mixed return vs throw; unpredictable surface for consumers.
- Acceptance:
  - Introduce `AgentError { code, message, retryable, cause? }` and map provider/MCP errors to it.
  - Public APIs return structured results (`{ success: boolean, error?: string }`) for predictable failures; internal layers can throw and be mapped at boundaries.
  - Logs use a consistent error envelope with context.

## P2 – Duplication / Extensibility

5) LLM provider duplication
- Finding: Streaming, tool wiring, error formatting repeated across providers.
- Acceptance:
  - Enhance BaseLLMProvider with shared streaming, tool message building, and error mapping.
  - Provider classes override only client init and request specifics; shared helpers in a central module.

6) Multi‑agent architecture completeness
- Finding: Sub‑agent registry is integrated, but isolation/concurrency/budgets need completion.
- Acceptance:
  - Concurrency‑safe sub‑agent loading (no `process.chdir`, baseDir everywhere).
  - Per‑session env overlays; no `process.env` reads during session.
  - Bounded sub‑agent concurrency (configurable default 2–4) and resource budgets (turns/tokens/time) enforced.
  - Recursion depth guard and clear error if exceeded.
  - Docs describe the canonical “sub‑agent tool” model and constraints.

## P3 – Versioning / Docs

7) Vercel AI SDK version alignment
- Finding: Docs mention “v5”; code may use older APIs.
- Acceptance:
  - Verify installed SDK versions; align code and docs with the chosen version.
  - Note any intentional deviations and why.

8) Documentation sync
- Acceptance:
  - Architecture/lifecycle updated; error/retry policy documented.
  - Internal tools and finalization flow described with examples.

Notes
- These items align with previously merged TODO entries (Kimi/Grok/Codex). Implementation should proceed incrementally with tests.
