# TODO: Unified Observability Architecture

## TL;DR

Unify logging, opTree, and tracing into a single coherent system where:
- opTree is the **single source of truth** for session structure
- Logs are **attached to opTree nodes**, not independent
- Every warning/error has a **slug** (stable identifier) for harness tests and debugging
- Payloads are **always captured** (never truncated) for post-mortem analysis
- Multiple interfaces (CLI, TUI, Web) consume the **same opTree structure**

---

## Analysis: Current State

### 1. opTree Structure (session-tree.ts)

**Current hierarchy:**
```
SessionNode
├─ turns: TurnNode[]
│   ├─ index: number (1-based)
│   ├─ ops: OperationNode[]
│   │   ├─ kind: 'llm' | 'tool' | 'session' | 'system'
│   │   ├─ request?: { kind, payload, size }
│   │   ├─ response?: { payload, size, truncated }
│   │   ├─ logs: LogEntry[]
│   │   ├─ accounting: AccountingEntry[]
│   │   └─ childSession?: SessionNode (for subagents)
```

**Problems:**
- No distinction between INIT phase, TURNs, and FIN phase
- No distinction between LLM-REQUEST, LLM-RETRY, TOOL calls
- `OperationNode.kind` is too coarse (`'llm'` vs `'tool'`)
- Retries are not tracked as separate operations
- No explicit phase markers

### 2. Logging System

**Current LogEntry structure (types.ts:108-149):**
```typescript
interface LogEntry {
  timestamp: number;
  severity: 'VRB' | 'WRN' | 'ERR' | 'TRC' | 'THK' | 'FIN';
  turn: number;
  subturn: number;
  path?: string;                    // opTree path like "1-2"
  direction: 'request' | 'response';
  type: 'llm' | 'tool';
  toolKind?: 'mcp' | 'rest' | 'agent' | 'command';
  remoteIdentifier: string;         // "provider:model" or "agent:something"
  message: string;                  // Human-readable, unstable
  llmRequestPayload?: LogPayload;
  llmResponsePayload?: LogPayload;
  // ... more fields
}
```

**Problems:**
- `message` is human-readable, changes frequently, breaks harness tests
- No **slug** (stable machine identifier) for each log type
- Payloads often NOT attached to warning/error logs
- Tests string-match `message` field (fragile)
- `remoteIdentifier` is used as pseudo-slug but not well-defined

### 3. Journald Sink (logging/journald-sink.ts)

**Current behavior:**
- Already captures `AI_LLM_REQUEST`, `AI_LLM_RESPONSE` fields (lines 286-301)
- BUT: payloads are only attached when the LogEntry has them
- Warning/error logs often DON'T have payloads attached

### 4. Message IDs (logging/message-ids.ts)

**Current registry (only 7 entries):**
```typescript
const MESSAGE_ID_REGISTRY = {
  'agent:init': '8f8c1c67-...',
  'agent:fin': '4cb03727-...',
  'agent:pricing': 'f53bb387-...',
  'agent:limits': '50a01c4a-...',
  'agent:EXIT-FINAL-ANSWER': '6f3db0fb-...',
  'agent:batch': 'c9a43c6d-...',
  'agent:progress': 'b8e3466f-...',
};
```

**Problems:**
- Only 7 slugs defined; dozens of log points exist without slugs
- UUID format is for journald MESSAGE_ID, not for code references
- No central catalog of all log points

### 5. Harness Tests (tests/phase1/runner.ts)

**Current approach:**
```typescript
// Fragile string matching
const log = result.logs.find(entry =>
  entry.message.includes('Unknown tool requested'));
invariant(log !== undefined, 'Expected warning');

// Checking remoteIdentifier as pseudo-slug
const exitLog = result.logs.find(log =>
  log.remoteIdentifier === 'agent:EXIT-NO-LLM-RESPONSE');
```

**Problems:**
- Tests break when message text changes
- No structured way to find specific events
- opTree is available but not used; tests examine flat `result.logs`

### 6. Snapshot Persistence (persistence.ts)

**Current behavior:**
- Saves `opTree` as gzipped JSON to `sessionsDir`
- Payloads are base64-encoded when attached to logs
- BUT: no CLI to read/query snapshots

---

## Target Architecture

### opTree Structure (NEW)

