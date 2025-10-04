# TODO – Session Visualization UI

## Goal
- Build a web UI that, given a transaction/session id, visualizes stored agent runs: summary, cost analysis, and interactive operation graphs.

## Current Facts
- Snapshots already persist after every run at `${sessionsDir || ~/.ai-agent/sessions}/<originTxnId>.json.gz` (see `src/ai-agent.ts:247`).
- Snapshot payload structure: `{ version: 1, reason, opTree }`; `opTree` matches `SessionNode` defined in `src/session-tree.ts` (hierarchical turns/ops with timestamps, accounting, nested sub-agents).
- Accounting entries hold per-tool and per-LLM usage, latency, and (when available) USD cost (`src/types.ts:440`).
- Live servers (`src/server/api.ts`, `session-manager.ts`) do not expose historical snapshots; only in-memory runs are accessible during execution.
- `buildSnapshotFromOpTree` and related helpers (`src/server/status-aggregator.ts`) already derive aggregate stats useful for a Summary tab.

## Gaps
- No endpoint or service loads/decompresses historical `.json.gz` archives.
- No index over snapshot files—filenames are UUIDs without user metadata, so discovery requires scanning/reading each file.
- No API schema agreed for the three UI tabs (Summary, Cost, Graph).
- No transformation layer yet for timeline vs. hierarchy views.
- Large tool responses may be truncated (`response.truncated === true`); UI needs to surface that state.

## Decisions Required (Costa)
1. Web stack & hosting: single-page app bundled into existing Express server vs. standalone frontend.
2. Backend strategy for snapshots: on-demand streaming per request vs. periodic indexing/cache.
3. Authentication/authorization expectations for the UI.
4. Scope for MVP (read-only explorer?) vs. long-term editing/annotation features.

## Workstreams

### Backend
- [ ] Implement snapshot cataloger:
  - Scan `sessionsDir`, read gzip headers, cache minimal metadata (originTxnId, startedAt, agentId, user question if available).
  - Store index in memory or on disk for quick lookup.
- [ ] Expose REST API:
  - `GET /sessions`: paginated list using catalog.
  - `GET /sessions/:id`: decompressed `opTree` (with redaction similar to `api.ts` route) plus metadata.
  - `GET /sessions/:id/cost`: pre-aggregated cost breakdown (group by provider, agent, tool).
  - `GET /sessions/:id/summary`: user prompt, final answer, run metrics (`buildSnapshotFromOpTree`).
  - Consider `ETag`/`Last-Modified` headers for caching.
- [ ] Add guards for missing/partial data (working theory: some older snapshots may lack `totals`; handle gracefully).

### Data Transformations
- [ ] Derive hierarchy model:
  - Traverse `SessionNode.turns[*].ops[*]` to build tree nodes keyed by paths (`SessionTreeBuilder.getOpPath`).
- [ ] Derive timeline model:
  - Normalize spans using `startedAt`/`endedAt`; fall back to synthetic durations when missing (flag uncertainty).
- [ ] Compute cost aggregations:
  - Group `LLMAccountingEntry` by `{provider, model}`.
  - Group `ToolAccountingEntry` by `{mcpServer, command}`.
  - Attribute costs to agents via op path ancestry.

### Frontend
- [ ] Define UI architecture once stack chosen.
- [ ] Summary tab:
  - Show user question, final report, high-level metrics (tokens, cost, duration).
- [ ] Cost tab:
  - Tables per provider/model, per agent/sub-agent, per tool (include percentage of total).
  - Highlight largest contributors, truncated responses, retry counts.
- [ ] Graph tab:
  - Hierarchy mode: tree/graph (e.g., D3 collapsible). Include runtime, token, cost badges.
  - Timeline mode: Gantt-style lanes per agent/tool turn.
  - Support tool/agent filtering and hover tooltips sourced from logs/accounting.

### UX & Validation
- [ ] Decide formatting for truncated payloads, errors, and missing timestamps.
- [ ] Provide download/export (raw JSON, CSV for costs) if needed.
- [ ] Ensure UI handles very large sessions without freezing (virtualized lists, chunked graph rendering).

### Ops & Testing
- [ ] Unit-test snapshot loader (gzip handling, error cases).
- [ ] Integration tests for REST endpoints (verify redaction, pagination).
- [ ] Lint/build must remain clean (`npm run lint`, `npm run build`).
- [ ] Load-test API on realistic snapshot volume (working theory: nightly batch to rebuild index may be required).

## Risks & Considerations
- Large snapshots can be tens of MB; streaming decompression and pagination are critical.
- Nested sub-agents increase graph depth—need UI safeguards (collapse, search).
- Accounting records may lack cost data for some providers; display zero/unknown explicitly.
- Timeline accuracy depends on consistent timestamp capture; older runs may need fallback messaging.

## References
- Snapshot persistence: `src/ai-agent.ts:247-266`.
- Session tree structure: `src/session-tree.ts`.
- Accounting types: `src/types.ts:420-478`.
- Status summarizer helpers: `src/server/status-aggregator.ts`.
- Existing API patterns: `src/server/api.ts`, `src/server/session-manager.ts`.
