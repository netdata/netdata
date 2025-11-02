# Progress Update Display Issues - Research

## Problems Identified

1. **Empty bullet after "Turn N"**
2. **Brackets in callPath display** (e.g., `[web-research]` should be `web-research`)
3. **First status update parsed as clickable URL**
4. **Duplicate agent name in callPath** (e.g., `web-research:web-search:web-search` should be `web-research:web-search`)
5. **Final summary line has excessive asterisks**

## CallPath Construction Issue (Problem #4)

### Expected Behavior
- Pattern: `agent:tool:agent:tool:agent:tool`
- Should become: `agent:agent:agent:tool`
- Omit tool names between agents, keep only the final actual tool

### Example
```
Current:  web-research:web-search:web-search
Expected: web-research:web-search
```

From log line:
```
VRB 1.1-1.3 → MCP web-research:web-search:web-search: brave:brave_web_search(...)
                    ^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^  ^^^^^^^^^^^^^^^^^^^^^^
                              callPath                  console enricher (green)
```

### Correct Implementation Reference
**Console formatter enricher** (`src/logging/rich-format.ts:202`) does it RIGHT:
```typescript
const agent = event.agentPath ?? event.agentId ?? 'main';
```
Uses `agentPath`, not `callPath`.

### CallPath Construction Points

#### 1. Agent Session Creation (`src/ai-agent.ts:682-684`)
```typescript
const childAgentPath = this.agentPath.length > 0
  ? `${this.agentPath}:${normalizedChildName}`
  : normalizedChildName;
```
- Appends normalized child agent name to parent's agentPath
- Sets both `callPath` and `agentPath` to same value (line 705)
- **Appears correct**

#### 2. Session Info for ToolsOrchestrator (`src/ai-agent.ts:536-537`)
```typescript
{
  agentId: sessionConfig.agentId,
  callPath: this.getCallPathLabel(),  // ← Uses callPath
  ...
}
```

#### 3. getCallPathLabel (`src/ai-agent.ts:256-259`)
```typescript
private getCallPathLabel(): string {
  if (typeof this.callPath === 'string' && this.callPath.length > 0) return this.callPath;
  return this.agentPath;
}
```
- Prefers `this.callPath` over `this.agentPath`

#### 4. ToolsOrchestrator.getSessionCallPath (`src/tools/tools.ts:65-70`)
```typescript
private getSessionCallPath(): string {
  const { callPath, agentId } = this.sessionInfo;
  if (typeof callPath === 'string' && callPath.length > 0) return callPath;
  if (typeof agentId === 'string' && agentId.length > 0) return agentId;
  return 'agent';
}
```
- Uses sessionInfo.callPath if available

#### 5. Progress Events (`src/tools/tools.ts:252-253`)
```typescript
this.progress?.toolStarted({
  callPath: callPathLabel,  // ← From getSessionCallPath()
  ...
});
```

#### 6. Stub Session Creation (`src/tools/tools.ts:208-215`)
```typescript
const childCallPathBase = (typeof parentSession.callPath === 'string' && parentSession.callPath.length > 0)
  ? parentSession.callPath
  : (typeof parentSession.agentId === 'string' && parentSession.agentId.length > 0 ? parentSession.agentId : 'agent');
const stub: SessionNode = {
  agentId: childName,
  callPath: `${childCallPathBase}:${childName}`,
  ...
};
```
- **Question**: Does parentSession.callPath already contain an incorrect path?

#### 7. Headend sanitizeCallPath (`src/headends/anthropic-completions-headend.ts:362-371`)
```typescript
const sanitizeCallPath = (raw: string): string => {
  const segments = raw.split(':').filter((part) => part.length > 0);
  const result: string[] = [];
  segments.forEach((segment) => {
    if (segment === 'tool' && result.length > 0 && result[result.length - 1] === 'agent') return;
    result.push(segment);
  });
  if (result.length === 0) return raw;
  return result.join(':');
};
```
- **This is treating the symptom, not the cause**
- Tries to remove literal 'tool' string after literal 'agent' string
- But actual tool/agent names are used, not these literals

### Key Questions

1. **Why does console formatter work correctly?**
   - Uses `event.agentPath` not `event.callPath`
   - Where does `agentPath` come from in log events?

2. **Where is the duplicate segment introduced?**
   - Is it during child agent session creation?
   - Is it during progress event emission?
   - Is it during log event creation?

3. **Should we use `agentPath` everywhere instead of `callPath`?**
   - Or should `callPath` == `agentPath` for agent sessions?

### Fields to Investigate

Need to trace through a sub-agent call:
- `this.agentPath` in parent AIAgentSession
- `this.callPath` in parent AIAgentSession
- `childAgentPath` constructed for child
- `trace.callPath` passed to child
- `sessionInfo.callPath` in child's ToolsOrchestrator
- `event.callPath` in ProgressEvent
- `event.agentPath` in LogEntry

### Next Steps

1. Find where `agentPath` is set in LogEntry events (why console formatter gets it right)
2. Trace a sub-agent execution to find where duplicate is introduced
3. Determine if `callPath` should always equal `agentPath` for agents
4. Fix at the source, not in sanitizeCallPath

## Other Issues

### Empty Bullet (Problem #1)
- Location: `src/headends/anthropic-completions-headend.ts:391-399`
- `renderReasoning()` creates turn heading then immediately adds empty bullet if `turn.updates` starts empty

### Brackets (Problem #2)
- Location: `src/headends/anthropic-completions-headend.ts:294`
- `agentHeadingLabel = **[${escapeMarkdown(agent.toolName ?? agent.id)}]**`
- Should be: `**${escapeMarkdown(agent.toolName ?? agent.id)}**`

### URL Parsing (Problem #3)
- Need to investigate markdown rendering of first update line

### Asterisk Formatting (Problem #5)
- Location: Final summary line rendering
- Need to check markdown escaping in metrics formatting