```
SessionNode
├─ phase: 'init' | 'running' | 'fin' | 'error'
├─ init: InitNode                    // NEW: explicit INIT phase
│   ├─ startedAt: number
│   ├─ endedAt?: number
│   ├─ logs: LogEntry[]
│   └─ config: { providers, tools, prompts }
├─ turns: TurnNode[]
│   ├─ index: number (1-based)
│   ├─ startedAt: number
│   ├─ endedAt?: number
│   ├─ llmRequest: LLMRequestNode    // NEW: explicit LLM request
│   │   ├─ attempt: number (0-based)
│   │   ├─ provider: string
│   │   ├─ model: string
│   │   ├─ request: { payload, size }
│   │   ├─ startedAt: number
│   │   ├─ endedAt?: number
│   │   └─ logs: LogEntry[]
│   ├─ llmRetries: LLMRequestNode[]  // NEW: explicit retries
│   ├─ tools: ToolNode[]             // NEW: tools as separate array
│   │   ├─ index: number (1-based within turn)
│   │   ├─ kind: 'mcp' | 'rest' | 'agent' | 'command'
│   │   ├─ name: string
│   │   ├─ request: { payload, size }
│   │   ├─ response?: { payload, size }
│   │   ├─ startedAt: number
│   │   ├─ endedAt?: number
│   │   ├─ status: 'ok' | 'failed' | 'timeout'
│   │   ├─ childSession?: SessionNode  // For subagent tools
│   │   ├─ logs: LogEntry[]
│   │   └─ accounting?: ToolAccountingEntry
│   ├─ llmResponse: LLMResponseNode  // NEW: explicit LLM response
│   │   ├─ response: { payload, size }
│   │   ├─ tokens: TokenUsage
│   │   ├─ stopReason: string
│   │   ├─ logs: LogEntry[]
│   │   └─ accounting: LLMAccountingEntry
│   └─ logs: LogEntry[]              // Turn-level logs (warnings, etc.)
├─ fin: FinNode                      // NEW: explicit FIN phase
│   ├─ startedAt: number
│   ├─ endedAt?: number
│   ├─ success: boolean
│   ├─ error?: string
│   ├─ finalReport?: FinalReportPayload
│   └─ logs: LogEntry[]
└─ totals: { tokensIn, tokensOut, ... }
```

### Log Entry Structure (ENHANCED)

```typescript
interface LogEntry {
  // Existing fields...
  timestamp: number;
  severity: 'VRB' | 'WRN' | 'ERR' | 'TRC' | 'THK' | 'FIN';

  // NEW: Stable slug (machine identifier)
  slug: string;  // e.g., "llm.synthetic_retry", "tool.timeout", "context.exceeded"

  // NEW: Code location for debugging
  source?: {
    file: string;    // e.g., "session-turn-runner.ts"
    line: number;    // e.g., 1080
    function?: string; // e.g., "executeTurn"
  };

  // Existing but better defined
  message: string;  // Human-readable (can change)

  // ENHANCED: Always attach payloads for WRN/ERR
  context?: {
    llmRequest?: { payload: unknown; size: number };
    llmResponse?: { payload: unknown; size: number };
    toolRequest?: { payload: unknown; size: number };
    toolResponse?: { payload: unknown; size: number };
    conversation?: ConversationMessage[];  // Optional: full context
  };
}
```

### Slug Registry (NEW)

Create `src/logging/slug-registry.ts`:

