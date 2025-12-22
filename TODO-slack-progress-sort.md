# TODO: Slack Progress Sorting

## TL;DR
- Slack progress blocks are built in `src/server/status-aggregator.ts` (`buildStatusBlocks`), and currently sort agents by **ascending** `elapsedSec` (newest first).
- That same function **dedups by `callPath`/`agentId`**, which collapses parallel runs that share the same chain (e.g., `neda→web-research→web-search→web-fetch`). The kept entry depends on sort order, so flip‑flop is expected.
- The **full parent‑child hierarchy is available** in the opTree (SessionNode → childSession), but the Slack UI currently **flattens** it and drops duplicates.

## Analysis
- Docs reviewed per project requirements (SPECS/IMPLEMENTATION/DESIGN/MULTI-AGENT/TESTING/AI-AGENT-INTERNAL-API/README/AI-AGENT-GUIDE, plus SLACK guide).
- Slack progress updates are rendered in `src/server/slack.ts` using:
  - `buildSnapshotFromOpTree(...)` → aggregates per-session status lines.
  - `buildStatusBlocks(...)` → constructs Block Kit list of running agents.
  - `formatSlackStatus(...)` → plain-text fallback for the `text` field.
- **Full hierarchy exists** in the opTree:
  - `SessionNode` contains nested `childSession` (see `src/session-tree.ts`).
  - Each op has `kind: 'llm' | 'tool' | 'session' | 'system'`, with `startedAt/endedAt` so we can infer what is currently active.
  - Sub-agent calls are represented as `kind: 'session'` ops (see `src/tools/tools.ts`).
- Slack UI today **does not use the tree structure** for display:
  - `buildSnapshotFromOpTree` flattens nodes into `AgentStatusLine` (agentId, callPath, elapsedSec, status, latestStatus).
  - `buildStatusBlocks` then sorts and **dedups by callPath/agentId**, which hides parallel siblings.
- **Reliability constraints**:
  - We **cannot** 100% reliably infer “waiting on tools” for queued tools (queue wait happens before tool op starts).
  - Tree visualization and status inference increase complexity and Slack block/char‑limit risk.

## Decisions (Costa)
1. Reverse order (newest at bottom = sort by elapsed descending).
2. Keep only the newest 10 entries.
3. Remove dedup.
4. **No numbering**.

## Risks / Considerations
- **Visibility risk:** Keeping only newest 10 can hide long‑running parents (including the root), which may reduce context.
- **Duplicate labels:** Without dedup, multiple entries may have identical `agentName` (last path segment), making them hard to distinguish without numbering.
- **Ordering jitter:** Ties in `elapsedSec` can still reorder within newest 10 unless we add a stable tie‑breaker.
- **Slack limits:** With a 10‑entry cap, block count should stay safely under 50 (current layout uses ~1–2 blocks per entry). Low risk here.

## Plan
- Implement:
  - Sort by elapsed descending (oldest at top, newest bottom).
  - Drop dedup.
  - Truncate list to newest 10 entries.
  - Add stable tie‑breaker (e.g., `callPath` + `agentId`) to reduce jitter.
- Run `npm run lint` and `npm run build` after changes.

## Implied Decisions
- None.

## Testing Requirements
- Run `npm run lint` and `npm run build` after implementation.
- Optional: manual Slack run to validate ordering and truncation behavior.

## Documentation Updates Required
- Likely none.
