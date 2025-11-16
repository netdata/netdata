# Docs/Specs Verification & Remediation

## TL;DR
- Re-verify every markdown file under `docs/specs/`, correcting any inaccurate statements and adding missing business logic so the specs match the live code paths.
- Track discrepancies with severity tags inside `/tmp/codex-spec-verification.txt`, fix the relevant docs (plus `README.md`, `index.md`, `CLAUDE.md`, `AI-AGENT-GUIDE.md` when behavior descriptions change), and keep the structure flat.
- Finish with the mandated repo quality checks (`npm run lint`, `npm run build`) and deliver Costa a coverage report summarizing fixes, remaining questions, and evidence references.

## Analysis
-### Inventory (facts from `ls docs/specs` on 2025-11-16)
- 38 markdown files spanning architecture, lifecycle, context guard, configuration/frontmatter, six provider adapters, seven tool stacks, six headend surfaces, observability (logging/telemetry/accounting/pricing), and auxiliary modules (snapshots, optree, library API, meta docs).
- Each document maps to concrete code areas already identified: core orchestration (`src/ai-agent.ts`, `src/llm-client.ts`, `src/session-tree.ts`, `src/op-tree/*`, `src/types.ts`), configuration/frontmatter (`src/config*.ts`, `src/frontmatter.ts`, CLI glue), providers (`src/providers/*`, `src/tokenizer-registry.ts`), tools (`src/tools/*.ts`, `src/mcp/*.ts`, `src/rest-tools/*.ts`, `src/progress-report/*.ts`), headends/concurrency (`src/headends/*`, `src/headend-manager.ts`, `src/cli/headend/*.ts`), and observability (`src/logging/*`, `src/telemetry/*`, `src/metrics/*`, `src/accounting/*`).
- Required baseline documents (docs/SPECS.md, docs/IMPLEMENTATION.md, docs/DESIGN.md, docs/MULTI-AGENT.md, docs/TESTING.md, docs/AI-AGENT-GUIDE.md, docs/AI-AGENT-INTERNAL-API.md, README.md) were all re-read on 2025-11-16 to satisfy the repo’s prerequisite policy before any doc edits.
- Known drifts to confirm explicitly: context guard defaults (131072 ctx window / 256 buffer), reasoning controls via `ProviderReasoningMapping`, shared MCP restart rules across transports, queue manager bypass for internal helpers, accounting JSON schema (per-turn/per-tool), telemetry metrics `ai_agent_queue_depth`, Slack headend routing updates, and `stopRef` propagation inside `LLMClient`.

