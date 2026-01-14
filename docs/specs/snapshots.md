# Session Snapshots

## TL;DR
Point-in-time captures of session state including opTree, accounting, and metadata. Persisted by default to `~/.ai-agent/sessions/` (gzipped JSON). All entry points (CLI, headends, library API) share the same defaults.

## Source Files
- `src/ai-agent.ts:368-386` - persistSessionSnapshot()
- `src/types.ts:514-528` - Snapshot payload definitions
- `src/session-tree.ts` - opTree snapshot structure
- `src/persistence.ts` - File persistence callbacks and defaults

## Data Structures

### SessionSnapshotPayload
**Location**: `src/types.ts:514-521`

```typescript
interface SessionSnapshotPayload {
  reason?: string;
  sessionId: string;
  originId: string;
  timestamp: number;
  snapshot: {
    version: number;
    opTree: SessionNode;
  };
}
```

### AccountingFlushPayload
**Location**: `src/types.ts:523-528`

```typescript
interface AccountingFlushPayload {
  sessionId: string;
  originId: string;
  timestamp: number;
  entries: AccountingEntry[];
}
```

## Snapshot Events

### Trigger Points

| Event | Reason String | Location |
|-------|--------------|----------|
| Sub-agent completion | `'subagent_finish'` | `src/ai-agent.ts:737` |
| Session end | `'final'` | `src/ai-agent.ts:1307` |

## Snapshot Flow

### 1. Capture Request
**Location**: `src/ai-agent.ts:368-386`

```typescript
private persistSessionSnapshot(reason?: string): void {
  const payload: SessionSnapshotPayload = {
    reason,
    sessionId: this.txnId,
    originId: this.originTxnId ?? this.txnId,
    timestamp: Date.now(),
    snapshot: {
      version: 1,
      opTree: this.opTree.getSession()
    },
  };
  this.emitEvent({ type: 'snapshot', payload });
}
```

### 2. OpTree Extraction
```typescript
this.opTree.getSession();  // Returns SessionNode
```

### 3. Callback Invocation
```typescript
this.emitEvent({ type: 'snapshot', payload });
```

### 4. Error Handling
```typescript
try {
  this.emitEvent({ type: 'snapshot', payload });
} catch (e) {
  warn(`persistSessionSnapshot failed: ${e.message}`);
}
```

## OpTree Structure

### SessionNode (Simplified)
```typescript
interface SessionNode {
  traceId: string;
  agentId?: string;
  callPath?: string;
  sessionTitle?: string;
  startedAt?: number;
  endedAt?: number;
  success?: boolean;
  error?: string;
  turns: TurnNode[];
  totals?: SessionTotals;
}
```

### TurnNode
```typescript
interface TurnNode {
  index: number;
  system?: boolean;
  label?: string;
  ops: OperationNode[];
  startedAt?: number;
  endedAt?: number;
}
```

### OperationNode
```typescript
interface OperationNode {
  opId: string;
  kind: 'llm' | 'tool' | 'system';
  startedAt: number;
  endedAt?: number;
  status?: 'ok' | 'failed';
  request?: object;
  response?: object;
  logs?: LogEntry[];
  accounting?: AccountingEntry[];
}
```

## Snapshot Contents

### Included Data
1. Session metadata (IDs, timestamps)
2. Complete opTree hierarchy
3. All turns and operations
4. Logs attached to operations
5. Accounting entries per operation
6. Session totals
7. LLM request/response payloads under `opTree.turns[].ops[].request.payload` and `opTree.turns[].ops[].response.payload`:
   - `payload.raw` = base64 of the full HTTP/SSE body capture (no placeholders; `[unavailable]` indicates a capture bug)
   - `payload.sdk` = base64 of serialized SDK request/response (verification copy)

### Excluded Data
1. Raw conversation (summarized in ops)
2. Tool parameters remain attached to opTree requests (not truncated at snapshot time)
3. Response bodies are preserved in opTree responses via `payload.raw` (full HTTP/SSE capture) and `payload.sdk` (SDK snapshot); any shortening reflects upstream limits (e.g., toolResponseMaxBytes or token-budget truncation), not snapshot truncation.

## Persistence Patterns

### File-Based
```typescript
onEvent: async (event) => {
  if (event.type !== 'snapshot') return;
  const filename = `session-${event.payload.sessionId}.json`;
  await fs.writeFile(filename, JSON.stringify(event.payload, null, 2));
}
```

### Database Storage
```typescript
onEvent: async (event) => {
  if (event.type !== 'snapshot') return;
  await db.sessions.upsert({
    id: event.payload.sessionId,
    origin: event.payload.originId,
    timestamp: event.payload.timestamp,
    snapshot: event.payload.snapshot,
  });
}
```