```typescript
// Central catalog of all log slugs
export const LOG_SLUGS = {
  // Session lifecycle
  'session.init': { severity: 'VRB', description: 'Session initialization started' },
  'session.init_complete': { severity: 'VRB', description: 'Session initialization complete' },
  'session.fin': { severity: 'FIN', description: 'Session completed' },
  'session.error': { severity: 'ERR', description: 'Session failed with error' },

  // LLM operations
  'llm.request': { severity: 'VRB', description: 'LLM request sent' },
  'llm.response': { severity: 'VRB', description: 'LLM response received' },
  'llm.retry': { severity: 'WRN', description: 'LLM request retry' },
  'llm.synthetic_retry': { severity: 'WRN', description: 'Content without tool calls, retrying' },
  'llm.empty_response': { severity: 'WRN', description: 'Empty response without tools' },
  'llm.rate_limited': { severity: 'WRN', description: 'Rate limited, backing off' },
  'llm.auth_error': { severity: 'ERR', description: 'Authentication failed' },
  'llm.timeout': { severity: 'WRN', description: 'LLM request timed out' },

  // Tool operations
  'tool.request': { severity: 'VRB', description: 'Tool invocation started' },
  'tool.response': { severity: 'VRB', description: 'Tool returned result' },
  'tool.failed': { severity: 'WRN', description: 'Tool execution failed' },
  'tool.timeout': { severity: 'WRN', description: 'Tool execution timed out' },
  'tool.not_found': { severity: 'WRN', description: 'Unknown tool requested' },
  'tool.invalid_params': { severity: 'WRN', description: 'Invalid tool parameters' },

  // Context management
  'context.exceeded': { severity: 'WRN', description: 'Context window limit exceeded' },
  'context.shrink': { severity: 'VRB', description: 'Context shrinking applied' },
  'context.final_turn': { severity: 'WRN', description: 'Forcing final turn' },

  // Final report
  'final_report.missing': { severity: 'WRN', description: 'Final report not provided' },
  'final_report.extracted': { severity: 'WRN', description: 'Final report extracted from text' },
  'final_report.received': { severity: 'VRB', description: 'Final report received' },

  // MCP operations
  'mcp.connect': { severity: 'VRB', description: 'MCP server connected' },
  'mcp.disconnect': { severity: 'WRN', description: 'MCP server disconnected' },
  'mcp.restart': { severity: 'WRN', description: 'MCP server restarted' },
  'mcp.error': { severity: 'ERR', description: 'MCP server error' },

  // ... more slugs
} as const;

export type LogSlug = keyof typeof LOG_SLUGS;
```

---

## Rules

### Rule 1: opTree is the Single Source of Truth
- All session structure must be in opTree
- Logs are children of opTree nodes, not independent
- CLI/TUI/Web all read opTree, not raw logs

### Rule 2: Every Log Has a Slug
- Every `this.log()` call must specify a slug
- Slugs are stable across releases
- Harness tests reference slugs, not message text

### Rule 3: VRB Logs Are Excluded from opTree by Default
- VRB (verbose) logs go to journal/stderr only
- WRN and ERR logs are automatically attached to their parent opTree node
- Option to include VRB in opTree for debugging

### Rule 4: WRN/ERR Logs Auto-Attach Context
- When logging WRN or ERR, automatically attach:
  - Current LLM request/response payloads (if in LLM operation)
  - Current tool request/response payloads (if in tool operation)
  - The slug is logged alongside the message

### Rule 5: Timestamps Enable Timeline Derivation
- Every node has `startedAt` and `endedAt`
- Every log has `timestamp`
- Timeline can be reconstructed from any opTree

### Rule 6: Payloads Are Never Truncated in Snapshots
- Raw, unprocessed payloads stored in snapshot files
- Truncation only for display/journal (configurable limit)
- Snapshots are the source for debugging

### Rule 7: One Log Call = One Slug
- Each distinct log point in code has exactly one slug
- No slug reuse across different log points
- Slug includes source file:line for debugging

### Rule 8: Accounting Is Part of Response Nodes
- LLMAccountingEntry attached to LLMResponseNode
- ToolAccountingEntry attached to ToolNode
- Not floating in separate arrays

### Rule 9: Child Sessions Are Fully Embedded
- Subagent sessions are complete SessionNodes
- Nested in parent ToolNode.childSession
- Same structure recursively

### Rule 10: Harness Tests Use opTree, Not Logs
- Tests query opTree structure:
  ```typescript
  expect(session.turns[0].llmRetries.length).toBe(2);
  expect(session.turns[0].tools[0].status).toBe('failed');
  expect(findLog(session, 'llm.synthetic_retry')).toBeDefined();
  ```
- No string matching on message content

---

## Implementation Plan

### Phase 1: Slug Registry & Log Enhancement (Foundation)

1. Create `src/logging/slug-registry.ts` with all slugs
2. Add `slug` field to `LogEntry` interface
3. Add `source?: { file, line }` field to `LogEntry`
4. Update all `this.log()` calls to include slug
5. Create helper: `logWithSlug(slug, message, context)`
6. Add context auto-capture for WRN/ERR

### Phase 2: opTree Structure Refactor

1. Add `InitNode` and `FinNode` types
2. Refactor `TurnNode` to have explicit phases:
   - `llmRequest: LLMRequestNode`
   - `llmRetries: LLMRequestNode[]`
   - `tools: ToolNode[]`
   - `llmResponse: LLMResponseNode`
3. Move accounting into response nodes
4. Update `SessionTreeBuilder` methods
5. Migrate existing callers

### Phase 3: Harness Test Migration

