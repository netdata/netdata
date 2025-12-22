# TODO-tools-cost

## TL;DR
Implement tool-cost pricing at the MCP server/tool level and propagate it into ToolAccountingEntry, session totals, and all cost summaries so every surface can show **total + models + tools** cost. Today only LLM cost is calculated and surfaced, so we need new config, accounting fields, aggregation, and display updates across headends, logs, progress metrics, telemetry, and docs.

## Analysis
### Facts (from code/docs)
- **LLM pricing exists; tool pricing does not.**
  - LLM pricing is defined in `Configuration.pricing` (`src/types.ts`) and validated in `src/config.ts`.
  - MCP server config (`MCPServerConfig`) has no tool pricing fields (`src/types.ts`, `src/config.ts`).
- **Tool accounting has no cost field.**
  - `ToolAccountingEntry` includes only chars in/out + latency (`src/types.ts`).
  - Tool accounting entries are created in `src/tools/tools.ts` and `src/session-tool-executor.ts` without cost.
- **All cost aggregation is LLM-only.**
  - FIN summaries in `src/ai-agent.ts` and `src/session-turn-runner.ts` sum `costUsd` only from LLM entries.
  - `SessionNode.totals.costUsd` in `src/session-tree.ts` is derived only from LLM entries.
  - `ProgressMetrics.costUsd` is the only cost field (`src/types.ts`), fed from session totals in `src/ai-agent.ts`.
  - `server/status-aggregator.ts` uses only `totals.costUsd` for Slack status footers.
- **Headend summaries show only LLM cost.**
  - OpenAI/Anthropic headends compute `costUsd` by summing LLM entries (`src/headends/openai-completions-headend.ts`, `src/headends/anthropic-completions-headend.ts`).
  - Summary formatting utilities only know a single cost (`src/headends/summary-utils.ts`).
- **Telemetry only tracks LLM cost.**
  - `ai_agent_llm_cost_usd_total` exists; tool metrics carry no cost (`src/telemetry/index.ts`).
- **Documentation/specs describe cost as LLM-only.**
  - Accounting and pricing docs reference only LLM pricing and LLM cost (`docs/specs/accounting.md`, `docs/specs/pricing.md`).
  - Headend specs mention a single “cost” value in summaries (`docs/specs/headend-openai.md`, `docs/specs/headend-anthropic.md`).

### Considerations / Risks
- **Potential double-emission of tool accounting callbacks** (needs verification).
  - ToolsOrchestrator emits `onAccounting` on success (`src/tools/tools.ts`) while `SessionToolExecutor` also records tool accounting and calls `onAccounting` (`src/ai-agent.ts` wiring). If both are active, downstream headends might see duplicate tool entries. This is a *working theory* until confirmed by tracing.
- **REST tools and internal tools have no pricing model.**
  - Requirement mentions MCP tools, but REST/agent tools share the same accounting type; we need to decide if they should be costed or explicitly excluded.
- **Cost semantics for failures.**
  - Some providers bill on failed calls; others do not. We need explicit policy so totals match real invoices.

## Decisions (resolved by Costa)
1) **Tool pricing configuration (per call)**
   - Decision: Per tool provider with optional per-tool override; per-tool overrides take priority. Pricing is **per call only**.
   - Implication: Add a per-provider default (e.g., `toolCostUsd`) plus per-tool overrides (e.g., `toolCosts.<toolName>`).

2) **Cost fields & backward compatibility**
   - Decision: **Option A** — keep `costUsd` as **total** and add explicit `costUsdModels` + `costUsdTools` to totals/metrics.
   - Implication: Existing consumers still see a single cost; semantics change from LLM-only to total.

3) **Charge tool costs on failures**
   - Decision: **Option A** — charge on every attempted call (ok + failed).

4) **Scope**
   - Decision: **Option A** — MCP tools only (REST later if needed).

5) **Display format**
   - Decision: **Option A** — `cost total=$X (models=$Y, tools=$Z)`.

## Plan
1. **Finalize decisions** (config shape, cost fields, failure policy, scope, display format).
2. **Config + schema**
   - Extend `MCPServerConfig` (and `Configuration` if needed) to accept tool pricing.
   - Validate in `src/config.ts` (Zod) and ensure config resolver merges/loads it.
   - Update `.ai-agent.json` schema docs in `docs/AI-AGENT-GUIDE.md` and `docs/specs/configuration-loading.md`.
3. **Accounting model**
   - Add tool cost field on `ToolAccountingEntry` (or chosen naming).
   - Add totals breakdown to `SessionNode.totals` and `ProgressMetrics`.
4. **Tool cost computation**
   - Implement lookup: `(mcpServerName, toolName) -> costUsd` using server config + optional default.
   - Inject cost into tool accounting at the canonical accounting write path (likely `SessionToolExecutor`), and ensure callback emissions don’t create mismatched duplicates.
5. **Aggregation & totals**
   - Update `session-tree.ts` aggregation to compute `modelsCost`, `toolsCost`, `totalCost`.
   - Update `ai-agent.ts` and `session-turn-runner.ts` FIN summaries with the new cost breakdown.
6. **Headends & messages**
   - OpenAI/Anthropic headends: totals and summary lines in `openai-completions-headend.ts` / `anthropic-completions-headend.ts`.
   - Summary formatting (`summary-utils.ts`) and Slack status (`server/status-aggregator.ts`).
   - Any other surfaces that show “cost” should show total + models + tools.
7. **Telemetry**
   - Add tool cost metric (e.g., `ai_agent_tool_cost_usd_total`) and include cost in ToolMetricsRecord if desired.
8. **Tests**
   - Update phase1 harness expectations where cost totals are asserted.
   - Add deterministic tests for tool cost aggregation + summary formatting (Phase 1 harness).
9. **Docs**
   - Update: `docs/specs/accounting.md`, `docs/specs/pricing.md` (or new tool-pricing section), `docs/specs/optree.md`, `docs/specs/headend-openai.md`, `docs/specs/headend-anthropic.md`, `docs/specs/headends-overview.md`, `docs/specs/telemetry-overview.md`, `docs/AI-AGENT-GUIDE.md`, `docs/AI-AGENT-INTERNAL-API.md`, `README.md`.

## Implied Decisions (to confirm)
- Currency remains **USD** and tool prices are **per-call** only.
- Total cost = model cost + tool cost (does **not** include `upstreamInferenceCostUsd` unless explicitly requested).
- Tool cost lookup uses **raw MCP tool names** within each server (not namespaced `server__tool`), mapped via MCP provider tool name mapping.

## Testing Requirements
- Run `npm run lint` and `npm run build` after implementation.
- Add/adjust Phase 1 harness tests to cover:
  - Tool cost on success and failure per chosen policy.
  - Summary outputs include total/models/tools.
  - Accounting JSONL includes tool costs and totals are correct.

## Documentation Updates Required
- `docs/specs/accounting.md`
- `docs/specs/pricing.md` (or new section for tool pricing)
- `docs/specs/optree.md`
- `docs/specs/headend-openai.md`
- `docs/specs/headend-anthropic.md`
- `docs/specs/headends-overview.md`
- `docs/specs/telemetry-overview.md`
- `docs/AI-AGENT-GUIDE.md`
- `docs/AI-AGENT-INTERNAL-API.md`
- `README.md`
