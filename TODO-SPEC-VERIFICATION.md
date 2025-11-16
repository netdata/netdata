# Specification Verification Report

## TL;DR
- Costa needs every file under `docs/specs/` re-verified against the current implementation to ensure *no* business logic is missing or misrepresented.
- Deliverable: up-to-date specs + supporting evidence log for each discrepancy, with inline fixes landed alongside any required meta-doc updates (index, guide, README).
- Work cannot start until plan + open decisions below are confirmed.

## Analysis
- Inventory confirmed via `ls docs/specs`: 38 markdown files spanning architecture, lifecycle, configuration/frontmatter, six provider docs, seven tool docs, six headend docs, observability (logging + telemetry + accounting + pricing), optional subsystems (snapshots, optree), and meta files (`README.md`, `index.md`, `CLAUDE.md`).
- Source areas per cluster:
  - Architecture/session/control path: `src/ai-agent.ts`, `src/session/*`, `src/op-tree/*`, `src/types.ts`, `src/llm-client.ts`.
  - Configuration/frontmatter/accounting: `src/config/*.ts`, `src/frontmatter.ts`, `src/accounting/*.ts`, `src/context-guard/*.ts`.
  - Providers: `src/llm-providers/{anthropic,openai,google,ollama,openrouter,test-llm}.ts` + `src/providers/base-llm-provider.ts`.
  - Tools + integrations: `src/tools/*.ts`, `src/mcp/*.ts`, `src/openapi/*.ts`, `src/rest-tools/*.ts`, `src/progress-report/*.ts`.
  - Headends + concurrency: `src/headends/*`, `src/headend-manager.ts`, CLI/rest wrappers.
  - Observability & aux: `src/logging/*`, `src/telemetry/*`, `src/metrics/*`, `src/snapshots/*`.
- Notable recent changes risk doc drift: context guard rework, reasoning defaults, Slack routing updates, MCP crash handling, shared tool registry mutex, token-cost accounting tweaks. Need to ensure these behaviors are spelled out.
- Project rules force consultation of README + docs/SPECS.md + docs/IMPLEMENTATION.md + docs/DESIGN.md + docs/MULTI-AGENT.md + docs/TESTING.md + docs/AI-AGENT-INTERNAL-API.md + docs/AI-AGENT-GUIDE.md before editing specs; those will anchor verification notes.
- Output must separate “doc inaccuracies” (statements contradicting code) from “missing business logic” (behaviors present in code but absent from doc) with file:line evidence for both sides.

## Decisions Required / Status
1. **Output expectation** – Costa confirmed: fix docs as discrepancies are found, then deliver consolidated report. ✅
2. **Evidence log location** – Use `/tmp/codex-spec-verification.txt` for the running findings log unless Costa instructs otherwise. ✅
3. **Severity tagging** – Adopt four-level tags (critical/high/medium/low) in the log + summary to highlight urgency. ✅
4. **Review order** – Default order (architecture → lifecycle → config/frontmatter → providers → tools → headends → observability → meta) accepted. ✅
5. **Business-logic scope** – Treat *every* rule (pricing, concurrency, failure handling, accounting, configuration precedence, reasoning controls) as in scope for verification. ✅

## Plan
1. **Inventory + mapping** – Confirm complete file list under `docs/specs/`, map each to its owning modules/tests, and capture dependencies in the TODO log for traceability. (In progress)
2. **Template + evidence scaffolding** – Define a per-doc verification checklist (claims log, code references, gaps, missing logic). Stand up `/tmp/codex-spec-verification.txt` (or alternate path per decision) to collect structured findings as we go.
3. **Clustered deep review** – Move sequentially through the agreed order. For every paragraph/claim, trace the implementation (source + relevant tests) and record whether it matches reality. Note missing logic and new behaviors uncovered in code.
4. **Doc updates** – As soon as discrepancies are confirmed, update the corresponding markdown plus any cross-references (`README.md`, `index.md`, `AI-AGENT-GUIDE.md`, etc.), ensuring edits stay in sync across documents.
5. **Validation pass** – After edits, run `npm run lint` and `npm run build` (repo policy, even for doc-only changes) to prove the tree is clean. Spot-check rendered markdown if needed.
6. **Summary + handoff** – Provide Costa with a bulletized summary (per doc status, outstanding open questions, risks) and capture completion in this TODO file.