1. Create opTree query helpers:
   - `findTurn(session, index)`
   - `findTool(turn, name)`
   - `findLogBySlug(node, slug)`
   - `countRetries(turn)`
2. Rewrite tests to use opTree queries
3. Remove all string-matching on `message`
4. Add tests for new structure invariants

### Phase 4: Observability Interfaces

1. **CLI**: `ai-agent snapshot <session-id> [--turn N] [--slug X]`
   - Dump full session or specific turn/operation
   - Filter by slug
   - Output JSON or human-readable
2. **TUI**: ncurses-based session explorer
   - Tree view of session structure
   - Drill into any node
   - Search by slug
3. **Web UI**: Single-page app
   - Timeline visualization
   - Cost breakdown
   - Full payload inspection

### Phase 5: Live Journal Integration

1. Ensure all WRN/ERR logs have payloads attached
2. Add slug to journald fields: `AI_SLUG=llm.synthetic_retry`
3. Update Netdata queries to filter by slug
4. Dashboard for common failure patterns

---

## Decisions Made

### Decision 1: Slug Naming Convention
**DECIDED: A) Dot-separated hierarchy**
- Examples: `llm.retry`, `tool.timeout`, `context.exceeded`
- Matches Go/Java conventions, hierarchical, grep-friendly

### Decision 2: Payload Storage Format
**DECIDED: A) Raw JSON**
- Human-readable in snapshots
- Enables direct inspection without decoding
- Trade-off: larger files (acceptable for debugging priority)

### Decision 3: VRB Log Handling
**DECIDED: B) Opt-in via flag, BUT session settings always in opTree**
- VRB logs excluded from opTree by default
- INIT phase with session config IS included (not optional)
- Flag `--verbose-in-tree` to include VRB for debugging sessions

### Decision 4: Migration Strategy
**PENDING - Need to understand implications first**
- opTree structure is used in multiple places
- Must map all consumers before deciding approach
- Work incrementally to bring the result

---

## opTree Usage Analysis (for Migration Planning)

### Summary: 127 references across 14 files

| File | Count | Role |
|------|-------|------|
| `ai-agent.ts` | 51 | Main orchestrator, creates/manages opTree |
| `tools/tools.ts` | 25 | Tool execution, creates tool ops |
| `session-turn-runner.ts` | 15 | Turn execution, creates LLM ops |
| `session-tree.ts` | 11 | Definition (SessionTreeBuilder class) |
| `subagent-registry.ts` | 6 | Subagent tree attachment |
| `tests/smoke-logger.ts` | 3 | Test utilities |
| `server/status-aggregator.ts` | 3 | Live status aggregation |
| `tools/agent-provider.ts` | 2 | Subagent integration |
| `server/session-manager.ts` | 2 | Session lifecycle |
| `cli.ts` | 2 | CLI output |
| `tests/phase1/runner.ts` | 2 | Harness tests |
| `persistence.ts` | 1 | Snapshot saving |
| `tools/types.ts` | 2 | Type definitions |
| `types.ts` | 2 | Type definitions |

### SessionTreeBuilder API Currently Used

```typescript
// Creation
new SessionTreeBuilder(meta)

// Session-level
getSession(): SessionNode
setSessionTitle(title)
setLatestStatus(status)
endSession(success, error?)

// Turn-level
beginTurn(index, attributes?)
endTurn(index, attributes?)

// Operation-level
beginOp(turnIndex, kind, attributes?): opId
endOp(opId, status, attributes?)
getOpPath(opId): string

// Data attachment
setRequest(opId, { kind, payload, size })
setResponse(opId, { payload, size, truncated? })
appendLog(opId, log)
appendAccounting(opId, acc)
attachChildSession(opId, child)

// Reasoning (streaming)
appendReasoningChunk(opId, text)
setReasoningFinal(opId, text)

// Output
flatten(): { logs, accounting }
renderAscii(): string
```

### Current Call Sites by Pattern

#### 1. Turn Management (ai-agent.ts, session-turn-runner.ts)
```typescript
// BEGIN: Turn 0 is "system" turn for init/fin
this.opTree.beginTurn(0, { system: true, label: 'init' });
this.opTree.beginTurn(currentTurn, turnAttrs);

// END
this.opTree.endTurn(currentTurn, attrs);
this.opTree.endTurn(0);
```
**Impact:** Turn 0 is special (init/fin). Need to preserve or migrate this pattern.

