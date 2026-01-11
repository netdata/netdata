# Call Path & Trace Context

## TL;DR
Hierarchical path tracking for multi-agent orchestration. Includes callPath, agentPath, turnPath, and transaction IDs (txnId, parentTxnId, originTxnId).

## Source Files
- `src/utils/call-path.ts` - Path manipulation utilities
- `src/ai-agent.ts:163-171` - Session trace state
- `src/ai-agent.ts:254-283` - Path helpers
- `src/ai-agent.ts:493-505` - Trace initialization

## Data Structures

### Trace Context Fields
```typescript
{
  txnId: string;           // This session's unique ID
  originTxnId: string;     // Root session ID (same for root agent)
  parentTxnId?: string;    // Parent session ID (undefined for root)
  callPath: string;        // Hierarchical path (e.g., "root:tool1:subtool2")
  agentPath: string;       // Agent hierarchy (e.g., "root:subagent1")
  turnPath: string;        // Turn location (e.g., "1.2-3.1")
}
```

### Session State
**Location**: `src/ai-agent.ts:163-171`

```typescript
private readonly txnId: string;
private readonly originTxnId?: string;
private readonly parentTxnId?: string;
private readonly callPath?: string;
private readonly agentPath: string;
private readonly turnPathPrefix: string;
```

## Path Components

### callPath
**Format**: `segment1:segment2:segment3`

**Purpose**: Full hierarchical path from root to current operation

**Examples**:
- Root agent: `agent`
- Sub-agent: `agent:researcher`
- Nested sub-agent: `agent:researcher:summarizer`
- Tool operation: `agent:researcher:search` (tool name sanitized; provider/namespace removed)

### agentPath
**Format**: `agent1:agent2:agent3`

**Purpose**: Agent hierarchy only (no tools)

**Examples**:
- Root: `agent`
- Child: `agent:researcher`
- Grandchild: `agent:researcher:fact_checker`

### turnPath
**Format**: `turn.subturn` or `parent-turn.subturn`

**Purpose**: Turn execution location

**Examples**:
- Turn 1, subturn 0: `1.0`
- Turn 2, subturn 3: `2.3`
- Parent turn 1.2, child turn 3.1: `1.2-3.1`

## Path Manipulation

### Normalization
**Location**: `src/utils/call-path.ts`

```typescript
export const normalizeCallPath = (path: string | undefined): string => {
  if (typeof path !== 'string') return '';
  const segments = path
    .split(':')
    .map((segment) => segment.trim())
    .filter((segment) => segment.length > 0 && segment !== 'tool');
  const normalized: string[] = [];
  segments.forEach((segment) => {
    if (normalized.length > 0 && normalized[normalized.length - 1] === segment) return;
    normalized.push(segment);
  });
  return normalized.join(':');
};
```

**Notes**:
- Removes placeholder `tool` segments emitted by some providers
- Collapses duplicate consecutive segments to keep paths stable during retries

### Appending Segments
**Location**: `src/utils/call-path.ts`

```typescript
export const appendCallPathSegment = (base: string | undefined, segment: string | undefined): string => {
  const normalizedSegment = typeof segment === 'string' ? segment.trim() : '';
  const normalizedBase = normalizeCallPath(base);
  if (normalizedSegment.length === 0 || normalizedSegment === 'tool') return normalizedBase;
  if (normalizedBase.length === 0) return normalizedSegment;
  const segments = normalizedBase.split(':');
  if (segments[segments.length - 1] === normalizedSegment) return normalizedBase;
  segments.push(normalizedSegment);
  return normalizeCallPath(segments.join(':'));
};
```

## Session Initialization

### Root Agent
**Location**: `src/ai-agent.ts:769-797`

```typescript
const sessionTxnId = crypto.randomUUID();
const inferredAgentPath = sessionConfig.agentPath ?? sessionConfig.agentId ?? 'agent';

const enrichedSessionConfig = {
  ...sessionConfig,
  trace: {
    selfId: sessionTxnId,
    originId: sessionConfig.trace?.originId ?? sessionTxnId,  // Self for root
    parentId: sessionConfig.trace?.parentId,  // undefined for root
    callPath: sessionConfig.trace?.callPath ?? inferredAgentPath,
    agentPath: inferredAgentPath,
  },
};
```

### Sub-Agent
**Location**: `src/ai-agent.ts:700-733`

```typescript
const childAgentPath = appendCallPathSegment(this.agentPath, normalizedChildName);

await subAgents.execute(name, parameters, {
  trace: {
    originId: this.originTxnId,      // Same as root
    parentId: this.txnId,             // This session
    callPath: childAgentPath,
    agentPath: childAgentPath,
    turnPath: parentTurnPath,
  },
});
```

## Turn Path Composition

**Location**: `src/ai-agent.ts:278-283`

