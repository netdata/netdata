# Operation Tree (opTree)

## TL;DR
Hierarchical structure tracking session turns, operations, logs, accounting, and child sessions. Provides aggregated totals and ASCII rendering.

## Source Files
- `src/session-tree.ts` - SessionTreeBuilder implementation
- `src/ai-agent.ts:210` - opTree instance creation
- `src/ai-agent.ts:873-898` - Log attachment
- `src/ai-agent.ts:1819` - Accounting attachment

## Data Structures

### SessionNode
**Location**: `src/session-tree.ts:33-56`

```typescript
interface SessionNode {
  id: string;
  traceId?: string;
  agentId?: string;
  callPath?: string;
  sessionTitle: string;
  latestStatus?: string;
  startedAt: number;
  endedAt?: number;
  success?: boolean;
  error?: string;
  attributes?: Record<string, unknown>;
  totals?: SessionTotals;
  turns: TurnNode[];
}
```
**Notes**
- `latestStatus` is updated only by `agent__task_status` progress updates (not by thinking streams).
- Thinking output is stored under `OperationNode.reasoning` chunks and streamed via the `onThinking` callback.

### TurnNode
**Location**: `src/session-tree.ts:24-31`

```typescript
interface TurnNode {
  id: string;
  index: number;  // 1-based
  startedAt: number;
  endedAt?: number;
  attributes?: Record<string, unknown>;
  ops: OperationNode[];
}
```

### OperationNode
**Location**: `src/session-tree.ts:7-22`

```typescript
interface OperationNode {
  opId: string;
  kind: 'llm' | 'tool' | 'session' | 'system';
  startedAt: number;
  endedAt?: number;
  status?: 'ok' | 'failed';
  attributes?: Record<string, unknown>;
  logs: LogEntry[];
  accounting: AccountingEntry[];
  childSession?: SessionNode;
  reasoning?: { chunks: { text: string; ts: number }[]; final?: string };
  request?: { kind: 'llm'|'tool'; payload: unknown; size?: number };
  response?: { payload: unknown; size?: number; truncated?: boolean };
}
```
**Notes**
- LLM op payloads store both **HTTP/SSE capture** and **SDK snapshots** under `payload.raw` and `payload.sdk` (base64-encoded strings with `format` and `encoding` metadata). `payload.raw` is the full HTTP/SSE body; `payload.sdk` is the serialized SDK request/response used to verify payload integrity. If `payload.raw` ever shows `[unavailable]`, treat it as a capture bug.

### SessionTotals
**Location**: `src/session-tree.ts:46-54`

```typescript
interface SessionTotals {
  tokensIn: number;
  tokensOut: number;
  tokensCacheRead: number;
  tokensCacheWrite: number;
  costUsd?: number;
  toolsRun: number;
  agentsRun: number;
}
```

## Builder Operations

### Construction
**Location**: `src/session-tree.ts:67-78`

```typescript
new SessionTreeBuilder({
  traceId?: string;
  agentId?: string;
  callPath?: string;
  sessionTitle?: string;
  attributes?: Record<string, unknown>;
});
```

Creates:
- Unique session ID via `uid()`
- StartedAt timestamp
- Empty turns array

### Turn Management

**beginTurn(index, attributes)**
- Creates TurnNode with unique ID
- Sets 1-based index
- Pushes to turns array
- Indexes by turn number
- Recomputes totals

**endTurn(index, attributes)**
- Sets endedAt timestamp
- Merges attributes if provided
- Recomputes totals

### Operation Management

**beginOp(turnIndex, kind, attributes)**
- Creates OperationNode with unique ID
- Empty logs and accounting arrays
- Pushes to turn's ops array
- Indexes by opId
- Returns opId for reference

**endOp(opId, status, attributes)**
- Sets endedAt timestamp
- Sets status ('ok' | 'failed')
- Merges attributes
- Recomputes totals

### Data Attachment

**appendLog(opId, log)**
- Adds log entry to operation
- Computes and attaches stable path label

**appendAccounting(opId, acc)**
- Adds accounting entry to operation
- Recomputes totals

**appendReasoningChunk(opId, text)**
- Adds timestamped reasoning chunk
- Accumulates for streaming reasoning

**setReasoningFinal(opId, text)**
- Sets final reasoning summary
- Overwrites if called multiple times

**setRequest(opId, req)**
- Sets or merges request payload
- Tracks request size

**setResponse(opId, res)**
- Sets or merges response payload
- Tracks response size and truncation

**attachChildSession(opId, child)**
- Embeds child SessionNode in operation
- For sub-agent execution tracking
- Recomputes totals to include child

### Session Operations

**setSessionTitle(title)**
- Updates session title string
- Used by task_status tool

**setLatestStatus(status)**
- Updates current status string
- Used by task_status tool

**endSession(success, error)**
- Sets endedAt timestamp
- Sets success boolean
- Sets error string if provided
- Verifies totals consistency (warns on mismatch)

**getSession()**
- Returns SessionNode reference
- Used for snapshots and callbacks

## Totals Recomputation

**Location**: `src/session-tree.ts:388-446`

### Trigger Points
- beginTurn
- endTurn
- beginOp
- endOp
- appendAccounting
- attachChildSession

### Algorithm
1. Initialize counters to 0
2. Recursively visit all nodes
3. For each operation:
   - If kind='tool': increment toolsRun
   - For each accounting entry:
     - If type='llm': accumulate tokens and cost
4. For child sessions: increment agentsRun and recurse
5. Normalize cost to 4 decimal places
6. Store totals on session node

