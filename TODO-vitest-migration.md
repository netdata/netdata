# TL;DR
- Migrate the Phase 1/Phase 2 deterministic harnesses and any unit tests to Vitest so we can split the massive harness into modular files while keeping deterministic behaviour and existing CLI scripts.
- Work must keep the current deterministic guarantees (`npm run test:phase1` semantics, logging expectations) and align with the MAKE IT WORK FIRST manifesto: introduce Vitest scaffolding, then refactor tests in small, verifiable steps.

# Analysis
- Current scripts (`package.json`) run `npm run build` followed by `node dist/tests/phase1-harness.js` or `dist/tests/phase2-runner.js`; there is no standard test runner beyond these entrypoints and a small `unit/` folder executed via the harness itself.
- `src/tests/phase1-harness.ts` is a monolithic file (>9k LOC) that defines helpers, fixtures, deterministic LLM providers, and 100+ scenarios. Splitting it requires extracting shared fixtures/runner logic into reusable modules to avoid reinitialising globals.
- There is no Vitest/Jest tooling today; adopting Vitest requires new devDependencies, a `vitest.config.ts` mirroring `tsconfig` paths, and npm scripts for `vitest run`/`watch`. We also need to integrate coverage reporting (`@vitest/coverage-v8`) per repo quality standards.
- CI and documentation reference `npm run test:phase1` / `test:phase2`; we must preserve these commands (likely by keeping thin Node entrypoints that call the Vitest suites or by aliasing them to Vitest commands post-build).
- Deterministic harness order matters: scenarios rely on the shared `TEST_SCENARIOS` array and seeded randomness; splitting across files must retain import order determinism.

# Decisions Needed
1. Should the historical `npm run test:phase1` / `test:phase2` commands continue to run after Vitest adoption (wrapping Vitest) or can we replace them entirely with `vitest run`? (Default assumption: keep the existing commands for backwards compatibility.)
2. Do we want coverage reports (e.g., `vitest --coverage`) as part of CI immediately, or defer until after the migration lands?
3. Can we introduce a dedicated `tests/phase1/` directory that exports scenario groups (e.g., context guard, retries, queueing), with a single top-level spec orchestrating them, or do you prefer one spec per scenario?

# Plan
1. **Bootstrap Vitest**
   - Add devDependencies (`vitest`, `@vitest/coverage-v8`, optional `jsdom`), create `vitest.config.ts` sharing TS paths, and add npm scripts (`test`, `test:watch`, `test:unit`).
   - Keep existing `test:phase1` / `test:phase2` scripts initially; once Vitest specs exist we can point them at `vitest run`.
2. **Extract Harness Core**
   - Move shared helpers (deterministic LLM provider, fixture loader, registry setup) into `src/tests/phase1/core/` modules.
   - Provide a registration API so individual scenario files can call `registerScenario(...)` without touching global arrays directly.
3. **Split Scenarios**
   - Create per-topic spec files under `src/tests/phase1/scenarios/` (e.g., `context-guard.spec.ts`, `tool-failures.spec.ts`). Each spec imports the shared runner and registers its scenarios.
   - Ensure scenario order matches the current harness to avoid diff churn.
4. **Integrate Phase 2 & Unit Tests**
   - Wrap `phase2-runner` and any `unit/` suites in Vitest specs so all tests share the same runner.
   - Update npm scripts/CI to call `vitest run` (optionally with filters for Phase 1 vs Phase 2).
5. **Docs & CI**
   - Update `docs/TESTING.md`, README, and any developer guides to describe the new workflows (Vitest commands, how Phase 1 deterministic suite is structured).
6. **Validation**
   - Run `npm run lint`, `npm run build`, `vitest run` (or the updated scripts) to prove parity before merging.

# Implied Decisions
- The deterministic harness will remain TypeScript-first and continue to rely on the same fixtures; no behavioural changes are desired beyond file organisation.
- We will keep running `npm run build` before executing the harness unless instructed otherwise, to preserve the current `dist/tests/...` execution model.
- Splitting scenarios implies adjusting imports/exports in `src/tests/fixtures/test-llm-scenarios.ts`; we assume this is acceptable as long as external behaviour is identical.

# Testing Requirements
- `npm run lint`
- `npm run build`
- `vitest run` (or updated `npm run test:phase1` / `test:phase2`) covering all deterministic suites.

# Documentation Updates Required
- `docs/TESTING.md` (explain Vitest usage, scenario layout, and commands).
- README + any contributor docs referencing `npm run test:*` commands.
- Potentially `docs/AI-AGENT-GUIDE.md` if it documents testing workflows.

# Additional Constraints (Costa 2025-11-16)
- Every test/scenario must remain runnable independently; introducing Vitest must not create hidden coupling between spec files.
- Phase 2 deterministic tests stay isolated and are not run by default (to avoid provider cost). Provide dedicated commands to run them explicitly.
- Existing scenario scope stays intact: many tests intentionally cover multiple behaviors—do not split or shrink them, only reorganize file structure/tooling.
