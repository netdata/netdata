# TODO: Subagent task-status linking and accounting

## TL;DR
- Investigate why task-status (tool_output read-grep) subagent progress is not linked to parent agent.
- Verify missing accounting entries in llm-headends when that subagent finishes, and whether callPath is correct.
- Identify root cause(s), propose fixes, and update docs/tests as required.

## Analysis
- Code references indicate tool_output read-grep subagent is spawned via AIAgent without trace/callPath propagation, unlike normal subagents.
  - `src/tool-output/extractor.ts:448-494` creates `childConfig` with `agentId: 'tool_output.read_grep'` but no `trace.callPath`, `agentPath`, `originId`, or `parentId`. This means callPath defaults to agentId and is not linked to the parent session. (evidence: lines shown in `src/tool-output/extractor.ts`)
  - `src/subagent-registry.ts:264-312` shows normal subagents derive `childAgentPath` via `appendCallPathSegment(...)` and set `trace` with `originId/parentId/callPath/agentPath`. That linking is missing in read-grep. (evidence: lines shown in `src/subagent-registry.ts`)
  - LLM headends filter progress events by `callPath` prefix (not just agentId). `src/headends/openai-completions-headend.ts:598-625` only accepts progress when `callPath` starts with the root callPath; non-matching callPath updates are ignored. This explains lost progress for read-grep when callPath is not a child of the parent. (evidence: lines shown in `src/headends/openai-completions-headend.ts`)
- Unit tests currently mock read-grep child `callPath` as `tool_output.read_grep` (`src/tests/unit/tool-output.spec.ts:31-48`), so changing callPath propagation will require updating tests. (evidence: `src/tests/unit/tool-output.spec.ts`)
- External review notes:
  - Codex review flagged a potential parity gap: read-grep turnPath uses `turnPathPrefix` only, whereas normal subagents can use parent tool-call context (`turn`/`subturn`) to derive a per-call turnPath. (evidence: `src/tool-output/extractor.ts` vs `src/ai-agent.ts` around `composeTurnPath`)
  - Claude review noted parentId linkage differs from subagent-registry but aligns with spawn-child patterns and may be more correct (direct parent txnId).
  - GLM review approved the current implementation; no double-accounting or security concerns found.

## Decisions
- Decision 1: How should tool_output read-grep callPath/agentPath be linked to the parent?
  - Option A (Chosen): Append a single segment to the parent callPath, e.g. `${parentCallPath}:tool_output.read_grep`.
    - Pros: Matches subagent behavior; progress events pass headend filters; opTree hierarchy consistent.
    - Cons: Slightly different callPath value than existing tests; requires updates.
    - Implications/Risks: Any consumers expecting the old `tool_output.read_grep` path will change; need to update tests/docs.
  - Option B: Append two segments, e.g. `${parentCallPath}:tool_output:read_grep`.
    - Pros: Clearer hierarchy with dedicated tool_output segment.
    - Cons: Longer/less consistent with existing tool naming; requires broader updates.
    - Implications/Risks: Potential confusion if other tooling assumes a single segment per agent.
  - Option C: Keep callPath as `tool_output.read_grep`, but relax headend progress filters to accept it as a child even when it doesn’t match rootCallPath.
    - Pros: Avoids callPath changes; minimal change in tool_output.
    - Cons: Loosens filtering; can leak unrelated progress events into a headend session.
    - Implications/Risks: Risk of mixing progress from unrelated sessions if callPath collisions occur.

- Decision 2: Should read-grep trace IDs be linked to the parent (originId/parentId)?
  - Option A (Chosen): Propagate `originId` and set `parentId` to the parent session’s txnId, with a fresh `selfId`.
    - Pros: Accounting/logs line up with the parent chain; consistent with subagent trace behavior.
    - Cons: Requires wiring extra fields into ToolOutputExtractor deps.
    - Implications/Risks: Any tooling that assumes tool_output subagent has standalone origin will see new linkage.
  - Option B: Keep child trace standalone (no parent/origin propagation).
    - Pros: Minimal code change.
    - Cons: Harder to correlate in headends/snapshots; inconsistent with subagents.
    - Implications/Risks: Progress/accounting may remain “lost” in parent flows.
  - Option C: Propagate only `originId` (leave `parentId` undefined).
    - Pros: Partial linkage while keeping lineage shallow.
    - Cons: Still breaks explicit parent-child correlation.
    - Implications/Risks: Ambiguous lineage in metrics/logs.

- Decision 3 (Optional): Do we want strict turnPath parity with normal subagents (derive turnPath from parent tool-call context)?
  - Option A (Chosen): Wire parentContext into tool_output read-grep and set child turnPath to `composeTurnPath(parentTurn, parentSubturn)`.
    - Pros: Exact parity with normal subagents; per-call log grouping aligns.
    - Cons: Requires new plumbing from ToolsOrchestrator → ToolOutputProvider → ToolOutputExtractor.
    - Implications/Risks: Slightly more code churn; tests/docs updates.
  - Option B: Keep current behavior (turnPathPrefix only).
    - Pros: Minimal change; current implementation already fixes progress/accounting linkage.
    - Cons: TurnPath remains session-level, not per tool call.
    - Implications/Risks: Some log grouping may be less granular than normal subagents.

## Plan
- After decisions:
  - Extend ToolOutputExtractor deps to include parent trace/agentPath details.
  - Set child trace/callPath/agentPath in read-grep AIAgent config to match selected option.
  - Update unit tests for tool_output read-grep expectations.
  - Add/adjust Phase 1 harness coverage if core orchestration behavior changes (per docs).

## Implied decisions
- None yet.

## Testing requirements
- Run: `npm run lint`, `npm run build`.
- Add/adjust Phase 1 tests around read-grep callPath/progress forwarding if behavior changes.

## Documentation updates required
- If callPath/trace behavior is changed, update relevant docs (likely `docs/specs/MULTI-AGENT.md` or `docs/skills/ai-agent-session-snapshots.md`) to reflect tool_output subagent linkage semantics.
