# task_status Tool

## TL;DR
Tool for reporting task execution state and tracking completion status. Standalone calls are allowed (turn-consuming). Can signal explicit task completion via `status: completed` to force final turn.

## Source Files
- `src/tools/internal-provider.ts` - Tool definition and handler
- `src/session-tool-executor.ts` - Completion tracking
- `src/session-turn-runner.ts` - Final-turn forcing logic
- `src/context-guard.ts` - Forced final turn reason management
- `src/ai-agent.ts:598-600` - Enablement logic

## Tool Definition

**Name**: `agent__task_status`
**Short Name**: `task_status`

### Input Schema
```json
{
  "type": "object",
  "additionalProperties": false,
  "required": ["status", "done", "pending", "now", "ready_for_final_report", "need_to_run_more_tools"],
  "properties": {
    "status": {
      "type": "string",
      "enum": ["starting", "in-progress", "completed"],
      "description": "Current task status"
    },
    "done": {
      "type": "string",
      "description": "What has been completed so far (max 15 words guidance)"
    },
    "pending": {
      "type": "string",
      "description": "What remains to be done (max 15 words guidance)"
    },
    "now": {
      "type": "string",
      "description": "Current immediate step (max 15 words guidance)"
    },
    "ready_for_final_report": {
      "type": "boolean",
      "description": "Set to true when you have enough information to provide final report"
    },
    "need_to_run_more_tools": {
      "type": "boolean",
      "description": "Set to true when you need to run more tools"
    }
  }
}
```

## Behavioral Rules

### Rule 1: Standalone Calls Allowed
| Condition | Behavior |
|-----------|----------|
| `task_status` called alone (no other tools) | **Allowed** — display status, **turn consumed** |

### Rule 2: Explicit Completion
| Condition | Behavior |
|-----------|----------|
| `task_status` with `status: completed` | **Force final-turn** immediately (regardless of other tools) |

### Rule 3: Normal Usage
| Condition | Behavior |
|-----------|----------|
| `task_status` alongside real tools | Normal — just status reporting |

### Rule 4: Graceful Exhaustion
| Condition | Behavior |
|-----------|----------|
| Retry exhaustion (any cause) | **Force final-turn** instead of session failure |
| Exception | If already in final-turn, fail normally |

## Enablement Conditions

**Location**: `src/ai-agent.ts:598-600`

**Summary**: Enabled when:
1. `headendWantsProgressUpdates !== false`
2. AND (has external tools OR has subagents)

## State Tracking

### TurnRunnerState Fields
```typescript
interface TurnRunnerState {
  lastTaskStatusCompleted?: boolean;  // true when status: completed received
}
```

### ToolExecutionState Fields
```typescript
interface ToolExecutionState {
  lastTaskStatusCompleted?: boolean;
}
```

## Forced Final Turn Reasons

| Reason | Trigger |
|--------|---------|
| `task_status_completed` | Model called with `status: completed` |
| `retry_exhaustion` | All retry attempts exhausted |

**Precedence**: `task_status_completed` > `retry_exhaustion`

## Execution Flow

### Single Call Path (`internal-provider.ts`)
1. Validate status enum: `starting`, `in-progress`, `completed`
2. Extract done/pending/now strings
3. Update status via callback
4. Return result with `taskStatusCompleted` flag in extras

### Batch Call Path (`internal-provider.ts`)
1. Validate status enum (same as single call)
2. Extract done/pending/now strings
3. Update status via callback
4. Return result with completion signal

### Session Tracking (`session-tool-executor.ts`)
1. Track `taskStatusCompleted` from tool result extras
2. Do NOT reset completion flag when other tools run

### Final Turn Forcing (`session-turn-runner.ts`)
1. Check `lastTaskStatusCompleted === true` → force final turn
2. Check retry exhaustion → force final turn (lowest precedence)

## Tool Response

**Success Response**:
```json
{
  "status": "in-progress",
  "taskStatusCompleted": false
}
```

**Completion Response**:
```json
{
  "status": "completed",
  "taskStatusCompleted": true
}
```

## Use Cases

### Task Starting
```json
{
  "status": "starting",
  "done": "Initialized analysis",
  "pending": "Parse log entries",
  "now": "Analyze server logs",
  "ready_for_final_report": false,
  "need_to_run_more_tools": true
}
```

### In Progress
```json
{
  "status": "in-progress",
  "done": "Parsed 1000 log entries",
  "pending": "Search for error patterns",
  "now": "Find critical errors",
  "ready_for_final_report": false,
  "need_to_run_more_tools": true
}
```

### Task Completion
```json
{
  "status": "completed",
  "done": "Found 3 critical errors",
  "pending": "None",
  "now": "Server log analysis complete",
  "ready_for_final_report": true,
  "need_to_run_more_tools": false
}
```

## Configuration Effects

| Setting | Effect |
|---------|--------|
| `headendWantsProgressUpdates` | Master enable/disable |
| `tools` | External tools trigger enablement |
| `subAgents` | Sub-agents trigger enablement |

## Telemetry

**FinalReportMetricsRecord.forcedFinalReason**:
- `'task_status_completed'`
- `'retry_exhaustion'`

## Logging

**Status Update Log**:
```
[VRB] agent:progress - [PROGRESS UPDATE] Done text | Pending text | Goal text
```

## Invariants

1. **Optional tool**: May not be available based on configuration
2. **Non-blocking**: Does not block other tool execution
3. **Completion is sticky**: Once `status: completed` is received, final turn is forced
4. **Batch parity**: Batch handler validates and signals completion same as single call

## Business Logic Coverage

- **Conditional enablement**: The tool is only registered when `headendWantsProgressUpdates !== false` *and* the agent exposes external tools or subagents (`src/ai-agent.ts:598-610`).
- **OpTree + headend sync**: Every status update pushes a new opTree snapshot via `callbacks.onOpTree` (`src/ai-agent.ts:624-677`).
- **Completion forcing**: When `status: completed` is received, `taskStatusCompleted` is set and forces final turn regardless of other tools in the same turn.

## Test Coverage

**Phase 1 Tests**:
- `run-test-task-status-standalone-first` - Standalone call allowed, session continues
- `run-test-task-status-standalone-second` - Progress-only turns fail at maxTurns
- `run-test-task-status-completed` - Completion forces final
- `run-test-task-status-reset-on-real-tool` - Normal usage with real tools
- `run-test-retry-exhaustion-forces-final` - Retry exhaustion forces final
- `run-test-old-progress-report-rejected` - Old tool name rejected

## Troubleshooting

### Tool not available
- Check tool enablement conditions
- Verify external tools or subagents present
- Check headendWantsProgressUpdates setting

### Completion not forcing final turn
- Verify status is exactly `"completed"` (string)
- Check that completion flag is propagated in extras
- Verify batch handler returns completion signal