## Current Status (2025-11-16)
- [x] User reconfirmed objective + deliverables; no new blocking decisions outstanding.
- [x] Document inventory verified (`docs/specs` currently 38 files) and mapped to owning modules/tests (table above).
- [x] Evidence log path locked (`/tmp/codex-spec-verification.txt`); templated severity tags ready.
- [ ] Detailed per-doc verification sweep pending (0/38 docs fully validated in this pass).
- [ ] Cross-doc updates + meta files pending (to follow once discrepancies logged).

## Immediate Next Actions
1. Finish Plan Step #1 by double-checking supporting specs (`docs/SPECS.md`, `docs/IMPLEMENTATION.md`, `docs/DESIGN.md`, `docs/MULTI-AGENT.md`, `docs/TESTING.md`, `docs/AI-AGENT-GUIDE.md`, `docs/docs/AI-AGENT-INTERNAL-API.md`, root `README.md`).
2. Set up `/tmp/codex-spec-verification.txt` with severity template + placeholder entries per spec to speed logging.
3. Begin Cluster #1 review (`architecture.md`, `session-lifecycle.md`, `call-path.md`, `context-management.md`, `retry-strategy.md`).

## Document Mapping
| Spec file | Key source modules | Associated tests/fixtures |
| --- | --- | --- |
| accounting.md | `src/ai-agent.ts`, `src/session-tree.ts`, `src/server/session-manager.ts`, `src/server/status-aggregator.ts`, `src/types.ts` | `src/tests/phase1/runner.ts` (token/cost coverage), `src/tests/fixtures/test-llm-scenarios.ts` (accounting cases) |
| architecture.md | `src/ai-agent.ts`, `src/llm-client.ts`, `src/tools/*.ts`, `src/headends/*`, `src/server/*`, `src/config*.ts`, `src/session-tree.ts` | `src/tests/phase1/runner.ts`, `src/tests/phase2-runner.ts` |
| call-path.md | `src/session-tree.ts`, `src/session-progress-reporter.ts`, `src/tools/tools.ts`, `src/types.ts` | `src/tests/phase1/runner.ts` (opTree assertions) |
| CLAUDE.md | `docs/specs/*`, `AGENTS.md`, `docs/SPECS.md` (maintenance guidance) | n/a |
| configuration-loading.md | `src/config-resolver.ts`, `src/config.ts`, `src/include-resolver.ts`, `src/options-resolver.ts`, `src/options-schema.ts`, `src/options-registry.ts` | `src/tests/phase1/runner.ts` (config layering), `src/tests/fixtures/test-llm-scenarios.ts` |
| context-management.md | `src/ai-agent.ts` (context guard + schema tokens), `src/telemetry/index.ts`, `src/config.ts` (guards), `src/utils.ts` | `src/tests/phase1/runner.ts` (context guard coverage) |
| frontmatter.md | `src/frontmatter.ts`, `src/agent-loader.ts`, `src/include-resolver.ts`, `src/options-registry.ts` | `src/tests/fixtures/subagents/*.ai`, `src/tests/phase1/runner.ts` |
| headend-anthropic.md | `src/headends/anthropic-completions-headend.ts`, `src/headends/types.ts`, `src/headend-manager.ts` | `src/tests/phase1/runner.ts` (anthropic coverage), `src/tests/fixtures/test-llm-scenarios.ts` |
| headend-manager.md | `src/headends/headend-manager.ts`, `src/headends/concurrency.ts`, `src/headends/types.ts`, `src/server/session-manager.ts` | `src/tests/phase1/runner.ts` (headend concurrency cases) |
| headend-mcp.md | `src/headends/mcp-headend.ts`, `src/headends/mcp-ws-transport.ts`, `src/server/session-manager.ts` | `src/tests/phase1/runner.ts` (MCP harness), `src/tests/fixtures/test-llm-scenarios.ts` |
| headend-openai.md | `src/headends/openai-completions-headend.ts`, `src/headends/types.ts`, `src/headends/http-utils.ts` | `src/tests/phase1/runner.ts` |
| headend-rest.md | `src/headends/rest-headend.ts`, `src/headends/http-utils.ts`, `src/server/session-manager.ts` | `src/tests/phase1/runner.ts` (REST cases) |
| headend-slack.md | `src/headends/slack-headend.ts`, `src/headends/summary-utils.ts`, `src/server/slack.ts`, `src/headends/http-utils.ts` | `src/tests/phase1/runner.ts` (Slack flows) |
| headends-overview.md | `src/headends/*`, `src/server/session-manager.ts`, `src/server/status-aggregator.ts` | `src/tests/phase1/runner.ts` |
| index.md | All `docs/specs/*.md` | n/a |
| library-api.md | `src/index.ts`, `src/ai-agent.ts`, `src/types.ts`, `src/internal-tools.ts`, `src/subagent-registry.ts` | `src/tests/phase1/runner.ts`, `src/tests/fixtures/test-llm-scenarios.ts` |
| logging-overview.md | `src/logging/*.ts`, `src/log-formatter.ts`, `src/log-sink-tty.ts`, `src/logging/message-ids.ts` | `src/tests/phase1/runner.ts` (log capture), manual smoke tests |
| models-overview.md | `src/llm-client.ts`, `src/llm-providers/*.ts`, `src/tokenizer-registry.ts`, `src/options-schema.ts` | `src/tests/fixtures/test-llm-scenarios.ts`, `src/tests/phase1/runner.ts` |
| optree.md | `src/session-tree.ts`, `src/ai-agent.ts`, `src/tools/agent-provider.ts`, `src/tools/tools.ts`, `src/types.ts` | `src/tests/phase1/runner.ts` (opTree snapshot tests) |
| pricing.md | `src/ai-agent.ts`, `src/config-resolver.ts`, `src/config.ts`, `src/llm-client.ts`, `src/types.ts`, `src/logging/message-ids.ts` | `src/tests/phase1/runner.ts` (pricing coverage), `src/tests/fixtures/test-llm-scenarios.ts` |
| providers-anthropic.md | `src/llm-providers/anthropic.ts`, `src/llm-client.ts`, `src/tokenizer-registry.ts` | `src/tests/fixtures/test-llm-scenarios.ts` |
| providers-google.md | `src/llm-providers/google.ts`, `src/llm-client.ts` | `src/tests/fixtures/test-llm-scenarios.ts` |
| providers-ollama.md | `src/llm-providers/ollama.ts`, `src/tokenizer-registry.ts` | `src/tests/fixtures/test-llm-scenarios.ts` |
| providers-openai.md | `src/llm-providers/openai.ts`, `src/llm-client.ts` | `src/tests/fixtures/test-llm-scenarios.ts` |
| providers-openrouter.md | `src/llm-providers/openrouter.ts`, `src/llm-client.ts` | `src/tests/fixtures/test-llm-scenarios.ts` |
| providers-test.md | `src/llm-providers/test-llm.ts`, `src/tests/fixtures/test-llm-scenarios.ts`, `src/tests/phase1/runner.ts` | Deterministic harness |
| README.md | `docs/specs/*.md`, `docs/AI-AGENT-GUIDE.md` | n/a |
| retry-strategy.md | `src/ai-agent.ts` (turn retries, fallback providers), `src/llm-client.ts` (request retry), `src/tools/tools.ts` | `src/tests/phase1/runner.ts` |
| session-lifecycle.md | `src/ai-agent.ts`, `src/persistence.ts`, `src/session-tree.ts`, `src/server/session-manager.ts` | `src/tests/phase1/runner.ts`, `src/tests/phase2-runner.ts` |
| snapshots.md | `src/ai-agent.ts`, `src/persistence.ts`, `src/session-tree.ts`, `src/tests/phase1/runner.ts` | Harness snapshot coverage |
| telemetry-overview.md | `src/telemetry/index.ts`, `src/logging/*.ts`, `src/config.ts` (telemetry config) | `src/tests/phase1/runner.ts` (telemetry toggles) |
| tools-agent.md | `src/tools/agent-provider.ts`, `src/subagent-registry.ts`, `src/agent-loader.ts`, `src/tools/tools.ts` | `src/tests/fixtures/test-llm-scenarios.ts` (subagents), `src/tests/phase1/runner.ts` |
| tools-batch.md | `src/tools/internal-provider.ts`, `src/tools/tools.ts`, `src/tools/queue-manager.ts`, `src/ai-agent.ts` (batch handling) | `src/tests/phase1/runner.ts` |
| tools-final-report.md | `src/tools/internal-provider.ts`, `src/ai-agent.ts` (final report enforcement), `src/internal-tools.ts` | `src/tests/phase1/runner.ts` |
| tools-mcp.md | `src/tools/mcp-provider.ts`, `src/mcp/*`, `src/tools/queue-manager.ts` | `src/tests/fixtures/test-llm-scenarios.ts` |
| tools-overview.md | `src/tools/tools.ts`, `src/tools/types.ts`, `src/tools/queue-manager.ts`, `src/tools/internal-provider.ts` | `src/tests/phase1/runner.ts` |
| tools-progress-report.md | `src/tools/internal-provider.ts`, `src/ai-agent.ts` (progress enable/disable), `src/internal-tools.ts` | `src/tests/phase1/runner.ts` |
| tools-rest.md | `src/tools/rest-provider.ts`, `src/config-resolver.ts` (rest tools), `src/tools/openapi-importer.ts` | `src/tests/fixtures/test-llm-scenarios.ts` |