### Verification matrix (source-of-truth mapping)
| docs/specs file | Primary source modules/tests | Verification status |
| --- | --- | --- |
| architecture.md | `src/ai-agent.ts`, `src/session-tree.ts`, `src/tools/tools-orchestrator.ts`, `src/logging/*`, `src/accounting/*`, `src/tests/phase1-harness.ts` | ✅ Verified 2025-11-16 (no changes needed) |
| session-lifecycle.md | `src/ai-agent.ts`, `src/op-tree/*`, `src/shutdown-controller.ts`, `src/tests/fixtures/*` | ✅ Verified 2025-11-16 (no changes needed) |
| context-management.md | `src/context-guard/*.ts`, `src/token-usage/*.ts`, `src/providers/base-llm-provider.ts`, `src/tests/fixtures/test-context-guard.ts` | ✅ Verified 2025-11-16 (no changes needed) |
| call-path.md | `src/op-tree/*`, `src/ai-agent.ts`, `src/logging/log-builder.ts` | ✅ Verified 2025-11-16 (no changes needed) |
| retry-strategy.md | `src/ai-agent.ts`, `src/llm-client.ts`, `src/providers/*`, `src/tests/phase1/scenarios/retry-*` | ✅ Verified 2025-11-16 (no changes needed) |
| configuration-loading.md | `src/config/*.ts`, `src/cli/config-loader.ts`, `src/env/*.ts`, `src/tests/config/*` | ✅ Verified 2025-11-16 (no changes needed) |
| frontmatter.md | `src/frontmatter.ts`, `src/prompt-loader.ts`, `src/tests/frontmatter/*.ts` | ✅ Verified 2025-11-16 (no changes needed) |
| models-overview.md | `src/llm-client.ts`, `src/providers/base-llm-provider.ts`, `src/providers/*`, `src/tokenizer-registry.ts` | ✅ Verified 2025-11-16 (no changes needed) |
| providers-openai.md | `src/providers/openai.ts`, `src/tests/providers/openai*.ts`, `src/tokenizer-registry.ts` | ✅ Verified 2025-11-16 (no changes needed) |
| providers-anthropic.md | `src/providers/anthropic.ts`, `src/tests/providers/anthropic*.ts` | ✅ Verified 2025-11-16 (no changes needed) |
| providers-google.md | `src/providers/google.ts`, `src/tests/providers/google*.ts` | ✅ Verified 2025-11-16 (no changes needed) |
| providers-ollama.md | `src/providers/ollama.ts`, `src/tests/providers/ollama*.ts` | ✅ Updated 2025-11-16 (documented merged dynamic options) |
| providers-openrouter.md | `src/providers/openrouter.ts`, `src/tests/providers/openrouter*.ts` | ✅ Verified 2025-11-16 (no changes needed) |
| providers-test.md | `src/llm-providers/test-llm.ts`, `src/tests/phase1-harness.ts`, `src/tests/fixtures/test-llm-scenarios.ts` | ✅ Verified 2025-11-16 (no changes needed) |
| tools-overview.md | `src/tools/tools-orchestrator.ts`, `src/tools/tool-registry.ts`, `src/tests/tools/*.ts` | ✅ Verified 2025-11-16 (no changes needed) |
| tools-agent.md | `src/tools/agent-tool.ts`, `src/agent-registry.ts`, `src/tests/sub-agents/*.ts` | ✅ Verified 2025-11-16 (no changes needed) |
| tools-batch.md | `src/tools/batch-tool.ts`, `src/tests/tools/batch*.ts`, `src/tests/phase1/scenarios/batch-*.ts` | ✅ Verified 2025-11-16 (no changes needed) |
| tools-mcp.md | `src/mcp-client.ts`, `src/mcp/*.ts`, `src/tests/mcp/*` | ✅ Verified 2025-11-16 (no changes needed) |
| tools-progress-report.md | `src/tools/progress-report-tool.ts`, `src/tests/tools/progress-report*.ts` | ✅ Verified 2025-11-16 (no changes needed) |
| tools-final-report.md | `src/tools/final-report-tool.ts`, `src/tests/tools/final-report*.ts` | ✅ Verified 2025-11-16 (no changes needed) |
| tools-rest.md | `src/tools/rest-tool.ts`, `src/openapi/*.ts`, `src/tests/tools/rest*.ts` | ✅ Verified 2025-11-16 (no changes needed) |
| headends-overview.md | `src/headend-manager.ts`, `src/headends/*.ts`, `src/cli.ts` | ✅ Verified 2025-11-16 (no changes needed) |
| headend-rest.md | `src/headends/rest-headend.ts`, `src/cli/rest-server.ts`, `src/tests/headends/rest/*.ts` | ✅ Verified 2025-11-16 (no changes needed) |
| headend-mcp.md | `src/headends/mcp-headend.ts`, `src/mcp/server/*.ts`, `src/tests/headends/mcp/*.ts` | ✅ Verified 2025-11-16 (no changes needed) |
| headend-openai.md | `src/headends/openai-headend.ts`, `src/tests/headends/openai/*.ts` | ✅ Verified 2025-11-16 (no changes needed) |
| headend-anthropic.md | `src/headends/anthropic-headend.ts`, `src/tests/headends/anthropic/*.ts` | ✅ Verified 2025-11-16 (no changes needed) |
| headend-slack.md | `src/headends/slack-headend.ts`, `src/slack/*.ts`, `docs/SLACK.md`, `src/tests/slack/*.ts` | ✅ Verified 2025-11-16 (no changes needed) |
| headend-manager.md | `src/headend-manager.ts`, `src/concurrency/concurrency-limiter.ts`, `src/tests/headends/*.ts` | ✅ Verified 2025-11-16 (no changes needed) |
| logging-overview.md | `src/logging/*.ts`, `src/types.ts`, `src/tests/logging/*.ts` | ✅ Verified 2025-11-16 (no changes needed) |
| telemetry-overview.md | `src/telemetry/*.ts`, `src/metrics/*.ts`, `src/tests/telemetry/*.ts` | ✅ Verified 2025-11-16 (no changes needed) |
| accounting.md | `src/accounting/*.ts`, `src/types.ts`, `src/tests/accounting/*.ts` | ✅ Verified 2025-11-16 (no changes needed) |
| pricing.md | `src/accounting/pricing-table.ts`, `src/tests/accounting/pricing*.ts`, `docs/openai-pricing.md` | ✅ Verified 2025-11-16 (no changes needed) |
| snapshots.md | `src/snapshots/*.ts`, `src/tests/snapshots/*.ts` | ✅ Verified 2025-11-16 (no changes needed) |
| optree.md | `src/op-tree/*`, `src/tests/op-tree/*.ts`, `docs/MULTI-AGENT.md` | ✅ Verified 2025-11-16 (no changes needed) |
| library-api.md | `src/ai-agent.ts`, `src/types.ts`, `docs/AI-AGENT-INTERNAL-API.md` | ✅ Verified 2025-11-16 (no changes needed) |
| README.md (specs meta) | `docs/specs/README.md`, repo root README cross references | ✅ Verified 2025-11-16 (no changes needed) |
| index.md (specs meta) | `docs/specs/index.md`, directory inventory automation | ✅ Verified 2025-11-16 (no changes needed) |
| CLAUDE.md (specs meta) | `docs/specs/CLAUDE.md`, AI guide instructions | ✅ Verified 2025-11-16 (no changes needed) |