### Aggregated Fields
- `tokensIn`: Sum of inputTokens
- `tokensOut`: Sum of outputTokens
- `tokensCacheRead`: Sum of cacheReadInputTokens or cachedTokens
- `tokensCacheWrite`: Sum of cacheWriteInputTokens
- `costUsd`: Sum of costUsd (normalized)
- `toolsRun`: Count of tool operations
- `agentsRun`: Count of sessions (including root)

## Path Computation

**getOpPath(opId)**
**Location**: `src/session-tree.ts:167-189`

Format: `turn-op` or `turn-op.turn-op` for nested

Example paths:
- `1-1` - Turn 1, operation 1
- `2-3` - Turn 2, operation 3
- `1-2.3-1` - Turn 1, op 2 (session), then turn 3, op 1

Used for:
- Stable bijective identification
- Log enrichment
- Debug tracing

## Flattening

**flatten()**
**Location**: `src/session-tree.ts:251-267`

Returns:
- `logs`: All logs from all operations, sorted by timestamp
- `accounting`: All accounting entries, sorted by timestamp

Used for:
- Legacy consumers
- Flat event streams
- Simple aggregation

## ASCII Rendering

**renderAscii()**
**Location**: `src/session-tree.ts:280-355`

Output format:
```
SessionTree {id} trace={traceId} agent={agentId} callPath={callPath}
├─ started={ISO} ended={ISO} dur={ms} success={bool} error={str} | tokens...
├─ turns={count}
├─ Turn#1 {ISO} → {ISO} ({ms})
│  system: {truncated prompt}
│  user:   {truncated prompt}
│  ├─ LLM op={id} [provider:model] {ISO} → {ISO} ({ms}) status={ok|failed}
│  │  logs={count} accounting={count}
│  │  request: {truncated payload}
│  │  response: {truncated payload}
│  └─ TOOL op={id} [name] {ISO} → {ISO} ({ms}) status={ok|failed}
│     logs={count} accounting={count}
└─ Turn#2 ...
```

Features:
- Box-drawing characters
- Timestamp formatting
- Duration calculation
- Attribute extraction (provider, model, name)
- Prompt summaries (truncated to 120 chars)
- Request/response previews (truncated to 200 chars)
- Recursive child session rendering

## Invariants

1. **Unique IDs**: Session, turn, operation IDs generated via uid()
2. **Index consistency**: Turn index stored and mapped
3. **Totals sync**: Recomputed after every mutation
4. **Path stability**: getOpPath returns consistent bijective paths
5. **Timestamp ordering**: All timestamps based on Date.now()
6. **Child containment**: Child sessions fully embedded in operations
7. **Mismatch detection**: endSession warns on totals discrepancy

## Configuration Effects

| Setting | Effect |
|---------|--------|
| `agentId` | Stored in session node |
| `traceId` | Stored in session node |
| `callPath` | Stored in session node |
| `sessionTitle` | Initial title value |

## Telemetry

**Session attributes**:
- Session ID
- Trace ID
- Agent ID
- Call path
- Start/end timestamps
- Success/error status
- Aggregated totals

## Logging

**Warnings emitted**:
- Totals mismatch on endSession
- Path computation failures

## Events

**onOpTree callback**:
- Receives SessionNode reference
- Called on status/title updates
- Called on progress reports

## Business Logic Coverage (Verified 2025-11-16)

- **Reasoning capture**: `appendReasoningChunk` timestamps thinking deltas per operation and `setReasoningFinal` stores the merged summary so headends can render thinking transcripts after the fact (`src/session-tree.ts:200-260`).
- **Child embedding**: `attachChildSession` nests sub-agent SessionNodes, preserving their entire turn/op trees; totals recomputation aggregates child tokens/costs into the parent (`src/session-tree.ts:300-446`).
- **Status/title publishing**: `setLatestStatus` and `setSessionTitle` update the root node and trigger callback invocations so UI clients see live progress (`src/session-tree.ts:120-190`).
- **Operation payload retention**: Request/response payloads are stored in base64 under `payload.raw` (full HTTP/SSE capture) and `payload.sdk` (serialized SDK request/response). `truncated` is only set when upstream limits already shortened the payload (e.g., toolResponseMaxBytes or token-budget truncation) (`src/session-tree.ts:230-320`, `src/ai-agent.ts`, `src/session-turn-runner.ts`, `src/llm-client.ts`).

## Use Cases

### Session Monitoring
1. Watch turns being added
2. Track operation completion
3. Monitor token consumption
4. Calculate running cost

### Debug Analysis
1. Identify slow operations
2. Trace error propagation
3. Analyze request/response payloads
4. Review reasoning streams

### Session Recovery
1. Serialize SessionNode
2. Reconstruct turn state
3. Resume from checkpoint
4. Validate consistency

### Cost Attribution
1. Track per-operation cost
2. Aggregate by agent
3. Separate LLM vs tool usage
4. Compare cache effectiveness

## Test Coverage

**Phase 2**:
- Builder construction
- Turn lifecycle
- Operation lifecycle
- Log attachment
- Accounting aggregation
- Totals computation
- Path generation
- ASCII rendering

**Gaps**:
- Deep nesting scenarios (3+ levels)
- Concurrent operation handling
- Large tree serialization performance
- Mismatch detection accuracy

## Troubleshooting

### Totals not updating
- Check mutation triggers recomputation
- Verify accounting entries have correct type
- Check child session attachment

### Path computation wrong
- Check opId exists in index
- Verify turn/operation ordering
- Check child session boundaries

### ASCII rendering issues
- Check all operations have timestamps
- Verify attributes are objects
- Check truncation logic

### Missing logs/accounting
- Check opId is valid
- Verify appendLog/appendAccounting called
- Check flatten() sorting

### Child session not visible
- Check attachChildSession called
- Verify child session properly constructed
- Check recursive rendering