## Verification Status
| Spec file | Status | Notes |
| --- | --- | --- |
| accounting.md | Completed | Fixed ledger persistence + character-count notes (2025-11-16). |
| architecture.md | Completed | Updated business-logic line refs + phase1 harness path (2025-11-16). |
| call-path.md | Completed | Updated appendCallPathSegment snippet to match optional-arg helper (2025-11-16). |
| CLAUDE.md | Completed | Verified maintenance rules + review requirements already match project process (2025-11-16). |
| configuration-loading.md | Completed | Updated multi-layer order to include prompt/binary directories (2025-11-16). |
| context-management.md | Completed | Added canExecuteTool gating, debug topics, and accurate guard invariants (2025-11-16). |
| frontmatter.md | Completed | Fixed error-handling description for schema warnings/non-strict fallback (2025-11-16). |
| headend-anthropic.md | Completed | Verified Anthropic messages endpoint, streaming event types, and model mapping rules (2025-11-16). |
| headend-manager.md | Completed | Verified start/stop/fatal handling matches implementation (2025-11-16). |
| headend-mcp.md | Completed | Verified transport specs, concurrency, and session handling match implementation (2025-11-16). |
| headend-openai.md | Completed | Verified OpenAI-compatible endpoint behavior + streaming shape; docs already align (2025-11-16). |
| headend-rest.md | Completed | Verified REST headend routing/concurrency + extra route behavior (2025-11-16). |
| headend-slack.md | Completed | Verified Slack routing, limiter, and Bolt integration flow; docs already align (2025-11-16). |
| headends-overview.md | Completed | Verified headend catalog + manager responsibilities already match code (2025-11-16). |
| index.md | Completed | Verified catalog + meta guidance are still accurate after provider doc updates; no changes required (2025-11-16). |
| library-api.md | Completed | Verified exports, config, and callback contracts; no changes needed (2025-11-16). |
| logging-overview.md | Completed | Removed file sink + corrected logger configuration/trace flag descriptions (2025-11-16). |
| models-overview.md | Completed | Corrected Google reasoning support details (2025-11-16). |
| optree.md | Completed | Verified structures + totals/ASCII rendering match session-tree.ts; no doc changes required (2025-11-16). |
| pricing.md | Completed | Verified pricing table schema + cost priority logic; no updates required (2025-11-16). |
| providers-anthropic.md | Completed | Documented truncation-only reasoning behavior and rate-limit retry directives (2025-11-16). |
| providers-google.md | Completed | Clarified reasoningValue handling (truncation, null disable) and updated invariants (2025-11-16). |
| providers-ollama.md | Completed | Verified URL normalization + options overlay logic (2025-11-16). |
| providers-openai.md | Completed | Verified OpenAI provider mode/option handling matches code (2025-11-16). |
| providers-openrouter.md | Completed | Added metadata backfill details and fixed reasoning budget description (2025-11-16). |
| providers-test.md | Completed | Noted one-time failure simulation + corrected reasoning reference (2025-11-16). |
| README.md | Completed | Confirmed maintenance instructions (index + AI-AGENT-GUIDE updates) still align with TODO-INTERNAL-DOCUMENTATION; no edits needed (2025-11-16). |
| retry-strategy.md | Completed | Corrected backoff description to match clamp + cycle wait logic (2025-11-16). |
| session-lifecycle.md | Completed | Updated executeSingleTurn location + cleanup/snapshot behavior (2025-11-16). |
| snapshots.md | Completed | Updated snapshot reason list + callback behavior (2025-11-16). |
| telemetry-overview.md | Completed | Reviewed telemetry/runtime config + metrics/traces/log export; docs already aligned (2025-11-16). |
| tools-agent.md | Completed | Verified agent provider/execFn documentation; no edits needed (2025-11-16). |
| tools-batch.md | Completed | Verified schema generation + execution flow; docs already match code (2025-11-16). |
| tools-final-report.md | Completed | Reviewed internal provider behavior; docs already aligned (2025-11-16). |
| tools-mcp.md | Completed | Verified shared registry, filtering, and execution details; docs already align (2025-11-16). |
| tools-overview.md | Completed | Updated orchestrator method list + flow to match executeWithManagement (2025-11-16). |
| tools-progress-report.md | Completed | Corrected schema/examples to use `progress` field and re-verified enablement logic (2025-11-16). |
| tools-rest.md | Completed | Verified template substitution + complex query handling; docs already correct (2025-11-16). |