> Status column will be updated as each doc is validated/fixed, ensuring no file is overlooked.

### Evidence mapping
- `/tmp/codex-spec-verification.txt` will store structured findings: `[severity] docs/specs/<file>.md:<section> -> src/...:line`, resolution status, and date.
- Each verified doc will reference owning modules/tests to avoid omissions; progress tracked here (38/38 required).

## Decisions
1. **Evidence log location** – `/tmp/codex-spec-verification.txt` confirmed (per TODO-SPEC-VERIFICATION.md + user brief). ✅
2. **Severity schema** – Critical/High/Medium/Low tags approved (per same brief). ✅
3. **Review order** – Architecture → Lifecycle → Config/Frontmatter → Providers → Tools → Headends → Observability → Meta. ✅
4. **Business-logic scope** – Every runtime rule (pricing, concurrency, accounting, configuration precedence, reasoning, crash handling) must be represented; missing logic counts as discrepancies. ✅

## Plan
1. **Baseline reading** – Re-read remaining mandated docs (`docs/MULTI-AGENT.md`, `docs/TESTING.md`, `docs/AI-AGENT-GUIDE.md`, `docs/docs/AI-AGENT-INTERNAL-API.md`, `README.md`) to refresh terminology and ensure no upstream assumptions slip in. _Status: pending._
2. **Spec inventory matrix** – Extend this TODO with a table linking each `docs/specs/*.md` to its source modules/tests plus a verification checkbox. _Status: in progress._
3. **Clustered verification loop** – For each cluster (architecture/session, config/frontmatter, providers, tools, headends, observability, auxiliary/meta):
   - Log every claim per section, trace the exact implementation/tests, and label as accurate, inaccurate, or missing.
   - Record discrepancies with severity + references in `/tmp/codex-spec-verification.txt`.
   - Update the doc immediately (no batching) and adjust cross-references in `docs/specs/index.md`, `docs/specs/README.md`, and `docs/specs/CLAUDE.md` as needed.
4. **Meta alignment** – Whenever runtime behavior text changes, sync `docs/AI-AGENT-GUIDE.md`, root `README.md`, and any headend/provider-specific instructions so there is no divergent guidance.
5. **Quality gates** – Run `npm run lint` and `npm run build` after documentation edits to uphold repo policy. Capture logs for the final report.
6. **Final reporting** – Summarize verification coverage (38/38), highlight key fixes + remaining open questions, and provide Costa with next-step recommendations.

## Implied Decisions
- Missing documentation about runtime knobs (reasoning cues, context guard budgets, queue telemetry, accounting payloads, MCP restart strategy) will be filled in now; no deferrals unless Costa overrules.
- Maintain the flat directory structure and keep `docs/specs/index.md` + `docs/specs/README.md` synchronized whenever files change.
- Update `docs/AI-AGENT-GUIDE.md` concurrently with any runtime behavior clarifications to avoid two sources of truth diverging.

## Testing Requirements
- `npm run lint`
- `npm run build`

## Documentation Updates Required
- Potentially every `docs/specs/*.md` file (architecture, lifecycle, config, providers, tools, headends, observability, auxiliary systems).
- Meta docs (`docs/specs/index.md`, `docs/specs/README.md`, `docs/specs/CLAUDE.md`, `docs/AI-AGENT-GUIDE.md`, README.md) when behavior or file listings shift.
- `/tmp/codex-spec-verification.txt` serves as the auditable discrepancy log referenced in the final report.
