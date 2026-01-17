# TODO: Fix Advisors, Routers, Handoff, Tool-Output Sub-Sessions

## TL;DR
Sub-sessions (advisors, router, handoff) have two critical issues:
1. Their LLM requests are NOT captured in the parent opTree/snapshots
2. They don't receive the full conversation history
Tool-output extraction is different: its child opTree is attached, but verbose logs are not forwarded for read-grep.

---

## Issue 1: Orchestration Sub-Session LLM Requests Missing from OpTree

### Problem
LLM requests made by orchestration sub-sessions are NOT recorded in the session's opTree, making debugging impossible. When examining session snapshots, we cannot see:
- What the advisor LLM received
- What the router LLM received
- What the handoff target LLM received
Note: tool_output extraction uses a different path and SHOULD appear as a child session (see table below).

### Affected Components

| Component | Status | Evidence |
|-----------|--------|----------|
| Advisors | ❌ NOT IN OPTREE | No child opTree attach path in `src/orchestration/spawn-child.ts` and `src/ai-agent.ts` merge only logs/accounting/childConversations (no opTree attach). |
| Router (orchestration) | ❌ NOT IN OPTREE | Router uses `spawnOrchestrationChild()` without opTree attach (same path as advisors). |
| Direct handoff | ❌ NOT IN OPTREE | Handoff uses `spawnOrchestrationChild()` without opTree attach (same path as advisors). |
| Tool-output extraction (full-chunked) | ✅ SHOULD BE IN OPTREE | Returns `childOpTree` from SessionTreeBuilder and tools attach it. |
| Tool-output extraction (read-grep) | ✅ SHOULD BE IN OPTREE | Runs child AIAgent, returns `result.opTree`, tools attach it. |
| Tool-output verbose logs (read-grep) | ❌ NOT FORWARDED | Child callbacks only record accounting, so logs/tool calls are not streamed to parent. |

### Expected Behavior
All LLM requests/responses from sub-sessions MUST appear in the opTree as child sessions or nested operations, so they are included in session snapshots.

### Root Cause (Orchestration Sub-Sessions)
Advisors/router/handoff run via `spawnOrchestrationChild()` which creates a new session, but the child session's opTree is NOT attached to the parent's opTree. The merge path only combines logs/accounting/childConversations.

### Snapshot Evidence (5e992547-3d3c-46c7-8d1d-bcbdb8539ab1)
- Snapshot opCounts: only `llm` and `system` ops (no `session` ops).
- Turn 1 user prompt includes `<advisory__...>` block, which only exists after advisors run.
- Conclusion: advisors ran, but no child opTree was attached to the parent.
- No tool_output evidence in this snapshot (no tool ops, no `tool_output` strings).

### Snapshot Evidence (6eff7054-ef43-465d-90db-b4c42495db0b)
- tool_output **is present** as a `session` op (not a `tool` op): `attributes.name == "tool_output"` with provider `tool-output`.
- tool_output has a child session: `agentId = "tool_output.read_grep"` with 4 turns.
- Child session includes LLM ops and tool_output_fs reads (`tool_output_fs__Read`).
- Conclusion: read-grep tool_output **does attach** its child opTree in this snapshot; missing verbose logs is a separate issue.

### Files to Investigate
- `src/orchestration/advisors.ts` - advisor execution
- `src/orchestration/spawn-child.ts` - child session spawning
- `src/ai-agent.ts` - main orchestration (lines 2331-2337 for advisors)
- `src/session-tree.ts` - opTree structure
- `src/orchestration/handoff.ts` - handoff path
- `src/orchestration/router.ts` - router path
- `src/tools/tools.ts` - opTree child attach for tool_output
- `src/tool-output/extractor.ts` - full-chunked + read-grep child opTree creation
- `src/tool-output/provider.ts` - returns `childOpTree` via extras
- `src/subagent-registry.ts` - sub-agent tools also drop history (if required)

---

## Issue 2: Advisors/Router/Handoff Don't Receive Conversation History

### Problem
Advisors receive ONLY the current user message, not the full conversation history. This makes them unable to provide contextual advice for follow-up questions.

### Evidence
In `src/orchestration/spawn-child.ts:184-186`:

```typescript
return await loaded.run(opts.systemTemplate, opts.userPrompt, {
  history: undefined,  // <-- HARDCODED TO UNDEFINED!
  callbacks: parentSession.callbacks,
  ...
});
```

### Data Flow (Current - Broken)
1. embed-headend receives `history` array with previous messages ✅
2. `spawnSession()` passes `history` to session config ✅
3. Session config stores it as `conversationHistory` ✅
4. `AIAgent.run()` calls `executeAdvisors()` with only `userPrompt: originalUserPrompt` ⚠️
5. `executeAdvisors()` calls `spawnOrchestrationChild()` with just `userPrompt` ⚠️
6. Router and handoff also call `spawnOrchestrationChild()` with only `userPrompt` ⚠️
7. `spawnOrchestrationChild()` explicitly sets `history: undefined` ❌