## Implied Decisions
- By default, discrepancies will be resolved via documentation updates unless Costa instructs us to change code instead.
- Meta docs (`docs/specs/index.md`, `docs/specs/README.md`, `docs/AI-AGENT-GUIDE.md`) must be updated whenever spec text changes runtime expectations.
- Verification log should cite exact file:line references for both docs and source so future reviewers can audit quickly.

## Testing Requirements
- Repo policy mandates running `npm run lint` and `npm run build` after the documentation edits land, even though TypeScript sources might remain untouched.
- Phase 1 harness only required if doc verification reveals runtime drift that forces code changes (not anticipated, but note here).

## Documentation Updates Required
- Expect to touch:
  - Each spec exhibiting inaccuracies/missing sections.
  - `docs/specs/index.md`, `docs/specs/README.md`, and `docs/specs/CLAUDE.md` for bookkeeping + instructions.
  - `docs/AI-AGENT-GUIDE.md`, README.md, docs/AI-AGENT-INTERNAL-API.md if they depend on the clarified behaviors.
  - Any supplemental changelog Costa requests once scope is confirmed.


## Conclusion

The specifications are **highly accurate** for core architecture, type definitions, and session lifecycle. Line number references are remarkably precise. However, there are critical issues in peripheral specs (REST headend, HeadendManager) and significant undocumented business logic (47+ behaviors). The specs serve as good reference but should be updated to reflect actual implementation and document hidden behaviors.