### Real-Time Streaming
```typescript
onEvent: async (event) => {
  if (event.type !== 'snapshot') return;
  await websocket.send(JSON.stringify(event.payload));
}
```

## Business Logic Coverage (Verified 2025-11-16)

- **Callback isolation**: `persistSessionSnapshot` emits `onEvent(type='snapshot')` and wraps the call in `try/catch`, logging `[warn] persistSessionSnapshot(...) failed` without interrupting the session (`src/ai-agent.ts:368-386`).
- **Reason tagging**: Snapshots are emitted with explicit reasons—`'subagent_finish'` after every child agent completes and `'final'` exactly once at session end—so downstream systems can distinguish mid-run updates from the terminal snapshot (`src/ai-agent.ts:737`, `src/ai-agent.ts:1307`).
- **Child captures via opTree**: Because each agent tool execution attaches the child SessionNode to its parent op, snapshot consumers automatically inherit the full nested structure without needing a secondary API (`src/ai-agent.ts:700-884`, `src/session-tree.ts:90-200`).

## Accounting Flush

### Final Flush
**Location**: `src/ai-agent.ts:388-408`

```typescript
private async flushAccounting(entries: AccountingEntry[]): Promise<void> {
  const payload: AccountingFlushPayload = {
    sessionId: this.txnId,
    originId: this.originTxnId ?? this.txnId,
    timestamp: Date.now(),
    entries: entries.map((entry) => ({ ...entry })),
  };
  this.emitEvent({ type: 'accounting_flush', payload });
}
```

### Timing
- Called once at session end
- After opTree finalization
- Before result return

## Use Cases

### Session Recovery
1. Load last snapshot
2. Reconstruct session state
3. Resume from checkpoint

### Monitoring Dashboard
1. Stream snapshots in real-time
2. Display session progress
3. Show turn-by-turn execution

### Debugging
1. Capture snapshot on error
2. Analyze operation sequence
3. Identify failure point

### Billing
1. Aggregate accounting entries
2. Calculate session cost
3. Report usage metrics

## Configuration Effects

| Setting | Effect |
|---------|--------|
| `persistence.sessionsDir` | Directory for snapshot files (default: `~/.ai-agent/sessions/`) |
| `persistence.billingFile` | Path for accounting ledger (default: `~/.ai-agent/accounting.jsonl`) |
| `onEvent(type='snapshot')` | Custom snapshot sink callback (overrides file persistence) |
| `onEvent(type='accounting_flush')` | Custom accounting sink callback (overrides file persistence) |

### Default Persistence
**Location**: `src/persistence.ts:22-30`

All entry points (CLI, headends, library API) apply the same defaults via `resolvePeristenceConfig()`:
- Sessions saved to `~/.ai-agent/sessions/{originId}.json.gz`
- Accounting appended to `~/.ai-agent/accounting.jsonl`

User-provided `persistence` config values override defaults. Custom callbacks (`onEvent` for `snapshot` / `accounting_flush`) bypass file persistence entirely.

## Telemetry

**Snapshot Events**:
- Timestamp
- Reason code
- Session/origin IDs
- Snapshot size

## Logging

**On Snapshot**:
- Warning on failure
- Reason string logged
- Error details captured

## Invariants

1. **Non-blocking**: Snapshot failures don't fail session
2. **Consistent state**: OpTree reflects current execution
3. **Version tagged**: Snapshot includes version number
4. **ID preservation**: Session/origin IDs maintained
5. **Final snapshot**: Always attempted at end
6. **Raw payload retention**: LLM request/response payloads are always preserved in opTree (base64, full body, including streaming SSE).

## Test Coverage

**Phase 2**:
- Snapshot payload structure
- Callback invocation
- Error handling
- Accounting flush

**Gaps**:
- Large snapshot serialization
- Concurrent snapshot requests
- Recovery scenarios

## Troubleshooting

### Snapshot not saved
- Check callback registration
- Verify callback doesn't throw
- Check async completion

### Missing data in snapshot
- Check opTree state
- Verify operation completion
- Check log attachment
- Confirm payload capture completed (LLM responses are recorded in full, even for streaming).

### Large snapshot size
- Check upstream truncation settings (toolResponseMaxBytes, token-budget truncation)
- Verify log pruning
- Consider selective snapshots

### Accounting mismatch
- Check flush timing
- Verify entry collection
- Compare with opTree

### Performance impact
- Check snapshot frequency
- Monitor callback latency
- Consider async processing
