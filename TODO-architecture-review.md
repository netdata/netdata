# TODO - Architecture Doc Accuracy Review

## TL;DR
- Validate `docs/specs/architecture.md` claims (files, flows, data structures) against actual implementation in `src/`.
- Produce an evidence-backed list of inaccuracies in `/tmp/codex-architecture-review.txt`.

## Analysis
- Architecture doc focuses on AIAgentSession, LLMClient, ToolsOrchestrator, SessionTreeBuilder, exit codes, config, callbacks, telemetry, logging, events, tests, troubleshooting.
- Actual implementation spans `src/ai-agent.ts`, `src/llm-client.ts`, `src/tools/*.ts`, `src/session-tree.ts`, `src/types.ts`, and tests under `src/tests/`.
- Key deltas already spotted:
  - Tool execution path exposes `ToolsOrchestrator.executeWithManagement` (no `executeTools`).
  - Log `remoteIdentifier` for tools uses `kind:namespace:tool`, not `component:action`.
  - `AIAgentSessionConfig` in code requires `outputFormat` and exposes many more knobs not listed.
  - `AIAgentCallbacks` also expose `onOutput`/`onThinking`, missing from doc.
  - Deterministic harness lives under `src/tests/phase1/*`, not `tests/phase1/*`.
- Need to comb the doc for any other mismatches (missing components, behaviors) and cite code evidence.

## Decisions Needed
- None: purely observational review per request.

## Plan
1. Re-read required docs (SPECS, IMPLEMENTATION, DESIGN, MULTI-AGENT, TESTING, AI-AGENT-GUIDE, README, AI-AGENT-INTERNAL-API) to ensure architecture context is correct.
2. Walk through `docs/specs/architecture.md` section by section, logging each claim.
3. Cross-reference with implementation files (`src/ai-agent.ts`, `src/llm-client.ts`, `src/tools/*.ts`, `src/types.ts`, `src/tests/phase1/*`).
4. Catalog discrepancies (incorrect references, missing critical components, mismatched data structures, wrong exit code info) and gather code line evidence.
5. Write findings (only inaccuracies) to `/tmp/codex-architecture-review.txt`, citing both doc line(s) and code evidence.
6. Share summary of findings and highlight any risks or recommended doc updates.

## Implied Decisions
- Treat required `outputFormat` and callback omissions as critical doc gaps because they affect API contracts.
- Consider method name mismatches (e.g., `executeTools`) as inaccuracies because they could mislead implementers.
- Treat wrong test path references as inaccuracies because new contributors would fail to locate suites.

## Testing Requirements
- No automated tests to run; task is documentation review.

## Documentation Updates Required
- Architecture doc needs corrections for each identified mismatch (method names, config shape, callback list, log schema, test locations). To be updated after Costa reviews findings.
