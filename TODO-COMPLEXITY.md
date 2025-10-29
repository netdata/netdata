# TL;DR
- Target: eliminate 401 complexity violations so `eslint.complexity.config.mjs` returns zero errors.
- Hotspots span the agent core (`src/ai-agent.ts`), tool orchestration (`src/tools/**`), loaders, telemetry, CLI, and utilities; each requires structural refactors, not cosmetic tweaks.
- Awaiting design decisions on how to decompose `executeAgentLoop`, tool management, and loader/config flows before coding can begin.

# Analysis
## Lint baseline
- `./lint.sh` invokes `eslint.complexity.config.mjs` and currently reports 401 blocking errors (mix of cyclomatic complexity, max-depth, max-statements, max-lines-per-function).
- Parser complaints surface for `.d.ts` shims and certain `src/tests/**` files because the complexity config does not load the TS project; these must be addressed via ignores or by relocating non-source artifacts.

## Hotspot deep dives
- **`src/ai-agent.ts:1155 executeAgentLoop` (914 lines, complexity 249)**
  - Responsibilities: retry loop over provider/model pairs, telemetry/opTree wiring, accounting, tool planning, progress reporting, final report management, persistence, and error synthesis.
  - State coupling: touches ~40 private fields (telemetry labels, progress reporter, concurrency planning, txn metadata, child conversations, planned subturns, tool orchestrator). Multiple inline helper closures mutate shared arrays.
  - Nested control flow: triple-nested loops (turns → providers/models → tool batches) with numerous early `continue`/`break` branches, making straightforward extraction risky without a dedicated context object.
- **`src/tools/tools.ts:140 executeWithManagementInternal` (458 lines, complexity 69)**
  - Mixes concurrency slots, span attributes, session tree ops, telemetry metrics, accounting, provider dispatch, error shaping, retries, and result formatting.
  - Inline branches handle MCP vs. internal tools vs. sub-agents, leading to repeated condition checks and duplicated logging blocks.
- **`src/tools/internal-provider.ts:52 execute` (355 lines, complexity 59)**
  - Handles final-report tool invocation, report validation, formatting for multiple output targets (markdown, json, slack, tty, pipe, sub-agent), persistence hooks, and failure fallbacks.
  - Shares state with `AIAgentSession` via callbacks (accounting, opTree) without a clear intermediate abstraction.
- **`src/agent-loader.ts:128 constructLoadedAgent` (370 lines, complexity 78)**
  - Performs file IO, prompt parsing, frontmatter validation, defaults merging, env overlay injection, tool discovery, and registry registration in a linear, monolithic block.
- **Telemetry + CLI**
  - `src/telemetry/runtime-config.ts`, `src/telemetry/index.ts`, `src/cli.ts`, and headend runners each have 150–250 line functions combining option parsing, validation, and side-effect orchestration.

## Structural observations
- Core agent loop, tooling orchestrator, and loader each violate the single responsibility principle, blending orchestration, bookkeeping, and formatting.
- Shared mutable state (e.g., `this.opTree`, `this.plannedSubturns`, `toolSlotsInUse`) hampers naive extraction; helpers must receive explicit context objects to avoid hidden mutations.
- Telemetry and logging requirements mean any refactor must keep exact sequencing of `this.log`, `opTree` calls, and callback invocations.
- Existing helper methods (`logExit`, `pushSystemRetryMessage`, `sanitizeTurnMessages`) already hint at natural boundaries; we can extend this pattern rather than inventing entirely new paradigms.

## Constraints & risks
- Behavioural parity is mandatory; even minor changes to logging or accounting order could break tests or downstream consumers.
- Refactors must also satisfy `functional/no-loop-statements` where applicable; introducing loops may require justified rule disables.
- Tool concurrency and progress reporting rely on shared counters; moving logic into helpers without careful locking could introduce race conditions.
- Lint config treats some generated/test artifacts as first-class; need a strategy (ignore list vs. relocations) that keeps repo policy intact.
- Telemetry spans depend on execution order; reordering operations could degrade tracing fidelity unless spans are preserved.

# Decisions
1. Remediation will proceed sequentially: agent core → tool orchestration → loaders/config → telemetry/CLI.
2. Update `eslint.complexity.config.mjs` to ignore `.d.ts` and `src/tests/**`, documenting the rationale.
3. Introduce dedicated helper classes (`TurnLoopRunner`, `ToolExecutionManager`) with typed context structs; additional helpers may remain as pure functions where appropriate.
4. Encapsulate telemetry/logging via thin utilities that guarantee identical emission order.
5. Per-stage verification runs `npm run lint`, `npm run build`, and `npm run test:phase1`; Phase 2 suite defers until final integration pass.
6. Loader/config cleanup starts with incremental private helpers inside existing modules, promoting to separate files only after behaviour is stable.
7. Add lightweight unit tests for the new helper classes while continuing to rely on Phase 1/Phase 2 for end-to-end assurance.

# Plan
1. Draft detailed design for `TurnLoopRunner` (context shape, public API, interaction points with `AIAgentSession`, telemetry/log guarantees) and review with Costa.
2. Implement `TurnLoopRunner`, migrate `executeAgentLoop` to delegate, and add targeted unit tests + Phase 1 verification.
3. Extract `ToolExecutionManager` from `ToolsOrchestrator`, covering concurrency, telemetry, and accounting; add unit coverage and Phase 1 validation.
4. Refine `internal-provider` final-report handling to utilise the new tooling abstractions while preserving output semantics.
5. Refactor `agent-loader.ts` incrementally using private helpers for IO, frontmatter parsing, and env/default merging; revisit module splits after stabilisation.
6. Tackle telemetry/runtime and CLI functions, applying agreed helper utilities for logging/validation without reordering observable outputs.
7. Sweep remaining utilities for residual complexity breaches, adding helpers or module splits as needed.
8. After each stage, run `npm run lint`, `npm run build`, and `npm run test:phase1`; schedule a full `npm run test:phase2 -- --tier=1` once all refactors land.

# Implied decisions
- New helper modules/files likely required; naming/location conventions should align with existing folder structure (e.g., `core/`, `tools/`, `loaders/`).
- Additional tests may be necessary to cover extracted logic even if high-level behaviour is unchanged.
- Relocating inline lambdas to named helpers will adjust import structure; ensure tree-shaking/build impact is acceptable.
- Temporary feature flags/logging may be required to validate parity; must be removed once refactor stabilises.

# Testing requirements
- `npm run lint` and `npx eslint --config eslint.complexity.config.mjs` (or equivalent filtered command) must pass with zero errors.
- `npm run build` after each major refactor stage.
- `npm run test:phase1` after each stage; defer `npm run test:phase2 -- --tier=1` until the final integration pass.
- Targeted unit/integration tests where functions gain new helper boundaries (e.g., `TurnLoopRunner`, `ToolExecutionManager`).

# Documentation updates required
- Update relevant TODO/technical debt docs (e.g., `TODO-DEBT.md`, `docs/IMPLEMENTATION.md` or new log/event docs) with refactoring strategy and outcomes.
- Document new helper modules/context structs, especially around agent execution flow and telemetry hooks.
- Note complexity lint expectations and how to run the dedicated check in developer docs (`LINTING.md`).