```typescript
private composeTurnPath(turn: number, subturn: number): string {
  const segment = `${String(turn)}.${String(subturn)}`;
  return this.turnPathPrefix.length > 0
    ? `${this.turnPathPrefix}-${segment}`
    : segment;
}
```

**Examples**:
- Root turn 1, subturn 0: `1.0`
- Child with prefix `1.2`, turn 3, subturn 1: `1.2-3.1`

## Log Enrichment

### All Logs
**Location**: `src/ai-agent.ts:873-898`

```typescript
const enriched: LogEntry = {
  agentId: this.sessionConfig.agentId,
  agentPath: this.agentPath,
  callPath: this.agentPath,  // Base path
  txnId: this.txnId,
  parentTxnId: this.parentTxnId,
  originTxnId: this.originTxnId,
  turnPath: this.composeTurnPath(entry.turn, entry.subturn),
  ...entry,
};
```

### Tool Logs
**Location**: `src/ai-agent.ts:890-896`

```typescript
if (enriched.type === 'tool') {
  const toolName = this.extractToolNameForCallPath(enriched);
  enriched.callPath = appendCallPathSegment(this.agentPath, toolName);
}
```

## Transaction ID Propagation

### Root Session
- `txnId`: New UUID
- `originTxnId`: Same as txnId
- `parentTxnId`: undefined

### Child Session
- `txnId`: New UUID
- `originTxnId`: Parent's originTxnId (root's txnId)
- `parentTxnId`: Parent's txnId

### Grandchild Session
- `txnId`: New UUID
- `originTxnId`: Still root's txnId
- `parentTxnId`: Parent's txnId (child's txnId)

## Use Cases

### Distributed Tracing
```
Origin: abc-123
  └─ Session: abc-123 (root)
       └─ Session: def-456 (child)
            └─ Session: ghi-789 (grandchild)
```

### Log Filtering
```bash
# All logs for specific agent
grep "agentPath=root:researcher"

# All logs for origin session
grep "originTxnId=abc-123"

# Specific tool operation
grep "callPath=root:researcher:mcp__search"
```

### Hierarchical Status
```
root (turn 1.0)
  └─ researcher (turn 1.0-1.0)
       └─ fact_checker (turn 1.0-1.0-1.0)
```

## Configuration Effects

| Setting | Effect |
|---------|--------|
| `agentId` | Base identifier |
| `agentPath` | Explicit path override |
| `trace.callPath` | External path context |
| `trace.turnPath` | External turn context |
| `turnPathPrefix` | Turn numbering prefix |

## Telemetry

**Trace Context Attributes**:
- `ai.agent.call_path`
- `ai.session.txn_id`
- `ai.session.parent_txn_id`
- `ai.session.origin_txn_id`

## Logging

**Every Log Entry Includes**:
- `agentId`
- `agentPath`
- `callPath`
- `txnId`
- `parentTxnId`
- `originTxnId`
- `turnPath`

## Invariants

1. **Unique txnId**: Every session has unique transaction ID
2. **Stable originTxnId**: Same across entire hierarchy
3. **Parent chain**: parentTxnId forms valid chain to root
4. **Path normalization**: No leading/trailing colons
5. **Turn hierarchy**: Child turns prefixed with parent path

## Test Coverage

**Phase 2**:
- Path normalization
- Segment appending
- Turn path composition
- Trace propagation
- Log enrichment

**Gaps**:
- Deep nesting scenarios
- Concurrent sub-agent paths
- Path collision handling

## Business Logic Coverage (Verified 2025-11-16)

- **Tool call normalization**: Tool log entries sanitize names via `sanitizeToolName` + `clampToolName` before appending to `callPath`, preventing invalid characters from contaminating trace strings (`src/ai-agent.ts:883-896`, `src/utils.ts:28-88`).
- **Duplicate suppression**: `appendCallPathSegment` trims colons/whitespace and ignores empty segments so recursive agents never emit `agent::tool` artifacts (`src/utils/call-path.ts:4-60`).
- **Headend propagation**: `headendId`, `txnId`, `parentTxnId`, and `callPath` are injected into every `LogEntry` so Slack, REST, and CLI sinks can correlate nested activity (`src/ai-agent.ts:873-905`).
- **Turn-path inheritance**: Sub-agent invocations pass `turnPathPrefix` (e.g., parent `1.2`) so nested turns surface as `1.2-3.1` in opTree and progress logs, keeping parent/child ordering consistent (`src/ai-agent.ts:274-283`, `src/ai-agent.ts:700-760`).

## Troubleshooting

### Missing trace context
- Check session initialization
- Verify trace parameter propagation
- Check enrichment in addLog

### Wrong origin ID
- Check root session creation
- Verify propagation through hierarchy
- Check trace parameter merging

### Malformed path
- Check normalization function
- Verify segment characters
- Check colon escaping

### Turn path confusion
- Check turnPathPrefix setting
- Verify composition logic
- Check subturn incrementing

### Parent ID mismatch
- Check sub-agent invocation
- Verify trace context passing
- Check session config merging