#### 2. LLM Operations (session-turn-runner.ts)
```typescript
// BEGIN
const llmOpId = this.ctx.opTree.beginOp(currentTurn, 'llm', { provider, model, isFinalTurn });

// DATA
this.ctx.opTree.setRequest(llmOpId, { kind: 'llm', payload: {...}, size });
this.ctx.opTree.setResponse(llmOpId, { payload: {...}, size, truncated });
this.ctx.opTree.appendAccounting(llmOpId, accountingEntry);

// END
this.ctx.opTree.endOp(llmOpId, 'ok'|'failed', { latency });
```
**Impact:** Currently one 'llm' op per turn. Retries are NOT tracked as separate ops.

#### 3. Tool Operations (tools/tools.ts)
```typescript
// BEGIN
const opId = this.opTree.beginOp(ctx.turn, opKind, { name, provider, kind });

// DATA
this.opTree.setRequest(opId, { kind: 'tool', payload, size });
this.opTree.setResponse(opId, { payload, size, truncated });
this.opTree.appendAccounting(opId, acc);

// SUBAGENT
this.opTree.attachChildSession(opId, childTree);

// END
this.opTree.endOp(opId, 'ok'|'failed', { latency, error?, size? });
```
**Impact:** Tools already well-structured. Subagent attachment works.

#### 4. Init/Fin (ai-agent.ts)
```typescript
// INIT - multiple places, inconsistent
this.opTree.beginTurn(0, { system: true, label: 'init' });
const sysInitOp = this.opTree.beginOp(0, 'system', { label: 'init' });
this.opTree.endOp(sysInitOp, 'ok');

// FIN - repeated pattern in multiple error handlers
const finOp = this.opTree.beginOp(0, 'system', { label: 'fin' });
this.opTree.endOp(finOp, 'ok');
this.opTree.endTurn(0);
this.opTree.endSession(success, error);
```
**Impact:** Init/Fin currently use Turn 0. Code is duplicated across multiple exit paths.

#### 5. Callbacks/Snapshots
```typescript
this.sessionConfig.callbacks?.onOpTree?.(this.opTree.getSession());
```
**Impact:** External consumers (UI, persistence) receive `SessionNode`. Any structure change affects them.

### Breaking Change Risk Assessment

| Change | Risk | Affected |
|--------|------|----------|
| Add new fields to SessionNode | LOW | Consumers ignore unknown fields |
| Add new node types (InitNode, FinNode) | MEDIUM | Consumers may expect specific structure |
| Change TurnNode.ops structure | HIGH | All turn iteration code breaks |
| Change OperationNode structure | HIGH | All op handling code breaks |
| Remove/rename existing fields | HIGH | All consumers break |

### Recommended Migration Path

**Step 1: Add, don't change** (safe)
- Add `slug` field to LogEntry
- Add `init?: InitNode` to SessionNode (optional)
- Add `fin?: FinNode` to SessionNode (optional)
- Keep existing `turns[0]` pattern working

**Step 2: Enhance logging** (safe)
- Create slug registry
- Update log calls to include slug
- Auto-attach payloads to WRN/ERR

**Step 3: Dual-write period** (medium risk)
- Write to both old structure AND new fields
- New consumers can use new fields
- Old consumers still work

**Step 4: Migrate consumers** (controlled)
- Update harness tests to use new structure
- Update CLI/TUI/Web to use new structure
- Deprecation warnings for old access patterns

**Step 5: Remove legacy** (after validation)
- Remove dual-write
- Clean up old patterns

---

## Testing Requirements

1. **Slug Coverage Test**: Every `this.log()` call must have a registered slug
2. **opTree Structure Tests**: Validate INIT → TURNs → FIN sequence
3. **Payload Attachment Tests**: WRN/ERR logs must have context
4. **Timeline Consistency Tests**: All timestamps monotonically increasing
5. **Accounting Integrity Tests**: Totals match sum of parts

---

## Documentation Updates Required

- [ ] `docs/SPECS.md`: Add observability architecture section
- [ ] `docs/AI-AGENT-GUIDE.md`: Update slug catalog
- [ ] `README.md`: Add snapshot/debugging CLI usage
- [ ] New: `docs/OBSERVABILITY.md`: Full architecture documentation

---

## Notes

- This is a significant refactor but enables:
  - Stable harness tests that don't break on message changes
  - Complete debugging capability via snapshots
  - Consistent view across CLI/TUI/Web/Journal
  - Machine-queryable session structure
