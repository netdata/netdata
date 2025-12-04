# progress_report Tool

## TL;DR
Optional tool for reporting intermediate status during execution. Updates session title and emits progress events. Enabled only when external tools or subagents present. Progress uses native tool_calls; the XML slot for progress is deprecated.

## Source Files
- `src/tools/internal-provider.ts` - Tool definition and handler
- `src/ai-agent.ts:598-600` - Enablement logic
- `src/ai-agent.ts:624-657` - updateStatus callback
- `src/ai-agent.ts:659-677` - setTitle callback

## Tool Definition

**Name**: `agent__progress_report`
**Short Name**: `progress_report`

### Input Schema
```json
{
  "type": "object",
  "properties": {
    "progress": {
      "type": "string",
      "description": "Current status message"
    },
    "title": {
      "type": "string",
      "description": "Optional session title"
    },
    "emoji": {
      "type": "string",
      "description": "Optional emoji for title"
    }
  },
  "required": ["progress"]
}
```

## Enablement Conditions

**Location**: `src/ai-agent.ts:598-600`

```typescript
const hasNonInternalDeclaredTools = declaredTools.some((toolName) => {
  // Check if not batch, progress_report, or final_report
  return true; // Has external tools
});
const hasSubAgentsConfigured = Array.isArray(this.sessionConfig.subAgents) && this.sessionConfig.subAgents.length > 0;
const wantsProgressUpdates = this.sessionConfig.headendWantsProgressUpdates !== false;
const enableProgressTool = wantsProgressUpdates && (hasNonInternalDeclaredTools || hasSubAgentsConfigured);
```

**Summary**: Enabled when:
1. `headendWantsProgressUpdates !== false`
2. AND (has external tools OR has subagents)

## Execution Flow

### XML Transport
- Progress always follows native tool_calls; the XML progress slot is no longer emitted.

### 1. Status Update
**Location**: `src/ai-agent.ts:624-657`

```typescript
updateStatus: (text: string) => {
  const t = text.trim();
  if (t.length > 0) {
    // Update opTree status
    this.opTree.setLatestStatus(t);

    // Emit opTree update
    this.sessionConfig.callbacks?.onOpTree?.(this.opTree.getSession());

    // Log progress
    const entry: LogEntry = {
      severity: 'VRB',
      type: 'tool',
      remoteIdentifier: 'agent:progress',
      bold: true,
      message: `[PROGRESS UPDATE] ${t}`,
    };
    this.log(entry);

    // Emit progress event
    this.progressReporter.agentUpdate({
      message: t,
      // ...trace context
    });
  }
}
```

### 2. Title Update (Optional)
**Location**: `src/ai-agent.ts:659-677`

```typescript
setTitle: (title: string, emoji?: string) => {
  const clean = title.trim();
  if (clean.length === 0) return;

  this.sessionTitle = { title: clean, emoji, ts: Date.now() };
  this.opTree.setSessionTitle(clean);

  const entry: LogEntry = {
    severity: 'VRB',
    type: 'tool',
    remoteIdentifier: 'agent:title',
    bold: true,
    message: emoji ? `${emoji} ${clean}` : clean,
  };
  this.log(entry);
}
```

## Tool Response

**Success Response**:
```
Progress update recorded: [status message]
```

## Session State Updates

### OpTree
- `setLatestStatus(text)` - Update current status
- `setSessionTitle(title)` - Update session title

### Progress Events
- `agent_update` event emitted via progressReporter
- Contains: callPath, agentId, agentPath, message, trace context

### Logs
- VRB severity
- Bold emphasis
- Remote identifier: `agent:progress` or `agent:title`

## Use Cases

### Long-Running Tasks
```json
{
  "progress": "Searching documentation for API endpoints..."
}
```

### Multi-Step Operations
```json
{
  "progress": "Step 2/5: Analyzing code dependencies"
}
```

### Session Labeling
```json
{
  "progress": "Starting research",
  "title": "API Documentation Research",
  "emoji": "📚"
}
```

## Configuration Effects

| Setting | Effect |
|---------|--------|
| `headendWantsProgressUpdates` | Master enable/disable |
| `tools` | External tools trigger enablement |
| `subAgents` | Sub-agents trigger enablement |

## Telemetry

**Progress Events Include**:
- Timestamp
- Message text
- Agent context (callPath, agentId)
- Transaction IDs

## Logging

**Status Update Log**:
```
[VRB] agent:progress - [PROGRESS UPDATE] Current status message
```

**Title Update Log**:
```
[VRB] agent:title - 📚 Session Title
```

## Events

- `agent_update` - Progress event emitted
- OpTree snapshot update via callback

## Invariants

1. **Optional tool**: May not be available
2. **Non-blocking**: Does not affect execution flow
3. **Idempotent**: Multiple calls are safe
4. **Empty check**: Empty strings ignored

## Business Logic Coverage (Verified 2025-11-16)

- **Conditional enablement**: The tool is only registered when `headendWantsProgressUpdates !== false` *and* the agent exposes external tools or subagents, preventing progress spam for simple prompts (`src/ai-agent.ts:598-610`).
- **OpTree + headend sync**: Every status/title update pushes a new opTree snapshot via `callbacks.onOpTree`, so Slack/REST/OpenAI headends can render progress bars in real time (`src/ai-agent.ts:624-677`).
- **Final-turn guard**: The internal provider warns if `progress_report` is called in the same turn as `final_report` so models learn to separate updates from final answers (`src/tools/internal-provider.ts:172-197`).

## Test Coverage

**Phase 1**:
- Tool enablement logic
- Status update propagation
- Title setting
- Event emission

**Gaps**:
- High-frequency update performance
- Emoji handling edge cases
- Concurrent update scenarios

## Troubleshooting

### Tool not available
- Check tool enablement conditions
- Verify external tools or subagents present
- Check headendWantsProgressUpdates setting

### Status not appearing
- Check text is non-empty after trim
- Verify opTree callback registered
- Check log filtering

### Title not updating
- Check title is non-empty
- Verify setSessionTitle callback
- Check opTree state

### Events not received
- Check progressReporter initialized
- Verify onProgress callback registered
- Check event propagation