### Real-World Impact
User conversation:
1. User: "idrac"
2. Assistant: [detailed response about Dell iDRAC monitoring with SNMP]
3. User: "I don't understand why you prefer SNMP over IPMI..."

The advisor (support-request) only saw message #3 without context of messages #1 and #2, leading to a response that didn't acknowledge the previous conversation.

### Fix Required
1. Pass `parentSession.conversationHistory` through `executeAdvisors()`
2. Pass `parentSession.conversationHistory` through router and handoff paths
3. Update `spawnOrchestrationChild()` to accept and pass history
4. Change line 185 from `history: undefined` to `history: opts.history ?? parentSession.conversationHistory`

### Files to Modify
- `src/orchestration/spawn-child.ts` - line 185, add history parameter
- `src/orchestration/advisors.ts` - pass history through ExecuteAdvisorsOptions
- `src/orchestration/handoff.ts` - pass history through ExecuteHandoffOptions
- `src/ai-agent.ts` - pass conversationHistory to executeAdvisors/router/handoff

---

## Issue 3: Handoff Must Receive Conversation History (Verified)

### Problem (Verified)
Handoff targets (router-based and direct) do NOT receive conversation history because they call `spawnOrchestrationChild()` which hardcodes `history: undefined`.

### Evidence
- Router: `src/ai-agent.ts:2365-2391` uses `spawnOrchestrationChild()` with only `userPrompt`.
- Handoff: `src/orchestration/handoff.ts:64-81` uses `spawnOrchestrationChild()` with only `userPrompt`.

### Expected Behavior
When a session hands off to another agent, the target agent MUST receive the full conversation history to maintain context continuity.

---

## Issue 4: Tool-Output Subagent Visibility in Verbose Mode

### Problem (Verified)
When tool_output uses read-grep (child AIAgent), verbose logs and tool calls are NOT forwarded to the parent session because the child callbacks only capture accounting.

### Evidence
- `src/tool-output/extractor.ts:441-449` sets callbacks that only record accounting.

### Expected vs Actual
- Expected: `--verbose` shows child tool calls for read-grep.
- Actual: no child log events are forwarded; only accounting is captured.

### Note
The opTree child session SHOULD still be attached via tool_output extras. Lack of verbose logs does not necessarily mean missing opTree.

## Implementation Plan

### Phase 1: Fix History Propagation
1. Update `SpawnChildAgentOptions` interface to include optional `history` field
2. Update `spawnOrchestrationChild()` to pass history to child sessions
3. Update `executeAdvisors()` to pass history from parent session
4. Update `AIAgent.run()` to include conversationHistory when calling executeAdvisors/router/handoff

### Phase 2: Fix OpTree Capture
1. Attach orchestration child opTrees to parent opTree (advisors/router/handoff)
2. Verify tool_output child opTree is attached in snapshots
3. If needed, forward tool_output child logs for `--verbose` (read-grep)

### Phase 3: Verification
1. Create test case with multi-turn conversation
2. Verify advisor receives full history
3. Verify all sub-session LLM requests appear in snapshot
4. Verify handoff receives full history
5. Verify tool_output read-grep opTree attachment and verbose logs

---

## Test Scenario

```
User message 1: "idrac"
Assistant response 1: [iDRAC monitoring details]
User message 2: "Why SNMP over IPMI?"
```

After fix:
- Advisor should see both user messages and assistant response
- Snapshot should contain advisor's LLM request/response
- Advisor should provide context-aware advice

---

## Related Files

### Core Orchestration
- `src/ai-agent.ts` - main run() orchestration
- `src/orchestration/advisors.ts` - advisor execution
- `src/orchestration/spawn-child.ts` - child session spawning
- `src/orchestration/index.ts` - orchestration exports

### Session Management
- `src/session-tree.ts` - opTree structure
- `src/persistence.ts` - snapshot persistence
- `src/types.ts` - type definitions

### Reference Documentation
- `docs/skills/ai-agent-session-snapshots.md` - snapshot extraction guide
- `docs/specs/snapshots.md` - snapshot technical spec

---

## Notes / Corrections from Code Review

- tool_output modes are `full-chunked`, `read-grep`, `truncate`, `auto` (no explicit "subagent" mode).
- Docs/tool descriptions say full-chunked "spawns a subagent", but code uses a local SessionTreeBuilder + LLM client (no AIAgent).
- read-grep uses AIAgent (child session) but its callbacks only forward accounting, not logs.

---

## Priority
**HIGH** - These issues affect:
1. Debugging capability (can't see what sub-sessions did)
2. Conversation continuity (advisors give context-blind advice)
3. User experience (responses don't acknowledge prior context)
