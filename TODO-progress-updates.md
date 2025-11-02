# Progress Update Display Issues - Fix Plan

## TL;DR
- callPath currently mixes agent/tool segments, so child session stubs inherit tool names and headends render duplicates.
- We will propagate the pure agentPath through the orchestrator and progress events, build stubs from it, and switch headends to display agentPath.
- Real-time streaming stays untouched; Slack output must remain unchanged.

## Analysis
- `AIAgentSession` only passes `callPath` into `ToolsOrchestrator` (`src/ai-agent.ts:538`), so the orchestrator cannot distinguish between agent-only and agent+tool paths when emitting progress.
- Child session stubs use `parentSession.callPath` (`src/tools/tools.ts:212-224`); if the parent just executed a tool, the stub inherits the tool segment, leading to `agent:tool:agent` duplication.
- Progress events (`src/session-progress-reporter.ts:18-122`) carry just `callPath`, so downstream consumers (headends, telemetry) cannot opt into the agent-only view even if they want to.
- OpenAI and Anthropic headends (`src/headends/openai-completions-headend.ts:520-613`, `src/headends/anthropic-completions-headend.ts:430-520`) therefore normalize the polluted callPath and still surface duplicates like `web-research:web-search:web-search`.

## Decisions
- Display colon-delimited agent paths (e.g., `web-research:web-search`); no delimiter change is required once duplication is removed.
- Emit both `callPath` (tool-inclusive) and `agentPath` (agent-only) so consumers can choose; Slack remains wired to existing behaviour.

## Plan
1. **Session info propagation** – add `agentPath` to the orchestrator session info (`src/ai-agent.ts`), ensuring it carries the agents-only label alongside `callPath`.
2. **Progress payloads** – extend progress event types and reporter to include `agentPath`, keeping existing fields intact (`src/types.ts`, `src/session-progress-reporter.ts`).
3. **Stub construction** – update `ToolsOrchestrator` to seed child stubs from the agent-only path and fall back safely (`src/tools/tools.ts`).
4. **Headend formatting** – switch OpenAI and Anthropic headends to display `agentPath` for reasoning bullets while leaving `callPath` available for metrics origin checks.

Each step should be lint-clean and individually testable.

## Implied Decisions
- Slack formatter continues using its current data; no changes unless explicitly requested.
- `callPath` contract stays tool-inclusive for telemetry and tracing; we simply offer `agentPath` in parallel.

## Testing Requirements
- `npm run lint`
- `npm run build`
- Manual streaming smoke test against OpenAI and Anthropic headends to confirm real-time updates show deduplicated agent paths.

## Documentation Updates Required
- None expected; behaviour aligns with existing intent once duplication is removed.
